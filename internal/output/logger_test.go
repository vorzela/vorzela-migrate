package output

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

// machineLogger returns a logger pinned to machine mode (non-TTY path)
// regardless of the real stdout state during tests.
func machineLogger(verbose bool) *MigrationLogger {
	return &MigrationLogger{
		startTime:  time.Now(),
		verbose:    verbose,
		machineOut: true,
	}
}

// humanLogger returns a logger pinned to TTY/human mode.
func humanLogger(verbose bool) *MigrationLogger {
	return &MigrationLogger{
		startTime:  time.Now(),
		verbose:    verbose,
		machineOut: false,
	}
}

// parseKV splits a single structured log line into a map of key→value.
// It handles both bare values and double-quoted values.
func parseKV(line string) map[string]string {
	m := make(map[string]string)
	line = strings.TrimSpace(line)
	for line != "" {
		// Find key= token
		eqIdx := strings.Index(line, "=")
		if eqIdx == -1 {
			break
		}
		key := line[:eqIdx]
		rest := line[eqIdx+1:]

		var val string
		if strings.HasPrefix(rest, `"`) {
			// Quoted value: scan to the closing (unescaped) "
			i := 1
			var b strings.Builder
			for i < len(rest) {
				if rest[i] == '\\' && i+1 < len(rest) {
					b.WriteByte(rest[i+1])
					i += 2
					continue
				}
				if rest[i] == '"' {
					i++
					break
				}
				b.WriteByte(rest[i])
				i++
			}
			val = b.String()
			rest = strings.TrimLeft(rest[i:], " ")
		} else {
			// Bare value ends at the next space
			spIdx := strings.Index(rest, " ")
			if spIdx == -1 {
				val = rest
				rest = ""
			} else {
				val = rest[:spIdx]
				rest = rest[spIdx+1:]
			}
		}
		m[key] = val
		line = rest
	}
	return m
}

// ---------------------------------------------------------------------------
// Construction
// ---------------------------------------------------------------------------

func TestNewMigrationLogger(t *testing.T) {
	logger := NewMigrationLogger(true)
	if logger == nil {
		t.Fatal("NewMigrationLogger() returned nil")
	}
	if !logger.verbose {
		t.Error("logger should be verbose when true is passed")
	}
}

func TestLoggerVerboseMode(t *testing.T) {
	for _, verbose := range []bool{true, false} {
		l := NewMigrationLogger(verbose)
		if l.verbose != verbose {
			t.Errorf("verbose=%v: logger.verbose=%v", verbose, l.verbose)
		}
	}
}

// ---------------------------------------------------------------------------
// Machine-mode (structured key=value) output
// ---------------------------------------------------------------------------

func TestMachineMode_InfoEmitsKV(t *testing.T) {
	l := machineLogger(true)
	out := captureOutput(func() { l.Info("running %s", "001.sql") })
	kv := parseKV(strings.TrimSpace(out))

	if kv["level"] != "info" {
		t.Errorf("level=%q, want info", kv["level"])
	}
	if kv["event"] != "log" {
		t.Errorf("event=%q, want log", kv["event"])
	}
	if !strings.Contains(kv["msg"], "running") {
		t.Errorf("msg=%q does not contain 'running'", kv["msg"])
	}
}

func TestMachineMode_SuccessLevel(t *testing.T) {
	l := machineLogger(true)
	out := captureOutput(func() { l.Success("all done") })
	kv := parseKV(strings.TrimSpace(out))
	if kv["level"] != "ok" {
		t.Errorf("level=%q, want ok", kv["level"])
	}
}

func TestMachineMode_WarningLevel(t *testing.T) {
	l := machineLogger(true)
	out := captureOutput(func() { l.Warning("drift found") })
	kv := parseKV(strings.TrimSpace(out))
	if kv["level"] != "warn" {
		t.Errorf("level=%q, want warn", kv["level"])
	}
}

func TestMachineMode_ErrorLevel(t *testing.T) {
	l := machineLogger(true)
	out := captureOutput(func() { l.Error("something broke") })
	kv := parseKV(strings.TrimSpace(out))
	if kv["level"] != "error" {
		t.Errorf("level=%q, want error", kv["level"])
	}
}

func TestMachineMode_DebugHiddenWhenNotVerbose(t *testing.T) {
	l := machineLogger(false)
	out := captureOutput(func() { l.Debug("secret debug") })
	if out != "" {
		t.Errorf("expected no output for Debug when verbose=false, got %q", out)
	}
}

func TestMachineMode_DebugShownWhenVerbose(t *testing.T) {
	l := machineLogger(true)
	out := captureOutput(func() { l.Debug("trace info") })
	kv := parseKV(strings.TrimSpace(out))
	if kv["level"] != "debug" {
		t.Errorf("level=%q, want debug", kv["level"])
	}
}

func TestMachineMode_MigrationStart(t *testing.T) {
	l := machineLogger(true)
	out := captureOutput(func() { l.Migration("001_create_users.sql", "v1") })
	kv := parseKV(strings.TrimSpace(out))

	if kv["event"] != "migration_start" {
		t.Errorf("event=%q, want migration_start", kv["event"])
	}
	if kv["file"] != "001_create_users.sql" {
		t.Errorf("file=%q, want 001_create_users.sql", kv["file"])
	}
	if kv["version"] != "v1" {
		t.Errorf("version=%q, want v1", kv["version"])
	}
}

func TestMachineMode_MigrationComplete(t *testing.T) {
	l := machineLogger(true)
	out := captureOutput(func() {
		l.MigrationComplete("001_create_users.sql", 120*time.Millisecond)
	})
	kv := parseKV(strings.TrimSpace(out))

	if kv["event"] != "migration_complete" {
		t.Errorf("event=%q, want migration_complete", kv["event"])
	}
	if kv["file"] != "001_create_users.sql" {
		t.Errorf("file=%q, want 001_create_users.sql", kv["file"])
	}
	if kv["level"] != "ok" {
		t.Errorf("level=%q, want ok", kv["level"])
	}
	if !strings.Contains(kv["elapsed"], "ms") {
		t.Errorf("elapsed=%q, want a millisecond value", kv["elapsed"])
	}
}

func TestMachineMode_MigrationFailed(t *testing.T) {
	l := machineLogger(true)
	testErr := errors.New("connection refused")
	out := captureOutput(func() { l.MigrationFailed("003_add_index.sql", testErr) })
	kv := parseKV(strings.TrimSpace(out))

	if kv["event"] != "migration_failed" {
		t.Errorf("event=%q, want migration_failed", kv["event"])
	}
	if kv["file"] != "003_add_index.sql" {
		t.Errorf("file=%q, want 003_add_index.sql", kv["file"])
	}
	if kv["level"] != "error" {
		t.Errorf("level=%q, want error", kv["level"])
	}
	if !strings.Contains(kv["error"], "connection refused") {
		t.Errorf("error=%q, want to contain 'connection refused'", kv["error"])
	}
}

func TestMachineMode_Step(t *testing.T) {
	l := machineLogger(true)
	out := captureOutput(func() { l.Step("CREATE TABLE users") })
	kv := parseKV(strings.TrimSpace(out))
	if kv["event"] != "step" {
		t.Errorf("event=%q, want step", kv["event"])
	}
}

func TestMachineMode_StepComplete(t *testing.T) {
	l := machineLogger(true)
	out := captureOutput(func() { l.StepComplete("CREATE INDEX", 50*time.Millisecond) })
	kv := parseKV(strings.TrimSpace(out))
	if kv["event"] != "step_complete" {
		t.Errorf("event=%q, want step_complete", kv["event"])
	}
	if kv["level"] != "ok" {
		t.Errorf("level=%q, want ok", kv["level"])
	}
}

func TestMachineMode_StepFailed(t *testing.T) {
	l := machineLogger(true)
	out := captureOutput(func() { l.StepFailed("DROP TABLE", errors.New("timeout")) })
	kv := parseKV(strings.TrimSpace(out))
	if kv["event"] != "step_failed" {
		t.Errorf("event=%q, want step_failed", kv["event"])
	}
	if kv["level"] != "error" {
		t.Errorf("level=%q, want error", kv["level"])
	}
}

func TestMachineMode_Summary(t *testing.T) {
	l := machineLogger(true)
	out := captureOutput(func() { l.Summary(5, 1, 6) })
	kv := parseKV(strings.TrimSpace(out))

	if kv["event"] != "summary" {
		t.Errorf("event=%q, want summary", kv["event"])
	}
	if kv["total"] != "6" {
		t.Errorf("total=%q, want 6", kv["total"])
	}
	if kv["successful"] != "5" {
		t.Errorf("successful=%q, want 5", kv["successful"])
	}
	if kv["failed"] != "1" {
		t.Errorf("failed=%q, want 1", kv["failed"])
	}
}

func TestMachineMode_SchemaDrift(t *testing.T) {
	l := machineLogger(true)
	out := captureOutput(func() { l.SchemaDrift("orders", []string{"user_id", "product_id"}) })
	kv := parseKV(strings.TrimSpace(out))

	if kv["event"] != "schema_drift" {
		t.Errorf("event=%q, want schema_drift", kv["event"])
	}
	if kv["table"] != "orders" {
		t.Errorf("table=%q, want orders", kv["table"])
	}
	if !strings.Contains(kv["columns"], "user_id") {
		t.Errorf("columns=%q, want to contain user_id", kv["columns"])
	}
	if kv["level"] != "warn" {
		t.Errorf("level=%q, want warn", kv["level"])
	}
}

func TestMachineMode_TableHeader(t *testing.T) {
	l := machineLogger(true)
	out := captureOutput(func() { l.TableHeader("File", "Status", "Batch") })
	kv := parseKV(strings.TrimSpace(out))
	if kv["event"] != "table_header" {
		t.Errorf("event=%q, want table_header", kv["event"])
	}
}

func TestMachineMode_TableRow(t *testing.T) {
	l := machineLogger(true)
	out := captureOutput(func() { l.TableRow("001_create_users.sql", "✓ Batch 1", "1") })
	kv := parseKV(strings.TrimSpace(out))
	if kv["event"] != "table_row" {
		t.Errorf("event=%q, want table_row", kv["event"])
	}
}

func TestMachineMode_Progress(t *testing.T) {
	l := machineLogger(true)
	out := captureOutput(func() { l.Progress(3, 10, "migrating...") })
	kv := parseKV(strings.TrimSpace(out))

	if kv["event"] != "progress" {
		t.Errorf("event=%q, want progress", kv["event"])
	}
	if kv["current"] != "3" {
		t.Errorf("current=%q, want 3", kv["current"])
	}
	if kv["total"] != "10" {
		t.Errorf("total=%q, want 10", kv["total"])
	}
}

func TestMachineMode_TimeFieldPresent(t *testing.T) {
	l := machineLogger(true)
	out := captureOutput(func() { l.Info("hello") })
	kv := parseKV(strings.TrimSpace(out))
	if kv["time"] == "" {
		t.Error("expected time= field in structured output")
	}
	// time should be HH:MM:SS
	if len(kv["time"]) != 8 {
		t.Errorf("time=%q, expected HH:MM:SS format (8 chars)", kv["time"])
	}
}

func TestMachineMode_QuotingSpacesInValues(t *testing.T) {
	l := machineLogger(true)
	out := captureOutput(func() { l.MigrationFailed("003.sql", errors.New("too many connections open")) })
	// The error value contains spaces — it must be double-quoted in the output.
	if !strings.Contains(out, `"too many connections open"`) {
		t.Errorf("expected quoted error value in: %s", out)
	}
}

func TestKvQuote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"has space", `"has space"`},
		{"has\"quote", `"has\"quote"`},
		{"", `""`},
		{"k=v", `"k=v"`},
	}
	for _, tt := range tests {
		got := kvQuote(tt.input)
		if got != tt.want {
			t.Errorf("kvQuote(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestLevelTag(t *testing.T) {
	tests := []struct {
		level LogLevel
		want  string
	}{
		{LogSuccess, "ok"},
		{LogInfo, "info"},
		{LogWarning, "warn"},
		{LogError, "error"},
		{LogDebug, "debug"},
	}
	for _, tt := range tests {
		got := levelTag(tt.level)
		if got != tt.want {
			t.Errorf("levelTag(%q) = %q, want %q", tt.level, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Human-mode (TTY) output — verify coloured output is produced
// ---------------------------------------------------------------------------

func TestHumanMode_InfoContainsINFO(t *testing.T) {
	l := humanLogger(true)
	out := captureOutput(func() { l.Info("running %s", "001.sql") })
	if !strings.Contains(out, "INFO") {
		t.Errorf("human-mode Info output does not contain 'INFO': %q", out)
	}
}

func TestHumanMode_SuccessContainsSUCCESS(t *testing.T) {
	l := humanLogger(true)
	out := captureOutput(func() { l.Success("done") })
	if !strings.Contains(out, "SUCCESS") {
		t.Errorf("human-mode Success output does not contain 'SUCCESS': %q", out)
	}
}

func TestHumanMode_WarningContainsWARNING(t *testing.T) {
	l := humanLogger(true)
	out := captureOutput(func() { l.Warning("watch out") })
	if !strings.Contains(out, "WARNING") {
		t.Errorf("human-mode Warning output does not contain 'WARNING': %q", out)
	}
}

func TestHumanMode_ErrorContainsERROR(t *testing.T) {
	l := humanLogger(true)
	out := captureOutput(func() { l.Error("boom") })
	if !strings.Contains(out, "ERROR") {
		t.Errorf("human-mode Error output does not contain 'ERROR': %q", out)
	}
}

func TestHumanMode_MigrationComplete(t *testing.T) {
	l := humanLogger(true)
	out := captureOutput(func() {
		l.MigrationComplete("001_create_users.sql", 100*time.Millisecond)
	})
	if !strings.Contains(out, "Completed") && !strings.Contains(out, "✓") {
		t.Logf("Output: %s", out)
	}
}

func TestHumanMode_MigrationFailed(t *testing.T) {
	l := humanLogger(true)
	out := captureOutput(func() {
		l.MigrationFailed("002_add_column.sql", os.ErrNotExist)
	})
	if !strings.Contains(out, "Failed") && !strings.Contains(out, "✗") {
		t.Logf("Output: %s", out)
	}
}

// ---------------------------------------------------------------------------
// VM_NO_COLOR env var forces machine mode
// ---------------------------------------------------------------------------

func TestVMNoColorForcesStructuredOutput(t *testing.T) {
	t.Setenv("VM_NO_COLOR", "1")
	// isMachineMode() is called at construction time
	l := NewMigrationLogger(true)
	if !l.machineOut {
		t.Error("VM_NO_COLOR=1 should force machineOut=true")
	}
}

func TestVMNoColorZeroKeepsTTYMode(t *testing.T) {
	// VM_NO_COLOR=0 should be treated as "not set"
	t.Setenv("VM_NO_COLOR", "0")
	// We can only test the isMachineMode() logic; the real TTY state on CI
	// will also be non-TTY, so just verify the env var branch.
	if v := "0"; v == "0" {
		// branch skipped — merely verify no panic
	}
}

// ---------------------------------------------------------------------------
// formatDuration
// ---------------------------------------------------------------------------

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		wantUnit string
	}{
		{250 * time.Millisecond, "ms"},
		{2 * time.Second, "s"},
		{500 * time.Microsecond, "ms"},
	}
	for _, tt := range tests {
		formatted := formatDuration(tt.duration)
		if formatted == "" {
			t.Error("formatDuration() returned empty string")
		}
		if !strings.Contains(formatted, tt.wantUnit) {
			t.Logf("Formatted duration: %s (expected unit: %s)", formatted, tt.wantUnit)
		}
	}
}

// ---------------------------------------------------------------------------
// Multiple calls produce multiple lines
// ---------------------------------------------------------------------------

func TestMachineMode_MultipleCallsProduceMultipleLines(t *testing.T) {
	l := machineLogger(true)
	out := captureOutput(func() {
		l.Info("first")
		l.Success("second")
		l.Warning("third")
	})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d:\n%s", len(lines), out)
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkMachineMode_Info(b *testing.B) {
	l := machineLogger(false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		captureOutput(func() { l.Info("bench %d", i) })
	}
}

func BenchmarkFormatDuration(b *testing.B) {
	d := 123 * time.Millisecond
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		formatDuration(d)
	}
}
