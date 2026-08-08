package query

import (
	"context"
	"fmt"
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
