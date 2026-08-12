package query

import (
	"context"
	"fmt"
	"strings"
)

// PostgresDriver selects which Postgres client library OpenPostgres uses.
type PostgresDriver string

const (
	// PostgresPgx is the default driver (jackc/pgx/v5 pool).
	PostgresPgx PostgresDriver = "pgx"
	// PostgresPQ uses database/sql + lib/pq.
	PostgresPQ PostgresDriver = "pq"
)

type openConfig struct {
	Driver PostgresDriver
}

// OpenOption configures OpenPostgres.
type OpenOption func(*openConfig)

// WithDriver selects pgx (default) or pq for Postgres.
//
//	query.OpenPostgres(ctx, url)                          // pgx
//	query.OpenPostgres(ctx, url, query.WithDriver(query.PostgresPQ))
func WithDriver(d PostgresDriver) OpenOption {
	return func(c *openConfig) { c.Driver = d }
}

// Conn is DB + Beginner + Close (pool / sql.DB).
type Conn interface {
	DB
	Beginner
	Close()
}

// DetectDialect infers the dialect from a connection string. Anything that is
// not recognisably MySQL/MariaDB is treated as Postgres.
func DetectDialect(databaseURL string) Dialect {
	s := strings.ToLower(strings.TrimSpace(databaseURL))
	switch {
	case strings.HasPrefix(s, "mysql://"),
		strings.HasPrefix(s, "mariadb://"),
		strings.Contains(s, "@tcp("),
		strings.Contains(s, "mariadb"):
		return DialectMySQL
	default:
		return DialectPostgres
	}
}

// Open connects using the dialect implied by databaseURL. This is the one-call
// entry point for applications: Postgres URLs go through pgx (or lib/pq with
// WithDriver), MySQL and MariaDB DSNs through database/sql.
func Open(ctx context.Context, databaseURL string, opts ...OpenOption) (Conn, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, fmt.Errorf("vorm/query: empty DATABASE_URL")
	}
	if DetectDialect(databaseURL) == DialectMySQL {
		return OpenMySQL(stripMySQLScheme(databaseURL))
	}
	return OpenPostgres(ctx, databaseURL, opts...)
}

// stripMySQLScheme converts a mysql:// URL into the DSN go-sql-driver expects.
func stripMySQLScheme(url string) string {
	for _, prefix := range []string{"mysql://", "mariadb://"} {
		if rest, ok := strings.CutPrefix(url, prefix); ok {
			return rest
		}
	}
	return url
}

// OpenPostgres opens Postgres. Default driver is pgx v5; pass WithDriver(PostgresPQ) for lib/pq.
func OpenPostgres(ctx context.Context, databaseURL string, opts ...OpenOption) (Conn, error) {
	cfg := openConfig{Driver: PostgresPgx}
	for _, o := range opts {
		o(&cfg)
	}
	switch cfg.Driver {
	case "", PostgresPgx:
		return openPostgresPgx(ctx, databaseURL)
	case PostgresPQ:
		return OpenPostgresPQ(databaseURL)
	default:
		return nil, fmt.Errorf("vorm/query: unknown Postgres driver %q (use pgx or pq)", cfg.Driver)
	}
}
