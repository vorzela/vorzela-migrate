package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/vorzela/vorzela-migrate/internal/db"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
)

// IsMySQLDSN reports whether the DSN should use the MySQL/MariaDB driver.
func IsMySQLDSN(dsn string) bool {
	return strings.HasPrefix(dsn, "mysql://") ||
		strings.Contains(dsn, "@tcp") ||
		strings.Contains(dsn, "tcp(")
}

// Connect detects the database type from the DSN, opens a connection, and
// verifies it with a ping. A lazy connect (especially pgxpool) otherwise makes
// configuration errors appear much later as opaque query failures.
func Connect(dsn string) (db.DB, error) {
	var (
		conn db.DB
		err  error
	)
	if IsMySQLDSN(dsn) {
		conn, err = db.ConnectMySQL(dsn)
	} else {
		conn, err = db.ConnectPostgres(dsn)
	}
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := conn.Ping(ctx); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	return conn, nil
}

// GetSQLDB returns a standard *sql.DB connection for advanced features.
// The MySQL URL form is normalised to the driver DSN before opening.
func GetSQLDB(dsn string) (*sql.DB, error) {
	var driverName, connStr string

	if IsMySQLDSN(dsn) {
		driverName = "mysql"
		normalized, err := db.NormalizeMySQLDSN(dsn)
		if err != nil {
			return nil, err
		}
		connStr = normalized
	} else {
		driverName = "postgres"
		connStr = dsn
	}

	sqlDB, err := sql.Open(driverName, connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	sqlDB.SetMaxOpenConns(5)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return sqlDB, nil
}
