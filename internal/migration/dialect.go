package migration

import (
	"fmt"
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

// ResolveDialect returns PostgreSQL when d is empty (scaffold / opts default).
func ResolveDialect(d Dialect) Dialect {
	if d == "" {
		return PostgreSQL
	}
	return d
}

// IsMySQLFamily reports whether d is MySQL or MariaDB.
func IsMySQLFamily(d Dialect) bool {
	d = ResolveDialect(d)
	return d == MySQL || d == MariaDB
}

// PrimaryKeyColumnSQL returns the id column definition for scaffolds.
func PrimaryKeyColumnSQL(d Dialect) string {
	if IsMySQLFamily(d) {
		return "id BIGINT AUTO_INCREMENT PRIMARY KEY"
	}
	return "id BIGSERIAL PRIMARY KEY"
}

// TimestampType returns TIMESTAMPTZ (Postgres) or TIMESTAMP (MySQL/MariaDB).
func TimestampType(d Dialect) string {
	if IsMySQLFamily(d) {
		return "TIMESTAMP"
	}
	return "TIMESTAMPTZ"
}

// TimestampColumnSQL returns e.g. "created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP".
func TimestampColumnSQL(d Dialect, name string) string {
	return fmt.Sprintf("%s %s DEFAULT CURRENT_TIMESTAMP", name, TimestampType(d))
}

// SoftDeleteColumnSQL returns the deleted_at column definition.
func SoftDeleteColumnSQL(d Dialect) string {
	if IsMySQLFamily(d) {
		return "deleted_at TIMESTAMP NULL"
	}
	return "deleted_at TIMESTAMPTZ DEFAULT NULL"
}

// DropTableSQL returns DROP TABLE for the dialect (no CASCADE on MySQL/MariaDB).
func DropTableSQL(d Dialect, table string) string {
	if IsMySQLFamily(d) {
		return fmt.Sprintf("DROP TABLE IF EXISTS %s;", table)
	}
	return fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE;", table)
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
