package migrate

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestDiscover(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "200_second.sql", "-- Up\nSELECT 2;\n")
	writeFile(t, dir, "100_first.sql", "-- Up\nSELECT 1;\n")
	writeFile(t, dir, "300_b_same_stamp.sql", "-- Up\nSELECT 4;\n")
	writeFile(t, dir, "300_a_same_stamp.sql", "-- Up\nSELECT 3;\n")
	writeFile(t, dir, "extensions.sql", "CREATE EXTENSION IF NOT EXISTS citext;\n")
	writeFile(t, dir, "functions.sql", "-- functions\n")
	writeFile(t, dir, "enums.sql", "-- enums\n")
	writeFile(t, dir, "notes.md", "hello\n")
	writeFile(t, dir, "400_not_sql.txt", "SELECT 5;\n")
	writeFile(t, dir, ".vm_enums_hash", "deadbeef")
	if err := os.Mkdir(filepath.Join(dir, "500_subdir.sql"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	got, err := Discover(dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	want := []struct {
		name string
		ts   int64
	}{
		{"100_first.sql", 100},
		{"200_second.sql", 200},
		{"300_a_same_stamp.sql", 300},
		{"300_b_same_stamp.sql", 300},
	}
	if len(got) != len(want) {
		t.Fatalf("Discover returned %d migrations, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Name != w.name {
			t.Errorf("migration %d: name = %q, want %q", i, got[i].Name, w.name)
		}
		if got[i].Timestamp != w.ts {
			t.Errorf("%s: timestamp = %d, want %d", got[i].Name, got[i].Timestamp, w.ts)
		}
		if got[i].Path != filepath.Join(dir, w.name) {
			t.Errorf("%s: path = %q", got[i].Name, got[i].Path)
		}
		if len(got[i].Checksum) != 64 {
			t.Errorf("%s: checksum = %q, want 64 hex characters", got[i].Name, got[i].Checksum)
		}
	}
}

func TestDiscoverMissingDirectory(t *testing.T) {
	got, err := Discover(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("Discover on missing directory: %v", err)
	}
	if got != nil {
		t.Fatalf("Discover on missing directory returned %+v, want nil", got)
	}
}

func TestDiscoverEmptyDirUsesDefault(t *testing.T) {
	// DefaultDir does not exist inside the test's working directory, so this only
	// asserts that an empty Dir is resolved rather than read as "".
	if _, err := Discover(""); err != nil {
		t.Fatalf("Discover(\"\"): %v", err)
	}
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name string
		want int64
		ok   bool
	}{
		{"1712345678_create_users_table.sql", 1712345678, true},
		{"0001_init.sql", 1, true},
		{"42.sql", 42, true},
		{"extensions.sql", 0, false},
		{"_1_leading_underscore.sql", 0, false},
		{"v2_1712345678.sql", 0, false},
		{"99999999999999999999999_overflow.sql", 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseTimestamp(tc.name)
			if ok != tc.ok || got != tc.want {
				t.Errorf("parseTimestamp(%q) = (%d, %t), want (%d, %t)", tc.name, got, ok, tc.want, tc.ok)
			}
		})
	}
}
