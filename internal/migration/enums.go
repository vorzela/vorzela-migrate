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
	var enums []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		if m := createTypeRe.FindStringSubmatch(trimmed); len(m) > 1 {
			name := strings.Trim(m[1], `"`)
			enums = append(enums, name)
		}
	}
	return enums
}
