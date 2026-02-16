package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsValidMigrationName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid snake_case", "create_users_table", true},
		{"valid with numbers", "add_column_123", true},
		{"valid single word", "users", true},
		{"invalid with spaces", "create users table", false},
		{"invalid with uppercase", "CreateUsersTable", false},
		{"invalid with dash", "create-users-table", false},
		{"invalid with special chars", "create_users@table", false},
		{"invalid empty", "", false},
		{"valid long name", "create_very_long_table_name_with_many_underscores", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidMigrationName(tt.input)
			if got != tt.want {
				t.Errorf("isValidMigrationName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractTableName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"with create prefix", "create_users_table", "users"},
		{"with table suffix", "posts_table", "posts"},
		{"both prefix and suffix", "create_posts_table", "posts"},
		{"no prefix or suffix", "users", "users"},
		{"add prefix", "add_email_to_users", "add_email_to_users"},
		{"complex name", "create_user_profiles_table", "user_profiles"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTableName(tt.input)
			if got != tt.want {
				t.Errorf("extractTableName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestGenerateMigrationTemplate(t *testing.T) {
	tests := []struct {
		name        string
		migName     string
		opts        CreateMigrationOptions
		wantStrings []string
	}{
		{
			name:    "basic migration",
			migName: "create_users_table",
			opts:    CreateMigrationOptions{},
			wantStrings: []string{
				"CREATE TABLE IF NOT EXISTS users",
				"id BIGSERIAL PRIMARY KEY",
				"created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP",
				"updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP",
				"DROP TABLE IF EXISTS users CASCADE",
			},
		},
		{
			name:    "with soft delete",
			migName: "create_posts_table",
			opts:    CreateMigrationOptions{SoftDelete: true},
			wantStrings: []string{
				"deleted_at TIMESTAMP DEFAULT NULL",
				"CREATE INDEX IF NOT EXISTS idx_posts_deleted_at",
			},
		},
		{
			name:    "with triggers",
			migName: "create_comments_table",
			opts:    CreateMigrationOptions{Triggers: true},
			wantStrings: []string{
				"CREATE TRIGGER trigger_comments_auto_update",
				"EXECUTE FUNCTION auto_update_timestamp()",
				"DROP TRIGGER IF EXISTS trigger_comments_auto_update",
			},
		},
		{
			name:    "with sqlc support",
			migName: "create_articles_table",
			opts:    CreateMigrationOptions{SqlcSupport: true},
			wantStrings: []string{
				"-- +goose Up",
				"-- +goose Down",
			},
		},
		{
			name:    "without sqlc support",
			migName: "create_reviews_table",
			opts:    CreateMigrationOptions{SqlcSupport: false},
			wantStrings: []string{
				"-- ⬆ Up (Run when migrating forward)",
				"-- ⬇ Down (Run when rolling back)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateMigrationTemplate(tt.migName, tt.opts)
			for _, want := range tt.wantStrings {
				if !strings.Contains(got, want) {
					t.Errorf("generateMigrationTemplate() missing string %q", want)
				}
			}
		})
	}
}

func TestCreateMigrationWithOptions(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		migName string
		opts    CreateMigrationOptions
		wantErr bool
	}{
		{
			name:    "valid basic migration",
			migName: "create_users_table",
			opts:    CreateMigrationOptions{},
			wantErr: false,
		},
		{
			name:    "invalid name with spaces",
			migName: "create users table",
			opts:    CreateMigrationOptions{},
			wantErr: true,
		},
		{
			name:    "invalid name with uppercase",
			migName: "CreateUsers",
			opts:    CreateMigrationOptions{},
			wantErr: true,
		},
		{
			name:    "valid with soft delete",
			migName: "create_posts_table",
			opts:    CreateMigrationOptions{SoftDelete: true},
			wantErr: false,
		},
		{
			name:    "valid with triggers",
			migName: "create_comments_table",
			opts:    CreateMigrationOptions{Triggers: true},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use unique subdirectory for each test
			testDir := filepath.Join(tmpDir, tt.name)
			err := CreateMigrationWithOptions(tt.migName, testDir, tt.opts)
			
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateMigrationWithOptions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify migration file was created
				files, err := os.ReadDir(testDir)
				if err != nil {
					t.Fatal(err)
				}
				if len(files) == 0 {
					t.Error("No migration file created")
				}

				// Check if triggers option created functions.sql
				if tt.opts.Triggers {
					functionsPath := filepath.Join(testDir, "functions.sql")
					if _, err := os.Stat(functionsPath); os.IsNotExist(err) {
						t.Error("functions.sql should be created with triggers option")
					}
				}
			}
		})
	}
}

func TestEnsureExtensionsFile(t *testing.T) {
	tmpDir := t.TempDir()

	err := EnsureExtensionsFile(tmpDir)
	if err != nil {
		t.Fatalf("EnsureExtensionsFile() error = %v", err)
	}

	// Check file was created
	extensionsPath := filepath.Join(tmpDir, "extensions.sql")
	if _, err := os.Stat(extensionsPath); os.IsNotExist(err) {
		t.Error("extensions.sql was not created")
	}

	// Check content
	content, err := os.ReadFile(extensionsPath)
	if err != nil {
		t.Fatal(err)
	}

	expectedStrings := []string{
		"uuid-ossp",
		"pg_trgm",
		"citext",
		"CREATE EXTENSION IF NOT EXISTS",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(string(content), expected) {
			t.Errorf("extensions.sql missing expected content: %q", expected)
		}
	}

	// Test idempotency - calling again should not overwrite
	originalContent := string(content)
	err = EnsureExtensionsFile(tmpDir)
	if err != nil {
		t.Fatalf("Second EnsureExtensionsFile() error = %v", err)
	}

	newContent, err := os.ReadFile(extensionsPath)
	if err != nil {
		t.Fatal(err)
	}

	if string(newContent) != originalContent {
		t.Error("EnsureExtensionsFile() should not overwrite existing file")
	}
}

func TestEnsureFunctionsFile(t *testing.T) {
	tmpDir := t.TempDir()

	err := EnsureFunctionsFile(tmpDir)
	if err != nil {
		t.Fatalf("EnsureFunctionsFile() error = %v", err)
	}

	// Check file was created
	functionsPath := filepath.Join(tmpDir, "functions.sql")
	if _, err := os.Stat(functionsPath); os.IsNotExist(err) {
		t.Error("functions.sql was not created")
	}

	// Check content
	content, err := os.ReadFile(functionsPath)
	if err != nil {
		t.Fatal(err)
	}

	expectedFunctions := []string{
		"auto_update_timestamp",
		"protect_soft_deleted",
		"auto_update_with_soft_delete_protection",
		"prevent_hard_delete",
		"CUSTOM FUNCTIONS",
	}

	for _, expected := range expectedFunctions {
		if !strings.Contains(string(content), expected) {
			t.Errorf("functions.sql missing expected function: %q", expected)
		}
	}
}
