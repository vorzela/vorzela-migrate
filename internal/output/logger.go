package output

import (
	"fmt"
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

// MigrationLogger provides colored, formatted logging for migrations
type MigrationLogger struct {
	startTime time.Time
	verbose   bool
}

// NewMigrationLogger creates a new migration logger
func NewMigrationLogger(verbose bool) *MigrationLogger {
	return &MigrationLogger{
		startTime: time.Now(),
		verbose:   verbose,
	}
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

// log formats and prints a log message with color and timestamp
func (l *MigrationLogger) log(level LogLevel, color string, format string, args ...interface{}) {
	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf(format, args...)

	levelStr := fmt.Sprintf("%s%-7s%s", color, level, ColorReset)
	timeStr := fmt.Sprintf("%s%s%s", ColorGray, timestamp, ColorReset)

	fmt.Printf("[%s] %s %s\n", levelStr, timeStr, message)
}

// Migration logs the start of a migration
func (l *MigrationLogger) Migration(name string, version string) {
	fmt.Printf("%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", ColorCyan, ColorReset)
	fmt.Printf("%s▶ Migration:%s %s\n", ColorCyan, ColorReset, name)
	if version != "" {
		fmt.Printf("%s  Version:%s %s\n", ColorGray, ColorReset, version)
	}
	fmt.Printf("%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", ColorCyan, ColorReset)
}

// MigrationComplete logs the completion of a migration with elapsed time
func (l *MigrationLogger) MigrationComplete(name string, elapsed time.Duration) {
	fmt.Printf("%s✓ Completed:%s %s %s(%s)%s\n",
		ColorGreen, ColorReset, name,
		ColorGray, formatDuration(elapsed), ColorReset)
}

// MigrationFailed logs a failed migration
func (l *MigrationLogger) MigrationFailed(name string, err error) {
	fmt.Printf("%s✗ Failed:%s %s\n", ColorRed, ColorReset, name)
	fmt.Printf("%s  Error:%s %v\n", ColorRed, ColorReset, err)
}

// Step logs a migration step
func (l *MigrationLogger) Step(step string) {
	fmt.Printf("  %s•%s %s\n", ColorBlue, ColorReset, step)
}

// StepComplete logs a completed step with timing
func (l *MigrationLogger) StepComplete(step string, elapsed time.Duration) {
	fmt.Printf("  %s✓%s %s %s(%s)%s\n",
		ColorGreen, ColorReset, step,
		ColorGray, formatDuration(elapsed), ColorReset)
}

// StepFailed logs a failed step
func (l *MigrationLogger) StepFailed(step string, err error) {
	fmt.Printf("  %s✗%s %s: %v\n", ColorRed, ColorReset, step, err)
}

// Summary logs a summary of the migration run
func (l *MigrationLogger) Summary(successful, failed, total int) {
	elapsed := time.Since(l.startTime)

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

// Prompt displays a user prompt
func (l *MigrationLogger) Prompt(question string) {
	fmt.Printf("%s? %s%s ", ColorYellow, question, ColorReset)
}

// Table prints a simple table header
func (l *MigrationLogger) TableHeader(headers ...string) {
	for i, header := range headers {
		fmt.Printf("%s%-30s%s", ColorBold, header, ColorReset)
		if i < len(headers)-1 {
			fmt.Printf(" │ ")
		}
	}
	fmt.Println()
	fmt.Println(ColorGray + "─────────────────────────────────────────────────────────────────────────────────" + ColorReset)
}

// TableRow prints a table row
func (l *MigrationLogger) TableRow(cells ...string) {
	for i, cell := range cells {
		fmt.Printf("%-30s", cell)
		if i < len(cells)-1 {
			fmt.Printf(" │ ")
		}
	}
	fmt.Println()
}

// Progress shows a progress indicator
func (l *MigrationLogger) Progress(current, total int, message string) {
	percentage := float64(current) / float64(total) * 100
	fmt.Printf("\r%s[%d/%d]%s %s%.0f%%%s %s",
		ColorCyan, current, total, ColorReset,
		ColorGreen, percentage, ColorReset,
		message)
}

// ProgressComplete clears the progress line and shows completion
func (l *MigrationLogger) ProgressComplete() {
	fmt.Println()
}

// formatDuration formats a duration for display
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
	return fmt.Sprintf("%.2fm", d.Minutes())
}

// Checksum logs checksum validation
func (l *MigrationLogger) Checksum(name string, match bool) {
	if match {
		l.Debug("Checksum verified for %s", name)
	} else {
		l.Warning("Checksum mismatch for %s - file may have been modified", name)
	}
}

// Lock logs lock acquisition
func (l *MigrationLogger) Lock(acquired bool) {
	if acquired {
		l.Debug("Migration lock acquired")
	} else {
		l.Warning("Waiting for migration lock...")
	}
}

// SchemaDrift logs detected schema drift
func (l *MigrationLogger) SchemaDrift(table string, columns []string) {
	l.Warning("Schema drift detected in table '%s'", table)
	for _, col := range columns {
		fmt.Printf("  %s+%s %s\n", ColorYellow, ColorReset, col)
	}
}
