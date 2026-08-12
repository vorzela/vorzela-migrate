package query

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"sync"
)

// RelationKind names the shape of an association.
type RelationKind string

const (
	RelationBelongsTo     RelationKind = "belongs_to"
	RelationHasOne        RelationKind = "has_one"
	RelationHasMany       RelationKind = "has_many"
	RelationBelongsToMany RelationKind = "belongs_to_many"
)

// Relation is the descriptive metadata for an association. Generated model code
// registers one per foreign key so `.With("posts")` can resolve by name.
type Relation struct {
	Name            string
	Kind            RelationKind
	Table           string
	LocalKey        string
	ForeignKey      string
	PivotTable      string
	PivotLocalKey   string
	PivotForeignKey string
}

// Loader fills one relation for a batch of parents. Implementations must issue a
// bounded number of queries for the whole batch — never one per parent.
type Loader[P any] func(ctx context.Context, db DB, parents []*P) error

type registeredRelation struct {
	rel  Relation
	load any // Loader[P]
}

var (
	relationMu sync.RWMutex
	relations  = map[reflect.Type]map[string]registeredRelation{}
)

// RegisterRelation binds a named relation on P to a batched loader.
func RegisterRelation[P any](rel Relation, load Loader[P]) {
	if rel.Name == "" {
		panic("vorm/query: RegisterRelation requires Relation.Name")
	}
	if load == nil {
		panic("vorm/query: RegisterRelation requires a loader")
	}
	t := reflect.TypeFor[P]()
	relationMu.Lock()
	defer relationMu.Unlock()
	if relations[t] == nil {
		relations[t] = map[string]registeredRelation{}
	}
	relations[t][rel.Name] = registeredRelation{rel: rel, load: load}
}

// Relations lists the relations registered for P, ordered by name.
func Relations[P any]() []Relation {
	t := reflect.TypeFor[P]()
	relationMu.RLock()
	defer relationMu.RUnlock()
	m := relations[t]
	out := make([]Relation, 0, len(m))
	for _, r := range m {
		out = append(out, r.rel)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func lookupLoader[P any](name string) (Loader[P], bool) {
	t := reflect.TypeFor[P]()
	relationMu.RLock()
	defer relationMu.RUnlock()
	r, ok := relations[t][name]
	if !ok {
		return nil, false
	}
	l, ok := r.load.(Loader[P])
	return l, ok
}

func knownRelationNames[P any]() []string {
	rels := Relations[P]()
	names := make([]string, len(rels))
	for i, r := range rels {
		names[i] = r.Name
	}
	return names
}

// loadRelations runs the requested loaders against the result batch.
func loadRelations[T any](ctx context.Context, db DB, names []string, rows []T) error {
	if len(names) == 0 || len(rows) == 0 {
		return nil
	}
	parents := make([]*T, len(rows))
	for i := range rows {
		parents[i] = &rows[i]
	}
	for _, name := range names {
		load, ok := lookupLoader[T](name)
		if !ok {
			return validationErr("with", "", "unknown relation %q (registered: %v)", name, knownRelationNames[T]())
		}
		if err := load(ctx, db, parents); err != nil {
			return fmt.Errorf("vorm/query: load relation %q: %w", name, err)
		}
	}
	return nil
}

// HasMany configures a one-to-many load: children whose ForeignKey matches the
// parent's local key. Resolved in a single IN query for the whole batch.
type HasMany[P any, C any] struct {
	Related    *Entity[C]
	ForeignKey string
	ParentKey  func(*P) any
	ChildKey   func(*C) any
	Assign     func(*P, []C)
	Modify     func(*Builder[C]) *Builder[C]
}

// LoadHasMany resolves a has-many (or has-one, via Assign) relation for parents.
func LoadHasMany[P any, C any](ctx context.Context, db DB, parents []*P, opts HasMany[P, C]) error {
	if opts.Related == nil || opts.ParentKey == nil || opts.ChildKey == nil || opts.Assign == nil {
		return validationErr("with", "", "LoadHasMany requires Related, ParentKey, ChildKey and Assign")
	}
	keys, order := distinctKeys(parents, opts.ParentKey)
	if len(keys) == 0 {
		return nil
	}
	b := opts.Related.New().WhereIn(opts.ForeignKey, keys...)
	if opts.Modify != nil {
		b = opts.Modify(b)
	}
	children, err := b.Get(ctx, db)
	if err != nil {
		return err
	}

	grouped := make(map[any][]C, len(order))
	for i := range children {
		k := normalizeKey(opts.ChildKey(&children[i]))
		grouped[k] = append(grouped[k], children[i])
	}
	for _, p := range parents {
		opts.Assign(p, grouped[normalizeKey(opts.ParentKey(p))])
	}
	return nil
}

// BelongsTo configures an inverse load: the single owner row a parent points at.
type BelongsTo[P any, C any] struct {
	Related   *Entity[C]
	OwnerKey  string // column on the related table, defaults to its primary key
	ParentKey func(*P) any
	ChildKey  func(*C) any
	Assign    func(*P, *C)
	Modify    func(*Builder[C]) *Builder[C]
}

// LoadBelongsTo resolves a belongs-to relation for parents in one query.
func LoadBelongsTo[P any, C any](ctx context.Context, db DB, parents []*P, opts BelongsTo[P, C]) error {
	if opts.Related == nil || opts.ParentKey == nil || opts.ChildKey == nil || opts.Assign == nil {
		return validationErr("with", "", "LoadBelongsTo requires Related, ParentKey, ChildKey and Assign")
	}
	ownerKey := opts.OwnerKey
	if ownerKey == "" {
		ownerKey = opts.Related.meta.PrimaryKey
	}
	keys, _ := distinctKeys(parents, opts.ParentKey)
	if len(keys) == 0 {
		return nil
	}
	b := opts.Related.New().WhereIn(ownerKey, keys...)
	if opts.Modify != nil {
		b = opts.Modify(b)
	}
	owners, err := b.Get(ctx, db)
	if err != nil {
		return err
	}

	byKey := make(map[any]*C, len(owners))
	for i := range owners {
		byKey[normalizeKey(opts.ChildKey(&owners[i]))] = &owners[i]
	}
	for _, p := range parents {
		if owner, ok := byKey[normalizeKey(opts.ParentKey(p))]; ok {
			opts.Assign(p, owner)
		}
	}
	return nil
}

// BelongsToMany configures a pivot-table load. It costs two queries for the whole
// batch: one over the pivot, one over the related table.
type BelongsToMany[P any, C any] struct {
	Related         *Entity[C]
	PivotTable      string
	PivotParentKey  string
	PivotRelatedKey string
	RelatedKey      string // related column the pivot points at, defaults to its primary key
	ParentKey       func(*P) any
	ChildKey        func(*C) any
	Assign          func(*P, []C)
	Modify          func(*Builder[C]) *Builder[C]
}

// LoadBelongsToMany resolves a many-to-many relation through a pivot table.
func LoadBelongsToMany[P any, C any](ctx context.Context, db DB, parents []*P, opts BelongsToMany[P, C]) error {
	if opts.Related == nil || opts.ParentKey == nil || opts.ChildKey == nil || opts.Assign == nil {
		return validationErr("with", "", "LoadBelongsToMany requires Related, ParentKey, ChildKey and Assign")
	}
	if opts.PivotTable == "" || opts.PivotParentKey == "" || opts.PivotRelatedKey == "" {
		return validationErr("with", "", "LoadBelongsToMany requires PivotTable, PivotParentKey and PivotRelatedKey")
	}
	relatedKey := opts.RelatedKey
	if relatedKey == "" {
		relatedKey = opts.Related.meta.PrimaryKey
	}
	parentKeys, _ := distinctKeys(parents, opts.ParentKey)
	if len(parentKeys) == 0 {
		return nil
	}

	links, err := loadPivotLinks(ctx, db, DefaultDialect(), opts.PivotTable, opts.PivotParentKey, opts.PivotRelatedKey, parentKeys)
	if err != nil {
		return err
	}
	if len(links) == 0 {
		return nil
	}

	relatedKeys := make([]any, 0, len(links))
	seen := make(map[any]bool, len(links))
	for _, l := range links {
		k := normalizeKey(l.related)
		if seen[k] {
			continue
		}
		seen[k] = true
		relatedKeys = append(relatedKeys, l.related)
	}

	b := opts.Related.New().WhereIn(relatedKey, relatedKeys...)
	if opts.Modify != nil {
		b = opts.Modify(b)
	}
	children, err := b.Get(ctx, db)
	if err != nil {
		return err
	}
	byKey := make(map[any]C, len(children))
	for i := range children {
		byKey[normalizeKey(opts.ChildKey(&children[i]))] = children[i]
	}

	grouped := make(map[any][]C, len(parentKeys))
	for _, l := range links {
		if c, ok := byKey[normalizeKey(l.related)]; ok {
			pk := normalizeKey(l.parent)
			grouped[pk] = append(grouped[pk], c)
		}
	}
	for _, p := range parents {
		opts.Assign(p, grouped[normalizeKey(opts.ParentKey(p))])
	}
	return nil
}

type pivotLink struct {
	parent  any
	related any
}

func loadPivotLinks(ctx context.Context, db DB, dialect Dialect, table, parentCol, relatedCol string, parentKeys []any) ([]pivotLink, error) {
	for _, ident := range []string{table, parentCol, relatedCol} {
		if err := SafeIdent(ident); err != nil {
			return nil, err
		}
	}
	tableQ, err := QuoteIdent(dialect, table)
	if err != nil {
		return nil, err
	}
	parentQ, err := QuoteIdent(dialect, parentCol)
	if err != nil {
		return nil, err
	}
	relatedQ, err := QuoteIdent(dialect, relatedCol)
	if err != nil {
		return nil, err
	}

	holders := make([]string, len(parentKeys))
	for i := range parentKeys {
		if dialect == DialectMySQL {
			holders[i] = "?"
		} else {
			holders[i] = fmt.Sprintf("$%d", i+1)
		}
	}
	sqlText := fmt.Sprintf("SELECT %s, %s FROM %s WHERE %s IN (%s)",
		parentQ, relatedQ, tableQ, parentQ, joinComma(holders))

	obs := observe(ctx, "select", table, sqlText, parentKeys)
	rows, err := db.QueryContext(ctx, sqlText, parentKeys...)
	if err != nil {
		return nil, obs.done(ctx, 0, wrapErr("select", table, sqlText, len(parentKeys), err))
	}
	defer rows.Close()

	var out []pivotLink
	for rows.Next() {
		var parent, related any
		if err := rows.Scan(&parent, &related); err != nil {
			return nil, obs.done(ctx, len(out), wrapErr("scan", table, sqlText, len(parentKeys), err))
		}
		out = append(out, pivotLink{parent: parent, related: related})
	}
	if err := rows.Err(); err != nil {
		return nil, obs.done(ctx, len(out), wrapErr("select", table, sqlText, len(parentKeys), err))
	}
	_ = obs.done(ctx, len(out), nil)
	return out, nil
}

// distinctKeys collects non-nil parent keys, preserving first-seen order.
func distinctKeys[P any](parents []*P, key func(*P) any) ([]any, []any) {
	seen := make(map[any]bool, len(parents))
	keys := make([]any, 0, len(parents))
	order := make([]any, 0, len(parents))
	for _, p := range parents {
		if p == nil {
			continue
		}
		raw := key(p)
		if raw == nil {
			continue
		}
		k := normalizeKey(raw)
		if k == nil || seen[k] {
			continue
		}
		seen[k] = true
		keys = append(keys, raw)
		order = append(order, k)
	}
	return keys, order
}

// normalizeKey makes join keys comparable across driver representations
// (int32 vs int64, []byte vs string, *T vs T).
func normalizeKey(v any) any {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case []byte:
		return string(x)
	case string:
		return x
	case int:
		return int64(x)
	case int8:
		return int64(x)
	case int16:
		return int64(x)
	case int32:
		return int64(x)
	case int64:
		return x
	case uint:
		return int64(x)
	case uint8:
		return int64(x)
	case uint16:
		return int64(x)
	case uint32:
		return int64(x)
	case uint64:
		return int64(x)
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		return normalizeKey(rv.Elem().Interface())
	}
	switch rv.Kind() {
	case reflect.String:
		return rv.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(rv.Uint())
	}
	if rv.Type().Comparable() {
		return v
	}
	return fmt.Sprint(v)
}

func joinComma(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	}
	n := len(parts) - 1
	for _, p := range parts {
		n += len(p)
	}
	b := make([]byte, 0, n)
	for i, p := range parts {
		if i > 0 {
			b = append(b, ',', ' ')
		}
		b = append(b, p...)
	}
	return string(b)
}
