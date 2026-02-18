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

var RefreshCommand = &cli.Command{
	Name:  "refresh",
	Usage: "Drop all tables (via DOWN migrations) then re-run all migrations",
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

		// Confirmation prompt
		if !c.Bool("force") {
			output.Warning("⚠️  This will DROP all tables and re-run every migration. Data will be lost!")
			fmt.Print("Are you sure you want to continue? (yes/no): ")
			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer != "yes" && answer != "y" {
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

		// Initialize migration table
		if err := migration.InitMigrationTable(db, cfg.DatabaseURL); err != nil {
			output.Error("Failed to initialize migration table: %v", err)
			return fmt.Errorf("failed to initialize migration table: %w", err)
		}

		overall := time.Now()

		output.Info("Dropping all migrations...")
		dropCount, dropDur, err := migration.RollbackAllMigrations(db, cfg.MigrationPath, cfg.DatabaseURL, "Dropped")
		if err != nil {
			output.Error("Drop failed: %v", err)
			return fmt.Errorf("drop failed: %w", err)
		}
		output.Success("Dropped %d migration(s) in %s", dropCount, dropDur.Round(time.Millisecond))

		output.Info("Running all migrations...")
		migrateCount, migrateDur, err := migration.RunMigrations(db, cfg.MigrationPath, cfg.DatabaseURL)
		if err != nil {
			output.Error("Migration failed: %v", err)
			return fmt.Errorf("migration failed: %w", err)
		}
		output.Success("Ran %d migration(s) in %s", migrateCount, migrateDur.Round(time.Millisecond))

		output.Success("✨ Refresh completed in %s", time.Since(overall).Round(time.Millisecond))
		return nil
	},
}
