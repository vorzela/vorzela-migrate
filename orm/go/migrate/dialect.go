package migrate

import (
	"strings"

	"github.com/vorzela/vorm/query"
)

// DetectDialect infers the dialect from a database URL or DSN the same way the
// vm tool picks its driver. Anything that does not look like MySQL/MariaDB is
// treated as PostgreSQL.
func DetectDialect(databaseURL string) query.Dialect {
	dsn := strings.ToLower(databaseURL)
	for _, marker := range []string{"mysql://", "mariadb", "@tcp(", "tcp("} {
		if strings.Contains(dsn, marker) {
			return query.DialectMySQL
		}
	}
	return query.DialectPostgres
}

// isMySQL reports whether d belongs to the MySQL family. MariaDB has no
// constant in the query package but callers may pass it through from config.
func isMySQL(d query.Dialect) bool {
	switch strings.ToLower(string(d)) {
	case string(query.DialectMySQL), "mariadb":
		return true
	default:
		return false
	}
}
