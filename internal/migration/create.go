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
	Triggers      bool // Add trigger functions for updated_at automation
	Dialect       string
	Relationships []Relationship // FK relationships (belongs-to, one-to-one)
	IsPivot       bool           // True when creating a many-to-many pivot table
	PivotTables   [2]string      // The two table names for pivot generation
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

	// Ensure functions.sql exists if triggers are enabled
	if opts.Triggers {
		if err := EnsureFunctionsFile(path); err != nil {
			return err
		}
	}

	// Generate timestamp-based filename
	timestamp := time.Now().Unix()
	filename := fmt.Sprintf("%d_%s.sql", timestamp, name)
	filepath := filepath.Join(path, filename)

	// Check if file already exists
	if _, err := os.Stat(filepath); err == nil {
		return fmt.Errorf("migration file already exists: %s", filename)
	}

	// Create migration file with template
	var template string
	if opts.IsPivot {
		template = GeneratePivotMigration(opts.PivotTables[0], opts.PivotTables[1], opts)
	} else {
		template = generateMigrationTemplate(name, opts)
	}
	if err := os.WriteFile(filepath, []byte(template), 0644); err != nil {
		return fmt.Errorf("failed to create migration file: %w", err)
	}

	fmt.Printf("Migration file created: %s\n", filename)
	return nil
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

// generateMigrationTemplate generates a migration template
func generateMigrationTemplate(name string, opts CreateMigrationOptions) string {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	upperName := strings.ToUpper(name)
	tableName := extractTableName(name)

	// Build column definitions
	var columnParts []string
	columnParts = append(columnParts, "    id BIGSERIAL PRIMARY KEY")

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

	columnParts = append(columnParts, "    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP")
	columnParts = append(columnParts, "    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP")

	if opts.SoftDelete {
		columnParts = append(columnParts, "    deleted_at TIMESTAMP DEFAULT NULL")
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
			extraIndexes += fmt.Sprintf("\nCREATE INDEX IF NOT EXISTS idx_%s_%s ON %s(%s);", tableName, fkCol, tableName, fkCol)
		}
	}

	if opts.SoftDelete {
		extraIndexes += fmt.Sprintf("\nCREATE INDEX IF NOT EXISTS idx_%s_deleted_at ON %s(deleted_at);", tableName, tableName)
	}

	if extraIndexes != "" {
		extraIndexes = "\n" + extraIndexes + "\n"
	}

	// Add trigger using centralized functions from functions.sql
	var triggerFunctions string
	var triggerCleanup string
	if opts.Triggers {
		functionName := "auto_update_timestamp"
		if opts.SoftDelete {
			functionName = "auto_update_with_soft_delete_protection"
		}

		triggerFunctions = fmt.Sprintf(`

-- Create trigger using centralized function from functions.sql
-- IMPORTANT: Run 'vm functions migrate' first to install the required functions
DROP TRIGGER IF EXISTS trigger_%s_auto_update ON %s;
CREATE TRIGGER trigger_%s_auto_update
    BEFORE UPDATE ON %s
    FOR EACH ROW
    EXECUTE FUNCTION %s();
`, tableName, tableName, tableName, tableName, functionName)

		triggerCleanup = fmt.Sprintf(`
DROP TRIGGER IF EXISTS trigger_%s_auto_update ON %s;
`, tableName, tableName)
	}

	// Build relationship comment
	relComment := RelationshipComment(tableName, opts.Relationships)

	return fmt.Sprintf(`-- Migration: %s
-- Created at: %s
%s
-- ⬆ Up (Run when migrating forward)
BEGIN;

CREATE TABLE IF NOT EXISTS %s (
%s
);%s%s
COMMIT;

-- ⬇ Down (Run when rolling back)
BEGIN;
%s
DROP TABLE IF EXISTS %s CASCADE;

COMMIT;
`, upperName, timestamp, relComment, tableName, columns, extraIndexes, triggerFunctions, triggerCleanup, tableName)
}
