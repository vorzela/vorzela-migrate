package db

import "context"

// Rows is a minimal rows interface used by the migration code
type Rows interface {
	Next() bool
	Scan(dest ...interface{}) error
	Close()
	Err() error
}

// Row is a minimal single-row scanner
type Row interface {
	Scan(dest ...interface{}) error
}

// DB is a minimal database interface used by migrations
type DB interface {
	Exec(ctx context.Context, query string, args ...interface{}) error
	Query(ctx context.Context, query string, args ...interface{}) (Rows, error)
	QueryRow(ctx context.Context, query string, args ...interface{}) Row
	Ping(ctx context.Context) error
	Close()
}
