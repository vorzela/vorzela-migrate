# AGENTS.md — Vorzela Migrate (`vm`)

This file is the short contract for AI agents and automation. Deep reference:
[`LLM.md`](LLM.md). Architecture: [`ARCHITECTURE.md`](ARCHITECTURE.md).

## Install location (no root)

Use the user-local binary, never `sudo` or `/usr/local/bin`:

```bash
GOBIN="$HOME/.local/bin" go install github.com/vorzela/vorzela-migrate/v2/cmd/vm@v2.3.4
```

Canonical paths: `~/.local/bin/vm` (Linux/macOS) and
`%LOCALAPPDATA%\Programs\vm\bin\vm.exe` (Windows). The install scripts and
`vm upgrade` write there. `vm uninstall` only manages that per-user copy.

## Golden rules

1. **Use `vm` commands, do not hand-write migration SQL** unless the user explicitly asks.
2. The only file an agent should create from scratch is `.vm` — prefer `vm init`.
3. Never create `extensions.sql`, `functions.sql`, or `enums.sql` by hand; let `vm` generate them.
4. Prefer editing the existing `create_*_table.sql` + `vm migrate --force --detect-drift`
   (or `vm fresh --force` on disposable DBs) over stacking `add_*` files. See LLM.md §1.1.
5. Run `vm check --json` before `vm migrate` when you wrote or changed SQL.
6. PostgreSQL is the default. Only use MySQL/MariaDB when the DSN/user requires it.

## Fast path

```bash
vm init --database-url "$DATABASE_URL"
vm lint
vm check --json
vm make migration users        # new table
vm check --json
vm migrate --yes               # non-interactive, safe auto-run defaults
vm status --json
```

## Safety / exit codes

| Command | Meaning |
|---------|---------|
| `vm check` | static checks; exit 1 on hard errors |
| `vm check --strict` | exit 1 on warnings too (CI gate) |
| `vm check --db` | also verifies checksums/missing executed files |
| `vm check --allow-destructive` | do not fail `--strict` on DROP/TRUNCATE warnings |

Hard errors: missing/empty Up, `CREATE EXTENSION`/`FUNCTION` in a migration,
unterminated SQL, unbalanced parentheses, explicit `BEGIN`/`COMMIT`/`ROLLBACK`.
Warnings: missing/empty Down, destructive Up SQL, `DROP TABLE`/`DROP COLUMN`/
`TRUNCATE`/`DELETE`/`UPDATE` without `WHERE`, transaction-hostile SQL without
`-- vm:no-transaction`, duplicate/missing timestamp prefixes.

## Non-interactive mode

- Global `--non-interactive` or `VM_NON_INTERACTIVE=1`.
- `vm migrate --yes` assumes **yes only for safe additive auto-run** (install
  missing extension/function/enum). It never approves destructive actions.
- `vm fresh`, `vm refresh`, `vm rollback`, and drop commands still need `--force`.
- `--json` output suppresses the version notice; stdout stays parseable.

## Important flags

```bash
vm migrate --dry-run                  # no lock, no execution
vm migrate --force                    # checksum override only
vm migrate --force --detect-drift     # after editing an executed create_* file
vm migrate --strict --allow-destructive
vm migrate --drift-handling=auto|prompt|reject
vm rollback --dry-run                 # preview Down SQL (works with --migration)
vm upgrade --check                    # semver-aware; no downgrades
vm uninstall --dry-run                # show what would be removed
```

## Migration file format

```sql
-- ⬆ Up (Run when migrating forward)
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- ⬇ Down (Run when rolling back)
DROP TABLE IF EXISTS users CASCADE;
```

- Add `-- vm:no-transaction` when using `CREATE INDEX CONCURRENTLY` or `VACUUM`.
- PostgreSQL migrations run in one transaction. MySQL/MariaDB DDL implicitly
  commits; vm reports statements applied before a failure.
- `add_<col>_to_<table>` generates an `ALTER TABLE … ADD COLUMN` scaffold (no
  `CREATE TABLE` and no `_table` suffix).

## Config precedence

CLI flags > real env vars (`DATABASE_URL`, `MIGRATION_PATH`) > `.vm` > `.env` > defaults.
`MIGRATION_PATH` in a parent `.vm` resolves relative to that file.

## Do not

- Do not add `BEGIN`/`COMMIT` to migrations.
- Do not treat commented-out template examples in `extensions.sql` / `enums.sql`
  as drop instructions; only `vm extensions drop` / `vm enums drop` remove things.
- Do not use `--force` for accidental checksum changes; restore from git instead.
- Do not run `vm extensions`/`functions`/`enums` on MySQL/MariaDB.
