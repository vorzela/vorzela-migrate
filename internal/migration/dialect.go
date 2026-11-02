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

// CreateIndexSQL returns a CREATE INDEX statement for scaffolds. MySQL 8.0
// does not support IF NOT EXISTS for CREATE INDEX (MariaDB does); omitting it
// keeps the generated migration portable across both.
func CreateIndexSQL(d Dialect, unique bool, name, table string, columns ...string) string {
	colList := strings.Join(columns, ", ")
	if IsMySQLFamily(d) {
		if unique {
			return fmt.Sprintf("CREATE UNIQUE INDEX %s ON %s(%s);", name, table, colList)
		}
		return fmt.Sprintf("CREATE INDEX %s ON %s(%s);", name, table, colList)
	}
	uniqueWord := ""
	if unique {
		uniqueWord = "UNIQUE "
	}
	return fmt.Sprintf("CREATE %sINDEX IF NOT EXISTS %s ON %s(%s);", uniqueWord, name, table, colList)
}

// DropIndexSQL returns a portable DROP INDEX statement.
func DropIndexSQL(d Dialect, name, table string) string {
	if IsMySQLFamily(d) {
		return fmt.Sprintf("DROP INDEX %s ON %s;", name, table)
	}
	return fmt.Sprintf("DROP INDEX IF EXISTS %s;", name)
}

// AddColumnSQL returns a portable ALTER TABLE ADD COLUMN statement. MySQL 8.0
// does not support IF NOT EXISTS here; MariaDB does, but omitting it keeps the
// statement valid on both.
func AddColumnSQL(d Dialect, table, column, definition string) string {
	if IsMySQLFamily(d) {
		return fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s;", table, column, definition)
	}
	return fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s %s;", table, column, definition)
}

// DropColumnSQL returns a portable ALTER TABLE DROP COLUMN statement.
func DropColumnSQL(d Dialect, table, column string) string {
	if IsMySQLFamily(d) {
		return fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", table, column)
	}
	return fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS %s;", table, column)
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
