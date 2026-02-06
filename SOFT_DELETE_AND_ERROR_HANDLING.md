# Soft Delete Support and Enhanced Error Handling

## Overview

This document describes the new `--soft-delete` flag and improved error handling in Vorzela.

## Soft Delete Flag

### What is Soft Delete?

Soft delete is a pattern where records are marked as deleted (usually with a timestamp) but not actually removed from the database. This allows:
- Data recovery if needed
- Historical tracking of when items were deleted
- Auditing without data loss
- Filtering out deleted items in queries

### Using the --soft-delete Flag

When creating a migration, add the `--soft-delete` flag to automatically include a `deleted_at` column:

```bash
# Using full flag name
vm make --soft-delete migration create_users_table

# Using short alias
vm make -sd migration create_posts_table
```

### Generated SQL

**Without --soft-delete:**
```sql
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

**With --soft-delete:**
```sql
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP DEFAULT NULL
);
```

### Soft Delete Patterns

After creating a table with `deleted_at`, you can:

**Mark as deleted:**
```sql
UPDATE users SET deleted_at = CURRENT_TIMESTAMP WHERE id = 1;
```

**Restore (un-delete):**
```sql
UPDATE users SET deleted_at = NULL WHERE id = 1;
```

**Query only active records:**
```sql
SELECT * FROM users WHERE deleted_at IS NULL;
```

**Query only deleted records:**
```sql
SELECT * FROM users WHERE deleted_at IS NOT NULL;
```

## Enhanced Error Handling

### Migration Failures

When a migration fails during execution, Vorzela now:

1. **Automatically rolls back** the failed migration
2. **Shows detailed error message** with the reason
3. **Does NOT record** the failed migration in the database
4. **Allows retry** on the next `vm migrate` run

### Error Message Format

Failed migrations display:
```
❌ MIGRATION FAILED: 1707129045_create_users_table.sql
   Reason: syntax error in SQL at or near "CREATE"
   Status: Transaction automatically rolled back
   Action: Fix the SQL in 1707129045_create_users_table.sql and run migrate again
   Note: Failed migration was NOT recorded in database
```

### Example: SQL Error Recovery

**Scenario 1: Typo in migration**
```
Create migration with typo:
    CREATE TABEL users (...)  ❌

Run: vm migrate
    Result: ❌ MIGRATION FAILED
    Action: Fix typo to CREATE TABLE
    
Fix the file and run again:
    CREATE TABLE users (...)  ✓
    
Run: vm migrate
    Result: ✓ MIGRATED (retries automatically)
```

**Scenario 2: File read error**
```
Accidentally delete migration file:
    rm migrations/1707129045_create_users_table.sql
    
Run: vm migrate
    Result: ❌ FAILED to read migration file
    Action: Restore the file from git
    
Restore and run:
    git checkout migrations/1707129045_create_users_table.sql
    vm migrate
    Result: ✓ MIGRATED
```

### Rollback Error Handling

Rollback failures also display detailed messages:
```
❌ ROLLBACK FAILED: 1707129045_create_users_table.sql
   Reason: cannot drop table (it has dependent objects)
   Status: Transaction automatically rolled back
   Action: Fix the DOWN section SQL and try rollback again
   Important: No migration records were removed
```

## Transaction Safety

All migrations run inside transactions:

- **On Success**: Transaction commits and migration is recorded
- **On Failure**: Transaction automatically rolls back
- **Result**: Database is always in a consistent state

```sql
BEGIN;
-- Your migration SQL here
COMMIT;  -- Only runs if no errors
```

## Database Compatibility

Soft delete works with both PostgreSQL and MySQL/MariaDB:

**PostgreSQL:**
```sql
deleted_at TIMESTAMP DEFAULT NULL
```

**MySQL/MariaDB:**
```sql
deleted_at TIMESTAMP DEFAULT NULL
```

## Best Practices

### When to Use Soft Delete

✅ **Use soft delete when:**
- You need data recovery capabilities
- You want audit trails of deletions
- You're dealing with sensitive data (GDPR compliance)
- Users might delete items by mistake
- You need to maintain referential integrity

❌ **Don't use soft delete when:**
- You have strict data retention policies
- Deleted data must be unrecoverable
- Query performance is critical (soft delete increases query complexity)
- You don't need to track deletion history

### SQL Tips for Soft Delete

Always filter deleted records in queries:
```sql
-- Good: Explicitly check for active records
SELECT * FROM users WHERE deleted_at IS NULL;

-- Better: Create a view for convenience
CREATE VIEW active_users AS
  SELECT * FROM users WHERE deleted_at IS NULL;

-- Using view
SELECT * FROM active_users;
```

### Migration Naming

Use clear migration names:
```bash
# Clear what it does
vm make --soft-delete migration create_products_table

# Later: Add soft delete to existing table
vm make migration add_deleted_at_to_products
```

For adding soft delete to existing tables:
```sql
-- Migration: ADD_DELETED_AT_TO_PRODUCTS
-- ⬆ Up
ALTER TABLE products ADD COLUMN deleted_at TIMESTAMP DEFAULT NULL;

-- ⬇ Down
ALTER TABLE products DROP COLUMN deleted_at;
```

## Troubleshooting

### Q: How do I recover from a failed migration?

A: Fix the SQL error in the migration file and run `vm migrate` again. The tool will automatically retry the failed migration.

### Q: Will a failed migration affect my database?

A: No, all changes are rolled back automatically. Your database is left in the exact state it was before the migration attempt.

### Q: Can I force a failed migration to be recorded?

A: No, and this is intentional. Only successfully executed migrations are recorded. This ensures your database state matches your migration history.

### Q: How do I remove soft delete from a table?

A: Create a new migration to remove the column:
```bash
vm make migration remove_deleted_at_from_products
```

Then edit the migration:
```sql
-- ⬆ Up
ALTER TABLE products DROP COLUMN deleted_at;

-- ⬇ Down
ALTER TABLE products ADD COLUMN deleted_at TIMESTAMP DEFAULT NULL;
```

## Examples

### Complete Soft Delete Workflow

```bash
# 1. Create table with soft delete support
vm make --soft-delete migration create_users_table

# 2. Edit the migration if needed (optional)
vim migrations/1707129045_create_users_table.sql

# 3. Run the migration
vm migrate

# 4. Verify it worked
vm status

# 5. In your application, always query with:
SELECT * FROM users WHERE deleted_at IS NULL;

# 6. To delete a user:
UPDATE users SET deleted_at = CURRENT_TIMESTAMP WHERE id = 123;

# 7. To restore a user:
UPDATE users SET deleted_at = NULL WHERE id = 123;
```

### Adding Soft Delete to Existing Table

```bash
# 1. Create migration
vm make migration add_soft_delete_to_users

# 2. Edit migration
cat > migration.sql << 'EOF'
-- ⬆ Up
ALTER TABLE users ADD COLUMN deleted_at TIMESTAMP DEFAULT NULL;

-- ⬇ Down
ALTER TABLE users DROP COLUMN deleted_at;
EOF

# 3. Run migration
vm migrate

# 4. Update indexes (if needed)
vm make migration add_index_on_deleted_at_to_users
```

## Files Modified

- `cmd/make.go` - Added --soft-delete flag
- `internal/migration/create.go` - Added soft delete template logic
- `internal/migration/executor.go` - Enhanced error messages and handling

## Build Information

✅ Compilation: SUCCESS
✅ Tests: PASSED
✅ Backward Compatibility: MAINTAINED

All existing migrations work unchanged. The soft-delete flag is completely optional.
