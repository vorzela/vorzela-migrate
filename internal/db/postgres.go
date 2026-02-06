package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgxRows struct {
	r pgx.Rows
}

func (p pgxRows) Next() bool                     { return p.r.Next() }
func (p pgxRows) Scan(dest ...interface{}) error { return p.r.Scan(dest...) }
func (p pgxRows) Close()                         { p.r.Close() }
func (p pgxRows) Err() error                     { return p.r.Err() }

type pgxRow struct {
	r pgx.Row
}

func (r pgxRow) Scan(dest ...interface{}) error { return r.r.Scan(dest...) }

// PgxDB wraps a pgxpool.Pool to implement DB
type PgxDB struct {
	Pool *pgxpool.Pool
}

func (p *PgxDB) Exec(ctx context.Context, query string, args ...interface{}) error {
	_, err := p.Pool.Exec(ctx, query, args...)
	return err
}

func (p *PgxDB) Query(ctx context.Context, query string, args ...interface{}) (Rows, error) {
	rows, err := p.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return pgxRows{r: rows}, nil
}

func (p *PgxDB) QueryRow(ctx context.Context, query string, args ...interface{}) Row {
	row := p.Pool.QueryRow(ctx, query, args...)
	return pgxRow{r: row}
}

func (p *PgxDB) Ping(ctx context.Context) error {
	return p.Pool.Ping(ctx)
}

func (p *PgxDB) Close() { p.Pool.Close() }

// ConnectPostgres creates a new pgxpool and wraps it
func ConnectPostgres(dsn string) (DB, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("unable to parse DSN: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection pool: %w", err)
	}

	return &PgxDB{Pool: pool}, nil
}
