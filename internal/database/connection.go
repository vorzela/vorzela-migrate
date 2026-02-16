package database

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
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

// GetSQLDB returns a standard *sql.DB connection for advanced features
func GetSQLDB(dsn string) (*sql.DB, error) {
	// Detect driver
	var driverName string
	var connStr string

	if strings.HasPrefix(dsn, "mysql://") || strings.Contains(dsn, "@tcp") || strings.Contains(dsn, "tcp(") {
		driverName = "mysql"
		// Convert mysql:// to standard DSN if needed
		connStr = strings.TrimPrefix(dsn, "mysql://")
	} else {
		driverName = "postgres"
		connStr = dsn
	}

	sqlDB, err := sql.Open(driverName, connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return sqlDB, nil
}
