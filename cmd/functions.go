package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v2"
	"github.com/vorzela/vorzela-migrate/internal/config"
	"github.com/vorzela/vorzela-migrate/internal/database"
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

				// Execute the SQL
				output.Info(fmt.Sprintf("Applying functions from %s...", functionsPath))
				ctx := context.Background()
				if err := db.Exec(ctx, string(sqlContent)); err != nil {
					output.Error(fmt.Sprintf("Failed to apply functions: %v", err))
					return err
				}

				output.Success("Database functions installed successfully")
				output.Info("✓ auto_update_timestamp()")
				output.Info("✓ protect_soft_deleted()")
				output.Info("✓ auto_update_with_soft_delete_protection()")
				output.Info("✓ prevent_hard_delete()")

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
