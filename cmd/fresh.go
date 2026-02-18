package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
	"github.com/vorzela/vorzela-migrate/internal/config"
	"github.com/vorzela/vorzela-migrate/internal/database"
	"github.com/vorzela/vorzela-migrate/internal/migration"
	"github.com/vorzela/vorzela-migrate/internal/output"
)

var FreshCommand = &cli.Command{
	Name:  "fresh",
	Usage: "Rollback all migrations and re-run them (with confirmation)",
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
			Usage: "skip confirmation prompt and run fresh immediately",
		},
	},
	Action: func(c *cli.Context) error {
		dsn := c.String("dsn")
		migrationPath := c.String("path")
		force := c.Bool("force")

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

		// Show warnings
		output.Warning("⚠️  CAUTION: This will rollback ALL migrations and re-run them")
		output.Warning("⚠️  This may cause data loss in your database!")
		output.Info("Database: %s", cfg.DatabaseURL)

		// Confirm with user if not force flag
		if !force {
			reader := bufio.NewReader(os.Stdin)
			output.Info("\nDo you want to continue? (yes/no): ")
			response, err := reader.ReadString('\n')
			if err != nil {
				output.Error("Failed to read input: %v", err)
				return fmt.Errorf("failed to read input: %w", err)
			}

			response = strings.TrimSpace(strings.ToLower(response))
			if response != "yes" && response != "y" {
				output.Info("Fresh operation cancelled")
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

		// Initialize migration table
		if err := migration.InitMigrationTable(db, cfg.DatabaseURL); err != nil {
			output.Error("Failed to initialize migration table: %v", err)
			return fmt.Errorf("failed to initialize migration table: %w", err)
		}

		// Rollback all migrations
		output.Warning("\nDropping all migrations...")
		rollbackCount, dropDur, err := migration.RollbackAllMigrations(db, cfg.MigrationPath, cfg.DatabaseURL, "Dropped")
		if err != nil {
			output.Error("Drop failed: %v", err)
			return fmt.Errorf("drop failed: %w", err)
		}
		output.Success("Dropped %d migration(s) in %s", rollbackCount, dropDur.Round(time.Millisecond))

		// Re-run all migrations
		output.Warning("Re-running all migrations...")
		migrateCount, migrateDur, err := migration.RunMigrations(db, cfg.MigrationPath, cfg.DatabaseURL)
		if err != nil {
			output.Error("Migration failed: %v", err)
			return fmt.Errorf("migration failed: %w", err)
		}
		output.Success("Ran %d migration(s) in %s", migrateCount, migrateDur.Round(time.Millisecond))

		output.Success("\n✨ Fresh completed successfully!")
		return nil
	},
}
