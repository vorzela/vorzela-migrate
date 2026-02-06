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
	SoftDelete bool
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

	// Generate timestamp-based filename
	timestamp := time.Now().Unix()
	filename := fmt.Sprintf("%d_%s.sql", timestamp, name)
	filepath := filepath.Join(path, filename)

	// Check if file already exists
	if _, err := os.Stat(filepath); err == nil {
		return fmt.Errorf("migration file already exists: %s", filename)
	}

	// Create migration file with template
	template := generateMigrationTemplate(name, opts)
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
//	create_users_table -> users
//	add_email_to_users -> add_email_to_users (no extraction)
//	create_posts_table -> posts
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
	upperName := strings.ToUpper(name)
	tableName := extractTableName(name)
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	columns := `    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP`

	if opts.SoftDelete {
		columns += `,
    deleted_at TIMESTAMP DEFAULT NULL`
	}

	return fmt.Sprintf(`-- Migration: %s
-- Created at: %s

-- ⬆ Up (Run when migrating forward)
BEGIN;

CREATE TABLE IF NOT EXISTS %s (
%s
);

COMMIT;

-- ⬇ Down (Run when rolling back)
BEGIN;

DROP TABLE IF EXISTS %s CASCADE;

COMMIT;
`, upperName, timestamp, tableName, columns, tableName)
}
