package migration

import (
	"strings"
	"testing"
)

func TestDetectDialect(t *testing.T) {
	tests := []struct {
		name       string
		driverName string
		want       Dialect
	}{
		{"PostgreSQL URL", "postgres://localhost/db", PostgreSQL},
		{"PostgreSQL @host", "user@host/db", PostgreSQL},
		{"MySQL tcp", "mysql://localhost/db", MySQL},
		{"MySQL with @tcp", "user:pass@tcp(localhost)/db", MySQL},
		{"MariaDB URL", "mysql://localhost/mariadb", MariaDB},
		{"Default to PostgreSQL", "sqlite://test.db", PostgreSQL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectDialect(tt.driverName)
			if got != tt.want {
				t.Errorf("DetectDialect(%q) = %v, want %v", tt.driverName, got, tt.want)
			}
		})
	}
}

func TestCreateMigrationTableSQL(t *testing.T) {
	tests := []struct {
		name        string
		dialect     Dialect
		wantStrings []string
	}{
		{
			name:    "PostgreSQL",
			dialect: PostgreSQL,
			wantStrings: []string{
				"CREATE TABLE IF NOT EXISTS migrations",
				"id SERIAL PRIMARY KEY",
				"migration VARCHAR(255) NOT NULL UNIQUE",
				"batch INTEGER NOT NULL",
				"executed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP",
			},
		},
		{
			name:    "MySQL",
			dialect: MySQL,
			wantStrings: []string{
				"CREATE TABLE IF NOT EXISTS migrations",
				"id INT AUTO_INCREMENT PRIMARY KEY",
				"migration VARCHAR(255) NOT NULL UNIQUE",
				"batch INT NOT NULL",
				"executed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP",
			},
		},
		{
			name:    "MariaDB",
			dialect: MariaDB,
			wantStrings: []string{
				"CREATE TABLE IF NOT EXISTS migrations",
				"id INT AUTO_INCREMENT PRIMARY KEY",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CreateMigrationTableSQL(tt.dialect)
			for _, want := range tt.wantStrings {
				if !strings.Contains(got, want) {
					t.Errorf("CreateMigrationTableSQL(%v) missing string %q", tt.dialect, want)
				}
			}
		})
	}
}

func TestExtractSection(t *testing.T) {
	standardMigration := `-- Migration: TEST
-- Created at: 2026-01-01 00:00:00

-- ⬆ Up (Run when migrating forward)
CREATE TABLE users (id BIGSERIAL PRIMARY KEY);
CREATE INDEX idx_users_id ON users(id);

-- ⬇ Down (Run when rolling back)
DROP TABLE IF EXISTS users;
`

	gooseMigration := `-- Migration: TEST
-- +goose Up
CREATE TABLE roles (id BIGSERIAL PRIMARY KEY);

-- +goose Down
DROP TABLE IF EXISTS roles;
`

	tests := []struct {
		name    string
		content string
		section string
		want    string
	}{
		{
			name:    "Extract Up section (arrow format)",
			content: standardMigration,
			section: "Up",
			want:    "CREATE TABLE users (id BIGSERIAL PRIMARY KEY);\nCREATE INDEX idx_users_id ON users(id);",
		},
		{
			name:    "Extract Down section (arrow format)",
			content: standardMigration,
			section: "Down",
			want:    "DROP TABLE IF EXISTS users;",
		},
		{
			name:    "Extract Up section (goose format)",
			content: gooseMigration,
			section: "Up",
			want:    "CREATE TABLE roles (id BIGSERIAL PRIMARY KEY);",
		},
		{
			name:    "Extract Down section (goose format)",
			content: gooseMigration,
			section: "Down",
			want:    "DROP TABLE IF EXISTS roles;",
		},
		{
			name: "No section found",
			content: `-- Migration: TEST
CREATE TABLE users (id INT);`,
			section: "Up",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSection(tt.content, tt.section)
			// Normalize whitespace for comparison
			gotNorm := strings.TrimSpace(got)
			wantNorm := strings.TrimSpace(tt.want)
			if gotNorm != wantNorm {
				t.Errorf("extractSection() = %q, want %q", gotNorm, wantNorm)
			}
		})
	}
}

func TestExtractMigrationName(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     string
	}{
		{"standard format", "1707129045_create_users_table.sql", "create users table"},
		{"multiple underscores", "1707129045_add_email_to_users_table.sql", "add email to users table"},
		{"no timestamp", "create_users_table.sql", "users table"},
		{"short filename", "123_test.sql", "test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractMigrationName(tt.filename)
			if got != tt.want {
				t.Errorf("extractMigrationName(%q) = %q, want %q", tt.filename, got, tt.want)
			}
		})
	}
}
