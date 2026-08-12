package schema_test

import (
	"strings"
	"testing"

	"github.com/vorzela/vorm/schema"
)

func TestPostgresEnumAndIndexDrop(t *testing.T) {
	bp := schema.NewBlueprint("posts")
	bp.ID()
	bp.Enum("status", "draft", "published")
	bp.Index("status")
	up, down := bp.Compile("postgres")
	if !strings.Contains(up, "CREATE TYPE posts_status AS ENUM ('draft', 'published')") {
		t.Fatalf("up:\n%s", up)
	}
	if !strings.Contains(up, "status posts_status NOT NULL") {
		t.Fatalf("column:\n%s", up)
	}
	for _, want := range []string{
		"DROP INDEX IF EXISTS idx_posts_status",
		"DROP TABLE IF EXISTS posts CASCADE",
		"DROP TYPE IF EXISTS posts_status CASCADE",
	} {
		if !strings.Contains(down, want) {
			t.Fatalf("down missing %q:\n%s", want, down)
		}
	}
}

func TestCreateExtensionMigration(t *testing.T) {
	dir := t.TempDir()
	f := &schema.Facade{MigrationPath: dir, AutoMigrate: false, EnsureVM: false, Dialect: "postgres"}
	if err := f.CreateExtension("pgcrypto"); err != nil {
		t.Fatal(err)
	}
}
