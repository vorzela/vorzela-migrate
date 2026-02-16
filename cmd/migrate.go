package cmd

import (
	"fmt"

	"github.com/urfave/cli/v2"
	"github.com/vorzela/vorzela-migrate/internal/config"
	"github.com/vorzela/vorzela-migrate/internal/database"
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
			Name:    "verify-checksums",
			Usage:   "verify checksums of previously run migrations",
			Value:   false,
		},
		&cli.BoolFlag{
			Name:    "detect-drift",
			Usage:   "detect and report schema drift",
			Value:   false,
		},
		&cli.BoolFlag{
			Name:    "online",
			Usage:   "use zero-downtime migration strategies where possible",
			Value:   false,
		},
		&cli.BoolFlag{
			Name:    "dry-run",
			Usage:   "show what would be executed without running migrations",
			Value:   false,
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
	},
	Action: func(c *cli.Context) error {
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

		// Initialize migration table
		if err := migration.InitMigrationTable(db, cfg.DatabaseURL); err != nil {
			output.Error("Failed to initialize migration table: %v", err)
			return fmt.Errorf("failed to initialize migration table: %w", err)
		}

		// Merge CLI flags with config settings (CLI flags take precedence)
		enhanced := c.Bool("enhanced") || cfg.Enhanced
		online := c.Bool("online") || cfg.Online
		verifyChecksums := c.Bool("verify-checksums") || cfg.VerifyChecksums
		detectDrift := c.Bool("detect-drift") || cfg.DetectDrift
		verbose := c.Bool("verbose") || cfg.Verbose
		dryRun := c.Bool("dry-run")
		force := c.Bool("force")

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
				DriftHandling:   cfg.DriftHandling, // Pass drift handling mode
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
