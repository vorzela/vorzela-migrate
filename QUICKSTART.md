# Quick Start Guide

Get up and running with Vorzela migrations in 5 minutes!

## 1. Build the Tool

```bash
cd vorzela-migrate
make build
```

Or manually:
```bash
go mod tidy
go build -o vm main.go
```

## 2. Setup Database

Create a `.vm` config file in your project root:

**PostgreSQL:**
```ini
DATABASE_URL=postgres://localhost/myapp
MIGRATION_PATH=./migrations
```

**MySQL/MariaDB:**
```ini
DATABASE_URL=mysql://user:pass@localhost:3306/myapp
MIGRATION_PATH=./migrations
```

Or use environment variable:
```bash
export DATABASE_URL="postgres://localhost/myapp"
```

## 3. Create Your First Migration

**Basic migration:**
```bash
vm make migration users
```

**With soft delete support:**
```bash
vm make migration users --soft-delete
# or short form:
vm make migration users -sd
```

This creates: `migrations/TIMESTAMP_create_users_table.sql`

**Note**: File names are automatically normalized:
- `users` → `create_users_table.sql`
- `add_email` → `add_email_table.sql`

## 4. Edit the Migration

Open `migrations/TIMESTAMP_create_users_table.sql` and add your SQL:

**Without soft delete:**
```sql
-- ⬆ Up (Run when migrating forward)
BEGIN;

CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMIT;

-- ⬇ Down (Run when rolling back)
BEGIN;

DROP TABLE IF EXISTS users CASCADE;

COMMIT;
```

**With soft delete (auto-generated when using -sd flag):**
```sql
-- ⬆ Up (Run when migrating forward)
BEGIN;

CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP DEFAULT NULL
);

-- Create index for soft delete queries
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users(deleted_at);

COMMIT;

-- ⬇ Down (Run when rolling back)
BEGIN;

DROP TABLE IF EXISTS users CASCADE;

COMMIT;
```

## 5. Run the Migration

```bash
vm migrate
```

Or with explicit DSN:
```bash
vm migrate --dsn $DATABASE_URL
```

Output:
```
✓ Migrated: 1707123456_create_users_table.sql
✓ Successfully ran 1 migration(s)
```

## 6. Check Status

```bash
vm status
```

Output:
```
Migration Status
────────────────────────────────────────────────────────────────────────────────
Migration                                | Status
────────────────────────────────────────────────────────────────────────────────
1707123456_create_users_table.sql        | ✓ Batch 1
────────────────────────────────────────────────────────────────────────────────

Summary: 1 executed, 0 pending
```

## 7. Create More Migrations

**Basic migrations:**
```bash
vm make migration add_posts_table
vm make migration add_email_verification_to_users
```

**With soft delete:**
```bash
vm make migration create_products_table --soft-delete
vm make migration create_orders_table -sd
```

## 8. Run All Pending Migrations

```bash
vm migrate
```

## Common Commands

### Create Migrations
```bash
# Basic migration
vm make migration create_table_name

# With soft delete support
vm make migration create_table_name --soft-delete
vm make migration create_table_name -sd
```

### Run Migrations
```bash
vm migrate
vm migrate --dsn $DATABASE_URL
```

### Check Status
```bash
vm status
vm status --dsn $DATABASE_URL
```

### Rollback
```bash
# Last batch
vm rollback

# Last 3 batches
vm rollback --steps 3
```

### Refresh (rollback all + re-run)
```bash
vm refresh
vm refresh --dsn $DATABASE_URL
```

## Make Installation Global

```bash
sudo cp vm /usr/local/bin/
chmod +x /usr/local/bin/vm
# Now you can use 'vm' from anywhere
vm make migration create_users_table
```

## Configuration

**Option 1: .vm config file (recommended for projects)**
```ini
DATABASE_URL=postgres://user:pass@localhost:5432/myapp
MIGRATION_PATH=./migrations
```

**Option 2: Environment variables (recommended for CI/CD)**
```bash
export DATABASE_URL="postgres://user:password@localhost:5432/database"
export MIGRATION_PATH="./migrations"
```

**Option 3: CLI flags (for one-off commands)**
```bash
vm migrate --dsn "postgres://user:pass@localhost:5432/db"
```

## Tips

1. **Always test rollbacks** - Make sure your DOWN section works
2. **Use descriptive names** - `create_users_table` not `update_db`
3. **One concern per migration** - Don't mix multiple changes
4. **Keep transactions** - Use BEGIN/COMMIT for safety
5. **Test locally first** - Try on a dev database before production
6. **Use soft delete** - Add `-sd` flag when creating tables that need soft deletes

## Soft Delete Pattern

When using `--soft-delete` / `-sd`, you get automatic soft delete support:

```sql
-- Get active records only
SELECT * FROM users WHERE deleted_at IS NULL;

-- Soft delete a record
UPDATE users SET deleted_at = CURRENT_TIMESTAMP WHERE id = 1;

-- Restore a soft-deleted record
UPDATE users SET deleted_at = NULL WHERE id = 1;

-- Get only deleted records
SELECT * FROM users WHERE deleted_at IS NOT NULL;
```

## Next Steps

- Read [README.md](README.md) for complete documentation
- Check [QUICK_REFERENCE.md](QUICK_REFERENCE.md) for command reference
- Visit [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for common issues

## Getting Help

```bash
vm --help
vm make --help
vm migrate --help
vm rollback --help
vm refresh --help
vm status --help
```
