# Migrations with vorm (via `vm`)

vorm does **not** reimplement migrations. The CLI shells out to **Vorzela Migrate (`vm`)**, which already handles locking, checksums, drift, rollbacks, and PostgreSQL extras (extensions, enums, functions).

## Install

```bash
go install github.com/vorzela/vorm/cmd/vorm@latest
# vm is auto-installed on first migrate if missing
vorm ensure-vm
```

Configure `DATABASE_URL` in `.vm` or the environment (same as `vm`).

## Laravel-like workflow

**Author in Go** (primary):

```go
vorm.Schema.Create("users", func(t *schema.Blueprint) {
    t.ID()
    t.String("email").Unique()
    t.Timestamps()
})
// → migrations/*.sql + vm migrate automatically
```

Scaffold a stub: `vorm make migration create_users_table`

Then use `vm` via vorm for status/rollback:

```bash
vorm lint
vorm migrate --enhanced --detect-drift
vorm status
vorm rollback
```

### Editing schema (thin history)

Prefer **editing the original create migration** and fixing drift when the migration has not been widely applied — same guidance as Vorzela Migrate / `LLM.md`.

```bash
vorm migrate --detect-drift
# or generate an alter:
# (Go) schema.Facade.Table("users", func(t *schema.Blueprint) { t.String("phone"); })
```

Avoid long `add_*` / `alter_*` chains when a create-file edit + drift repair is enough.

### Fresh / refresh

```bash
vorm fresh      # destructive — confirm
vorm refresh    # down all + up all
```

## PostgreSQL: extensions, enums, functions

| Concern | vorm | notes |
|---------|------|--------|
| Extension | `vorm make extension uuid-ossp` or `vorm extensions migrate` | Up: `CREATE EXTENSION IF NOT EXISTS`; Down: `DROP … CASCADE` |
| Enum type | `vorm make enum …` or Blueprint `t.Enum("status", …)` | Down drops type with `CASCADE` after table/indexes |
| Functions | `vorm functions migrate` or `Facade.CreateFunction` / `t.Raw` | Down: `DROP FUNCTION IF EXISTS … CASCADE` |
| Indexes | Blueprint `Index` / `Unique` | Down drops indexes then `DROP TABLE … CASCADE` then enum types |

Blueprint `Enum` on Postgres emits `CREATE TYPE table_column AS ENUM (…)`. Rollback drops indexes → table CASCADE → types CASCADE.

## Race conditions & errors

`vm migrate` uses **advisory locks** (Postgres) or **named locks** (MySQL) so two migrators cannot apply at once.

If vorm/vm exits with a lock-related error:

1. Wait for the other process
2. Check for a stuck session holding the lock
3. Re-run `vorm status` then `vorm migrate`

Checksum mismatch: someone edited an already-applied file. Prefer restoring the file, or `--force` only when you understand the risk.

vorm wraps non-zero exits in `vmtool.ExitError` with hints (lock / checksum / drift / connection).

## Drift

```bash
vorm migrate --enhanced --detect-drift
```

Configure drift mode in `.vm` (`auto` / `prompt` / `reject`) per Vorzela Migrate docs.

## Queries after schema

```bash
vorm generate   # // vorm:query → vorm/gen/ (no sqlc)
```

Pagination (offset vs cursor), search, and `ForceDelete` are documented in [`../README.md`](../README.md) and [`../examples/`](../examples/).
