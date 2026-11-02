package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v2"
	"github.com/vorzela/vorzela-migrate/internal/config"
	"github.com/vorzela/vorzela-migrate/internal/database"
	"github.com/vorzela/vorzela-migrate/internal/migration"
	"github.com/vorzela/vorzela-migrate/internal/output"
)

var FunctionsCommand = &cli.Command{
	Name:  "functions",
	Usage: "Manage database functions",
	Subcommands: []*cli.Command{
		{
			Name:  "migrate",
			Usage: "Install common database functions from functions.sql",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "dsn",
					Aliases: []string{"d"},
					Usage:   "database connection string",
				},
				&cli.StringFlag{
					Name:    "path",
					Aliases: []string{"p"},
					Value:   "./migrations",
					Usage:   "path to migrations directory",
				},
			},
			Action: func(c *cli.Context) error {
				// Load configuration
				cfg, err := config.LoadConfig(c.String("dsn"), c.String("path"))
				if err != nil {
					output.Error(err.Error())
					return err
				}

				// Functions use PL/pgSQL — PostgreSQL only
				if d := migration.DetectDialect(cfg.DatabaseURL); d != migration.PostgreSQL {
					return fmt.Errorf("vm functions is not supported on %s — PL/pgSQL trigger functions are a PostgreSQL-only feature", d)
				}

				// Connect to database
				db, err := database.Connect(cfg.DatabaseURL)
				if err != nil {
					output.Error(fmt.Sprintf("Failed to connect: %v", err))
					return err
				}
				defer db.Close()

				// Read functions.sql file
				functionsPath := filepath.Join(cfg.MigrationPath, "functions.sql")
				if _, statErr := os.Stat(functionsPath); os.IsNotExist(statErr) {
					output.Info("functions.sql not found, creating template...")
					if err := migration.EnsureFunctionsFile(cfg.MigrationPath); err != nil {
						output.Error(err.Error())
						return err
					}
					output.Info("Edit functions.sql and add the functions you need, then run this command again.")
					return nil
				}
				sqlContent, err := os.ReadFile(functionsPath)
				if err != nil {
					output.Error(fmt.Sprintf("Failed to read %s: %v", functionsPath, err))
					return err
				}
				if err := migration.ValidateFunctionsFile(functionsPath); err != nil {
					output.Error("%v", err)
					return err
				}

				content := string(sqlContent)

				// ── Pre-flight: classify each function ──────────────────────────
				enabled, _ := migration.ParseAllFunctionNames(content)
				ctx := context.Background()

				type fnState struct {
					name    string
					existed bool
				}
				states := make([]fnState, 0, len(enabled))
				for _, fn := range enabled {
					rows, qErr := db.Query(ctx,
						`SELECT 1 FROM pg_proc p
						  JOIN pg_namespace n ON n.oid = p.pronamespace
						 WHERE p.proname = $1 AND n.nspname = 'public'`, fn)
					existed := false
					if qErr == nil {
						existed = rows.Next()
						rows.Close()
					}
					states = append(states, fnState{fn, existed})
				}

				// ── Apply ───────────────────────────────────────────────────────
				output.Info(fmt.Sprintf("Syncing functions from %s...", functionsPath))
				statements, parseErr := migration.SplitSQLStatements(content)
				if parseErr != nil {
					output.Error(fmt.Sprintf("Failed to parse functions.sql: %v", parseErr))
					return parseErr
				}
				for _, stmt := range statements {
					if err := db.Exec(ctx, stmt); err != nil {
						output.Error(fmt.Sprintf("Failed to apply functions: %v", err))
						return err
					}
				}

				createdCount, updatedCount := 0, 0
				for _, s := range states {
					if s.existed {
						output.Println("  ~ updated  %s()", s.name)
						updatedCount++
					} else {
						output.Println("  + created  %s()", s.name)
						createdCount++
					}
				}

				// Commented-out entries are deliberately NOT dropped here.
				// Use `vm functions drop` (explicit) to remove functions.

				output.Success(fmt.Sprintf("Function sync complete (created: %d, updated: %d)", createdCount, updatedCount))
				return nil
			},
		},
		{
			Name:  "regenerate",
			Usage: "Regenerate the auto-generated section of functions.sql, preserving custom functions",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "path",
					Aliases: []string{"p"},
					Value:   "./migrations",
					Usage:   "path to migrations directory",
				},
			},
			Action: func(c *cli.Context) error {
				cfg, err := config.LoadConfig("", c.String("path"))
				if err != nil {
					output.Error(err.Error())
					return err
				}
				if err := migration.RegenerateFunctions(cfg.MigrationPath); err != nil {
					output.Error("%v", err)
					return err
				}
				output.Success("functions.sql regenerated")
				return nil
			},
		},
		{
			Name:  "drop",
			Usage: "Drop common database functions",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "dsn",
					Aliases: []string{"d"},
					Usage:   "database connection string",
				},
				&cli.StringFlag{
					Name:    "path",
					Aliases: []string{"p"},
					Value:   "./migrations",
					Usage:   "path to migrations directory",
				},
				&cli.BoolFlag{
					Name:    "step",
					Aliases: []string{"s"},
					Usage:   "drop functions one by one with confirmation",
				},
				&cli.BoolFlag{
					Name:  "force",
					Usage: "skip the drop confirmation prompt",
				},
			},
			Action: func(c *cli.Context) error {
				// Load configuration
				cfg, err := config.LoadConfig(c.String("dsn"), c.String("path"))
				if err != nil {
					output.Error(err.Error())
					return err
				}
				if d := migration.DetectDialect(cfg.DatabaseURL); d != migration.PostgreSQL {
					return fmt.Errorf("vm functions is not supported on %s — PL/pgSQL trigger functions are a PostgreSQL-only feature", d)
				}

				// Connect to database
				db, err := database.Connect(cfg.DatabaseURL)
				if err != nil {
					output.Error(fmt.Sprintf("Failed to connect: %v", err))
					return err
				}
				defer db.Close()

				ctx := context.Background()
				stepMode := c.Bool("step")

				// List of functions to drop
				functions := []string{
					"auto_update_timestamp",
					"protect_soft_deleted",
					"auto_update_with_soft_delete_protection",
					"prevent_hard_delete",
				}

				if stepMode {
					output.Info("Dropping functions step by step...")
					for _, fn := range functions {
						if !output.Confirm(fmt.Sprintf("Drop function %s?", fn)) {
							output.Info(fmt.Sprintf("Skipped: %s", fn))
							continue
						}

						dropSQL := fmt.Sprintf("DROP FUNCTION IF EXISTS %s() CASCADE;", fn)
						if err := db.Exec(ctx, dropSQL); err != nil {
							output.Error(fmt.Sprintf("Failed to drop %s: %v", fn, err))
							continue
						}
						output.Success(fmt.Sprintf("Dropped: %s", fn))
					}
				} else {
					if !c.Bool("force") && !output.Confirm("Drop all common functions?") {
						output.Info("Cancelled")
						return nil
					}
					output.Info("Dropping all common functions...")
					for _, fn := range functions {
						dropSQL := fmt.Sprintf("DROP FUNCTION IF EXISTS %s() CASCADE;", fn)
						if err := db.Exec(ctx, dropSQL); err != nil {
							output.Error(fmt.Sprintf("Failed to drop %s: %v", fn, err))
							continue
						}
						output.Info(fmt.Sprintf("Dropped: %s", fn))
					}
					output.Success("All functions dropped successfully")
				}

				return nil
			},
		},
	},
}
