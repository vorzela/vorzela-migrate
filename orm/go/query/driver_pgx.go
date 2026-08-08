package query

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OpenPostgresPgx opens a pgx/v5 pool (same as OpenPostgres with default driver).
func OpenPostgresPgx(ctx context.Context, databaseURL string) (*PgxDB, error) {
	return openPostgresPgx(ctx, databaseURL)
}

func openPostgresPgx(ctx context.Context, databaseURL string) (*PgxDB, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	return &PgxDB{Pool: pool}, nil
}

// PgxDB adapts *pgxpool.Pool to DB + Beginner.
type PgxDB struct {
	Pool *pgxpool.Pool
}

func (p *PgxDB) Close() { p.Pool.Close() }

func (p *PgxDB) QueryRowContext(ctx context.Context, sql string, args ...any) Row {
	return pgxRow{p.Pool.QueryRow(ctx, sql, args...)}
}

func (p *PgxDB) QueryContext(ctx context.Context, sql string, args ...any) (Rows, error) {
	rows, err := p.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return &pgxRows{rows: rows}, nil
}

func (p *PgxDB) ExecContext(ctx context.Context, sql string, args ...any) (Result, error) {
	tag, err := p.Pool.Exec(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgxResult{tag: tag}, nil
}

func (p *PgxDB) BeginTx(ctx context.Context, _ *TxOptions) (Tx, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &PgxTx{ctx: ctx, tx: tx}, nil
}

// PgxTx adapts pgx.Tx.
type PgxTx struct {
	ctx context.Context
	tx  pgx.Tx
}

func (t *PgxTx) QueryRowContext(ctx context.Context, sql string, args ...any) Row {
	return pgxRow{t.tx.QueryRow(ctx, sql, args...)}
}

func (t *PgxTx) QueryContext(ctx context.Context, sql string, args ...any) (Rows, error) {
	rows, err := t.tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return &pgxRows{rows: rows}, nil
}

func (t *PgxTx) ExecContext(ctx context.Context, sql string, args ...any) (Result, error) {
	tag, err := t.tx.Exec(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgxResult{tag: tag}, nil
}

func (t *PgxTx) Commit() error {
	ctx := t.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return t.tx.Commit(ctx)
}

func (t *PgxTx) Rollback() error {
	ctx := t.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return t.tx.Rollback(ctx)
}

type pgxRow struct{ r pgx.Row }

func (r pgxRow) Scan(dest ...any) error { return r.r.Scan(dest...) }

type pgxRows struct{ rows pgx.Rows }

func (r *pgxRows) Next() bool             { return r.rows.Next() }
func (r *pgxRows) Scan(dest ...any) error { return r.rows.Scan(dest...) }
func (r *pgxRows) Close() error           { r.rows.Close(); return nil }
func (r *pgxRows) Err() error             { return r.rows.Err() }

type pgxResult struct{ tag pgconn.CommandTag }

func (r pgxResult) RowsAffected() (int64, error) { return r.tag.RowsAffected(), nil }
func (r pgxResult) LastInsertId() (int64, error) {
	return 0, fmt.Errorf("vorm/query: LastInsertId unsupported on Postgres — use RETURNING")
}
