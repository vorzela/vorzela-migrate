# Schema + models from CLI

```bash
vorm make migration posts
vorm make migration post_user
```

Creates:

| File | Purpose |
|------|---------|
| `schema/migrations/create_posts.go` | Blueprint (`CreatePostsTable`) — ID, Timestamps, SoftDeletes (+ comments) |
| `models/post.go` | Model + `query.Model` registry |
| `queries/posts.go` | `// vorm:query` stub for you to fill |

Pivot names like `post_user` scaffold both FKs automatically.

## Workflow

1. **Edit** the Blueprint (columns, enums, FKs):

```go
return s.Create("posts", func(t *schema.Blueprint) {
    t.ID()
    t.String("title")
    t.Enum("status", "draft", "published", "archived")
    t.ForeignId("user_id").Constrained("users").CascadeOnDelete()
    t.Timestamps()
    t.SoftDeletes()
})
```

2. **Apply** schema: `CreatePostsTable(nil)` → SQL + `vm migrate`

3. **Refresh model** (enums, fields, columns list):

```bash
vorm generate models
```

4. **Write queries** in `queries/*.go`, then:

```bash
vorm generate    # → vorm/gen (pgx v5 / MySQL; no sqlc)
```

Hand-written examples remain in this folder for reference (`create_users.go`, etc.).
