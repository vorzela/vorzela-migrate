# Migrations

vorm applies migrations itself, in the process that calls it. The file format,
the `migrations` tracking table, checksums, batch numbers and locks match the
Vorzela Migrate (`vm`) CLI, so the same directory works with either tool.

```bash
export DATABASE_URL=postgres://user:pass@localhost:5432/app?sslmode=disable
vorm migrate
```

Set `RUNNER=vm` in `.vorm` to shell out to the `vm` binary instead
(`vorm ensure-vm` installs it). Nothing else in vorm depends on it.

## File format

One file per migration, named `<unix_timestamp>_<snake_case>.sql`, holding both
directions:

```sql
-- ⬆ Up (Run when migrating forward)
CREATE TABLE posts (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- ⬇ Down (Run when rolling back)
DROP TABLE IF EXISTS posts;
```

Files without a numeric prefix are ignored by the sequence, which is what keeps
`extensions.sql`, `enums.sql` and `functions.sql` out of it.

Scaffold one with `vorm make migration posts`; it also writes a matching model
placeholder and query stub.

## Commands

```bash
vorm migrate [--steps=N] [--dry-run] [--verbose] [--no-lint]
vorm status                       # applied, pending, changed-since-applied
vorm rollback [--steps=1]         # by batch, newest first
vorm rollback --steps=all
vorm rollback --migration=create_posts
vorm fresh --force                # roll everything back, then re-apply
```

`--dry-run` reports what would run without touching the database.
`--skip-lock` is available for environments where advisory locks are
unavailable, and should otherwise be left alone.

## Safety

**Locking.** PostgreSQL advisory locks and MySQL named locks mean two deploys
cannot apply at the same time. On a lock error: wait for the other process,
check for a stuck session, then rerun `vorm status` and `vorm migrate`.

**Transactions.** Each migration runs in a transaction when the dialect allows
it, so a failing statement leaves nothing half-applied. MySQL DDL is implicitly
committed; that is a database limitation, not a vorm one.

**Checksums.** The SHA-256 of each applied file is stored. `vorm status` reports
`CHANGED SINCE APPLIED` when a file was edited afterwards. Prefer restoring the
file; `--force` exists but should be a deliberate choice.

**Linting.** `vorm migrate` lints the directory first and refuses to run on
errors. `vorm lint` runs it alone; `--no-lint` skips the gate.

## Editing schema

Prefer editing the original create migration while it is still local or
unreleased, then rebuilding with `vorm fresh --force`. Long `add_*` / `alter_*`
chains are worth it only once the migration has been applied somewhere you
cannot reset.

Once a table exists in an environment you cannot drop, write the alter:

```sql
-- ⬆ Up (Run when migrating forward)
ALTER TABLE users ADD COLUMN phone VARCHAR(32);

-- ⬇ Down (Run when rolling back)
ALTER TABLE users DROP COLUMN phone;
```

Then `vorm generate` to pick the column up in the models.

## PostgreSQL prerequisites

`extensions.sql`, `enums.sql` and `functions.sql` live next to the migrations and
are declarative: an uncommented `CREATE` line is enabled, a commented one is
disabled. They are applied before the migrations, and only when their contents
changed since the last run (tracked in `.vm_*_hash` sidecars).

```bash
vorm extensions             # sync migrations/extensions.sql
vorm enums                  # create types and add new values
vorm functions              # CREATE OR REPLACE every function
vorm enums status           # file versus database
vorm enums --drop-disabled  # remove commented-out types too
vorm enums --dry-run        # print the SQL instead of running it
```

Enum syncing is re-runnable: each type becomes a block that creates it when
missing and otherwise issues `ALTER TYPE … ADD VALUE IF NOT EXISTS`. PostgreSQL
cannot remove an enum value, so deleting one from the file is not synced.
Dropping a disabled type is guarded — it is kept while any column still uses it.

For a change that must be versioned alongside a table, use a migration instead:
`vorm make enum order_status pending,paid,shipped` or
`vorm make extension pgcrypto` write ordinary migration files.

| Concern | Declarative | Versioned |
|---------|-------------|-----------|
| Extension | `extensions.sql` + `vorm extensions` | `vorm make extension <name>` |
| Enum type | `enums.sql` + `vorm enums` | `vorm make enum <type> a,b,c` |
| Function / trigger helper | `functions.sql` + `vorm functions` | `Facade.CreateFunction` |

## After migrating

```bash
vorm generate    # models from the live schema, then typed queries
```

Models are regenerated from the database, so a migration is only fully applied
once `vorm generate` has run.
