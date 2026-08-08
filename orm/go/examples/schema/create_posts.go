package schemaex

import "github.com/vorzela/vorm/schema"

// CreatePostsTable — own migration (FK → users). Pass the same Facade when batching.
func CreatePostsTable(s *schema.Facade) error {
	if s == nil {
		s = schema.Default
	}
	return s.Create("posts", func(t *schema.Blueprint) {
		t.ID()
		t.String("title")
		t.Text("body")
		t.Enum("status", "draft", "published", "archived")
		t.ForeignId("user_id").Constrained("users").CascadeOnDelete()
		t.Index("user_id")
		t.Timestamps()
		t.SoftDeletes()
	})
}

// CreateTagsTable — own migration.
func CreateTagsTable(s *schema.Facade) error {
	if s == nil {
		s = schema.Default
	}
	return s.Create("tags", func(t *schema.Blueprint) {
		t.ID()
		t.String("name").Unique()
		t.Timestamps()
	})
}

// CreatePostTagTable — many-to-many pivot (own migration).
func CreatePostTagTable(s *schema.Facade) error {
	if s == nil {
		s = schema.Default
	}
	return s.Create("post_tag", func(t *schema.Blueprint) {
		t.ID()
		t.ForeignId("post_id").Constrained("posts").CascadeOnDelete()
		t.ForeignId("tag_id").Constrained("tags").CascadeOnDelete()
		t.Unique("post_id", "tag_id")
		t.Timestamps()
	})
}
