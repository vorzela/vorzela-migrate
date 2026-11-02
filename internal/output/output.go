package output

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
)

var (
	nonInteractive bool
)

// SetNonInteractive controls whether Confirm may read from stdin. In
// non-interactive mode ConfirmDefault returns the supplied default instead of
// blocking an agent or CI job.
func SetNonInteractive(v bool) { nonInteractive = v }

// DisableColor turns off ANSI colour for the output helpers.
func DisableColor() { color.NoColor = true }

// IsInteractive reports whether prompts may read stdin.
func IsInteractive() bool { return !nonInteractive }

// Confirm displays a Y/N prompt and returns true if the user answers yes or y.
// It is safe in non-interactive mode (returns false).
func Confirm(question string) bool {
	return ConfirmDefault(question, false)
}

// ConfirmDefault displays a Y/N prompt and returns defaultValue when the
// process is non-interactive. It accepts y, yes (case-insensitive).
//
// It deliberately ignores convenience flags: force/yes shortcuts must not
// silently approve destructive operations such as `vm fresh`.
func ConfirmDefault(question string, defaultValue bool) bool {
	if nonInteractive {
		return defaultValue
	}
	Yellow.Printf("\n%s [y/N]: ", question)
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	fmt.Println()
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	default:
		return defaultValue
	}
}

// Color constants
var (
	// Success colors
	Green = color.New(color.FgGreen, color.Bold)
	Cyan  = color.New(color.FgCyan, color.Bold)

	// Warning/Error colors
	Yellow = color.New(color.FgYellow, color.Bold)
	Red    = color.New(color.FgRed, color.Bold)

	// Neutral
	White = color.New(color.FgWhite)
	Gray  = color.New(color.FgWhite, color.Faint)
)

// Success prints a success message
func Success(msg string, args ...interface{}) {
	Green.Printf("✓ "+msg+"\n", args...)
}

// Error prints an error message
func Error(msg string, args ...interface{}) {
	Red.Printf("✗ "+msg+"\n", args...)
}

// Warning prints a warning message
func Warning(msg string, args ...interface{}) {
	Yellow.Printf("⚠ "+msg+"\n", args...)
}

// Info prints an info message
func Info(msg string, args ...interface{}) {
	Cyan.Printf("ℹ "+msg+"\n", args...)
}

// Println prints a line
func Println(msg string, args ...interface{}) {
	White.Printf(msg+"\n", args...)
}

// Separator prints a separator line
func Separator() {
	Gray.Println(strings.Repeat("─", 80))
}

// Header prints a header
func Header(title string) {
	Cyan.Printf("\n🐘 %s\n", title)
	Gray.Println(strings.Repeat("─", 80))
}

// TableRow prints a table row with alignment
func TableRow(left, right string) {
	fmt.Printf("%-40s | %s\n", left, right)
}

// ExecutedStatus returns colored migration status
func ExecutedStatus(batch int) string {
	return Green.Sprintf("✓ Batch %d", batch)
}

// PendingStatus returns colored pending status
func PendingStatus() string {
	return Yellow.Sprintf("⏳ Pending")
}

// WarningStatus returns a colored warning status label.
func WarningStatus(label string) string {
	return Yellow.Sprintf("⚠ %s", label)
}

// Migrated returns colored migrated message
func Migrated(filename string) string {
	return Green.Sprintf("✓ Migrated: %s", filename)
}

// RolledBack returns colored rollback message
func RolledBack(filename string) string {
	return Green.Sprintf("✓ Rolled back: %s", filename)
}

// SummaryLine returns a colored summary line
func SummaryLine(label string, count int) string {
	if count > 0 {
		return Green.Sprintf("✓ %s: %d", label, count)
	}
	return Gray.Sprintf("  %s: %d", label, count)
}
