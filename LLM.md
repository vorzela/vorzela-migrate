# Vorzela Migration Tool — LLM / AI Agent Reference

> **Primary directive for AI agents:** Run `vm` commands — do not create files manually unless explicitly asked.  
> The CLI generates migration files, `extensions.sql`, `functions.sql`, and `enums.sql` for you.  
> Use `vm lint` to validate the `.vm` config. Use `vm migrate --detect-drift` to surface and fix manual schema changes.
>
> **Schema changes — keep migration history thin:** Prefer editing the existing `create_*_table` migration + drift (or `vm fresh` in disposable DBs).  
> Do **not** default to `vm make migration add_…`. Use `add_` only when drift cannot safely apply the change (see [§1.1](#11-schema-change-decision-tree--avoid-migration-sprawl)).
>
> **`.vm` syntax highlighting:** see [`editors/README.md`](editors/README.md) (VS Code/Cursor: hover docs + live lint for unknown/required keys under `editors/vscode/`).

**Supported databases (auto-detected from `DATABASE_URL`):** MySQL · MariaDB · **PostgreSQL (default)**

**Project agent skills** (`.cursor/skills/`): `vorzela-migrate` (CLI), `vorzela-dialects` (drivers/matrix), `vorzela-codebase` (Go layout).

---

## Table of Contents

0. [Supported Databases](#0-supported-databases)
1. [Key Principle: Prefer Commands Over Files](#1-key-principle-prefer-commands-over-files)
   - [1.1 Schema change decision tree](#11-schema-change-decision-tree--avoid-migration-sprawl)
2. [.vm Config File — Setup and Variables](#2-vm-config-file--setup-and-variables)
3. [Linting the .vm Config](#3-linting-the-vm-config)
4. [Creating Migration Files](#4-creating-migration-files)
5. [Running Migrations](#5-running-migrations)
   - [5.1 `vm migrate --force`](#51-vm-migrate---force--checksum-override)
6. [Schema Drift Detection](#6-schema-drift-detection)
7. [Rolling Back](#7-rolling-back)
8. [Status](#8-status)
9. [Refresh / Fresh (Reset the Schema)](#9-refresh--fresh-reset-the-schema)
10. [Extensions (PostgreSQL only)](#10-extensions-postgresql-only)
11. [Functions (PostgreSQL only)](#11-functions-postgresql-only)
12. [Enums (PostgreSQL only)](#12-enums-postgresql-only)
13. [Migration File Format](#13-migration-file-format)
14. [Complete .vm Variable Reference](#14-complete-vm-variable-reference)
15. [Validation Rules](#15-validation-rules)
16. [Error Reference](#16-error-reference)
17. [Command Quick-Reference](#17-command-quick-reference)

---

## 0. Supported Databases

Vorzela supports three engines. The driver and dialect are inferred from `DATABASE_URL` / `--dsn` — there is no separate `--driver` flag.

| Engine | Versions | Detection | Driver |
|--------|----------|-----------|--------|
| **PostgreSQL** (default) | 10+ | Anything that is not MySQL/MariaDB (including `postgres://`, `postgresql://`, or ambiguous DSNs) | `pgx/v5` |
| **MySQL** | 5.7+ | `mysql://…` or DSN containing `@tcp` / `tcp(` | `go-sql-driver/mysql` |
| **MariaDB** | 10.3+ | Same MySQL patterns **and** the substring `mariadb` in the DSN | `go-sql-driver/mysql` |

### Connection string formats

```ini
# PostgreSQL (default) — prefer this unless the user asks for MySQL/MariaDB
DATABASE_URL=postgres://user:pass@localhost:5432/mydb
# Also accepted: postgresql://…

# MySQL — URL style
DATABASE_URL=mysql://user:pass@localhost:3306/mydb

# MySQL — classic DSN style
DATABASE_URL=user:pass@tcp(localhost:3306)/mydb

# MariaDB — use mysql:// (or @tcp) and include "mariadb" so dialect = mariadb
DATABASE_URL=mysql://user:pass@localhost:3306/mariadb_mydb
# Or host/path that contains "mariadb":
# DATABASE_URL=mysql://user:pass@mariadb-host:3306/mydb
```

Detection logic (source of truth: `internal/migration/dialect.go` + `internal/database/connection.go`):
1. If DSN starts with `mysql://` **or** contains `@tcp` / `tcp(` → MySQL path
2. Within that path, if DSN also contains `mariadb` → dialect `mariadb`, else `mysql`
3. **Everything else defaults to PostgreSQL**

### Feature matrix

| Feature | PostgreSQL | MySQL | MariaDB |
|---------|:----------:|:-----:|:-------:|
| `vm migrate` / `rollback` / `status` / `fresh` / `refresh` | ✓ | ✓ | ✓ |
| Enhanced mode (checksums + lock + drift) | ✓ | ✓ | ✓ |
| Migration lock | advisory lock | `GET_LOCK` | `GET_LOCK` |
| Schema drift inspect/repair | ✓ | ✓ | ✓ |
| Online / zero-downtime (`--online`) | ✓ | ✓ (8.0+) | ✓ (same MySQL-family DDL) |
| `vm extensions` / `functions` / `enums` | ✓ | ✗ | ✗ |
| Auto-run extensions/functions/enums before migrate | ✓ | skipped | skipped |
| `vm make migration` scaffold SQL | PG types + PL/pgSQL triggers | MySQL types + `SET NEW.updated_at` trigger | same as MySQL |

### Critical agent rules for MySQL / MariaDB

1. **Do not** run `vm extensions`, `vm functions`, or `vm enums` — they error with “not supported on mysql/mariadb”.
2. Dialect comes from `DATABASE_URL` in `.vm` — `vm make migration` scaffolds the correct types automatically (`BIGINT AUTO_INCREMENT`, `TIMESTAMP`, no `DROP … CASCADE`).
3. `--triggers` is supported on MySQL/MariaDB: generates a single-statement `SET NEW.updated_at = CURRENT_TIMESTAMP` trigger (does **not** create `functions.sql`). Soft-delete *update protection* remains PostgreSQL-only; MySQL/MariaDB still get the timestamp auto-update.
4. Prefer PostgreSQL in examples and new projects unless the user explicitly needs MySQL/MariaDB.
5. `--online` works for PostgreSQL, MySQL, and MariaDB (MariaDB uses the same ALGORITHM=INSTANT/INPLACE path as MySQL).

---

## 1. Key Principle: Prefer Commands Over Files

| Task | Wrong approach | Correct approach |
|------|---------------|-----------------|
| Create a **new table** | Write a `.sql` file manually | `vm make migration <name>` |
| Add a **column / tweak** an existing table | `vm make migration add_…` (default instinct — wrong) | Edit the existing `create_*_table.sql`, then drift or `fresh` — see [§1.1](#11-schema-change-decision-tree--avoid-migration-sprawl) |
| Add extensions | Write to `extensions.sql` manually | `vm extensions migrate` (generates file on first run) |
| Add trigger functions | Write to `functions.sql` manually | `vm functions migrate` (generates file on first run) |
| Add enum types | Write to `enums.sql` manually | `vm enums migrate` (generates file on first run) |
| Validate config | Read the `.vm` file | `vm lint` |
| Live DB out of sync with migration SQL | Write ALTER SQL or a new `add_` file | `vm migrate --detect-drift` → answer **yes** |
| Check what's pending | Inspect files | `vm status` |

**Never** create migration files directly. **Never** create `extensions.sql`, `functions.sql`, or `enums.sql` directly. Run the command — it creates the correctly formatted template, then you (or the user) edit it if needed.

---

## 1.1 Schema change decision tree — avoid migration sprawl

**Goal:** One `create_*_table` file per table is the source of truth. Avoid a pile of `add_*` / `fix_schema_drift` files for every tweak.

### When the user asks to change a table (add column, rename field, add index, soft-delete, etc.)

Walk this tree **in order**. Stop at the first match.

```
1. Is this a brand-new table that does not exist yet?
   → YES: vm make migration <table>   (only legitimate default for "make")
   → NO: continue

2. Can you safely rebuild this DB? (local / dev / empty / user OK with vm fresh)
   → YES: Edit the existing create_<table>_table.sql (Up + Down).
          Then: vm fresh --force   (or rollback that batch + migrate)
          Done. No new migration file.

3. Is the create_<table>_table migration already executed, but you can still edit it?
   (dev team owns the DB; checksums can be forced; not a locked production history)
   → YES: Edit create_<table>_table.sql so CREATE TABLE matches the desired final schema.
          Sync the live DB with:
            vm migrate --force --detect-drift
          → answer yes to drift prompts.
          (--force is required because the executed file’s checksum changed.)
          If the DB is disposable, prefer: vm fresh --force
          Done. Still no add_ file.

4. Did drift print an advisory that it CANNOT auto-apply?
   (NOT NULL without DEFAULT, or UNIQUE on a populated table)
   → YES: NOW you may use  vm make migration add_<col>_to_<table>
          Write backfill + constraint by hand, then vm migrate.
          This is the main legitimate use of add_.

5. Is this a production / shared DB where executed migration files are immutable history?
   → YES: Only then prefer a new forward migration (add_ / alter) that other envs will run.
          Still prefer one purposeful migration over many tiny ones.
```

### Prefer / avoid cheatsheet

| Prefer | Avoid |
|--------|--------|
| Edit `create_*_table.sql` so it describes the **final** schema | Spawning `add_email_to_users`, `add_phone_to_users`, … for every column |
| `vm migrate --force --detect-drift` → **yes** to sync live DB after editing create | `generate` → keeping a permanent `fix_schema_drift` file when create could be edited |
| `vm fresh --force` on disposable DBs after editing create | New migration files for WIP schema exploration |
| One `add_` when drift advisories require backfill | Using `add_` as the default reaction to “add a column” |
| Fold related alters into **one** migration if `add_` is truly required | One migration file per tiny ALTER |

### Anti-patterns (do not do these)

- User: “add email to users” → agent immediately runs `vm make migration add_email_to_users` without checking for an existing `create_users_table.sql`.
- Creating a new migration to hold ALTER statements that drift would apply with **yes**.
- Leaving the create migration stale (missing columns) while stacking `add_` files — drift compares against migration SQL; the create file should stay complete when you control history.
- Treating `generate` (fix_schema_drift) as the normal path — it is an escape hatch, not the default.

### Short examples

**Dev — add `email` to users (preferred):**
```bash
# 1. Edit migrations/<ts>_create_users_table.sql — add email column in CREATE TABLE (+ Down if needed)
# 2. Accept new checksums + sync live schema:
vm migrate --force --detect-drift
# → yes
# Or if the DB is disposable:
vm fresh --force
```

**Only when drift refuses (NOT NULL, no default) — `add_` is necessary:**
```bash
vm make migration add_email_to_users
# Edit: ADD COLUMN … NULL → backfill → SET NOT NULL (or ADD with DEFAULT)
vm migrate
```

---

## 2. .vm Config File — Setup and Variables

The `.vm` file is **the only file an LLM should create** (when the user needs one). Everything else is generated by CLI commands.

Create it with `echo` or by writing a `.vm` file at the project root. Walk through:

### Minimal config (PostgreSQL — default)
```ini
DATABASE_URL=postgres://user:pass@localhost:5432/mydb
```

### Minimal config (MySQL)
```ini
DATABASE_URL=mysql://user:pass@localhost:3306/mydb
```

### Minimal config (MariaDB — include `mariadb` in host or path)
```ini
DATABASE_URL=mysql://user:pass@mariadb-host:3306/mydb
```

### Development config (recommended for local)
```ini
DATABASE_URL=postgres://user:pass@localhost:5432/mydb
ENVIRONMENT=development
DRIFT_HANDLING=prompt
```

### Production config
```ini
DATABASE_URL=postgres://prod-user:pass@prod-host:5432/mydb
ENVIRONMENT=production
DRIFT_HANDLING=reject
```

### Full config with all variables explained

```ini
# ── Required ──────────────────────────────────────────────────────────────────
# Default / preferred: PostgreSQL
DATABASE_URL=postgres://user:pass@localhost:5432/mydb
# MySQL URL:   mysql://user:pass@localhost:3306/mydb
# MySQL DSN:   user:pass@tcp(localhost:3306)/mydb
# MariaDB:     mysql://… with "mariadb" in host or path (see §0)

# ── Environment (auto-applies defaults listed below) ──────────────────────────
# Values: development | dev | production | prod
ENVIRONMENT=development

# ── Drift handling ────────────────────────────────────────────────────────────
# auto   → apply fixes silently
# prompt → ask before applying (default)
# reject → fail migration if drift detected
DRIFT_HANDLING=prompt

# ── Optional path override ────────────────────────────────────────────────────
MIGRATION_PATH=./migrations          # default: ./migrations

# ── sqlc / goose compatibility ────────────────────────────────────────────────
SQLC_SUPPORT=false                   # adds +goose Up/Down markers

# ── Auto-run before vm migrate (PostgreSQL only; silently skipped on MySQL/MariaDB) ───
AUTO_RUN_EXTENSIONS=true             # run extensions.sql first
AUTO_RUN_FUNCTIONS=true              # run functions.sql first
AUTO_RUN_ENUMS=true                  # run enums.sql first

# ── Manual overrides (override ENVIRONMENT defaults) ─────────────────────────
# ENHANCED=true                      # checksum + locking + drift
# ONLINE=true                        # zero-downtime strategies (PG + MySQL 8+ / MariaDB)
# VERIFY_CHECKSUMS=true              # detect modified migration files
# DETECT_DRIFT=true                  # detect manual schema changes
# VERBOSE=true                       # coloured output with timing
```

### What ENVIRONMENT sets automatically

| Setting          | development | production |
|------------------|-------------|------------|
| `ENHANCED`       | true        | true       |
| `ONLINE`         | false       | true       |
| `VERIFY_CHECKSUMS` | true      | true       |
| `DETECT_DRIFT`   | true        | true       |
| `VERBOSE`        | true        | false      |
| `DRIFT_HANDLING` | prompt      | prompt     |

### Config resolution order (highest priority first)
1. CLI flag (`--dsn`, `--path`, etc.)
2. Environment variable `DATABASE_URL`
3. `.vm` file (current dir or any parent)
4. `.env` file (current dir)
5. Built-in defaults

### Lint the config immediately after creating it
```bash
vm lint
```

---

## 3. Linting the .vm Config

`vm lint` validates the `.vm` file for unknown keys, wrong value types, duplicate keys, and missing required values. Always run it after creating or editing `.vm`.

```bash
vm lint                        # auto-finds .vm in current or parent directories
vm lint --file /path/to/.vm    # explicit path
```

**What it checks:**
- `DATABASE_URL` is present and non-empty
- All keys are in the known set (unknown keys → error with a spelling suggestion)
- Boolean keys (`ENHANCED`, `ONLINE`, `VERIFY_CHECKSUMS`, `DETECT_DRIFT`, `VERBOSE`, `SQLC_SUPPORT`, `AUTO_RUN_*`) have values `true`/`false`/`1`/`0`
- `ENVIRONMENT` / `ENV` is one of: `development`, `dev`, `production`, `prod`
- `DRIFT_HANDLING` is one of: `auto`, `prompt`, `reject`
- No duplicate keys
- No malformed `KEY=VALUE` lines

**Exit codes:** 0 = clean, non-zero = errors found.

**Full known key list:**
`DATABASE_URL`, `MIGRATION_PATH`, `SQLC_SUPPORT`, `ENVIRONMENT`, `ENV`, `ENHANCED`, `ONLINE`, `VERIFY_CHECKSUMS`, `DETECT_DRIFT`, `VERBOSE`, `AUTO_RUN_EXTENSIONS`, `AUTO_RUN_FUNCTIONS`, `AUTO_RUN_ENUMS`, `DRIFT_HANDLING`

---

## 4. Creating Migration Files

Always use `vm make migration <name>` for **new tables** (and for `add_` only when [§1.1](#11-schema-change-decision-tree--avoid-migration-sprawl) says it is necessary). Never write `.sql` files directly.

```bash
vm make migration <name> [flags]
```

### Naming rules
- snake_case only (lowercase, digits, underscores)
- `create_` prefix added automatically if missing
- `_table` suffix added automatically if missing
- Names starting with `trigger_` are left exactly as-is
- Names starting with `add_` are treated as alterations (no `CREATE TABLE` generated) — **rare; see §1.1**

### Flags

| Flag | Alias | Effect |
|------|-------|--------|
| `--path <dir>` | `-p` | Override migrations directory |
| `--soft-delete` | `-sd` | Add `deleted_at` column + index |
| `--triggers` | `-t` | Add `updated_at` auto-update trigger scaffold |
| `--belongs-to <table>` | `-bt` | Add FK column (repeatable) |
| `--one-to-one <table>` | `-oto` | Add unique FK column (repeatable) |
| `--many-to-many <table>` | `-mm`, `--pivot` | Generate pivot/junction table |

### Common examples

```bash
# Basic table (normal path)
vm make migration users
# → migrations/<ts>_create_users_table.sql

# With soft delete / triggers — put these on the create migration up front when possible
vm make migration posts --soft-delete --triggers

# Foreign key — posts belongs to users
vm make migration posts --belongs-to users

# FK to two parents
vm make migration posts --belongs-to users --belongs-to categories

# One-to-one
vm make migration user_profiles --one-to-one users

# Pivot / many-to-many (creates users_roles migration)
vm make migration users --many-to-many roles

# add_ — ONLY when §1.1 step 4/5 applies (drift advisory or immutable prod history)
# Prefer editing create_users_table.sql + drift instead of this:
# vm make migration add_email_to_users
```

**Cannot combine:** `--many-to-many` with `--belongs-to` or `--one-to-one`.

**Trigger dependency (PostgreSQL):** `--triggers` embeds a call to `auto_update_timestamp()` (or `auto_update_with_soft_delete_protection()` when combined with `--soft-delete`). Run `vm functions migrate` before running these migrations or set `AUTO_RUN_FUNCTIONS=true` in `.vm`.

**Triggers on MySQL/MariaDB:** `--triggers` scaffolds a single-statement `SET NEW.updated_at = CURRENT_TIMESTAMP` trigger. No `functions.sql` / `vm functions migrate` required. Soft-delete update protection remains PostgreSQL-only.

Dialect for scaffolds is taken from `DATABASE_URL` in `.vm` (defaults to PostgreSQL when unset).

---

## 5. Running Migrations

```bash
vm migrate [flags]
```

### Auto-run order on every `vm migrate` (PostgreSQL only, configurable via .vm)
1. `extensions.sql` — if `AUTO_RUN_EXTENSIONS=true`
2. `functions.sql` — if `AUTO_RUN_FUNCTIONS=true`
3. `enums.sql` — if `AUTO_RUN_ENUMS=true`
4. All pending migration files (in timestamp order)

MySQL/MariaDB: steps 1–3 are silently skipped.

### Flags

| Flag | Alias | Description |
|------|-------|-------------|
| `--dsn <url>` | `-d` | Override database connection |
| `--path <dir>` | `-p` | Override migrations directory |
| `--step N` | `-s` | Run only N pending migrations then stop |
| `--enhanced` | `-e` | Enable all enhanced features |
| `--verify-checksums` | | Detect modified migration files |
| `--detect-drift` | | Detect manual schema changes |
| `--online` | | Zero-downtime strategies (PostgreSQL + MySQL 8+ / MariaDB) |
| `--dry-run` | | Preview SQL without executing |
| `--force` | | Override checksum mismatches after editing already-executed migration files (see [§5.1](#51-vm-migrate---force--checksum-override)) |
| `--verbose` | `-v` | Coloured output with timing |

### Common invocations

```bash
vm migrate                                        # standard run, reads .vm
vm migrate --dry-run                              # preview without touching DB
vm migrate --step 3                               # run next 3 pending only
vm migrate --enhanced                             # all safety features
vm migrate --verify-checksums --detect-drift      # targeted safety checks
vm migrate --enhanced --online                    # production zero-downtime
# After editing an already-executed create_* file (dev workflow):
vm migrate --force --detect-drift                 # accept new checksums + sync live schema
```

### Enhanced mode behaviour
- Acquires a distributed lock (advisory lock on PG, named lock on MySQL, table lock as fallback)
- Verifies SHA-256 checksums of all already-executed migrations
- Detects and optionally repairs schema drift
- Shows per-migration timing

---

## 5.1 `vm migrate --force` — checksum override

Checksums record the content of each migration **at the moment it first ran**. If you later edit an already-executed file (the normal §1.1 path: change `create_*_table.sql`), `VERIFY_CHECKSUMS` / enhanced mode will fail with:

```text
checksum verification failed (use --force to override)
```

`--force` tells `vm` that the edit was intentional.

### What `--force` does

1. Continues past checksum mismatches (without `--force`, migrate **stops**).
2. **Re-hashes** the modified executed files and writes the new checksums into the `migrations` table so the next run is clean.
3. Does **not** by itself ALTER the live database — pair with `--detect-drift` (or `DRIFT_HANDLING` / env defaults) to sync columns, or use `vm fresh --force` on disposable DBs.

### When LLMs **should** use `--force`

| Situation | Command |
|-----------|---------|
| Edited `create_*_table.sql` that already ran; need live DB to match | `vm migrate --force --detect-drift` → answer **yes** |
| Edited create; disposable DB; OK to rebuild | Prefer `vm fresh --force` (no checksum dance) |
| Checksum error after a deliberate edit; no schema sync needed | `vm migrate --force` |

### When LLMs must **not** use `--force`

- Checksum mismatch you did **not** intentionally cause (restore the file from git instead).
- Production / shared DBs where executed migration files are **immutable** — do not edit create + force; use a forward `add_` / alter migration (§1.1 step 5).
- As a substitute for fixing real SQL errors, lock contention, or missing `DATABASE_URL`.
- Alone when the goal was “add a column to the live table” — without drift/fresh the DB stays unchanged.

### Canonical agent recipe (edit create → sync DB)

```bash
# 1. Edit migrations/<ts>_create_users_table.sql  (add column to CREATE TABLE + Down)
# 2. Accept new file hash + sync live schema:
vm migrate --force --detect-drift
# → if prompted for drift: yes
# 3. Optional: vm status
```

If `ENVIRONMENT=development` (checksums + drift already on), the same is:

```bash
vm migrate --force --detect-drift
```

`--force` alone is enough only when you only need the stored checksums updated and the live schema already matches the file (or you will `fresh` next).

### Related flags named `--force` (different commands)

| Command | Meaning of `--force` |
|---------|----------------------|
| `vm migrate --force` | Override **checksum** mismatches; update stored hashes |
| `vm fresh --force` / `vm refresh --force` | Skip **confirmation** prompt before rebuild |
| `vm rollback --force` | Skip **confirmation** prompt |
| `vm enums drop --force` | Skip **confirmation** prompt |

Do not confuse migrate’s checksum `--force` with fresh/rollback confirmation `--force`.

---

## 6. Schema Drift Detection

Drift occurs when someone alters the database directly (e.g. `ALTER TABLE`, `DROP COLUMN`) without creating a migration. `vm` detects this by comparing the live schema against what your migration files describe.

### How to detect and fix drift

```bash
vm migrate --detect-drift
```

Or set `DETECT_DRIFT=true` / `ENVIRONMENT=development` in `.vm` — drift detection runs automatically.

### What happens

1. `vm` compares each tracked table's live columns against what the migration SQL says.
2. If extra or missing columns are found, it reports them and proposes fix statements.
3. Choices:
   - `yes` — apply statements immediately, then continue
   - `no` — skip drift repair, continue with pending migrations
   - `generate` — write a `<ts>_fix_schema_drift.sql` migration file instead of applying live

### How drift generates ADD COLUMN statements (missing columns)

| Column declaration in migration | Generated statement |
|---------------------------------|---------------------|
| `col TYPE` (nullable, no default) | `ALTER TABLE t ADD COLUMN IF NOT EXISTS col TYPE;` |
| `col TYPE NOT NULL DEFAULT val` | `ALTER TABLE t ADD COLUMN IF NOT EXISTS col TYPE NOT NULL DEFAULT val;` |
| `col TYPE NOT NULL` (no default) | Advisory comment — **not executed** |
| `col TYPE … UNIQUE` | Advisory comment — **not executed** |

**NOT NULL + DEFAULT** — the full constraint clause is placed in the `ADD COLUMN` statement so the column is created non-nullable immediately and existing rows get the default value. This prevents pgx (and other drivers) from panicking when they scan a column defined as non-nullable in Go but backed by NULL rows in the DB.

**NOT NULL without DEFAULT** — adding such a column to a non-empty table fails at the DB level. Drift emits an advisory comment:
```
-- NOT NULL COLUMN: t.col (type) has no DEFAULT value.
-- Create an add_col_to_t migration file to supply a DEFAULT before enforcing NOT NULL.
```
This is one of the **few** cases where `add_` is appropriate (see §1.1 step 4). Prefer putting a DEFAULT on the column in the create migration when the DB is disposable / still editable via drift.

**UNIQUE columns** — uniqueness cannot be enforced on existing rows that may contain duplicates. Drift emits:
```
-- UNIQUE COLUMN: t.col — cannot safely auto-add to a populated table.
-- Create an add_col_to_t migration file to backfill values and add the UNIQUE constraint manually.
```
Same rule: `add_` only because drift cannot auto-apply. Otherwise keep the column in `create_*_table.sql` and use fresh/drift.

Advisory comments are **never sent to the database driver** — only printed as guidance.

### Drift handling modes (set in `.vm`)

| `DRIFT_HANDLING` | Behaviour |
|-----------------|-----------|
| `prompt` | Interactive — asks before applying (default) |
| `auto` | Silently applies all drift fixes |
| `reject` | Fails migration immediately when drift is found |

### LLM workflow for drift

Follow [§1.1](#11-schema-change-decision-tree--avoid-migration-sprawl) first. Then:

1. **Update the create migration** so `CREATE TABLE` matches the desired schema (columns, indexes, FKs).
2. Run **`vm migrate --force --detect-drift`** when that file was already executed (checksum will mismatch). See [§5.1](#51-vm-migrate---force--checksum-override).
3. **Preferred:** answer **yes** — apply fix statements now; continue. Do not create a new file.
4. **Disposable DB:** `vm fresh --force` after editing create — even cleaner than drift + force.
5. **`add_` only if** drift prints a NOT NULL-without-DEFAULT or UNIQUE advisory (or production history is immutable). Then `vm make migration add_<col>_to_<table>`, backfill, `vm migrate` (no `--force` needed for a new pending file).
6. **Avoid `generate`** unless you need a one-off SQL dump to review — do not keep `fix_schema_drift` files as normal history when you can edit create + `--force --detect-drift` → **yes**.

---

## 7. Rolling Back

```bash
vm rollback [flags]
```

| Flag | Alias | Description |
|------|-------|-------------|
| `--steps N` / `all` | `--step`, `-n` | Batches to roll back (default: 1) |
| `--migration <name>` | `-m` | Roll back one specific migration (substring match) |
| `--enhanced` | `-e` | Confirmation prompts + warnings |
| `--dry-run` | | Preview what would roll back |
| `--force` | | Skip confirmation prompts |
| `--verbose` | `-v` | Detailed logging |

```bash
vm rollback                            # last batch
vm rollback --steps 3                  # last 3 batches
vm rollback --steps all                # everything
vm rollback --migration users          # only the migration whose name contains "users"
vm rollback --migration 1771076648_create_users_table.sql   # exact filename
vm rollback --dry-run                  # preview only
```

**Batch system:** each `vm migrate` run gets its own batch number. `--steps 1` rolls back the entire latest batch. `--steps all` rolls back every executed migration.

---

## 8. Status

```bash
vm status
```

Lists every migration file with:
- Status: Executed / Pending
- Batch number
- Execution timestamp (for executed ones)

---

## 9. Refresh / Fresh (Reset the Schema)

Both commands do the same thing: roll back all migrations (runs all Down sections), then re-run all migrations forward. Used to rebuild the schema from scratch.

```bash
vm refresh              # prompts for confirmation
vm refresh --force      # no prompt

vm fresh                # same as refresh
vm fresh --force
```

---

## 10. Extensions (PostgreSQL only)

Manages `extensions.sql` in the migrations directory. **Never edit or create this file directly — let `vm` generate it.**

### Setup workflow
```bash
vm extensions migrate   # first run: creates extensions.sql template, exits
                        # edit the file: uncomment the extensions you need
vm extensions migrate   # second run: installs all uncommented extensions
```

### Active line format in extensions.sql
```sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";      -- active (uncommented)
-- CREATE EXTENSION IF NOT EXISTS "pgcrypto";    -- inactive (commented out)
```

### Drop extensions
```bash
vm extensions drop              # prompt, then drop all
vm extensions drop --step       # one at a time with y/N per extension
```

### Auto-run
When `AUTO_RUN_EXTENSIONS=true` (default), `vm migrate` calls extension sync automatically before running migration files. No manual invocation needed after initial setup.

If extension / function / enum sync **fails** (e.g. bad password), migrate **stops** and offers `Retry …?` after you fix `.vm` / `DATABASE_URL` — it does not continue to the next step. Decline retry to abort; or set `AUTO_RUN_*=false` to skip that step.

---

## 11. Functions (PostgreSQL only)

Manages `functions.sql` containing four standard PL/pgSQL trigger functions. **Never write these functions manually.**

### Setup workflow
```bash
vm functions migrate    # first run: creates functions.sql with all 4 functions, exits
vm functions migrate    # second run: installs all functions
```

### Functions provided

| Function | Purpose |
|----------|---------|
| `auto_update_timestamp()` | Sets `NEW.updated_at = CURRENT_TIMESTAMP` on UPDATE |
| `protect_soft_deleted()` | Blocks updates on rows where `deleted_at IS NOT NULL` |
| `auto_update_with_soft_delete_protection()` | Combines both (used when `--soft-delete --triggers`) |
| `prevent_hard_delete()` | BEFORE DELETE trigger — raises exception to block hard deletes |

### Drop functions
```bash
vm functions drop              # drops all immediately (no confirmation)
vm functions drop --step       # one at a time with y/N per function
```

### Auto-run
When `AUTO_RUN_FUNCTIONS=true` (default), `vm migrate` calls `vm functions migrate` before running migration files.

### Required before `--triggers` migrations (PostgreSQL only)
If `vm make migration <name> --triggers` was used against PostgreSQL, run `vm functions migrate` once (or rely on `AUTO_RUN_FUNCTIONS=true`) before running `vm migrate`. MySQL/MariaDB triggers do not need `functions.sql`.

---

## 12. Enums (PostgreSQL only)

Manages `enums.sql` containing `CREATE TYPE … AS ENUM` definitions. **Never write enum types into migration files.**

### Setup workflow
```bash
vm enums migrate        # first run: creates enums.sql template with examples, exits
                        # edit the file: uncomment or add your CREATE TYPE blocks
vm enums migrate        # second run: installs all uncommented types (idempotent)
```

### Active line format in enums.sql
```sql
CREATE TYPE user_status AS ENUM ('active', 'inactive', 'banned');
-- CREATE TYPE post_status AS ENUM ('draft', 'published', 'archived');  -- inactive
```

Each type is wrapped automatically in a `DO $$ BEGIN … EXCEPTION WHEN duplicate_object THEN NULL; END $$;` block, so re-runs are safe.

### Subcommands

```bash
vm enums migrate                # create file or install types
vm enums drop                   # prompt, then drop all with CASCADE
vm enums drop --force           # drop all without prompt
vm enums drop --step            # one at a time with y/N
vm enums status                 # compare file vs live database
```

### `vm enums status` output groups
- ✓ Defined in file AND in database (shows current values)
- ✗ In file but NOT in database — run `vm enums migrate`
- ? In database but NOT in file — unknown/manually added type

### Auto-run
When `AUTO_RUN_ENUMS=true` (default), `vm migrate` calls `vm enums migrate` before running migration files.

---

## 13. Migration File Format

Every migration file must have exactly one Up and one Down section.

### Arrow format (default)
```sql
-- ⬆ Up (Run when migrating forward)
CREATE TABLE IF NOT EXISTS users (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(255) NOT NULL,
    created_at  TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- ⬇ Down (Run when rolling back)
DROP TABLE IF EXISTS users CASCADE;
```

### Goose format (when `SQLC_SUPPORT=true`)
```sql
-- +goose Up
-- ⬆ Up (Run when migrating forward)
CREATE TABLE IF NOT EXISTS users ( … );

-- +goose Down
-- ⬇ Down (Run when rolling back)
DROP TABLE IF EXISTS users CASCADE;
```

### Column type conventions

| Purpose | PostgreSQL | MySQL/MariaDB |
|---------|-----------|--------------|
| Auto-increment PK | `BIGSERIAL PRIMARY KEY` | `BIGINT AUTO_INCREMENT PRIMARY KEY` |
| Timestamps | `TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP` | `TIMESTAMP DEFAULT CURRENT_TIMESTAMP` |
| Soft delete | `deleted_at TIMESTAMPTZ DEFAULT NULL` | `deleted_at TIMESTAMP NULL` |
| Drop table | `DROP TABLE IF EXISTS t CASCADE` | `DROP TABLE IF EXISTS t` |

`vm make migration` picks the column set from `DetectDialect(DATABASE_URL)` (PostgreSQL when unset).

### Standard column set — PostgreSQL
```sql
id          BIGSERIAL PRIMARY KEY,
created_at  TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
updated_at  TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
```

### Standard column set — MySQL / MariaDB
```sql
id          BIGINT AUTO_INCREMENT PRIMARY KEY,
created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
```

### With `--soft-delete` adds
PostgreSQL: `deleted_at TIMESTAMPTZ DEFAULT NULL`  
MySQL/MariaDB: `deleted_at TIMESTAMP NULL`  
Plus: `CREATE INDEX IF NOT EXISTS idx_<table>_deleted_at ON <table>(deleted_at);`

### With `--triggers` adds (at end of Up section)

**PostgreSQL:**
```sql
DROP TRIGGER IF EXISTS trigger_<table>_auto_update ON <table>;
CREATE TRIGGER trigger_<table>_auto_update
    BEFORE UPDATE ON <table>
    FOR EACH ROW
    EXECUTE FUNCTION auto_update_timestamp();
```
With `--soft-delete --triggers` uses `auto_update_with_soft_delete_protection()` instead.

**MySQL / MariaDB:**
```sql
DROP TRIGGER IF EXISTS trigger_<table>_auto_update;
CREATE TRIGGER trigger_<table>_auto_update
    BEFORE UPDATE ON <table>
    FOR EACH ROW
    SET NEW.updated_at = CURRENT_TIMESTAMP;
```
(No `functions.sql`. Soft-delete update protection is not scaffolded.)

### Validation rules (`vm migrate` checks these before executing)
- Must have at least one Up section marker (`⬆` or `+goose Up`)
- Must **not** contain `CREATE EXTENSION` — use `extensions.sql` via `vm extensions migrate`
- Must **not** contain `CREATE OR REPLACE FUNCTION` — use `functions.sql` via `vm functions migrate`

---

## 14. Complete .vm Variable Reference

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `DATABASE_URL` | string | — | **Required.** Connection string |
| `MIGRATION_PATH` | string | `./migrations` | Path to migration files |
| `ENVIRONMENT` / `ENV` | enum | — | `development`/`dev`/`production`/`prod`; auto-applies group defaults |
| `ENHANCED` | bool | env-dependent | Enables checksum + locking + drift as a group |
| `ONLINE` | bool | env-dependent | Zero-downtime migration strategies |
| `VERIFY_CHECKSUMS` | bool | env-dependent | Detect modified executed migration files |
| `DETECT_DRIFT` | bool | env-dependent | Detect manual schema changes before running |
| `VERBOSE` | bool | env-dependent | Coloured output with per-migration timing |
| `DRIFT_HANDLING` | enum | `prompt` | `auto` / `prompt` / `reject` |
| `AUTO_RUN_EXTENSIONS` | bool | `true` | Run `extensions.sql` before migrations (PG only) |
| `AUTO_RUN_FUNCTIONS` | bool | `true` | Run `functions.sql` before migrations (PG only) |
| `AUTO_RUN_ENUMS` | bool | `true` | Run `enums.sql` before migrations (PG only) |
| `SQLC_SUPPORT` | bool | `false` | Add `+goose Up/Down` markers for sqlc compatibility |

**Boolean values accepted:** `true`, `false`, `1`, `0` (case-insensitive)

---

## 15. Validation Rules

`vm migrate` validates **all pending files before executing any of them**. It fails if any file:
- Contains `CREATE EXTENSION` → move it to `extensions.sql` and use `vm extensions migrate`
- Contains `CREATE OR REPLACE FUNCTION` → move it to `functions.sql` and use `vm functions migrate`
- Has no `-- ⬆ Up` or `-- +goose Up` marker

---

## 16. Error Reference

| Error message | Cause | Fix |
|---------------|-------|-----|
| `database URL is required` | No `DATABASE_URL` anywhere | Add to `.vm` or use `--dsn` |
| `migration validation failed: CREATE EXTENSION found` | Extension in migration file | Move to `extensions.sql`, run `vm extensions migrate` |
| `migration validation failed: CREATE FUNCTION found` | Function in migration file | Move to `functions.sql`, run `vm functions migrate` |
| `checksum mismatch` / `checksum verification failed (use --force to override)` | Edited an already-executed migration file | **Intentional (dev):** `vm migrate --force --detect-drift` → yes (§5.1). **Accidental:** restore file from git. **Prod immutable history:** do not force — use `add_` instead |
| `another migration is currently running` | Another process holds the lock | Wait, or manually clear `migrations_lock` table |
| `vm extensions is not supported on mysql` / `mariadb` | Using extensions/functions/enums on non-PG | Those commands are PostgreSQL-only; skip them on MySQL/MariaDB |
| `no DOWN section found` | Migration file missing `-- ⬇ Down` | Add a Down section |
| `function auto_update_timestamp() does not exist` | Trigger function not installed | Run `vm functions migrate` |
| Drift advisory: `NOT NULL COLUMN … has no DEFAULT value` | Missing column is NOT NULL without a DEFAULT | **Only then** `vm make migration add_<col>_to_<table>` with DEFAULT/backfill — otherwise edit create + drift (§1.1) |
| Drift advisory: `UNIQUE COLUMN … cannot safely auto-add` | Missing column has a UNIQUE constraint | **Only then** `add_` with backfill — otherwise edit create + drift (§1.1) |
| Lint: `unknown key — did you mean X?` | Typo in `.vm` key | Fix key name; run `vm lint` again |
| Lint: `malformed line — expected KEY=VALUE` | Line without `=` | Fix or comment out the line |

---

## 17. Command Quick-Reference

```bash
# Config
vm lint                                      # validate .vm file
vm lint --file /path/to/.vm                  # explicit path

# Migrations
vm make migration <name>                     # NEW tables (not for every column tweak)
vm make migration <name> --soft-delete       # add deleted_at column (on create)
vm make migration <name> --triggers          # add updated_at trigger (on create)
vm make migration <name> --belongs-to <t>    # add FK (repeatable)
vm make migration <name> --one-to-one <t>    # add unique FK
vm make migration <name> --many-to-many <t>  # pivot table
# add_* only when §1.1 says so — prefer edit create + drift/fresh

vm migrate                                   # run all pending
vm migrate --dry-run                         # preview SQL
vm migrate --detect-drift                    # sync live DB to migration SQL (prefer yes)
vm migrate --force --detect-drift            # after editing executed create_* (checksum + sync)
vm migrate --force                           # checksum override only (see §5.1)
vm migrate --enhanced                        # all safety features
vm migrate --step N                          # run N migrations then stop

vm rollback                                  # last batch
vm rollback --steps N                        # N batches
vm rollback --steps all                      # everything
vm rollback --migration <name>               # one specific migration
vm rollback --dry-run                        # preview only

vm refresh [--force]                         # rollback all + re-run all
vm fresh [--force]                           # same as refresh

vm status                                    # show executed / pending

# PostgreSQL helpers (error on MySQL/MariaDB — do not call)
vm extensions migrate                        # create/install extensions.sql
vm extensions drop [--step]                 # drop extensions
vm functions migrate                         # create/install functions.sql
vm functions drop [--step]                  # drop functions
vm enums migrate                             # create/install enums.sql
vm enums drop [--force] [--step]            # drop enum types
vm enums status                              # compare file vs live DB

# Tool management
vm upgrade                                   # upgrade to latest release
vm uninstall [--yes] [--keep-path]           # remove vm binary
vm --version
```

---

## Typical LLM Workflows

### New project from scratch (PostgreSQL — default)
```bash
# 1. Create .vm config
cat > .vm <<'EOF'
DATABASE_URL=postgres://user:pass@localhost:5432/mydb
ENVIRONMENT=development
DRIFT_HANDLING=prompt
EOF

# 2. Validate config
vm lint

# 3. Set up extensions (uncomment needed ones in the generated file)
vm extensions migrate

# 4. Set up trigger functions
vm functions migrate

# 5. Set up enums (uncomment needed types in the generated file)
vm enums migrate

# 6. Create first migration
vm make migration users

# 7. Edit the generated file, then run
vm migrate
```

### New project from scratch (MySQL or MariaDB)
```bash
# 1. Create .vm — MySQL example (for MariaDB, put "mariadb" in host or DB name)
cat > .vm <<'EOF'
DATABASE_URL=mysql://user:pass@localhost:3306/mydb
ENVIRONMENT=development
DRIFT_HANDLING=prompt
AUTO_RUN_EXTENSIONS=false
AUTO_RUN_FUNCTIONS=false
AUTO_RUN_ENUMS=false
EOF

vm lint

# 2. Scaffold uses MySQL/MariaDB types automatically from DATABASE_URL
vm make migration users
# Optional: --soft-delete --triggers (MySQL trigger, no functions.sql)

vm migrate
```

### Add a related table (PostgreSQL)
```bash
vm make migration posts --belongs-to users --soft-delete --triggers
vm migrate
```

### Add a related table (MySQL/MariaDB)
```bash
vm make migration posts --belongs-to users --soft-delete --triggers
vm migrate
```

### Detect and address schema drift / “add a column”

```bash
# Preferred: edit create_*_table.sql, then:
vm migrate --force --detect-drift
# → yes   (or vm fresh --force on disposable DBs)
# Do NOT default to vm make migration add_…
# --force accepts the edited file’s new checksum; --detect-drift ALTERs the live DB
```

### Production-safe deployment
```ini
# .vm
ENVIRONMENT=production
DRIFT_HANDLING=reject
```
```bash
vm migrate   # auto-enables: enhanced, online, checksums, drift detection
```

### Rollback a bad release
```bash
vm rollback --dry-run    # check what will roll back
vm rollback              # roll back last batch
```

### Rebuild schema in development
```bash
vm fresh --force
```

