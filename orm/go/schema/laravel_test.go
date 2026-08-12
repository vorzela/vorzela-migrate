package schema_test

import (
	"strings"
	"testing"

	"github.com/vorzela/vorm/schema"
)

func TestUsersBlueprintLikeLaravel(t *testing.T) {
	bp := schema.NewBlueprint("users")
	bp.ID()
	bp.String("first_name")
	bp.String("last_name")
	bp.String("email").Unique()
	bp.String("password")
	bp.Boolean("active").Default(true)
	bp.Integer("age").Nullable()
	bp.Timestamps()
	bp.SoftDeletes()
	if err := schema.ValidateBlueprint(bp); err != nil {
		t.Fatal(err)
	}
	up, down := bp.Compile("postgres")
	for _, want := range []string{
		"first_name VARCHAR(255) NOT NULL",
		"email VARCHAR(255) NOT NULL UNIQUE",
		"active BOOLEAN NOT NULL DEFAULT TRUE",
		"age INTEGER NULL",
		"DROP TABLE IF EXISTS users CASCADE",
	} {
		if !strings.Contains(up, want) && !strings.Contains(down, want) {
			t.Fatalf("missing %q\nup:\n%s", want, up)
		}
	}
}

func TestForeignKeyOnDelete(t *testing.T) {
	bp := schema.NewBlueprint("posts")
	bp.ID()
	bp.ForeignId("user_id").Constrained("users").CascadeOnDelete()
	up, _ := bp.Compile("postgres")
	if !strings.Contains(up, "REFERENCES users(id) ON DELETE CASCADE") {
		t.Fatal(up)
	}
	bp2 := schema.NewBlueprint("posts")
	bp2.ForeignId("user_id").Constrained("users").NullOnDelete()
	up2, _ := bp2.Compile("postgres")
	if !strings.Contains(up2, "ON DELETE SET NULL") || !strings.Contains(up2, "user_id BIGINT NULL") {
		t.Fatal(up2)
	}
}
