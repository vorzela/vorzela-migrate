package migration

import (
	"strings"
)

// Dialect represents SQL dialect differences (PostgreSQL, MySQL, etc.)
type Dialect string

const (
	PostgreSQL Dialect = "postgres"
	MySQL      Dialect = "mysql"
	MariaDB    Dialect = "mariadb"
)

// DetectDialect infers the dialect from a database URL/DSN
func DetectDialect(dsn string) Dialect {
	if strings.HasPrefix(dsn, "mysql://") || strings.Contains(dsn, "@tcp") || strings.Contains(dsn, "tcp(") {
		if strings.Contains(dsn, "mariadb") {
			return MariaDB
		}
		return MySQL
	}
	return PostgreSQL
}

// CreateMigrationTableSQL returns the CREATE TABLE statement for migrations table based on dialect
func CreateMigrationTableSQL(dialect Dialect) string {
	switch dialect {
	case MySQL, MariaDB:
		return `
		CREATE TABLE IF NOT EXISTS migrations (
			id INT AUTO_INCREMENT PRIMARY KEY,
			migration VARCHAR(255) NOT NULL UNIQUE,
			batch INT NOT NULL,
			checksum VARCHAR(64),
			executed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			execution_time_ms INT DEFAULT 0
		);
		`
	default: // PostgreSQL
		return `
		CREATE TABLE IF NOT EXISTS migrations (
			id SERIAL PRIMARY KEY,
			migration VARCHAR(255) NOT NULL UNIQUE,
			batch INTEGER NOT NULL,
			checksum VARCHAR(64),
			executed_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
			execution_time_ms INTEGER DEFAULT 0
		);
		`
	}
}
