# Vorzela Migrate v1.1.0 (Draft Release Notes)

Release date: 2026-02-06

Highlights

- Experimental Cassandra/Scylla support
  - Added a minimal `gocql`-based adapter at `internal/db/cassandra.go`.
  - Basic connection and session creation is supported via DSN like `cassandra://host1,host2/keyspace`.
  - Important: Query/QueryRow semantics and full migration behavior for CQL differ from SQL-based databases; migrations should be reviewed and adapted before running in production.

- Installer improvements
  - `install.sh` and `install.ps1` attempt to detect the latest repository tag when building from source and embed it into the binary using `-ldflags`.
  - If no tag is found, the builder falls back to `dev` as the embedded version.

- Misc
  - Updated `README.md` to note Cassandra/Scylla experimental support and bumped version to v1.1.0.
  - Added migration templates for Cassandra under `migrations/templates/` to guide writing CQL migrations.

Notes and migration guidance

- Cassandra/Scylla are not drop-in replacements for relational databases. CQL lacks multi-statement transactions and has different schema evolution semantics. Review and test migrations carefully.
- For now, the tool's migration engine treats Cassandra support as experimental. Use a staging Scylla/Cassandra instance to validate migrations before applying to production.

Security & Upgrade

- As with prior releases, the CI builds and uploads release artifacts on tag push. Use the `vm upgrade` command to fetch and install newer releases via the project's installer scripts.

Known issues

- `Query`/`QueryRow` are intentionally unimplemented for Cassandra in this release — calling code that expects SQL-like row iteration will fail. This was done to avoid accidental misuse; future work will add a CQL-aware migration execution path.

How to test

1. Clone the repo and build locally: `go build -o vm main.go`
2. Start a Scylla/Cassandra instance (Docker recommended).
3. Create a CQL migration file using `migrations/templates/cassandra_up.cql` and `cassandra_down.cql` as guides.
4. Run `vm migrate --dsn "cassandra://127.0.0.1:9042/my_keyspace"` after reviewing the migration contents.
