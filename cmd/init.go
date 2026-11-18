package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v2"
	"github.com/vorzela/vorzela-migrate/v2/internal/config"
	"github.com/vorzela/vorzela-migrate/v2/internal/output"
)

// InitCommand creates a vm.yaml config file. It never overwrites an existing
// config unless --force is passed, so agents can run it idempotently.
var InitCommand = &cli.Command{
	Name:  "init",
	Usage: "Create a vm.yaml configuration file in the current directory",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "file",
			Usage: "config filename to create: vm.yaml or vm.yml",
			Value: "vm.yaml",
		},
		&cli.StringFlag{
			Name:  "database-url",
			Usage: "DATABASE_URL to write into the config file",
			Value: "postgres://user:password@localhost:5432/myapp",
		},
		&cli.StringFlag{
			Name:    "path",
			Aliases: []string{"p"},
			Usage:   "migrations directory to write into the config file",
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
			Usage:   "overwrite the existing config file",
		},
	},
	Action: func(c *cli.Context) error {
		file := strings.TrimSpace(c.String("file"))
		if file == "" {
			file = "vm.yaml"
		}
		if file != filepath.Base(file) {
			return fmt.Errorf("--file must be a filename in the current directory")
		}
		if file != "vm.yaml" && file != "vm.yml" {
			return fmt.Errorf("--file must be vm.yaml or vm.yml")
		}
		if found := config.FindConfigFileInDir("."); found != "" && !c.Bool("force") {
			output.Warning("%s already exists; use --force to overwrite", found)
			return nil
		}

		content := fmt.Sprintf(`# Vorzela Migration Configuration (%s)
# Validate with: vm lint
# Full reference: LLM.md
database_url: %s
environment: %s
migration_path: %s
drift_handling: prompt

# Optional overrides:
# enhanced: true
# online: false
# verify_checksums: true
# detect_drift: true
# verbose: false
# strict: false
# sqlc_support: false
# auto_run_extensions: true
# auto_run_functions: true
# auto_run_enums: true
`, file,
			config.QuoteYAMLValue(c.String("database-url")),
			config.QuoteYAMLValue(c.String("environment")),
			config.QuoteYAMLValue(c.String("path")))

		if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
			output.Error("Failed to write %s: %v", file, err)
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
