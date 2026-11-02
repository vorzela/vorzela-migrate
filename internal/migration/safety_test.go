package migration

import (
	"os"
	"path/filepath"
	"testing"
)

func writeMigration(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckMigrationFiles(t *testing.T) {
	dir := t.TempDir()
	writeMigration(t, dir, "001_create_users.sql", `-- Up
CREATE TABLE users (id BIGSERIAL PRIMARY KEY, email TEXT);

-- Down
DROP TABLE IF EXISTS users;
`)
	writeMigration(t, dir, "002_bad.sql", `-- Up
DROP TABLE customers;
`)

	report := CheckMigrationFiles(dir)
	if report.HasErrors() {
		t.Fatalf("unexpected errors: %v", report.Issues)
	}
	if report.Warnings() == 0 {
		t.Fatal("expected a missing Down marker warning")
	}
}

func TestCheckMigrationFilesErrorsAndDestructive(t *testing.T) {
	dir := t.TempDir()
	writeMigration(t, dir, "001_drop_table.sql", `-- Up
DROP TABLE users;

-- Down
CREATE TABLE users (id int);
`)
	writeMigration(t, dir, "002_no_up.sql", `CREATE TABLE x (id int);`)

	report := CheckMigrationFiles(dir)
	if report.Errors() == 0 {
		t.Fatal("expected missing Up marker to be an error")
	}
	if report.DestructiveCount() == 0 {
		t.Fatal("expected destructive DROP TABLE to be reported")
	}

	var sawDestructive bool
	for _, issue := range report.Issues {
		if issue.Code == "destructive_sql" && issue.Destructive {
			sawDestructive = true
			if issue.Section != "Up" {
				t.Fatalf("destructive issue should be in Up, got %q", issue.Section)
			}
		}
	}
	if !sawDestructive {
		t.Fatal("missing destructive issue")
	}
}

func TestCheckMigrationFilesDoesNotWarnDestructiveDown(t *testing.T) {
	dir := t.TempDir()
	writeMigration(t, dir, "001_create_users.sql", `-- Up
CREATE TABLE users (id int);

-- Down
DROP TABLE users;
`)
	report := CheckMigrationFiles(dir)
	if report.DestructiveCount() != 0 {
		t.Fatalf("Down-section DROP should not fail a migrate safety check: %v", report.Issues)
	}
	if report.Errors() != 0 {
		t.Fatalf("unexpected errors: %v", report.Issues)
	}
}

func TestCheckMigrationFilesHelperAndTimestamp(t *testing.T) {
	dir := t.TempDir()
	writeMigration(t, dir, "001_create_users.sql", "-- Up\nCREATE TABLE users (id int);\n-- Down\nDROP TABLE users;")
	writeMigration(t, dir, "001_other.sql", "-- Up\nCREATE TABLE other (id int);\n-- Down\nDROP TABLE other;")
	writeMigration(t, dir, "no_timestamp.sql", "-- Up\nCREATE TABLE nope (id int);\n-- Down\nDROP TABLE nope;")
	writeMigration(t, dir, "extensions.sql", "-- CREATE EXTENSION IF NOT EXISTS pgcrypto;\n")

	report := CheckMigrationFiles(dir)
	var duplicate, missingTimestamp bool
	for _, issue := range report.Issues {
		switch issue.Code {
		case "duplicate_timestamp":
			duplicate = true
		case "missing_timestamp":
			missingTimestamp = true
		}
	}
	if !duplicate {
		t.Fatal("expected duplicate_timestamp warning")
	}
	if !missingTimestamp {
		t.Fatal("expected missing_timestamp warning")
	}
}

func TestIsDestructiveStatement(t *testing.T) {
	tests := []struct {
		sql  string
		want bool
	}{
		{"DROP TABLE users;", true},
		{"ALTER TABLE users DROP COLUMN email;", true},
		{"TRUNCATE users;", true},
		{"DELETE FROM users;", true},
		{"DELETE FROM users WHERE id = 1;", false},
		{"UPDATE users SET active = false;", true},
		{"UPDATE users SET active = false WHERE id = 1;", false},
		{"CREATE TABLE users (id int);", false},
		{"DROP INDEX idx ON users;", false},
		{"DROP TRIGGER IF EXISTS trg ON users;", false},
	}
	for _, tt := range tests {
		got, _ := IsDestructiveStatement(tt.sql)
		if got != tt.want {
			t.Errorf("IsDestructiveStatement(%q)=%v, want %v", tt.sql, got, tt.want)
		}
	}
}

func TestCheckMigrationFilesRejectsTransactionControl(t *testing.T) {
	dir := t.TempDir()
	writeMigration(t, dir, "001_bad_tx.sql", `-- Up
BEGIN;
CREATE TABLE users (id int);
COMMIT;
-- Down
DROP TABLE users;
`)
	report := CheckMigrationFiles(dir)
	if report.Errors() == 0 {
		t.Fatalf("expected explicit transaction control to be an error: %v", report.Issues)
	}
	var saw bool
	for _, issue := range report.Issues {
		if issue.Code == "transaction_control" && issue.Severity == SafetyError {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("missing transaction_control error: %v", report.Issues)
	}
}
