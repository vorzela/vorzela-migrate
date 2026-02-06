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
go build -o vc main.go
```

## 2. Setup Database

Ensure PostgreSQL is running:
```bash
# Local development
createdb myapp
export DATABASE_URL="postgres://localhost/myapp"
```

## 3. Create Your First Migration

```bash
vc make migration create_users_table
```

This creates: `migrations/dev/TIMESTAMP_create_users_table.sql`

## 4. Edit the Migration

Open `migrations/dev/TIMESTAMP_create_users_table.sql` and add your SQL:

```sql
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

DROP TABLE IF EXISTS users CASCADE;

COMMIT;
```

## 5. Run the Migration

```bash
vc migrate --dsn $DATABASE_URL
```

Output:
```
✓ Migrated: 1707123456_create_users_table.sql
✓ Successfully ran 1 migration(s)
```

## 6. Check Status

```bash
vc status --dsn $DATABASE_URL
```

Output:
```
🐘 Migration Status [dev]
────────────────────────────────────────────────────────────────────────────────
Migration                                | Status
────────────────────────────────────────────────────────────────────────────────
1707123456_create_users_table.sql        | ✓ Batch 1
────────────────────────────────────────────────────────────────────────────────

Summary: 1 executed, 0 pending
```

## 7. Create More Migrations

```bash
vc make migration add_posts_table
vc make migration add_email_verification_to_users
```

## 8. Run All Pending Migrations

```bash
vc migrate --dsn $DATABASE_URL
```

## Common Commands

### Create Migrations
```bash
# Dev environment
vc make migration create_table_name

# Server environment
vc make migration create_table_name -e server
```

### Run Migrations
```bash
vc migrate --dsn $DATABASE_URL
vc migrate --dsn $DATABASE_URL --env server
```

### Check Status
```bash
vc status --dsn $DATABASE_URL
```

### Rollback
```bash
# Last batch
vc rollback --dsn $DATABASE_URL

# Last 3 batches
vc rollback --dsn $DATABASE_URL --steps 3
```

### Refresh (rollback all + re-run)
```bash
vc refresh --dsn $DATABASE_URL
```

## Make Installation Global

```bash
make install
# Now you can use 'vc' from anywhere
vc make migration create_users_table
```

## Environment Variables

```bash
# Required for migrate/rollback/status commands
export DATABASE_URL="postgres://user:password@localhost:5432/database"

# Optional (defaults to 'dev')
export VORZELA_ENV=server
```

## Tips

1. **Always test rollbacks** - Make sure your DOWN section works
2. **Use descriptive names** - `create_users_table` not `update_db`
3. **One concern per migration** - Don't mix multiple changes
4. **Keep transactions** - Use BEGIN/COMMIT for safety
5. **Test locally first** - Try on a dev database before production

## Next Steps

- Read [CONFIG.md](CONFIG.md) for advanced configuration
- Check [README.md](README.md) for complete documentation
- Look at example migrations in `migrations/dev/`
- Set up CI/CD integration for automatic migrations

## Getting Help

```bash
vc --help
vc make --help
vc migrate --help
vc rollback --help
vc refresh --help
vc status --help
```
