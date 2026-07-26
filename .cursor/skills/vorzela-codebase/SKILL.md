---
name: vorzela-codebase
description: >-
  Navigate and change the Vorzela Migrate Go codebase — packages under cmd/
  and internal/, dialect detection, DB adapters, migration executor, drift,
  locks, online DDL. Use when editing Go source, adding commands, fixing
  bugs, reviewing architecture, or extending database support.
---

# Vorzela Codebase

## Read first

- [ARCHITECTURE.md](ARCHITECTURE.md) — design overview
- [LLM.md](LLM.md) — agent-facing CLI contract (keep in sync when changing CLI)
- Skills: `vorzela-migrate` (CLI), `vorzela-dialects` (PG/MySQL/MariaDB)

## Layout

```
main.go                 # urfave/cli app wiring
cmd/                    # one file per command (make, migrate, rollback, …)
internal/
  config/               # .vm / .env loading + lint
  database/             # Connect() auto-detect → db.DB
  db/                   # DB interface + postgres (pgx) + mysql adapters
  migration/            # create, execute, dialect, drift, lock, online, …
  output/               # coloured logging
  version/              # version + upgrade notice
migrations/             # generated SQL (not source)
```

## Entry points by task

| Task | Start here |
|------|------------|
| New CLI command | `main.go` + new `cmd/*.go` |
| Config / `.vm` keys | `internal/config/config.go`, `lint.go` |
| DSN → driver | `internal/database/connection.go` |
| Dialect enum / detect | `internal/migration/dialect.go` |
| Scaffold SQL | `internal/migration/create.go`, `relationship.go` |
| Run / rollback | `internal/migration/executor.go`, `enhanced_executor.go` |
| Drift | `internal/migration/drift.go`, `schema_parser.go` |
| Locks | `internal/migration/lock.go` |
| Online DDL | `internal/migration/online.go` |
| PG extensions/functions/enums | `cmd/{extensions,functions,enums}.go` + matching `migration/*.go` |

## Design rules

1. **Adapter pattern** — migration code talks to `db.DB`, never raw drivers (except advanced paths that take `*sql.DB` for locks/online/drift).
2. **Detect, don’t configure** — no `--driver` flag; infer from DSN (`DetectDialect` / `Connect`).
3. **Default dialect is PostgreSQL** when DSN is ambiguous.
4. **MariaDB shares MySQL** adapter and SQL paths (`case MySQL, MariaDB:`), including online DDL.
5. **PG-only features** must gate on `DetectDialect(...) == PostgreSQL` and fail clearly otherwise (see `cmd/extensions.go`, `functions.go`, `enums.go`). Auto-run on migrate **skips** silently for non-PG.
6. **Scaffolds are dialect-aware** — `cmd/make.go` sets `CreateMigrationOptions.Dialect` from `DetectDialect(cfg.DatabaseURL)`. Column/trigger/drop SQL helpers live in `dialect.go`; keep `create.go` / `relationship.go` using those helpers.

## Changing behaviour checklist

When adding a `.vm` key or CLI flag:

- [ ] Parse in `internal/config`
- [ ] Add to known keys in `lint.go` (+ tests)
- [ ] Wire in the relevant `cmd/*.go`
- [ ] Update `LLM.md` and user `README.md`
- [ ] Add/adjust tests under `internal/**`

When touching dialect-sensitive SQL:

- [ ] Cover `PostgreSQL`, `MySQL`, and `MariaDB` in switch arms (or document intentional omission)
- [ ] Add table-driven tests in `*_test.go` (see `dialect_test.go`, `drift_test.go`, `lock_test.go`)

## Tests

```bash
go test ./...
go test ./internal/migration/ -run Dialect
```

## Do not

- Commit the `vm` binary or secrets in `.vm`
- Put `CREATE EXTENSION` / `CREATE OR REPLACE FUNCTION` into migration templates (validation rejects them; use dedicated files)
- Hand-rewrite MySQL types after `vm make` when `.vm` already has a MySQL/MariaDB `DATABASE_URL`
