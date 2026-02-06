package cmd

import (
	"fmt"

	"github.com/urfave/cli/v2"
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
		&cli.BoolFlag{
			Name:    "soft-delete",
			Aliases: []string{"sd"},
			Usage:   "add deleted_at column for soft delete support",
		},
	},
	Action: func(c *cli.Context) error {
		if c.NArg() < 1 {
			return fmt.Errorf("migration name is required. Usage: vc make migration <name>")
		}

		args := c.Args().Slice()
		if len(args) < 2 || args[0] != "migration" {
			return fmt.Errorf("usage: vc make migration <migration_name>")
		}

		migrationName := args[1]
		migrationPath := c.String("path")
		softDelete := c.Bool("soft-delete")

		// Create migration file
		if err := migration.CreateMigrationWithOptions(migrationName, migrationPath, migration.CreateMigrationOptions{
			SoftDelete: softDelete,
		}); err != nil {
			output.Error(err.Error())
			return err
		}

		output.Success("Migration created successfully")
		return nil
	},
}
