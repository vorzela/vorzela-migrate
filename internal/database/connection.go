package database

import (
	"strings"

	"github.com/vorzela/vorzela-migrate/internal/db"
)

// Connect detects DB type from DSN and returns a DB adapter.
func Connect(dsn string) (db.DB, error) {
	// crude detection: if DSN looks like mysql or contains @tcp, treat as MySQL
	if strings.HasPrefix(dsn, "mysql://") || strings.Contains(dsn, "@tcp") || strings.Contains(dsn, "tcp(") {
		return db.ConnectMySQL(dsn)
	}

	// Cassandra/Scylla support: cassandra:// or scylla:// prefixes
	if strings.HasPrefix(dsn, "cassandra://") || strings.HasPrefix(dsn, "scylla://") {
		return db.ConnectCassandra(dsn)
	}

	return db.ConnectPostgres(dsn)
}
