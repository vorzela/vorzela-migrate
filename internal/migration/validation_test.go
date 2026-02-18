package migration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateMigrationContent_ValidMigration(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a valid migration file
	validMigration := `-- Up
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(100) NOT NULL,
    email VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_users_email ON users(email);

-- Down
DROP TABLE IF EXISTS users;
`

	filePath := filepath.Join(tmpDir, "1234567890_create_users.sql")
	if err := os.WriteFile(filePath, []byte(validMigration), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	err := ValidateMigrationContent(filePath)
	if err != nil {
		t.Errorf("Valid migration should pass validation, got error: %v", err)
	}
}

func TestValidateMigrationContent_ContainsFunction(t *testing.T) {
	tmpDir := t.TempDir()

	// Create migration with function definition (invalid)
	invalidMigration := `-- Up
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    updated_at TIMESTAMP
);

CREATE OR REPLACE FUNCTION auto_update_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Down
DROP TABLE IF EXISTS users;
DROP FUNCTION IF EXISTS auto_update_timestamp();
`

	filePath := filepath.Join(tmpDir, "1234567890_create_users_with_function.sql")
	if err := os.WriteFile(filePath, []byte(invalidMigration), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	err := ValidateMigrationContent(filePath)
	if err == nil {
		t.Error("Migration with function should fail validation")
	}

	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Errorf("Expected ValidationError, got %T", err)
	}

	if validationErr != nil && !contains(validationErr.Message, "function definitions") {
		t.Errorf("Error message should mention function definitions, got: %s", validationErr.Message)
	}
}

func TestValidateMigrationContent_ContainsExtension(t *testing.T) {
	tmpDir := t.TempDir()

	// Create migration with extension creation (invalid)
	invalidMigration := `-- Up
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username VARCHAR(100) NOT NULL
);

-- Down
DROP TABLE IF EXISTS users;
DROP EXTENSION IF EXISTS "uuid-ossp";
`

	filePath := filepath.Join(tmpDir, "1234567890_create_users_with_extension.sql")
	if err := os.WriteFile(filePath, []byte(invalidMigration), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	err := ValidateMigrationContent(filePath)
	if err == nil {
		t.Error("Migration with extension should fail validation")
	}

	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Errorf("Expected ValidationError, got %T", err)
	}

	if validationErr != nil && !contains(validationErr.Message, "extension creation") {
		t.Errorf("Error message should mention extension creation, got: %s", validationErr.Message)
	}
}

func TestValidateFunctionsFile_Valid(t *testing.T) {
	tmpDir := t.TempDir()

	validFunctionsFile := `-- Common Database Functions

CREATE OR REPLACE FUNCTION auto_update_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION protect_soft_deleted()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.deleted_at IS NOT NULL THEN
        RAISE EXCEPTION 'Cannot modify soft-deleted record';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
`

	filePath := filepath.Join(tmpDir, "functions.sql")
	if err := os.WriteFile(filePath, []byte(validFunctionsFile), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	err := ValidateFunctionsFile(filePath)
	if err != nil {
		t.Errorf("Valid functions file should pass validation, got error: %v", err)
	}
}

func TestValidateFunctionsFile_ContainsExtension(t *testing.T) {
	tmpDir := t.TempDir()

	invalidFunctionsFile := `-- Functions with extension (invalid)

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE OR REPLACE FUNCTION auto_update_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
`

	filePath := filepath.Join(tmpDir, "functions.sql")
	if err := os.WriteFile(filePath, []byte(invalidFunctionsFile), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	err := ValidateFunctionsFile(filePath)
	if err == nil {
		t.Error("functions.sql with extension should fail validation")
	}

	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Errorf("Expected ValidationError, got %T", err)
	}

	if validationErr != nil && !contains(validationErr.Message, "extension creation") {
		t.Errorf("Error message should mention extension creation, got: %s", validationErr.Message)
	}
}

func TestValidateExtensionsFile_Valid(t *testing.T) {
	tmpDir := t.TempDir()

	validExtensionsFile := `-- PostgreSQL Extensions

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";
`

	filePath := filepath.Join(tmpDir, "extensions.sql")
	if err := os.WriteFile(filePath, []byte(validExtensionsFile), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	err := ValidateExtensionsFile(filePath)
	if err != nil {
		t.Errorf("Valid extensions file should pass validation, got error: %v", err)
	}
}

func TestValidateExtensionsFile_ContainsFunction(t *testing.T) {
	tmpDir := t.TempDir()

	invalidExtensionsFile := `-- Extensions with function (invalid)

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE OR REPLACE FUNCTION auto_update_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
`

	filePath := filepath.Join(tmpDir, "extensions.sql")
	if err := os.WriteFile(filePath, []byte(invalidExtensionsFile), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	err := ValidateExtensionsFile(filePath)
	if err == nil {
		t.Error("extensions.sql with function should fail validation")
	}

	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Errorf("Expected ValidationError, got %T", err)
	}

	if validationErr != nil && !contains(validationErr.Message, "function definitions") {
		t.Errorf("Error message should mention function definitions, got: %s", validationErr.Message)
	}
}

func TestContainsFunctionDefinition(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "Simple CREATE FUNCTION",
			content:  "CREATE FUNCTION my_func() RETURNS void AS $$ BEGIN END; $$ LANGUAGE plpgsql;",
			expected: true,
		},
		{
			name:     "CREATE OR REPLACE FUNCTION",
			content:  "CREATE OR REPLACE FUNCTION my_func() RETURNS void AS $$ BEGIN END; $$ LANGUAGE plpgsql;",
			expected: true,
		},
		{
			name:     "Commented function (should be ignored)",
			content:  "-- CREATE FUNCTION my_func() RETURNS void AS $$ BEGIN END; $$ LANGUAGE plpgsql;",
			expected: false,
		},
		{
			name:     "Table creation (no function)",
			content:  "CREATE TABLE users (id SERIAL PRIMARY KEY);",
			expected: false,
		},
		{
			name:     "Case insensitive",
			content:  "create function my_func() returns void as $$ begin end; $$ language plpgsql;",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsFunctionDefinition(tt.content)
			if result != tt.expected {
				t.Errorf("containsFunctionDefinition() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestContainsExtensionCreation(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "Simple CREATE EXTENSION",
			content:  `CREATE EXTENSION "uuid-ossp";`,
			expected: true,
		},
		{
			name:     "CREATE EXTENSION IF NOT EXISTS",
			content:  `CREATE EXTENSION IF NOT EXISTS "pgcrypto";`,
			expected: true,
		},
		{
			name:     "Commented extension (should be ignored)",
			content:  `-- CREATE EXTENSION "uuid-ossp";`,
			expected: false,
		},
		{
			name:     "Table creation (no extension)",
			content:  "CREATE TABLE users (id UUID PRIMARY KEY);",
			expected: false,
		},
		{
			name:     "Case insensitive",
			content:  `create extension "uuid-ossp";`,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsExtensionCreation(tt.content)
			if result != tt.expected {
				t.Errorf("containsExtensionCreation() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestValidateAllMigrations(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid migration
	validMigration := `-- Up
CREATE TABLE users (id SERIAL PRIMARY KEY);
-- Down
DROP TABLE users;
`
	if err := os.WriteFile(filepath.Join(tmpDir, "1_create_users.sql"), []byte(validMigration), 0644); err != nil {
		t.Fatalf("Failed to write valid migration: %v", err)
	}

	// Create invalid migration with function
	invalidMigration := `-- Up
CREATE FUNCTION bad() RETURNS void AS $$ BEGIN END; $$ LANGUAGE plpgsql;
-- Down
DROP FUNCTION bad();
`
	if err := os.WriteFile(filepath.Join(tmpDir, "2_bad_migration.sql"), []byte(invalidMigration), 0644); err != nil {
		t.Fatalf("Failed to write invalid migration: %v", err)
	}

	errors := ValidateAllMigrations(tmpDir)
	if len(errors) == 0 {
		t.Error("Expected validation errors, got none")
	}

	if len(errors) != 1 {
		t.Errorf("Expected 1 validation error, got %d", len(errors))
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}
