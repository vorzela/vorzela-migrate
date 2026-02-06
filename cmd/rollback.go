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
			Name:  "steps",
			Value: "1",
			Usage: "number of batches to rollback: 1, 2, or 'all'",
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

		// Parse steps (1, 2, or "all")
		var count int
		if stepsStr == "all" {
			// Rollback all migrations
			count, err = migration.RollbackAllMigrations(db, cfg.MigrationPath, cfg.DatabaseURL)
			if err != nil {
				output.Error("Rollback failed: %v", err)
				return fmt.Errorf("rollback failed: %w", err)
			}
		} else {
			// Parse numeric steps
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
