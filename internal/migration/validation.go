package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ValidationError represents a migration validation error
type ValidationError struct {
	File    string
	Message string
	Line    string
}

func (e *ValidationError) Error() string {
	if e.Line != "" {
		return fmt.Sprintf("%s: %s\n   Found: %s", e.File, e.Message, e.Line)
	}
	return fmt.Sprintf("%s: %s", e.File, e.Message)
}

// ValidateMigrationContent checks that migration files don't contain functions or extensions
func ValidateMigrationContent(filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read migration file: %w", err)
	}

	contentStr := string(content)
	fileName := filepath.Base(filePath)

	// Check for function definitions
	if containsFunctionDefinition(contentStr) {
		line := extractMatchingLine(contentStr, `CREATE\s+(OR\s+REPLACE\s+)?FUNCTION`)
		return &ValidationError{
			File:    fileName,
			Message: "Migration files should not contain function definitions. Use 'vm functions migrate' to install functions from functions.sql",
			Line:    line,
		}
	}

	// Check for extension creation
	if containsExtensionCreation(contentStr) {
		line := extractMatchingLine(contentStr, `CREATE\s+EXTENSION`)
		return &ValidationError{
			File:    fileName,
			Message: "Migration files should not contain extension creation. Use 'vm extensions migrate' to install extensions from extensions.sql",
			Line:    line,
		}
	}

	return nil
}

// ValidateFunctionsFile checks that functions.sql doesn't contain extension creation
func ValidateFunctionsFile(filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read functions file: %w", err)
	}

	contentStr := string(content)

	if containsExtensionCreation(contentStr) {
		line := extractMatchingLine(contentStr, `CREATE\s+EXTENSION`)
		return &ValidationError{
			File:    "functions.sql",
			Message: "functions.sql should not contain extension creation. Use extensions.sql for that",
			Line:    line,
		}
	}

	return nil
}

// ValidateExtensionsFile checks that extensions.sql doesn't contain function definitions
func ValidateExtensionsFile(filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read extensions file: %w", err)
	}

	contentStr := string(content)

	if containsFunctionDefinition(contentStr) {
		line := extractMatchingLine(contentStr, `CREATE\s+(OR\s+REPLACE\s+)?FUNCTION`)
		return &ValidationError{
			File:    "extensions.sql",
			Message: "extensions.sql should not contain function definitions. Use functions.sql for that",
			Line:    line,
		}
	}

	return nil
}

// ValidateAllMigrations validates all migration files in a directory
func ValidateAllMigrations(migrationPath string) []error {
	var errors []error

	files, err := getMigrationFiles(migrationPath)
	if err != nil {
		return []error{fmt.Errorf("failed to read migration files: %w", err)}
	}

	for _, file := range files {
		filePath := filepath.Join(migrationPath, file.Filename)
		if err := ValidateMigrationContent(filePath); err != nil {
			errors = append(errors, err)
		}
	}

	// Validate functions.sql if it exists
	functionsPath := filepath.Join(migrationPath, "functions.sql")
	if _, err := os.Stat(functionsPath); err == nil {
		if err := ValidateFunctionsFile(functionsPath); err != nil {
			errors = append(errors, err)
		}
	}

	// Validate extensions.sql if it exists
	extensionsPath := filepath.Join(migrationPath, "extensions.sql")
	if _, err := os.Stat(extensionsPath); err == nil {
		if err := ValidateExtensionsFile(extensionsPath); err != nil {
			errors = append(errors, err)
		}
	}

	return errors
}

// containsFunctionDefinition checks if content contains CREATE FUNCTION statements
func containsFunctionDefinition(content string) bool {
	// Match CREATE FUNCTION or CREATE OR REPLACE FUNCTION
	// Case-insensitive, ignoring comments
	pattern := `(?i)CREATE\s+(OR\s+REPLACE\s+)?FUNCTION\s+\w+`
	re := regexp.MustCompile(pattern)

	// Filter out commented lines
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip comment lines
		if strings.HasPrefix(trimmed, "--") || strings.HasPrefix(trimmed, "/*") {
			continue
		}
		if re.MatchString(line) {
			return true
		}
	}

	return false
}

// containsExtensionCreation checks if content contains CREATE EXTENSION statements
func containsExtensionCreation(content string) bool {
	// Match CREATE EXTENSION (case-insensitive)
	pattern := `(?i)CREATE\s+EXTENSION`
	re := regexp.MustCompile(pattern)

	// Filter out commented lines
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip comment lines
		if strings.HasPrefix(trimmed, "--") || strings.HasPrefix(trimmed, "/*") {
			continue
		}
		if re.MatchString(line) {
			return true
		}
	}

	return false
}

// extractMatchingLine extracts the first line matching a pattern
func extractMatchingLine(content, pattern string) string {
	re := regexp.MustCompile("(?i)" + pattern)
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") || strings.HasPrefix(trimmed, "/*") {
			continue
		}
		if re.MatchString(line) {
			if len(trimmed) > 80 {
				return trimmed[:80] + "..."
			}
			return trimmed
		}
	}

	return ""
}

// CheckDependenciesInstalled verifies that required dependencies are installed
func CheckDependenciesInstalled(db interface{}, dsn string) error {
	// Note: This would require database-specific queries
	// For now, we'll return nil and enhance in future
	// This is a placeholder for checking if extensions and functions are installed
	return nil
}
