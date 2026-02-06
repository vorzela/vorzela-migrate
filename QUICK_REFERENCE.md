# Quick Reference Guide - Vorzela Migration Tool v1.1.0

## 🚀 Quick Start

```bash
# 1. Create migration (with timestamps + templates)
vm make migration create_users_table

# 2. Run migrations
vm migrate

# 3. Check status
vm status

# 4. Rollback if needed
vm rollback              # Last 1 batch
vm rollback --steps=all  # All migrations
```

---

## 📋 All Commands

| Command | Purpose | Example |
|---------|---------|---------|
| `make` | Create new migration | `vm make migration create_table` |
| `migrate` | Run pending migrations | `vm migrate` |
| `rollback` | Rollback migrations | `vm rollback --steps=all` |
| `fresh` | Rollback all + Re-run (safe) | `vm fresh` |
| `refresh` | Rollback all + Re-run | `vm refresh` |
| `status` | Show migration status | `vm status` |

---

## 🔄 Rollback Options

```bash
vm rollback              # Rollback 1 batch (default)
vm rollback --steps=1    # Rollback 1 batch (explicit)
vm rollback --steps=2    # Rollback 2 batches
vm rollback --steps=5    # Rollback 5 batches
vm rollback --steps=all  # Rollback ALL migrations ⭐ NEW
```

---

## 🆕 Fresh Command (Best for Dev)

```bash
# Interactive mode (shows warning + asks for confirmation)
vm fresh

# Force mode (skip confirmation)
vm fresh --force

# Perfect for CI/CD pipelines
vm fresh --force --env=test
```

### Why Use `fresh`?

✅ Safety warnings before destructive operation  
✅ Asks for confirmation (yes/no)  
✅ Colored output with status  
✅ Perfect for development and testing  

---

## 📁 Config Setup (Optional but Recommended)

Create `.vorzela` in project root:

```ini
DATABASE_URL=postgres://localhost:5432/myapp
VORZELA_ENV=dev
MIGRATION_PATH=./migrations
```

Then use without `--dsn` flag:
```bash
vm migrate   # Uses .vorzela
vm fresh     # Uses .vorzela
vm status    # Uses .vorzela
```

---

## 📝 Migration Template (Auto-Generated with Timestamps)

Every migration includes:
- ✅ Timestamp of creation
- ✅ CREATE TABLE template with id, created_at, updated_at
- ✅ DROP TABLE template for rollback

```sql
-- Migration: CREATE_USERS_TABLE
-- Created at: 2026-02-05 23:29:57
-- Environment: dev/server

-- ⬆ Up
BEGIN;
CREATE TABLE IF NOT EXISTS create_users_table (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
COMMIT;

-- ⬇ Down
BEGIN;
DROP TABLE IF EXISTS create_users_table CASCADE;
COMMIT;
```

---

## 🎯 Workflow Examples

### Development Workflow
```bash
vm make migration add_email_column
vim migrations/dev/1707129057_add_email_column.sql
vm migrate
vm status
# Need to redo? 
vm fresh --force
```

### Testing Workflow
```bash
vm make migration create_test_table
vm migrate
vm status
# Test complete, reset database
vm fresh --force
```

### Production Workflow
```bash
# Backup first!
pg_dump myapp > backup.sql

# Check what will run
vm status --env=server

# Run migrations
vm migrate --env=server

# Verify
vm status --env=server
```

---

## ⚠️ Important Notes

1. **Always backup production databases before running `fresh`**
   ```bash
   pg_dump myapp > backup.sql
   ```

2. **Use `fresh` for dev/test, `refresh` for automated CI**
   - `fresh` = interactive with confirmation
   - `refresh` = no prompts, automated

3. **Config file is optional**
   - Can always use `--dsn` flag
   - Or set `DATABASE_URL` env var

4. **Migrations run in order by timestamp**
   - File names like: `1707129057_create_users_table.sql`
   - Always added with Unix timestamp

---

## 📚 More Information

- Read [NEW_FEATURES.md](NEW_FEATURES.md) for detailed feature docs
- Read [.vorzela.example](.vorzela.example) for config examples
- Run `vm <command> --help` for command-specific help

---

## Version Info

- **Version**: 1.1.0
- **Status**: Production Ready
- **Features Added**: 
  - Timestamped templates with CREATE/DROP examples
  - Flexible rollback with `--steps=all`
  - New `fresh` command with confirmation warnings

All backward compatible with existing migrations! 🎉
