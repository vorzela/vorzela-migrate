package generate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/vorzela/vorm/introspect"
	"github.com/vorzela/vorm/query"
)

// relPlan is one association to generate on a single owning table.
type relPlan struct {
	Owner        string
	Name         string
	Field        string
	Kind         query.RelationKind
	RelatedTable string

	// LocalKey is the owner column carrying the join value; ForeignKey is the
	// related column it is matched against. This holds for belongs-to and
	// has-many alike, only the direction of the FK differs.
	LocalKey   string
	ForeignKey string

	Pivot           string
	PivotOwnerKey   string
	PivotRelatedKey string
	RelatedKey      string
}

// tableFields resolves the Go field name for every column, applying the same
// collision handling the model emitter uses so relation loaders reference the
// identifiers that actually exist.
func tableFields(t introspect.Table) (map[string]string, map[string]bool) {
	fields := make(map[string]string, len(t.Columns))
	taken := map[string]bool{}
	for _, c := range t.Columns {
		fields[c.Name] = uniqueName(GoFieldName(c.Name), taken)
	}
	return fields, taken
}

// planRelations derives associations from foreign keys, keyed by owning table.
func planRelations(tables []introspect.Table) map[string][]relPlan {
	known := make(map[string]introspect.Table, len(tables))
	for _, t := range tables {
		known[strings.ToLower(t.Name)] = t
	}

	out := map[string][]relPlan{}
	add := func(p relPlan) { out[p.Owner] = append(out[p.Owner], p) }

	pivots := map[string]bool{}
	for _, t := range tables {
		if a, b, ok := pivotSides(t, known); ok {
			pivots[strings.ToLower(t.Name)] = true
			add(belongsToManyPlan(t, a, b))
			add(belongsToManyPlan(t, b, a))
		}
	}

	for _, t := range tables {
		fkCounts := map[string]int{}
		for _, fk := range t.ForeignKeys {
			if len(fk.Columns) == 1 && len(fk.RefColumns) == 1 {
				fkCounts[strings.ToLower(fk.RefTable)]++
			}
		}

		for _, fk := range t.ForeignKeys {
			if len(fk.Columns) != 1 || len(fk.RefColumns) != 1 {
				continue // composite keys need explicit modelling
			}
			ref, ok := known[strings.ToLower(fk.RefTable)]
			if !ok {
				continue
			}
			fkCol := fk.Columns[0]
			refCol := fk.RefColumns[0]
			base := relationBase(fkCol, ref.Name)

			add(relPlan{
				Owner:        t.Name,
				Name:         base,
				Field:        GoName(base),
				Kind:         query.RelationBelongsTo,
				RelatedTable: ref.Name,
				LocalKey:     fkCol,
				ForeignKey:   refCol,
			})

			if pivots[strings.ToLower(t.Name)] {
				continue // the pivot's own rows are exposed as belongs-to-many
			}

			kind := query.RelationHasMany
			inverse := Plural(Singular(t.Name))
			if uniqueSingleColumn(t, fkCol) {
				kind = query.RelationHasOne
				inverse = Singular(t.Name)
			}
			// Two FKs to the same table (author_id, editor_id) would collide, so
			// qualify the inverse side with the column's base name.
			if fkCounts[strings.ToLower(ref.Name)] > 1 {
				inverse = base + "_" + inverse
			}
			add(relPlan{
				Owner:        ref.Name,
				Name:         inverse,
				Field:        GoName(inverse),
				Kind:         kind,
				RelatedTable: t.Name,
				LocalKey:     refCol,
				ForeignKey:   fkCol,
			})
		}
	}

	for owner := range out {
		out[owner] = dedupeRelations(out[owner])
	}
	return out
}

// relationBase names a belongs-to after the column (author_id → author) so
// multiple FKs to one table stay distinguishable.
func relationBase(fkCol, refTable string) string {
	if trimmed := strings.TrimSuffix(fkCol, "_id"); trimmed != fkCol && trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSuffix(fkCol, "id"); trimmed != fkCol && trimmed != "" {
		return strings.TrimSuffix(trimmed, "_")
	}
	return Singular(refTable)
}

func uniqueSingleColumn(t introspect.Table, col string) bool {
	for _, idx := range t.Indexes {
		if idx.Unique && len(idx.Columns) == 1 && strings.EqualFold(idx.Columns[0], col) {
			return true
		}
	}
	return false
}

// pivotSides reports the two foreign keys of a join table. A pivot carries
// nothing but its own key, timestamps and the two references.
func pivotSides(t introspect.Table, known map[string]introspect.Table) (introspect.ForeignKey, introspect.ForeignKey, bool) {
	var fks []introspect.ForeignKey
	for _, fk := range t.ForeignKeys {
		if len(fk.Columns) == 1 && len(fk.RefColumns) == 1 {
			if _, ok := known[strings.ToLower(fk.RefTable)]; ok {
				fks = append(fks, fk)
			}
		}
	}
	if len(fks) != 2 || strings.EqualFold(fks[0].RefTable, fks[1].RefTable) {
		return introspect.ForeignKey{}, introspect.ForeignKey{}, false
	}

	structural := map[string]bool{
		strings.ToLower(fks[0].Columns[0]): true,
		strings.ToLower(fks[1].Columns[0]): true,
		"id":                               true, "created_at": true, "updated_at": true, "deleted_at": true,
	}
	for _, c := range t.Columns {
		if !structural[strings.ToLower(c.Name)] {
			return introspect.ForeignKey{}, introspect.ForeignKey{}, false
		}
	}
	return fks[0], fks[1], true
}

func belongsToManyPlan(pivot introspect.Table, own, other introspect.ForeignKey) relPlan {
	name := Plural(Singular(other.RefTable))
	return relPlan{
		Owner:           own.RefTable,
		Name:            name,
		Field:           GoName(name),
		Kind:            query.RelationBelongsToMany,
		RelatedTable:    other.RefTable,
		LocalKey:        own.RefColumns[0],
		RelatedKey:      other.RefColumns[0],
		Pivot:           pivot.Name,
		PivotOwnerKey:   own.Columns[0],
		PivotRelatedKey: other.Columns[0],
	}
}

func dedupeRelations(plans []relPlan) []relPlan {
	sort.SliceStable(plans, func(i, j int) bool { return plans[i].Name < plans[j].Name })
	seen := map[string]bool{}
	out := plans[:0]
	for _, p := range plans {
		key := p.Name
		for i := 2; seen[key]; i++ {
			key = fmt.Sprintf("%s_%d", p.Name, i)
		}
		seen[key] = true
		p.Name = key
		p.Field = GoName(key)
		out = append(out, p)
	}
	return out
}

func anyRelations(rels map[string][]relPlan) bool {
	for _, v := range rels {
		if len(v) > 0 {
			return true
		}
	}
	return false
}
