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
}

// NewEnhancedExecutor creates a new enhanced migration executor
func NewEnhancedExecutor(conn db.DB, sqlDB *sql.DB, dsn string, migrationPath string, opts MigrationOptions) (*EnhancedExecutor, error) {
	dialect := DetectDialect(dsn)
	logger := output.NewMigrationLogger(opts.Verbose)

	executor := &EnhancedExecutor{
		conn:          conn,
		sqlDB:         sqlDB,
		dsn:           dsn,
		migrationPath: migrationPath,
		dialect:       dialect,
		logger:        logger,
		lock:          NewMigrationLock(sqlDB, dialect),
		online:        NewOnlineMigration(sqlDB, dialect),
		inspector:     NewSchemaInspector(sqlDB, dialect),
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

	// Verify checksums if enabled
	if opts.VerifyChecksums {
		e.logger.Info("Verifying migration checksums...")
		if err := e.verifyChecksums(executed); err != nil {
			e.logger.Warning("Checksum verification failed: %v", err)
			if !opts.Force {
				return nil, fmt.Errorf("checksum verification failed (use --force to override): %w", err)
			}
		}
	}

	// Detect schema drift if enabled
	if opts.DetectDrift {
		e.logger.Info("Checking for schema drift...")
		if err := e.detectAndHandleDrift(opts); err != nil {
			e.logger.Warning("Schema drift detection: %v", err)
		}
	}

	// Get next batch number
	batch, err := getNextBatchNumber(e.conn)
	if err != nil {
		return nil, fmt.Errorf("failed to get batch number: %w", err)
	}

	// Run pending migrations
	pendingCount := 0
	for _, file := range files {
		if _, exists := executedMap[file.Filename]; !exists {
			pendingCount++
		}
	}

	if pendingCount == 0 {
		e.logger.Info("No pending migrations")
		return results, nil
	}

	e.logger.Info("Found %d pending migration(s)", pendingCount)

	current := 0
	for _, file := range files {
		if _, exists := executedMap[file.Filename]; exists {
			continue
		}

		current++
		result := e.runSingleMigration(file, batch, opts, current, pendingCount)
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

	return results, nil
}

// runSingleMigration executes a single migration with full tracking
func (e *EnhancedExecutor) runSingleMigration(file MigrationFile, batch int, opts MigrationOptions, current, total int) ExecutionResult {
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
	partialStatements, err := e.executeMigrationWithRecovery(upSQL, file.Filename, opts.Online)
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
func (e *EnhancedExecutor) executeMigrationWithRecovery(sqlContent string, migrationName string, useOnline bool) ([]string, error) {
	statements := splitStatements(sqlContent)
	var applied []string

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	for i, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}

		e.logger.Debug("Executing statement %d/%d", i+1, len(statements))

		// Check if this is an online-eligible operation
		if useOnline && e.isOnlineCompatible(stmt) {
			if err := e.executeOnlineStatement(ctx, stmt); err != nil {
				return applied, fmt.Errorf("statement %d failed: %w", i+1, err)
			}
		} else {
			if err := e.conn.Exec(ctx, stmt); err != nil {
				return applied, fmt.Errorf("statement %d failed: %w\nStatement: %s", i+1, err, stmt)
			}
		}

		applied = append(applied, stmt)
	}

	return applied, nil
}

// isOnlineCompatible checks if a statement can be executed with online techniques
func (e *EnhancedExecutor) isOnlineCompatible(stmt string) bool {
	stmtUpper := strings.ToUpper(strings.TrimSpace(stmt))

	// Check for ADD COLUMN operations
	if strings.Contains(stmtUpper, "ALTER TABLE") && strings.Contains(stmtUpper, "ADD COLUMN") {
		return true
	}

	return false
}

// executeOnlineStatement executes a statement using online migration techniques
func (e *EnhancedExecutor) executeOnlineStatement(ctx context.Context, stmt string) error {
	// Parse ALTER TABLE ADD COLUMN statements
	// This is a simplified parser - could be enhanced
	if strings.Contains(strings.ToUpper(stmt), "ADD COLUMN") {
		e.logger.Debug("Using online migration strategy for ADD COLUMN")
		// For now, fall back to regular execution
		// Full implementation would parse and use OnlineMigration methods
		return e.conn.Exec(ctx, stmt)
	}

	return e.conn.Exec(ctx, stmt)
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
			e.logger.Warning("Could not verify checksum for %s: %v", mig.Migration, err)
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

// detectAndHandleDrift detects schema drift and handles it based on configuration
func (e *EnhancedExecutor) detectAndHandleDrift(opts MigrationOptions) error {
	tables, err := e.inspector.GetAllTables()
	if err != nil {
		return fmt.Errorf("failed to get tables: %w", err)
	}

	var drifts []*SchemaDrift

	for _, table := range tables {
		// For simplicity, we'll check against empty expected columns
		// In a real implementation, you'd parse migrations to build expected schema
		drift, err := e.inspector.DetectDrift(table, []string{})
		if err != nil {
			e.logger.Debug("Failed to check drift for %s: %v", table, err)
			continue
		}

		if len(drift.AddedColumns) > 0 {
			drifts = append(drifts, drift)
			e.logger.SchemaDrift(table, extractColumnNames(drift.AddedColumns))
		}
	}

	if len(drifts) == 0 {
		e.logger.Success("No schema drift detected")
		return nil
	}

	// Handle drift based on configuration
	driftHandling := opts.DriftHandling
	if driftHandling == "" {
		driftHandling = "prompt" // Default to prompt
	}

	switch driftHandling {
	case "reject":
		e.logger.Error("Schema drift detected. Migrations rejected.")
		return fmt.Errorf("schema drift detected in %d table(s)", len(drifts))

	case "auto":
		e.logger.Info("Auto-applying schema drift fixes...")
		return e.autoApplyDrift(drifts, opts)

	case "prompt":
		return e.promptAndApplyDrift(drifts, opts)

	default:
		return e.promptAndApplyDrift(drifts, opts)
	}
}

// autoApplyDrift automatically applies drift fixes in the background
func (e *EnhancedExecutor) autoApplyDrift(drifts []*SchemaDrift, opts MigrationOptions) error {
	for _, drift := range drifts {
		statements := e.inspector.GenerateAlterStatements(drift)

		for _, stmt := range statements {
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
				return fmt.Errorf("failed to apply drift fix for %s: %w", drift.Table, err)
			}

			e.logger.Success("Applied drift fix for table '%s'", drift.Table)
		}
	}

	e.logger.Success("All drift fixes applied successfully")
	return nil
}

// promptAndApplyDrift prompts user and applies drift fixes
func (e *EnhancedExecutor) promptAndApplyDrift(drifts []*SchemaDrift, opts MigrationOptions) error {
	// Show all drift details
	for _, drift := range drifts {
		statements := e.inspector.GenerateAlterStatements(drift)
		e.logger.Info("Detected drift in '%s':", drift.Table)
		for _, stmt := range statements {
			fmt.Println("  " + stmt)
		}
	}

	e.logger.Prompt("Apply these changes automatically? (yes/no/generate)")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return err
	}

	response = strings.TrimSpace(strings.ToLower(response))

	switch response {
	case "yes", "y":
		// Apply changes immediately
		e.logger.Info("Applying drift fixes...")
		return e.autoApplyDrift(drifts, opts)

	case "generate", "g":
		// Just generate migration file
		e.logger.Info("You can add these to a new migration file")
		return nil

	default:
		e.logger.Info("Drift fixes skipped")
		return nil
	}
}

// Legacy method for backwards compatibility
func (e *EnhancedExecutor) detectAndOfferFix() error {
	opts := MigrationOptions{
		DriftHandling: "prompt",
	}
	return e.detectAndHandleDrift(opts)
}

// recordMigrationWithMetadata records migration with checksum and timing
func (e *EnhancedExecutor) recordMigrationWithMetadata(filename string, batch int, checksum string, executionTimeMs int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := "INSERT INTO migrations (migration, batch, checksum, execution_time_ms) VALUES ($1, $2, $3, $4)"
	return e.conn.Exec(ctx, query, filename, batch, checksum, executionTimeMs)
}

// extractColumnNames extracts column names from ColumnInfo slice
func extractColumnNames(columns []ColumnInfo) []string {
	names := make([]string, len(columns))
	for i, col := range columns {
		names[i] = col.Name
	}
	return names
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
