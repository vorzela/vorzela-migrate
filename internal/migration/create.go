package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CreateMigrationOptions defines options for migration creation
type CreateMigrationOptions struct {
	SoftDelete    bool
	Triggers      bool // Add trigger / ON UPDATE for updated_at automation
	Dialect       Dialect
	Relationships []Relationship // FK relationships (belongs-to, one-to-one)
	IsPivot       bool           // True when creating a many-to-many pivot table
	PivotTables   [2]string      // The two table names for pivot generation
	SqlcSupport   bool           // Include goose markers for sqlc compatibility
}

// CreateMigration creates a new migration file (backward compatible)
func CreateMigration(name, path string) error {
	return CreateMigrationWithOptions(name, path, CreateMigrationOptions{})
}

// CreateMigrationWithOptions creates a new migration file with custom options
func CreateMigrationWithOptions(name, path string, opts CreateMigrationOptions) error {
	// Validate migration name
	if !isValidMigrationName(name) {
		return fmt.Errorf("invalid migration name. Use snake_case without spaces")
	}

	// Create migrations directory if it doesn't exist
	if err := os.MkdirAll(path, 0755); err != nil {
		return fmt.Errorf("failed to create migrations directory: %w", err)
	}

	// Ensure functions.sql exists if triggers are enabled (PostgreSQL PL/pgSQL helpers only)
	if (opts.Triggers || strings.HasPrefix(name, "trigger_")) && !IsMySQLFamily(opts.Dialect) {
		if err := EnsureFunctionsFile(path); err != nil {
			return err
		}
	}

	// Generate timestamp-based filename. Two `vm make` calls in the same second
	// used to share a timestamp prefix, which makes ordering ambiguous; advance
	// until the prefix is unused.
	timestamp := time.Now().Unix()
	var filename string
	for {
		prefix := fmt.Sprintf("%d_", timestamp)
		used := false
		if entries, readErr := os.ReadDir(path); readErr == nil {
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), prefix) {
					used = true
					break
				}
			}
		}
		if !used {
			filename = prefix + name + ".sql"
			break
		}
		timestamp++
	}
	filepath := filepath.Join(path, filename)

	// Create migration file with template
	var template string
	switch {
	case opts.IsPivot:
		template = GeneratePivotMigration(opts.PivotTables[0], opts.PivotTables[1], opts)
	case isAlterationMigration(name):
		template = generateAlterationTemplate(name, opts)
	case strings.HasPrefix(name, "trigger_"):
		template = generateTriggerOnlyTemplate(name, opts)
	default:
		template = generateMigrationTemplate(name, opts)
	}
	if err := os.WriteFile(filepath, []byte(template), 0644); err != nil {
		return fmt.Errorf("failed to create migration file: %w", err)
	}

	fmt.Printf("Migration file created: %s\n", filename)
	return nil
}

// isAlterationMigration reports whether a migration name describes an ALTER
// operation rather than a new table.
func isAlterationMigration(name string) bool {
	return strings.HasPrefix(name, "add_") || strings.HasPrefix(name, "alter_")
}

// inferAddTarget parses add_<column>_to_<table> and returns (table, column).
// It returns empty strings when the name does not follow that convention.
func inferAddTarget(name string) (table, column string) {
	rest := strings.TrimPrefix(name, "add_")
	idx := strings.Index(rest, "_to_")
	if idx <= 0 || idx+4 >= len(rest) {
		return "", ""
	}
	column = rest[:idx]
	table = rest[idx+4:]
	if !isValidMigrationName(column) || !isValidMigrationName(table) {
		return "", ""
	}
	return table, column
}

// generateAlterationTemplate creates an ALTER TABLE scaffold. It infers the
// table/column from add_<col>_to_<table> when possible and otherwise leaves
// explicit placeholders for the user (or agent) to fill in.
func generateAlterationTemplate(name string, opts CreateMigrationOptions) string {
	now := time.Now().Format("2006-01-02 15:04:05")

	table, column := inferAddTarget(name)
	inferred := table != "" && column != ""
	if !inferred {
		table, column = "your_table", "your_column"
	}

	gooseUp, gooseDown := gooseMarkers(opts)
	hint := "-- TODO: replace TYPE with the real column type and add constraints/backfill as needed."
	if inferred {
		hint = fmt.Sprintf("-- TODO: replace TYPE with the real type for %s.%s; add NOT NULL/DEFAULT/backfill if needed.", table, column)
	}

	return fmt.Sprintf(`-- Migration: %s
-- Created at: %s
-- Alteration migration (no CREATE TABLE).
%s

%s-- ⬆ Up (Run when migrating forward)
ALTER TABLE %s ADD COLUMN %s TYPE;

%s-- ⬇ Down (Run when rolling back)
ALTER TABLE %s DROP COLUMN IF EXISTS %s;
`, strings.ToUpper(name), now, hint, gooseUp, table, column, gooseDown, table, column)
}

// generateTriggerOnlyTemplate creates a trigger scaffold without creating a
// table. The table name is left as a placeholder because trigger_<name>
// conventions vary.
func generateTriggerOnlyTemplate(name string, opts CreateMigrationOptions) string {
	dialect := ResolveDialect(opts.Dialect)
	now := time.Now().Format("2006-01-02 15:04:05")
	gooseUp, gooseDown := gooseMarkers(opts)

	if IsMySQLFamily(dialect) {
		return fmt.Sprintf(`-- Migration: %s
-- Created at: %s
-- Trigger-only migration (edit table_name).
%s
%s-- ⬆ Up (Run when migrating forward)
DROP TRIGGER IF EXISTS %s;
CREATE TRIGGER %s
    BEFORE UPDATE ON table_name
    FOR EACH ROW
    SET NEW.updated_at = CURRENT_TIMESTAMP;

%s-- ⬇ Down (Run when rolling back)
DROP TRIGGER IF EXISTS %s;
`, strings.ToUpper(name), now, "", gooseUp, name, name, gooseDown, name)
	}

	return fmt.Sprintf(`-- Migration: %s
-- Created at: %s
-- Trigger-only migration (edit table_name; requires functions.sql).
%s
%s-- ⬆ Up (Run when migrating forward)
DROP TRIGGER IF EXISTS %s ON table_name;
CREATE TRIGGER %s
    BEFORE UPDATE ON table_name
    FOR EACH ROW
    EXECUTE FUNCTION auto_update_timestamp();

%s-- ⬇ Down (Run when rolling back)
DROP TRIGGER IF EXISTS %s ON table_name;
`, strings.ToUpper(name), now, "", gooseUp, name, name, gooseDown, name)
}

func gooseMarkers(opts CreateMigrationOptions) (up, down string) {
	if opts.SqlcSupport {
		return "-- +goose Up\n", "-- +goose Down\n"
	}
	return "", ""
}

// isValidMigrationName validates the migration name format
func isValidMigrationName(name string) bool {
	if name == "" {
		return false
	}

	// Allow lowercase letters, numbers, and underscores
	for _, char := range name {
		if !((char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_') {
			return false
		}
	}

	return true
}

// extractTableName extracts the actual table name from migration name
// Examples:
//
// create_users_table -> users
// add_email_to_users -> add_email_to_users (no extraction)
// create_posts_table -> posts
func extractTableName(migrationName string) string {
	name := migrationName

	// Remove "create_" prefix if present
	if after, ok := strings.CutPrefix(name, "create_"); ok {
		name = after
	}

	// Remove "_table" suffix if present
	if before, ok := strings.CutSuffix(name, "_table"); ok {
		name = before
	}

	return name
}

// generateTriggerSQL returns Up/Down trigger snippets for the dialect.
// MySQL/MariaDB use a single-statement FOR EACH ROW SET … trigger (safe for statement splitters).
// PostgreSQL uses functions from functions.sql.
func generateTriggerSQL(tableName string, softDelete bool, dialect Dialect) (up string, down string) {
	if IsMySQLFamily(dialect) {
		note := ""
		if softDelete {
			note = "\n-- Note: soft-delete update protection (protect_soft_deleted) is PostgreSQL-only;\n-- only auto-updating updated_at is scaffolded for MySQL/MariaDB.\n"
		}
		up = fmt.Sprintf(`%s
DROP TRIGGER IF EXISTS trigger_%s_auto_update;
CREATE TRIGGER trigger_%s_auto_update
    BEFORE UPDATE ON %s
    FOR EACH ROW
    SET NEW.updated_at = CURRENT_TIMESTAMP;
`, note, tableName, tableName, tableName)
		down = fmt.Sprintf(`
DROP TRIGGER IF EXISTS trigger_%s_auto_update;
`, tableName)
		return up, down
	}

	functionName := "auto_update_timestamp"
	if softDelete {
		functionName = "auto_update_with_soft_delete_protection"
	}

	up = fmt.Sprintf(`

-- Create trigger using centralized function from functions.sql
-- IMPORTANT: Run 'vm functions migrate' first to install the required functions
DROP TRIGGER IF EXISTS trigger_%s_auto_update ON %s;
CREATE TRIGGER trigger_%s_auto_update
    BEFORE UPDATE ON %s
    FOR EACH ROW
    EXECUTE FUNCTION %s();
`, tableName, tableName, tableName, tableName, functionName)

	down = fmt.Sprintf(`
DROP TRIGGER IF EXISTS trigger_%s_auto_update ON %s;
`, tableName, tableName)
	return up, down
}

// generateMigrationTemplate generates a migration template
func generateMigrationTemplate(name string, opts CreateMigrationOptions) string {
	dialect := ResolveDialect(opts.Dialect)
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	upperName := strings.ToUpper(name)
	tableName := extractTableName(name)

	// Build column definitions
	var columnParts []string
	columnParts = append(columnParts, "    "+PrimaryKeyColumnSQL(dialect))

	// Add FK columns from relationships
	for _, rel := range opts.Relationships {
		fkCol := ForeignKeyColumn(rel.TargetTable)
		switch rel.Type {
		case OneToOne:
			columnParts = append(columnParts, fmt.Sprintf("    %s BIGINT NOT NULL UNIQUE", fkCol))
		case BelongsTo:
			columnParts = append(columnParts, fmt.Sprintf("    %s BIGINT NOT NULL", fkCol))
		}
	}

	columnParts = append(columnParts, "    "+TimestampColumnSQL(dialect, "created_at"))
	columnParts = append(columnParts, "    "+TimestampColumnSQL(dialect, "updated_at"))

	if opts.SoftDelete {
		columnParts = append(columnParts, "    "+SoftDeleteColumnSQL(dialect))
	}

	// Add FK constraints
	for _, rel := range opts.Relationships {
		fkCol := ForeignKeyColumn(rel.TargetTable)
		singular := Singularize(rel.TargetTable)
		columnParts = append(columnParts, fmt.Sprintf(
			"    CONSTRAINT fk_%s_%s FOREIGN KEY (%s) REFERENCES %s(id) ON DELETE CASCADE",
			tableName, singular, fkCol, rel.TargetTable,
		))
	}

	columns := strings.Join(columnParts, ",\n")

	// Build extra indexes (FK indexes for belongs-to, soft delete index)
	var extraIndexes string
	for _, rel := range opts.Relationships {
		if rel.Type == BelongsTo {
			fkCol := ForeignKeyColumn(rel.TargetTable)
			extraIndexes += "\n" + CreateIndexSQL(dialect, false, fmt.Sprintf("idx_%s_%s", tableName, fkCol), tableName, fkCol)
		}
	}

	if opts.SoftDelete {
		extraIndexes += "\n" + CreateIndexSQL(dialect, false, fmt.Sprintf("idx_%s_deleted_at", tableName), tableName, "deleted_at")
	}

	if extraIndexes != "" {
		extraIndexes = "\n" + extraIndexes + "\n"
	}

	var triggerFunctions string
	var triggerCleanup string
	if opts.Triggers {
		triggerFunctions, triggerCleanup = generateTriggerSQL(tableName, opts.SoftDelete, dialect)
	}

	// Build relationship comment
	relComment := RelationshipComment(tableName, opts.Relationships)

	// Build goose markers if sqlc support enabled
	gooseUp := ""
	gooseDown := ""
	if opts.SqlcSupport {
		gooseUp = "-- +goose Up\n"
		gooseDown = "-- +goose Down\n"
	}

	return fmt.Sprintf(`-- Migration: %s
-- Created at: %s
%s
%s-- ⬆ Up (Run when migrating forward)
CREATE TABLE IF NOT EXISTS %s (
%s
);%s%s

%s-- ⬇ Down (Run when rolling back)
%s
%s
`, upperName, timestamp, relComment, gooseUp, tableName, columns, extraIndexes, triggerFunctions, gooseDown, triggerCleanup, DropTableSQL(dialect, tableName))
}
