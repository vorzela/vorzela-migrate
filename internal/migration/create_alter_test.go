package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateAlterationTemplateInfersTableAndColumn(t *testing.T) {
	got := generateAlterationTemplate("add_email_to_users", CreateMigrationOptions{})
	for _, want := range []string{
		"ALTER TABLE users ADD COLUMN email TYPE;",
		"ALTER TABLE users DROP COLUMN IF EXISTS email;",
		"-- ⬆ Up",
		"-- ⬇ Down",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "\nCREATE TABLE") {
		t.Errorf("alteration template must not create a table:\n%s", got)
	}
}

func TestGenerateAlterationTemplateFallback(t *testing.T) {
	got := generateAlterationTemplate("add_something", CreateMigrationOptions{})
	if !strings.Contains(got, "your_table") || !strings.Contains(got, "your_column") {
		t.Errorf("expected placeholders, got:\n%s", got)
	}
}

func TestCreateMigrationUniqueTimestamps(t *testing.T) {
	dir := t.TempDir()
	if err := CreateMigrationWithOptions("create_users_table", dir, CreateMigrationOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := CreateMigrationWithOptions("add_email_to_users", dir, CreateMigrationOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := CreateMigrationWithOptions("create_posts_table", dir, CreateMigrationOptions{}); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		prefix := strings.SplitN(entry.Name(), "_", 2)[0]
		if seen[prefix] {
			t.Fatalf("duplicate timestamp prefix %q in %s", prefix, entry.Name())
		}
		seen[prefix] = true
	}

	addPath := ""
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "add_email_to_users") {
			addPath = filepath.Join(dir, entry.Name())
		}
	}
	if addPath == "" {
		t.Fatal("add migration not created")
	}
	content, err := os.ReadFile(addPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "\nCREATE TABLE") {
		t.Fatalf("add_ migration should be an ALTER template:\n%s", content)
	}
	if !strings.Contains(string(content), "ALTER TABLE users ADD COLUMN email") {
		t.Fatalf("add_ migration should infer users.email:\n%s", content)
	}
}
