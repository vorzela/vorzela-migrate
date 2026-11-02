package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
	"github.com/vorzela/vorzela-migrate/internal/config"
	"github.com/vorzela/vorzela-migrate/internal/database"
	"github.com/vorzela/vorzela-migrate/internal/migration"
	"github.com/vorzela/vorzela-migrate/internal/output"
)

var StatusCommand = &cli.Command{
	Name:  "status",
	Usage: "Show migration status",
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
			Name:  "json",
			Usage: "emit machine-readable JSON",
		},
	},
	Action: func(c *cli.Context) error {
		cfg, err := config.LoadConfig(c.String("dsn"), c.String("path"))
		if err != nil {
			output.Error(err.Error())
			return err
		}
		if err := cfg.Validate(); err != nil {
			output.Error(err.Error())
			return err
		}

		conn, err := database.Connect(cfg.DatabaseURL)
		if err != nil {
			output.Error("Failed to connect to database: %v", err)
			return fmt.Errorf("failed to connect to database: %w", err)
		}
		defer conn.Close()

		// Status should work on a fresh database too.
		if err := migration.InitMigrationTable(conn, cfg.DatabaseURL); err != nil {
			output.Error("Failed to initialize migration table: %v", err)
			return fmt.Errorf("failed to initialize migration table: %w", err)
		}

		if c.Bool("json") {
			report, err := migration.GetStatus(conn, cfg.MigrationPath, cfg.DatabaseURL)
			if err != nil {
				output.Error("Failed to get status: %v", err)
				return err
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(report)
		}

		if err := migration.ShowStatus(conn, cfg.MigrationPath, cfg.DatabaseURL); err != nil {
			output.Error("Failed to show status: %v", err)
			return fmt.Errorf("failed to show status: %w", err)
		}
		return nil
	},
}
