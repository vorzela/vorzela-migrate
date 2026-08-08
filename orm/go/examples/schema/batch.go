package schemaex

import (
	"github.com/vorzela/vorm"
	"github.com/vorzela/vorm/schema"
)

// MigrateBlogSchema creates related tables with one vm migrate at the end.
// Still one Schema.Create per table — not flag-based CLI scaffolds.
func MigrateBlogSchema() error {
	return vorm.Schema.Batch(func(s *schema.Facade) error {
		if err := CreateUsersTableWith(s); err != nil {
			return err
		}
		if err := CreateTagsTable(s); err != nil {
			return err
		}
		if err := CreatePostsTable(s); err != nil {
			return err
		}
		return CreatePostTagTable(s)
	})
}

// CreateUsersTableWith is CreateUsersTable against an explicit Facade (for Batch).
func CreateUsersTableWith(s *schema.Facade) error {
	if s == nil {
		s = schema.Default
	}
	return s.Create("users", func(t *schema.Blueprint) {
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
