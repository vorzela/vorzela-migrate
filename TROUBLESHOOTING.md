# Troubleshooting Guide

## Common Issues and Solutions

### Build Issues

#### "go: command not found"
**Problem**: Go is not installed on your system.

**Solution**:
```bash
# Install Go 1.21+ from https://golang.org/dl
# Verify installation
go version
```

#### "package not found"
**Problem**: Missing dependencies

**Solution**:
```bash
go mod tidy
go build -o vm main.go
```

#### "cannot find module"
**Problem**: Go modules not initialized

**Solution**:
```bash
go mod download
go mod verify
go build -o vm main.go
```

---

### Connection Issues

#### "failed to connect to database"
**Problem**: Cannot establish connection to PostgreSQL

**Solutions**:
```bash
# 1. Check PostgreSQL is running
pg_isready -h localhost -p 5432

# 2. Verify connection string format
# postgres://user:password@host:port/database
echo $DATABASE_URL

# 3. Test connection manually
psql $DATABASE_URL -c "SELECT 1"

# 4. Check credentials
psql -U postgres -d postgres

# 5. Ensure user has correct permissions
psql -U postgres
> CREATE ROLE myuser WITH LOGIN PASSWORD 'password';
> GRANT ALL PRIVILEGES ON DATABASE myapp TO myuser;
> GRANT CONNECT ON DATABASE myapp TO myuser;
```

#### "unable to parse DSN"
**Problem**: Invalid connection string format

**Solution**: Use correct PostgreSQL URL format:
```bash
# Basic
postgres://localhost/myapp

# With user and password
postgres://user:password@localhost/myapp

# With port
postgres://user:password@localhost:5432/myapp

# With SSL
postgres://user:password@localhost:5432/myapp?sslmode=require
```

#### "permission denied for database"
**Problem**: User doesn't have required permissions

**Solution**:
```bash
# Connect as superuser
psql -U postgres

# Grant permissions
GRANT CONNECT ON DATABASE myapp TO myuser;
GRANT CREATE ON DATABASE myapp TO myuser;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO myuser;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO myuser;
```

---

### Migration Issues

#### "migration file already exists"
**Problem**: Tried to create a migration that already exists

**Solution**: Check the migrations directory:
```bash
ls migrations/dev/
ls migrations/server/
```

Use a different name if needed:
```bash
vm make migration create_users_table_v2
```

#### "invalid migration name"
**Problem**: Migration name contains invalid characters

**Valid names**:
- Lowercase letters: `a-z`
- Numbers: `0-9`
- Underscores: `_`

**Solutions**:
```bash
✓ vm make migration create_users_table
✓ vm make migration add_email_column
✗ vm make migration Create Users Table  # Spaces not allowed
✗ vm make migration create-users-table  # Dashes not allowed
✗ vm make migration createUsersTable    # Uppercase not allowed
```

#### "No UP section found in migration"
**Problem**: Migration file missing UP section

**Solution**: Ensure migration file has proper format:
```sql
-- ⬆ Up (Run when migrating forward)
BEGIN;
-- Your SQL here
COMMIT;

-- ⬇ Down (Run when rolling back)
BEGIN;
-- Your SQL here
COMMIT;
```

#### "No DOWN section found in migration"
**Problem**: Rollback cannot work without DOWN section

**Solution**: Add DOWN section to migration file:
```sql
-- ⬇ Down (Run when rolling back)
BEGIN;
DROP TABLE IF EXISTS users CASCADE;
COMMIT;
```

#### "migrations table does not exist"
**Problem**: The migrations tracking table hasn't been created yet

**Solution**: Run your first migration to initialize the migrations table:
```bash
vm migrate
```

The migrations table is automatically created the first time you run `vm migrate`.

#### "failed to execute migration"
**Problem**: SQL syntax error or constraint violation

**Solution**:
1. Check SQL syntax
2. Verify table/column doesn't already exist
3. Check foreign key constraints
4. Test SQL manually:
   ```bash
   psql $DATABASE_URL -f migrations/dev/timestamp_migration.sql
   ```

---

### Rollback Issues

#### "No migrations to rollback"
**Problem**: All migrations already rolled back

**Solution**: Check status first:
```bash
vm status --dsn $DATABASE_URL
```

#### "failed to rollback migration"
**Problem**: DOWN section SQL error

**Solutions**:
1. Fix the SQL in the migration file
2. Manually execute the DOWN SQL:
   ```bash
   psql $DATABASE_URL
   # Run the DOWN SQL from the migration file
   ```
3. Remove the migration record:
   ```bash
   psql $DATABASE_URL
   > DELETE FROM migrations WHERE migration = 'timestamp_name.sql';
   ```

#### "Rollback partially failed"
**Problem**: Some migrations rolled back, others failed

**Solution**:
1. Check which migrations were rolled back:
   ```bash
   psql $DATABASE_URL -c "SELECT * FROM migrations;"
   ```
2. Check database schema:
   ```bash
   psql $DATABASE_URL -c "\dt"
   ```
3. Manually fix remaining issues
4. Update migrations table as needed:
   ```bash
   psql $DATABASE_URL -c "DELETE FROM migrations WHERE id = 5;"
   ```

---

### Status Command Issues

#### "Migration Status [dev] header looks wrong"
**Problem**: Output formatting issue

**Solution**: This is just cosmetic. The data is correct. Ensure terminal width is sufficient.

#### "Cannot see pending migrations"
**Problem**: Migrations not showing in status

**Solution**:
1. Verify migration files exist:
   ```bash
   ls migrations/dev/
   ```
2. Check file naming:
   - Must have timestamp prefix: `1234567890_name.sql`
   - Must have .sql extension
3. Verify permissions on migration directory:
   ```bash
   ls -la migrations/dev/
   chmod 755 migrations/dev/
   ```

---

### Environment Issues

#### "invalid environment"
**Problem**: Using invalid environment flag

**Solution**: Only use 'dev' or 'server':
```bash
vm migrate --env dev        # ✓ Correct
vm migrate --env server     # ✓ Correct
vm migrate --env production # ✗ Invalid
```

#### "Wrong environment migrations running"
**Problem**: Migrations from wrong environment executed

**Solution**: Specify correct environment:
```bash
# Check current
vm status --dsn $DATABASE_URL

# Run correct environment
vm migrate --dsn $DATABASE_URL --env dev
vm migrate --dsn $DATABASE_URL --env server
```

---

### Global Installation Issues

#### "vm command not found after install"
**Problem**: Binary not in PATH after installation

**Solution**:
```bash
# Verify binary exists
ls -la /usr/local/bin/vm

# Add to PATH (if needed)
export PATH="/usr/local/bin:$PATH"

# Make permanent by adding to ~/.bashrc or ~/.zshrc
echo 'export PATH="/usr/local/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
```

#### "Permission denied when running vm"
**Problem**: Executable doesn't have run permission

**Solution**:
```bash
chmod +x /usr/local/bin/vm
# Or via make
make install
```

---

### Performance Issues

#### "Migrations running very slowly"
**Problem**: Slow SQL execution or large migrations

**Solutions**:
1. Break into smaller migrations:
   ```
   ✗ One migration creating 10 tables
   ✓ Each table in separate migration
   ```
2. Check database performance:
   ```bash
   psql $DATABASE_URL -c "SELECT query, mean_exec_time FROM pg_stat_statements;"
   ```
3. Add indexes strategically:
   ```sql
   CREATE INDEX idx_column ON table(column);
   ```

#### "Database locks during migration"
**Problem**: Long-running migrations lock tables

**Solutions**:
1. Use `CONCURRENTLY` for indexes:
   ```sql
   CREATE INDEX CONCURRENTLY idx_name ON table(column);
   ```
2. Break large migrations into smaller ones
3. Run during low-traffic periods
4. Use `--min-idle` to control connection pooling:
   ```bash
   # (In code if using as library)
   config.MinConns = 1
   ```

---

### Data Loss / Disaster Recovery

#### "Accidentally rolled back all data"
**Problem**: Used `refresh` unexpectedly

**Solution**:
1. Stop using the database
2. Restore from backup:
   ```bash
   pg_restore -d myapp backup.sql
   ```
3. Check migrations status:
   ```bash
   vm status --dsn $DATABASE_URL
   ```

#### "Migration table corrupted"
**Problem**: Migrations table has inconsistent data

**Solution**:
1. Backup current state:
   ```bash
   pg_dump -d myapp > backup.sql
   ```
2. Rebuild migrations table:
   ```bash
   psql $DATABASE_URL
   > DROP TABLE migrations CASCADE;
   ```
3. Re-run tool (it will recreate table)
4. Verify status

---

### Advanced Debugging

#### Enable verbose output
Create a simple debug wrapper:
```bash
#!/bin/bash
# debug-vm.sh
set -x  # Enable command tracing
vm "$@"
```

Run with:
```bash
./debug-vm.sh migrate --dsn $DATABASE_URL
```

#### Check PostgreSQL logs
```bash
# Ubuntu/Debian
tail -f /var/log/postgresql/postgresql.log

# macOS
tail -f /usr/local/var/log/postgres.log
```

#### Monitor database queries in real-time
```bash
psql $DATABASE_URL
> SELECT pid, query, state FROM pg_stat_activity;
```

---

### Getting Help

1. **Check existing issues**: https://github.com/vorzela/vorzela-migrate/issues
2. **Read documentation**:
   - README.md - Overview
   - QUICKSTART.md - Getting started
   - CONFIG.md - Configuration
   - ARCHITECTURE.md - Technical details
3. **Test commands**: Use `--help` for each command
4. **Create detailed issue** with:
   - Command that failed
   - Full error message
   - Migration file content
   - PostgreSQL version
   - Go version

---

### Still Stuck?

Try these diagnostic steps:

```bash
# 1. Check Go version
go version

# 2. Check database connection
psql $DATABASE_URL -c "SELECT version();"

# 3. Check migrations table
psql $DATABASE_URL -c "SELECT * FROM migrations;"

# 4. List migration files
find migrations -name "*.sql"

# 5. Test manual SQL
psql $DATABASE_URL -f migrations/dev/your_migration.sql

# 6. Check file permissions
ls -la migrations/
ls -la ./vm
```

Include output of these when reporting issues.
