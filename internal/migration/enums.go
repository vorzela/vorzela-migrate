package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const enumsTemplate = `-- PostgreSQL Enum Types for Vorzela Migration Tool
-- Define your custom enum types here so they are created before migrations run.
--
-- Usage:
--   vm enums migrate    - Create all enum types in the database
--   vm enums drop       - Drop all enum types defined in this file
--   vm enums status     - List enums defined here vs what exists in the database
--
-- ⚠️  IMPORTANT: Define enums here, NOT inside individual migration files.
-- This ensures enum types exist before any table that references them is created.
--
-- Each CREATE TYPE statement must be on its own line (or uncommented block).

-- ============================================================================
-- COMMON ENUM TYPES (Uncomment and customise the ones you need)
-- ============================================================================

-- User / account status
-- CREATE TYPE user_status AS ENUM ('active', 'inactive', 'suspended', 'banned');

-- Generic publish/draft state
-- CREATE TYPE publish_status AS ENUM ('draft', 'published', 'archived');

-- Gender
-- CREATE TYPE gender AS ENUM ('male', 'female', 'other', 'prefer_not_to_say');

-- Payment status
-- CREATE TYPE payment_status AS ENUM ('pending', 'completed', 'failed', 'refunded', 'cancelled');

-- Order status
-- CREATE TYPE order_status AS ENUM ('pending', 'processing', 'shipped', 'delivered', 'cancelled', 'returned');

-- Notification type
-- CREATE TYPE notification_type AS ENUM ('email', 'sms', 'push', 'in_app');

-- Priority level
-- CREATE TYPE priority_level AS ENUM ('low', 'medium', 'high', 'critical');

-- Role type
-- CREATE TYPE role_type AS ENUM ('admin', 'staff', 'user', 'guest');

-- ============================================================================
-- PROJECT-SPECIFIC ENUM TYPES (Add your custom enums below)
-- ============================================================================

-- CREATE TYPE your_enum AS ENUM ('value1', 'value2', 'value3');
`

// EnsureEnumsFile creates the enums.sql file if it doesn't exist.
func EnsureEnumsFile(migrationPath string) error {
	enumsFile := filepath.Join(migrationPath, "enums.sql")

	if _, err := os.Stat(enumsFile); err == nil {
		return nil // already exists
	}

	if err := os.WriteFile(enumsFile, []byte(enumsTemplate), 0644); err != nil {
		return fmt.Errorf("failed to create enums.sql: %w", err)
	}

	fmt.Printf("✓ Created enums.sql with common PostgreSQL enum types\n")
	fmt.Printf("  💡 Uncomment and edit the types you need in migrations/enums.sql\n")
	fmt.Printf("  💡 Apply to database: vm enums migrate\n")
	return nil
}

// createTypeRe matches: CREATE TYPE <name> AS ENUM (...)  (non-commented lines)
var createTypeRe = regexp.MustCompile(`(?i)CREATE\s+TYPE\s+(["\w]+)\s+AS\s+ENUM`)

// ParseEnabledEnums extracts enum type names from enabled (non-commented) CREATE TYPE lines.
func ParseEnabledEnums(content string) []string {
	enabled, _ := ParseAllEnumNames(content)
	return enabled
}

// ParseAllEnumNames returns (enabled, disabled) enum type names.
// enabled  = names on active (non-commented) CREATE TYPE lines.
// disabled = names found on commented-out CREATE TYPE lines.
// Only enum names that appear in the file at all are managed; anything that
// exists in the DB but never appeared in the file is left untouched.
func ParseAllEnumNames(content string) (enabled, disabled []string) {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "--") {
			// Uncomment and check for a CREATE TYPE statement.
			uncommented := strings.TrimSpace(strings.TrimLeft(trimmed, "-"))
			if m := createTypeRe.FindStringSubmatch(uncommented); len(m) > 1 {
				disabled = append(disabled, strings.Trim(m[1], `"`))
			}
		} else {
			if m := createTypeRe.FindStringSubmatch(trimmed); len(m) > 1 {
				enabled = append(enabled, strings.Trim(m[1], `"`))
			}
		}
	}
	return
}

// ExtractEnumStatement finds the full CREATE TYPE ... AS ENUM (...); statement
// for a given type name within a SQL file's content.
func ExtractEnumStatement(content, name string) string {
	lines := strings.Split(content, "\n")
	var buf strings.Builder
	capturing := false
	depth := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}

		upper := strings.ToUpper(trimmed)
		if !capturing {
			if strings.Contains(upper, "CREATE") && strings.Contains(upper, "TYPE") &&
				strings.Contains(strings.ToLower(trimmed), strings.ToLower(name)) &&
				strings.Contains(upper, "ENUM") {
				capturing = true
				buf.Reset()
			}
		}

		if capturing {
			buf.WriteString(" ")
			buf.WriteString(trimmed)
			for _, ch := range trimmed {
				switch ch {
				case '(':
					depth++
				case ')':
					depth--
				}
			}
			if depth == 0 && buf.Len() > 0 {
				stmt := strings.TrimSpace(buf.String())
				if !strings.HasSuffix(stmt, ";") {
					stmt += ";"
				}
				return stmt
			}
		}
	}
	return ""
}

// enumValuesRe extracts individual quoted values from inside an ENUM (...) definition.
var enumValuesRe = regexp.MustCompile(`'([^']*)'`)

// ParseEnumValues extracts the ordered list of quoted enum labels from a
// CREATE TYPE ... AS ENUM (...) statement.
func ParseEnumValues(stmt string) []string {
	// Find the content between the first ( and the matching )
	open := strings.Index(stmt, "(")
	if open == -1 {
		return nil
	}
	close := strings.LastIndex(stmt, ")")
	if close <= open {
		return nil
	}
	body := stmt[open+1 : close]
	var vals []string
	for _, m := range enumValuesRe.FindAllStringSubmatch(body, -1) {
		vals = append(vals, m[1])
	}
	return vals
}

// GenerateEnumSyncSQL returns a DO block that:
//   - Creates the enum type if it does not exist yet.
//   - Adds any missing values (from wantValues) to an already-existing type using
//     ALTER TYPE … ADD VALUE IF NOT EXISTS, which is idempotent.
//
// Note: PostgreSQL does not support removing values from an existing enum type
// without dropping and recreating it (which requires CASCADE and loses data).
// If you need to remove a value, create a new migration that drops and recreates
// the type, or use a different approach (e.g. CHECK constraint on text column).
func GenerateEnumSyncSQL(name string, wantValues []string) string {
	if len(wantValues) == 0 {
		return ""
	}

	// Build the CREATE TYPE literal list  'v1', 'v2', ...
	escaped := make([]string, len(wantValues))
	for i, v := range wantValues {
		escaped[i] = "'" + strings.ReplaceAll(v, "'", "''") + "'"
	}
	createList := strings.Join(escaped, ", ")

	// Build ALTER TYPE ... ADD VALUE IF NOT EXISTS statements for each value.
	var alterStmts []string
	for _, e := range escaped {
		alterStmts = append(alterStmts,
			fmt.Sprintf("    ALTER TYPE %s ADD VALUE IF NOT EXISTS %s;", name, e))
	}
	alterBlock := strings.Join(alterStmts, "\n")

	safeName := strings.ReplaceAll(name, "'", "''")

	return fmt.Sprintf(`DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_type t
    JOIN pg_namespace n ON n.oid = t.typnamespace
    WHERE t.typname = '%s' AND n.nspname = 'public' AND t.typtype = 'e'
  ) THEN
    CREATE TYPE %s AS ENUM (%s);
  ELSE
%s
  END IF;
END $$;`, safeName, name, createList, alterBlock)
}

// GenerateDropEnumIfUnusedSQL returns a guarded PostgreSQL DO block that
// attempts to drop an enum type only when no table columns in public schema
// reference it anymore. It never uses CASCADE.
//
// If the enum still has non-column dependents (for example function signatures),
// PostgreSQL may raise dependent_objects_still_exist; that exception is trapped
// so sync flows can continue without failing the whole run.
func GenerateDropEnumIfUnusedSQL(name string) string {
	safeName := strings.ReplaceAll(name, "'", "''")

	return fmt.Sprintf(`DO $$
BEGIN
	IF EXISTS (
		SELECT 1
		FROM pg_type t
		JOIN pg_namespace n ON n.oid = t.typnamespace
		WHERE t.typname = '%s'
			AND n.nspname = 'public'
			AND t.typtype = 'e'
	)
	AND NOT EXISTS (
		SELECT 1
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_type t ON t.oid = a.atttypid
		WHERE n.nspname = 'public'
			AND c.relkind = 'r'
			AND a.attnum > 0
			AND NOT a.attisdropped
			AND t.typtype = 'e'
			AND t.typname = '%s'
	) THEN
		BEGIN
			EXECUTE format('DROP TYPE IF EXISTS %%I', '%s');
		EXCEPTION
			WHEN dependent_objects_still_exist THEN
				NULL;
		END;
	END IF;
END $$;`, safeName, safeName, safeName)
}
