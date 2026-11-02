package cmd

import (
	"time"

	"github.com/vorzela/vorzela-migrate/internal/config"
	"github.com/vorzela/vorzela-migrate/internal/database"
	"github.com/vorzela/vorzela-migrate/internal/db"
	"github.com/vorzela/vorzela-migrate/internal/migration"
	"github.com/vorzela/vorzela-migrate/internal/output"
)

// rollbackAllMigrationsEnhanced rolls back every executed migration using the
// enhanced executor (atomic per-migration PostgreSQL Down sections).
func rollbackAllMigrationsEnhanced(cfg *config.Config, conn db.DB) (int, time.Duration, error) {
	sqlDB, err := database.GetSQLDB(cfg.DatabaseURL)
	if err != nil {
		return 0, 0, err
	}
	defer sqlDB.Close()

	opts := migration.MigrationOptions{
		Verbose:        cfg.Verbose,
		SkipLock:       true,
		Force:          true,
		NonInteractive: !output.IsInteractive(),
	}
	executor, err := migration.NewEnhancedExecutor(conn, sqlDB, cfg.DatabaseURL, cfg.MigrationPath, opts)
	if err != nil {
		return 0, 0, err
	}
	return executor.RollbackAll(opts)
}

// runPendingMigrationsEnhanced runs pending migrations through the enhanced
// executor without acquiring the lock (fresh/refresh already hold it). This
// gives PostgreSQL atomic per-migration transactions and stores checksums.
func runPendingMigrationsEnhanced(cfg *config.Config, conn db.DB) (int, time.Duration, error) {
	sqlDB, err := database.GetSQLDB(cfg.DatabaseURL)
	if err != nil {
		return 0, 0, err
	}
	defer sqlDB.Close()

	opts := migration.MigrationOptions{
		Force:          true, // history was just rolled back; avoid stale checksum noise
		Online:         cfg.Online,
		Verbose:        cfg.Verbose,
		SkipLock:       true,
		DriftHandling:  cfg.DriftHandling,
		NonInteractive: !output.IsInteractive(),
	}
	executor, err := migration.NewEnhancedExecutor(conn, sqlDB, cfg.DatabaseURL, cfg.MigrationPath, opts)
	if err != nil {
		return 0, 0, err
	}

	start := time.Now()
	results, err := executor.RunMigrationsEnhanced(opts)
	count := 0
	for _, r := range results {
		if r.Success {
			count++
		}
	}
	return count, time.Since(start), err
}
