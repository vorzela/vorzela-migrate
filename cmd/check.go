package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v2"
	"github.com/vorzela/vorzela-migrate/internal/config"
	"github.com/vorzela/vorzela-migrate/internal/database"
	"github.com/vorzela/vorzela-migrate/internal/migration"
	"github.com/vorzela/vorzela-migrate/internal/output"
)

var CheckCommand = &cli.Command{
	Name:    "check",
	Aliases: []string{"validate", "safety"},
	Usage:   "Run static safety checks on migrations and .vm config",
	Description: `Examples:
  vm check                         # fast, no database connection
  vm check --strict                # warnings (destructive SQL) also fail
  vm check --db                    # also compare checksums with the live DB
  vm check --json                  # machine-readable report for agents/CI

Exit codes: 0 = clean (warnings allowed unless --strict), 1 = errors or strict failure.`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "dsn",
			Aliases: []string{"d"},
			Usage:   "database connection string (used with --db)",
			EnvVars: []string{"DATABASE_URL"},
		},
		&cli.StringFlag{
			Name:    "path",
			Aliases: []string{"p"},
			Value:   "./migrations",
			Usage:   "path to migrations directory",
		},
		&cli.BoolFlag{
			Name:  "db",
			Usage: "connect to the database and verify executed migrations/checksums",
		},
		&cli.BoolFlag{
			Name:  "strict",
			Usage: "treat warnings as errors",
		},
		&cli.BoolFlag{
			Name:  "allow-destructive",
			Usage: "do not fail strict mode for destructive SQL warnings",
		},
		&cli.BoolFlag{
			Name:  "json",
			Usage: "emit a machine-readable JSON report",
		},
	},
	Action: func(c *cli.Context) error {
		cfg, err := config.LoadConfig(c.String("dsn"), c.String("path"))
		if err != nil {
			return err
		}

		report := migration.CheckMigrationFiles(cfg.MigrationPath)

		// Lint the .vm file too: a typo in a safety key is exactly the kind of
		// problem this command should catch.
		if vmPath := findVMFile(); vmPath != "" {
			issues, lerr := config.LintVMFile(vmPath)
			if lerr != nil {
				report.Add(migration.SafetyIssue{
					File:     vmPath,
					Code:     "config_read",
					Severity: migration.SafetyError,
					Message:  lerr.Error(),
				})
			}
			for _, issue := range issues {
				severity := migration.SafetyWarning
				if issue.Severity == config.LintError {
					severity = migration.SafetyError
				}
				report.Add(migration.SafetyIssue{
					File:     vmPath,
					Line:     issue.Line,
					Code:     "config_" + strings.ToLower(string(issue.Severity)),
					Severity: severity,
					Message:  issue.Key + ": " + issue.Message,
				})
			}
		}

		if c.Bool("db") {
			if cfg.DatabaseURL == "" {
				report.Add(migration.SafetyIssue{
					Code:     "database_url_missing",
					Severity: migration.SafetyError,
					Message:  "DATABASE_URL is not set; cannot run --db checks",
				})
			} else {
				checkDatabaseSafety(report, cfg)
			}
		}

		report.Sort()

		strict := c.Bool("strict") || cfg.Strict
		allowDestructive := c.Bool("allow-destructive")

		if c.Bool("json") {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(report); err != nil {
				return err
			}
		} else {
			printSafetyReport(report)
		}

		if report.HasErrors() {
			return fmt.Errorf("safety check failed: %d error(s), %d warning(s)", report.Errors(), report.Warnings())
		}
		if strict {
			failures := report.Warnings()
			if allowDestructive {
				failures -= report.DestructiveCount()
			}
			if failures > 0 {
				return fmt.Errorf("strict safety check failed: %d warning(s)", failures)
			}
		}
		return nil
	},
}

func checkDatabaseSafety(report *migration.SafetyReport, cfg *config.Config) {
	conn, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		report.Add(migration.SafetyIssue{
			Code:     "database_connect",
			Severity: migration.SafetyError,
			Message:  "could not connect to database: " + err.Error(),
		})
		return
	}
	defer conn.Close()

	status, err := migration.GetStatus(conn, cfg.MigrationPath, cfg.DatabaseURL)
	if err != nil {
		// A brand-new database has no migrations table yet. That is not a
		// safety problem; report it as info and continue.
		lower := strings.ToLower(err.Error())
		if strings.Contains(lower, "migrations table does not exist") ||
			(strings.Contains(lower, "migrations") && (strings.Contains(lower, "does not exist") || strings.Contains(lower, "doesn't exist"))) {
			report.Add(migration.SafetyIssue{
				Code:     "database_not_initialized",
				Severity: migration.SafetyInfo,
				Message:  "migrations table does not exist yet (run vm migrate to initialize it)",
			})
			return
		}
		report.Add(migration.SafetyIssue{
			Code:     "database_status",
			Severity: migration.SafetyError,
			Message:  "could not read migration status: " + err.Error(),
		})
		return
	}

	for _, entry := range status.Entries {
		if entry.FileMissing {
			report.Add(migration.SafetyIssue{
				File:     entry.Filename,
				Code:     "executed_file_missing",
				Severity: migration.SafetyError,
				Message:  "migration is recorded as executed but its file is missing on disk",
			})
		}
		if entry.ChecksumOK != nil && !*entry.ChecksumOK {
			report.Add(migration.SafetyIssue{
				File:     entry.Filename,
				Code:     "checksum_mismatch",
				Severity: migration.SafetyError,
				Message:  "file was modified after it ran (use 'vm migrate --force' only for intentional edits)",
			})
		}
	}

	if status.Pending > 0 {
		report.Add(migration.SafetyIssue{
			Code:     "pending_migrations",
			Severity: migration.SafetyInfo,
			Message:  fmt.Sprintf("%d pending migration(s)", status.Pending),
		})
	}
}

func printSafetyReport(report *migration.SafetyReport) {
	if len(report.Issues) == 0 {
		output.Success("No safety issues found (%d migration(s), %d file(s))", report.Migrations, report.Files)
		return
	}

	for _, issue := range report.Issues {
		loc := issue.File
		if issue.Section != "" {
			loc += " [" + issue.Section + "]"
		}
		if issue.Line > 0 {
			loc += fmt.Sprintf(":%d", issue.Line)
		}
		switch issue.Severity {
		case migration.SafetyError:
			output.Error("%s: %s", loc, issue.Message)
		case migration.SafetyWarning:
			output.Warning("%s: %s", loc, issue.Message)
		default:
			output.Info("%s: %s", loc, issue.Message)
		}
		if issue.Statement != "" {
			output.Println("    %s", issue.Statement)
		}
	}

	fmt.Println()
	output.Info("Checked %d migration(s) in %d file(s)", report.Migrations, report.Files)
	if report.Errors() > 0 {
		output.Error("%d error(s), %d warning(s)", report.Errors(), report.Warnings())
	} else {
		output.Success("No errors (%d warning(s))", report.Warnings())
	}
}
