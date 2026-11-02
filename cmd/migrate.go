package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v2"
	"github.com/vorzela/vorzela-migrate/v2/internal/config"
	"github.com/vorzela/vorzela-migrate/v2/internal/database"
	"github.com/vorzela/vorzela-migrate/v2/internal/db"
	"github.com/vorzela/vorzela-migrate/v2/internal/migration"
	"github.com/vorzela/vorzela-migrate/v2/internal/output"
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
			Usage: "use non-blocking strategies for SQL vm generates (drift repair)",
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
		&cli.BoolFlag{
			Name:  "strict",
			Usage: "fail on safety warnings (destructive SQL, missing Down section, transaction-hostile statements)",
			Value: false,
		},
		&cli.BoolFlag{
			Name:  "allow-destructive",
			Usage: "acknowledge destructive SQL/warnings when --strict is used",
			Value: false,
		},
		&cli.BoolFlag{
			Name:    "yes",
			Aliases: []string{"y"},
			Usage:   "assume yes for safe prompts (never for destructive actions); also VM_NON_INTERACTIVE=1",
			Value:   false,
		},
		&cli.StringFlag{
			Name:  "drift-handling",
			Usage: "override DRIFT_HANDLING: auto, prompt, or reject",
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

		// Non-interactive/agent mode: never block on a prompt. Safe auto-run
		// operations default to yes; destructive actions still require flags.
		nonInteractive := c.Bool("yes") || !output.IsInteractive() || envTruthy("VM_NON_INTERACTIVE")
		if nonInteractive {
			output.SetNonInteractive(true)
		}

		// Static safety check. Hard errors always block. Warnings (destructive
		// SQL, missing Down, transaction-hostile statements) only block with
		// --strict or STRICT=true.
		dryRun := c.Bool("dry-run")
		strict := c.Bool("strict") || cfg.Strict
		allowDestructive := c.Bool("allow-destructive")
		safety := migration.CheckMigrationFiles(cfg.MigrationPath)
		if len(safety.Issues) > 0 {
			output.Info("Running migration safety check...")
			for _, issue := range safety.Issues {
				if issue.Severity == migration.SafetyInfo {
					continue
				}
				loc := issue.File
				if issue.Section != "" {
					loc += " [" + issue.Section + "]"
				}
				if issue.Severity == migration.SafetyError {
					output.Error("%s: %s", loc, issue.Message)
				} else {
					output.Warning("%s: %s", loc, issue.Message)
				}
			}
		}
		if safety.HasErrors() {
			return fmt.Errorf("migration safety check failed: %d error(s); run 'vm check' for details", safety.Errors())
		}
		if strict {
			failures := safety.Warnings()
			if allowDestructive {
				failures -= safety.DestructiveCount()
			}
			if failures > 0 {
				return fmt.Errorf("strict safety check failed: %d warning(s); run 'vm check --strict' for details", failures)
			}
		}

		// Connect to database
		db, err := database.Connect(cfg.DatabaseURL)
		if err != nil {
			output.Error("Failed to connect to database: %v", err)
			return fmt.Errorf("failed to connect to database: %w", err)
		}
		defer func() { db.Close() }()

		// Hold the migration lock across auto-run prerequisites and the actual
		// migration run. The executor is told SkipLock=true so it does not try
		// to acquire the same advisory/named lock on a second connection.
		if !dryRun {
			releaseLock, lockErr := acquireMigrationLock(cfg)
			if lockErr != nil {
				output.Error("Failed to acquire migration lock: %v", lockErr)
				return lockErr
			}
			defer releaseLock()
		}

		// Auto-run extensions, functions and enums if enabled (default: true).
		// These are PostgreSQL-only — skip silently on MySQL/MariaDB.
		// On failure: do NOT continue to the next step; offer retry after the user fixes the issue.
		// Dry runs must not mutate the database.
		if !dryRun {
			if err := runAutoPrerequisites(c, &cfg, &db); err != nil {
				return err
			}
		}

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
		force := c.Bool("force")
		step := c.Int("step")
		if step < 0 {
			return fmt.Errorf("--step cannot be negative")
		}
		driftHandling := c.String("drift-handling")
		if driftHandling == "" {
			driftHandling = cfg.DriftHandling
		}
		switch driftHandling {
		case "auto", "prompt", "reject":
		default:
			return fmt.Errorf("invalid --drift-handling %q (expected auto, prompt, or reject)", driftHandling)
		}

		// Check if enhanced features are requested
		useEnhanced := enhanced || verifyChecksums || detectDrift || online || dryRun

		if useEnhanced {
			// Show environment info
			if verbose || cfg.Verbose {
				envColor := "\033[36m" // Cyan
				if cfg.IsProduction() {
					envColor = "\033[33m" // Yellow for production
				}
				fmt.Fprintf(os.Stderr, "%s[%s]%s Running in %s mode\n", envColor, cfg.Environment, "\033[0m", cfg.Environment)
			}

			// Use enhanced executor
			sqlDB, err := database.GetSQLDB(cfg.DatabaseURL)
			if err != nil {
				output.Error("Failed to get SQL DB connection: %v", err)
				return fmt.Errorf("failed to get SQL DB connection: %w", err)
			}
			defer sqlDB.Close()

			opts := migration.MigrationOptions{
				DryRun:           dryRun,
				Force:            force,
				Online:           online,
				VerifyChecksums:  verifyChecksums,
				DetectDrift:      detectDrift,
				Verbose:          verbose,
				SkipLock:         true, // acquired above for the whole command
				DriftHandling:    driftHandling,
				Step:             step,
				Strict:           strict,
				AllowDestructive: allowDestructive,
				NonInteractive:   nonInteractive,
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

		if !output.ConfirmDefault(fmt.Sprintf("Retry %s?", name), false) {
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

// runAutoPrerequisites runs the configured PostgreSQL prerequisites in order:
// extensions → functions → enums. It is shared by migrate/fresh/refresh so a
// rebuild cannot fail because triggers reference missing functions.
func runAutoPrerequisites(c *cli.Context, cfg **config.Config, dbp *db.DB) error {
	dialect := migration.DetectDialect((*cfg).DatabaseURL)
	if dialect != migration.PostgreSQL {
		if (*cfg).AutoRunExtensions || (*cfg).AutoRunFunctions || (*cfg).AutoRunEnums {
			output.Info("Skipping auto-run extensions/functions/enums (PostgreSQL only)")
		}
		return nil
	}

	if (*cfg).AutoRunExtensions {
		if err := runAutoPrerequisite(c, "extensions", "AUTO_RUN_EXTENSIONS", cfg, dbp, runExtensionsIfNeeded); err != nil {
			return err
		}
	}
	if (*cfg).AutoRunFunctions {
		if err := runAutoPrerequisite(c, "functions", "AUTO_RUN_FUNCTIONS", cfg, dbp, runFunctionsIfNeeded); err != nil {
			return err
		}
	}
	if (*cfg).AutoRunEnums {
		if err := runAutoPrerequisite(c, "enums", "AUTO_RUN_ENUMS", cfg, dbp, runEnumsIfNeeded); err != nil {
			return err
		}
	}
	return nil
}

// enumOp describes a single pending enum change for display before prompting.
type enumOp struct {
	kind string // "create" | "add-values"
	name string
	vals []string
}

// runEnumsIfNeeded syncs enum types from enums.sql:
//   - Creates each enabled enum if it does not exist yet.
//   - Adds missing values to existing enums.
//   - Never drops types based on commented template entries; use `vm enums drop`.
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
	enabled, _ := migration.ParseAllEnumNames(content)
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
		}
	}

	if !output.ConfirmDefault("Apply enum sync?", true) {
		output.Info("Skipping enum sync")
		return nil
	}

	// ── Execute ─────────────────────────────────────────────────────────────
	// Only additive changes are applied automatically. Commented-out example
	// entries in the template are NOT treated as "drop this type" — use
	// `vm enums drop` explicitly for removals.
	createCount, updateCount := 0, 0

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
		if err := execSQLScript(ctx, conn, syncSQL); err != nil {
			return fmt.Errorf("failed to sync enum '%s': %w", name, err)
		}
	}
	for _, op := range ops {
		switch op.kind {
		case "create":
			createCount++
		case "add-values":
			updateCount++
		}
	}

	output.Success("Enum types synced (created: %d, updated: %d)", createCount, updateCount)
	return nil
}

// runExtensionsIfNeeded syncs extensions from extensions.sql:
//   - Installs enabled extensions that are not yet present in pg_extension.
//   - Never drops anything automatically; use `vm extensions drop` explicitly.
//   - Prompts the user before applying additive changes.
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
	enabled, _ := migration.ParseAllExtensionNames(content)
	ctx := context.Background()

	// ── Pre-flight ──────────────────────────────────────────────────────────
	type extOp struct {
		kind string // "install"
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

	if len(ops) == 0 {
		return nil // Nothing to do
	}

	// ── Display plan ────────────────────────────────────────────────────────
	output.Info("Extension sync plan:")
	for _, op := range ops {
		output.Println("  + install extension %s", op.name)
	}

	if !output.ConfirmDefault("Apply extension sync?", true) {
		output.Info("Skipping extension sync")
		return nil
	}

	// ── Execute ─────────────────────────────────────────────────────────────
	// The file uses CREATE EXTENSION IF NOT EXISTS, so applying it is idempotent.
	// Splitting keeps behaviour predictable on both drivers and avoids sending
	// comments/disabled example lines to the server.
	statements, err := migration.SplitSQLStatements(content)
	if err != nil {
		return fmt.Errorf("failed to parse extensions.sql: %w", err)
	}
	for _, stmt := range statements {
		if err := conn.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("failed to install extensions: %w", err)
		}
	}

	output.Success("PostgreSQL extensions synced (installed: %d)", len(ops))
	return nil
}

// runFunctionsIfNeeded syncs functions from functions.sql during auto-run:
//   - Creates functions that don't yet exist in the DB.
//   - Never drops anything automatically; use `vm functions drop` explicitly.
//   - When at least one function is missing, the full file is applied
//     (CREATE OR REPLACE), so body edits in the file are picked up too.
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
	enabled, _ := migration.ParseAllFunctionNames(content)
	ctx := context.Background()

	// ── Pre-flight: only care about truly new functions ─────────────────────
	type fnOp struct {
		kind string // "create"
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

	if len(ops) == 0 {
		return nil // Nothing to do — all functions already installed
	}

	// ── Display plan and prompt ─────────────────────────────────────────────
	output.Info("Function sync plan:")
	for _, op := range ops {
		output.Println("  + create function %s", op.name)
	}

	if !output.ConfirmDefault("Apply function sync?", true) {
		output.Info("Skipping function sync")
		return nil
	}

	// ── Execute ─────────────────────────────────────────────────────────────
	// CREATE OR REPLACE makes applying the file idempotent, and the splitter
	// preserves dollar-quoted function bodies.
	statements, err := migration.SplitSQLStatements(content)
	if err != nil {
		return fmt.Errorf("failed to parse functions.sql: %w", err)
	}
	for _, stmt := range statements {
		if err := conn.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("failed to install functions: %w", err)
		}
	}

	output.Success("Database functions synced (created: %d)", len(ops))
	return nil
}
