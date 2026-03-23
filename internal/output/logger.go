package output

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// ANSI color codes
const (
	ColorReset   = "\033[0m"
	ColorRed     = "\033[31m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorMagenta = "\033[35m"
	ColorCyan    = "\033[36m"
	ColorWhite   = "\033[37m"
	ColorGray    = "\033[90m"

	ColorBold      = "\033[1m"
	ColorDim       = "\033[2m"
	ColorUnderline = "\033[4m"
)

// LogLevel represents the severity of a log message
type LogLevel string

const (
	LogInfo    LogLevel = "INFO"
	LogSuccess LogLevel = "SUCCESS"
	LogWarning LogLevel = "WARNING"
	LogError   LogLevel = "ERROR"
	LogDebug   LogLevel = "DEBUG"
)

// MigrationLogger provides colored, formatted logging for migrations.
// When stdout is not a TTY (piped to another process or an AI agent) — or when
// VM_NO_COLOR=1 is set — every line is emitted as a machine-parseable key=value
// record instead of the human-friendly coloured output.
//
// Structured format (non-TTY):
//
//	level=ok   time=14:05:02 event=migration_complete file=001_create_users.sql elapsed=12ms
//	level=warn time=14:05:03 event=schema_drift       table=orders columns="user_id,product_id"
//	level=error time=14:05:04 event=migration_failed  file=003_add_index.sql error="..."
//
// Multi-word values are double-quoted with internal quotes escaped.
// Set VM_NO_COLOR=1 to force machine mode even on a TTY.
type MigrationLogger struct {
	startTime  time.Time
	verbose    bool
	machineOut bool // true → emit key=value lines
}

// NewMigrationLogger creates a new migration logger.
// Machine mode is auto-detected from the stdout TTY state but can be forced
// with the VM_NO_COLOR=1 environment variable.
func NewMigrationLogger(verbose bool) *MigrationLogger {
	return &MigrationLogger{
		startTime:  time.Now(),
		verbose:    verbose,
		machineOut: isMachineMode(),
	}
}

// isMachineMode returns true when output should be key=value structured lines.
// This is the case when stdout is not a character device (i.e. it is piped /
// redirected) or when VM_NO_COLOR is set to a non-empty, non-zero value.
func isMachineMode() bool {
	if v := os.Getenv("VM_NO_COLOR"); v != "" && v != "0" {
		return true
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	// ModeCharDevice is set for real terminals; absent when piped/redirected.
	return fi.Mode()&os.ModeCharDevice == 0
}

// kvQuote wraps v in double quotes when it contains spaces, equals signs,
// double quotes, or newlines, escaping any internal double quotes.
func kvQuote(v string) string {
	if v == "" {
		return `""`
	}
	if strings.ContainsAny(v, " \t\n\r=\"") {
		escaped := strings.ReplaceAll(v, `"`, `\"`)
		return `"` + escaped + `"`
	}
	return v
}

// levelTag maps a LogLevel to the short machine-mode level token.
func levelTag(l LogLevel) string {
	switch l {
	case LogSuccess:
		return "ok"
	case LogWarning:
		return "warn"
	case LogError:
		return "error"
	case LogDebug:
		return "debug"
	default:
		return "info"
	}
}

// kvLine emits a single structured log line.
// event and any number of key=value pairs are appended after level+time.
func kvLine(level LogLevel, event string, kv ...string) {
	ts := time.Now().Format("15:04:05")
	var b strings.Builder
	b.WriteString("level=")
	b.WriteString(levelTag(level))
	b.WriteString(" time=")
	b.WriteString(ts)
	b.WriteString(" event=")
	b.WriteString(event)
	for i := 0; i+1 < len(kv); i += 2 {
		b.WriteByte(' ')
		b.WriteString(kv[i])
		b.WriteByte('=')
		b.WriteString(kvQuote(kv[i+1]))
	}
	fmt.Println(b.String())
}

// Info logs an informational message
func (l *MigrationLogger) Info(format string, args ...interface{}) {
	l.log(LogInfo, ColorBlue, format, args...)
}

// Success logs a success message
func (l *MigrationLogger) Success(format string, args ...interface{}) {
	l.log(LogSuccess, ColorGreen, format, args...)
}

// Warning logs a warning message
func (l *MigrationLogger) Warning(format string, args ...interface{}) {
	l.log(LogWarning, ColorYellow, format, args...)
}

// Error logs an error message
func (l *MigrationLogger) Error(format string, args ...interface{}) {
	l.log(LogError, ColorRed, format, args...)
}

// Debug logs a debug message (only if verbose mode is enabled)
func (l *MigrationLogger) Debug(format string, args ...interface{}) {
	if l.verbose {
		l.log(LogDebug, ColorGray, format, args...)
	}
}

// log formats and prints a log message.
// In machine mode it emits a key=value line; otherwise it uses ANSI colour.
func (l *MigrationLogger) log(level LogLevel, color string, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	if l.machineOut {
		kvLine(level, "log", "msg", message)
		return
	}
	timestamp := time.Now().Format("15:04:05")
	levelStr := fmt.Sprintf("%s%-7s%s", color, level, ColorReset)
	timeStr := fmt.Sprintf("%s%s%s", ColorGray, timestamp, ColorReset)
	fmt.Printf("[%s] %s %s\n", levelStr, timeStr, message)
}

// Migration logs the start of a migration.
func (l *MigrationLogger) Migration(name string, version string) {
	if l.machineOut {
		kvLine(LogInfo, "migration_start", "file", name, "version", version)
		return
	}
	fmt.Printf("%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", ColorCyan, ColorReset)
	fmt.Printf("%s▶ Migration:%s %s\n", ColorCyan, ColorReset, name)
	if version != "" {
		fmt.Printf("%s  Version:%s %s\n", ColorGray, ColorReset, version)
	}
	fmt.Printf("%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", ColorCyan, ColorReset)
}

// MigrationComplete logs the completion of a migration with elapsed time.
func (l *MigrationLogger) MigrationComplete(name string, elapsed time.Duration) {
	if l.machineOut {
		kvLine(LogSuccess, "migration_complete", "file", name, "elapsed", formatDuration(elapsed))
		return
	}
	fmt.Printf("%s✓ Completed:%s %s %s(%s)%s\n",
		ColorGreen, ColorReset, name,
		ColorGray, formatDuration(elapsed), ColorReset)
}

// MigrationFailed logs a failed migration.
func (l *MigrationLogger) MigrationFailed(name string, err error) {
	if l.machineOut {
		kvLine(LogError, "migration_failed", "file", name, "error", err.Error())
		return
	}
	fmt.Printf("%s✗ Failed:%s %s\n", ColorRed, ColorReset, name)
	fmt.Printf("%s  Error:%s %v\n", ColorRed, ColorReset, err)
}

// Step logs a migration step.
func (l *MigrationLogger) Step(step string) {
	if l.machineOut {
		kvLine(LogInfo, "step", "msg", step)
		return
	}
	fmt.Printf("  %s•%s %s\n", ColorBlue, ColorReset, step)
}

// StepComplete logs a completed step with timing.
func (l *MigrationLogger) StepComplete(step string, elapsed time.Duration) {
	if l.machineOut {
		kvLine(LogSuccess, "step_complete", "msg", step, "elapsed", formatDuration(elapsed))
		return
	}
	fmt.Printf("  %s✓%s %s %s(%s)%s\n",
		ColorGreen, ColorReset, step,
		ColorGray, formatDuration(elapsed), ColorReset)
}

// StepFailed logs a failed step.
func (l *MigrationLogger) StepFailed(step string, err error) {
	if l.machineOut {
		kvLine(LogError, "step_failed", "msg", step, "error", err.Error())
		return
	}
	fmt.Printf("  %s✗%s %s: %v\n", ColorRed, ColorReset, step, err)
}

// Summary logs a summary of the migration run.
func (l *MigrationLogger) Summary(successful, failed, total int) {
	elapsed := time.Since(l.startTime)
	if l.machineOut {
		kvLine(LogInfo, "summary",
			"total", fmt.Sprintf("%d", total),
			"successful", fmt.Sprintf("%d", successful),
			"failed", fmt.Sprintf("%d", failed),
			"elapsed", formatDuration(elapsed),
		)
		return
	}
	fmt.Printf("\n%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", ColorCyan, ColorReset)
	fmt.Printf("%sSummary%s\n", ColorBold, ColorReset)
	fmt.Printf("  Total migrations:     %d\n", total)
	fmt.Printf("  %sSuccessful:%s          %d\n", ColorGreen, ColorReset, successful)
	if failed > 0 {
		fmt.Printf("  %sFailed:%s             %d\n", ColorRed, ColorReset, failed)
	}
	fmt.Printf("  Total time:           %s\n", formatDuration(elapsed))
	fmt.Printf("%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", ColorCyan, ColorReset)
}

// Prompt displays a user prompt (always human-readable — machine consumers
// should use --drift-handling=auto to avoid interactive prompts).
func (l *MigrationLogger) Prompt(question string) {
	fmt.Printf("%s? %s%s ", ColorYellow, question, ColorReset)
}

// TableHeader prints a table header row.
func (l *MigrationLogger) TableHeader(headers ...string) {
	if l.machineOut {
		kvLine(LogInfo, "table_header", "columns", strings.Join(headers, ","))
		return
	}
	for i, header := range headers {
		fmt.Printf("%s%-30s%s", ColorBold, header, ColorReset)
		if i < len(headers)-1 {
			fmt.Printf(" │ ")
		}
	}
	fmt.Println()
	fmt.Println(ColorGray + "─────────────────────────────────────────────────────────────────────────────────" + ColorReset)
}

// TableRow prints a table data row.
func (l *MigrationLogger) TableRow(cells ...string) {
	if l.machineOut {
		kvLine(LogInfo, "table_row", "values", strings.Join(cells, ","))
		return
	}
	for i, cell := range cells {
		fmt.Printf("%-30s", cell)
		if i < len(cells)-1 {
			fmt.Printf(" │ ")
		}
	}
	fmt.Println()
}

// Progress shows a progress indicator.
func (l *MigrationLogger) Progress(current, total int, message string) {
	if l.machineOut {
		kvLine(LogInfo, "progress",
			"current", fmt.Sprintf("%d", current),
			"total", fmt.Sprintf("%d", total),
			"msg", message,
		)
		return
	}
	percentage := float64(current) / float64(total) * 100
	fmt.Printf("\r%s[%d/%d]%s %s%.0f%%%s %s",
		ColorCyan, current, total, ColorReset,
		ColorGreen, percentage, ColorReset,
		message)
}

// ProgressComplete clears the progress line and shows completion.
func (l *MigrationLogger) ProgressComplete() {
	if !l.machineOut {
		fmt.Println()
	}
}

// formatDuration formats a duration for display.
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
	return fmt.Sprintf("%.2fm", d.Minutes())
}

// Checksum logs checksum validation.
func (l *MigrationLogger) Checksum(name string, match bool) {
	if match {
		l.Debug("Checksum verified for %s", name)
	} else {
		l.Warning("Checksum mismatch for %s - file may have been modified", name)
	}
}

// Lock logs lock acquisition.
func (l *MigrationLogger) Lock(acquired bool) {
	if acquired {
		l.Debug("Migration lock acquired")
	} else {
		l.Warning("Waiting for migration lock...")
	}
}

// SchemaDrift logs detected schema drift.
func (l *MigrationLogger) SchemaDrift(table string, columns []string) {
	if l.machineOut {
		kvLine(LogWarning, "schema_drift", "table", table, "columns", strings.Join(columns, ","))
		return
	}
	l.Warning("Schema drift detected in table '%s'", table)
	for _, col := range columns {
		fmt.Printf("  %s+%s %s\n", ColorYellow, ColorReset, col)
	}
}
