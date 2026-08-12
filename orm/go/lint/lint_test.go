package lint_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vorzela/vorm/lint"
	"github.com/vorzela/vorm/scaffold"
)

func TestLintCatchesMissingDownType(t *testing.T) {
	body := `-- ⬆ Up
CREATE TYPE foo AS ENUM ('a');
CREATE TABLE x (id int);
-- ⬇ Down
DROP TABLE IF EXISTS x;
`
	fs := lint.File("t.sql", body)
	found := false
	for _, f := range fs {
		if f.Severity == lint.Error && strings.Contains(f.Message, "DROP TYPE") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected DROP TYPE error, got %+v", fs)
	}
}

func TestDirIgnoresDeclarativeHelpers(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"1700000000_create_users_table.sql": "-- ⬆ Up\nCREATE TABLE users (id int);\n-- ⬇ Down\nDROP TABLE IF EXISTS users;\n",
		"extensions.sql":                    "CREATE EXTENSION IF NOT EXISTS citext;\n",
		"enums.sql":                         "CREATE TYPE status AS ENUM ('on');\n",
		"functions.sql":                     "CREATE OR REPLACE FUNCTION f() RETURNS int AS $$ SELECT 1 $$ LANGUAGE sql;\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	res, err := lint.Dir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range res.Findings {
		if strings.Contains(f.File, "extensions.sql") || strings.Contains(f.File, "enums.sql") || strings.Contains(f.File, "functions.sql") {
			t.Errorf("declarative helper was linted as a migration: %+v", f)
		}
	}
	if res.HasErrors() {
		t.Errorf("clean directory reported errors: %+v", res.Findings)
	}
}

func TestScaffoldAndLintOK(t *testing.T) {
	dir := t.TempDir()
	path, err := scaffold.CreateTable(scaffold.TableOptions{
		Table:     "posts",
		Path:      dir,
		Dialect:   "postgres",
		Soft:      true,
		BelongsTo: []string{"user_id:users"},
		Strings:   []string{"title"},
		Enums:     []string{"status:draft,published"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "vorm scaffold") {
		t.Fatalf("missing suggestion header:\n%s", body)
	}
	res, err := lint.Dir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.HasErrors() {
		t.Fatal(lint.Format(res))
	}
	// ensure only one file
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("files: %v", entries)
	}
	_ = filepath.Base(path)
}
