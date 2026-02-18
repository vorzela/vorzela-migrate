package cmd

import (
	"fmt"

	"github.com/urfave/cli/v2"
	"github.com/vorzela/vorzela-migrate/internal/config"
	"github.com/vorzela/vorzela-migrate/internal/database"
	"github.com/vorzela/vorzela-migrate/internal/migration"
	"github.com/vorzela/vorzela-migrate/internal/output"
)

var RollbackCommand = &cli.Command{
	Name:  "rollback",
	Usage: "Rollback the last migration batch",
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
		&cli.StringFlag{
			Name:    "steps",
			Aliases: []string{"step", "n"},
			Value:   "1",
			Usage:   "number of batches to rollback: 1, 2, or 'all'",
		},
		&cli.BoolFlag{
			Name:    "enhanced",
			Aliases: []string{"e"},
			Usage:   "use enhanced rollback with warnings and confirmations",
			Value:   false,
		},
		&cli.BoolFlag{
			Name:  "force",
			Usage: "skip confirmation prompts",
			Value: false,
		},
		&cli.BoolFlag{
			Name:  "dry-run",
			Usage: "show what would be rolled back without executing",
			Value: false,
		},
		&cli.BoolFlag{
			Name:    "verbose",
			Aliases: []string{"v"},
			Usage:   "enable verbose logging",
			Value:   false,
		},
	},
	Action: func(c *cli.Context) error {
		dsn := c.String("dsn")
		migrationPath := c.String("path")
		stepsStr := c.String("steps")

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

		// Connect to database
		db, err := database.Connect(cfg.DatabaseURL)
		if err != nil {
			output.Error("Failed to connect to database: %v", err)
			return fmt.Errorf("failed to connect to database: %w", err)
		}
		defer db.Close()

		// Check if enhanced mode is requested
		useEnhanced := c.Bool("enhanced") || c.Bool("dry-run")

		if useEnhanced {
			// Use enhanced executor with warnings
			sqlDB, err := database.GetSQLDB(cfg.DatabaseURL)
			if err != nil {
				output.Error("Failed to get SQL DB connection: %v", err)
				return fmt.Errorf("failed to get SQL DB connection: %w", err)
			}
			defer sqlDB.Close()

			opts := migration.MigrationOptions{
				DryRun:  c.Bool("dry-run"),
				Force:   c.Bool("force"),
				Verbose: c.Bool("verbose"),
			}

			executor, err := migration.NewEnhancedExecutor(db, sqlDB, cfg.DatabaseURL, cfg.MigrationPath, opts)
			if err != nil {
				output.Error("Failed to create enhanced executor: %v", err)
				return err
			}

			// Parse steps
			steps := 1
			if stepsStr != "all" {
				fmt.Sscanf(stepsStr, "%d", &steps)
				if steps < 1 {
					steps = 1
				}
			} else {
				steps = 999 // Large number to rollback all
			}

			results, err := executor.RollbackWithWarnings(steps, opts)
			if err != nil && !c.Bool("force") {
				return err
			}

			if c.Bool("dry-run") {
				output.Info("DRY RUN completed - no rollbacks were actually executed")
			}

			if len(results) == 0 {
				output.Info("No migrations to rollback")
			}

			return nil
		}

		// Use standard rollback (legacy mode)
		var count int
		if stepsStr == "all" {
			count, err = migration.RollbackAllMigrations(db, cfg.MigrationPath, cfg.DatabaseURL)
			if err != nil {
				output.Error("Rollback failed: %v", err)
				return fmt.Errorf("rollback failed: %w", err)
			}
		} else {
			steps := 1
			fmt.Sscanf(stepsStr, "%d", &steps)
			if steps < 1 {
				steps = 1
			}

			count, err = migration.RollbackMigrations(db, cfg.MigrationPath, steps, cfg.DatabaseURL)
			if err != nil {
				output.Error("Rollback failed: %v", err)
				return fmt.Errorf("rollback failed: %w", err)
			}
		}

		if count == 0 {
			output.Info("No migrations to rollback")
		} else {
			output.Success("Successfully rolled back %d migration(s)", count)
		}

		return nil
	},
}
