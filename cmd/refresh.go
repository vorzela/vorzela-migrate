package cmd

import (
	"fmt"
	"time"

	"github.com/urfave/cli/v2"
	"github.com/vorzela/vorzela-migrate/internal/config"
	"github.com/vorzela/vorzela-migrate/internal/database"
	"github.com/vorzela/vorzela-migrate/internal/migration"
	"github.com/vorzela/vorzela-migrate/internal/output"
)

var RefreshCommand = &cli.Command{
	Name:  "refresh",
	Usage: "Roll back all migrations (Down) then re-run all migrations",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "dsn",
			Aliases: []string{"d"},
			Usage:   "database connection string",
			EnvVars: []string{"DATABASE_URL"},
		},
		&cli.StringFlag{
			Name:    "path",
			Aliases: []string{"p"},
			Value:   "./migrations",
			Usage:   "path to migrations directory",
		},
		&cli.BoolFlag{
			Name:  "force",
			Usage: "skip confirmation prompt",
			Value: false,
		},
	},
	Action: func(c *cli.Context) error {
		dsn := c.String("dsn")
		migrationPath := c.String("path")

		// Load config with overrides
		cfg, err := config.LoadConfig(dsn, migrationPath)
		if err != nil {
			output.Error(err.Error())
			return err
		}

		// Validate config
		if err := cfg.Validate(); err != nil {
			output.Error(err.Error())
			return err
		}

		if report := migration.CheckMigrationFiles(cfg.MigrationPath); report.HasErrors() {
			output.Error("Migration safety check failed: %d error(s); run 'vm check' for details", report.Errors())
			return fmt.Errorf("migration safety check failed")
		}

		// Confirmation prompt
		if !c.Bool("force") {
			output.Warning("This will roll back all migrations and re-run every migration. Data will be lost!")
			if !output.ConfirmDefault("Are you sure you want to continue?", false) {
				output.Info("Refresh cancelled.")
				return nil
			}
		}

		// Connect to database
		db, err := database.Connect(cfg.DatabaseURL)
		if err != nil {
			output.Error("Failed to connect to database: %v", err)
			return fmt.Errorf("failed to connect to database: %w", err)
		}
		defer db.Close()

		// Hold the migration lock for the whole destructive rebuild.
		releaseLock, err := acquireMigrationLock(cfg)
		if err != nil {
			output.Error("Failed to acquire migration lock: %v", err)
			return err
		}
		defer releaseLock()

		// Initialize migration table
		if err := migration.InitMigrationTable(db, cfg.DatabaseURL); err != nil {
			output.Error("Failed to initialize migration table: %v", err)
			return fmt.Errorf("failed to initialize migration table: %w", err)
		}

		overall := time.Now()

		output.Info("Dropping all migrations...")
		dropCount, dropDur, err := rollbackAllMigrationsEnhanced(cfg, db)
		if err != nil {
			output.Error("Drop failed: %v", err)
			return fmt.Errorf("drop failed: %w", err)
		}
		output.Success("Dropped %d migration(s) in %s", dropCount, dropDur.Round(time.Millisecond))

		if err := runAutoPrerequisites(c, &cfg, &db); err != nil {
			return err
		}

		output.Info("Running all migrations...")
		migrateCount, migrateDur, err := runPendingMigrationsEnhanced(cfg, db)
		if err != nil {
			output.Error("Migration failed: %v", err)
			return fmt.Errorf("migration failed: %w", err)
		}
		output.Success("Ran %d migration(s) in %s", migrateCount, migrateDur.Round(time.Millisecond))

		output.Success("✨ Refresh completed in %s", time.Since(overall).Round(time.Millisecond))
		return nil
	},
}
