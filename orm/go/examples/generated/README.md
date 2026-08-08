# Example generated output (no sqlc binary — vorm owns codegen)

| Sample | Maps to |
|--------|---------|
| `models/*.go` | `./models/` — **DO NOT EDIT** |
| `vorm/gen/queries_gen.go` | sqlc-style `*Row` / `*Params` + SQL |

```go
rows, err := gen.ListActiveAdults(ctx, db) // []ListActiveAdultsRow
u, err := gen.GetUserByEmail(ctx, db, gen.GetUserByEmailParams{Email: "a@b.c"})
```
