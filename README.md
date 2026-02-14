# Vorzela Migration Tool (v1.1.3)


## ✨ Features

- 🎨 **Colorized Output** - Beautiful, easy-to-read colored terminal output
- ⚙️ **Multiple Configuration Methods** - `.vm` config files, `.env` files, or environment variables
- 🚀 **No DSN Flag Required** - Use config files instead of repeating `--dsn` flag
 - 🐘 **Multi-Database Support** - PostgreSQL and MySQL/MariaDB with automatic detection
- � **Batch Tracking** - Organized rollback with batch numbers
- 🔒 **Transaction Safety** - All-or-nothing migration execution
- ⚠️ **Warning System** - Alerts for missing migration sections
- 🌍 **Global CLI** - Install and use from anywhere
- 📚 **Comprehensive Docs** - Full documentation and examples

## Requirements

- **Go** 1.16+ (uses modern `os` package APIs, no deprecated `io/ioutil`)
- **PostgreSQL** 10+ or **MySQL** 5.7+ or **MariaDB** 10.3+

## Installation

```bash
go mod download
go build -o vm main.go
```

### Installers (Recommended)

**Linux/macOS (bash):**

```bash
curl -fsSL https://raw.githubusercontent.com/vorzela/vorzela-migrate/main/install.sh | bash
```

**Windows (PowerShell):**

```powershell
iex (New-Object Net.WebClient).DownloadString('https://raw.githubusercontent.com/vorzela/vorzela-migrate/main/install.ps1')
```

Note: The installers try to add the install directory to your PATH. On Linux/macOS this updates your shell profile (best effort). On Windows this updates your user PATH. Restart your shell if the `vm` command is not found right away.

For more options and platform notes, see [INSTALL.md](INSTALL.md).

## Supported Databases

- **PostgreSQL** 10+ (via pgx v5)
- **MySQL** 5.7+ (via go-sql-driver/mysql)
- **MariaDB** 10.3+ (via go-sql-driver/mysql)

Database type is automatically detected from the DSN URL.

## Usage

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
- Adds `deleted_at TIMESTAMP DEFAULT NULL` column
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

## Migration File Format

Migrations are SQL files with a special format. The tool automatically extracts the UP and DOWN sections:

```sql
-- Migration: CREATE_USERS_TABLE
-- Created at: 2026-02-05 10:30:45

-- ⬆ Up (Run when migrating forward)
BEGIN;

CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMIT;

-- ⬇ Down (Run when rolling back)
BEGIN;

DROP TABLE IF EXISTS users;

COMMIT;
```

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

## Migration File Format

| Command | Description |
|---------|-------------|
| `make migration <name>` | Create a new migration file |
| `migrate` | Run all pending migrations |
| `rollback [--steps N]` | Rollback last N batches |
| `refresh` | Rollback all and re-run all migrations |
| `status` | Show migration status |

## Flags

- `-d, --dsn` - Database connection string
- `-p, --path` - Path to migrations directory (default: migrations)
- `--steps` - Number of batches to rollback (only for rollback command)

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
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP DEFAULT NULL
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

## Troubleshooting

### "migration file already exists"
The migration was already created. Check the `migrations/` directory.

### "failed to connect to database"
Check your DATABASE_URL or --dsn flag. Ensure PostgreSQL is running.

### "invalid migration name"
Use snake_case with only lowercase letters, numbers, and underscores.

### "No UP section found"
Ensure your migration file has the proper format with `-- ⬆ Up` marker.

## License

MIT
