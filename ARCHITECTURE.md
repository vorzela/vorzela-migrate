# Architecture Guide

## Overview

Vorzela is a Laravel-inspired database migration tool for Go that supports both PostgreSQL and MySQL/MariaDB. It provides a simple CLI interface for creating, running, and rolling back migrations with automatic database detection and batch-based tracking.

## Key Design Principles

1. **Single Migration Folder**: All migrations live in `migrations/` directory (no environment subfolders)
2. **Auto-Detection**: Database type is automatically detected from the DSN
3. **Dialect-Aware SQL**: SQL is generated based on the detected database type
4. **Batch Tracking**: Migrations are grouped into batches for organized rollback
5. **Simplicity**: Minimal configuration, intuitive commands, consistent behavior

## Project Structure

```
vorzela-migrate/
├── main.go                          # CLI entry point
├── go.mod                           # Go module definition  
├── go.sum                           # Dependency lock file
│
├── cmd/                             # CLI Commands
│   ├── make.go                      # Create new migration
│   ├── migrate.go                   # Run pending migrations
│   ├── rollback.go                  # Rollback migrations
│   ├── refresh.go                   # Rollback all + re-run
│   ├── fresh.go                     # Drop tables + migrate
│   └── status.go                    # Show migration status
│
├── internal/                        # Internal packages
│   ├── config/
│   │   └── config.go                # Configuration loading
│   ├── database/
│   │   └── connection.go            # Database connection + auto-detection
│   ├── db/
│   │   ├── db.go                    # DB interface (adapter pattern)
│   │   ├── postgres.go              # PostgreSQL adapter (pgx v5)
│   │   └── mysql.go                 # MySQL adapter (database/sql)
│   └── migration/
│       ├── types.go                 # Data structures (MigrationFile, Migration)
│       ├── create.go                # Migration creation logic
│       ├── relationship.go          # Relationship & foreign key generation
│       ├── executor.go              # Migration execution logic
│       ├── status.go                # Migration status display
│       └── dialect.go               # Dialect-aware SQL generation
│
├── migrations/                      # Migration files (generated)
│   ├── .gitkeep
│   ├── TIMESTAMP_migration1.sql
│   └── TIMESTAMP_migration2.sql
│
├── README.md                        # User documentation
├── INSTALL.md                       # Installation guide (all platforms)
├── NAMING_CONVENTIONS.md            # Naming conventions guide
├── ARCHITECTURE.md                  # This file
├── install.sh                       # Auto-install script (macOS/Linux)
├── install.ps1                      # Auto-install script (Windows)
└── .vm.example                 # Example config file
```

## Core Components

### 1. CLI Layer (main.go + cmd/*)

The CLI layer uses `urfave/cli` v2 for command-line parsing:

**Commands**:
- `make migration <name>` - Creates new migration files with templates
- `migrate` - Runs pending migrations  
- `rollback [--steps N]` - Rolls back last N batches (default: 1)
- `refresh` - Drops all tables and re-runs all migrations
- `fresh --seed` - Drops tables with confirmation warnings
- `status` - Shows current migration status

**Design**:
- No `--env` flag (single migrations folder for all environments)
- DSN from CLI flags, environment variables, or `.vm` config file
- Database type auto-detected from DSN URL scheme
- Batch tracking for organized rollback

### 2. Database Abstraction Layer (internal/db/*)

Implements the adapter pattern for database independence:

**DB Interface** (`db.go`):
```go
type DB interface {
    Exec(ctx context.Context, sql string, args ...interface{}) error
    Query(ctx context.Context, sql string, args ...interface{}) (Rows, error)
    QueryRow(ctx context.Context, sql string, args ...interface{}) Row
    Ping(ctx context.Context) error
    Close() error
}
```

**Adapters**:
- **postgres.go**: Wraps pgx v5 `pgxpool.Pool`
  - Connection pooling (configurable)
  - Automatic retry logic
  - Prepared statement support

- **mysql.go**: Wraps `database/sql` with `go-sql-driver/mysql`
  - Standard library based
  - Connection pooling (configurable)
  - Multi-statement support

**Connection Logic** (`internal/database/connection.go`):
- Auto-detects database type from DSN:
  - `postgres://` or `postgresql://` → PostgreSQL adapter
  - `mysql://` or `user@tcp(` → MySQL adapter
- Returns DB interface (not driver-specific)
- Enables transparent driver switching

### 3. Dialect-Aware SQL Generation (internal/migration/dialect.go)

Generates SQL that's correct for the target database:

**PostgreSQL**:
- Uses `BIGSERIAL` for auto-increment IDs (supports up to ~9 quintillion records)
- Uses `CASCADE` for foreign key drops
- Uses `CURRENT_TIMESTAMP` for timestamps

**MySQL**:
- Uses `AUTO_INCREMENT` for auto-increment IDs  
- Supports `FOREIGN_KEY_CHECKS` pragma
- Compatible with MariaDB

### 4. Migration Logic (internal/migration/*)

**Types** (`types.go`):
```go
type Migration struct {
    ID        int
    Migration string
    Batch     int
    ExecutedAt time.Time
}

type MigrationFile struct {
    Filename  string
    Name      string
    Timestamp int64
}
```

**Creation** (`create.go`):
- Validates migration names (snake_case only)
- Creates `migrations/` directory if needed
- Generates templates with UP/DOWN sections
- Names files with Unix timestamp prefix for ordering
- Supports relationship flags for automatic FK generation

**Relationships** (`relationship.go`):
- **One-to-Many** (`--belongs-to`): Generates `BIGINT NOT NULL` FK columns with indexes
- **One-to-One** (`--one-to-one`): Generates `BIGINT NOT NULL UNIQUE` FK columns
- **Many-to-Many** (`--many-to-many`): Generates pivot tables with composite unique constraints
- Automatic table name singularization (users → user_id)
- Alphabetically sorted pivot table names (role_user, not user_role)
- Foreign key constraints with `ON DELETE CASCADE`
- Performance indexes on all FK columns

**Execution** (`executor.go`):
- Initializes migrations table on first run
- Reads migration files from disk (sorted by timestamp)
- Extracts UP/DOWN SQL sections
- Executes SQL with transaction support
- Tracks executed migrations with batch numbers
- Implements rollback by batch
- Handles both PostgreSQL and MySQL

**Status** (`status.go`):
- Lists all migration files with execution status
- Shows batch information
- Displays summary statistics
- Works with both database types

## Data Flow

### Creating a Migration

```
User Command: vm make migration create_users_table
                        ↓
            Parse command and arguments
                        ↓
        validate_migration_name()
                        ↓
        create_migrations_directory()
                        ↓
        generate_migration_template()
                        ↓
        write_file_to_disk()
                        ↓
        Display success message
```

### Running Migrations

```
User Command: vm migrate --dsn <dsn>
                        ↓
        database.Connect(dsn)
          ↓
          Auto-detect: postgres:// or mysql://
          ↓
          Return appropriate DB adapter
                        ↓
        migration.InitMigrationTable()
                        ↓
        migration.getMigrationFiles()
                        ↓
        migration.getExecutedMigrations()
                        ↓
        For each pending migration:
            - Read file from disk
            - Extract UP section
            - Execute SQL in transaction
            - Record in migrations table
                        ↓
        Display results and summary
```

### Rolling Back Migrations

```
User Command: vm rollback --dsn <dsn> --steps 1
                        ↓
        database.Connect(dsn)
          ↓
          Auto-detect database type
          ↓
          Return appropriate DB adapter
                        ↓
        migration.getExecutedMigrations()
                        ↓
        Sort by batch (descending)
                        ↓
        For last N batches:
            - Read migration file
            - Extract DOWN section
            - Execute SQL in transaction
            - Remove from migrations table
                        ↓
        Display results and summary
```

## Migration File Format

Migration files use a special format with UP and DOWN sections:

```sql
-- Migration: UPPERCASE_NAME
-- Created at: YYYY-MM-DD HH:MM:SS

-- ⬆ Up (Run when migrating forward)
BEGIN;
-- Your migration SQL here
COMMIT;

-- ⬇ Down (Run when rolling back)
BEGIN;
-- Your rollback SQL here
COMMIT;
```

### Optional: sqlc/goose Compatibility

If you use sqlc or other goose-compatible tools, enable goose markers in your `.vm` config:

```
SQLC_SUPPORT=true
```

This will add `-- +goose Up` and `-- +goose Down` markers to all new migrations.

### Section Parsing

The tool extracts UP/DOWN sections using markers:
1. `-- ⬆ Up` marks the start of the UP section
2. `-- ⬇ Down` marks the end of UP and start of DOWN section
3. End of file marks the end of DOWN section

### SQL Execution

- All SQL is wrapped in transactions (BEGIN/COMMIT)
- Errors cause immediate rollback
- Each migration is atomic
- SQL is dialect-aware (PostgreSQL or MySQL)

## Configuration

Vorzela supports three configuration methods with clear priority:

1. **CLI Flags** (highest priority) - `--dsn`, `--path`
2. **Environment Variables** - `DATABASE_URL`, `MIGRATION_PATH`
3. **Config Files** - `.vm` or `.env` (lowest priority)

**DSN URL Schemes**:
- PostgreSQL: `postgres://user:pass@host:port/dbname` or `postgresql://...`
- MySQL: `mysql://user:pass@host:port/dbname` or `user:pass@tcp(host:port)/dbname`

## Single Migration Folder Design

Unlike traditional Laravel, Vorzela uses a **single `migrations/` folder** instead of environment-specific subfolders:

**Why**:
- Simpler folder structure
- Fewer CLI options (no `--env` flag needed)
- Shared migration history across environments
- Batch tracking handles environment separation if needed
- Clearer mental model: one source of truth

**Migration Table Schema**:
```sql
CREATE TABLE migrations (
    id INT AUTO_INCREMENT PRIMARY KEY,
    migration VARCHAR(255) NOT NULL UNIQUE,
    batch INT NOT NULL,
    executed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

Note: No `environment` column needed with single-folder design

## Batch-Based Tracking

Migrations are organized into batches:

- **Batch Number**: Incremented for each `vm migrate` run
- **Batch Grouping**: All migrations in a batch can be rolled back together
- **Flexible Rollback**: `--steps N` rolls back last N batches

Example:
```
Batch 1: create_users_table, add_posts_table (rolled back together)
Batch 2: add_email_column (can rollback independently)
Batch 3: create_comments_table (current)
```

## Multi-Database Support

### Database Detection

DSN is parsed at connection time:
- If DSN contains `postgres://` or `postgresql://` → PostgreSQL adapter
- If DSN contains `mysql://` or `@tcp(` → MySQL adapter
- Defaults to PostgreSQL if ambiguous

### Dialect Handling

SQL generation adapts to database type:

**PostgreSQL** (in `dialect.go`):
```sql
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);
```

**MySQL** (in `dialect.go`):
```sql
CREATE TABLE users (
    id INT AUTO_INCREMENT PRIMARY KEY,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

### Driver Adapters

- **postgres.go**: Implements DB interface using pgx v5
  - High-performance connection pooling
  - Prepared statement support
  - Native PostgreSQL data types

- **mysql.go**: Implements DB interface using database/sql
  - Cross-compatible with MySQL 5.7+ and MariaDB 10.3+
  - Standard library based
  - Wide ecosystem support

## Error Handling

The tool implements comprehensive error handling:

1. **Connection Errors**: Database connection and DSN parsing failures
2. **File Errors**: Migration file read/write failures
3. **SQL Errors**: SQL execution failures with detailed messages
4. **Validation Errors**: Invalid migration names, invalid DSN format
5. **Warnings**: Missing UP/DOWN sections (non-fatal, continues execution)

Example warning:
```
⚠️  Warning: Missing UP section in 1707123456_users.sql
```

## Performance Characteristics

1. **Connection Pooling**: Database connections are reused across queries
2. **Batch Transactions**: All operations in a batch share a single transaction
3. **Indexed Queries**: Queries use indexed `migration` column for lookups
4. **Disk I/O**: Minimal file reads (sorted once, cached in memory)
5. **Concurrency**: Database-level locking prevents concurrent migrations

## Dependencies

### Runtime Dependencies
- **Go**: 1.16+ (uses `os` package, not deprecated `io/ioutil`)
- **PostgreSQL**: 10+ (via github.com/jackc/pgx/v5)
- **MySQL/MariaDB**: 5.7+/10.3+ (via github.com/go-sql-driver/mysql)

### Direct Go Dependencies
- `github.com/jackc/pgx/v5` - PostgreSQL driver with connection pooling
- `github.com/go-sql-driver/mysql` - MySQL/MariaDB driver
- `github.com/urfave/cli/v2` - CLI framework
- `github.com/fatih/color` - Colored terminal output
- `github.com/joho/godotenv` - .env file support

## Extension Points

The tool is designed to be extensible:

1. **New Commands**: Add new CLI commands in `cmd/` directory
2. **New Drivers**: Implement DB interface in `internal/db/` for additional databases
3. **Custom SQL**: Extend `dialect.go` for database-specific SQL generation
4. **Custom Templates**: Modify template generation in `create.go`
5. **Config Providers**: Extend `config.go` to read from other sources

## Security Considerations

1. **SQL Injection**: Uses parameterized queries where applicable
2. **Connection Security**: Supports SSL/TLS via DSN parameters
   - PostgreSQL: Add `?sslmode=require` to DSN
   - MySQL: Add `?tls=true` to DSN
3. **Permissions**: Requires database user with CREATE/ALTER/DROP/SELECT/INSERT/DELETE permissions
4. **Transaction Safety**: Uses transactions to prevent partial/inconsistent state
5. **Error Messages**: Includes detailed error information for debugging (sanitize in production)

## Testing Migrations

For testing migrations in development:

```bash
# Create test database
createdb test_db  # For PostgreSQL
# OR
mysql -e "CREATE DATABASE test_db;"  # For MySQL

# Run migrations
DATABASE_URL="postgres://localhost/test_db" vm migrate
# OR  
DATABASE_URL="mysql://root@localhost/test_db" vm migrate

# Verify schema
psql test_db -c "\dt"  # For PostgreSQL
# OR
mysql test_db -e "SHOW TABLES;"  # For MySQL

# Test rollback
vm rollback
vm status

# Clean up
dropdb test_db  # For PostgreSQL
# OR
mysql -e "DROP DATABASE test_db;"  # For MySQL
```

## Design Decisions

### Why Single Migration Folder?
- **Simpler UX**: No `--env` flag, fewer command options
- **Clearer History**: Single source of truth for all migrations
- **Easier CI/CD**: Same migrations run everywhere (no duplication)
- **Batch Tracking**: Provides environment separation if needed

### Why Adapter Pattern for Databases?
- **Pluggable**: Easy to add PostgreSQL, MySQL, SQLite, etc.
- **Clean**: Each driver is isolated in its own package
- **Type Safe**: Single DB interface, no type assertions needed
- **Testable**: Mock DB interface for unit tests

### Why Batch Tracking?
- **Safe Rollback**: Undo multiple related migrations together
- **Reproducible**: Same batch always rolls back together
- **Flexible**: `--steps N` provides fine-grained control
- **Clear History**: Shows when migrations were applied

## Future Enhancements

Potential improvements for future versions:

1. **Migration Seeding**: Dedicated seed command for data seeding
2. **Dry Run Mode**: Preview migrations without executing
3. **Pre/Post Hooks**: Execute custom code before/after migrations
4. **Schema Validation**: Verify schema matches migration files
5. **Multi-Database**: Support migrations across multiple databases
6. **Migration Scheduling**: Automatic scheduled execution
7. **Audit Trail**: Detailed logging of all operations
8. **Rollback Windows**: Limit how far back you can rollback
9. **SQLite Support**: Add SQLite database support
10. **YAML Migrations**: Alternative to SQL-based migrations

## Contributing

When contributing to Vorzela:

1. **Code Structure**: Follow existing patterns (adapters, interfaces)
2. **Testing**: Add tests for new features
3. **Documentation**: Update relevant docs
4. **Commits**: Use clear, descriptive messages
5. **Database Support**: Test with both PostgreSQL and MySQL when possible
