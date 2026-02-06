# Soft Delete Quick Start

## One-Line Summary
Add `--soft-delete` flag (or `-sd`) when creating migrations to include a `deleted_at` column for data recovery and audit trails.

## Quick Examples

### Create Table with Soft Delete
```bash
vm make --soft-delete migration create_users_table
```

Generated SQL:
```sql
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP DEFAULT NULL
);
```

### Create Table Without Soft Delete (Normal)
```bash
vm make migration create_posts_table
```

### Using Short Alias
```bash
vm make -sd migration create_comments_table
```

## Common Soft Delete Patterns

### Mark as Deleted
```sql
UPDATE users SET deleted_at = CURRENT_TIMESTAMP WHERE id = 1;
```

### Restore (Un-delete)
```sql
UPDATE users SET deleted_at = NULL WHERE id = 1;
```

### Query Active Records Only
```sql
SELECT * FROM users WHERE deleted_at IS NULL;
```

### Query Deleted Records
```sql
SELECT * FROM users WHERE deleted_at IS NOT NULL;
```

## Error Handling

If a migration fails:
1. Error details are shown (reason, what to fix)
2. Transaction automatically rolls back
3. Migration is NOT recorded in database
4. Next `vm migrate` automatically retries

```bash
# Migration fails with SQL error
vm migrate
# ❌ MIGRATION FAILED: ...
# Fix the SQL, then run again:
vm migrate
# ✓ MIGRATED (automatically retried)
```

## When to Use Soft Delete

✅ Use for:
- User accounts (never fully delete)
- Financial records (audit trail)
- Content (accidental deletion recovery)
- Any data that needs historical tracking

❌ Avoid for:
- Large tables with millions of rows (query performance)
- Temporary data
- Data with strict retention policies
- Where deletion must be unrecoverable

## For More Information

See [SOFT_DELETE_AND_ERROR_HANDLING.md](SOFT_DELETE_AND_ERROR_HANDLING.md)
