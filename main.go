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
			cmd.ExtensionsCommand,
			cmd.EnumsCommand,
			cmd.LintCommand,
			cmd.MigrateCommand,
			cmd.RollbackCommand,
			cmd.FreshCommand,
			cmd.RefreshCommand,
			cmd.StatusCommand,
			cmd.UpgradeCommand,
			cmd.UninstallCommand,
		},
		After: func(c *cli.Context) error {
			// In urfave/cli/v2 the app-level After hook context does not expose
			// the invoked subcommand name via c.Command, so we inspect os.Args directly.
			// Skip the version notice after upgrade/uninstall — they handle their own output.
			skip := len(os.Args) > 1 && (os.Args[1] == "upgrade" || os.Args[1] == "uninstall")
			if !skip {
				version.PrintVersionNotice()
			}
			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
