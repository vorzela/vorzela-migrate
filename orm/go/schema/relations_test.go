package schema_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vorzela/vorm/schema"
)

func TestBelongsToAndPivot(t *testing.T) {
	dir := t.TempDir()
	f := &schema.Facade{MigrationPath: dir, AutoMigrate: false, EnsureVM: false, Dialect: "postgres"}

	if err := f.Create("users", func(t *schema.Blueprint) {
		t.ID()
		t.String("email").NotNull()
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.Create("posts", func(t *schema.Blueprint) {
		t.ID()
		t.BelongsTo("user_id", "users")
		t.String("title").NotNull()
	}); err != nil {
		t.Fatal(err)
	}
	if err := schema.BelongsToMany(f, "post_tag", "post_id", "posts", "tag_id", "tags"); err != nil {
		t.Fatal(err)
	}

	posts := readOne(t, dir, "create_posts")
	if !strings.Contains(posts, "user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE") {
		t.Fatalf("belongsTo FK missing:\n%s", posts)
	}
	pivot := readOne(t, dir, "create_post_tag")
	if !strings.Contains(pivot, "post_id") || !strings.Contains(pivot, "tag_id") {
		t.Fatalf("pivot:\n%s", pivot)
	}
}

func readOne(t *testing.T, dir, substr string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), substr) {
			b, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			return string(b)
		}
	}
	t.Fatalf("no file matching %s in %v", substr, entries)
	return ""
}
