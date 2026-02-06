package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

type sqlRows struct {
	r *sql.Rows
}

func (s sqlRows) Next() bool                     { return s.r.Next() }
func (s sqlRows) Scan(dest ...interface{}) error { return s.r.Scan(dest...) }
func (s sqlRows) Close()                         { s.r.Close() }
func (s sqlRows) Err() error                     { return s.r.Err() }

type sqlRow struct {
	r *sql.Row
}

func (r sqlRow) Scan(dest ...interface{}) error { return r.r.Scan(dest...) }

// SqlDB wraps database/sql DB
type SqlDB struct {
	DB *sql.DB
}

func (s *SqlDB) Exec(ctx context.Context, query string, args ...interface{}) error {
	_, err := s.DB.ExecContext(ctx, query, args...)
	return err
}

func (s *SqlDB) Query(ctx context.Context, query string, args ...interface{}) (Rows, error) {
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return sqlRows{r: rows}, nil
}

func (s *SqlDB) QueryRow(ctx context.Context, query string, args ...interface{}) Row {
	row := s.DB.QueryRowContext(ctx, query, args...)
	return sqlRow{r: row}
}

func (s *SqlDB) Ping(ctx context.Context) error {
	return s.DB.PingContext(ctx)
}

func (s *SqlDB) Close() { s.DB.Close() }

// parseMySQLURL converts a mysql://... URL to DSN accepted by go-sql-driver/mysql
func parseMySQLURL(dsn string) (string, error) {
	if !strings.HasPrefix(dsn, "mysql://") {
		return dsn, nil
	}

	u, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}

	user := u.User.Username()
	pass, _ := u.User.Password()
	host := u.Host
	db := strings.TrimPrefix(u.Path, "/")
	// preserve query
	q := u.RawQuery

	dsnOut := fmt.Sprintf("%s:%s@tcp(%s)/%s", user, pass, host, db)
	if q != "" {
		dsnOut = dsnOut + "?" + q
	}
	return dsnOut, nil
}

// ConnectMySQL opens a database/sql connection for MySQL
func ConnectMySQL(dsn string) (DB, error) {
	out, err := parseMySQLURL(dsn)
	if err != nil {
		return nil, err
	}

	sqlDB, err := sql.Open("mysql", out)
	if err != nil {
		return nil, fmt.Errorf("failed to open mysql: %w", err)
	}

	// set some reasonable defaults
	sqlDB.SetMaxOpenConns(5)
	sqlDB.SetMaxIdleConns(1)

	return &SqlDB{DB: sqlDB}, nil
}
