package output

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
)

// Confirm displays a Y/N prompt and returns true if the user answers "y".
func Confirm(question string) bool {
	Yellow.Printf("\n%s [y/N]: ", question)
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	fmt.Println()
	return strings.ToLower(strings.TrimSpace(answer)) == "y"
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
	Gray.Println(string(make([]byte, 80)))
}

// Header prints a header
func Header(title string) {
	Cyan.Printf("\n🐘 %s\n", title)
	Gray.Println(string(make([]byte, 80)))
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
