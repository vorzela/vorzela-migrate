package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v2"
	"github.com/vorzela/vorzela-migrate/internal/config"
	"github.com/vorzela/vorzela-migrate/internal/database"
	"github.com/vorzela/vorzela-migrate/internal/migration"
	"github.com/vorzela/vorzela-migrate/internal/output"
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

				// Parse enabled extensions (non-commented CREATE EXTENSION lines)
				enabledExtensions := parseEnabledExtensions(string(sqlContent))
				if len(enabledExtensions) == 0 {
					output.Warning("No extensions enabled in extensions.sql")
					output.Info("Uncomment the extensions you need in migrations/extensions.sql")
					return nil
				}

				// Execute the SQL
				output.Info(fmt.Sprintf("Installing extensions from %s...", extensionsPath))
				ctx := context.Background()
				if err := db.Exec(ctx, string(sqlContent)); err != nil {
					output.Error(fmt.Sprintf("Failed to install extensions: %v", err))
					return err
				}

				output.Success(fmt.Sprintf("PostgreSQL extensions installed successfully (%d total)", len(enabledExtensions)))
				for _, ext := range enabledExtensions {
					output.Info(fmt.Sprintf("✓ %s", ext))
				}

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
			},
			Action: func(c *cli.Context) error {
				// Load configuration
				cfg, err := config.LoadConfig(c.String("dsn"), c.String("path"))
				if err != nil {
					output.Error(err.Error())
					return err
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
				enabledExtensions := parseEnabledExtensions(string(sqlContent))
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
						fmt.Printf("Drop extension %s? [y/N]: ", ext)
						var response string
						fmt.Scanln(&response)
						if response != "y" && response != "Y" {
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
					output.Warning("⚠️  This will drop all enabled extensions from extensions.sql")
					fmt.Printf("Are you sure? [y/N]: ")
					var response string
					fmt.Scanln(&response)
					if response != "y" && response != "Y" {
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

// parseEnabledExtensions extracts enabled (non-commented) extension names from SQL
func parseEnabledExtensions(content string) []string {
	var extensions []string
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}

		// Look for CREATE EXTENSION statements
		if strings.Contains(strings.ToUpper(trimmed), "CREATE EXTENSION") {
			// Extract extension name
			// Handle: CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
			// or: CREATE EXTENSION IF NOT EXISTS pg_trgm;
			parts := strings.Fields(trimmed)
			for i, part := range parts {
				if strings.ToUpper(part) == "EXISTS" && i+1 < len(parts) {
					extName := parts[i+1]
					// Remove quotes and semicolon
					extName = strings.Trim(extName, `"';`)
					if extName != "" {
						extensions = append(extensions, extName)
					}
					break
				}
			}
		}
	}

	return extensions
}
