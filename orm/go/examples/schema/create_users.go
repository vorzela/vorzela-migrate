// Package schemaex shows Laravel-style Schema.Create in Go — one function per table.
package schemaex

import (
	"github.com/vorzela/vorm"
	"github.com/vorzela/vorm/schema"
)

// CreateUsersTable mirrors:
//
//	Schema::create('users', function (Blueprint $table) { … });
//
// Auto-writes migrations/*.sql and runs `vm migrate` (vorm.Schema.AutoMigrate).
func CreateUsersTable() error {
	return vorm.Schema.Create("users", func(t *schema.Blueprint) {
		t.ID()

		t.String("first_name")
		t.String("last_name")

		t.String("email").Unique()

		t.String("password")

		t.Boolean("active").Default(true)

		t.Integer("age").Nullable()

		t.Timestamps()

		t.SoftDeletes()
	})
}
