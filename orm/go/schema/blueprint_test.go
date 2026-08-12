package schema_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vorzela/vorm/schema"
)

func TestBlueprintPostgresCreate(t *testing.T) {
	bp := schema.NewBlueprint("users")
	bp.ID()
	bp.String("email").Unique().NotNull()
	bp.Timestamps()
	bp.SoftDeletes()
	up, down := bp.Compile("postgres")
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS users",
		"id BIGSERIAL PRIMARY KEY",
		"email VARCHAR(255) NOT NULL UNIQUE",
		"created_at TIMESTAMPTZ",
		"deleted_at TIMESTAMPTZ NULL",
		"DROP TABLE IF EXISTS users CASCADE",
	} {
		if !strings.Contains(up, want) && !strings.Contains(down, want) {
			t.Fatalf("missing %q\nup:\n%s\ndown:\n%s", want, up, down)
		}
	}
}

func TestBlueprintMySQLCreate(t *testing.T) {
	bp := schema.NewBlueprint("posts")
	bp.ID()
	bp.String("title").NotNull()
	bp.Timestamps()
	up, down := bp.Compile("mysql")
	if !strings.Contains(up, "BIGINT AUTO_INCREMENT PRIMARY KEY") {
		t.Fatalf("expected mysql PK, got:\n%s", up)
	}
	if !strings.Contains(down, "DROP TABLE IF EXISTS posts;") || strings.Contains(down, "CASCADE") {
		t.Fatalf("unexpected mysql drop:\n%s", down)
	}
}

func TestFacadeWritesMigration(t *testing.T) {
	dir := t.TempDir()
	f := &schema.Facade{
		MigrationPath: dir,
		AutoMigrate:   false,
		EnsureVM:      false,
		Dialect:       "postgres",
	}
	if err := f.Create("accounts", func(t *schema.Blueprint) {
		t.ID()
		t.String("name").NotNull()
		t.Timestamps()
	}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 migration file, got %d", len(entries))
	}
	body, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if !strings.Contains(string(body), "CREATE TABLE IF NOT EXISTS accounts") {
		t.Fatalf("bad migration body:\n%s", body)
	}
}
