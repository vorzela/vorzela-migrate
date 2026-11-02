package main

import (
	"log"
	"os"

	"github.com/urfave/cli/v2"
	"github.com/vorzela/vorzela-migrate/cmd"
	"github.com/vorzela/vorzela-migrate/internal/output"
	"github.com/vorzela/vorzela-migrate/internal/version"
)

func main() {
	app := &cli.App{
		Name:    "vm",
		Usage:   "Vorzela Migrate - Database migration tool (inspired by Laravel migrations)",
		Version: version.CurrentVersion,
		Description: `Vorzela runs SQL migrations against PostgreSQL, MySQL, or MariaDB.

Agent/LLM quick start:
  vm init                    create .vm config
  vm lint                    validate .vm
  vm check --json            static safety report (no DB needed)
  vm check --db --json       checksums + missing files against the live DB
  vm status --json           executed/pending migrations
  vm make migration <name>   scaffold a migration
  vm migrate --yes           run pending migrations non-interactively

Use "vm check --strict" in CI to fail on destructive SQL and missing Down sections.`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "non-interactive",
				Usage: "never prompt; safe auto-run defaults apply (also VM_NON_INTERACTIVE=1)",
			},
			&cli.BoolFlag{
				Name:  "no-color",
				Usage: "disable ANSI colours (also VM_NO_COLOR=1)",
			},
		},
		Commands: []*cli.Command{
			cmd.InitCommand,
			cmd.CheckCommand,
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
		Before: func(c *cli.Context) error {
			v := os.Getenv("VM_NON_INTERACTIVE")
			if c.Bool("non-interactive") || (v != "" && v != "0" && v != "false") {
				output.SetNonInteractive(true)
			}
			if c.Bool("no-color") {
				output.DisableColor()
				_ = os.Setenv("VM_NO_COLOR", "1")
				_ = os.Setenv("NO_COLOR", "1")
			}
			return nil
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
