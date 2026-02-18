package migration

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// RelationType represents the type of database relationship
type RelationType string

const (
	BelongsTo  RelationType = "belongs-to"   // One-to-Many (FK on this table)
	OneToOne   RelationType = "one-to-one"   // One-to-One (unique FK on this table)
	ManyToMany RelationType = "many-to-many" // Pivot/junction table
)

// Relationship defines a table relationship for migration generation
type Relationship struct {
	Type        RelationType
	TargetTable string // The referenced table (e.g., "users")
}

// Singularize converts a plural table name to its singular form.
// Handles common English patterns used in database naming.
func Singularize(s string) string {
	if s == "" {
		return s
	}

	// Common irregular plurals found in databases
	irregulars := map[string]string{
		"people":   "person",
		"children": "child",
		"men":      "man",
		"women":    "woman",
		"mice":     "mouse",
		"geese":    "goose",
		"teeth":    "tooth",
		"feet":     "foot",
		"data":     "datum",
		"media":    "medium",
		"criteria": "criterion",
		"indices":  "index",
		"matrices": "matrix",
		"vertices": "vertex",
		"analyses": "analysis",
	}

	if singular, ok := irregulars[s]; ok {
		return singular
	}

	// "ies" → "y" (categories → category, companies → company)
	if strings.HasSuffix(s, "ies") && len(s) > 3 {
		return s[:len(s)-3] + "y"
	}

	// "ves" → "fe" (knives → knife, lives → life)
	if strings.HasSuffix(s, "ves") && len(s) > 3 {
		return s[:len(s)-3] + "fe"
	}

	// "ches", "shes", "sses", "xes", "zes" → remove "es"
	for _, suffix := range []string{"ches", "shes", "sses", "xes", "zes"} {
		if strings.HasSuffix(s, suffix) {
			return s[:len(s)-2]
		}
	}

	// Generic "s" → remove (users → user, posts → post)
	// Preserve words ending in "ss" (address), "us" (status), "is" (basis)
	if strings.HasSuffix(s, "s") && !strings.HasSuffix(s, "ss") &&
		!strings.HasSuffix(s, "us") && !strings.HasSuffix(s, "is") {
		return s[:len(s)-1]
	}

	return s
}

// ForeignKeyColumn returns the FK column name for a referenced table.
// e.g., "users" → "user_id", "categories" → "category_id"
func ForeignKeyColumn(tableName string) string {
	return Singularize(tableName) + "_id"
}

// PivotTableName creates an alphabetically-sorted pivot table name.
// Uses singular forms: ("users", "roles") → "role_user"
func PivotTableName(table1, table2 string) string {
	s1 := Singularize(table1)
	s2 := Singularize(table2)

	names := []string{s1, s2}
	sort.Strings(names)

	return names[0] + "_" + names[1]
}

// GeneratePivotMigration generates a complete many-to-many pivot table migration
func GeneratePivotMigration(table1, table2 string, opts CreateMigrationOptions) string {
	now := time.Now().Format("2006-01-02 15:04:05")
	pivotName := PivotTableName(table1, table2)
	upperName := strings.ToUpper("CREATE_" + pivotName + "_TABLE")

	// Sort tables alphabetically for consistent column ordering
	tables := []string{table1, table2}
	sort.Strings(tables)

	fk1 := ForeignKeyColumn(tables[0])
	fk2 := ForeignKeyColumn(tables[1])
	s1 := Singularize(tables[0])
	s2 := Singularize(tables[1])

	// Build column list
	var columnParts []string
	columnParts = append(columnParts, "    id BIGSERIAL PRIMARY KEY")
	columnParts = append(columnParts, fmt.Sprintf("    %s BIGINT NOT NULL", fk1))
	columnParts = append(columnParts, fmt.Sprintf("    %s BIGINT NOT NULL", fk2))
	columnParts = append(columnParts, "    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP")

	if opts.SoftDelete {
		columnParts = append(columnParts, "    deleted_at TIMESTAMPTZ DEFAULT NULL")
	}

	// Add constraints
	columnParts = append(columnParts, fmt.Sprintf(
		"    CONSTRAINT fk_%s_%s FOREIGN KEY (%s) REFERENCES %s(id) ON DELETE CASCADE",
		pivotName, s1, fk1, tables[0],
	))
	columnParts = append(columnParts, fmt.Sprintf(
		"    CONSTRAINT fk_%s_%s FOREIGN KEY (%s) REFERENCES %s(id) ON DELETE CASCADE",
		pivotName, s2, fk2, tables[1],
	))
	columnParts = append(columnParts, fmt.Sprintf(
		"    CONSTRAINT uq_%s_%s_%s UNIQUE (%s, %s)",
		pivotName, s1, s2, fk1, fk2,
	))

	columns := strings.Join(columnParts, ",\n")

	// Build indexes
	indexes := fmt.Sprintf(`
CREATE INDEX IF NOT EXISTS idx_%s_%s ON %s(%s);
CREATE INDEX IF NOT EXISTS idx_%s_%s ON %s(%s);`,
		pivotName, fk1, pivotName, fk1,
		pivotName, fk2, pivotName, fk2,
	)

	// Soft delete index
	if opts.SoftDelete {
		indexes += fmt.Sprintf("\nCREATE INDEX IF NOT EXISTS idx_%s_deleted_at ON %s(deleted_at);",
			pivotName, pivotName)
	}

	// Trigger section
	var triggerUp, triggerDown string
	if opts.Triggers {
		funcName := "auto_update_timestamp"
		if opts.SoftDelete {
			funcName = "auto_update_with_soft_delete_protection"
		}
		triggerUp = fmt.Sprintf(`

-- Auto-update trigger (requires: vm functions migrate)
DROP TRIGGER IF EXISTS trigger_%s_auto_update ON %s;
CREATE TRIGGER trigger_%s_auto_update
    BEFORE UPDATE ON %s
    FOR EACH ROW
    EXECUTE FUNCTION %s();`,
			pivotName, pivotName, pivotName, pivotName, funcName)
		triggerDown = fmt.Sprintf("\nDROP TRIGGER IF EXISTS trigger_%s_auto_update ON %s;\n",
			pivotName, pivotName)
	}

	// Build goose markers if sqlc support enabled
	gooseUp := ""
	gooseDown := ""
	if opts.SqlcSupport {
		gooseUp = "-- +goose Up\n"
		gooseDown = "-- +goose Down\n"
	}

	return fmt.Sprintf(`-- Migration: %s
-- Created at: %s
-- Relationship: %s <-> %s (Many-to-Many)

%s-- ⬆ Up (Run when migrating forward)
CREATE TABLE IF NOT EXISTS %s (
%s
);
%s%s

%s-- ⬇ Down (Run when rolling back)
%s
DROP TABLE IF EXISTS %s CASCADE;
`, upperName, now, tables[0], tables[1],
		gooseUp, pivotName, columns, indexes, triggerUp,
		gooseDown, triggerDown, pivotName)
}

// RelationshipComment generates a comment line describing the relationships
func RelationshipComment(tableName string, relationships []Relationship) string {
	if len(relationships) == 0 {
		return ""
	}

	var descs []string
	for _, rel := range relationships {
		switch rel.Type {
		case BelongsTo:
			descs = append(descs, fmt.Sprintf("%s → %s (Many-to-One)", tableName, rel.TargetTable))
		case OneToOne:
			descs = append(descs, fmt.Sprintf("%s → %s (One-to-One)", tableName, rel.TargetTable))
		}
	}

	if len(descs) == 1 {
		return fmt.Sprintf("-- Relationship: %s\n", descs[0])
	}
	return fmt.Sprintf("-- Relationships: %s\n", strings.Join(descs, ", "))
}

// RelationshipFeatureDescription returns a human-readable feature description for console output
func RelationshipFeatureDescription(relationships []Relationship, pivotTable1, pivotTable2 string) string {
	if pivotTable1 != "" && pivotTable2 != "" {
		return fmt.Sprintf("many-to-many: %s <-> %s", pivotTable1, pivotTable2)
	}

	var parts []string
	for _, rel := range relationships {
		switch rel.Type {
		case BelongsTo:
			parts = append(parts, fmt.Sprintf("belongs-to %s", rel.TargetTable))
		case OneToOne:
			parts = append(parts, fmt.Sprintf("one-to-one %s", rel.TargetTable))
		}
	}
	return strings.Join(parts, ", ")
}
