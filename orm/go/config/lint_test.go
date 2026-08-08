package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vorzela/vorm/config"
)

func TestLintFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".vorm")
	body := `# ok
PACKAGE=vormgen
DRIVER=bad
DIALECT=postgres
UNKNOWNS=1
PACKAGE=dup
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, err := config.LintFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !config.HasErrors(fs) {
		t.Fatalf("expected errors: %+v", fs)
	}
	var sawDriver, sawUnknown, sawDup bool
	for _, f := range fs {
		if f.Key == "DRIVER" {
			sawDriver = true
		}
		if f.Key == "UNKNOWNS" {
			sawUnknown = true
		}
		if f.Key == "PACKAGE" && f.Level == "warning" {
			sawDup = true
		}
	}
	if !sawDriver || !sawUnknown || !sawDup {
		t.Fatalf("missing findings: driver=%v unknown=%v dup=%v all=%+v", sawDriver, sawUnknown, sawDup, fs)
	}
}
