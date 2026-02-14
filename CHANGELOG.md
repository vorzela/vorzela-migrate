# Changelog

All notable changes to Vorzela Migration Tool are documented in this file.

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
