package database

import (
	"strings"

	"github.com/vorzela/vorzela-migrate/internal/db"
)

// Connect detects DB type from DSN and returns a DB adapter.
func Connect(dsn string) (db.DB, error) {
	// MySQL/MariaDB: mysql:// prefix or @tcp or tcp( patterns
	if strings.HasPrefix(dsn, "mysql://") || strings.Contains(dsn, "@tcp") || strings.Contains(dsn, "tcp(") {
		return db.ConnectMySQL(dsn)
	}

	// Default to PostgreSQL
	return db.ConnectPostgres(dsn)
}
