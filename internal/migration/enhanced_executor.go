package migration

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vorzela/vorzela-migrate/internal/db"
	"github.com/vorzela/vorzela-migrate/internal/output"
)

// MigrationOptions configures migration execution
type MigrationOptions struct {
	DryRun          bool
	Force           bool
	Online          bool // Use zero-downtime techniques
	VerifyChecksums bool
	DetectDrift     bool
	Verbose         bool
	SkipLock        bool
	DriftHandling   string // auto, reject, prompt
	// Step limits how many pending migrations to run. 0 means unlimited.
	Step int
}

// ExecutionResult tracks migration execution details
type ExecutionResult struct {
	MigrationName    string
	Success          bool
	ExecutionTime    time.Duration
	Error            error
	Checksum         string
	PartiallyApplied []string // Statements that succeeded before failure
}

// EnhancedExecutor provides advanced migration execution
type EnhancedExecutor struct {
	conn          db.DB
	sqlDB         *sql.DB
	dsn           string
	migrationPath string
	dialect       Dialect
	logger        *output.MigrationLogger
	lock          *MigrationLock
	online        *OnlineMigration
	inspector     *SchemaInspector
	// handledMissingMigrations tracks files already prompted for removal in this run
	// so we don't double-prompt when both verifyChecksums and updateChecksums are called.
	handledMissingMigrations map[string]bool
}

// NewEnhancedExecutor creates a new enhanced migration executor
func NewEnhancedExecutor(conn db.DB, sqlDB *sql.DB, dsn string, migrationPath string, opts MigrationOptions) (*EnhancedExecutor, error) {
	dialect := DetectDialect(dsn)
	logger := output.NewMigrationLogger(opts.Verbose)

	executor := &EnhancedExecutor{
		conn:                     conn,
		sqlDB:                    sqlDB,
		dsn:                      dsn,
		migrationPath:            migrationPath,
		dialect:                  dialect,
		logger:                   logger,
		lock:                     NewMigrationLock(sqlDB, dialect),
		online:                   NewOnlineMigration(sqlDB, dialect),
		inspector:                NewSchemaInspector(sqlDB, dialect),
		handledMissingMigrations: make(map[string]bool),
	}

	return executor, nil
}

// RunMigrationsEnhanced runs migrations with all enhanced features
func (e *EnhancedExecutor) RunMigrationsEnhanced(opts MigrationOptions) ([]ExecutionResult, error) {
	var results []ExecutionResult

	// Acquire migration lock
	if !opts.SkipLock {
		e.logger.Debug("Acquiring migration lock...")
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()

		if err := e.lock.AcquireLock(ctx); err != nil {
			e.logger.Error("Failed to acquire lock: %v", err)
			return nil, err
		}
		defer e.lock.ReleaseLock()
		e.logger.Debug("Lock acquired")
	}

	// Get pending migrations
	files, err := getMigrationFiles(e.migrationPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read migration files: %w", err)
	}

	executed, err := getExecutedMigrations(e.conn)
	if err != nil {
		return nil, fmt.Errorf("failed to get executed migrations: %w", err)
	}

	executedMap := make(map[string]Migration)
	for _, mig := range executed {
		executedMap[mig.Migration] = mig
	}

	// Verify checksums if enabled.
	// checksumsMismatch is true when any executed file was modified after being run.
	// We still allow the user to check drift so they can discover schema differences
	// caused by the modification (e.g. columns added to a file that already ran).
	checksumsMismatch := false
	if opts.VerifyChecksums {
		e.logger.Info("Verifying migration checksums...")
		if err := e.verifyChecksums(executed); err != nil {
			checksumsMismatch = true
			e.logger.Warning("Checksum verification failed: %v", err)
			if !opts.Force {
				return nil, fmt.Errorf("checksum verification failed (use --force to override): %w", err)
			}
			// When forcing, re-hash files so future runs pass cleanly.
			e.logger.Info("Updating checksums for modified migrations...")
			if updateErr := e.updateChecksums(executed); updateErr != nil {
				e.logger.Warning("Failed to update checksums: %v", updateErr)
			} else {
				e.logger.Success("Checksums updated successfully")
			}
		}
	}

	// Count pending migrations first
	pendingCount := 0
	for _, file := range files {
		if _, exists := executedMap[file.Filename]; !exists {
			pendingCount++
		}
	}

	// Detect schema drift.
	//
	// Normal path (no checksum mismatch):
	//   pendingCount == 0            → pre-run drift check (DB is fully up-to-date)
	//   step exhausts all pending    → post-run drift (runs after migrations below)
	//   step leaves some pending     → skipped (avoid mid-run false positives)
	//
	// Checksum mismatch path:
	//   Files were modified after running. The user can opt in to drift detection
	//   to discover columns added to the DB outside of migrations. If found they
	//   can apply them immediately via ALTER TABLE; then pending migrations run.
	if opts.DetectDrift {
		if checksumsMismatch {
			e.logger.Warning("Migration files were modified (checksum mismatch) — expected schema may be imprecise.")
			if e.promptYesNo("Run drift check anyway to look for added columns?") {
				e.logger.Info("Checking for schema drift...")
				executedFilenames := make([]string, 0, len(executed))
				for _, m := range executed {
					executedFilenames = append(executedFilenames, m.Migration)
				}
				applied, driftErr := e.detectAndHandleDrift(opts, executedFilenames)
				if driftErr != nil {
					// error only arises from apply failure; warn and continue
					e.logger.Warning("Schema drift check: %v", driftErr)
				}
				if applied {
					// Columns were added — update checksums so the next run starts clean.
					e.logger.Info("Drift applied successfully — updating checksums...")
					if updateErr := e.updateChecksums(executed); updateErr != nil {
						e.logger.Warning("Failed to update checksums: %v", updateErr)
					} else {
						e.logger.Success("Checksums updated")
					}
				}
				// Columns applied (or user skipped) — fall through to pending migrations.
			}
			// Whether user said yes or no: always continue to pending migrations.
		} else {
			willExhaustPending := opts.Step == 0 || opts.Step >= pendingCount
			if pendingCount > 0 && !willExhaustPending {
				remaining := pendingCount - opts.Step
				e.logger.Debug("Skipping drift detection - %d pending migration(s) will remain after --step %d", remaining, opts.Step)
			} else if pendingCount == 0 {
				// All up-to-date: run drift against current DB
				e.logger.Info("Checking for schema drift...")
				executedFilenames := make([]string, 0, len(executed))
				for _, m := range executed {
					executedFilenames = append(executedFilenames, m.Migration)
				}
				if _, err := e.detectAndHandleDrift(opts, executedFilenames); err != nil {
					e.logger.Warning("Schema drift detection: %v", err)
				}
			}
			// When willExhaustPending && pendingCount > 0: drift runs AFTER migrations below
		}
	}

	// Get next batch number
	batch, err := getNextBatchNumber(e.conn)
	if err != nil {
		return nil, fmt.Errorf("failed to get batch number: %w", err)
	}

	if pendingCount == 0 {
		e.logger.Info("No pending migrations")
		return results, nil
	}

	if opts.Step > 0 && opts.Step < pendingCount {
		e.logger.Info("Found %d pending migration(s), running %d (--step %d)", pendingCount, opts.Step, opts.Step)
	} else {
		e.logger.Info("Found %d pending migration(s)", pendingCount)
	}

	stepLimit := opts.Step // 0 = unlimited
	current := 0
	for _, file := range files {
		if _, exists := executedMap[file.Filename]; exists {
			continue
		}
		if stepLimit > 0 && current >= stepLimit {
			break
		}

		current++
		result := e.runSingleMigration(file, batch, opts)
		results = append(results, result)

		if !result.Success {
			e.logger.Error("Migration failed, stopping execution")
			break
		}
	}

	// Print summary
	successful := 0
	failed := 0
	for _, r := range results {
		if r.Success {
			successful++
		} else {
			failed++
		}
	}

	e.logger.Summary(successful, failed, len(results))

	if failed > 0 {
		return results, fmt.Errorf("%d migration(s) failed", failed)
	}

	// Drift detection post-run: runs when --step covered all pending migrations
	// and checksum files are trustworthy (no modifications detected).
	if opts.DetectDrift && !checksumsMismatch {
		willExhaustPending := opts.Step == 0 || opts.Step >= pendingCount
		if willExhaustPending && pendingCount > 0 && failed == 0 {
			e.logger.Info("Checking for schema drift...")
			// Include just-executed migrations in expected schema
			allExecuted, _ := getExecutedMigrations(e.conn)
			executedFilenames := make([]string, 0, len(allExecuted))
			for _, m := range allExecuted {
				executedFilenames = append(executedFilenames, m.Migration)
			}
			if _, err := e.detectAndHandleDrift(opts, executedFilenames); err != nil {
				e.logger.Warning("Schema drift detection: %v", err)
			}
		}
	}

	return results, nil
}

// runSingleMigration executes a single migration with full tracking
func (e *EnhancedExecutor) runSingleMigration(file MigrationFile, batch int, opts MigrationOptions) ExecutionResult {
	result := ExecutionResult{
		MigrationName: file.Filename,
		Success:       false,
	}

	startTime := time.Now()
	e.logger.Migration(file.Filename, "")

	// Calculate checksum
	filePath := filepath.Join(e.migrationPath, file.Filename)
	checksum, err := CalculateChecksum(filePath)
	if err != nil {
		result.Error = fmt.Errorf("failed to calculate checksum: %w", err)
		e.logger.MigrationFailed(file.Filename, result.Error)
		return result
	}
	result.Checksum = checksum

	// Read migration file
	content, err := os.ReadFile(filePath)
	if err != nil {
		result.Error = fmt.Errorf("failed to read file: %w", err)
		e.logger.MigrationFailed(file.Filename, result.Error)
		return result
	}

	// Extract UP section
	upSQL := extractSection(string(content), "Up")
	if upSQL == "" {
		result.Error = fmt.Errorf("no UP section found")
		e.logger.MigrationFailed(file.Filename, result.Error)
		return result
	}

	if opts.DryRun {
		e.logger.Info("DRY RUN - Would execute:\n%s", upSQL)
		result.Success = true
		result.ExecutionTime = time.Since(startTime)
		return result
	}

	// Execute with recovery
	partialStatements, err := e.executeMigrationWithRecovery(upSQL)
	result.PartiallyApplied = partialStatements

	if err != nil {
		result.Error = err
		result.ExecutionTime = time.Since(startTime)
		e.logger.MigrationFailed(file.Filename, err)
		e.logger.Warning("Partially applied statements: %v", partialStatements)
		return result
	}

	// Record migration with checksum and execution time
	executionTimeMs := time.Since(startTime).Milliseconds()
	err = e.recordMigrationWithMetadata(file.Filename, batch, checksum, executionTimeMs)
	if err != nil {
		result.Error = fmt.Errorf("failed to record migration: %w", err)
		e.logger.Error("Migration executed but failed to record: %v", err)
		return result
	}

	result.Success = true
	result.ExecutionTime = time.Since(startTime)
	e.logger.MigrationComplete(file.Filename, result.ExecutionTime)

	return result
}

// executeMigrationWithRecovery executes migration and tracks partial success
func (e *EnhancedExecutor) executeMigrationWithRecovery(sqlContent string) ([]string, error) {
	statements := splitStatements(sqlContent)
	var applied []string

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Start a proper database transaction
	tx, err := e.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Ensure rollback on panic or error
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	for i, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}

		// Execute within the transaction
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			// Rollback the transaction
			tx.Rollback()
			return applied, e.enhanceError(err, i+1, stmt)
		}

		applied = append(applied, stmt)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return applied, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return applied, nil
}

// enhanceError provides helpful error messages for common migration issues
func (e *EnhancedExecutor) enhanceError(err error, statementNum int, stmt string) error {
	errStr := err.Error()
	errStrLower := strings.ToLower(errStr)

	// Check for missing function errors (case-insensitive)
	if strings.Contains(errStrLower, "does not exist") && strings.Contains(errStrLower, "function") {
		// Extract function name if possible
		functionName := ""
		if strings.Contains(errStr, "auto_update_timestamp") {
			functionName = "auto_update_timestamp()"
		} else if strings.Contains(errStr, "protect_soft_deleted") {
			functionName = "protect_soft_deleted()"
		} else if strings.Contains(errStr, "auto_update_with_soft_delete_protection") {
			functionName = "auto_update_with_soft_delete_protection()"
		} else if strings.Contains(errStr, "prevent_hard_delete") {
			functionName = "prevent_hard_delete()"
		}

		if functionName != "" {
			return fmt.Errorf("statement %d failed: %w\n\n❌ MISSING DATABASE FUNCTION\n\n"+
				"The migration uses trigger function '%s' which doesn't exist in the database.\n\n"+
				"💡 SOLUTION:\n"+
				"   1. Run: vm functions migrate\n"+
				"   2. Then run: vm migrate\n\n"+
				"This installs the required database functions before running migrations.\n"+
				"See documentation: vm functions --help\n\nStatement: %s",
				statementNum, err, functionName, stmt)
		}
	}

	// Check for missing table/relation errors (case-insensitive)
	if strings.Contains(errStrLower, "does not exist") && (strings.Contains(errStrLower, "relation") || strings.Contains(errStrLower, "table")) {
		return fmt.Errorf("statement %d failed: %w\n\n"+
			"💡 TIP: This table might be created in a migration that hasn't run yet.\n"+
			"   Check your migration order and dependencies.\n\nStatement: %s",
			statementNum, err, stmt)
	}

	// Default error with statement context
	return fmt.Errorf("statement %d failed: %w\nStatement: %s", statementNum, err, stmt)
}

// splitStatements splits SQL content into individual statements
func splitStatements(sqlContent string) []string {
	var statements []string
	var current strings.Builder
	inString := false
	var stringChar rune

	lines := strings.Split(sqlContent, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip comments
		if strings.HasPrefix(trimmed, "--") {
			continue
		}

		for _, char := range line {
			if char == '\'' || char == '"' {
				if !inString {
					inString = true
					stringChar = char
				} else if char == stringChar {
					inString = false
				}
			}

			current.WriteRune(char)

			if char == ';' && !inString {
				stmt := strings.TrimSpace(current.String())
				if stmt != "" && stmt != ";" {
					statements = append(statements, stmt)
				}
				current.Reset()
			}
		}

		current.WriteRune('\n')
	}

	// Add remaining content
	if current.Len() > 0 {
		stmt := strings.TrimSpace(current.String())
		if stmt != "" && stmt != ";" {
			statements = append(statements, stmt)
		}
	}

	return statements
}

// verifyChecksums verifies that executed migrations haven't been modified
func (e *EnhancedExecutor) verifyChecksums(executed []Migration) error {
	var mismatches []string

	for _, mig := range executed {
		if mig.Checksum == "" {
			continue // Migration was run before checksum feature
		}

		filePath := filepath.Join(e.migrationPath, mig.Migration)
		match, err := ChecksumMatch(filePath, mig.Checksum)

		if err != nil {
			if os.IsNotExist(err) && !e.handledMissingMigrations[mig.Migration] {
				e.handledMissingMigrations[mig.Migration] = true
				e.logger.Warning("Migration file '%s' no longer exists on disk", mig.Migration)
				if e.promptYesNo(fmt.Sprintf("Remove '%s' from the migration table?", mig.Migration)) {
					if removeErr := removeMigrationRecord(e.conn, mig); removeErr != nil {
						e.logger.Warning("Failed to remove migration record: %v", removeErr)
					} else {
						e.logger.Success("Removed '%s' from migration table", mig.Migration)
					}
				}
			} else if !os.IsNotExist(err) {
				e.logger.Warning("Could not verify checksum for %s: %v", mig.Migration, err)
			}
			continue
		}

		if !match {
			mismatches = append(mismatches, mig.Migration)
			e.logger.Warning("Checksum mismatch: %s", mig.Migration)
		}
	}

	if len(mismatches) > 0 {
		return fmt.Errorf("checksum mismatch detected in %d migration(s): %s",
			len(mismatches), strings.Join(mismatches, ", "))
	}

	e.logger.Success("All checksums verified")
	return nil
}

// updateChecksums updates checksums for executed migrations to match current file state
func (e *EnhancedExecutor) updateChecksums(executed []Migration) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var updated []string

	for _, mig := range executed {
		filePath := filepath.Join(e.migrationPath, mig.Migration)

		// Calculate current checksum
		currentChecksum, err := CalculateChecksum(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				if !e.handledMissingMigrations[mig.Migration] {
					e.handledMissingMigrations[mig.Migration] = true
					e.logger.Warning("Migration file '%s' no longer exists on disk", mig.Migration)
					if e.promptYesNo(fmt.Sprintf("Remove '%s' from the migration table?", mig.Migration)) {
						if removeErr := removeMigrationRecord(e.conn, mig); removeErr != nil {
							e.logger.Warning("Failed to remove migration record: %v", removeErr)
						} else {
							e.logger.Success("Removed '%s' from migration table", mig.Migration)
						}
					}
				}
				// silently skip — user was already prompted (or just acted)
			} else {
				e.logger.Warning("Could not calculate checksum for %s: %v", mig.Migration, err)
			}
			continue
		}

		// Skip if checksum matches
		if currentChecksum == mig.Checksum {
			continue
		}

		// Update checksum in database
		query := "UPDATE migrations SET checksum = $1 WHERE migration = $2"
		if err := e.conn.Exec(ctx, query, currentChecksum, mig.Migration); err != nil {
			return fmt.Errorf("failed to update checksum for %s: %w", mig.Migration, err)
		}

		updated = append(updated, mig.Migration)
		e.logger.Info("✓ Updated checksum: %s", mig.Migration)
	}

	if len(updated) == 0 {
		e.logger.Info("No checksums needed updating")
	}

	return nil
}

// detectAndHandleDrift detects schema drift and handles it based on configuration.
// executedFiles is the list of migration filenames that have already been run.
// Returns (true, nil) when drift was detected AND at least one fix was successfully applied.
func (e *EnhancedExecutor) detectAndHandleDrift(opts MigrationOptions, executedFiles []string) (bool, error) {
	// Build expected schema (with type info) from executed migration files
	expectedSchema := buildExpectedColumnDefsFromFiles(e.migrationPath, executedFiles)
	expectedIndexes := buildExpectedIndexesFromFiles(e.migrationPath, executedFiles)
	expectedTriggers := buildExpectedTriggersFromFiles(e.migrationPath, executedFiles)
	expectedConstraints := buildExpectedConstraintsFromFiles(e.migrationPath, executedFiles)

	tables, err := e.inspector.GetAllTables()
	if err != nil {
		return false, fmt.Errorf("failed to get tables: %w", err)
	}

	var drifts []*SchemaDrift
	checkedCount := 0

	for _, table := range tables {
		tblKey := strings.ToLower(table)

		expectedCols := expectedSchema[tblKey]
		_, hasExpectedIndexes := expectedIndexes[tblKey]
		_, hasExpectedTriggers := expectedTriggers[tblKey]
		_, hasExpectedConstraints := expectedConstraints[tblKey]

		// Skip tables not referenced in any migration file — these are either
		// manually created tables or extension tables not caught by pg_depend.
		if len(expectedCols) == 0 && !hasExpectedIndexes && !hasExpectedTriggers && !hasExpectedConstraints {
			e.logger.Debug("Skipping drift check for '%s' — not defined in any migration file", table)
			continue
		}

		checkedCount++

		// --- Column drift (both directions) ---
		drift, err := e.inspector.DetectDrift(table, expectedCols)
		if err != nil {
			e.logger.Debug("Failed to check column drift for %s: %v", table, err)
			continue
		}

		if len(drift.AddedColumns) > 0 {
			var names []string
			for _, c := range drift.AddedColumns {
				names = append(names, c.Name)
			}
			e.logger.Warning("Schema drift in table '%s': columns in DB not defined in any migration (will be dropped): %v",
				table, names)
			e.logger.Warning("Action required: remove definitions for column(s) %v in table '%s' from your migration files, or create a new migration to drop them.",
				names, table)

			// Detect indexes in the DB that cover only orphaned columns — they must
			// be dropped before the columns themselves can be removed.
			orphanedSet := make(map[string]bool, len(drift.AddedColumns))
			for _, c := range drift.AddedColumns {
				orphanedSet[strings.ToLower(c.Name)] = true
			}
			actualIdxs, idxErr := e.inspector.GetTableIndexes(table)
			if idxErr != nil {
				e.logger.Debug("Failed to fetch indexes for orphaned-column check on %s: %v", table, idxErr)
			} else {
				for _, idx := range actualIdxs {
					// Include the index if ALL its columns are orphaned.
					if indexCoveredByOrphans(idx, orphanedSet) {
						e.logger.Warning("Index '%s' on table '%s' covers only orphaned column(s) and will be dropped: %v",
							idx.Name, table, idx.Columns)
						drift.ExtraIndexes = append(drift.ExtraIndexes, idx)
					}
				}
			}
		}
		if len(drift.MissingColumns) > 0 {
			var names []string
			for _, c := range drift.MissingColumns {
				names = append(names, c.Name)
			}
			e.logger.Warning("Schema drift in table '%s': columns defined in migrations but missing from DB: %v",
				table, names)
		}

		// --- Index drift: indexes defined in migrations but missing from DB ---
		if wantIdxs, ok := expectedIndexes[tblKey]; ok && len(wantIdxs) > 0 {
			actualIdxs, idxErr := e.inspector.GetTableIndexes(table)
			if idxErr != nil {
				e.logger.Debug("Failed to check index drift for %s: %v", table, idxErr)
			} else {
				actualSet := make(map[string]bool)
				for _, ai := range actualIdxs {
					actualSet[strings.ToLower(ai.Name)] = true
				}
				for _, wi := range wantIdxs {
					if !actualSet[strings.ToLower(wi.Name)] {
						e.logger.Warning("Missing index '%s' on table '%s'", wi.Name, table)
						drift.MissingIndexes = append(drift.MissingIndexes, wi)
					}
				}
			}
		}

		// --- Trigger drift: triggers defined in migrations but missing from DB ---
		if wantTrigs, ok := expectedTriggers[tblKey]; ok && len(wantTrigs) > 0 {
			actualTrigs, trigErr := e.inspector.GetTableTriggers(table)
			if trigErr != nil {
				e.logger.Debug("Failed to check trigger drift for %s: %v", table, trigErr)
			} else {
				actualSet := make(map[string]bool)
				for _, at := range actualTrigs {
					actualSet[strings.ToLower(at.Name)] = true
				}
				for _, wt := range wantTrigs {
					if !actualSet[strings.ToLower(wt.Name)] {
						e.logger.Warning("Missing trigger '%s' on table '%s'", wt.Name, table)
						drift.MissingTriggers = append(drift.MissingTriggers, wt)
					}
				}
			}
		}

		// --- FK constraint drift: FK constraints defined in migrations but missing from DB ---
		if wantCIs, ok := expectedConstraints[tblKey]; ok && len(wantCIs) > 0 {
			actualCIs, ciErr := e.inspector.GetTableConstraints(table)
			if ciErr != nil {
				e.logger.Debug("Failed to check FK constraint drift for %s: %v", table, ciErr)
			} else {
				// Match by content (columns + ref_table), not by name, to handle
				// Postgres auto-naming vs our fk_ convention.
				actualSet := make(map[string]bool)
				for _, ai := range actualCIs {
					actualSet[constraintKey(ai)] = true
				}
				for _, wi := range wantCIs {
					if !actualSet[constraintKey(wi)] {
						e.logger.Warning("Missing FK constraint '%s' on table '%s' (%v → %s.%v)",
							wi.Name, table, wi.Columns, wi.RefTable, wi.RefColumns)
						drift.MissingConstraints = append(drift.MissingConstraints, wi)
					}
				}
			}
		}

		if len(drift.AddedColumns) > 0 || len(drift.MissingColumns) > 0 ||
			len(drift.MissingIndexes) > 0 || len(drift.ExtraIndexes) > 0 ||
			len(drift.MissingTriggers) > 0 || len(drift.MissingConstraints) > 0 {
			drifts = append(drifts, drift)
		}
	}

	if len(drifts) == 0 {
		e.logger.Success("No schema drift detected (%d table(s) checked)", checkedCount)
		return false, nil
	}

	// Handle drift based on configuration
	driftHandling := opts.DriftHandling
	if driftHandling == "" {
		driftHandling = "prompt" // Default to interactive prompt
	}

	switch driftHandling {
	case "reject":
		e.logger.Error("Schema drift detected. Migrations rejected.")
		return false, fmt.Errorf("schema drift detected in %d table(s)", len(drifts))

	case "auto":
		e.logger.Info("Auto-applying schema drift fixes...")
		return e.autoApplyDrift(drifts, opts)

	case "prompt":
		return e.promptAndApplyDrift(drifts, opts)

	default:
		return e.promptAndApplyDrift(drifts, opts)
	}
}

// autoApplyDrift automatically applies drift fixes in the background.
// Returns (true, nil) when at least one fix statement was executed successfully.
func (e *EnhancedExecutor) autoApplyDrift(drifts []*SchemaDrift, opts MigrationOptions) (bool, error) {
	applied := false
	for _, drift := range drifts {
		statements := e.inspector.GenerateAllStatements(drift)

		for _, stmt := range statements {
			// Advisory comments (UNIQUE / NOT NULL warnings) are informational only
			// and must not be executed as SQL.
			if strings.HasPrefix(strings.TrimSpace(stmt), "--") {
				e.logger.Info("Advisory: %s", stmt)
				continue
			}

			e.logger.Info("Auto-applying: %s", stmt)

			if opts.DryRun {
				e.logger.Info("DRY RUN - Would execute: %s", stmt)
				continue
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			err := e.conn.Exec(ctx, stmt)
			cancel()

			if err != nil {
				e.logger.Error("Failed to apply drift fix: %v", err)
				return applied, fmt.Errorf("failed to apply drift fix for %s: %w", drift.Table, err)
			}

			applied = true
			e.logger.Success("Applied drift fix for table '%s'", drift.Table)
		}
	}

	if applied {
		e.logger.Success("All drift fixes applied successfully")
	}
	return applied, nil
}

// promptYesNo prints a question and reads a yes/no answer from stdin.
// Returns true for "yes"/"y", false for anything else or on read error.
func (e *EnhancedExecutor) promptYesNo(question string) bool {
	e.logger.Prompt(question + " (yes/no)")
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "yes" || response == "y"
}

// promptAndApplyDrift prompts user and applies drift fixes.
// Returns (true, nil) when the user chose yes and all fix statements executed successfully.
func (e *EnhancedExecutor) promptAndApplyDrift(drifts []*SchemaDrift, opts MigrationOptions) (bool, error) {
	// Show all drift details
	for _, drift := range drifts {
		statements := e.inspector.GenerateAllStatements(drift)
		if len(statements) > 0 {
			e.logger.Info("Detected drift in '%s':", drift.Table)
			for _, stmt := range statements {
				fmt.Println("  " + stmt)
			}
		}
	}

	e.logger.Prompt("Apply these changes automatically? (yes/no/generate)")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	response = strings.TrimSpace(strings.ToLower(response))

	switch response {
	case "yes", "y":
		// Apply changes immediately
		e.logger.Info("Applying drift fixes...")
		return e.autoApplyDrift(drifts, opts)

	case "generate", "g":
		filename, err := e.generateDriftMigration(drifts)
		if err != nil {
			e.logger.Error("Failed to generate migration file: %v", err)
			return false, err
		}
		e.logger.Success("Generated migration file: %s", filename)
		e.logger.Info("Review it then run 'vm migrate' to apply.")
		return false, nil

	default:
		e.logger.Info("Drift fixes skipped")
		return false, nil
	}
}

// generateDriftMigration creates a timestamped migration file that fixes all
// detected drift: ALTER TABLE ADD COLUMN for missing columns, CREATE INDEX for
// missing indexes, and advisory comments for missing triggers.
// The Down section contains the exact reversal (DROP COLUMN / DROP INDEX).
// Returns the generated filename.
func (e *EnhancedExecutor) generateDriftMigration(drifts []*SchemaDrift) (string, error) {
	timestamp := time.Now().Unix()
	filename := fmt.Sprintf("%d_fix_schema_drift.sql", timestamp)
	fullPath := filepath.Join(e.migrationPath, filename)

	var up, down strings.Builder

	for _, drift := range drifts {
		// --- Up: ADD missing columns ---
		for _, col := range drift.MissingColumns {
			if col.Type == "" {
				up.WriteString(fmt.Sprintf(
					"-- MISSING COLUMN: %s.%s — type unknown, add manually.\n",
					drift.Table, col.Name))
				continue
			}
			// UNIQUE columns must be handled manually — backfilling is required.
			if col.IsUnique {
				up.WriteString(fmt.Sprintf(
					"-- UNIQUE COLUMN: %s.%s — cannot safely auto-add to a populated table.\n"+
						"-- Create an add_%s_to_%s migration file to backfill values and add the UNIQUE constraint manually.\n",
					drift.Table, col.Name, col.Name, drift.Table))
				continue
			}
			// Build NOT NULL DEFAULT clause when the schema declares it non-nullable.
			colDef := col.Type
			if !col.Nullable {
				if col.Default.Valid {
					colDef += " NOT NULL DEFAULT " + col.Default.String
				} else {
					up.WriteString(fmt.Sprintf(
						"-- NOT NULL COLUMN: %s.%s (%s) has no DEFAULT value.\n"+
							"-- Create an add_%s_to_%s migration file to supply a DEFAULT before enforcing NOT NULL.\n",
						drift.Table, col.Name, col.Type, col.Name, drift.Table))
					continue
				}
			}
			up.WriteString(fmt.Sprintf(
				"ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s %s;\n",
				drift.Table, col.Name, colDef))
			// Down: DROP the column we just added.
			down.WriteString(fmt.Sprintf(
				"ALTER TABLE %s DROP COLUMN IF EXISTS %s;\n",
				drift.Table, col.Name))
		}

		// --- Up: columns that exist in DB but not migrations (AddedColumns) ---
		// These are orphaned columns — drop them to bring the DB in line with
		// the migration-defined schema.  The Down section restores them so the
		// migration is safely reversible.

		// Drop covering indexes first so the DROP COLUMN statements are not blocked.
		for _, idx := range drift.ExtraIndexes {
			up.WriteString(fmt.Sprintf(
				"DROP INDEX IF EXISTS %s; -- index on orphaned column(s) %s\n",
				idx.Name, strings.Join(idx.Columns, ", ")))
			// Down: recreate the index (best-effort)
			down.WriteString(generateCreateIndexSQL(idx, e.inspector.dialect) + "\n")
		}

		for _, col := range drift.AddedColumns {
			up.WriteString(fmt.Sprintf(
				"ALTER TABLE %s DROP COLUMN IF EXISTS %s;\n",
				drift.Table, col.Name))
			up.WriteString(fmt.Sprintf(
				"-- NOTE: also remove the definition of column '%s' in table '%s' from your migration files.\n",
				col.Name, drift.Table))
			// Down: restore the dropped column (best-effort; type comes from DB introspection)
			nullable := ""
			if !col.Nullable {
				nullable = " NOT NULL"
			}
			def := ""
			if col.Default.Valid {
				def = fmt.Sprintf(" DEFAULT %s", col.Default.String)
			}
			down.WriteString(fmt.Sprintf(
				"ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s %s%s%s;\n",
				drift.Table, col.Name, col.Type, nullable, def))
		}

		// --- Up: CREATE missing indexes ---
		for _, idx := range drift.MissingIndexes {
			up.WriteString(generateCreateIndexSQL(idx, e.inspector.dialect) + "\n")
			down.WriteString(fmt.Sprintf("DROP INDEX IF EXISTS %s;\n", idx.Name))
		}

		// --- Up: ADD missing FK constraints ---
		for _, ci := range drift.MissingConstraints {
			name := ci.Name
			if name == "" {
				name = generateConstraintName(ci.TableName, ci.Columns)
			}
			cols := strings.Join(ci.Columns, ", ")
			refCols := strings.Join(ci.RefColumns, ", ")
			stmt := fmt.Sprintf(
				"ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)",
				ci.TableName, name, cols, ci.RefTable, refCols)
			if ci.OnDelete != "" && ci.OnDelete != "NO ACTION" {
				stmt += " ON DELETE " + ci.OnDelete
			}
			if ci.OnUpdate != "" && ci.OnUpdate != "NO ACTION" {
				stmt += " ON UPDATE " + ci.OnUpdate
			}
			up.WriteString(stmt + ";\n")
			// Down: drop the constraint we just added.
			if e.inspector.dialect == MySQL || e.inspector.dialect == MariaDB {
				down.WriteString(fmt.Sprintf(
					"ALTER TABLE %s DROP FOREIGN KEY %s;\n", ci.TableName, name))
			} else {
				down.WriteString(fmt.Sprintf(
					"ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s;\n", ci.TableName, name))
			}
		}

		// --- Up: advisory comments for missing triggers ---
		for _, trig := range drift.MissingTriggers {
			up.WriteString(fmt.Sprintf(
				"-- MISSING TRIGGER: %s (%s %s ON %s) — re-apply the original SQL here.\n",
				trig.Name, trig.Timing, trig.Event, trig.TableName))
		}
	}

	content := fmt.Sprintf("-- Up\n%s\n-- Down\n%s", up.String(), down.String())

	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write migration file: %w", err)
	}

	return filename, nil
}

// recordMigrationWithMetadata records migration with checksum and timing
func (e *EnhancedExecutor) recordMigrationWithMetadata(filename string, batch int, checksum string, executionTimeMs int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := "INSERT INTO migrations (migration, batch, checksum, execution_time_ms) VALUES ($1, $2, $3, $4)"
	return e.conn.Exec(ctx, query, filename, batch, checksum, executionTimeMs)
}

// RollbackWithWarnings performs rollback with safety warnings
func (e *EnhancedExecutor) RollbackWithWarnings(steps int, opts MigrationOptions) ([]ExecutionResult, error) {
	var results []ExecutionResult

	// Get executed migrations
	executed, err := getExecutedMigrations(e.conn)
	if err != nil {
		return nil, err
	}

	if len(executed) == 0 {
		e.logger.Info("No migrations to rollback")
		return results, nil
	}

	// Determine migrations to rollback
	toRollback := selectMigrationsToRollback(executed, steps)

	if len(toRollback) == 0 {
		e.logger.Info("No migrations selected for rollback")
		return results, nil
	}

	// Warning prompt
	e.logger.Warning("About to rollback %d migration(s):", len(toRollback))
	for _, mig := range toRollback {
		fmt.Printf("  - %s (batch %d)\n", mig.Migration, mig.Batch)
	}

	if !opts.Force {
		e.logger.Prompt("Continue with rollback? (yes/no)")

		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}

		response = strings.TrimSpace(strings.ToLower(response))
		if response != "yes" && response != "y" {
			e.logger.Info("Rollback cancelled")
			return results, nil
		}
	}

	// Acquire lock
	if !opts.SkipLock {
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()

		if err := e.lock.AcquireLock(ctx); err != nil {
			return nil, err
		}
		defer e.lock.ReleaseLock()
	}

	// Execute rollbacks
	for i := len(toRollback) - 1; i >= 0; i-- {
		mig := toRollback[i]
		result := e.rollbackSingleMigration(mig, opts)
		results = append(results, result)

		if !result.Success && !opts.Force {
			e.logger.Error("Rollback failed, stopping")
			break
		}
	}

	return results, nil
}

// rollbackSingleMigration rolls back a single migration
func (e *EnhancedExecutor) rollbackSingleMigration(mig Migration, opts MigrationOptions) ExecutionResult {
	result := ExecutionResult{
		MigrationName: mig.Migration,
		Success:       false,
	}

	startTime := time.Now()
	e.logger.Migration(mig.Migration, fmt.Sprintf("batch %d", mig.Batch))

	filePath := filepath.Join(e.migrationPath, mig.Migration)
	content, err := os.ReadFile(filePath)
	if err != nil {
		result.Error = fmt.Errorf("failed to read file: %w", err)
		e.logger.MigrationFailed(mig.Migration, result.Error)
		return result
	}

	downSQL := extractSection(string(content), "Down")
	if downSQL == "" {
		result.Error = fmt.Errorf("no DOWN section found")
		e.logger.MigrationFailed(mig.Migration, result.Error)
		return result
	}

	if opts.DryRun {
		e.logger.Info("DRY RUN - Would execute:\n%s", downSQL)
		result.Success = true
		result.ExecutionTime = time.Since(startTime)
		return result
	}

	// Execute
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := e.conn.Exec(ctx, downSQL); err != nil {
		result.Error = err
		result.ExecutionTime = time.Since(startTime)
		e.logger.MigrationFailed(mig.Migration, err)
		return result
	}

	// Remove record
	if err := removeMigrationRecord(e.conn, mig); err != nil {
		result.Error = fmt.Errorf("rollback succeeded but failed to remove record: %w", err)
		e.logger.Warning("%v", result.Error)
	}

	result.Success = true
	result.ExecutionTime = time.Since(startTime)
	e.logger.MigrationComplete(mig.Migration, result.ExecutionTime)

	return result
}

// selectMigrationsToRollback selects migrations based on batch steps
func selectMigrationsToRollback(executed []Migration, steps int) []Migration {
	if len(executed) == 0 {
		return nil
	}

	// Get unique batches in descending order
	batchSet := make(map[int]bool)
	for _, m := range executed {
		batchSet[m.Batch] = true
	}

	var batches []int
	for b := range batchSet {
		batches = append(batches, b)
	}

	// Sort descending
	for i := 0; i < len(batches)-1; i++ {
		for j := i + 1; j < len(batches); j++ {
			if batches[i] < batches[j] {
				batches[i], batches[j] = batches[j], batches[i]
			}
		}
	}

	// Select batches to rollback
	batchesToRollback := make(map[int]bool)
	for i := 0; i < len(batches) && i < steps; i++ {
		batchesToRollback[batches[i]] = true
	}

	// Select migrations
	var toRollback []Migration
	for _, mig := range executed {
		if batchesToRollback[mig.Batch] {
			toRollback = append(toRollback, mig)
		}
	}

	return toRollback
}
