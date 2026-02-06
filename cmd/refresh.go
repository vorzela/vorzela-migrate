package cmd

import (
	"fmt"

	"github.com/urfave/cli/v2"
	"github.com/vorzela/vorzela-migrate/internal/config"
	"github.com/vorzela/vorzela-migrate/internal/database"
	"github.com/vorzela/vorzela-migrate/internal/migration"
	"github.com/vorzela/vorzela-migrate/internal/output"
)

var RefreshCommand = &cli.Command{
	Name:  "refresh",
	Usage: "Rollback all migrations and re-run them",
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

		// Connect to database
		db, err := database.Connect(cfg.DatabaseURL)
		if err != nil {
			output.Error("Failed to connect to database: %v", err)
			return fmt.Errorf("failed to connect to database: %w", err)
		}
		defer db.Close()

		// Initialize migration table
		if err := migration.InitMigrationTable(db, cfg.DatabaseURL); err != nil {
			output.Error("Failed to initialize migration table: %v", err)
			return fmt.Errorf("failed to initialize migration table: %w", err)
		}

		output.Info("Rolling back all migrations...")
		rollbackCount, err := migration.RollbackAllMigrations(db, cfg.MigrationPath)
		if err != nil {
			output.Error("Rollback failed: %v", err)
			return fmt.Errorf("rollback failed: %w", err)
		}
		output.Success("Rolled back %d migration(s)", rollbackCount)

		output.Info("Running all migrations...")
		migrateCount, err := migration.RunMigrations(db, cfg.MigrationPath)
		if err != nil {
			output.Error("Migration failed: %v", err)
			return fmt.Errorf("migration failed: %w", err)
		}
		output.Success("Ran %d migration(s)", migrateCount)

		return nil
	},
}
