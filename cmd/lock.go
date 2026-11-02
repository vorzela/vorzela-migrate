package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/vorzela/vorzela-migrate/internal/config"
	"github.com/vorzela/vorzela-migrate/internal/database"
	"github.com/vorzela/vorzela-migrate/internal/migration"
	"github.com/vorzela/vorzela-migrate/internal/output"
)

// acquireMigrationLock pins a dedicated connection and takes the migration
// lock. It is used by commands that do not go through EnhancedExecutor
// (fresh/refresh, legacy rollback/migrate paths).
//
// The returned cleanup releases the lock and closes the connection. Callers
// must defer it immediately.
func acquireMigrationLock(cfg *config.Config) (func(), error) {
	sqlDB, err := database.GetSQLDB(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get SQL DB connection: %w", err)
	}

	lock := migration.NewMigrationLock(sqlDB, migration.DetectDialect(cfg.DatabaseURL))
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	if err := lock.AcquireLock(ctx); err != nil {
		cancel()
		sqlDB.Close()
		return nil, err
	}

	return func() {
		cancel()
		if err := lock.ReleaseLock(); err != nil {
			output.Warning("Failed to release migration lock: %v", err)
		}
		sqlDB.Close()
	}, nil
}
