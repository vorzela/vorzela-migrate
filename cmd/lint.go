package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v2"
	"github.com/vorzela/vorzela-migrate/v2/internal/config"
	"github.com/vorzela/vorzela-migrate/v2/internal/output"
)

var LintCommand = &cli.Command{
	Name:  "lint",
	Usage: "Validate the vm.yaml/vm.yml configuration file for errors and warnings",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "file",
			Aliases: []string{"f"},
			Usage:   "path to the config file to lint (defaults to vm.yaml/vm.yml in current or parent directories)",
		},
	},
	Action: func(c *cli.Context) error {
		// Resolve path
		vmPath := c.String("file")
		if vmPath == "" {
			vmPath = findVMFile()
		}

		if vmPath == "" {
			output.Warning("No vm.yaml, vm.yml or .vm file found in the current or parent directories")
			output.Info("Create one with `vm init` (vm.yaml) or copy vm.example.yaml to get started")
			return nil
		}

		absPath, _ := filepath.Abs(vmPath)
		output.Info("Linting %s", absPath)

		issues, err := config.LintVMFile(vmPath)
		if err != nil {
			output.Error("Failed to read file: %v", err)
			return err
		}

		if len(issues) == 0 {
			output.Success("No issues found")
			return nil
		}

		errors := 0
		warnings := 0
		for _, iss := range issues {
			loc := ""
			if iss.Line > 0 {
				loc = fmt.Sprintf("line %d  ", iss.Line)
			}
			switch iss.Severity {
			case config.LintError:
				errors++
				fmt.Fprintf(os.Stderr, "  ✗ %s[%s] %s\n", loc, iss.Key, iss.Message)
			case config.LintWarning:
				warnings++
				fmt.Printf("  ⚠ %s[%s] %s\n", loc, iss.Key, iss.Message)
			}
		}

		fmt.Println()
		if errors > 0 && warnings > 0 {
			output.Error("Found %d error(s) and %d warning(s)", errors, warnings)
		} else if errors > 0 {
			output.Error("Found %d error(s)", errors)
		} else {
			output.Warning("Found %d warning(s)", warnings)
		}

		if errors > 0 {
			return fmt.Errorf("lint failed with %d error(s)", errors)
		}
		return nil
	},
}

// findVMFile searches the current directory and parents for a config file.
func findVMFile() string {
	return config.FindConfigFile()
}
