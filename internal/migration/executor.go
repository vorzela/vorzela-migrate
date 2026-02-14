package migration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vorzela/vorzela-migrate/internal/db"
	"github.com/vorzela/vorzela-migrate/internal/output"
)

// InitMigrationTable creates the migrations table if it doesn't exist
// It auto-detects the database type from the DSN to use the correct SQL dialect
func InitMigrationTable(conn db.DB, dsn string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dialect := DetectDialect(dsn)
	query := CreateMigrationTableSQL(dialect)

	if err := conn.Exec(ctx, query); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	return nil
}

// RunMigrations runs all pending migrations
func RunMigrations(conn db.DB, migrationPath string, dsn string) (int, error) {
	// Get all migration files
	files, err := getMigrationFiles(migrationPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read migration files: %w", err)
	}

	// Sort by timestamp
	sort.Slice(files, func(i, j int) bool {
		return files[i].Timestamp < files[j].Timestamp
	})

	// Get executed migrations
	dialect := DetectDialect(dsn)
	executed, err := getExecutedMigrations(conn, dialect)
	if err != nil {
		return 0, fmt.Errorf("failed to get executed migrations: %w", err)
	}

	executedMap := make(map[string]bool)
	for _, mig := range executed {
		executedMap[mig.Migration] = true
	}

	// Get current batch number
	batch, err := getNextBatchNumber(conn, dialect)
	if err != nil {
		return 0, fmt.Errorf("failed to get batch number: %w", err)
	}

	// Run pending migrations
	count := 0
	for _, file := range files {
		if executedMap[file.Filename] {
			continue
		}

		// Read migration file
		content, err := os.ReadFile(filepath.Join(migrationPath, file.Filename))
		if err != nil {
			return count, fmt.Errorf("❌ FAILED to read migration file %s: %v\n   Reason: %w\n   Action: Fix the file path and run migrate again", file.Filename, err, err)
		}

		// Extract UP part
		upSQL := extractSection(string(content), "Up")
		if upSQL == "" {
			output.Warning("No UP section found in migration %s, skipping", file.Filename)
			continue
		}

		// Execute migration with better error handling
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err = conn.Exec(ctx, upSQL)
		cancel()

		if err != nil {
			// Migration failed - provide detailed error message
			errorMsg := fmt.Sprintf("❌ MIGRATION FAILED: %s\n", file.Filename)
			errorMsg += fmt.Sprintf("   Reason: %v\n", err)
			errorMsg += "   Status: Transaction automatically rolled back\n"
			errorMsg += fmt.Sprintf("   Action: Fix the SQL in %s and run migrate again\n", file.Filename)
			errorMsg += "   Note: Failed migration was NOT recorded in database\n"

			output.Error(errorMsg)
			return count, fmt.Errorf("%s", errorMsg)
		}

		// Record migration
		err = recordMigration(conn, file.Filename, batch, dialect)
		if err != nil {
			return count, fmt.Errorf("failed to record migration %s: %w", file.Filename, err)
		}

		fmt.Printf("✓ Migrated: %s\n", file.Filename)
		count++
	}

	return count, nil
}

// RollbackMigrations rolls back the last N batches of migrations
func RollbackMigrations(conn db.DB, migrationPath string, steps int, dsn string) (int, error) {
	// Get executed migrations in reverse order
	dialect := DetectDialect(dsn)
	executed, err := getExecutedMigrations(conn, dialect)
	if err != nil {
		return 0, fmt.Errorf("failed to get executed migrations: %w", err)
	}

	if len(executed) == 0 {
		return 0, nil
	}

	// Determine latest N batches to rollback
	// Collect distinct batches in descending order
	batchSet := make(map[int]bool)
	for _, m := range executed {
		batchSet[m.Batch] = true
	}
	var batches []int
	for b := range batchSet {
		batches = append(batches, b)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(batches)))
	batchesToRollback := make(map[int]bool)
	for i := 0; i < len(batches) && i < steps; i++ {
		batchesToRollback[batches[i]] = true
	}

	// Rollback migrations
	count := 0
	for i := len(executed) - 1; i >= 0; i-- {
		mig := executed[i]
		if !batchesToRollback[mig.Batch] {
			continue
		}

		// Read migration file
		filePath := filepath.Join(migrationPath, mig.Migration)
		content, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Printf("⚠ Warning: Failed to read migration file %s: %v\n", mig.Migration, err)
			continue
		}

		// Extract DOWN part
		downSQL := extractSection(string(content), "Down")
		if downSQL == "" {
			fmt.Printf("⚠ Warning: No DOWN section found in migration %s, skipping\n", mig.Migration)
			continue
		}

		// Execute rollback
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err = conn.Exec(ctx, downSQL)
		cancel()

		if err != nil {
			errorMsg := fmt.Sprintf("❌ ROLLBACK FAILED: %s\n", mig.Migration)
			errorMsg += fmt.Sprintf("   Reason: %v\n", err)
			errorMsg += "   Status: Transaction automatically rolled back\n"
			errorMsg += "   Action: Fix the DOWN section SQL and try rollback again\n"
			errorMsg += "   Important: No migration records were removed\n"

			output.Error(errorMsg)
			return count, fmt.Errorf("%s", errorMsg)
		}

		// Remove migration record
		err = removeMigrationRecord(conn, mig, dialect)
		if err != nil {
			return count, fmt.Errorf("failed to remove migration record %s: %w", mig.Migration, err)
		}

		fmt.Printf("✓ Rolled back: %s\n", mig.Migration)
		count++
	}

	return count, nil
}

// RollbackAllMigrations rolls back all migrations
func RollbackAllMigrations(conn db.DB, migrationPath string, dsn string) (int, error) {
	dialect := DetectDialect(dsn)
	executed, err := getExecutedMigrations(conn, dialect)
	if err != nil {
		return 0, err
	}

	// Sort by batch descending then migration name
	sort.Slice(executed, func(i, j int) bool {
		if executed[i].Batch != executed[j].Batch {
			return executed[i].Batch > executed[j].Batch
		}
		return executed[i].Migration > executed[j].Migration
	})

	count := 0
	for _, mig := range executed {
		filePath := filepath.Join(migrationPath, mig.Migration)
		content, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Printf("⚠ Warning: Failed to read migration file %s: %v\n", mig.Migration, err)
			continue
		}

		downSQL := extractSection(string(content), "Down")
		if downSQL == "" {
			fmt.Printf("⚠ Warning: No DOWN section found in migration %s, skipping\n", mig.Migration)
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err = conn.Exec(ctx, downSQL)
		cancel()

		if err != nil {
			return count, fmt.Errorf("failed to rollback migration %s: %w", mig.Migration, err)
		}

		err = removeMigrationRecord(conn, mig, dialect)
		if err != nil {
			return count, fmt.Errorf("failed to remove migration record: %w", err)
		}

		fmt.Printf("✓ Rolled back: %s\n", mig.Migration)
		count++
	}

	return count, nil
}

// Helper functions

func getMigrationFiles(basePath string) ([]MigrationFile, error) {
	files, err := os.ReadDir(basePath)
	if err != nil {
		// Directory might not exist yet
		if os.IsNotExist(err) {
			return []MigrationFile{}, nil
		}
		return nil, err
	}

	var migrations []MigrationFile
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if !strings.HasSuffix(file.Name(), ".sql") {
			continue
		}

		var timestamp int64
		_, err := fmt.Sscanf(file.Name(), "%d", &timestamp)
		if err != nil {
			continue
		}

		migrations = append(migrations, MigrationFile{
			Filename:  file.Name(),
			Timestamp: timestamp,
		})
	}

	return migrations, nil
}

func getExecutedMigrations(conn db.DB, dialect Dialect) ([]Migration, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := conn.Query(ctx, "SELECT id, migration, batch FROM migrations ORDER BY batch, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var migrations []Migration
	for rows.Next() {
		var mig Migration
		err := rows.Scan(&mig.ID, &mig.Migration, &mig.Batch)
		if err != nil {
			return nil, err
		}
		migrations = append(migrations, mig)
	}

	return migrations, rows.Err()
}

func recordMigration(conn db.DB, filename string, batch int, dialect Dialect) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := "INSERT INTO migrations (migration, batch) VALUES ($1, $2)"
	return conn.Exec(ctx, query, filename, batch)
}

func removeMigrationRecord(conn db.DB, mig Migration, dialect Dialect) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return conn.Exec(ctx, "DELETE FROM migrations WHERE id = $1", mig.ID)
}

func getNextBatchNumber(conn db.DB, dialect Dialect) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var batch int
	row := conn.QueryRow(ctx, "SELECT COALESCE(MAX(batch), 0) + 1 FROM migrations")
	err := row.Scan(&batch)
	return batch, err
}

func extractSection(content string, section string) string {
	lines := strings.Split(content, "\n")
	var inSection bool
	var sectionContent []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if we're entering the section
		if strings.Contains(trimmed, "⬆") && strings.Contains(trimmed, section) {
			inSection = true
			continue
		}

		// Check if we're exiting the section
		if strings.Contains(trimmed, "⬇") {
			inSection = false
			break
		}

		// If we're entering another section marker, stop
		if strings.HasPrefix(trimmed, "--") && (strings.Contains(trimmed, "⬆") || strings.Contains(trimmed, "⬇")) {
			if inSection {
				inSection = false
				break
			}
		}

		// Collect lines from the current section
		if inSection && !strings.HasPrefix(trimmed, "--") {
			sectionContent = append(sectionContent, line)
		}
	}

	return strings.TrimSpace(strings.Join(sectionContent, "\n"))
}
