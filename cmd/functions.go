package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v2"
	"github.com/vorzela/vorzela-migrate/internal/config"
	"github.com/vorzela/vorzela-migrate/internal/database"
	"github.com/vorzela/vorzela-migrate/internal/migration"
	"github.com/vorzela/vorzela-migrate/internal/output"
)

var FunctionsCommand = &cli.Command{
	Name:  "functions",
	Usage: "Manage database functions",
	Subcommands: []*cli.Command{
		{
			Name:  "migrate",
			Usage: "Install common database functions from functions.sql",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "dsn",
					Aliases: []string{"d"},
					Usage:   "database connection string",
				},
				&cli.StringFlag{
					Name:    "path",
					Aliases: []string{"p"},
					Value:   "./migrations",
					Usage:   "path to migrations directory",
				},
			},
			Action: func(c *cli.Context) error {
				// Load configuration
				cfg, err := config.LoadConfig(c.String("dsn"), c.String("path"))
				if err != nil {
					output.Error(err.Error())
					return err
				}

				// Functions use PL/pgSQL — PostgreSQL only
				if d := migration.DetectDialect(cfg.DatabaseURL); d != migration.PostgreSQL {
					return fmt.Errorf("vm functions is not supported on %s — PL/pgSQL trigger functions are a PostgreSQL-only feature", d)
				}

				// Connect to database
				db, err := database.Connect(cfg.DatabaseURL)
				if err != nil {
					output.Error(fmt.Sprintf("Failed to connect: %v", err))
					return err
				}
				defer db.Close()

				// Read functions.sql file
				functionsPath := filepath.Join(cfg.MigrationPath, "functions.sql")
				sqlContent, err := os.ReadFile(functionsPath)
				if err != nil {
					output.Error(fmt.Sprintf("Failed to read %s: %v", functionsPath, err))
					output.Info("Tip: The functions.sql file should be in your configured migration path")
					return err
				}

				content := string(sqlContent)
				enabled, disabled := migration.ParseAllFunctionNames(content)

				ctx := context.Background()

				// ── Install / update enabled functions (CREATE OR REPLACE handles idempotency) ──
				output.Info(fmt.Sprintf("Syncing functions from %s...", functionsPath))
				if err := db.Exec(ctx, content); err != nil {
					output.Error(fmt.Sprintf("Failed to apply functions: %v", err))
					return err
				}
				for _, fn := range enabled {
					output.Success(fmt.Sprintf("✓ %s()", fn))
				}

				// ── Drop disabled / commented-out functions ───────────────────────
				droppedCount := 0
				for _, fn := range disabled {
					dropSQL := fmt.Sprintf("DROP FUNCTION IF EXISTS %s() CASCADE;", fn)
					if err := db.Exec(ctx, dropSQL); err != nil {
						output.Warning(fmt.Sprintf("Failed to drop function %s: %v", fn, err))
						continue
					}
					output.Info(fmt.Sprintf("↓ Removed disabled function: %s()", fn))
					droppedCount++
				}

				output.Success(fmt.Sprintf("Function sync complete (enabled: %d, removed: %d)", len(enabled), droppedCount))
				return nil
			},
		},
		{
			Name:  "drop",
			Usage: "Drop common database functions",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "dsn",
					Aliases: []string{"d"},
					Usage:   "database connection string",
				},
				&cli.StringFlag{
					Name:    "path",
					Aliases: []string{"p"},
					Value:   "./migrations",
					Usage:   "path to migrations directory",
				},
				&cli.BoolFlag{
					Name:    "step",
					Aliases: []string{"s"},
					Usage:   "drop functions one by one with confirmation",
				},
			},
			Action: func(c *cli.Context) error {
				// Load configuration
				cfg, err := config.LoadConfig(c.String("dsn"), c.String("path"))
				if err != nil {
					output.Error(err.Error())
					return err
				}

				// Connect to database
				db, err := database.Connect(cfg.DatabaseURL)
				if err != nil {
					output.Error(fmt.Sprintf("Failed to connect: %v", err))
					return err
				}
				defer db.Close()

				ctx := context.Background()
				stepMode := c.Bool("step")

				// List of functions to drop
				functions := []string{
					"auto_update_timestamp",
					"protect_soft_deleted",
					"auto_update_with_soft_delete_protection",
					"prevent_hard_delete",
				}

				if stepMode {
					output.Info("Dropping functions step by step...")
					for _, fn := range functions {
						fmt.Printf("Drop function %s? [y/N]: ", fn)
						var response string
						fmt.Scanln(&response)
						if response != "y" && response != "Y" {
							output.Info(fmt.Sprintf("Skipped: %s", fn))
							continue
						}

						dropSQL := fmt.Sprintf("DROP FUNCTION IF EXISTS %s() CASCADE;", fn)
						if err := db.Exec(ctx, dropSQL); err != nil {
							output.Error(fmt.Sprintf("Failed to drop %s: %v", fn, err))
							continue
						}
						output.Success(fmt.Sprintf("Dropped: %s", fn))
					}
				} else {
					output.Info("Dropping all common functions...")
					for _, fn := range functions {
						dropSQL := fmt.Sprintf("DROP FUNCTION IF EXISTS %s() CASCADE;", fn)
						if err := db.Exec(ctx, dropSQL); err != nil {
							output.Error(fmt.Sprintf("Failed to drop %s: %v", fn, err))
							continue
						}
						output.Info(fmt.Sprintf("✓ Dropped: %s", fn))
					}
					output.Success("All functions dropped successfully")
				}

				return nil
			},
		},
	},
}
