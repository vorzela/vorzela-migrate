package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeVM(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".vm")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writeVM: %v", err)
	}
	return path
}

func TestLintVMFile_NotExist(t *testing.T) {
	issues, err := LintVMFile("/nonexistent/.vm")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if issues != nil {
		t.Errorf("expected nil issues for missing file, got %v", issues)
	}
}

func TestLintVMFile_ValidFile(t *testing.T) {
	path := writeVM(t, `DATABASE_URL=postgres://user:pass@localhost:5432/db
ENVIRONMENT=development
DRIFT_HANDLING=prompt
ENHANCED=true
VERBOSE=false
AUTO_RUN_FUNCTIONS=true
AUTO_RUN_EXTENSIONS=true
AUTO_RUN_ENUMS=false
MIGRATION_PATH=./migrations
`)
	issues, err := LintVMFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, iss := range issues {
		if iss.Severity == LintError {
			t.Errorf("unexpected error issue: %s", iss)
		}
	}
}

func TestLintVMFile_UnknownKey(t *testing.T) {
	path := writeVM(t, `DATABASE_URL=postgres://x
DATABSE_URL=postgres://typo
`)
	issues, err := LintVMFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, iss := range issues {
		if iss.Key == "DATABSE_URL" && iss.Severity == LintError {
			found = true
		}
	}
	if !found {
		t.Error("expected LintError for unknown key DATABSE_URL")
	}
}

func TestLintVMFile_InvalidBoolean(t *testing.T) {
	path := writeVM(t, `DATABASE_URL=postgres://x
ENHANCED=yes
`)
	issues, err := LintVMFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, iss := range issues {
		if iss.Key == "ENHANCED" && iss.Severity == LintError {
			found = true
		}
	}
	if !found {
		t.Error("expected LintError for invalid boolean ENHANCED=yes")
	}
}

func TestLintVMFile_InvalidEnvironment(t *testing.T) {
	path := writeVM(t, `DATABASE_URL=postgres://x
ENVIRONMENT=staging
`)
	issues, err := LintVMFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, iss := range issues {
		if iss.Key == "ENVIRONMENT" && iss.Severity == LintError {
			found = true
		}
	}
	if !found {
		t.Error("expected LintError for ENVIRONMENT=staging")
	}
}

func TestLintVMFile_InvalidDriftHandling(t *testing.T) {
	path := writeVM(t, `DATABASE_URL=postgres://x
DRIFT_HANDLING=skip
`)
	issues, err := LintVMFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, iss := range issues {
		if iss.Key == "DRIFT_HANDLING" && iss.Severity == LintError {
			found = true
		}
	}
	if !found {
		t.Error("expected LintError for DRIFT_HANDLING=skip")
	}
}

func TestLintVMFile_MissingDatabaseURL(t *testing.T) {
	path := writeVM(t, `ENVIRONMENT=development
`)
	issues, err := LintVMFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, iss := range issues {
		if iss.Key == "DATABASE_URL" && iss.Severity == LintWarning {
			found = true
		}
	}
	if !found {
		t.Error("expected LintWarning for missing DATABASE_URL")
	}
}

func TestLintVMFile_DuplicateKey(t *testing.T) {
	path := writeVM(t, `DATABASE_URL=postgres://a
DATABASE_URL=postgres://b
ENVIRONMENT=development
`)
	issues, err := LintVMFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, iss := range issues {
		if iss.Key == "DATABASE_URL" && iss.Severity == LintWarning {
			found = true
		}
	}
	if !found {
		t.Error("expected LintWarning for duplicate DATABASE_URL")
	}
}

func TestLintVMFile_MalformedLine(t *testing.T) {
	path := writeVM(t, `DATABASE_URL=postgres://x
BADLINE
`)
	issues, err := LintVMFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, iss := range issues {
		if iss.Severity == LintWarning && iss.Key == "BADLINE" {
			found = true
		}
	}
	if !found {
		t.Error("expected LintWarning for malformed line")
	}
}

func TestLintVMFile_CommentsIgnored(t *testing.T) {
	path := writeVM(t, `# This is a comment
DATABASE_URL=postgres://x
# ENVIRONMENT=not_parsed
`)
	issues, err := LintVMFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, iss := range issues {
		if iss.Severity == LintError {
			t.Errorf("unexpected error for commented-out content: %s", iss)
		}
	}
}

func TestClosestKey_Suggestion(t *testing.T) {
	// DATABSE_URL should suggest DATABASE_URL
	suggestion := closestKey("DATABSE_URL")
	if suggestion != "DATABASE_URL" {
		t.Errorf("closestKey(DATABSE_URL) = %q, want DATABASE_URL", suggestion)
	}
}
