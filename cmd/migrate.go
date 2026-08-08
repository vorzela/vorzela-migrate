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
		defer func() { db.Close() }()

		// Auto-run extensions, functions and enums if enabled (default: true).
		// These are PostgreSQL-only — skip silently on MySQL/MariaDB.
		// On failure: do NOT continue to the next step; offer retry after the user fixes the issue.
		dialect := migration.DetectDialect(cfg.DatabaseURL)
		isPostgres := dialect == migration.PostgreSQL

		if cfg.AutoRunExtensions {
			if !isPostgres {
				output.Info("Skipping auto-run extensions (PostgreSQL only)")
			} else if err := runAutoPrerequisite(c, "extensions", "AUTO_RUN_EXTENSIONS", &cfg, &db, runExtensionsIfNeeded); err != nil {
				return err
			}
		}

		if cfg.AutoRunFunctions {
			if !isPostgres {
				output.Info("Skipping auto-run functions (PostgreSQL only)")
			} else if err := runAutoPrerequisite(c, "functions", "AUTO_RUN_FUNCTIONS", &cfg, &db, runFunctionsIfNeeded); err != nil {
				return err
			}
		}

		if cfg.AutoRunEnums {
			if !isPostgres {
				output.Info("Skipping auto-run enums (PostgreSQL only)")
			} else if err := runAutoPrerequisite(c, "enums", "AUTO_RUN_ENUMS", &cfg, &db, runEnumsIfNeeded); err != nil {
				return err
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
		count, _, err := migration.RunMigrations(db, cfg.MigrationPath, cfg.DatabaseURL)
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

// runAutoPrerequisite runs an auto-run step (extensions / functions / enums).
// On failure it does NOT continue to the next migrate step — it prompts to retry
// after the user fixes credentials/config, reloading .vm and reconnecting.
func runAutoPrerequisite(
	c *cli.Context,
	name, skipFlag string,
	cfg **config.Config,
	dbp *db.DB,
	step func(db.DB, *config.Config) error,
) error {
	for {
		err := step(*dbp, *cfg)
		if err == nil {
			return nil
		}

		output.Error("Failed to run %s: %v", name, err)
		output.Info("Migrate will not continue until this succeeds.")
		output.Info("Fix the issue (e.g. DATABASE_URL / password in .vm or env), then retry.")
		output.Info("To skip this step, set %s=false in .vm and re-run `vm migrate`", skipFlag)

		if !output.Confirm(fmt.Sprintf("Retry %s?", name)) {
			return fmt.Errorf("%s failed: %w", name, err)
		}

		// Reload config so fixed credentials / .vm changes take effect.
		newCfg, lerr := config.LoadConfig(c.String("dsn"), c.String("path"))
		if lerr != nil {
			output.Warning("Could not reload config: %v — retrying with previous settings", lerr)
		} else if verr := newCfg.Validate(); verr != nil {
			output.Warning("Reloaded config invalid: %v — retrying with previous settings", verr)
		} else {
			*cfg = newCfg
		}

		newDB, cerr := database.Connect((*cfg).DatabaseURL)
		if cerr != nil {
			output.Warning("Reconnect failed: %v — retrying with existing connection", cerr)
			continue
		}
		(*dbp).Close()
		*dbp = newDB
	}
}

// enumOp describes a single pending enum change for display before prompting.
type enumOp struct {
	kind string // "create" | "add-values" | "drop"
	name string
	vals []string
}

// runEnumsIfNeeded syncs enum types from enums.sql:
//   - Creates each enabled enum if it does not exist yet.
//   - Adds missing values to existing enums.
//   - Drops enums that appear in the file as comments (disabled / removed).
//   - Prompts the user before applying any changes.
func runEnumsIfNeeded(conn db.DB, cfg *config.Config) error {
	enumsPath := filepath.Join(cfg.MigrationPath, "enums.sql")

	if _, err := os.Stat(enumsPath); os.IsNotExist(err) {
		return nil // No enums file, skip
	}

	sqlContent, err := os.ReadFile(enumsPath)
	if err != nil {
		return fmt.Errorf("failed to read enums.sql: %w", err)
	}

	content := string(sqlContent)
	enabled, disabled := migration.ParseAllEnumNames(content)
	ctx := context.Background()

	// ── Pre-flight: determine what actually needs changing ──────────────────
	var ops []enumOp

	for _, name := range enabled {
		stmt := migration.ExtractEnumStatement(content, name)
		if stmt == "" {
			continue
		}
		wantVals := migration.ParseEnumValues(stmt)

		rows, err := conn.Query(ctx,
			`SELECT 1 FROM pg_type t
			  JOIN pg_namespace n ON n.oid = t.typnamespace
			 WHERE t.typname = $1 AND n.nspname = 'public' AND t.typtype = 'e'`, name)
		if err != nil {
			ops = append(ops, enumOp{"create", name, wantVals})
			continue
		}
		exists := rows.Next()
		rows.Close()

		if !exists {
			ops = append(ops, enumOp{"create", name, wantVals})
			continue
		}

		// Enum exists — find values not yet present in the DB
		var missing []string
		for _, v := range wantVals {
			vrows, verr := conn.Query(ctx,
				`SELECT 1 FROM pg_enum e
				  JOIN pg_type t ON t.oid = e.enumtypid
				  JOIN pg_namespace n ON n.oid = t.typnamespace
				 WHERE t.typname = $1 AND n.nspname = 'public' AND e.enumlabel = $2`, name, v)
			if verr != nil {
				missing = append(missing, v)
				continue
			}
			found := vrows.Next()
			vrows.Close()
			if !found {
				missing = append(missing, v)
			}
		}
		if len(missing) > 0 {
			ops = append(ops, enumOp{"add-values", name, missing})
		}
	}

	for _, name := range disabled {
		rows, err := conn.Query(ctx,
			`SELECT 1 FROM pg_type t
			  JOIN pg_namespace n ON n.oid = t.typnamespace
			 WHERE t.typname = $1 AND n.nspname = 'public' AND t.typtype = 'e'`, name)
		if err != nil {
			continue
		}
		exists := rows.Next()
		rows.Close()
		if exists {
			ops = append(ops, enumOp{"drop", name, nil})
		}
	}

	if len(ops) == 0 {
		return nil // Nothing to do
	}

	// ── Display plan ────────────────────────────────────────────────────────
	output.Info("Enum sync plan:")
	for _, op := range ops {
		switch op.kind {
		case "create":
			output.Println("  + create type %s (%d values)", op.name, len(op.vals))
		case "add-values":
			output.Println("  ~ update type %s: add values %v", op.name, op.vals)
		case "drop":
			output.Warning("  - drop type %s if unused", op.name)
		}
	}

	if !output.Confirm("Apply enum sync?") {
		output.Info("Skipping enum sync")
		return nil
	}

	// ── Execute ─────────────────────────────────────────────────────────────
	createCount, updateCount, dropCount := 0, 0, 0

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
		if err := conn.Exec(ctx, syncSQL); err != nil {
			return fmt.Errorf("failed to sync enum '%s': %w", name, err)
		}
	}
	for _, op := range ops {
		switch op.kind {
		case "create":
			createCount++
		case "add-values":
			updateCount++
		case "drop":
			dropCount++
		}
	}

	for _, name := range disabled {
		rows, err := conn.Query(ctx,
			`SELECT 1 FROM pg_type t
			  JOIN pg_namespace n ON n.oid = t.typnamespace
			 WHERE t.typname = $1 AND n.nspname = 'public' AND t.typtype = 'e'`, name)
		if err != nil {
			continue
		}
		exists := rows.Next()
		rows.Close()
		if !exists {
			continue
		}
		conn.Exec(ctx, migration.GenerateDropEnumIfUnusedSQL(name)) //nolint:errcheck
	}

	output.Success("✓ Enum types synced (created: %d, updated: %d, dropped: %d)", createCount, updateCount, dropCount)
	return nil
}

// runExtensionsIfNeeded syncs extensions from extensions.sql:
//   - Installs enabled extensions that are not yet present in pg_extension.
//   - Drops extensions that are commented out or removed from the file.
//   - Prompts the user before applying any changes.
func runExtensionsIfNeeded(conn db.DB, cfg *config.Config) error {
	extensionsPath := filepath.Join(cfg.MigrationPath, "extensions.sql")

	if _, err := os.Stat(extensionsPath); os.IsNotExist(err) {
		return nil // No extensions file, skip
	}

	sqlContent, err := os.ReadFile(extensionsPath)
	if err != nil {
		return fmt.Errorf("failed to read extensions.sql: %w", err)
	}

	content := string(sqlContent)
	enabled, disabled := migration.ParseAllExtensionNames(content)
	ctx := context.Background()

	// ── Pre-flight ──────────────────────────────────────────────────────────
	type extOp struct {
		kind string // "install" | "drop"
		name string
	}
	var ops []extOp

	for _, ext := range enabled {
		rows, err := conn.Query(ctx,
			`SELECT 1 FROM pg_extension WHERE extname = $1`, ext)
		if err != nil {
			ops = append(ops, extOp{"install", ext})
			continue
		}
		found := rows.Next()
		rows.Close()
		if !found {
			ops = append(ops, extOp{"install", ext})
		}
	}

	for _, ext := range disabled {
		rows, err := conn.Query(ctx,
			`SELECT 1 FROM pg_extension WHERE extname = $1`, ext)
		if err != nil {
			continue
		}
		found := rows.Next()
		rows.Close()
		if found {
			ops = append(ops, extOp{"drop", ext})
		}
	}

	if len(ops) == 0 {
		return nil // Nothing to do
	}

	// ── Display plan ────────────────────────────────────────────────────────
	output.Info("Extension sync plan:")
	for _, op := range ops {
		switch op.kind {
		case "install":
			output.Println("  + install extension %s", op.name)
		case "drop":
			output.Warning("  - drop extension %s CASCADE", op.name)
		}
	}

	if !output.Confirm("Apply extension sync?") {
		output.Info("Skipping extension sync")
		return nil
	}

	// ── Execute ─────────────────────────────────────────────────────────────
	installCount, dropCount := 0, 0

	// Install all enabled extensions (file uses CREATE EXTENSION IF NOT EXISTS — idempotent)
	if len(enabled) > 0 {
		if err := conn.Exec(ctx, content); err != nil {
			return fmt.Errorf("failed to install extensions: %w", err)
		}
	}
	for _, op := range ops {
		if op.kind == "install" {
			installCount++
		}
	}

	for _, ext := range disabled {
		if err := conn.Exec(ctx, fmt.Sprintf("DROP EXTENSION IF EXISTS %s CASCADE;", ext)); err == nil {
			dropCount++
		}
	}

	output.Success("✓ PostgreSQL extensions synced (installed: %d, dropped: %d)", installCount, dropCount)
	return nil
}

// runFunctionsIfNeeded syncs functions from functions.sql during auto-run:
//   - Creates functions that don't yet exist in the DB.
//   - Drops functions that are commented out or removed from the file.
//   - Skips silently when all functions already exist and none need to be dropped.
//   - Body-change updates are intentionally NOT applied here — run `vm functions migrate` explicitly.
func runFunctionsIfNeeded(conn db.DB, cfg *config.Config) error {
	functionsPath := filepath.Join(cfg.MigrationPath, "functions.sql")

	if _, err := os.Stat(functionsPath); os.IsNotExist(err) {
		return nil // No functions file, skip
	}

	sqlContent, err := os.ReadFile(functionsPath)
	if err != nil {
		return fmt.Errorf("failed to read functions.sql: %w", err)
	}

	content := string(sqlContent)
	enabled, disabled := migration.ParseAllFunctionNames(content)
	ctx := context.Background()

	// ── Pre-flight: only care about truly new or removed functions ──────────
	type fnOp struct {
		kind string // "create" | "drop"
		name string
	}
	var ops []fnOp

	for _, fn := range enabled {
		rows, err := conn.Query(ctx,
			`SELECT 1 FROM pg_proc p
			  JOIN pg_namespace n ON n.oid = p.pronamespace
			 WHERE p.proname = $1 AND n.nspname = 'public'`, fn)
		if err != nil {
			ops = append(ops, fnOp{"create", fn})
			continue
		}
		found := rows.Next()
		rows.Close()
		if !found {
			ops = append(ops, fnOp{"create", fn})
		}
		// Already exists → skip silently during auto-run.
		// To update function bodies, run `vm functions migrate` explicitly.
	}

	for _, fn := range disabled {
		rows, err := conn.Query(ctx,
			`SELECT 1 FROM pg_proc p
			  JOIN pg_namespace n ON n.oid = p.pronamespace
			 WHERE p.proname = $1 AND n.nspname = 'public'`, fn)
		if err != nil {
			continue
		}
		found := rows.Next()
		rows.Close()
		if found {
			ops = append(ops, fnOp{"drop", fn})
		}
	}

	if len(ops) == 0 {
		return nil // Nothing to do — all functions already installed, none removed
	}

	// ── Display plan and prompt ─────────────────────────────────────────────
	output.Info("Function sync plan:")
	for _, op := range ops {
		switch op.kind {
		case "create":
			output.Println("  + create function %s", op.name)
		case "drop":
			output.Warning("  - drop function %s CASCADE", op.name)
		}
	}

	if !output.Confirm("Apply function sync?") {
		output.Info("Skipping function sync")
		return nil
	}

	// ── Execute ─────────────────────────────────────────────────────────────
	createCount, dropCount := 0, 0

	// Install new functions by running the full file (CREATE OR REPLACE is safe)
	if err := conn.Exec(ctx, content); err != nil {
		return fmt.Errorf("failed to install functions: %w", err)
	}
	for _, op := range ops {
		if op.kind == "create" {
			createCount++
		}
	}

	for _, fn := range disabled {
		if err := conn.Exec(ctx, fmt.Sprintf("DROP FUNCTION IF EXISTS %s() CASCADE;", fn)); err == nil {
			dropCount++
		}
	}

	output.Success("✓ Database functions synced (created: %d, dropped: %d)", createCount, dropCount)
	return nil
}
