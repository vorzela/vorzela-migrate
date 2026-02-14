package cmd

import (
	"fmt"
	"strings"

	"github.com/urfave/cli/v2"
	"github.com/vorzela/vorzela-migrate/internal/config"
	"github.com/vorzela/vorzela-migrate/internal/migration"
	"github.com/vorzela/vorzela-migrate/internal/output"
)

var MakeCommand = &cli.Command{
	Name:  "make",
	Usage: "Create new migrations and other resources",
	Subcommands: []*cli.Command{
		{
			Name:      "migration",
			Usage:     "Create a new migration file",
			ArgsUsage: "<migration_name> [--soft-delete|-sd]",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "path",
					Aliases: []string{"p"},
					Usage:   "path to migrations directory (overrides .vm config)",
				},
				&cli.BoolFlag{
					Name:    "soft-delete",
					Aliases: []string{"sd"},
					Usage:   "add deleted_at column for soft delete support",
				},
				&cli.BoolFlag{
					Name:    "triggers",
					Aliases: []string{"t"},
					Usage:   "add trigger functions for automatic updated_at timestamp",
				},
			},
			Action: func(c *cli.Context) error {
				if c.NArg() < 1 {
					return fmt.Errorf("migration name is required. Usage: vm make migration <name> [--soft-delete|-sd]")
				}

				migrationName := c.Args().First()

				// Normalize migration name: add create_ prefix and _table suffix
				// Exception: trigger function files (start with "trigger_")
				if !strings.HasPrefix(migrationName, "trigger_") {
					// Add create_ or add_ prefix if missing
					if !strings.HasPrefix(migrationName, "create_") && !strings.HasPrefix(migrationName, "add_") {
						migrationName = "create_" + migrationName
					}

					// Add _table suffix if missing
					if !strings.HasSuffix(migrationName, "_table") {
						migrationName = migrationName + "_table"
					}
				}

				// Check for soft-delete flag (supports at beginning or end)
				softDelete := c.Bool("soft-delete")
				triggers := c.Bool("triggers")

				// Also manually check remaining args for flags
				// This handles flags placed after the migration name
				if !softDelete || !triggers {
					args := c.Args().Slice()
					for _, arg := range args {
						if arg == "-sd" || arg == "--soft-delete" || arg == "--sd" {
							softDelete = true
						}
						if arg == "-t" || arg == "--triggers" {
							triggers = true
						}
					}
				}

				// Only use path flag if explicitly set by user
				var pathOverride string
				if c.IsSet("path") {
					pathOverride = c.String("path")
				}

				// Load config to get MIGRATION_PATH from .vm if not specified via flag
				cfg, err := config.LoadConfig("", pathOverride)
				if err != nil {
					output.Error(err.Error())
					return err
				}

				migrationPath := cfg.MigrationPath

				// Create migration file
				if err := migration.CreateMigrationWithOptions(migrationName, migrationPath, migration.CreateMigrationOptions{
					SoftDelete: softDelete,
					Triggers:   triggers,
				}); err != nil {
					output.Error(err.Error())
					return err
				}

				// Build success message
				var features []string
				if softDelete {
					features = append(features, "soft delete")
				}
				if triggers {
					features = append(features, "auto-update triggers")
				}

				if len(features) > 0 {
					output.Success(fmt.Sprintf("Migration created successfully (with %s)", strings.Join(features, " + ")))
				} else {
					output.Success("Migration created successfully")
				}
				return nil
			},
		},
	},
}
