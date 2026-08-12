# vorm (Go)

A codegen-first data layer for PostgreSQL, MySQL and MariaDB: migrations, models
generated from the live database, and sqlc-style typed queries — one binary, no
external migration tool required.

```bash
go install github.com/vorzela/vorm/cmd/vorm@latest

export DATABASE_URL=postgres://user:pass@localhost:5432/app?sslmode=disable
vorm init                 # write .vorm (dialect detected from DATABASE_URL)
vorm make migration posts # scaffold Blueprint + model + query stub
vorm migrate              # apply migrations in-process
vorm generate             # models from the database + typed queries from stubs
```

## What generates what

| Path | Who writes it | Contents |
|------|---------------|----------|
| `migrations/` | you / `vorm make` | SQL applied by the runner |
| `migrations/{extensions,enums,functions}.sql` | you | declarative PostgreSQL prerequisites |
| `queries/` | you | `// vorm:query` stubs |
| `schema/migrations/` | you (optional) | Laravel-style `Blueprint` Go |
| **`models/`** | `vorm generate models` | **never hand-edit** — structs, enums, indexes, relations |
| **`vorm/gen/`** | `vorm generate` | **never hand-edit** — `*Row` / `*Params` + SQL |

## Migrations run in-process

The runner is a Go package (`migrate`), not a subprocess. It uses the same file
format, `migrations` tracking table, checksums, batches and advisory locks as
the `vm` CLI, so an existing project can switch either way.

```bash
vorm migrate [--steps=N] [--dry-run] [--verbose]
vorm status
vorm rollback [--steps=1] [--migration=create_users] [--steps=all]
vorm fresh --force          # roll everything back, then re-apply
```

A migration file is one `.sql` with both directions:

```sql
-- ⬆ Up (Run when migrating forward)
CREATE TABLE posts (id BIGSERIAL PRIMARY KEY, title TEXT NOT NULL);

-- ⬇ Down (Run when rolling back)
DROP TABLE IF EXISTS posts;
```

`vorm migrate` lints first (`--no-lint` to skip), takes a lock so two deploys
cannot race, runs each file in a transaction where the dialect allows it, and
records a SHA-256 of the file. `vorm status` flags a file that changed after it
was applied.

Set `RUNNER=vm` in `.vorm` to shell out to the `vm` binary instead; nothing else
in vorm needs it.

### PostgreSQL prerequisites

`extensions.sql`, `enums.sql` and `functions.sql` sit next to the migrations and
are declarative: an uncommented `CREATE` line means "I want this", a commented
one means "I do not". They are applied before the migrations, and only when the
file changed.

```bash
vorm extensions            # sync migrations/extensions.sql
vorm enums                 # create types, add new values
vorm functions             # CREATE OR REPLACE every function
vorm enums status          # compare the file against the database
vorm enums --drop-disabled # also remove commented-out types
```

Syncing is re-runnable: enums become a create-or-add-values block rather than a
`CREATE TYPE` that fails the second time, and a disabled enum is only dropped
when no column still uses it.

## Models come from the database

`vorm generate models` introspects the live schema, so nullability, enums,
indexes and foreign keys are exact rather than inferred:

```go
type UserStatus string

const (
	UserStatusActive  UserStatus = "active"
	UserStatusInvited UserStatus = "invited"
)

type User struct {
	ID        int64      `json:"id" db:"id"`
	Email     string     `json:"email" db:"email"`
	Name      *string    `json:"name" db:"name"`     // nullable → pointer
	Status    UserStatus `json:"status" db:"status"` // enum → typed constant
	DeletedAt *time.Time `json:"deleted_at" db:"deleted_at"`

	Posts []Post `json:"posts,omitempty" db:"-"` // from the foreign key
}

var Users = query.Model[User](query.Meta{
	Table:       UserTable,
	Columns:     UserColumnList,
	PrimaryKey:  "id",
	SoftDeletes: true,
	Indexes:     UserIndexes,
})
```

Without a reachable database, `--from-blueprint` parses `schema/migrations/`
instead. `vorm introspect [--json]` prints exactly what vorm reads.

## Queries: runtime builder and generated functions

Write a stub once. It is real, callable Go — and the generator lowers it to
static SQL where it can.

```go
// vorm:query name=SearchUsers
func SearchUsers(ctx context.Context, db query.DB, q string, limit int) ([]models.User, error) {
	return models.Users.WhereSearch([]string{"name", "email"}, q).OrderBy("name").Limit(limit).Get(ctx, db)
}
```

`vorm generate` emits a typed `Row`, a `Params` struct and the SQL:

```go
type SearchUsersRow struct { /* exactly the projected columns */ }
type SearchUsersParams struct { Q string; Limit int }

func SearchUsers(ctx context.Context, db query.DB, arg SearchUsersParams) ([]SearchUsersRow, error) {
	pattern1 := query.LikePattern(arg.Q)
	const searchUsersSQL = `SELECT "id", … FROM "users" WHERE ("name" ILIKE $1 OR "email" ILIKE $2) AND "deleted_at" IS NULL ORDER BY "name" ASC LIMIT $3`
	// …
}
```

Call `gen.SearchUsers` from application code. A stub the generator cannot lower
completely is reported by name and reason, and keeps working through the runtime
builder — generation never silently produces a function that fails.

### The builder

```go
users, err := models.Users.
	Where("active", true).
	Where("age", ">=", 18).
	WhereIn("status", "active", "invited").
	With("posts").              // batched eager load, no N+1
	OrderBy("created_at", "DESC").
	Limit(20).
	Get(ctx, db)

page, err := models.Users.Where("active", true).OrderBy("id").
	Paginate(ctx, db, query.PageRequest{Page: 2, PerPage: 25})
fmt.Println(page.TotalCount(), page.HasMore)

n, err := models.Users.Where("id", id).SoftDelete(ctx, db)
```

Rows scan into structs automatically through a cached reflection plan; no mapper
registration is needed.

## Type safety

Column names are checked against `Meta.Columns` and values against the model's
Go types, before any SQL is sent:

```go
models.Users.Where("actve", true)          // unknown column "actve"
models.Users.Where("age", "not-a-number")  // column "age" expects int32, got string
```

Identifiers are validated and quoted per dialect, operators come from a
whitelist, `ORDER BY` accepts only `ASC`/`DESC`, and values are always bound as
`$n` / `?`. Generated code never emits `SELECT *`.

## Errors and logging

Driver errors are classified, so callers branch on meaning instead of matching
strings:

```go
if _, err := models.Users.New().Create(ctx, db, values); err != nil {
	switch {
	case query.IsUniqueViolation(err):
		return fmt.Errorf("email %s is taken (%s)", email, query.Constraint(err))
	case query.IsRetryable(err):
		return retry(ctx)
	}
}
```

`query.Code`, `query.Constraint`, `query.Classify` and the `Is*` helpers cover
PostgreSQL SQLSTATE and MySQL errno. Logging is an interface with an `slog`
implementation:

```go
query.SetDefaultLogger(query.NewSlogLogger(slog.Default()))
ctx = query.WithLogger(ctx, myLogger) // or per-request
```

Each event carries the SQL, arguments, duration, rows affected and error.

## Relations

Generated models register their foreign keys, so `With("posts")` loads every
parent's children in one extra query:

```go
query.RegisterRelation(query.Relation{
	Name: "posts", Kind: query.RelationHasMany, Table: "posts",
	LocalKey: "id", ForeignKey: "user_id",
}, func(ctx context.Context, db query.DB, rows []*User) error {
	return query.LoadHasMany(ctx, db, rows, query.HasMany[User, Post]{
		Related:    Posts,
		ForeignKey: "user_id",
		ParentKey:  func(m *User) any { return m.ID },
		ChildKey:   func(r *Post) any { return r.UserID },
		Assign:     func(m *User, rows []Post) { m.Posts = rows },
	})
})
```

`LoadHasMany`, `LoadBelongsTo` and `LoadBelongsToMany` are also usable directly.

## Drivers

```go
db, err := query.Open(ctx, os.Getenv("DATABASE_URL"))       // picks the driver
db, err := query.OpenPostgres(ctx, url)                     // pgx v5 (default)
db, err := query.OpenPostgres(ctx, url, query.WithDriver(query.PostgresPQ))
db, err := query.OpenMySQL("user:pass@tcp(localhost:3306)/app?parseTime=true")
```

## Config (`.vorm`)

```bash
vorm config                     # effective values and where they came from
vorm config set PACKAGE=vormgen # avoid a clash with another "gen" package
vorm config lint
```

| Key | Default | Meaning |
|-----|---------|---------|
| `PACKAGE` | `gen` | package name for generated queries |
| `OUT_DIR` | `./vorm/<PACKAGE>` | where `queries_gen.go` is written |
| `DRIVER` | `pgx` | `pgx` or `pq` |
| `DIALECT` | `postgres` | `postgres`, `mysql`, `mariadb` |
| `RUNNER` | `native` | `native` (in-process) or `vm` |
| `MIGRATION_PATH` | `./migrations` | SQL directory |
| `MODEL_SOURCE` | `db` | `db` (introspect) or `blueprint` |
| `MODEL_DIR` / `MODEL_PACKAGE` | `./models` / `models` | generated models |
| `DATABASE_URL` | — | overridden by the environment variable |

## Further reading

- [`docs/USAGE.md`](docs/USAGE.md) — end-to-end usage guide, from install to recipes
- [`docs/MIGRATIONS.md`](docs/MIGRATIONS.md) — file format, locks, prerequisites
- [`LLM.md`](LLM.md) — rules for agents working in a vorm project
- [`examples/`](examples/) — stubs and their generated output
