# Configuration Guide

## Environment Variables

### Required
- `DATABASE_URL` - PostgreSQL connection string for running migrations
  ```bash
  export DATABASE_URL="postgres://user:password@localhost:5432/myapp"
  # Or with more options:
  export DATABASE_URL="postgres://user:password@localhost:5432/myapp?sslmode=require&connect_timeout=5"
  ```

- `VORZELA_ENV` - Environment name (dev or server)
  ```bash
  export VORZELA_ENV=dev
  ```

## Connection String Format

### Local Development
```
postgres://localhost/myapp
postgres://localhost:5432/myapp
postgres://user:password@localhost:5432/myapp
```

### With Options
```
postgres://user:password@localhost:5432/myapp?sslmode=disable&connect_timeout=10
```

### Production (SSL)
```
postgres://user:password@prod.example.com:5432/myapp?sslmode=require&sslcert=/path/to/cert
```

## Migration Naming Conventions

Use descriptive, snake_case names following Laravel conventions:

### Create Tables
```
create_users_table
create_posts_table
create_comments_table
```

### Add Columns
```
add_email_to_users
add_status_to_posts
add_tags_to_articles
```

### Remove Columns
```
remove_password_from_users
drop_old_data_column
```

### Create Indexes
```
create_indexes_on_users
add_compound_index_on_orders
```

### Modify Columns
```
change_users_email_type
modify_posts_content_nullable
```

## Best Practices

### 1. One Concern Per Migration
```bash
✓ Good: vm make migration create_users_table
✓ Good: vm make migration add_email_verification_to_users
✗ Bad:  vm make migration create_users_and_posts_tables
```

### 2. Always Use Transactions
```sql
-- ⬆ Up
BEGIN;
CREATE TABLE users (...);
COMMIT;

-- ⬇ Down
BEGIN;
DROP TABLE users;
COMMIT;
```

### 3. Make Rollbacks Reversible
Bad:
```sql
-- ⬇ Down
BEGIN;
-- We lost the data, hope you had backups!
DROP TABLE users;
COMMIT;
```

Better:
```sql
-- ⬇ Down
BEGIN;
ALTER TABLE users DROP COLUMN email;
COMMIT;
```

### 4. Use Specific Column Definitions
```sql
✓ Good:
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

✗ Bad:
CREATE TABLE users (id INT, email TEXT, created_at TIMESTAMP);
```

### 5. Add Indexes for Foreign Keys
```sql
CREATE TABLE posts (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id),
    ...
);

CREATE INDEX idx_posts_user_id ON posts(user_id);
```

### 6. Use CASCADE for Foreign Key Constraints
```sql
-- Automatically deletes posts when user is deleted
CREATE TABLE posts (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    ...
);
```

### 7. Document Complex Migrations
```sql
-- Migration: PARTITION_LARGE_EVENTS_TABLE
-- Purpose: Improve query performance on events table
-- Expected downtime: 5-10 minutes
-- Tested on staging: Yes

-- ⬆ Up
...
```

## Testing Migrations Locally

```bash
# Create test database
createdb myapp_test

# Run migrations
export DATABASE_URL="postgres://localhost/myapp_test"
vm migrate --env dev

# Check status
vm status --env dev

# Rollback for testing
vm rollback --env dev

# Refresh for complete test
vm refresh --env dev

# Cleanup
dropdb myapp_test
```

## Common Issues & Solutions

### Connection Issues
```bash
# Test connection
psql $DATABASE_URL -c "SELECT 1"

# Check credentials
# Ensure user has CREATE, ALTER, DROP permissions
psql -U postgres
> GRANT ALL PRIVILEGES ON DATABASE myapp TO myuser;
```

### Missing Migrations Table
The tool automatically creates this on first run:
```sql
CREATE TABLE migrations (
    id SERIAL PRIMARY KEY,
    migration VARCHAR(255) NOT NULL UNIQUE,
    batch INTEGER NOT NULL,
    environment VARCHAR(50) NOT NULL,
    executed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

### Rollback Issues
If a rollback fails:
1. Check the migration DOWN section
2. Fix the SQL manually if needed
3. Remove the record:
   ```sql
   DELETE FROM migrations WHERE migration = 'filename';
   ```

### Different Migrations Per Environment
Create separate migration files for dev and server:
```bash
vm make migration setup_test_data -e dev
vm make migration add_audit_table -e server
```

## Workflow Example

```bash
# Start development
export DATABASE_URL="postgres://localhost/myapp"
vm make migration create_users_table

# Edit migrations/dev/1234567890_create_users_table.sql
# Add your table schema

# Run migrations
vm migrate --env dev

# Check status
vm status --env dev

# Create next migration
vm make migration add_posts_table

# Test rollback
vm rollback --env dev

# Refresh everything
vm refresh --env dev

# Deploy to production
export DATABASE_URL="postgres://prod-host/myapp"
vm migrate --env server
```

## CI/CD Integration

### GitHub Actions Example
```yaml
name: Migrations

on: [push]

jobs:
  migrate:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:15
        env:
          POSTGRES_PASSWORD: postgres
          POSTGRES_DB: myapp_test
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
        ports:
          - 5432:5432

    steps:
      - uses: actions/checkout@v2
      - uses: actions/setup-go@v2
        with:
          go-version: 1.21

      - name: Build
        run: go build -o vm main.go

      - name: Run migrations
        env:
          DATABASE_URL: postgres://postgres:postgres@localhost:5432/myapp_test
        run: vm migrate --env dev
```
