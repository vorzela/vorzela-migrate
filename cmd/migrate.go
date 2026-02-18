package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v2"
	"github.com/vorzela/vorzela-migrate/internal/config"
	"github.com/vorzela/vorzela-migrate/internal/database"
	"github.com/vorzela/vorzela-migrate/internal/db"
	"github.com/vorzela/vorzela-migrate/internal/migration"
	"github.com/vorzela/vorzela-migrate/internal/output"
)

var MigrateCommand = &cli.Command{
	Name:  "migrate",
	Usage: "Run pending migrations",
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
			Name:    "enhanced",
			Aliases: []string{"e"},
			Usage:   "use enhanced migration features (checksums, locking, drift detection)",
			Value:   false,
		},
		&cli.BoolFlag{
			Name:  "verify-checksums",
			Usage: "verify checksums of previously run migrations",
			Value: false,
		},
		&cli.BoolFlag{
			Name:  "detect-drift",
			Usage: "detect and report schema drift",
			Value: false,
		},
		&cli.BoolFlag{
			Name:  "online",
			Usage: "use zero-downtime migration strategies where possible",
			Value: false,
		},
		&cli.BoolFlag{
			Name:  "dry-run",
			Usage: "show what would be executed without running migrations",
			Value: false,
		},
		&cli.BoolFlag{
			Name:    "verbose",
			Aliases: []string{"v"},
			Usage:   "enable verbose logging",
			Value:   false,
		},
		&cli.BoolFlag{
			Name:  "force",
			Usage: "force migration even with checksum mismatches",
			Value: false,
		},
		&cli.IntFlag{
			Name:    "step",
			Aliases: []string{"s"},
			Usage:   "run only N pending migrations (0 = unlimited)",
			Value:   0,
		},
	},
	Action: func(c *cli.Context) error {
		// Reject extra positional arguments (e.g. 'vm migrate fresh' → use 'vm fresh')
		if c.NArg() > 0 {
			arg := c.Args().First()
			return fmt.Errorf("'vm migrate' takes no arguments (got %q). Did you mean 'vm %s'?", arg, arg)
		}

		dsn := c.String("dsn")
		migrationPath := c.String("path")

		// Load config with overrides
		cfg, err := config.LoadConfig(dsn, migrationPath)
		if err != nil {
			output.Error(err.Error())
			return err
		}

		// Validate config
		if err := cfg.Validate(); err != nil {
			output.Error(err.Error())
			return err
		}

		// Connect to database
		db, err := database.Connect(cfg.DatabaseURL)
		if err != nil {
			output.Error("Failed to connect to database: %v", err)
			return fmt.Errorf("failed to connect to database: %w", err)
		}
		defer db.Close()

		// Auto-run extensions and functions if enabled (default: true)
		// This prevents errors like "function does not exist" during migrations
		if cfg.AutoRunExtensions {
			if err := runExtensionsIfNeeded(db, cfg); err != nil {
				// Only show error if extensions.sql exists
				extensionsPath := filepath.Join(cfg.MigrationPath, "extensions.sql")
				if _, statErr := os.Stat(extensionsPath); statErr == nil {
					output.Warning("Failed to run extensions: %v", err)
					output.Info("To skip auto-run, set AUTO_RUN_EXTENSIONS=false in .vm file")
				}
			}
		}

		if cfg.AutoRunFunctions {
			if err := runFunctionsIfNeeded(db, cfg); err != nil {
				// Only show error if functions.sql exists
				functionsPath := filepath.Join(cfg.MigrationPath, "functions.sql")
				if _, statErr := os.Stat(functionsPath); statErr == nil {
					output.Warning("Failed to run functions: %v", err)
					output.Info("To skip auto-run, set AUTO_RUN_FUNCTIONS=false in .vm file")
				}
			}
		}

		// Initialize migration table
		if err := migration.InitMigrationTable(db, cfg.DatabaseURL); err != nil {
			output.Error("Failed to initialize migration table: %v", err)
			return fmt.Errorf("failed to initialize migration table: %w", err)
		}

		// Validate migration files before running them
		output.Info("Validating migration files...")
		validationErrors := migration.ValidateAllMigrations(cfg.MigrationPath)
		if len(validationErrors) > 0 {
			output.Error("❌ Migration validation failed:")
			output.Error("")
			for _, err := range validationErrors {
				output.Error("  %s", err.Error())
			}
			output.Error("")
			output.Error("📚 Migration File Rules:")
			output.Error("  • Extensions must be in extensions.sql - Run 'vm extensions migrate' first")
			output.Error("  • Functions must be in functions.sql - Run 'vm functions migrate' first")
			output.Error("  • Migrations should only contain schema changes (CREATE TABLE, ALTER TABLE, etc.)")
			output.Error("")
			output.Error("⚡ Correct execution order:")
			output.Error("  1. vm extensions migrate   (if using PostgreSQL extensions)")
			output.Error("  2. vm functions migrate    (if using trigger functions)")
			output.Error("  3. vm migrate              (schema migrations)")
			output.Error("")
			return fmt.Errorf("migration validation failed: %d error(s) found", len(validationErrors))
		}
		output.Success("✓ All migration files validated")

		// Merge CLI flags with config settings (CLI flags take precedence)
		enhanced := c.Bool("enhanced") || cfg.Enhanced
		online := c.Bool("online") || cfg.Online
		verifyChecksums := c.Bool("verify-checksums") || cfg.VerifyChecksums
		detectDrift := c.Bool("detect-drift") || cfg.DetectDrift
		verbose := c.Bool("verbose") || cfg.Verbose
		dryRun := c.Bool("dry-run")
		force := c.Bool("force")
		step := c.Int("step")

		// Check if enhanced features are requested
		useEnhanced := enhanced || verifyChecksums || detectDrift || online || dryRun

		if useEnhanced {
			// Show environment info
			if verbose || cfg.Verbose {
				envColor := "\033[36m" // Cyan
				if cfg.IsProduction() {
					envColor = "\033[33m" // Yellow for production
				}
				fmt.Printf("%s[%s]%s Running in %s mode\n", envColor, cfg.Environment, "\033[0m", cfg.Environment)
			}

			// Use enhanced executor
			sqlDB, err := database.GetSQLDB(cfg.DatabaseURL)
			if err != nil {
				output.Error("Failed to get SQL DB connection: %v", err)
				return fmt.Errorf("failed to get SQL DB connection: %w", err)
			}
			defer sqlDB.Close()

			opts := migration.MigrationOptions{
				DryRun:          dryRun,
				Force:           force,
				Online:          online,
				VerifyChecksums: verifyChecksums,
				DetectDrift:     detectDrift,
				Verbose:         verbose,
				SkipLock:        false,
				DriftHandling:   cfg.DriftHandling,
				Step:            step,
			}

			executor, err := migration.NewEnhancedExecutor(db, sqlDB, cfg.DatabaseURL, cfg.MigrationPath, opts)
			if err != nil {
				output.Error("Failed to create enhanced executor: %v", err)
				return err
			}

			results, err := executor.RunMigrationsEnhanced(opts)
			if err != nil {
				return err
			}

			if dryRun {
				output.Info("DRY RUN completed - no migrations were actually executed")
			}

			if len(results) == 0 {
				output.Info("No new migrations to run")
			}

			return nil
		}

		// Use standard executor (legacy mode)
		count, err := migration.RunMigrations(db, cfg.MigrationPath, cfg.DatabaseURL)
		if err != nil {
			output.Error("Migration failed: %v", err)
			return fmt.Errorf("migration failed: %w", err)
		}

		if count == 0 {
			output.Info("No new migrations to run")
		} else {
			output.Success("Successfully ran %d migration(s)", count)
		}

		return nil
	},
}

// runExtensionsIfNeeded runs extensions.sql if it exists
func runExtensionsIfNeeded(db db.DB, cfg *config.Config) error {
	extensionsPath := filepath.Join(cfg.MigrationPath, "extensions.sql")

	// Check if file exists
	if _, err := os.Stat(extensionsPath); os.IsNotExist(err) {
		return nil // No extensions file, skip
	}

	// Read and execute extensions
	sqlContent, err := os.ReadFile(extensionsPath)
	if err != nil {
		return fmt.Errorf("failed to read extensions.sql: %w", err)
	}

	// Parse enabled extensions (non-commented CREATE EXTENSION lines)
	enabledExtensions := parseEnabledExtensions(string(sqlContent))
	if len(enabledExtensions) == 0 {
		return nil // No extensions enabled, skip
	}

	// Execute the SQL silently
	ctx := context.Background()
	if err := db.Exec(ctx, string(sqlContent)); err != nil {
		return fmt.Errorf("failed to install extensions: %w", err)
	}

	output.Info("✓ PostgreSQL extensions installed (%d)", len(enabledExtensions))
	return nil
}

// runFunctionsIfNeeded runs functions.sql if it exists
func runFunctionsIfNeeded(db db.DB, cfg *config.Config) error {
	functionsPath := filepath.Join(cfg.MigrationPath, "functions.sql")

	// Check if file exists
	if _, err := os.Stat(functionsPath); os.IsNotExist(err) {
		return nil // No functions file, skip
	}

	// Read and execute functions
	sqlContent, err := os.ReadFile(functionsPath)
	if err != nil {
		return fmt.Errorf("failed to read functions.sql: %w", err)
	}

	// Execute the SQL silently
	ctx := context.Background()
	if err := db.Exec(ctx, string(sqlContent)); err != nil {
		return fmt.Errorf("failed to install functions: %w", err)
	}

	output.Info("✓ Database functions installed")
	return nil
}
