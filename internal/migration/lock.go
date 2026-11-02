package migration

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// MigrationLock represents a database-level migration lock.
//
// PostgreSQL advisory locks and MySQL GET_LOCK are session-scoped. Using a
// pooled *sql.DB directly is unsafe: acquisition and release can land on
// different connections, leaking the lock until that connection closes. This
// type pins a dedicated *sql.Conn for the whole lock lifetime.
type MigrationLock struct {
	db      *sql.DB
	dialect Dialect
	conn    *sql.Conn
	locked  bool
}

// NewMigrationLock creates a new migration lock manager.
func NewMigrationLock(db *sql.DB, dialect Dialect) *MigrationLock {
	return &MigrationLock{db: db, dialect: dialect}
}

// AcquireLock attempts to acquire a migration lock.
func (ml *MigrationLock) AcquireLock(ctx context.Context) error {
	conn, err := ml.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("failed to reserve lock connection: %w", err)
	}
	ml.conn = conn

	switch ml.dialect {
	case PostgreSQL:
		err = ml.acquirePostgresLock(ctx)
	case MySQL, MariaDB:
		err = ml.acquireMySQLLock(ctx)
	default:
		err = ml.acquireTableLock(ctx)
	}
	if err != nil {
		_ = conn.Close()
		ml.conn = nil
		return err
	}
	return nil
}

// ReleaseLock releases the migration lock and returns the pinned connection to
// the pool.
func (ml *MigrationLock) ReleaseLock() error {
	if !ml.locked {
		if ml.conn != nil {
			_ = ml.conn.Close()
			ml.conn = nil
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var err error
	switch ml.dialect {
	case PostgreSQL:
		err = ml.releasePostgresLock(ctx)
	case MySQL, MariaDB:
		err = ml.releaseMySQLLock(ctx)
	default:
		err = ml.releaseTableLock(ctx)
	}

	ml.locked = false
	if ml.conn != nil {
		// Closing the connection is also a safety net: if the explicit unlock
		// failed, the server releases session locks when the connection closes.
		closeErr := ml.conn.Close()
		ml.conn = nil
		if err == nil {
			err = closeErr
		}
	}
	return err
}

// acquirePostgresLock uses PostgreSQL advisory locks.
func (ml *MigrationLock) acquirePostgresLock(ctx context.Context) error {
	const lockKey = 1986324789 // hash of "vorzela-migrate"

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		var acquired bool
		err := ml.conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", lockKey).Scan(&acquired)
		if err != nil {
			return fmt.Errorf("failed to acquire lock: %w", err)
		}
		if acquired {
			ml.locked = true
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout: another migration is currently running")
		case <-ticker.C:
		}
	}
}

// releasePostgresLock releases the PostgreSQL advisory lock.
func (ml *MigrationLock) releasePostgresLock(ctx context.Context) error {
	const lockKey = 1986324789
	var released bool
	err := ml.conn.QueryRowContext(ctx, "SELECT pg_advisory_unlock($1)", lockKey).Scan(&released)
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}
	if !released {
		return fmt.Errorf("advisory lock was not held by this connection (it may have been lost)")
	}
	return nil
}

// acquireMySQLLock uses MySQL GET_LOCK on the pinned connection.
func (ml *MigrationLock) acquireMySQLLock(ctx context.Context) error {
	const lockName = "vorzela_migrate_lock"
	const lockTimeout = 30 // seconds

	var acquired sql.NullInt64
	err := ml.conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", lockName, lockTimeout).Scan(&acquired)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		return fmt.Errorf("another migration is currently running")
	}

	ml.locked = true
	return nil
}

// releaseMySQLLock releases the MySQL named lock.
func (ml *MigrationLock) releaseMySQLLock(ctx context.Context) error {
	const lockName = "vorzela_migrate_lock"
	var released sql.NullInt64
	err := ml.conn.QueryRowContext(ctx, "SELECT RELEASE_LOCK(?)", lockName).Scan(&released)
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}
	if released.Valid && released.Int64 == 0 {
		return fmt.Errorf("named lock was not held by this connection (it may have been lost)")
	}
	return nil
}

// acquireTableLock uses a migrations_lock table as a fallback.
func (ml *MigrationLock) acquireTableLock(ctx context.Context) error {
	createTableSQL := `
		CREATE TABLE IF NOT EXISTS migrations_lock (
			id INTEGER PRIMARY KEY DEFAULT 1,
			locked BOOLEAN NOT NULL DEFAULT FALSE,
			locked_at TIMESTAMPTZ,
			locked_by VARCHAR(255),
			CHECK (id = 1)
		);
	`
	if _, err := ml.conn.ExecContext(ctx, createTableSQL); err != nil {
		return fmt.Errorf("failed to create lock table: %w", err)
	}

	_, err := ml.conn.ExecContext(ctx, `
		INSERT INTO migrations_lock (id, locked)
		VALUES (1, FALSE)
		ON CONFLICT (id) DO NOTHING
	`)
	if err != nil {
		_, err = ml.conn.ExecContext(ctx, `
			INSERT IGNORE INTO migrations_lock (id, locked)
			VALUES (1, FALSE)
		`)
		if err != nil {
			return fmt.Errorf("failed to initialize lock: %w", err)
		}
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		result, err := ml.conn.ExecContext(ctx, `
			UPDATE migrations_lock
			SET locked = TRUE, locked_at = CURRENT_TIMESTAMP
			WHERE id = 1 AND locked = FALSE
		`)
		if err != nil {
			return fmt.Errorf("failed to acquire lock: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to check lock status: %w", err)
		}
		if rows > 0 {
			ml.locked = true
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout: another migration is currently running")
		case <-ticker.C:
		}
	}
}

// releaseTableLock releases the table-based fallback lock.
func (ml *MigrationLock) releaseTableLock(ctx context.Context) error {
	_, err := ml.conn.ExecContext(ctx, "UPDATE migrations_lock SET locked = FALSE WHERE id = 1")
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}
	return nil
}
