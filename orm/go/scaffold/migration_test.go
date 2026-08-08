package scaffold_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vorzela/vorm/scaffold"
)

func TestMakeMigrationPosts(t *testing.T) {
	root := t.TempDir()
	res, err := scaffold.MakeMigration("posts", scaffold.MigrationDirs{
		SchemaDir: filepath.Join(root, "schema"),
		ModelDir:  filepath.Join(root, "models"),
		QueryDir:  filepath.Join(root, "queries"),
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(res.MigrationFile)
	for _, want := range []string{
		"func CreatePostsTable(s *schema.Facade) error",
		`s.Create("posts"`,
		"t.ID()",
		"t.Timestamps()",
		"t.SoftDeletes()",
	} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("missing %q in:\n%s", want, body)
		}
	}
	if _, err := os.Stat(res.ModelFile); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(res.QueryFile); err != nil {
		t.Fatal(err)
	}
}

func TestMakeMigrationPivot(t *testing.T) {
	root := t.TempDir()
	res, err := scaffold.MakeMigration("post_user", scaffold.MigrationDirs{
		SchemaDir: filepath.Join(root, "schema"),
		ModelDir:  filepath.Join(root, "models"),
		QueryDir:  filepath.Join(root, "queries"),
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(res.MigrationFile)
	if !strings.Contains(string(body), `ForeignId("post_id")`) || !strings.Contains(string(body), `ForeignId("user_id")`) {
		t.Fatal(string(body))
	}
}
