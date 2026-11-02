package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v2"
	"github.com/vorzela/vorzela-migrate/v2/internal/output"
)

// InitCommand creates a .vm config file. It never overwrites an existing file
// unless --force is passed, so agents can run it idempotently.
var InitCommand = &cli.Command{
	Name:  "init",
	Usage: "Create a .vm configuration file in the current directory",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "database-url",
			Usage: "DATABASE_URL to write into .vm",
			Value: "postgres://user:password@localhost:5432/myapp",
		},
		&cli.StringFlag{
			Name:    "path",
			Aliases: []string{"p"},
			Usage:   "migrations directory to write into .vm",
			Value:   "./migrations",
		},
		&cli.StringFlag{
			Name:  "environment",
			Usage: "development or production",
			Value: "development",
		},
		&cli.BoolFlag{
			Name:    "force",
			Aliases: []string{"f"},
			Usage:   "overwrite an existing .vm file",
		},
	},
	Action: func(c *cli.Context) error {
		file := ".vm"
		if _, err := os.Stat(file); err == nil && !c.Bool("force") {
			output.Warning(".vm already exists; use --force to overwrite")
			return nil
		}

		content := fmt.Sprintf(`# Vorzela Migration Configuration (.vm)
# Validate with: vm lint
# Full reference: LLM.md
DATABASE_URL=%s
ENVIRONMENT=%s
MIGRATION_PATH=%s
DRIFT_HANDLING=prompt
`, c.String("database-url"), c.String("environment"), c.String("path"))

		if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
			output.Error("Failed to write .vm: %v", err)
			return err
		}

		// Make the configured migration directory so `vm status`/`vm check`
		// don't look broken before the first `vm make migration`.
		dir := c.String("path")
		if dir == "" {
			dir = "./migrations"
		}
		if err := os.MkdirAll(filepath.Clean(dir), 0o755); err != nil {
			output.Warning("Could not create migrations directory %s: %v", dir, err)
		}

		abs, _ := filepath.Abs(file)
		output.Success("Created %s", abs)
		output.Info("Next: vm lint && vm make migration users")
		return nil
	},
}
