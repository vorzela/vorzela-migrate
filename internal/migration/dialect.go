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
	Cassandra  Dialect = "cassandra"
)

// DetectDialect infers the dialect from a database URL/DSN
func DetectDialect(dsn string) Dialect {
	if strings.HasPrefix(dsn, "cassandra://") || strings.HasPrefix(dsn, "scylla://") {
		return Cassandra
	}
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
			executed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		`
	case Cassandra:
		// Cassandra/Scylla: use batch as partition key to allow grouping by batch
		return `
		CREATE TABLE IF NOT EXISTS migrations (
			batch int,
			migration text,
			executed_at timestamp,
			PRIMARY KEY (batch, migration)
		);
		`
	default: // PostgreSQL
		return `
		CREATE TABLE IF NOT EXISTS migrations (
			id SERIAL PRIMARY KEY,
			migration VARCHAR(255) NOT NULL UNIQUE,
			batch INTEGER NOT NULL,
			executed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		`
	}
}

// Add Cassandra case

// ConvertPlaceholders converts ? placeholders (MySQL) to $N (PostgreSQL) or vice versa
// For now, we keep queries dialect-neutral and rely on underlying drivers to handle it
func ConvertPlaceholders(query string, fromDialect, toDialect Dialect) string {
	// Database drivers typically handle placeholder conversion, so return as-is
	return query
}
