package migration

import (
	"fmt"
	"sort"
	"strings"

	"github.com/vorzela/vorzela-migrate/internal/db"
	"github.com/vorzela/vorzela-migrate/internal/output"
)

// ShowStatus displays migration status
func ShowStatus(conn db.DB, migrationPath string, dsn string) error {
	// Get all migration files
	files, err := getMigrationFiles(migrationPath)
	if err != nil {
		return fmt.Errorf("failed to read migration files: %w", err)
	}

	// Get executed migrations
	executed, err := getExecutedMigrations(conn)
	if err != nil {
		return fmt.Errorf("failed to get executed migrations: %w", err)
	}

	executedMap := make(map[string]int)
	for _, mig := range executed {
		executedMap[mig.Migration] = mig.Batch
	}

	// Sort files by timestamp
	sort.Slice(files, func(i, j int) bool {
		return files[i].Timestamp < files[j].Timestamp
	})

	fmt.Printf("\n🐘 Migration Status\n")
	fmt.Println(strings.Repeat("─", 80))
	fmt.Printf("%-50s | %-20s\n", "Migration Name", "Status")
	fmt.Println(strings.Repeat("─", 80))

	if len(files) == 0 {
		fmt.Println("No migration files found")
		return nil
	}

	for _, file := range files {
		// Extract readable migration name from filename
		// Format: timestamp_migration_name.sql -> migration_name
		migrationName := extractMigrationName(file.Filename)

		if batch, ok := executedMap[file.Filename]; ok {
			output.TableRow(migrationName, output.ExecutedStatus(batch))
		} else {
			output.TableRow(migrationName, output.PendingStatus())
		}
	}

	fmt.Println(strings.Repeat("─", 80))

	// Show summary
	pending := len(files) - len(executed)
	fmt.Printf("\nSummary: %d executed, %d pending\n\n", len(executed), pending)

	return nil
}

// extractMigrationName extracts the readable migration name from filename
// Example: 1707129045_create_users_table.sql -> create_users_table
func extractMigrationName(filename string) string {
	// Remove .sql extension
	filename = strings.TrimSuffix(filename, ".sql")

	// Split by underscore to find where timestamp ends
	parts := strings.Split(filename, "_")
	if len(parts) < 2 {
		return filename
	}

	// First part is timestamp (digits only), rest is the migration name
	// Join from index 1 onwards to get the migration name
	migrationName := strings.Join(parts[1:], "_")

	// Make it more readable: replace underscores with spaces, capitalize first letter
	readable := strings.ReplaceAll(migrationName, "_", " ")

	return readable
}
