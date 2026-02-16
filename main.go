package main

import (
	"log"
	"os"

	"github.com/urfave/cli/v2"
	"github.com/vorzela/vorzela-migrate/cmd"
	"github.com/vorzela/vorzela-migrate/internal/version"
)

func main() {
	app := &cli.App{
		Name:    "vm",
		Usage:   "Vorzela Migrate - Database migration tool (inspired by Laravel migrations)",
		Version: version.CurrentVersion,
		Commands: []*cli.Command{
			cmd.MakeCommand,
			cmd.FunctionsCommand,
			cmd.MigrateCommand,
			cmd.RollbackCommand,
			cmd.FreshCommand,
			cmd.RefreshCommand,
			cmd.StatusCommand,
			cmd.UpgradeCommand,
		},
		After: func(c *cli.Context) error {
			// Skip version notice for upgrade command (it handles its own messaging)
			if c.Command.Name != "upgrade" {
				version.PrintVersionNotice()
			}
			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
