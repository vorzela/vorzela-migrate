# Vorzela Migration Tool — LLM Reference (v2.0.7)

This document is a complete, structured reference for the `vm` CLI tool intended for use by LLMs and AI coding agents. It covers every command, every flag, all configuration options, and the expected behaviour in each scenario.

---

## Overview

`vm` is a database migration CLI tool written in Go. It manages SQL migration files for PostgreSQL, MySQL, and MariaDB. It is invoked as `vm <command> [flags]`.

- Binary name: `vm`
- Config file: `.vm` (in project root or any parent directory)
- Default migrations directory: `./migrations`
- Migration files are plain `.sql` files prefixed with a Unix timestamp: `1771076648_create_users_table.sql`

---

## Configuration

### Config Resolution Order (highest → lowest priority)

1. CLI flags (`--dsn`, `--path`, etc.)
2. Environment variable `DATABASE_URL`
3. `.vm` file in current directory or any parent
4. `.env` file in current directory
5. Built-in defaults

### `.vm` File Reference

All keys are optional. Values are `true`/`false` or `1`/`0` for booleans.

```ini
# Required
DATABASE_URL=postgres://user:pass@localhost:5432/mydb

# Optional
MIGRATION_PATH=./migrations        # default: ./migrations
ENVIRONMENT=development            # development | production  (default: development)
SQLC_SUPPORT=false                 # add +goose markers for sqlc compatibility

# Enhanced features (auto-enabled based on ENVIRONMENT if not set)
ENHANCED=true                      # enable checksum + locking + drift
ONLINE=false                       # zero-downtime strategies (PG + MySQL 8+)
VERIFY_CHECKSUMS=true              # detect modified migration files
DETECT_DRIFT=true                  # detect manual schema changes
VERBOSE=true                       # coloured output with timing
DRIFT_HANDLING=prompt              # auto | reject | prompt

# Auto-run before vm migrate (PostgreSQL only; silently skipped on MySQL/MariaDB)
AUTO_RUN_EXTENSIONS=true           # run extensions.sql first
AUTO_RUN_FUNCTIONS=true            # run functions.sql first
AUTO_RUN_ENUMS=true                # run enums.sql first
```

### Environment defaults (applied when ENHANCED is not explicitly set)

| Setting | development | production |
|---------|------------|------------|
| ENHANCED | true | true |
| ONLINE | false | true |
| VERIFY_CHECKSUMS | true | true |
| DETECT_DRIFT | true | true |
| VERBOSE | true | false |
| DRIFT_HANDLING | prompt | prompt |

---

## Migration File Format

Every migration file must contain exactly one Up section and one Down section. Two marker formats are supported:

**Arrow format (default):**
```sql
-- ⬆ Up (Run when migrating forward)
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- ⬇ Down (Run when rolling back)
DROP TABLE IF EXISTS users CASCADE;
```

**Goose format (when SQLC_SUPPORT=true):**
```sql
-- +goose Up
-- ⬆ Up (Run when migrating forward)
CREATE TABLE IF NOT EXISTS users ( ... );

-- +goose Down
-- ⬇ Down (Run when rolling back)
DROP TABLE IF EXISTS users CASCADE;
```

### Column type conventions

- Timestamps: `TIMESTAMPTZ` (PostgreSQL), `TIMESTAMP` (MySQL/MariaDB)
- Auto-increment: `BIGSERIAL` / `SERIAL` (PostgreSQL), `INT AUTO_INCREMENT` (MySQL)
- Primary key: always `id`
- Standard set: `id`, `created_at`, `updated_at`, optional `deleted_at` (soft delete)

---

## All Commands

### `vm make migration <name>`

Creates a new timestamped SQL migration file in the migrations directory.

**Rules for `<name>`:**
- snake_case only (lowercase, digits, underscores)
- `create_` prefix is added automatically if missing
- `_table` suffix is added automatically if missing
- Exception: names starting with `trigger_` are left unchanged

**Flags:**

| Flag | Alias | Description |
|------|-------|-------------|
| `--path <dir>` | `-p` | Override migrations directory |
| `--soft-delete` | `-sd` | Add `deleted_at TIMESTAMPTZ DEFAULT NULL` + index |
| `--triggers` | `-t` | Add `updated_at` auto-update trigger scaffold |
| `--belongs-to <table>` | `-bt` | Add FK column to parent table (one-to-many). Repeatable. |
| `--one-to-one <table>` | `-oto` | Add unique FK column (one-to-one). Repeatable. |
| `--many-to-many <table>` | `-mm`, `--pivot` | Generate a pivot/junction table |

**Examples:**

```bash
# Creates migrations/<ts>_create_users_table.sql
vm make migration users

# Explicit name (kept as-is with _table appended)
vm make migration create_blog_posts_table

# With soft delete + trigger
vm make migration posts --soft-delete --triggers

# FK: posts belongs to users
vm make migration posts --belongs-to users

# FK: posts belongs to users AND categories
vm make migration posts --belongs-to users --belongs-to categories

# One-to-one
vm make migration user_profiles --one-to-one users

# Pivot table (creates users_roles_table migration)
vm make migration users --many-to-many roles
```

**Generated columns for a standard migration:**
```sql
id BIGSERIAL PRIMARY KEY,
created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
```

**With `--soft-delete`** adds:
```sql
deleted_at TIMESTAMPTZ DEFAULT NULL
```
Plus: `CREATE INDEX IF NOT EXISTS idx_<table>_deleted_at ON <table>(deleted_at);`

**With `--triggers`** adds (requires `vm functions migrate` first):
```sql
DROP TRIGGER IF EXISTS trigger_<table>_auto_update ON <table>;
CREATE TRIGGER trigger_<table>_auto_update
    BEFORE UPDATE ON <table>
    FOR EACH ROW
    EXECUTE FUNCTION auto_update_timestamp();
```

**With `--soft-delete --triggers`** uses `auto_update_with_soft_delete_protection()` instead.

**Cannot combine:** `--many-to-many` with `--belongs-to` or `--one-to-one`.

---

### `vm migrate`

Runs all pending (not yet executed) migration files in timestamp order.

**Auto-run order (PostgreSQL only, all default true):**
1. `extensions.sql` (if `AUTO_RUN_EXTENSIONS=true`)
2. `functions.sql` (if `AUTO_RUN_FUNCTIONS=true`)
3. `enums.sql` (if `AUTO_RUN_ENUMS=true`)
4. Migration files

On MySQL/MariaDB the auto-run steps are skipped with an info message.

**Flags:**

| Flag | Alias | Description |
|------|-------|-------------|
| `--dsn <url>` | `-d` | Database connection string |
| `--path <dir>` | `-p` | Migrations directory |
| `--step N` | `-s` | Run only N pending migrations, then stop |
| `--enhanced` | `-e` | Enable all enhanced features |
| `--verify-checksums` | | Detect modified migration files |
| `--detect-drift` | | Detect manual schema changes |
| `--online` | | Zero-downtime strategies (PG + MySQL 8+) |
| `--dry-run` | | Preview SQL without executing |
| `--force` | | Override checksum mismatches |
| `--verbose` | `-v` | Coloured output with timing |

**Examples:**

```bash
vm migrate
vm migrate --step 3
vm migrate --enhanced
vm migrate --verify-checksums --detect-drift
vm migrate --dry-run
vm migrate --force
vm migrate --dsn "postgres://user:pass@localhost/mydb"
vm migrate --path ./db/migrations
```

**Standard mode:** runs all pending, records each in the `migrations` table under a batch number. All statements in a file run in a single transaction — if any fail the whole file rolls back.

**Enhanced mode** (triggered by `--enhanced`, `--verify-checksums`, `--detect-drift`, `--online`, or `--dry-run`):
- Acquires a distributed lock before running
- Verifies checksums of already-run migrations
- Detects and optionally repairs schema drift
- Shows per-migration timing
- `--dry-run` prints the SQL that would execute without touching the DB

---

### `vm rollback`

Rolls back previously executed migrations by reversing their Down sections.

**Flags:**

| Flag | Alias | Description |
|------|-------|-------------|
| `--dsn <url>` | `-d` | Database connection string |
| `--path <dir>` | `-p` | Migrations directory |
| `--steps N` | `--step`, `-n` | Number of batches to rollback (default: 1). Use `all` to rollback everything. |
| `--migration <name>` | `-m` | Rollback exactly one migration by partial case-insensitive name match |
| `--enhanced` | `-e` | Enhanced rollback with warnings and confirmation prompts |
| `--dry-run` | | Preview what would be rolled back |
| `--force` | | Skip confirmation prompts |
| `--verbose` | `-v` | Detailed logging |

**Examples:**

```bash
# Rollback last batch (default)
vm rollback

# Rollback last 3 batches
vm rollback --steps 3

# Rollback everything
vm rollback --steps all

# Rollback a specific migration (finds first match containing "users")
vm rollback --migration users

# Rollback by full filename
vm rollback --migration 1771076648_create_users_table.sql

# Dry run
vm rollback --dry-run

# Enhanced with confirmations
vm rollback --enhanced
```

**Batch system:** every `vm migrate` run increments a batch counter. `--steps 1` rolls back all migrations in the latest batch. `--steps 2` rolls back the last two batches, etc.

**`--migration` flag:** performs a case-insensitive substring search against migration filenames. The first match is rolled back. Error if no match found.

---

### `vm refresh`

Rolls back **all** migrations (running every Down section), then re-runs all migrations forward. Used to rebuild the schema from scratch without dropping tables manually.

**Flags:**

| Flag | Alias | Description |
|------|-------|-------------|
| `--dsn <url>` | `-d` | Database connection string |
| `--path <dir>` | `-p` | Migrations directory |
| `--force` | | Skip confirmation prompt |

**Examples:**

```bash
vm refresh
vm refresh --force
```

- Prompts for confirmation unless `--force` is given
- Shows per-migration "Dropped" label + timing
- Shows total elapsed time

---

### `vm fresh`

Same as `vm refresh` — rolls back all migrations then re-runs all. Identical behaviour, provided as an alias command that some users prefer.

**Flags:**

| Flag | Alias | Description |
|------|-------|-------------|
| `--dsn <url>` | `-d` | Database connection string |
| `--path <dir>` | `-p` | Migrations directory |
| `--force` | | Skip confirmation prompt |

```bash
vm fresh
vm fresh --force
```

---

### `vm status`

Shows which migrations have been executed and which are pending.

**Flags:**

| Flag | Alias | Description |
|------|-------|-------------|
| `--dsn <url>` | `-d` | Database connection string |
| `--path <dir>` | `-p` | Migrations directory |

```bash
vm status
```

Output lists each migration file with its status (Executed / Pending), batch number, and execution timestamp.

---

### `vm extensions` *(PostgreSQL only)*

Manages a `extensions.sql` file that installs PostgreSQL extensions before migrations run.

**How it works:**
- `extensions.sql` lives in the migrations directory
- Active extensions are uncommented `CREATE EXTENSION IF NOT EXISTS <name>;` lines
- Commented lines (`--`) are ignored

#### `vm extensions migrate`

Creates `extensions.sql` template on first run, then installs all enabled extensions.

```bash
vm extensions migrate
vm extensions migrate --dsn "postgres://..."
vm extensions migrate --path ./db/migrations
```

First run (file does not exist): creates the template and exits, asking you to uncomment the extensions you need.

Subsequent runs: executes all uncommented `CREATE EXTENSION` statements.

#### `vm extensions drop`

Drops all extensions listed in `extensions.sql`.

```bash
vm extensions drop              # prompts for confirmation then drops all
vm extensions drop --step       # one at a time with y/N prompt per extension
```

**Flags:** `--dsn`, `--path`, `--step`/`-s`

**Note:** Both subcommands return an error immediately if the DSN points to MySQL or MariaDB.

---

### `vm functions` *(PostgreSQL only)*

Manages a `functions.sql` file containing shared PL/pgSQL trigger functions.

**Included functions:**

| Function | Purpose |
|----------|---------|
| `auto_update_timestamp()` | Sets `NEW.updated_at = CURRENT_TIMESTAMP` on every UPDATE |
| `protect_soft_deleted()` | Blocks updates on soft-deleted rows (allows restoring via `deleted_at = NULL`) |
| `auto_update_with_soft_delete_protection()` | Combines both of the above |
| `prevent_hard_delete()` | BEFORE DELETE trigger — raises exception to block `DELETE` statements |

#### `vm functions migrate`

Creates `functions.sql` template on first run, then installs all functions.

```bash
vm functions migrate
vm functions migrate --dsn "postgres://..."
```

#### `vm functions drop`

Drops all four standard functions.

```bash
vm functions drop              # drops all with no confirmation
vm functions drop --step       # one at a time with y/N prompt per function
```

**Flags:** `--dsn`, `--path`, `--step`/`-s`

**Note:** Both subcommands return an error immediately if the DSN points to MySQL or MariaDB.

---

### `vm enums` *(PostgreSQL only)*

Manages a `enums.sql` file containing PostgreSQL `CREATE TYPE ... AS ENUM` definitions.

**How it works:**
- `enums.sql` lives in the migrations directory
- Active types are uncommented `CREATE TYPE <name> AS ENUM (...)` blocks
- `vm migrate` auto-runs `vm enums migrate` before migrations when `AUTO_RUN_ENUMS=true`
- Each type is installed inside a `DO $$ BEGIN ... EXCEPTION WHEN duplicate_object THEN NULL; END $$;` block so re-runs are idempotent

#### `vm enums migrate`

Creates `enums.sql` template on first run, then installs all enabled enum types.

```bash
vm enums migrate
vm enums migrate --dsn "postgres://..."
vm enums migrate --path ./db/migrations
```

First run: creates `enums.sql` template with common commented-out examples and exits.

Subsequent runs: installs each uncommented `CREATE TYPE` idempotently (duplicate types are silently skipped).

#### `vm enums drop`

Drops all enum types defined in `enums.sql` using `DROP TYPE IF EXISTS ... CASCADE`.

```bash
vm enums drop              # prompts for confirmation, then drops all
vm enums drop --force      # drops all without confirmation
vm enums drop --step       # one at a time with y/N prompt per type
```

**Flags:** `--dsn`, `--path`, `--force`, `--step`/`-s`

#### `vm enums status`

Compares what is defined in `enums.sql` against what actually exists in `pg_type` in the live database.

```bash
vm enums status
```

Output shows three groups:
- ✓ Defined in file AND in database (with current values)
- ✗ Defined in file but NOT in database (needs `vm enums migrate`)
- ? In database but NOT in file (unknown/extra type)

**Note:** All three subcommands return an error immediately if the DSN points to MySQL or MariaDB.

---

### `vm upgrade`

Upgrades the `vm` binary to the latest release from GitHub.

```bash
vm upgrade
```

- Checks the GitHub releases API for a newer version
- Downloads and runs the install script for your platform (`install.sh` on Linux/macOS, `install.ps1` on Windows)
- No flags

---

### `vm uninstall`

Removes the `vm` binary from the system and optionally cleans shell profile PATH entries.

```bash
vm uninstall
vm uninstall --yes               # skip confirmation
vm uninstall --keep-path         # remove binary but leave PATH entries in shell profiles
```

**Flags:**

| Flag | Alias | Description |
|------|-------|-------------|
| `--yes` | `-y` | Skip confirmation prompt |
| `--keep-path` | | Do not clean PATH export lines from shell profile files |

**What it removes:**
- The `vm` binary (found via `os.Executable()` or checked at `~/.local/bin/vm`, `/usr/local/bin/vm`, `/usr/bin/vm`)
- Any `vm.bak.*` backup files in the same directory (left by `vm upgrade`)
- PATH export lines added to `~/.bashrc`, `~/.zshrc`, `~/.profile`, `~/.bash_profile` unless `--keep-path`

---

## PostgreSQL-Only Features Summary

The following commands and auto-run steps are **not supported on MySQL/MariaDB** and will produce a clear error or be silently skipped:

| Feature | On MySQL/MariaDB |
|---------|-----------------|
| `vm extensions migrate/drop` | Error: "not supported on mysql" |
| `vm functions migrate/drop` | Error: "not supported on mysql" |
| `vm enums migrate/drop/status` | Error: "not supported on mysql" |
| Auto-run extensions in `vm migrate` | Silently skipped with info message |
| Auto-run functions in `vm migrate` | Silently skipped with info message |
| Auto-run enums in `vm migrate` | Silently skipped with info message |

---

## Database Connection String Formats

```bash
# PostgreSQL
postgres://user:pass@localhost:5432/dbname
postgresql://user:pass@localhost:5432/dbname

# PostgreSQL with SSL
postgres://user:pass@host:5432/dbname?sslmode=require

# MySQL
mysql://user:pass@tcp(localhost:3306)/dbname

# MySQL (DSN format)
user:pass@tcp(localhost:3306)/dbname

# MariaDB (same as MySQL; detected by "mariadb" in DSN)
mysql://user:pass@tcp(localhost:3306)/dbname?mariadb=true
```

---

## Internal Tables Created by `vm`

`vm migrate` automatically creates these tables on first run:

```sql
-- Migration tracking (PostgreSQL)
CREATE TABLE IF NOT EXISTS migrations (
    id SERIAL PRIMARY KEY,
    migration VARCHAR(255) NOT NULL UNIQUE,
    batch INTEGER NOT NULL,
    checksum VARCHAR(64),
    executed_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    execution_time_ms INTEGER DEFAULT 0
);

-- Migration tracking (MySQL/MariaDB)
CREATE TABLE IF NOT EXISTS migrations (
    id INT AUTO_INCREMENT PRIMARY KEY,
    migration VARCHAR(255) NOT NULL UNIQUE,
    batch INT NOT NULL,
    checksum VARCHAR(64),
    executed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    execution_time_ms INT DEFAULT 0
);

-- Migration lock table (fallback when advisory locks unavailable)
CREATE TABLE IF NOT EXISTS migrations_lock (
    id INTEGER PRIMARY KEY DEFAULT 1,
    locked BOOLEAN NOT NULL DEFAULT FALSE,
    locked_at TIMESTAMPTZ,
    locked_by VARCHAR(255),
    CHECK (id = 1)
);
```

---

## Common Workflows

### New project setup (PostgreSQL)

```bash
# 1. Create .vm config
echo "DATABASE_URL=postgres://user:pass@localhost/mydb" > .vm

# 2. Create first migration
vm make migration users

# 3. Run migrations
vm migrate
```

### Using extensions + functions + enums (PostgreSQL)

```bash
# Creates extensions.sql template → edit it → install
vm extensions migrate

# Creates functions.sql template (auto-created when using --triggers flag too)
vm functions migrate

# Creates enums.sql template → uncomment types → install
vm enums migrate

# vm migrate will auto-run all three in order before running migration files
vm migrate
```

### Development reset

```bash
vm refresh          # rollback all + re-run all (uses Down SQL)
vm fresh            # same as refresh
```

### Rollback scenarios

```bash
vm rollback                            # last batch
vm rollback --steps 3                  # last 3 batches
vm rollback --steps all                # everything
vm rollback --migration users          # only the users migration
vm rollback --dry-run                  # preview only
```

### Production-safe migration

```bash
vm migrate --enhanced --online --verify-checksums --detect-drift
```

Or set in `.vm`:
```ini
ENVIRONMENT=production
```
(Production environment auto-enables all safety flags.)

### Step-by-step migration

```bash
vm migrate --step 1    # run one at a time, check between each
vm migrate --step 1
vm migrate --step 1
```

---

## Migration Validation Rules

`vm migrate` validates all pending migration files before executing any of them. Validation fails if:

- A migration file contains `CREATE EXTENSION` (should be in `extensions.sql`)
- A migration file contains `CREATE OR REPLACE FUNCTION` (should be in `functions.sql`)
- A migration file has no Up section marker (`⬆` or `+goose Up`)

---

## Error Messages and Fixes

| Error | Fix |
|-------|-----|
| `database URL is required` | Set `DATABASE_URL` in `.vm` or use `--dsn` |
| `migration validation failed: CREATE EXTENSION found` | Move extension to `extensions.sql`, run `vm extensions migrate` |
| `migration validation failed: CREATE FUNCTION found` | Move function to `functions.sql`, run `vm functions migrate` |
| `checksum mismatch` | Run `vm migrate --force` or restore the original file |
| `another migration is currently running` | Another process holds the lock; wait or manually clear `migrations_lock` |
| `vm extensions is not supported on mysql` | Extensions are PostgreSQL-only; remove from your MySQL workflow |
| `no DOWN section found` | Add a `-- ⬇ Down` section to the migration file |
| `function auto_update_timestamp() does not exist` | Run `vm functions migrate` first |

---

## File Structure

```
project/
├── .vm                          # config file
├── migrations/
│   ├── extensions.sql           # PostgreSQL extensions
│   ├── functions.sql            # PL/pgSQL trigger functions
│   ├── enums.sql                # PostgreSQL enum types
│   ├── 1771076648_create_users_table.sql
│   ├── 1771076700_create_posts_table.sql
│   └── 1771076800_add_email_to_users.sql
```

---

## Global Flags (all commands)

| Flag | Alias | Description |
|------|-------|-------------|
| `--dsn <url>` | `-d` | Database connection string (overrides config) |
| `--path <dir>` | `-p` | Migrations directory (overrides config) |
| `--version` | | Print version and exit |
| `--help` | `-h` | Show help |
