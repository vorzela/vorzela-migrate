# vorm (Go) — LLM / agent guide

## Hard rules

1. **No sqlc** — `vorm generate` → `vorm/gen/` only.
2. **Models are generated** — never hand-edit `models/`; use `vorm generate models` after Blueprint changes.
3. **Annotation:** `// vorm:query name=FuncName` (space after `//`).
4. **Never `SELECT *`** — `Meta.Columns` / `.Select(...)`.
5. **No SQL injection** — bind values; SafeIdent on names.
6. **Columns must exist** — `Where("active", …)` requires `active` on Meta; types must match model fields.
7. Call **`gen.*`** generated funcs in apps; stubs in `queries/` are IR for codegen.

## Generated return types (sqlc-style)

Each lowered query emits:

- `FuncNameRow` — exact selected columns / types
- `FuncNameParams` — bound args (when the stub has params)
- Typed func returning `[]Row` / `*Row` / `int64`

Never `SELECT *`. Call `gen.GetUserByEmail(ctx, db, gen.GetUserByEmailParams{Email: e})`.

## Security

All dialects: parameterized SQL, SafeIdent, SafeOp, column+type checks, no injection via names/ops.

## Config

`.vorm` (KEY=value). Default generated package is **`gen`**.

```bash
vorm init
vorm config set PACKAGE=vormgen   # conflict avoidance
```

Keys: `PACKAGE`, `OUT_DIR`, `DRIVER`, `DIALECT`, `QUERY_DIR`, `MODEL_DIR`, `SCHEMA_DIR`, `MODEL_PACKAGE`, `MODEL_IMPORT`.

## Drivers

```go
query.OpenPostgres(ctx, url)                                 // pgx (default)
query.OpenPostgres(ctx, url, query.WithDriver(query.PostgresPQ)) // pq
query.OpenMySQL(dsn) / OpenMariaDB(dsn)
```

```bash
vorm generate --driver=pgx|pq
```

## Generated shape

- SQL const (parameterized `$n` / `?`)
- Typed signature returning `[]models.T` / `*models.T` / `int64`
- `scanT` aligned to Meta.Columns

## Anti-patterns

- Editing `models/` or `vorm/gen/` by hand
- `Where("actve", …)` typo — fails column check
- `Where("active", "yes")` on bool column — fails type check
- Depending on sqlc for vorm

## CLI

```bash
vorm make migration posts
vorm generate models && vorm generate
vorm lint && vorm migrate
```
