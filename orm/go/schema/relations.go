package schema

import "fmt"

// BelongsToMany writes a classic pivot table migration (many-to-many).
//
//	schema.BelongsToMany(facade, "post_tag", "post_id", "posts", "tag_id", "tags")
func BelongsToMany(f *Facade, pivot, leftCol, leftTable, rightCol, rightTable string) error {
	if f == nil {
		f = Default
	}
	return f.Create(pivot, func(t *Blueprint) {
		t.ID()
		t.BelongsTo(leftCol, leftTable)
		t.BelongsTo(rightCol, rightTable)
		t.Unique(leftCol, rightCol)
		t.Timestamps()
	})
}

// HasManyComment documents the inverse of BelongsTo (FK lives on the many side).
func HasManyComment(parentTable, childTable, fk string) string {
	return fmt.Sprintf("// hasMany: %s has many %s via %s.%s → %s.id",
		parentTable, childTable, childTable, fk, parentTable)
}
