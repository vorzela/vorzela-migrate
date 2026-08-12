package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const (
	// pgLockKey is the advisory lock key vm uses, a hash of "vorzela-migrate".
	// Both tools must agree on it to lock each other out.
	pgLockKey = 1986324789

	mysqlLockName = "vorzela_migrate_lock"

	lockPollInterval   = 100 * time.Millisecond
	lockReleaseTimeout = 10 * time.Second
)

// withLock runs fn while holding the migration lock.
//
// Advisory and GET_LOCK locks are session scoped. When db is a pool the release
// can be sent on a different session than the acquire, so the lock guards
// against concurrent processes, not against concurrent goroutines sharing a pool.
func (r *Runner) withLock(ctx context.Context, fn func(context.Context) error) error {
	if err := r.ready(); err != nil {
		return err
	}
	if r.opts.SkipLock {
		return fn(ctx)
	}
	if err := r.acquireLock(ctx); err != nil {
		return err
	}
	// Deferred so the lock is also released while a panic unwinds.
	defer r.releaseLock(ctx)
	return fn(ctx)
}

func (r *Runner) acquireLock(ctx context.Context) error {
	if isMySQL(r.opts.Dialect) {
		return r.acquireMySQLLock(ctx)
	}
	return r.acquirePostgresLock(ctx)
}

func (r *Runner) acquirePostgresLock(ctx context.Context) error {
	deadline := time.NewTimer(r.opts.LockTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(lockPollInterval)
	defer ticker.Stop()

	for {
		var acquired bool
		if err := r.db.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", pgLockKey).Scan(&acquired); err != nil {
			return fmt.Errorf("vorm/migrate: acquire migration lock: %w", err)
		}
		if acquired {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("vorm/migrate: acquire migration lock: %w", ctx.Err())
		case <-deadline.C:
			return fmt.Errorf("vorm/migrate: %w", ErrLockTimeout)
		case <-ticker.C:
		}
	}
}

func (r *Runner) acquireMySQLLock(ctx context.Context) error {
	seconds := int(r.opts.LockTimeout.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	// GET_LOCK blocks for up to the given number of seconds and reports NULL when
	// something went wrong rather than 0.
	var acquired sql.NullInt64
	if err := r.db.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", mysqlLockName, seconds).Scan(&acquired); err != nil {
		return fmt.Errorf("vorm/migrate: acquire migration lock: %w", err)
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		return fmt.Errorf("vorm/migrate: %w", ErrLockTimeout)
	}
	return nil
}

// releaseLock reports failures through the logger: the caller's error, if any,
// is more useful than an unlock problem.
func (r *Runner) releaseLock(parent context.Context) {
	// Detached from the caller's context so a cancelled or expired ctx cannot
	// leave the lock held for the rest of the session.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), lockReleaseTimeout)
	defer cancel()

	var err error
	if isMySQL(r.opts.Dialect) {
		_, err = r.db.ExecContext(ctx, "SELECT RELEASE_LOCK(?)", mysqlLockName)
	} else {
		_, err = r.db.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", pgLockKey)
	}
	if err != nil {
		r.logf("migrate: warning: release migration lock: %v", err)
	}
}
