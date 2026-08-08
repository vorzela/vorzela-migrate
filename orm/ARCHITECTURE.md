# vorm architecture

## Layout

```
orm/
  go/            ← Go implementation (github.com/vorzela/vorm)
  typescript/    ← planned
  python/        ← planned
```

Languages share the same product ideas (schema → `vm`, hybrid `vorm:query` → parameterized SQL).
Runtimes and emitters stay per-language; do not share one binary across all.

## Goals

- Laravel-like **authoring** (Schema builder, fluent queries)
- **Runtime** = parameterized raw SQL via `vorm/gen` + drivers (**pgx v5** / MySQL) — **no sqlc**
- Stay **optional**: apps can keep using only `vm` + hand-written SQL
- Reuse **Vorzela Migrate** for apply/rollback/drift
- **Security**: bind all values; quote/validate identifiers; never `SELECT *`

## Hybrid query pipeline (Go)

```
  // vorm:query stub (fluent IR)  OR  Users.Where(...).Get
           │
           ▼
  vorm generate → vorm/gen/   OR   runtime CompileSelect (pgx $n / MySQL ?)
```

Today: annotation discovery + generated stubs + runtime builders with SafeIdent.  
Tomorrow: lower full IR to dialect-aware Go under `vorm/gen`.

## Schema pipeline

```
  vorm make migration …
           │
           ▼
  migrations/*.sql
           │
           ▼
  vorm migrate|rollback|status   →  vm (locks, drift, checksums)
```

## Module boundary

| Piece | Responsibility |
|-------|----------------|
| `vorzela-migrate` (`vm`) | Migrations, drift, online, locking |
| `vorm` (`orm/go`) | Schema DSL, query stubs, codegen, drivers |

ORM shells out to the **`vm` binary**; it does not import migrate packages.
