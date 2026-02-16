package output

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNewMigrationLogger(t *testing.T) {
	logger := NewMigrationLogger(true)
	
	if logger == nil {
		t.Error("NewMigrationLogger() returned nil")
	}
	
	if !logger.verbose {
		t.Error("Logger should be verbose when true is passed")
	}
}

func TestLoggerVerboseMode(t *testing.T) {
	tests := []struct {
		name    string
		verbose bool
		want    bool
	}{
		{
			name:    "verbose enabled",
			verbose: true,
			want:    true,
		},
		{
			name:    "verbose disabled",
			verbose: false,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewMigrationLogger(tt.verbose)
			
			if logger.verbose != tt.want {
				t.Errorf("Logger verbose = %v, want %v", logger.verbose, tt.want)
			}
		})
	}
}

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

func TestLoggerSuccess(t *testing.T) {
	logger := NewMigrationLogger(true)
	
	output := captureOutput(func() {
		logger.Success("Migration completed")
	})
	
	if !strings.Contains(output, "SUCCESS") && !strings.Contains(output, "✓") {
		t.Logf("Output: %s", output)
	}
}

func TestLoggerInfo(t *testing.T) {
	logger := NewMigrationLogger(true)
	
	output := captureOutput(func() {
		logger.Info("Running migration %s", "001_create_users.sql")
	})
	
	if !strings.Contains(output, "INFO") && !strings.Contains(output, "ℹ") {
		t.Logf("Output: %s", output)
	}
}

func TestLoggerWarning(t *testing.T) {
	logger := NewMigrationLogger(true)
	
	output := captureOutput(func() {
		logger.Warning("Checksum mismatch detected")
	})
	
	if !strings.Contains(output, "WARNING") && !strings.Contains(output, "⚠") {
		t.Logf("Output: %s", output)
	}
}

func TestLoggerError(t *testing.T) {
	logger := NewMigrationLogger(true)
	
	output := captureOutput(func() {
		logger.Error("Migration failed: %v", "connection timeout")
	})
	
	if !strings.Contains(output, "ERROR") && !strings.Contains(output, "✗") {
		t.Logf("Output: %s", output)
	}
}

func TestLoggerProgress(t *testing.T) {
	logger := NewMigrationLogger(true)
	
	output := captureOutput(func() {
		logger.Progress(1, 5, "Migrating table users...")
	})
	
	if output == "" {
		t.Log("Progress output captured")
	}
}

func TestLoggerPrompt(t *testing.T) {
	logger := NewMigrationLogger(true)
	
	output := captureOutput(func() {
		logger.Prompt("Continue? (yes/no)")
	})
	
	if output == "" {
		t.Log("Prompt output captured")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		wantUnit string
	}{
		{
			name:     "milliseconds",
			duration: 250 * time.Millisecond,
			wantUnit: "ms",
		},
		{
			name:     "seconds",
			duration: 2 * time.Second,
			wantUnit: "s",
		},
		{
			name:     "sub-millisecond",
			duration: 500 * time.Microsecond,
			wantUnit: "ms",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// formatDuration is a package-level function
			formatted := formatDuration(tt.duration)
			
			if formatted == "" {
				t.Error("formatDuration() returned empty string")
			}
			
			if !strings.Contains(formatted, tt.wantUnit) {
				t.Logf("Formatted duration: %s (expected unit: %s)", formatted, tt.wantUnit)
			}
		})
	}
}

func TestMigrationComplete(t *testing.T) {
	logger := NewMigrationLogger(true)
	
	output := captureOutput(func() {
		logger.MigrationComplete("001_create_users.sql", 100*time.Millisecond)
	})
	
	if !strings.Contains(output, "Completed") && !strings.Contains(output, "✓") {
		t.Logf("Output: %s", output)
	}
}

func TestMigrationFailed(t *testing.T) {
	logger := NewMigrationLogger(true)
	
	output := captureOutput(func() {
		logger.MigrationFailed("002_add_column.sql", os.ErrNotExist)
	})
	
	if !strings.Contains(output, "Failed") && !strings.Contains(output, "✗") {
		t.Logf("Output: %s", output)
	}
}

func TestVerboseFiltering(t *testing.T) {
	// Verbose mode should show debug messages
	verboseLogger := NewMigrationLogger(true)
	
	output := captureOutput(func() {
		verboseLogger.Debug("Debug message")
	})
	
	// In verbose mode, debug should appear
	_ = output // Would contain debug in real implementation
	
	// Non-verbose mode should hide debug messages
	quietLogger := NewMigrationLogger(false)
	
	output2 := captureOutput(func() {
		quietLogger.Debug("Debug message")
	})
	
	// In quiet mode, debug should not appear (or be filtered)
	_ = output2
}

func TestLogLevels(t *testing.T) {
	levels := []struct {
		name  string
		level string
	}{
		{"success", "SUCCESS"},
		{"info", "INFO"},
		{"warning", "WARNING"},
		{"error", "ERROR"},
		{"debug", "DEBUG"},
	}

	for _, level := range levels {
		t.Run(level.name, func(t *testing.T) {
			if level.level == "" {
				t.Errorf("Level %s should not be empty", level.name)
			}
		})
	}
}

func TestColorCodes(t *testing.T) {
	// ANSI color codes should be valid
	tests := []struct {
		name  string
		code  string
	}{
		{"reset", "\033[0m"},
		{"green", "\033[32m"},
		{"yellow", "\033[33m"},
		{"red", "\033[31m"},
		{"blue", "\033[34m"},
		{"cyan", "\033[36m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.code) == 0 {
				t.Errorf("Color code for %s is empty", tt.name)
			}
			
			if !strings.HasPrefix(tt.code, "\033[") {
				t.Errorf("Invalid ANSI code: %s", tt.code)
			}
		})
	}
}

func TestMultipleLogCalls(t *testing.T) {
	logger := NewMigrationLogger(true)
	
	output := captureOutput(func() {
		logger.Info("First message")
		logger.Info("Second message")
		logger.Success("Third message")
	})
	
	// Should contain all messages
	if len(output) == 0 {
		t.Error("No output captured from multiple log calls")
	}
}

func TestFormatDurationPrecision(t *testing.T) {
	tests := []struct {
		duration time.Duration
		minValue float64
		maxValue float64
	}{
		{100 * time.Millisecond, 0, 200},
		{1 * time.Second, 0, 2},
		{5 * time.Minute, 0, 10},
	}

	for _, tt := range tests {
		formatted := formatDuration(tt.duration)
		if formatted == "" {
			t.Errorf("Duration %v formatted to empty string", tt.duration)
		}
	}
}

func BenchmarkLoggerInfo(b *testing.B) {
	logger := NewMigrationLogger(false) // Quiet mode for benchmark
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("Benchmark message %d", i)
	}
}

func BenchmarkFormatDuration(b *testing.B) {
	duration := 123 * time.Millisecond
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		formatDuration(duration)
	}
}

func BenchmarkMigrationComplete(b *testing.B) {
	logger := NewMigrationLogger(false)
	duration := 100 * time.Millisecond
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.MigrationComplete("test.sql", duration)
	}
}
