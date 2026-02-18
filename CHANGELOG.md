# Changelog

All notable changes to Vorzela Migration Tool are documented in this file.

## [2.0.7] - 2026-02-19

### Bug Fixes

- **MySQL/MariaDB: Guard PostgreSQL-only Commands** 🛡️
  - `vm extensions`, `vm functions`, and `vm enums` now return a clear error when the DSN is MySQL/MariaDB instead of crashing with a cryptic SQL syntax error
  - `vm migrate` auto-run of extensions/functions/enums is silently skipped on non-PostgreSQL databases with an informational message

---

## [2.0.6] - 2026-02-19

### New Features

- **TIMESTAMPTZ for All PostgreSQL Columns** 🕐
  - All generated migration templates now use `TIMESTAMPTZ` instead of `TIMESTAMP` for `created_at`, `updated_at`, `deleted_at`
  - Applies to standard migrations, pivot/relationship tables, the migrations tracking table (`executed_at`), and the migrations lock table (`locked_at`)
  - MySQL/MariaDB dialect retains `TIMESTAMP` (TIMESTAMPTZ is PostgreSQL-specific)

### Bug Fixes

- **`prevent_hard_delete()` Missing Return** 🐛
  - Added `RETURN NULL;` to the `prevent_hard_delete` trigger function
  - PL/pgSQL requires a reachable return path in all trigger functions; without it some PostgreSQL versions reject the function definition
  - `RETURN NULL` is the correct value for a BEFORE DELETE trigger to cancel the delete operation if the exception were ever bypassed

---

## [2.0.5] - 2026-02-19

### New Features

- **vm enums Command** 🏷️
  - New `vm enums` command for managing PostgreSQL enum types
  - `vm enums migrate` — creates an `enums.sql` template on first run, then installs all enabled `CREATE TYPE` statements idempotently (using `DO` blocks so re-runs are safe)
  - `vm enums drop` — drops all enum types defined in `enums.sql` (supports `--step` for one-at-a-time and `--force` to skip confirmation)
  - `vm enums status` — compares enum types in `enums.sql` against live `pg_type`, showing current values and highlighting missing/extra types

- **Auto-Run Enums Before Migrations** 🚀
  - `vm migrate` now automatically runs enums before migrations
  - Execution order: Extensions → Functions → Enums → Migrations
  - New config option: `AUTO_RUN_ENUMS` (default: `true`)
  - Set to `false` in `.vm` file to disable auto-run behavior

---

## [2.0.4] - 2026-02-18

### New Features

- **vm refresh: Confirmation Prompt & Timing** 🔄
  - Prompts for confirmation before dropping all migrations (use `--force` to skip)
  - Shows "Dropped" label (not "Rolled back") to match the destructive semantic
  - Per-migration and total elapsed timing displayed

- **vm rollback: Rollback by Name** 🎯
  - New `--migration`/`-m` flag to rollback a specific migration by partial case-insensitive name match
  - Example: `vm rollback --migration create_users` rolls back that single file
  - New `--step`/`-n` aliases for the existing `--steps` flag

- **vm fresh: Consistent Labels & Timing** 🧹
  - Uses "Dropped" label and shows elapsed timing consistent with `vm refresh`

### Improvements

- All migration functions now return elapsed `time.Duration` for display

---

## [2.0.3] - 2026-02-18

### Bug Fixes

- **Fixed Down Section Detection** 🐛
  - `extractSection("Down")` never matched because only the `⬆` (Up) arrow was checked for section entry; added dedicated `isUpMarker` / `isDownMarker` helpers
  - Both `⬆`/`⬇` arrow format and `+goose Up`/`+goose Down` format are now correctly handled for both Up and Down sections
  - `vm rollback` and `vm refresh` no longer silently skip the DOWN SQL

- **Fixed `--step` Flag Not Recognised on Rollback**
  - Added `--step` and `-n` as aliases for the existing `--steps` flag on `vm rollback`

### New Features

- **Drift Detection: Indexes & Triggers** 🔍
  - Schema drift detection now covers missing indexes and triggers in addition to columns
  - Compares `CREATE INDEX` / `CREATE TRIGGER` statements in migration files against live database state
  - Auto-generates repair statements for missing indexes and triggers

- **vm uninstall Command** 🗑️
  - Removes the `vm` binary from the system
  - Cleans `PATH` export lines added by `vm` from shell profile files (`~/.bashrc`, `~/.zshrc`, etc.)
  - Supports `--yes` (skip confirmation) and `--keep-path` (leave shell profile untouched)

---

## [2.0.2] - 2026-02-18

### New Features

- **Transactional Migrations** 🔒
  - All migration statements now execute within a single database transaction
  - Failed migrations automatically rollback - no partial state left in database
  - Prevents orphaned tables, indexes, or constraints from failed migrations
  - Ensures database is either fully migrated or unchanged

- **Auto-Update Checksums with --force** 🔄
  - Using `--force` flag now automatically updates checksums to match current file state
  - Useful when you've intentionally modified already-run migrations
  - No more manual checksum updates needed
  - Provides clear feedback on which checksums were updated

- **Auto-Run Extensions and Functions Before Migrations** 🚀
  - `vm migrate` now automatically runs extensions and functions first
  - Prevents "function does not exist" and "extension does not exist" errors
  - Execution order: Extensions → Functions → Migrations
  - New config options: `AUTO_RUN_EXTENSIONS` and `AUTO_RUN_FUNCTIONS` (default: `true`)
  - Set to `false` in `.vm` file to disable auto-run behavior

- **Migration File Validation** 🛡️
  - Automatically validates migration files before execution
  - Prevents mixing extensions, functions, and schema migrations
  - Enforces proper execution order: extensions → functions → migrations
  - Clear error messages showing which files violate rules
  - Helpful guidance on correct migration workflow

- **Step-Limited Migration Runs** 🪜
  - New `--step N` / `-s N` flag for `vm migrate`: run only N pending migrations then stop
  - `--step 0` (default) means "run all pending" — fully backward-compatible
  - Drift detection is automatically skipped when `--step` would leave remaining migrations (avoids mid-run false positives)
  - Example: `vm migrate --step 3` runs only the next 3 pending migrations

- **Schema Drift SQL Parser** 🔬
  - New `internal/migration/schema_parser.go` builds an accurate expected schema directly from your migration SQL files
  - Parses `CREATE TABLE`, `ALTER TABLE ADD COLUMN`, and `ALTER TABLE DROP COLUMN` statements
  - Skips constraint lines (`PRIMARY KEY`, `FOREIGN KEY`, `UNIQUE`, `CHECK`, `INDEX`) to remove false positives
  - Drift detection now compares actual DB columns against columns that should exist per your migrations — not an empty map
  - Handles multiple tables modified across many migration files

- **Drift Applies Missing Columns, Then Continues** 🔧
  - When drift is detected and the user answers **yes** to "Apply these changes?", `vm migrate` now immediately runs `ALTER TABLE … ADD COLUMN` for each drifted column
  - Execution then continues to pending migrations — it no longer stops after drift detection
  - User choices at the drift prompt: `yes` (apply + continue), `no` (skip + continue), `generate` (print SQL only + continue)
  - The `auto` drift mode also applies columns and continues without prompting

- **Checksum-Mismatch Drift Interaction** 🔄
  - When migration file checksums differ from stored hashes, the tool now offers to run drift detection anyway
  - If the user accepts: missing columns are detected, applied via `ALTER TABLE`, and pending migrations run normally
  - If the user declines: pending migrations run without interruption
  - Either way, execution always continues — the old behaviour of stopping with an error is removed

- **Argument Validation for `vm migrate`** ✅
  - `vm migrate fresh` (and any other stray arguments) now prints a clear error: *"'vm migrate' takes no arguments (got "fresh"). Did you mean 'vm fresh'?"*
  - Prevents silent no-ops from typos in subcommand names

### Bug Fixes

- **Fixed VERBOSE Setting Being Overridden** 🔇
  - Fixed issue where `VERBOSE=false` in `.vm` file was ignored
  - Environment defaults (development/production) no longer override explicit user settings
  - Added `explicitVerbose` tracking to preserve user configuration
  - Users can now properly disable verbose output

- **Fixed Schema Drift False Positives** ✅
  - Fixed drift detection reporting false positives for just-migrated tables
  - Drift detection now skips when pending migrations exist (unless `--step` exhausts all of them)
  - Prevents comparing against empty expected schema — SQL parser provides accurate baseline
  - Only runs drift detection after all pending migrations are applied
  - Eliminates confusing "drift" warnings for legitimate migration columns

- **Fixed Test Nil Condition** ✅
  - Fixed impossible nil check in drift detection test
  - Removed unreachable code that caused linter warning

### Configuration

New `.vm` file options:
```
AUTO_RUN_EXTENSIONS=true   # Automatically run extensions.sql before migrations
AUTO_RUN_FUNCTIONS=true    # Automatically run functions.sql before migrations
```

### Validation Rules

- **Migration files** (.sql) should ONLY contain schema changes (CREATE TABLE, ALTER TABLE, etc.)
- **Functions** must be in `functions.sql` - run `vm functions migrate` first (or let auto-run handle it)
- **Extensions** must be in `extensions.sql` - run `vm extensions migrate` first (or let auto-run handle it)
- Files are automatically validated before migration execution
- Violations halt migration with clear error messages and resolution steps

### Technical Changes

- Updated `internal/config/config.go` - Added `explicitVerbose`, `AutoRunExtensions`, and `AutoRunFunctions` fields
- Updated `internal/migration/enhanced_executor.go` - Drift detection timing, step-gate logic, mismatch interactive flow, apply-and-continue drift behaviour
- Updated `cmd/migrate.go` - Added `--step`/`-s` flag, argument validation, pre-migration validation checks and auto-run helpers
- Added `internal/migration/schema_parser.go` - SQL-file based expected schema builder (CREATE TABLE + ALTER TABLE parsing)
- Added `internal/migration/validation.go` - Comprehensive migration file validation logic
- Added `internal/migration/drift_pending_test.go` - Tests for drift/step interactions and checksum-mismatch drift flow
- Added `internal/migration/schema_parser_test.go` - Unit tests for SQL parsing (CREATE TABLE, ALTER TABLE ADD/DROP, constraints)
- Added `internal/migration/validation_test.go` - Complete test coverage for validation rules
- Extended `internal/migration/relationship_test.go` - BelongsTo, OneToOne, Multiple, SoftDelete, Triggers, sqlc, MigrationOptionsStep

## [2.0.1] - 2026-02-18

### Bug Fixes

- **Fixed PostgreSQL RAISE EXCEPTION Syntax** 🐛
  - Fixed `prevent_hard_delete()` function template using incorrect `%%` placeholders
  - PostgreSQL requires single `%` for parameter substitution in RAISE statements
  - This caused "too many parameters specified for RAISE" error when running `vm functions migrate`

- **Fixed Upgrade Version Notice** ✅
  - Fixed duplicate version notice appearing after successful upgrade
  - Added nil check for `c.Command` in After hook to prevent potential panics
  - Upgrade command now properly suppresses version check message

- **Enhanced Error Messages for Missing Functions** 💡
  - Added helpful error detection for missing database functions
  - Provides clear solution: run `vm functions migrate` before `vm migrate`
  - Detects all common trigger functions: `auto_update_timestamp()`, `protect_soft_deleted()`, etc.
  - Works in both enhanced and regular migration modes
  - Includes specific instructions and documentation links

- **Improved Error Context for Missing Tables** 📋
  - Better error messages when migrations reference non-existent tables
  - Suggests checking migration order and dependencies

### Technical Changes

- Updated `internal/migration/functions.go` - Fixed RAISE EXCEPTION syntax
- Updated `main.go` - Added nil check for command in After hook  
- Updated `internal/migration/enhanced_executor.go` - Added `enhanceError()` method
- Updated `internal/migration/executor.go` - Added `enhanceMigrationError()` function

## [2.0.0] - 2026-02-16

### Major Features

- **Environment-Based Auto Configuration** 🌍
  - Set `ENVIRONMENT=development` or `ENVIRONMENT=production` in `.vm` file
  - Tool automatically applies appropriate migration strategy
  - Development: Enhanced mode + verbose logging + drift detection (no online mode)
  - Production: Enhanced mode + online migrations + checksums + drift detection
  - No more typing long command flags!
  - New methods: `Config.ApplyEnvironmentDefaults()`, `Config.IsProduction()`, `Config.IsDevelopment()`

- **Checksum Validation** ✅
  - SHA-256 checksums calculated and stored for each migration
  - Automatic verification on subsequent runs  - Detects if migration files were modified after execution
  - Prevents data corruption from modified migrations
  - New file: `internal/migration/checksum.go`

- **Migration Locking** 🔐
  - Prevents concurrent migrations from multiple processes
  - PostgreSQL: Advisory locks (`pg_try_advisory_lock`)
  - MySQL/MariaDB: Named locks (`GET_LOCK`)
  - Fallback: Table-based locking for other databases
  - 30-second timeout with clear error messages
  - New file: `internal/migration/lock.go`

- **Schema Drift Detection & Auto-Fix** 🔍
  - Detects manually added columns not tracked in migrations
  - Three handling modes:
    - `auto`: Automatically apply ALTER statements in background
    - `prompt`: Ask user yes/no/generate (default)
    - `reject`: Fail migration if drift detected
  - Generates proper ALTER TABLE statements
  - PostgreSQL and MySQL support
  - New file: `internal/migration/drift.go`

- **Enhanced Colored Logging** 🎨
  - ANSI color-coded output for better readability
  - Execution timing for each migration
  - Progress indicators for batch operations
  - Log levels: SUCCESS, INFO, WARNING, ERROR, DEBUG
  - Verbose mode for detailed output
  - New file: `internal/output/logger.go`

- **Online Migrations (Zero-Downtime)** 🌐
  - PostgreSQL: `CREATE INDEX CONCURRENTLY`
  - MySQL 8.0+: `ALGORITHM=INSTANT/INPLACE`
  - Batch updates to avoid long table locks
  - Adds columns without blocking reads/writes
  - Production-safe strategies
  - New file: `internal/migration/online.go`

- **Partial Failure Recovery** 🛡️
  - Tracks which statements succeeded before failure
  - Shows exactly where migration failed
  - Recovery guidance for manual fixes
  - Enhanced `EnhancedExecutor` with statement-level tracking

### Testing & Quality

- **Comprehensive Test Suite** ✨
  - 205+ tests across all packages
  - Unit tests for all new features
  - Checksum validation tests
  - Lock mechanism tests
  - Drift detection tests
  - Online migration tests
  - Logger output tests
  - Environment config tests
  - All tests passing ✅

### Configuration

- **New Config Options**
  - `ENVIRONMENT`: `development`, `dev`, `production`, `prod`
  - `DRIFT_HANDLING`: `auto`, `prompt`, `reject`
  - `ENHANCED`: Enable enhanced migration features
  - `ONLINE`: Enable zero-downtime migrations
  - `VERIFY_CHECKSUMS`: Enable checksum validation
  - `DETECT_DRIFT`: Enable drift detection
  - `VERBOSE`: Enable detailed logging

### Breaking Changes

- Migrations table schema updated with new columns:
  - `checksum VARCHAR(64)` - SHA-256 hash of migration content
  - `execution_time_ms BIGINT` - Execution duration in milliseconds
- Existing migrations table will be automatically migrated on first run

### Documentation

- Removed redundant .md files (consolidated into README.md)
- Updated README with v2.0.0 features
- Enhanced ARCHITECTURE.md with new components
- Improved TROUBLESHOOTING.md
- Added example.vm with full configuration options

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
- **sqlc & goose Integration**: Comprehensive documentation and full compatibility
  - New `SQLC_SUPPORT` configuration option in `.vm` file for enabling goose markers
  - When enabled, includes `-- +goose Up` and `-- +goose Down` markers in migration templates
  - Disabled by default to keep templates clean for users who don't use sqlc/goose
  - Complete workflow documentation for using Vorzela with sqlc (type-safe SQL)
  - Example projects showing migrations → sqlc → Go code generation pipeline
  - Goose compatibility for running Vorzela migrations with goose CLI
  - Works with both regular migrations and relationship-based migrations
  - Full integration guide with best practices and project structure examples
  
### Improved
- **Better Error Messages**: Enhanced error handling for missing migrations table
  - Clear message: "migrations table does not exist. Please run your first migration with: vm migrate"
  - Helps new users understand initialization process
  - Prevents confusion when running `vm status` before first migration
  - Updated troubleshooting documentation with migration table initialization guidance

### Changed
- **Template Format**: Migration templates no longer include goose markers by default
  - Cleaner templates for the majority of users who don't use sqlc/goose
  - Opt-in via `SQLC_SUPPORT=true` configuration keeps the tool flexible
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
