# vorm (Go) — Usage

Laravel-style `Schema.Create` + hybrid `// vorm:query`. **No sqlc** — `vorm generate` emits typed Go + SQL under `vorm/gen/`.

## Layout

| Path | Who | Purpose |
|------|-----|---------|
| `schema/migrations/` | you | Blueprint `Create*` |
| `queries/` | you | `// vorm:query` stubs (IR) |
| **`models/`** | generate | **DO NOT EDIT** — `vorm generate models` |
| `migrations/` | Schema.Create | SQL for `vm` |
| **`vorm/gen/`** | generate | SQL + sqlc-style `*Row` / `*Params` |

## Generated queries (sqlc-style)

Stub IR → typed **Row** + **Params** (like sqlc), never `SELECT *`:

```go
type ListActiveAdultsRow struct {
    ID     int64  `json:"id" db:"id"`
    Email  string `json:"email" db:"email"`
    Active bool   `json:"active" db:"active"`
    // …
}

type GetUserByEmailParams struct { Email string }

func ListActiveAdults(ctx context.Context, db query.DB) ([]ListActiveAdultsRow, error)
func GetUserByEmail(ctx context.Context, db query.DB, arg GetUserByEmailParams) (*GetUserByEmailRow, error)
```

## Security (Postgres / MySQL / MariaDB)

- Values **only** as bound params (`$n` / `?`) — never concatenated
- Identifiers: `SafeIdent` + dialect quoting
- Operators: whitelist (`SafeOp`); ORDER BY dir ASC/DESC only
- Column must exist on Meta; value types checked against model fields
- No `SELECT *`; `WhereRaw` rejects `;` / comments

## Config (`.vorm`)

```bash
vorm init                         # PACKAGE=gen by default
vorm config set PACKAGE=vormgen   # avoid name conflicts
vorm config get PACKAGE
vorm generate --package=vormgen   # one-off override
```

| Key | Default | Meaning |
|-----|---------|---------|
| `PACKAGE` | `gen` | Go package name for generated queries |
| `OUT_DIR` | `./vorm/<PACKAGE>` | Output directory |
| `DRIVER` | `pgx` | `pgx` or `pq` |
| `DIALECT` | `postgres` | `postgres` / `mysql` / `mariadb` |
| `MODEL_PACKAGE` | `models` | Models package name |

## Drivers

```go
// PostgreSQL — pgx v5 (default)
db, err := query.OpenPostgres(ctx, os.Getenv("DATABASE_URL"))

// PostgreSQL — lib/pq
db, err := query.OpenPostgres(ctx, url, query.WithDriver(query.PostgresPQ))
// or: query.OpenPostgresPQ(url)

// MySQL / MariaDB
db, err := query.OpenMySQL("user:pass@tcp(localhost:3306)/app?parseTime=true")
```

```bash
vorm generate --driver=pgx   # default
vorm generate --driver=pq
```

## Column checks

`Where("active", true)` fails at compile/exec if `active` is missing from `Meta.Columns`, or if the value type does not match the model field (e.g. string for `bool`).

## Generated queries

Stub:

```go
// vorm:query name=ListActiveAdults
func ListActiveAdults(ctx context.Context, db query.DB) ([]User, error) {
	return Users.Where("active", true).Where("age", ">", 18).OrderBy("name").Limit(10).Get(ctx, db)
}
```

Generated (`vorm/gen`):

```go
func ListActiveAdults(ctx context.Context, db query.DB) ([]models.User, error) {
	const listActiveAdultsSQL = `SELECT "id", … FROM "users" WHERE "active" = $1 AND "age" > $2 …`
	// scans into models.User — never SELECT *
}
```

Call **`gen.ListActiveAdults`**, not the stub, in app code.

See [`LLM.md`](LLM.md) · samples [`examples/generated/`](examples/generated/).
