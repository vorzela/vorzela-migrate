package migrate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Migration is a migration file on disk.
type Migration struct {
	// Name is the bare file name, e.g. 1712345678_create_users_table.sql. It is
	// also the value stored in the migration column of the tracking table.
	Name string
	// Path is Name joined with the migrations directory.
	Path string
	// Timestamp is the numeric prefix of Name.
	Timestamp int64
	// Checksum is the SHA-256 of the whole file, lowercase hex.
	Checksum string
}

// Discover returns the migration files in dir ordered by timestamp, then by
// name so that files sharing a timestamp keep a stable order.
//
// Only files with a numeric prefix and a .sql suffix count as migrations, which
// is what keeps the extensions.sql, functions.sql and enums.sql helpers — and
// any other hand-written SQL — out of the migration sequence. Subdirectories are
// ignored, and a missing directory yields no migrations rather than an error.
func Discover(dir string) ([]Migration, error) {
	if dir == "" {
		dir = DefaultDir
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("vorm/migrate: read directory %s: %w", dir, err)
	}

	out := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		ts, ok := parseTimestamp(name)
		if !ok {
			continue
		}
		path := filepath.Join(dir, name)
		sum, err := Checksum(path)
		if err != nil {
			return nil, err
		}
		out = append(out, Migration{Name: name, Path: path, Timestamp: ts, Checksum: sum})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Timestamp != out[j].Timestamp {
			return out[i].Timestamp < out[j].Timestamp
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// parseTimestamp reads the leading run of digits from a migration file name.
func parseTimestamp(name string) (int64, bool) {
	end := 0
	for end < len(name) && name[end] >= '0' && name[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	ts, err := strconv.ParseInt(name[:end], 10, 64)
	if err != nil {
		return 0, false
	}
	return ts, true
}
