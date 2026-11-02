package migration

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vorzela/vorzela-migrate/v2/internal/db"
	"github.com/vorzela/vorzela-migrate/v2/internal/output"
)

// StatusEntry describes one migration file (or an executed migration whose file
// is missing from disk).
type StatusEntry struct {
	Filename    string `json:"filename"`
	Name        string `json:"name"`
	Timestamp   int64  `json:"timestamp,omitempty"`
	Executed    bool   `json:"executed"`
	Batch       int    `json:"batch,omitempty"`
	ChecksumOK  *bool  `json:"checksum_ok,omitempty"`
	FileMissing bool   `json:"file_missing,omitempty"`
}

// StatusReport is the machine-readable form of `vm status`.
type StatusReport struct {
	Path      string        `json:"path"`
	Dialect   string        `json:"dialect"`
	Executed  int           `json:"executed"`
	Pending   int           `json:"pending"`
	Missing   int           `json:"missing_files"`
	Entries   []StatusEntry `json:"entries"`
	Batches   int           `json:"batches"`
	LastBatch int           `json:"last_batch,omitempty"`
}

// GetStatus returns migration status without printing it.
func GetStatus(conn db.DB, migrationPath string, dsn string) (*StatusReport, error) {
	files, err := getMigrationFiles(migrationPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read migration files: %w", err)
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].Timestamp != files[j].Timestamp {
			return files[i].Timestamp < files[j].Timestamp
		}
		return files[i].Filename < files[j].Filename
	})

	executed, err := getExecutedMigrations(conn)
	if err != nil {
		return nil, fmt.Errorf("failed to get executed migrations: %w", err)
	}

	executedMap := make(map[string]Migration)
	for _, mig := range executed {
		executedMap[mig.Migration] = mig
	}

	report := &StatusReport{
		Path:    migrationPath,
		Dialect: string(DetectDialect(dsn)),
		Entries: make([]StatusEntry, 0, len(files)+len(executed)),
	}

	seen := make(map[string]bool)
	batchSet := make(map[int]bool)
	lastBatch := 0
	for _, mig := range executed {
		batchSet[mig.Batch] = true
		if mig.Batch > lastBatch {
			lastBatch = mig.Batch
		}
	}

	for _, file := range files {
		seen[file.Filename] = true
		entry := StatusEntry{
			Filename:  file.Filename,
			Name:      extractMigrationName(file.Filename),
			Timestamp: file.Timestamp,
		}
		if mig, ok := executedMap[file.Filename]; ok {
			entry.Executed = true
			entry.Batch = mig.Batch
			if mig.Checksum != "" {
				match, err := ChecksumMatchForFile(migrationPath, mig)
				if err == nil {
					entry.ChecksumOK = &match
				}
			}
		}
		report.Entries = append(report.Entries, entry)
	}

	for _, mig := range executed {
		if seen[mig.Migration] {
			continue
		}
		report.Entries = append(report.Entries, StatusEntry{
			Filename:    mig.Migration,
			Name:        extractMigrationName(mig.Migration),
			Executed:    true,
			Batch:       mig.Batch,
			FileMissing: true,
		})
	}

	sort.SliceStable(report.Entries, func(i, j int) bool {
		a, b := report.Entries[i], report.Entries[j]
		if a.Timestamp != b.Timestamp {
			return a.Timestamp < b.Timestamp
		}
		return a.Filename < b.Filename
	})

	for _, e := range report.Entries {
		if e.Executed {
			report.Executed++
		} else {
			report.Pending++
		}
		if e.FileMissing {
			report.Missing++
		}
	}
	report.Batches = len(batchSet)
	report.LastBatch = lastBatch
	return report, nil
}

// ChecksumMatchForFile is a small convenience wrapper used by status.
func ChecksumMatchForFile(dir string, mig Migration) (bool, error) {
	filePath := filepath.Join(dir, mig.Migration)
	if mig.Checksum == "" {
		return true, nil
	}
	return ChecksumMatch(filePath, mig.Checksum)
}

// ShowStatus displays migration status.
func ShowStatus(conn db.DB, migrationPath string, dsn string) error {
	report, err := GetStatus(conn, migrationPath, dsn)
	if err != nil {
		return err
	}

	fmt.Printf("\n🐘 Migration Status\n")
	fmt.Println(strings.Repeat("─", 80))
	fmt.Printf("%-50s | %-20s\n", "Migration Name", "Status")
	fmt.Println(strings.Repeat("─", 80))

	if len(report.Entries) == 0 {
		fmt.Println("No migration files found")
		return nil
	}

	for _, entry := range report.Entries {
		status := output.PendingStatus()
		if entry.Executed {
			status = output.ExecutedStatus(entry.Batch)
		}
		if entry.FileMissing {
			status = output.WarningStatus("file missing")
		} else if entry.ChecksumOK != nil && !*entry.ChecksumOK {
			status += " (checksum changed)"
		}
		output.TableRow(entry.Name, status)
	}

	fmt.Println(strings.Repeat("─", 80))
	fmt.Printf("\nSummary: %d executed, %d pending", report.Executed, report.Pending)
	if report.Missing > 0 {
		fmt.Printf(", %d executed file(s) missing on disk", report.Missing)
	}
	if report.Batches > 0 {
		fmt.Printf(", %d batch(es), last batch %d", report.Batches, report.LastBatch)
	}
	fmt.Print("\n\n")
	return nil
}

// extractMigrationName extracts the readable migration name from filename.
// Example: 1707129045_create_users_table.sql -> create users table
func extractMigrationName(filename string) string {
	filename = strings.TrimSuffix(filename, ".sql")

	parts := strings.Split(filename, "_")
	if len(parts) < 2 {
		return filename
	}

	migrationName := strings.Join(parts[1:], "_")
	return strings.ReplaceAll(migrationName, "_", " ")
}
