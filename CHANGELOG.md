# Changelog

All notable changes to Vorzela Migration Tool are documented in this file.

## [Unreleased]

### Added
- **PostgreSQL Extensions Management**: New `vm extensions` command for managing database extensions
  - `vm extensions migrate` - Install extensions from extensions.sql
  - `vm extensions drop` - Remove extensions with optional step-by-step confirmation
  - Automatic template creation with common extensions (uuid-ossp, pg_trgm, citext, postgis, etc.)
  - Ensures extensions are separate from schema migrations
  - IF NOT EXISTS pattern for safe re-runs
  - Documentation on best practices for extension management

## [1.1.5] - 2026-02-16

### Added
- **Optional sqlc/goose Compatibility**: New `SQLC_SUPPORT` configuration option in `.vm` file
  - When enabled, includes `-- +goose Up` and `-- +goose Down` markers in migration templates
  - Disabled by default to keep templates clean for users who don't use sqlc
  - Add `SQLC_SUPPORT=true` to your `.vm` config file to enable
  - Works with both regular migrations and pivot table migrations
  
### Improved
- **Better Error Messages**: Enhanced error handling for missing migrations table
  - Clear message: "migrations table does not exist. Please run your first migration with: vm migrate"
  - Helps new users understand initialization process
  - Prevents confusion when running `vm status` before first migration
  - Updated troubleshooting documentation with migration table initialization guidance

### Changed
- **Template Format**: Migration templates no longer include goose markers by default
  - Cleaner templates for the majority of users who don't use sqlc
  - Opt-in via configuration keeps the tool flexible and unobtrusive
  - All documentation updated to reflect configurable goose support

## [1.1.4] - 2026-02-14

### Added
- **Database Relationships**: New relationship flags for automatic foreign key generation
  - `--belongs-to` / `-bt`: Creates one-to-many relationships (e.g., `vm make migration posts --belongs-to users`)
  - `--one-to-one` / `-oto`: Creates one-to-one relationships with UNIQUE constraint
  - `--many-to-many` / `-mm` / `--pivot`: Creates pivot/junction tables with composite unique constraints
  - Automatic table name singularization (users → user_id, categories → category_id)
  - Foreign key constraints with `ON DELETE CASCADE`
  - Performance indexes automatically created on all foreign key columns
  - Alphabetically sorted pivot table names for consistency (role_user, not user_role)
  - Multiple relationships supported: `--belongs-to users --belongs-to categories`
  - Combine with existing features: `--belongs-to users -sd -t`

### Changed
- **BIGSERIAL and BIGINT by default**: All migrations now use `BIGSERIAL` for primary keys and `BIGINT` for foreign keys
  - Previous: `id SERIAL PRIMARY KEY` (max ~2 billion records)
  - New: `id BIGSERIAL PRIMARY KEY` (max ~9 quintillion records)
  - Previous: `user_id INTEGER NOT NULL`
  - New: `user_id BIGINT NOT NULL`
  - Better scalability without future migration headaches
  - All documentation updated to reflect BIGSERIAL/BIGINT usage

### Improved
- **Documentation**: Comprehensive relationship examples added to README.md and QUICK_REFERENCE.md
  - One-to-Many query patterns
  - One-to-One query patterns
  - Many-to-Many join query examples
  - Best practices section updated with BIGSERIAL/BIGINT explanation

## [1.1.3] - 2026-02-14

### Improved
- **Functions Command Structure**: Restructured `vm functions` to use proper subcommands
  - `vm functions migrate` - Apply functions to database (replaces `vm functions`)
  - `vm functions drop` - Remove all common functions from database
  - `vm functions drop --step` - Interactive removal with confirmation for each function
  - Better CLI ergonomics with clear command hierarchy
  - Updated all documentation to reflect new command structure

## [1.1.2] - 2026-02-14

### Added
- **Centralized Trigger Functions**: New `migrations/functions.sql` file with reusable database functions
  - `auto_update_timestamp()` - Automatically updates `updated_at` column on changes
  - `protect_soft_deleted()` - Prevents updates on soft-deleted records  
  - `auto_update_with_soft_delete_protection()` - Combined auto-update + soft delete protection
  - `prevent_hard_delete()` - Forces soft delete only (blocks hard deletes)
  - Eliminates code duplication across migrations
  - Single source of truth for trigger logic
  - **Custom functions preserved**: Add your own functions below CUSTOM FUNCTIONS section
  - File never automatically overwritten (your custom functions are safe)
- **Functions Command**: New `vm functions` command with subcommands to manage database functions
  - `vm functions migrate` - Applies `functions.sql` to database
  - `vm functions drop` - Removes all common functions from database
  - `vm functions drop --step` - Removes functions one by one with confirmation
  - Idempotent: Safe to run multiple times
  - Run `vm functions migrate` once before using `--triggers` flag
- **Soft Delete Support**: New `--soft-delete` / `-sd` flag for `vm make migration` command
  - Automatically generates `deleted_at TIMESTAMP DEFAULT NULL` column
  - Creates index on `deleted_at` for efficient soft-delete queries
  - Flag can be placed anywhere in command (before or after migration name)
  - Example: `vm make migration users -sd` → creates `create_users_table.sql`
- **Auto-Update Triggers**: New `--triggers` / `-t` flag for automatic `updated_at` timestamp management
  - References centralized functions from `migrations/functions.sql`
  - Automatically updates `updated_at` column on every row change  
  - **With `-sd`**: Uses combined function that prevents updates on soft-deleted records
  - Prevents data anomalies from stale timestamps
  - Cleaner migrations: no inline function definitions
  - Example: `vm make migration users -t` or combined: `vm make migration users -sd -t`

### Improved
- **Command Structure**: Restructured `vm make` to use proper subcommands for better CLI ergonomics
  - Flags now work at any position: `vm make migration users --soft-delete` or `vm make migration users -sd`
  - Better help text and usage messages
- **Automatic File Naming**: Migration file names automatically normalized
  - `users` → `create_users_table.sql`
  - `add_email_to_users` → `add_email_to_users_table.sql`
  - Ensures consistent, descriptive file names across projects
- **Migration Templates**: Now reference centralized functions instead of inline definitions
  - Smaller, cleaner migration files
  - Easier to maintain and update trigger logic across all tables
  - One-time function setup with `vm functions migrate`

### Removed
- **Cassandra/Scylla Support**: Removed experimental Cassandra/Scylla support to focus on SQL databases (PostgreSQL, MySQL, MariaDB)
  - Cassandra requires fundamentally different query patterns (CQL vs SQL)
  - SQL-style migrations (BEGIN/COMMIT, SERIAL, CASCADE) don't translate to Cassandra
  - Removed cassandra.go, all Cassandra dialect handling, and CQL templates
  - Removed `--dialect` flag from `vm make migration` command
  - Cleaned up documentation to focus on PostgreSQL and MySQL/MariaDB only

### Why This Change?
Cassandra uses a completely different query language (CQL) and data model than SQL databases. Supporting both SQL and CQL in the same migration tool would require:
- Separate template systems
- Different transaction handling (Cassandra has no transactions)
- Different schema management approaches
- Complex dialect detection and routing

Instead of maintaining incomplete/experimental Cassandra support, we're focusing on excellent PostgreSQL and MySQL/MariaDB support.

## [1.1.1] - 2026-02-14

### Fixed
- **Configuration System**: Fixed `vm make migration` command to respect `MIGRATION_PATH` from `.vm` config file (previously ignored due to default flag value overriding config)
- **Cassandra/Scylla Keyspace Handling**: Complete rewrite of Cassandra connection flow:
  -  Automatically creates keyspace if it doesn't exist during connection
  - Proper keyspace initialization with SimpleStrategy replication
  - Keyspace is now required in DSN (format: `cassandra://host1,host2/keyspace`)
  - Fixed Query() and QueryRow() implementation for proper migration tracking
  - Status, rollback, and refresh commands now work correctly with Cassandra
- **Configuration File Naming**: Renamed `.vorzela` to `.vm` throughout the project for consistency
  - Configuration file is now `.vm` instead of `.vorzela`
  - Example file renamed to `.vm.example`
  - All documentation updated to reflect new naming
  - Legacy `.vorzela` files will still work but `.vm` is  recommended
- **Environment Variables**: Renamed `VORZELA_ENV` to `VM_ENV` throughout codebase and documentation

### Changed
- Configuration file now uses `.vm` extension instead of `.vorzela`
- Error messages now reference `.vm` config file instead of `.vorzela`
- All markdown documentation updated to use new naming conventions

### Technical Details
- cassandra.go: Implemented `cassandraRows` and `cassandraRow` structs for proper Query/QueryRow support
- cassandra.go: Added automatic keyspace creation during connection
- config.go: Updated comments and error messages to reference `.vm` files
- Renamed `loadVorzelaConfig()` function name kept for backward compatibility but now loads `.vm` files

## [1.0.0] - 2026-02-05

### Added
- Initial release of Vorzela Migration Tool
- `vm make migration` command for creating new migrations
- `vm migrate` command for running pending migrations
- `vm rollback` command for rolling back executed migrations
- `vm refresh` command for rolling back and re-running all migrations
- `vm status` command for showing migration status
- Environment support (dev/server) with separate migration directories
- Batch-based migration tracking
- Transaction-wrapped SQL execution
- Connection pooling using pgx v5
- Comprehensive error handling with helpful messages
- Migration template generation
- Support for UP and DOWN migration sections
- Example migrations for reference
- Global CLI tool installation support

### Features
- **CLI Commands**: Make, Migrate, Rollback, Refresh, Status
- **Environment Support**: Dev and server environments
- **Batch Tracking**: Group related migrations
- **Safety**: Transaction-based execution
- **Performance**: Connection pooling and indexed queries
- **User Experience**: Clear messages, progress indicators, warnings

### Documentation
- README.md - Comprehensive user guide
- QUICKSTART.md - 5-minute quick start guide
- CONFIG.md - Configuration and best practices
- ARCHITECTURE.md - Technical design documentation
- TROUBLESHOOTING.md - Common issues and solutions
- IMPLEMENTATION.md - Project overview

### Examples
- User table creation example
- Posts table with foreign keys example
- Audit log table with functions example

### Tools
- Makefile for building and installation
- setup.sh for project initialization
- .gitignore for version control

### Project Structure
- cmd/ - CLI command implementations
- internal/database/ - Database connection handling
- internal/migration/ - Migration logic
- migrations/ - Migration file storage

---

## Planned Future Releases

### [1.1.0] - Planned
- Dry-run mode for previewing migrations
- Migration validation before execution
- Migration file compression
- Better error recovery

### [1.2.0] - Planned
- Data seeding migrations
- Pre/post migration hooks
- Migration metrics

### [2.0.0] - Planned
- Multiple database support
- Migration scheduling
- Web UI for management

---

## Version Compatibility

### Go Versions
- Minimum: Go 1.21
- Tested: Go 1.25+

### PostgreSQL Versions
- Minimum: PostgreSQL 13
- Tested: PostgreSQL 13, 14, 15

### pgx Versions
- Using: pgx v5.5.5

---

## Release Notes

### 1.0.0 - Initial Release

This is the first stable release of Vorzela Migration Tool. All core functionality is implemented and tested:

- ✅ Create migrations with Laravel-style syntax
- ✅ Run migrations with transaction safety
- ✅ Rollback with batch tracking
- ✅ Refresh migrations
- ✅ Show migration status
- ✅ Environment-specific migrations
- ✅ Comprehensive documentation
- ✅ Example migrations
- ✅ Error handling and warnings
- ✅ Global CLI tool support

The tool is production-ready and can be used for managing PostgreSQL database migrations in development, staging, and production environments.

---

## Migration Guide

No migration needed for 1.0.0 as this is the initial release.

## [1.1.0] - 2026-02-06

- **Cassandra/Scylla support** - Full dialect-aware migration execution:
  - `vm migrate`, `vm status`, `vm rollback`, `vm refresh` all work with Cassandra/Scylla via `cassandra://` and `scylla://` DSN URLs.
  - Built-in Cassandra-specific migrations table with batch partition key and migration clustering key for deterministic rollbacks.
  - `vm make migration --dialect cassandra` generates CQL-style migration templates.
  - Unit tests added for dialect detection and Cassandra migrations table schema.
- Installer improvements: `install.sh` and `install.ps1` now attempt to detect the repository tag when building from source and fall back to `dev` when no tag is found.
- README updated to document new database support and bump the project to v1.1.0.

---

## Deprecations

None in 1.0.0.

---

## Known Issues

None known in 1.0.0.

---

## How to Contribute

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Update documentation
6. Submit a pull request

Please follow the existing code style and include descriptive commit messages.

---

## Support

For issues, questions, or suggestions:
1. Check TROUBLESHOOTING.md
2. Review documentation
3. Create an issue with detailed information

---

## Credits

Inspired by Laravel's database migration system.
Built with Go, pgx v5, and urfave/cli v2.

---

## License

MIT License - See LICENSE file for details
