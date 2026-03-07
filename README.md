# Vorzela Migration Tool (v2.1.5)

## 📖 Table of Contents

- [Features](#-features)
- [Requirements](#requirements)
- [Installation](#installation)
  - [Quick Install Script](#method-1-quick-install-script-recommended)
  - [Pre-built Binary](#method-2-download-pre-built-binary)
  - [Build from Source](#method-3-build-from-source)
- [Supported Databases](#supported-databases)
- [Usage](#usage)
  - [Environment-Based Auto Config](#-super-quick-start-environment-based-auto-config)
  - [Quick Start](#quick-start-no---dsn-needed)
  - [Create a Migration](#create-a-new-migration)
  - [Run Migrations](#run-migrations)
  - [Check Status](#check-migration-status)
  - [Rollback](#rollback-migrations)
  - [Refresh](#refresh-rollback-all-and-re-run)
- [Command Reference](#-command-reference)
- [Enhanced Migration Features](#-enhanced-migration-features)
  - [Checksum Validation](#checksum-validation)
  - [Migration Locking](#migration-locking)
  - [Schema Drift Detection](#schema-drift-detection)
  - [Step-Limited Runs](#step-limited-runs)
  - [Colored Logging with Timing](#colored-logging-with-timing)
  - [Partial Failure Recovery](#partial-failure-recovery)
  - [Safe Rollback with Warnings](#safe-rollback-with-warnings)
  - [Online Migrations](#online-migrations-zero-downtime)
  - [Dry Run Mode](#dry-run-mode)
- [Migration File Format](#migration-file-format)
- [Integration with sqlc & goose](#-integration-with-sqlc--goose)
- [Configuration](#configuration)
- [Soft Delete Patterns](#soft-delete-patterns)
- [PostgreSQL Extensions](#postgresql-extensions)
- [Auto-Update Triggers](#auto-update-triggers)
- [Database Relationships](#database-relationships)
- [Error Handling](#error-handling)
- [Project Structure](#project-structure)
- [Tips & Best Practices](#tips--best-practices)
- [Troubleshooting](#troubleshooting)
- [License](#license)

---

## ✨ Features

- 🎨 **Colorized Output** - Beautiful, easy-to-read colored terminal output
- ⚙️ **Multiple Configuration Methods** - `.vm` config files, `.env` files, or environment variables
- 🚀 **No DSN Flag Required** - Use config files instead of repeating `--dsn` flag
- 🐘 **Multi-Database Support** - PostgreSQL and MySQL/MariaDB with automatic detection
- 📦 **Batch Tracking** - Organized rollback with batch numbers
- 🔒 **Transaction Safety** - All-or-nothing migration execution
- ⚠️ **Warning System** - Alerts for missing migration sections
- 🌍 **Global CLI** - Install and use from anywhere
- � **sqlc & goose Compatible** - Full integration with sqlc (type-safe queries) and goose migrations
- �📚 **Comprehensive Docs** - Full documentation and examples

### 🚀 Enhanced Features (v2.0.2)

- 🌍 **Environment-Based Auto Config** - Set ENVIRONMENT in .vm file, tool auto-configures everything
- ✅ **Checksum Validation** - Detect if migration files have been modified after execution
- 🔐 **Migration Locking** - Prevent concurrent migrations (advisory locks for PostgreSQL, named locks for MySQL)
- 🔍 **Schema Drift Detection** - Automatically detect and fix manual database changes
- 🎨 **Enhanced Logging** - Colored output with execution timing and progress tracking
- 🛡️ **Partial Failure Recovery** - Track which statements succeeded before a failure
- ⚠️ **Safe Rollback** - Confirmation prompts and detailed warnings before rollback
- 🌐 **Online Migrations** - Zero-downtime strategies for production (PostgreSQL & MySQL 8.0+)
- 🤖 **Auto Drift Handling** - Configure auto/prompt/reject modes for schema drift

## Requirements

- **Go** 1.16+ (uses modern `os` package APIs, no deprecated `io/ioutil`)
- **PostgreSQL** 10+ or **MySQL** 5.7+ or **MariaDB** 10.3+

## Installation

### Method 1: Quick Install Script (Recommended)

The easiest way to install is using our installation scripts. They download the latest release, install it globally, and add it to your PATH.

**Linux/macOS:**

```bash
curl -fsSL https://raw.githubusercontent.com/vorzela/vorzela-migrate/main/install.sh | bash
```

**Windows PowerShell (Run as Administrator):**

```powershell
iex (New-Object Net.WebClient).DownloadString('https://raw.githubusercontent.com/vorzela/vorzela-migrate/main/install.ps1')
```

**What the installers do:**
- Download the latest binary for your platform
- Install to `/usr/local/bin` (Linux/macOS) or `%USERPROFILE%\bin` (Windows)
- Add the install directory to your PATH
- Make the binary executable

**Post-installation:**
- **Linux/macOS**: Restart your terminal or run `source ~/.bashrc` (or `~/.zshrc`)
- **Windows**: Restart PowerShell/Command Prompt
- Verify installation: `vm --version`

### Method 2: Download Pre-built Binary

Download the appropriate binary for your platform from the [releases page](https://github.com/vorzela/vorzela-migrate/releases/latest).

**Available platforms:**
- `vm-linux-amd64` - Linux (Intel/AMD 64-bit)
- `vm-linux-arm64` - Linux (ARM 64-bit, Raspberry Pi, etc.)
- `vm-macos-amd64` - macOS (Intel)
- `vm-macos-arm64` - macOS (Apple Silicon M1/M2/M3)
- `vm-windows-amd64.exe` - Windows (64-bit)
- `vm-windows-386.exe` - Windows (32-bit)

**Manual Installation Steps:**

**Linux/macOS:**
```bash
# Download (replace URL with your platform)
wget https://github.com/vorzela/vorzela-migrate/releases/download/v2.0.5/vm-linux-amd64

# Rename and make executable
chmod +x vm-linux-amd64
sudo mv vm-linux-amd64 /usr/local/bin/vm

# Verify
vm --version
```

**Windows:**
```powershell
# Download the .exe file from releases page
# Move to a directory in your PATH, or:

# Create a bin directory
New-Item -ItemType Directory -Force -Path "$env:USERPROFILE\bin"

# Move the downloaded file
Move-Item vm-windows-amd64.exe "$env:USERPROFILE\bin\vm.exe"

# Add to PATH (if not already)
[Environment]::SetEnvironmentVariable("Path", $env:Path + ";$env:USERPROFILE\bin", "User")

# Restart terminal and verify
vm --version
```

### Method 3: Build from Source

If you have Go installed and want the latest development version:

```bash
# Clone the repository
git clone https://github.com/vorzela/vorzela-migrate.git
cd vorzela-migrate

# Download dependencies
go mod download

# Build
go build -o vm main.go

# Install globally (optional)
sudo mv vm /usr/local/bin/  # Linux/macOS
# or move vm.exe to a directory in your PATH on Windows
```

**Build with version info:**
```bash
go build -ldflags "-X 'github.com/vorzela/vorzela-migrate/internal/version.CurrentVersion=v2.0.5'" -o vm main.go
```

### Verify Installation

After installation, verify it works:

```bash
vm --version
# Should output: vorzela-migrate v2.0.5

vm --help
# Shows all available commands
```

### Troubleshooting Installation

**"vm: command not found"**
- The binary is not in your PATH
- Run `echo $PATH` (Linux/macOS) or `echo $env:Path` (Windows) to see your PATH
- Add the installation directory to your PATH, or move the binary to an existing PATH directory

**Permission denied (Linux/macOS)**
- The binary is not executable: `chmod +x vm`
- Or you need sudo: `sudo mv vm /usr/local/bin/`

**Windows: "vm is not recognized"**
- Restart your terminal after installation
- Check if the directory is in your PATH: `echo $env:Path`
- Or run with full path: `C:\Users\YourName\bin\vm.exe`

For more help, see [TROUBLESHOOTING.md](TROUBLESHOOTING.md).

## Supported Databases

- **PostgreSQL** 10+ (via pgx v5)
- **MySQL** 5.7+ (via go-sql-driver/mysql)
- **MariaDB** 10.3+ (via go-sql-driver/mysql)

Database type is automatically detected from the DSN URL.

## Usage

### ⚡ Super Quick Start (Environment-Based Auto Config)

Create `.vm` file with your environment:

**Development:**
```ini
DATABASE_URL=postgres://user:password@localhost:5432/myapp
ENVIRONMENT=development
```

**Production:**
```ini
DATABASE_URL=postgres://prod-db:5432/myapp
ENVIRONMENT=production
DRIFT_HANDLING=auto
```

Then simply run:
```bash
vm migrate  # Auto-applies environment-based settings!
```

**What happens automatically:**
- **Development**: Enhanced mode + verbose logging + drift detection
- **Production**: Enhanced mode + online migrations + checksums + drift detection

No more typing `--enhanced --online --verify-checksums --detect-drift --verbose`!

**Configuration Options:**
- `ENVIRONMENT`: `development` or `production` (auto-applies settings)
- `DRIFT_HANDLING`: `auto` (apply fixes), `prompt` (ask), or `reject` (fail)
- `ENHANCED`, `ONLINE`, `VERIFY_CHECKSUMS`, `DETECT_DRIFT`, `VERBOSE`: Override defaults

See [ARCHITECTURE.md](ARCHITECTURE.md) for implementation details and [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for common issues.

### Quick Start (No --dsn Needed!)

Create `.vm` file:

**PostgreSQL:**
```ini
DATABASE_URL=postgres://user:password@localhost:5432/myapp
```

**MySQL/MariaDB:**
```ini
DATABASE_URL=mysql://user:password@localhost:3306/myapp
```

Then simply:
```bash
vm migrate
vm status
vm rollback
```

### Create a new migration

```bash
# Basic migration (PostgreSQL/MySQL)
vm make migration users        # Creates: create_users_table.sql
vm make migration posts        # Creates: create_posts_table.sql
vm make migration add_email_to_users      # Creates: add_email_to_users_table.sql

# With soft delete support (adds deleted_at column + index)
vm make migration users --soft-delete     # Creates: create_users_table.sql (with deleted_at)
vm make migration posts -sd               # Creates: create_posts_table.sql (with deleted_at)

# With auto-update triggers (automatically updates updated_at timestamp)
vm make migration users --triggers        # Creates: create_users_table.sql (with triggers)
vm make migration posts -t                # Creates: create_posts_table.sql (with triggers)

# Combine both features
vm make migration users --soft-delete --triggers
vm make migration posts -sd -t
```

**Automatic File Naming:**
- **`create_` prefix**: Automatically added if not present (e.g., `users` → `create_users_table.sql`)
- **`_table` suffix**: Automatically added if not present
- **`add_` prefix**: Recognized for alterations (e.g., `add_email_to_users` → `add_email_to_users_table.sql`)
- **Result**: Consistent, descriptive file names without manual typing

**Table Name Extraction (in SQL):**
- `create_users_table` → `CREATE TABLE users` (strips `create_` and `_table`)
- `add_email_to_users_table` → Used as-is (no table creation)

**Soft Delete Support (`--soft-delete` / `-sd`):**
- Adds `deleted_at TIMESTAMPTZ DEFAULT NULL` column
- Creates index on `deleted_at` for efficient queries
- Enables soft delete pattern: `WHERE deleted_at IS NULL`

**Trigger Support (`--triggers` / `-t`):**
- Automatically adds `updated_at` column with trigger function
- Auto-updates `updated_at` timestamp on every row change
- Uses `CREATE OR REPLACE` for idempotency (safe to re-run)
- **With `-sd`**: Prevents updates on soft-deleted records (must restore first)
- Prevents data anomalies from manual timestamp updates

The migration file will be created in the `migrations/` directory with a timestamp prefix.

### Run migrations

**Option 1: Using .vm config file** (Recommended for local development)
```bash
vm migrate
```

**Option 2: Using environment variables** (Recommended for CI/CD)
```bash
export DATABASE_URL="postgres://user:pass@localhost:5432/db"
vm migrate
```

Or for MySQL:
```bash
export DATABASE_URL="mysql://user:pass@localhost:3306/db"
vm migrate
```

**Option 3: Using CLI flags** (For one-off commands)

PostgreSQL:
```bash
vm migrate --dsn "postgres://user:pass@localhost:5432/db"
```

MySQL:
```bash
vm migrate --dsn "mysql://user:pass@localhost:3306/db"
```

```bash
```

### Check migration status

```bash
vm status
```

Or with explicit DSN:
```bash
vm status --dsn "postgres://user:pass@localhost:5432/db"
```

### Rollback migrations

```bash
# Rollback last batch
vm rollback

# Rollback last 3 batches
vm rollback --steps 3
```

### Refresh (rollback all and re-run)

```bash
vm refresh --dsn "postgres://user:pass@localhost:5432/db"
```

## Global Installation

To make `vm` available globally:

```bash
# Build the binary
go build -o vm main.go

# Copy to a location in your PATH
sudo cp vm /usr/local/bin/vm
chmod +x /usr/local/bin/vm

# Now you can use from anywhere
vm make migration create_table_name_table
vm migrate --dsn "postgres://user:pass@localhost:5432/db"
```

## 🚀 Enhanced Migration Features

### Checksum Validation

Detect if migration files have been modified after they were run:

```bash
# Verify all previously run migrations haven't been modified
vm migrate --verify-checksums

# Use --enhanced for automatic checksum verification
vm migrate --enhanced
```

**How it works:**
- SHA-256 checksums are calculated and stored when migrations run
- Verifies file integrity before running new migrations
- Prevents accidental modifications to executed migrations
- Use `--force` to override checksum mismatches

### Migration Locking

Prevent concurrent migrations from running simultaneously:

```bash
# Locking is automatic with enhanced mode
vm migrate --enhanced
```

**Lock mechanisms by database:**
- **PostgreSQL**: Advisory locks (pg_try_advisory_lock)
- **MySQL**: Named locks (GET_LOCK/RELEASE_LOCK)
- **Fallback**: Table-based locking for compatibility

**Benefits:**
- Prevents race conditions in CI/CD environments
- Safe for multiple deployment instances
- 30-second timeout with clear error messages

### Schema Drift Detection

Detect manual database changes not tracked in migrations:

```bash
# Detect and report schema drift
vm migrate --detect-drift

# Interactive mode - offers to generate ALTER statements
vm migrate --enhanced --detect-drift
```

**Example output:**
```
⚠ WARNING  Schema drift detected in table 'users'
  + email_verified_at
  + last_login_at
? Generate migration to document these columns? (y/n)
```

**Capabilities:**
- Compares current schema with migration history using your actual SQL migration files (no false positives)
- Detects manually added columns
- Applies `ALTER TABLE … ADD COLUMN` immediately when user answers **yes** — then continues to pending migrations
- Generates ALTER TABLE statements for review (`generate` option)
- Helps maintain migration consistency

**Drift prompt choices:**
- `yes` — apply the ALTER statements now, then continue with pending migrations
- `no` — skip drift and continue with pending migrations
- `generate` — print the ALTER SQL without executing, then continue

### Step-Limited Runs

Run only a specific number of pending migrations, then stop:

```bash
# Run the next 3 pending migrations only
vm migrate --step 3
vm migrate -s 3

# Run all pending (default behaviour)
vm migrate
```

**How it works:**
- `--step 0` (default) means "run all pending migrations" — fully backward-compatible
- Drift detection is automatically skipped when `--step` would leave remaining pending migrations (avoids false positives mid-run)
- Drift detection runs normally after the step if it exhausts all pending migrations

### Colored Logging with Timing

Get beautiful, informative output with execution times:

```bash
# Enable verbose colored logging
vm migrate --enhanced --verbose
```

**Example output:**
```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
▶ Migration: 1707129045_create_users_table.sql
  Version: 1.1.5
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
[SUCCESS]  15:04:05 Checksum verified
[INFO   ]  15:04:05 Executing statement 1/3
✓ Completed: 1707129045_create_users_table.sql (127ms)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Summary
  Total migrations:     3
  Successful:           3
  Total time:           425ms
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

### Partial Failure Recovery

Track which statements succeeded before a failure:

```bash
vm migrate --enhanced
```

**Automatic tracking:**
- Records each SQL statement execution
- Shows exactly where failure occurred
- Lists successfully applied statements
- Enables targeted troubleshooting

**Example failure output:**
```
✗ Failed: 1707129045_create_posts_table.sql
  Error: statement 3 failed: relation "categories" does not exist
⚠ WARNING  Partially applied statements:
  - CREATE TABLE posts (...)
  - CREATE INDEX idx_posts_user_id ON posts(user_id)
```

### Safe Rollback with Warnings

Get confirmation prompts and detailed information before rollback:

```bash
# Enhanced rollback with warnings
vm rollback --enhanced

# Dry run to see what would be rolled back
vm rollback --dry-run

# Force rollback without prompts (dangerous!)
vm rollback --enhanced --force
```

**Example output:**
```
⚠ WARNING  About to rollback 2 migration(s):
  - 1707129045_create_posts_table.sql (batch 3)
  - 1707129034_create_comments_table.sql (batch 3)
? Continue with rollback? (yes/no)
```

### Online Migrations (Zero Downtime)

Use zero-downtime strategies for production deployments:

```bash
# Enable online migration techniques
vm migrate --online

# Combine with other enhanced features
vm migrate --enhanced --online
```

**Online strategies:**

**PostgreSQL:**
- ADD COLUMN without table locks
- Batch updates for default values
- CREATE INDEX CONCURRENTLY
- NOT NULL constraints after data population

**MySQL 8.0+:**
- ALGORITHM=INSTANT for compatible changes
- ALGORITHM=INPLACE with LOCK=NONE
- Automatic fallback to safe methods

**Example:**
```sql
-- Traditional (locks table):
ALTER TABLE users ADD COLUMN email VARCHAR(255) NOT NULL DEFAULT '';

-- Online (no locks):
-- Step 1: Add nullable column
-- Step 2: Batch update defaults
-- Step 3: Add NOT NULL constraint
```

### Dry Run Mode

Preview migrations without executing:

```bash
# See what would be executed
vm migrate --dry-run

# Preview rollback
vm rollback --dry-run
```

### All Enhanced Features Together

```bash
# Production-ready migration run
vm migrate --enhanced \
  --online \
  --verify-checksums \
  --detect-drift \
  --verbose

# Safe rollback with all checks
vm rollback --enhanced \
  --verbose
```

### Flag Reference

**Migration flags:**
- `--enhanced, -e` - Enable all enhanced features
- `--verify-checksums` - Verify migration file integrity
- `--detect-drift` - Detect manual schema changes
- `--online` - Use zero-downtime strategies
- `--dry-run` - Preview without executing
- `--verbose, -v` - Detailed colored logging
- `--force` - Skip confirmations and override errors

**Rollback flags:**
- `--enhanced, -e` - Enhanced rollback with warnings
- `--steps N` - Rollback N batches (or "all")
- `--dry-run` - Preview rollback
- `--verbose, -v` - Detailed logging
- `--force` - Skip confirmation prompts

## Migration File Format

Migrations are SQL files with a special format. The tool automatically extracts the UP and DOWN sections.

> **Transactions are handled automatically.** Every migration runs inside its own database transaction. If any statement fails the entire migration is rolled back — no partial changes are left in the database. Do not add `BEGIN` / `COMMIT` to your migration files.

```sql
-- Migration: CREATE_USERS_TABLE
-- Created at: 2026-02-05 10:30:45

-- ⬆ Up (Run when migrating forward)
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_users_email ON users(email);

-- ⬇ Down (Run when rolling back)
DROP TABLE IF EXISTS users;
```

## 🔌 Integration with sqlc & goose

Vorzela migrations are fully compatible with [**sqlc**](https://sqlc.dev/) (SQL compiler) and [**goose**](https://github.com/pressly/goose) (database migration tool). When `SQLC_SUPPORT=true` is enabled, all generated migrations include goose-style directional markers.

### Why Use This Integration?

- **sqlc (Primary Use Case)**: Generate type-safe Go code from SQL queries. Requires goose-style migrations as schema source.
- **goose Compatibility**: Goose markers allow sqlc to parse your migrations. You still use `vm migrate` to run them.
- **Best of Both**: Vorzela's advanced features (drift detection, checksums, online migrations) + sqlc's type-safe queries.
- **Optional goose CLI**: Can optionally run migrations with goose instead of Vorzela, but you'll lose Vorzela-specific features.

### Enable sqlc/goose Support

Add `SQLC_SUPPORT=true` to your `.vm` configuration file:

```ini
# .vm file
DATABASE_URL=postgres://user:pass@localhost:5432/mydb
MIGRATION_PATH=./migrations
SQLC_SUPPORT=true  # Enables goose markers
```

Or set it as an environment variable:

```bash
export SQLC_SUPPORT=true
```

### Migration File Comparison

**Without SQLC_SUPPORT (default):**

```sql
-- Migration: CREATE_TABLE_USERS
-- Created at: 2024-01-15 10:30:00

-- ⬆ Up (Run when migrating forward)
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    email VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- ⬇ Down (Run when rolling back)
DROP TABLE IF EXISTS users CASCADE;
```

**With SQLC_SUPPORT=true:**

```sql
-- Migration: CREATE_TABLE_USERS
-- Created at: 2024-01-15 10:30:00

-- +goose Up
-- ⬆ Up (Run when migrating forward)
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    email VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- +goose Down
-- ⬇ Down (Run when rolling back)
DROP TABLE IF EXISTS users CASCADE;
```

### Using with sqlc Workflow

**Step 1:** Enable sqlc support in `.vm`:

```ini
DATABASE_URL=postgres://user:pass@localhost:5432/mydb
MIGRATION_PATH=./migrations
SQLC_SUPPORT=true
```

**Step 2:** Create migrations with Vorzela:

```bash
vm make migration users
vm make migration posts
```

**Step 3:** Configure sqlc (sqlc.yaml):

```yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "./queries"
    schema: "./migrations"  # Point to your Vorzela migrations
    gen:
      go:
        package: "db"
        out: "./internal/db"
        emit_json_tags: true
        emit_prepared_queries: true
        emit_interface: true
```

**Step 4:** Run migrations and generate Go code:

```bash
# Run Vorzela migrations
vm migrate

# Generate type-safe Go code from your queries
sqlc generate
```

**Step 5:** Write queries in `./queries/users.sql`:

```sql
-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: ListUsers :many
SELECT * FROM users ORDER BY created_at DESC;

-- name: CreateUser :one
INSERT INTO users (email) 
VALUES ($1) 
RETURNING *;
```

**Step 6:** Use generated code in Go:

```go
package main

import (
    "context"
    "database/sql"
    "yourapp/internal/db"
)

func main() {
    conn, _ := sql.Open("postgres", "...")
    queries := db.New(conn)
    
    // Type-safe database queries!
    user, err := queries.GetUser(context.Background(), 1)
    users, err := queries.ListUsers(context.Background())
}
```

### Using with goose Directly

**Important:** Goose requires `SQLC_SUPPORT=true` to be enabled in your `.vm` config. The goose markers (`-- +goose Up/Down`) are necessary for goose to parse the migrations.

With `SQLC_SUPPORT=true` enabled, Vorzela migrations can be run with goose:

```bash
# First, ensure SQLC_SUPPORT is enabled in .vm
echo "SQLC_SUPPORT=true" >> .vm

# Install goose
go install github.com/pressly/goose/v3/cmd/goose@latest

# Run migrations with goose (goose will manage its own migrations table)
goose -dir ./migrations postgres "postgres://user:pass@localhost/db" up

# Check status
goose -dir ./migrations postgres "postgres://user:pass@localhost/db" status

# Rollback
goose -dir ./migrations postgres "postgres://user:pass@localhost/db" down
```

**Note:** When using goose to run migrations, goose creates its own `goose_db_version` table. If you want to use Vorzela's migration tracking instead, use `vm migrate` commands. The primary use case for goose compatibility is **sqlc integration**, not as a replacement for Vorzela's migration runner.

### Complete Project Example

```
myproject/
├── .vm                          # Vorzela config
├── sqlc.yaml                    # sqlc config
├── migrations/                  # Vorzela-generated migrations
│   ├── 20240115103000_create_table_users.sql
│   ├── 20240115104000_create_table_posts.sql
│   └── 20240115105000_create_table_comments.sql
├── queries/                     # SQL queries for sqlc
│   ├── users.sql
│   ├── posts.sql
│   └── comments.sql
└── internal/
    └── db/                      # sqlc-generated Go code
        ├── db.go
        ├── models.go
        ├── querier.go
        ├── users.sql.go
        ├── posts.sql.go
        └── comments.sql.go
```

### Workflow Commands

```bash
# 1. Create migration
vm make migration users

# 2. Edit migration file if needed
# (migrations are auto-generated, but you can customize)

# 3. Run migrations
vm migrate

# 4. Generate type-safe Go code
sqlc generate

# 5. Use in your application
go run main.go
```

### Best Practices

1. **Version Control**: Commit both migrations and generated code
2. **CI/CD**: Run `vm migrate` before `sqlc generate` in pipelines
3. **Testing**: Use separate databases for development and testing
4. **Code Generation**: Add `internal/db/*` to `.gitignore` if regenerating in CI
5. **Migration Reviews**: Always review auto-generated migrations before running

### Features That Work Together

All Vorzela features work seamlessly with sqlc/goose markers:

```ini
# .vm - Full-featured configuration
DATABASE_URL=postgres://localhost:5432/mydb
MIGRATION_PATH=./migrations
ENVIRONMENT=production
SQLC_SUPPORT=true              # Enable goose markers
VERIFY_CHECKSUMS=true          # Ensure migration integrity
DETECT_DRIFT=true              # Detect manual schema changes
ONLINE_MIGRATIONS=true         # Zero-downtime migrations
```

This gives you:
- ✅ Type-safe database queries (sqlc)
- ✅ Auto-generated migrations (Vorzela)
- ✅ Goose compatibility markers
- ✅ Checksum validation
- ✅ Schema drift detection
- ✅ Zero-downtime deployments

## Configuration

### Three Ways to Configure

**1. Configuration File (.vm)** - Recommended for local development
```ini
DATABASE_URL=postgres://localhost:5432/myapp
MIGRATION_PATH=./migrations
```

**2. Environment Variables** - Recommended for CI/CD and production
```bash
export DATABASE_URL="postgres://user:pass@localhost:5432/db"
```

**3. CLI Flags** - For one-off commands
```bash
vm migrate --dsn "postgres://user:pass@localhost:5432/db"
```

### Priority (Highest to Lowest)
1. CLI flags (`--dsn`, `--path`)
2. Environment variables (`DATABASE_URL`)
3. `.vm` config file
4. `.env` file
5. Default values

See [CONFIG_ENHANCED.md](CONFIG_ENHANCED.md) for detailed configuration guide.

## 📋 Command Reference

### Core Commands

| Command | Description |
|---------|-------------|
| `vm migrate` | Run all pending migrations |
| `vm migrate --step N` | Run only N pending migrations (0 = unlimited) |
| `vm rollback` | Rollback the last batch of migrations |
| `vm rollback --steps N` | Rollback last N batches (`all` to rollback everything) |
| `vm rollback --migration <name>` | Rollback a single migration by partial name match |
| `vm refresh` | Rollback all migrations then re-run all |
| `vm fresh` | Drop all tables and re-run all migrations (⚠ destructive) |
| `vm status` | Show current migration status (pending / executed) |
| `vm make migration <name>` | Create a new migration file |
| `vm enums migrate` | Install enum types from `enums.sql` |
| `vm enums drop` | Drop enum types defined in `enums.sql` |
| `vm enums status` | Compare `enums.sql` types against live database |
| `vm upgrade` | Upgrade the `vm` binary to the latest release |
| `vm uninstall` | Remove the `vm` binary and clean shell profile |

### `vm make migration` Flags

| Flag | Alias | Description |
|------|-------|-------------|
| `--soft-delete` | `-sd` | Add `deleted_at` column + index for soft deletes |
| `--triggers` | `-t` | Add `updated_at` auto-update trigger |
| `--belongs-to <table>` | `-bt` | Add foreign key column (one-to-many relationship) |
| `--one-to-one <table>` | `-oto` | Add unique foreign key column (one-to-one relationship) |
| `--many-to-many <table>` | `-mm`, `--pivot` | Generate a pivot/junction table |

### `vm migrate` Flags

| Flag | Alias | Description |
|------|-------|-------------|
| `--step N` | `-s N` | Run only N pending migrations, then stop |
| `--enhanced` | `-e` | Enable all enhanced features (checksums, locking, drift) |
| `--verify-checksums` | | Verify that executed migration files haven't been modified |
| `--detect-drift` | | Detect manually added columns not tracked in migrations |
| `--online` | | Use zero-downtime migration strategies (PostgreSQL & MySQL 8+) |
| `--dry-run` | | Preview SQL without executing |
| `--force` | | Override checksum mismatches / skip confirmation prompts |
| `--verbose` | `-v` | Detailed colored logging with execution timing |
| `--dsn <url>` | `-d` | Database connection string (overrides `.vm` / env var) |
| `--path <dir>` | `-p` | Path to migrations directory (default: `migrations`) |

### `vm rollback` Flags

| Flag | Alias | Description |
|------|-------|-------------|
| `--steps N` | `--step`, `-n` | Number of batches to rollback (`all` for everything) |
| `--migration <name>` | `-m` | Rollback a single migration by partial case-insensitive name |
| `--enhanced` | `-e` | Enhanced rollback with warnings and confirmation prompts |
| `--dry-run` | | Preview what would be rolled back |
| `--force` | | Skip confirmation prompts |
| `--verbose` | `-v` | Detailed logging |
| `--dsn <url>` | `-d` | Database connection string |

### `vm extensions` Sub-commands

| Command | Description |
|---------|-------------|
| `vm extensions migrate` | Create `extensions.sql` template (first run) or install extensions |
| `vm extensions drop` | Drop all extensions defined in `extensions.sql` |
| `vm extensions drop --step` | Drop extensions one at a time with confirmation |

### `vm functions` Sub-commands

| Command | Description |
|---------|-------------|
| `vm functions migrate` | Create `functions.sql` template (first run) or install functions |
| `vm functions drop` | Drop all common trigger functions |
| `vm functions drop --step` | Drop functions one at a time with confirmation |

### `vm enums` Sub-commands

| Command | Description |
|---------|-------------|
| `vm enums migrate` | Create `enums.sql` template (first run) or install all enabled `CREATE TYPE` statements idempotently |
| `vm enums drop` | Drop all enum types defined in `enums.sql` |
| `vm enums drop --step` | Drop enum types one at a time with confirmation |
| `vm enums drop --force` | Drop all enum types without confirmation |
| `vm enums status` | Show which enum types are present in the live database vs `enums.sql` |

> **Auto-run:** `vm migrate` automatically runs `vm enums migrate` before applying migrations when `AUTO_RUN_ENUMS=true` (default). Set `AUTO_RUN_ENUMS=false` in your `.vm` file to disable.

### `vm uninstall` Flags

| Flag | Description |
|------|-------------|
| `--yes` | Skip confirmation prompt |
| `--keep-path` | Remove the binary but leave shell profile `PATH` entries untouched |

> Removes the `vm` binary located via `which vm` or known install paths, and cleans any `PATH` export lines added by `vm install.sh` from `~/.bashrc`, `~/.zshrc`, `~/.profile`, and `~/.bash_profile`.

### Global Flags (all commands)

| Flag | Alias | Description |
|------|-------|-------------|
| `--dsn <url>` | `-d` | Database connection string |
| `--path <dir>` | `-p` | Path to migrations directory |
| `--version` | | Print the current version |
| `--help` | `-h` | Show help for a command |

## Examples

### Development workflow

```bash
# Create migrations
vm make migration users
vm make migration posts
vm make migration add_user_indexes

# Run migrations
vm migrate --dsn "postgres://user:pass@localhost:5432/myapp"

# Check status
vm status --dsn "postgres://user:pass@localhost:5432/myapp"

# Rollback if needed
vm rollback --dsn "postgres://user:pass@localhost:5432/myapp"
```

### Production deployment

```bash
# Run migrations on production database
vm migrate --dsn "postgres://user:pass@prod:5432/myapp"

# Check production status
vm status --dsn "postgres://user:pass@prod:5432/myapp"
```

## Soft Delete Patterns

When using `--soft-delete` / `-sd` flag, migrations include `deleted_at` column and index:

```bash
vm make migration users --soft-delete
```

**Generated SQL includes:**
```sql
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ DEFAULT NULL
);

CREATE INDEX idx_users_deleted_at ON users(deleted_at);
```

**Usage in your application:**
```sql
-- Get active records only
SELECT * FROM users WHERE deleted_at IS NULL;

-- Soft delete
UPDATE users SET deleted_at = CURRENT_TIMESTAMP WHERE id = 1;

-- Restore soft-deleted record
UPDATE users SET deleted_at = NULL WHERE id = 1;

-- Get only deleted records
SELECT * FROM users WHERE deleted_at IS NOT NULL;

-- Hard delete (permanent)
DELETE FROM users WHERE id = 1;
```

## PostgreSQL Extensions

Manage PostgreSQL extensions separately from your schema migrations in `migrations/extensions.sql`.

### Why Separate Extensions?

- **Install Before Migrations**: Extensions must be available before creating tables that use them
- **Not Schema Changes**: Extensions are database-level, not schema changes
- **Reusable**: Same extensions file across all environments
- **IF NOT EXISTS**: Safe to re-run without errors

### Quick Start

```bash
# Create extensions.sql template (first time only)
vm extensions migrate

# Edit migrations/extensions.sql and uncomment what you need
# Then apply to database
vm extensions migrate
```

### Common Extensions Included

**UUID Generation (Recommended)**
```sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
```

**Full-Text Search with Trigrams**
```sql
CREATE EXTENSION IF NOT EXISTS pg_trgm;
```

**Case-Insensitive Text**
```sql
CREATE EXTENSION IF NOT EXISTS citext;
```

**Additional Extensions**
- `postgis` - Geographic data
- `pgcrypto` - Cryptographic functions
- `hstore` - Key-value storage
- `unaccent` - Remove accents from text

### Managing Extensions

```bash
# Install all enabled extensions
vm extensions migrate

# Drop all extensions
vm extensions drop

# Drop with confirmation for each
vm extensions drop --step
```

### Best Practices

✅ **DO**: Add extensions to `migrations/extensions.sql`
✅ **DO**: Use `IF NOT EXISTS` (already included in template)
✅ **DO**: Run `vm extensions migrate` before `vm migrate`

❌ **DON'T**: Add extensions in your schema migration files
❌ **DON'T**: Manually run CREATE EXTENSION in psql

## Auto-Update Triggers

The tool provides **centralized trigger functions** in `migrations/functions.sql` to avoid code duplication.

### Step 1: Install Functions (One Time Setup)

```bash
# Install common database functions
vm functions migrate
```

This installs reusable functions:
- `auto_update_timestamp()` - Auto-update `updated_at` on changes
- `protect_soft_deleted()` - Prevent updates on deleted records  
- `auto_update_with_soft_delete_protection()` - Combined functionality
- `prevent_hard_delete()` - Force soft delete only

### Step 2: Create Migrations with Triggers

```bash
# Basic table with auto-update trigger
vm make migration users --triggers

# With soft delete + triggers (uses combined protection function)
vm make migration users -sd -t
```

**Generated migration uses centralized functions:**
```sql
-- Create trigger using centralized function from migrations/functions.sql
-- IMPORTANT: Run 'vm functions migrate' first to install the required functions
DROP TRIGGER IF EXISTS trigger_users_auto_update ON users;
CREATE TRIGGER trigger_users_auto_update
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION auto_update_timestamp();
```

**With soft delete (-sd -t), uses the combined protection function:**
```sql
-- Uses auto_update_with_soft_delete_protection() function
-- Automatically:
--   1. Updates updated_at on every change
--   2. Blocks updates on soft-deleted records
--   3. Allows restoring via deleted_at = NULL
--   4. Prevents data anomalies
CREATE TRIGGER trigger_users_auto_update
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION auto_update_with_soft_delete_protection();
```

**Benefits:**
- **No code duplication**: Functions defined once in `functions.sql`
- **Easy maintenance**: Update functions in one place, affects all tables
- **Idempotent**: Safe to re-run `vm functions migrate` anytime
- **Cleaner migrations**: No inline function definitions
- **Data protection**: Prevents accidental updates on deleted records
- **Automatic restore handling**: Updating `deleted_at` always works  
- **Consistent behavior**: All tables use the same tested logic

**Usage examples:**
```sql
-- Normal update works fine
UPDATE users SET name = 'John' WHERE id = 1;  -- ✓ updated_at auto-updated

-- Soft delete
UPDATE users SET deleted_at = CURRENT_TIMESTAMP WHERE id = 1;  -- ✓ Works

-- Restore soft-deleted record
UPDATE users SET deleted_at = NULL WHERE id = 1;  -- ✓ Works

-- Try to update soft-deleted record
UPDATE users SET name = 'Jane' WHERE id = 1 AND deleted_at IS NOT NULL;
-- ❌ ERROR: Cannot update soft-deleted record. Restore it first by setting deleted_at to NULL.
```

### Adding Custom Functions (Preserved Across Regeneration)

You can add your own trigger functions to `migrations-path/functions.sql` below the **CUSTOM FUNCTIONS** section:

```sql
-- ============================================================================
-- CUSTOM FUNCTIONS (Add your own functions below - these are preserved)
-- ============================================================================

-- Your custom validation function
CREATE OR REPLACE FUNCTION validate_email()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.email !~ '^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$' THEN
        RAISE EXCEPTION 'Invalid email format: %', NEW.email;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Your custom audit logging function
CREATE OR REPLACE FUNCTION log_changes()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO audit_log (table_name, action, changed_at)
    VALUES (TG_TABLE_NAME, TG_OP, CURRENT_TIMESTAMP);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
```

**✓ Custom functions are preserved:**
- When `vm make migration` creates `functions.sql` for the first time, it includes the custom section
- The file is **never overwritten** automatically
- Add as many custom functions as you need
- Re-run `vm functions migrate` to apply your custom functions to the database

**✓ Dropping functions when needed:**
```bash
# Drop all common functions at once
vm functions drop

# Drop functions one by one with confirmation
vm functions drop --step
```

## Database Relationships

Vorzela Migrate provides built-in support for defining relationships between tables with automatic foreign key generation, constraints, and indexes.

### One-to-Many (belongs-to)

Use `--belongs-to` (or `-bt`) when a record belongs to a parent table:

```bash
# Posts belong to users
vm make migration posts --belongs-to users

# Orders belong to users and have a status
vm make migration orders --belongs-to users --belongs-to order_statuses
```

**Generated SQL:**
```sql
CREATE TABLE IF NOT EXISTS posts (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_posts_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_posts_user_id ON posts(user_id);
```

**Features:**
- ✅ Automatically creates `user_id BIGINT NOT NULL` column
- ✅ Adds foreign key constraint with `ON DELETE CASCADE`
- ✅ Creates index on foreign key column for query performance
- ✅ Handles table name singularization (users → user_id, categories → category_id)

### One-to-One (unique)

Use `--one-to-one` (or `-oto`) for unique relationships:

```bash
# Each user has one profile
vm make migration profiles --one-to-one users
```

**Generated SQL:**
```sql
CREATE TABLE IF NOT EXISTS profiles (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL UNIQUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_profiles_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
```

**Difference from belongs-to:**
- Uses `BIGINT NOT NULL UNIQUE` (enforces one-to-one relationship)
- No separate index needed (UNIQUE constraint creates one)

### Many-to-Many (pivot table)

Use `--many-to-many` (or `-mm` / `--pivot`) for junction tables:

```bash
# Users can have many roles, roles can have many users
vm make migration users --many-to-many roles
```

**Generated SQL:**
```sql
CREATE TABLE IF NOT EXISTS role_user (
    id BIGSERIAL PRIMARY KEY,
    role_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_role_user_role FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE,
    CONSTRAINT fk_role_user_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT uq_role_user_role_user UNIQUE (role_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_role_user_role_id ON role_user(role_id);
CREATE INDEX IF NOT EXISTS idx_role_user_user_id ON role_user(user_id);
```

**Features:**
- ✅ Pivot table name alphabetically sorted: `role_user` (not `user_role`)
- ✅ Both foreign keys with CASCADE delete
- ✅ Composite UNIQUE constraint prevents duplicates
- ✅ Indexes on both foreign key columns
- ✅ Optional soft delete support: `--many-to-many roles -sd`

### Combining Relationships with Other Features

```bash
# Posts belong to users + soft delete + auto-update triggers
vm make migration posts --belongs-to users -sd -t

# Orders with multiple relationships + soft delete
vm make migration orders --belongs-to users --belongs-to products -sd
```

**Query examples:**
```sql
-- One-to-Many: Get all posts by a user
SELECT * FROM posts WHERE user_id = 123;

-- One-to-One: Get user's profile
SELECT * FROM profiles WHERE user_id = 123;

-- Many-to-Many: Get all roles for a user
SELECT r.* FROM roles r
JOIN role_user ru ON r.id = ru.role_id
WHERE ru.user_id = 123;

-- Many-to-Many: Get all users with a specific role
SELECT u.* FROM users u
JOIN role_user ru ON u.id = ru.user_id
WHERE ru.role_id = 456;
```

## Error Handling

The tool provides helpful warnings and errors:

- ⚠️ Missing UP section in migration file
- ⚠️ Missing DOWN section in migration file
- ❌ Database connection failures
- ❌ Invalid migration names
- ❌ SQL execution errors with detailed messages

## Project Structure

```
vorzela-migrate/
├── main.go
├── go.mod
├── README.md
├── cmd/
│   ├── make.go
│   ├── migrate.go
│   ├── rollback.go
│   ├── refresh.go
│   ├── fresh.go
│   └── status.go
├── internal/
│   ├── config/
│   │   └── config.go
│   ├── database/
│   │   └── connection.go
│   ├── db/
│   │   ├── db.go
│   │   ├── postgres.go
│   │   └── mysql.go
│   └── migration/
│       ├── types.go
│       ├── create.go
│       ├── executor.go
│       ├── status.go
│       └── dialect.go
└── migrations/
    ├── .gitkeep
    └── 1707123456_create_users_table.sql
```

## Tips & Best Practices

1. **Always use snake_case** for migration names: `users`, `add_email_column`, `create_indexes`
2. **Use transactions** in your migrations (BEGIN/COMMIT) for safety
3. **Test rollbacks** to ensure your DOWN sections work correctly
4. **Separate concerns** - one migration per table/feature
5. **Use descriptive names** that clearly indicate what the migration does
6. **Keep DOWN migrations reversible** - don't lose data unless intentional
7. **BIGSERIAL and BIGINT for scalability** - All generated migrations use `BIGSERIAL` for primary keys and `BIGINT` for foreign keys, supporting up to ~9 quintillion records (vs `SERIAL`/`INTEGER`'s ~2 billion limit). This prevents future migration headaches as your app scales.

## Troubleshooting

### "migration file already exists"
The migration was already created. Check the `migrations/` directory.

### "failed to connect to database"
Check your DATABASE_URL or --dsn flag. Ensure PostgreSQL is running.

### "invalid migration name"
Use snake_case with only lowercase letters, numbers, and underscores.

### "No UP section found"
Ensure your migration file has the proper format with `-- ⬆ Up` marker.

### "migrations table does not exist"
Run your first migration with `vm migrate` to initialize the migrations table.

## License

MIT
