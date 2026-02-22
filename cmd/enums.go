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

var EnumsCommand = &cli.Command{
	Name:  "enums",
	Usage: "Manage PostgreSQL enum types",
	Subcommands: []*cli.Command{
		{
			Name:  "migrate",
			Usage: "Create enum types defined in enums.sql",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "dsn",
					Aliases: []string{"d"},
					Usage:   "database connection string",
					EnvVars: []string{"DATABASE_URL"},
				},
				&cli.StringFlag{
					Name:    "path",
					Aliases: []string{"p"},
					Value:   "./migrations",
					Usage:   "path to migrations directory",
				},
			},
			Action: func(c *cli.Context) error {
				cfg, err := config.LoadConfig(c.String("dsn"), c.String("path"))
				if err != nil {
					output.Error(err.Error())
					return err
				}

				// CREATE TYPE AS ENUM is a PostgreSQL-only feature
				if d := migration.DetectDialect(cfg.DatabaseURL); d != migration.PostgreSQL {
					return fmt.Errorf("vm enums is not supported on %s — CREATE TYPE AS ENUM is a PostgreSQL-only feature", d)
				}

				enumsPath := filepath.Join(cfg.MigrationPath, "enums.sql")
				if _, err := os.Stat(enumsPath); os.IsNotExist(err) {
					output.Info("enums.sql not found, creating template...")
					if err := migration.EnsureEnumsFile(cfg.MigrationPath); err != nil {
						output.Error(err.Error())
						return err
					}
					output.Info("Edit enums.sql and uncomment the types you need, then run this command again.")
					return nil
				}

				sqlContent, err := os.ReadFile(enumsPath)
				if err != nil {
					output.Error("Failed to read %s: %v", enumsPath, err)
					return err
				}

				content := string(sqlContent)
				enabled, disabled := migration.ParseAllEnumNames(content)

				db, err := database.Connect(cfg.DatabaseURL)
				if err != nil {
					output.Error("Failed to connect: %v", err)
					return err
				}
				defer db.Close()

				ctx := context.Background()

				// ── Create / update enabled enums ─────────────────────────────────
				if len(enabled) > 0 {
					output.Info("Syncing enum types from %s...", enumsPath)
				}
				for _, name := range enabled {
					stmt := migration.ExtractEnumStatement(content, name)
					if stmt == "" {
						continue
					}
					vals := migration.ParseEnumValues(stmt)
					syncSQL := migration.GenerateEnumSyncSQL(name, vals)
					if syncSQL == "" {
						continue
					}
					if err := db.Exec(ctx, syncSQL); err != nil {
						output.Error("Failed to sync enum '%s': %v", name, err)
						return err
					}
					output.Success("✓ %s", name)
				}

				// ── Drop disabled / commented-out enums ────────────────────────────
				// Only drop types that appear somewhere in the file (managed by vm).
				for _, name := range disabled {
					// Check if it actually exists in the DB before trying to drop.
					rows, err := db.Query(ctx,
						`SELECT 1 FROM pg_type t
						  JOIN pg_namespace n ON n.oid = t.typnamespace
						 WHERE t.typname = $1 AND n.nspname = 'public' AND t.typtype = 'e'`,
						name)
					if err != nil {
						output.Warning("Could not check enum '%s': %v", name, err)
						continue
					}
					exists := rows.Next()
					rows.Close()
					if !exists {
						continue
					}
					dropSQL := fmt.Sprintf("DROP TYPE IF EXISTS %s CASCADE;", name)
					if err := db.Exec(ctx, dropSQL); err != nil {
						output.Warning("Failed to drop enum '%s': %v", name, err)
						continue
					}
					output.Info("↓ Removed disabled enum: %s", name)
				}

				if len(enabled) == 0 && len(disabled) == 0 {
					output.Warning("No enum types found in enums.sql")
					output.Info("Uncomment the types you need in migrations/enums.sql")
					return nil
				}

				output.Success("Enum sync complete (enabled: %d, removed: %d)", len(enabled), len(disabled))
				return nil
			},
		},
		{
			Name:  "drop",
			Usage: "Drop enum types defined in enums.sql",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "dsn",
					Aliases: []string{"d"},
					Usage:   "database connection string",
					EnvVars: []string{"DATABASE_URL"},
				},
				&cli.StringFlag{
					Name:    "path",
					Aliases: []string{"p"},
					Value:   "./migrations",
					Usage:   "path to migrations directory",
				},
				&cli.BoolFlag{
					Name:  "force",
					Usage: "skip confirmation prompt",
				},
				&cli.BoolFlag{
					Name:    "step",
					Aliases: []string{"s"},
					Usage:   "drop enum types one by one with confirmation",
				},
			},
			Action: func(c *cli.Context) error {
				cfg, err := config.LoadConfig(c.String("dsn"), c.String("path"))
				if err != nil {
					output.Error(err.Error())
					return err
				}

				enumsPath := filepath.Join(cfg.MigrationPath, "enums.sql")
				sqlContent, err := os.ReadFile(enumsPath)
				if err != nil {
					output.Error("Failed to read %s: %v", enumsPath, err)
					output.Info("Tip: run 'vm enums migrate' first to create enums.sql")
					return err
				}

				enabled := migration.ParseEnabledEnums(string(sqlContent))
				if len(enabled) == 0 {
					output.Warning("No enum types found in enums.sql")
					return nil
				}

				db, err := database.Connect(cfg.DatabaseURL)
				if err != nil {
					output.Error("Failed to connect: %v", err)
					return err
				}
				defer db.Close()

				ctx := context.Background()
				stepMode := c.Bool("step")

				if stepMode {
					output.Info("Dropping enum types one by one...")
					for _, name := range enabled {
						fmt.Printf("Drop type %s? [y/N]: ", name)
						var resp string
						fmt.Scanln(&resp)
						if resp != "y" && resp != "Y" {
							output.Info("Skipped: %s", name)
							continue
						}
						dropSQL := fmt.Sprintf("DROP TYPE IF EXISTS %s CASCADE;", name)
						if err := db.Exec(ctx, dropSQL); err != nil {
							output.Error("Failed to drop '%s': %v", name, err)
							continue
						}
						output.Success("Dropped: %s", name)
					}
					return nil
				}

				if !c.Bool("force") {
					output.Warning("⚠️  This will drop %d enum type(s) with CASCADE.", len(enabled))
					fmt.Print("Are you sure? [y/N]: ")
					var resp string
					fmt.Scanln(&resp)
					if resp != "y" && resp != "Y" {
						output.Info("Cancelled")
						return nil
					}
				}

				output.Info("Dropping enum types...")
				for _, name := range enabled {
					dropSQL := fmt.Sprintf("DROP TYPE IF EXISTS %s CASCADE;", name)
					if err := db.Exec(ctx, dropSQL); err != nil {
						output.Error("Failed to drop '%s': %v", name, err)
						continue
					}
					output.Success("Dropped: %s", name)
				}
				return nil
			},
		},
		{
			Name:  "status",
			Usage: "Show which enum types are defined in enums.sql vs the database",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "dsn",
					Aliases: []string{"d"},
					Usage:   "database connection string",
					EnvVars: []string{"DATABASE_URL"},
				},
				&cli.StringFlag{
					Name:    "path",
					Aliases: []string{"p"},
					Value:   "./migrations",
					Usage:   "path to migrations directory",
				},
			},
			Action: func(c *cli.Context) error {
				cfg, err := config.LoadConfig(c.String("dsn"), c.String("path"))
				if err != nil {
					output.Error(err.Error())
					return err
				}

				enumsPath := filepath.Join(cfg.MigrationPath, "enums.sql")
				sqlContent, err := os.ReadFile(enumsPath)
				if err != nil {
					output.Warning("enums.sql not found; run 'vm enums migrate' to create it.")
					return nil
				}

				defined := migration.ParseEnabledEnums(string(sqlContent))

				db, err := database.Connect(cfg.DatabaseURL)
				if err != nil {
					output.Error("Failed to connect: %v", err)
					return err
				}
				defer db.Close()

				// Query existing enum types from pg_type
				ctx := context.Background()
				rows, err := db.Query(ctx,
					`SELECT t.typname, string_agg(e.enumlabel, ', ' ORDER BY e.enumsortorder) AS labels
					 FROM pg_type t
					 JOIN pg_enum e ON t.oid = e.enumtypid
					 JOIN pg_catalog.pg_namespace n ON n.oid = t.typnamespace
					 WHERE n.nspname = 'public'
					 GROUP BY t.typname
					 ORDER BY t.typname`)
				if err != nil {
					output.Error("Failed to query enum types: %v", err)
					return err
				}
				defer rows.Close()

				existing := make(map[string]string)
				for rows.Next() {
					var name, labels string
					if err := rows.Scan(&name, &labels); err != nil {
						continue
					}
					existing[name] = labels
				}

				definedSet := make(map[string]bool)
				for _, d := range defined {
					definedSet[d] = true
				}

				fmt.Println()
				fmt.Println("Enum types defined in enums.sql:")
				if len(defined) == 0 {
					output.Info("  (none enabled)")
				}
				for _, name := range defined {
					if labels, ok := existing[name]; ok {
						output.Success("  ✓ %-30s → (%s)", name, labels)
					} else {
						output.Warning("  ✗ %-30s → not in database (run: vm enums migrate)", name)
					}
				}

				fmt.Println()
				fmt.Println("Enum types in database not in enums.sql:")
				found := false
				for name, labels := range existing {
					if !definedSet[name] {
						output.Info("  ? %-30s → (%s)", name, labels)
						found = true
					}
				}
				if !found {
					output.Info("  (none)")
				}
				fmt.Println()

				return nil
			},
		},
	},
}
