---
name: vorzela-dialects
description: >-
  PostgreSQL (default), MySQL, and MariaDB support in Vorzela Migrate —
  DSN detection, feature matrix, scaffold SQL, online DDL. Use when choosing
  DATABASE_URL, writing dialect-specific SQL, debugging driver selection, or
  implementing multi-database behaviour.
---

# Vorzela Dialects

## Detection (authoritative)

`internal/migration/dialect.go` → `DetectDialect(dsn)`:

1. Prefix `mysql://` **or** substring `@tcp` / `tcp(` → MySQL family
2. If that DSN also contains `mariadb` → `MariaDB`, else `MySQL`
3. Else → **`PostgreSQL` (default)**

`internal/database/connection.go` → `Connect(dsn)` uses the same MySQL-family check, then:

- MySQL family → `db.ConnectMySQL`
- Else → `db.ConnectPostgres` (pgx pool)

MariaDB uses the MySQL adapter (no separate driver).

## DSN examples

```text
postgres://user:pass@localhost:5432/db          → postgres
postgresql://user:pass@localhost:5432/db       → postgres
mysql://user:pass@localhost:3306/db            → mysql
user:pass@tcp(localhost:3306)/db               → mysql
mysql://user:pass@mariadb:3306/db              → mariadb
mysql://user:pass@localhost:3306/mariadb_app   → mariadb
sqlite://… / anything else                     → postgres (default)
```

## Feature matrix

| Area | PostgreSQL | MySQL | MariaDB |
|------|:----------:|:-----:|:-------:|
| Connect + migrate/rollback/status/fresh/refresh | ✓ | ✓ | ✓ |
| Tracking table SQL | SERIAL / TIMESTAMPTZ | AUTO_INCREMENT / TIMESTAMP | same as MySQL |
| Lock | advisory | `GET_LOCK` | `GET_LOCK` |
| Drift inspect | ✓ | ✓ | ✓ |
| Online DDL (`--online`) | ✓ | ✓ | ✓ (MySQL-family path) |
| Concurrent index | ✓ only | ✗ | ✗ |
| extensions / functions / enums | ✓ | error | error |
| Auto-run ext/func/enums | ✓ | skip | skip |
| `vm make` scaffold | PG SQL + PL/pgSQL triggers | MySQL types + `SET NEW…` trigger | same as MySQL |

## Scaffold helpers

`cmd/make.go` passes `DetectDialect(cfg.DatabaseURL)` into `CreateMigrationOptions.Dialect`.

Helpers in `dialect.go`: `PrimaryKeyColumnSQL`, `TimestampColumnSQL`, `SoftDeleteColumnSQL`, `DropTableSQL`, `IsMySQLFamily`, `ResolveDialect`.

| | PostgreSQL | MySQL / MariaDB |
|--|------------|-----------------|
| PK | `BIGSERIAL PRIMARY KEY` | `BIGINT AUTO_INCREMENT PRIMARY KEY` |
| timestamps | `TIMESTAMPTZ` | `TIMESTAMP` |
| soft delete | `TIMESTAMPTZ DEFAULT NULL` | `TIMESTAMP NULL` |
| drop | `DROP TABLE IF EXISTS t CASCADE` | `DROP TABLE IF EXISTS t` |
| `--triggers` | `EXECUTE FUNCTION …()` + `functions.sql` | `SET NEW.updated_at = CURRENT_TIMESTAMP` (no `functions.sql`) |

## Online DDL

`online.go` switches:

```go
case PostgreSQL: …
case MySQL, MariaDB: … // ALGORITHM=INSTANT, fallback INPLACE
```

## Agent defaults

- Prefer **PostgreSQL** in docs/examples unless the user needs MySQL/MariaDB.
- Never call PG-only commands against a MySQL/MariaDB `DATABASE_URL`.
- Rely on `vm make` for dialect-correct scaffolds — do not hand-rewrite types.
