package query

import (
	"context"
	"database/sql"
	"fmt"
)

// TxOptions mirrors database/sql.TxOptions (isolation / read-only).
type TxOptions struct {
	Isolation sql.IsolationLevel
	ReadOnly  bool
}

// Tx is a database transaction that also satisfies DB.
type Tx interface {
	DB
	Commit() error
	Rollback() error
}

// Beginner starts transactions (*sql.DB implements via adapter).
type Beginner interface {
	BeginTx(ctx context.Context, opts *TxOptions) (Tx, error)
}

// SQLDB adapts *sql.DB to Beginner + DB + Close (MySQL, MariaDB, or pq).
type SQLDB struct {
	*sql.DB
}

func (d *SQLDB) Close() {
	if d != nil && d.DB != nil {
		_ = d.DB.Close()
	}
}

func (d SQLDB) QueryRowContext(ctx context.Context, query string, args ...any) Row {
	return d.DB.QueryRowContext(ctx, query, args...)
}

func (d SQLDB) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	return d.DB.QueryContext(ctx, query, args...)
}

func (d SQLDB) ExecContext(ctx context.Context, query string, args ...any) (Result, error) {
	return d.DB.ExecContext(ctx, query, args...)
}

func (d SQLDB) BeginTx(ctx context.Context, opts *TxOptions) (Tx, error) {
	var o *sql.TxOptions
	if opts != nil {
		o = &sql.TxOptions{Isolation: opts.Isolation, ReadOnly: opts.ReadOnly}
	}
	tx, err := d.DB.BeginTx(ctx, o)
	if err != nil {
		return nil, err
	}
	return SQLTx{Tx: tx}, nil
}

// SQLTx adapts *sql.Tx to Tx.
type SQLTx struct {
	*sql.Tx
}

func (t SQLTx) QueryRowContext(ctx context.Context, query string, args ...any) Row {
	return t.Tx.QueryRowContext(ctx, query, args...)
}

func (t SQLTx) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	return t.Tx.QueryContext(ctx, query, args...)
}

func (t SQLTx) ExecContext(ctx context.Context, query string, args ...any) (Result, error) {
	return t.Tx.ExecContext(ctx, query, args...)
}

// Transaction runs fn inside a transaction; commits on nil, rolls back on error/panic.
func Transaction(ctx context.Context, db Beginner, fn func(ctx context.Context, tx Tx) error) (err error) {
	return TransactionOpts(ctx, db, nil, fn)
}

// TransactionOpts is Transaction with isolation / read-only options.
func TransactionOpts(ctx context.Context, db Beginner, opts *TxOptions, fn func(ctx context.Context, tx Tx) error) (err error) {
	if db == nil {
		return fmt.Errorf("vorm/query: Transaction requires Beginner")
	}
	if fn == nil {
		return fmt.Errorf("vorm/query: Transaction requires callback")
	}
	tx, err := db.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("vorm/query: begin tx: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
			return
		}
		if cErr := tx.Commit(); cErr != nil {
			err = fmt.Errorf("vorm/query: commit: %w", cErr)
		}
	}()
	err = fn(ctx, tx)
	return err
}
