package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v2"
	"github.com/vorzela/vorzela-migrate/v2/internal/config"
	"github.com/vorzela/vorzela-migrate/v2/internal/database"
	"github.com/vorzela/vorzela-migrate/v2/internal/migration"
	"github.com/vorzela/vorzela-migrate/v2/internal/output"
)

var ExtensionsCommand = &cli.Command{
	Name:  "extensions",
	Usage: "Manage PostgreSQL extensions",
	Subcommands: []*cli.Command{
		{
			Name:  "migrate",
			Usage: "Install PostgreSQL extensions from extensions.sql",
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

				// Extensions are a PostgreSQL-only feature
				if d := migration.DetectDialect(cfg.DatabaseURL); d != migration.PostgreSQL {
					return fmt.Errorf("vm extensions is not supported on %s — CREATE EXTENSION is a PostgreSQL-only feature", d)
				}

				// Check if extensions.sql exists, create if not
				extensionsPath := filepath.Join(cfg.MigrationPath, "extensions.sql")
				if _, err := os.Stat(extensionsPath); os.IsNotExist(err) {
					output.Info("extensions.sql not found, creating template...")
					if err := migration.EnsureExtensionsFile(cfg.MigrationPath); err != nil {
						output.Error(err.Error())
						return err
					}
					output.Info("Please edit extensions.sql and uncomment the extensions you need, then run this command again")
					return nil
				}

				// Connect to database
				db, err := database.Connect(cfg.DatabaseURL)
				if err != nil {
					output.Error(fmt.Sprintf("Failed to connect: %v", err))
					return err
				}
				defer db.Close()

				// Read extensions.sql file
				sqlContent, err := os.ReadFile(extensionsPath)
				if err != nil {
					output.Error(fmt.Sprintf("Failed to read %s: %v", extensionsPath, err))
					return err
				}
				if err := migration.ValidateExtensionsFile(extensionsPath); err != nil {
					output.Error("%v", err)
					return err
				}

				content := string(sqlContent)
				enabled, _ := migration.ParseAllExtensionNames(content)
				ctx := context.Background()

				if len(enabled) == 0 {
					output.Warning("No enabled extensions found in extensions.sql")
					output.Info("Uncomment the extensions you need in extensions.sql")
					return nil
				}

				// ── Pre-flight: which extensions are already installed ───────────
				type extState struct {
					name    string
					existed bool
				}
				states := make([]extState, 0, len(enabled))
				for _, ext := range enabled {
					rows, qErr := db.Query(ctx,
						`SELECT 1 FROM pg_extension WHERE extname = $1`, ext)
					existed := false
					if qErr == nil {
						existed = rows.Next()
						rows.Close()
					}
					states = append(states, extState{ext, existed})
				}

				// ── Apply enabled extensions (CREATE EXTENSION IF NOT EXISTS is idempotent) ──
				output.Info(fmt.Sprintf("Syncing extensions from %s...", extensionsPath))
				statements, parseErr := migration.SplitSQLStatements(content)
				if parseErr != nil {
					output.Error("Failed to parse extensions.sql: %v", parseErr)
					return parseErr
				}
				for _, stmt := range statements {
					if err := db.Exec(ctx, stmt); err != nil {
						output.Error(fmt.Sprintf("Failed to install extensions: %v", err))
						return err
					}
				}
				for _, s := range states {
					if s.existed {
						output.Println("  · already  %s", s.name)
					} else {
						output.Println("  + installed %s", s.name)
					}
				}

				// Commented-out entries are deliberately NOT dropped here.
				// Use `vm extensions drop` (explicit) to remove extensions.

				installedCount := 0
				for _, s := range states {
					if !s.existed {
						installedCount++
					}
				}
				output.Success(fmt.Sprintf("Extension sync complete (installed: %d)", installedCount))
				return nil
			},
		},
		{
			Name:  "drop",
			Usage: "Drop PostgreSQL extensions",
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
					Usage:   "drop extensions one by one with confirmation",
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
					return fmt.Errorf("vm extensions is not supported on %s — CREATE EXTENSION is a PostgreSQL-only feature", d)
				}

				// Read extensions.sql to see which ones are enabled
				extensionsPath := filepath.Join(cfg.MigrationPath, "extensions.sql")
				sqlContent, err := os.ReadFile(extensionsPath)
				if err != nil {
					output.Error(fmt.Sprintf("Failed to read %s: %v", extensionsPath, err))
					output.Info("Tip: The extensions.sql file should be in your configured migration path")
					return err
				}

				// Parse enabled extensions
				enabledExtensions := migration.ParseEnabledExtensions(string(sqlContent))
				if len(enabledExtensions) == 0 {
					output.Warning("No extensions found in extensions.sql")
					return nil
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

				if stepMode {
					output.Info("Dropping extensions step by step...")
					for _, ext := range enabledExtensions {
						if !output.Confirm(fmt.Sprintf("Drop extension %s?", ext)) {
							output.Info(fmt.Sprintf("Skipped: %s", ext))
							continue
						}

						dropSQL := fmt.Sprintf("DROP EXTENSION IF EXISTS %s CASCADE;", ext)
						if err := db.Exec(ctx, dropSQL); err != nil {
							output.Error(fmt.Sprintf("Failed to drop %s: %v", ext, err))
							continue
						}
						output.Success(fmt.Sprintf("Dropped: %s", ext))
					}
				} else {
					output.Warning("This will drop all enabled extensions from extensions.sql")
					if !c.Bool("force") && !output.Confirm("Are you sure?") {
						output.Info("Cancelled")
						return nil
					}

					output.Info("Dropping all extensions...")
					for _, ext := range enabledExtensions {
						dropSQL := fmt.Sprintf("DROP EXTENSION IF EXISTS %s CASCADE;", ext)
						if err := db.Exec(ctx, dropSQL); err != nil {
							output.Error(fmt.Sprintf("Failed to drop %s: %v", ext, err))
							continue
						}
						output.Success(fmt.Sprintf("Dropped: %s", ext))
					}
				}

				return nil
			},
		},
	},
}
