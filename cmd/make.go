package cmd

import (
	"fmt"

	"github.com/urfave/cli/v2"
	"github.com/vorzela/vorzela-migrate/internal/config"
	"github.com/vorzela/vorzela-migrate/internal/migration"
	"github.com/vorzela/vorzela-migrate/internal/output"
)

var MakeCommand = &cli.Command{
	Name:      "make",
	Usage:     "Create a new migration file",
	ArgsUsage: "<migration_name>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "path",
			Aliases: []string{"p"},
			Value:   "./migrations",
			Usage:   "path to migrations directory",
		},
		&cli.StringFlag{
			Name:  "dialect",
			Usage: "database dialect for template (postgres|mysql|cassandra)",
		},
		&cli.BoolFlag{
			Name:    "soft-delete",
			Aliases: []string{"sd"},
			Usage:   "add deleted_at column for soft delete support",
		},
	},
	Action: func(c *cli.Context) error {
		if c.NArg() < 1 {
			return fmt.Errorf("migration name is required. Usage: vm make migration <name>")
		}

		args := c.Args().Slice()
		if len(args) < 2 || args[0] != "migration" {
			return fmt.Errorf("usage: vm make migration <migration_name>")
		}

		migrationName := args[1]
		dialect := c.String("dialect")
		softDelete := c.Bool("soft-delete")

		// Only use path flag if explicitly set by user
		var pathOverride string
		if c.IsSet("path") {
			pathOverride = c.String("path")
		}

		// Load config to get MIGRATION_PATH from .vorzela if not specified via flag
		cfg, err := config.LoadConfig("", pathOverride)
		if err != nil {
			output.Error(err.Error())
			return err
		}

		migrationPath := cfg.MigrationPath

		// Create migration file
		if err := migration.CreateMigrationWithOptions(migrationName, migrationPath, migration.CreateMigrationOptions{
			SoftDelete: softDelete,
			Dialect:    dialect,
		}); err != nil {
			output.Error(err.Error())
			return err
		}

		output.Success("Migration created successfully")
		return nil
	},
}
