package migration

import (
	"fmt"
	"os"
	"path/filepath"
)

const extensionsTemplate = `-- PostgreSQL Extensions for Vorzela Migration Tool
-- This file contains commonly used PostgreSQL extensions
-- Safe to add more extensions as needed for your project
--
-- Usage:
--   vm extensions migrate    - Install all extensions
--   vm extensions drop       - Remove all extensions
--
-- ⚠️  IMPORTANT: Add extensions here, NOT in your schema migrations!
-- This ensures extensions are installed before running migrations.

-- ============================================================================
-- COMMON EXTENSIONS (Uncomment the ones you need)
-- ============================================================================

-- UUID generation functions (recommended for ID generation)
-- CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Full-text search with trigram matching (for fuzzy search)
-- CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Case-insensitive text type
-- CREATE EXTENSION IF NOT EXISTS citext;

-- PostGIS for geographic data
-- CREATE EXTENSION IF NOT EXISTS postgis;

-- Additional cryptographic functions
-- CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Store JSON binary efficiently
-- CREATE EXTENSION IF NOT EXISTS hstore;

-- Unaccent - remove accents from strings
-- CREATE EXTENSION IF NOT EXISTS unaccent;

-- ============================================================================
-- CUSTOM EXTENSIONS (Add your project-specific extensions below)
-- ============================================================================

-- Add any additional extensions your project needs here
-- CREATE EXTENSION IF NOT EXISTS your_extension;

`

// EnsureExtensionsFile creates the extensions.sql file if it doesn't exist
func EnsureExtensionsFile(migrationPath string) error {
	extensionsFile := filepath.Join(migrationPath, "extensions.sql")

	// Check if file exists
	if _, err := os.Stat(extensionsFile); err == nil {
		// File exists, don't overwrite (users may have custom extensions)
		return nil
	}

	// Create extensions file
	if err := os.WriteFile(extensionsFile, []byte(extensionsTemplate), 0644); err != nil {
		return fmt.Errorf("failed to create extensions.sql: %w", err)
	}

	fmt.Printf("✓ Created extensions.sql with common PostgreSQL extensions\n")
	fmt.Printf("  💡 Uncomment the extensions you need in migrations/extensions.sql\n")
	fmt.Printf("  💡 Apply to database: vm extensions migrate\n")
	return nil
}
