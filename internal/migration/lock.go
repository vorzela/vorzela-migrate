package migration

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// MigrationLock represents a database-level migration lock
type MigrationLock struct {
	db      *sql.DB
	dialect Dialect
	locked  bool
}

// NewMigrationLock creates a new migration lock manager
func NewMigrationLock(db *sql.DB, dialect Dialect) *MigrationLock {
	return &MigrationLock{
		db:      db,
		dialect: dialect,
		locked:  false,
	}
}

// AcquireLock attempts to acquire a migration lock
func (ml *MigrationLock) AcquireLock(ctx context.Context) error {
	switch ml.dialect {
	case PostgreSQL:
		return ml.acquirePostgresLock(ctx)
	case MySQL, MariaDB:
		return ml.acquireMySQLLock()
	default:
		return ml.acquireTableLock(ctx)
	}
}

// ReleaseLock releases the migration lock
func (ml *MigrationLock) ReleaseLock() error {
	if !ml.locked {
		return nil
	}

	switch ml.dialect {
	case PostgreSQL:
		return ml.releasePostgresLock()
	case MySQL, MariaDB:
		return ml.releaseMySQLLock()
	default:
		return ml.releaseTableLock()
	}
}

// acquirePostgresLock uses PostgreSQL advisory locks
func (ml *MigrationLock) acquirePostgresLock(ctx context.Context) error {
	// Use a fixed advisory lock key for migrations
	// Hash of "vorzela-migrate" -> 1986324789
	const lockKey = 1986324789

	// Try to acquire lock with timeout
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(30 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout: another migration is currently running")
		case <-ticker.C:
			var acquired bool
			err := ml.db.QueryRow("SELECT pg_try_advisory_lock($1)", lockKey).Scan(&acquired)
			if err != nil {
				return fmt.Errorf("failed to acquire lock: %w", err)
			}
			if acquired {
				ml.locked = true
				return nil
			}
		}
	}
}

// releasePostgresLock releases PostgreSQL advisory lock
func (ml *MigrationLock) releasePostgresLock() error {
	const lockKey = 1986324789
	_, err := ml.db.Exec("SELECT pg_advisory_unlock($1)", lockKey)
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}
	ml.locked = false
	return nil
}

// acquireMySQLLock uses MySQL GET_LOCK function
func (ml *MigrationLock) acquireMySQLLock() error {
	const lockName = "vorzela_migrate_lock"
	const lockTimeout = 30 // seconds

	var acquired int
	err := ml.db.QueryRow("SELECT GET_LOCK(?, ?)", lockName, lockTimeout).Scan(&acquired)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}

	if acquired != 1 {
		return fmt.Errorf("another migration is currently running")
	}

	ml.locked = true
	return nil
}

// releaseMySQLLock releases MySQL named lock
func (ml *MigrationLock) releaseMySQLLock() error {
	const lockName = "vorzela_migrate_lock"
	_, err := ml.db.Exec("SELECT RELEASE_LOCK(?)", lockName)
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}
	ml.locked = false
	return nil
}

// acquireTableLock uses a migrations_lock table as fallback
func (ml *MigrationLock) acquireTableLock(ctx context.Context) error {
	// Create lock table if not exists
	createTableSQL := `
		CREATE TABLE IF NOT EXISTS migrations_lock (
			id INTEGER PRIMARY KEY DEFAULT 1,
			locked BOOLEAN NOT NULL DEFAULT FALSE,
			locked_at TIMESTAMPTZ,
			locked_by VARCHAR(255),
			CHECK (id = 1)
		);
	`
	_, err := ml.db.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create lock table: %w", err)
	}

	// Insert initial row if not exists
	_, err = ml.db.Exec(`
		INSERT INTO migrations_lock (id, locked) 
		VALUES (1, FALSE) 
		ON CONFLICT (id) DO NOTHING
	`)
	if err != nil {
		// Try MySQL syntax
		_, err = ml.db.Exec(`
			INSERT IGNORE INTO migrations_lock (id, locked) 
			VALUES (1, FALSE)
		`)
		if err != nil {
			return fmt.Errorf("failed to initialize lock: %w", err)
		}
	}

	// Try to acquire lock
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(30 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout: another migration is currently running")
		case <-ticker.C:
			result, err := ml.db.Exec(`
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
		}
	}
}

// releaseTableLock releases table-based lock
func (ml *MigrationLock) releaseTableLock() error {
	_, err := ml.db.Exec("UPDATE migrations_lock SET locked = FALSE WHERE id = 1")
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}
	ml.locked = false
	return nil
}
