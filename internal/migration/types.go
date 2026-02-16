package migration

import "time"

// Migration represents a database migration
type Migration struct {
	ID              int
	Migration       string
	Batch           int
	Checksum        string
	ExecutedAt      time.Time
	ExecutionTimeMs int64
}

// MigrationFile represents a migration file on disk
type MigrationFile struct {
	Filename  string
	Name      string
	Timestamp int64
}
