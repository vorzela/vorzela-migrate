package vmtool

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ExitError wraps a non-zero vm exit with stderr hints (locks, drift, checksum).
type ExitError struct {
	Args     []string
	ExitCode int
	Err      error
	Hint     string
}

func (e *ExitError) Error() string {
	msg := fmt.Sprintf("vm %s: exit %d", strings.Join(e.Args, " "), e.ExitCode)
	if e.Hint != "" {
		msg += " — " + e.Hint
	}
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

func (e *ExitError) Unwrap() error { return e.Err }

// Run executes `vm <args...>` with stdio attached. Ensures vm is available first.
func Run(installIfMissing bool, args ...string) error {
	vmPath, err := Ensure(installIfMissing)
	if err != nil {
		return err
	}
	return RunPath(vmPath, args...)
}

// RunPath executes an already-resolved vm binary.
func RunPath(vmPath string, args ...string) error {
	cmd := exec.Command(vmPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	err := cmd.Run()
	if err == nil {
		return nil
	}
	var ee *exec.ExitError
	code := 1
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	}
	return &ExitError{
		Args:     args,
		ExitCode: code,
		Err:      err,
		Hint:     hintFor(args, err),
	}
}

func hintFor(args []string, err error) string {
	joined := strings.ToLower(strings.Join(args, " ") + " " + err.Error())
	switch {
	case strings.Contains(joined, "lock") || strings.Contains(joined, "advisory"):
		return "another migrate may be running (race) — wait or check DB locks; vm uses advisory/named locks"
	case strings.Contains(joined, "checksum"):
		return "migration file changed after apply — edit carefully or use --force / fix drift (see docs/MIGRATIONS.md)"
	case strings.Contains(joined, "drift"):
		return "schema drift detected — prefer editing the create migration + re-migrate in thin histories, or generate an alter"
	case strings.Contains(joined, "connect") || strings.Contains(joined, "database_url"):
		return "check DATABASE_URL / .vm and that the database is reachable"
	default:
		if len(args) > 0 && args[0] == "migrate" {
			return "try: vorm migrate --enhanced --detect-drift  or  vorm status"
		}
		return "see vorm docs/MIGRATIONS.md and `vm " + args[0] + " --help`"
	}
}

// Migrate runs pending migrations (pass-through flags: --force, --enhanced, …).
func Migrate(vmPath string, extra ...string) error {
	return RunPath(vmPath, append([]string{"migrate"}, extra...)...)
}

// Rollback rolls back migration batches.
func Rollback(vmPath string, extra ...string) error {
	return RunPath(vmPath, append([]string{"rollback"}, extra...)...)
}

// Status shows migration status.
func Status(vmPath string, extra ...string) error {
	return RunPath(vmPath, append([]string{"status"}, extra...)...)
}

// Fresh rolls back all and re-runs (destructive).
func Fresh(vmPath string, extra ...string) error {
	return RunPath(vmPath, append([]string{"fresh"}, extra...)...)
}

// Refresh re-runs all migrations via down then up.
func Refresh(vmPath string, extra ...string) error {
	return RunPath(vmPath, append([]string{"refresh"}, extra...)...)
}

// MakeMigration creates a migration file via vm make migration.
func MakeMigration(vmPath string, name string, flags ...string) error {
	args := append([]string{"make", "migration", name}, flags...)
	return RunPath(vmPath, args...)
}
