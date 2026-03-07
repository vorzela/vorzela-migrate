package config

import (
	"fmt"
	"os"
	"strings"
)

// LintSeverity indicates whether a lint issue is an error or a warning.
type LintSeverity string

const (
	LintError   LintSeverity = "error"
	LintWarning LintSeverity = "warning"
)

// LintIssue represents a single problem found in a .vm file.
type LintIssue struct {
	Line     int
	Key      string
	Message  string
	Severity LintSeverity
}

func (l LintIssue) String() string {
	return fmt.Sprintf("line %d [%s] %s: %s", l.Line, l.Severity, l.Key, l.Message)
}

// knownKeys is the authoritative set of recognised .vm keys.
var knownKeys = map[string]struct{}{
	"DATABASE_URL":        {},
	"MIGRATION_PATH":      {},
	"SQLC_SUPPORT":        {},
	"ENVIRONMENT":         {},
	"ENV":                 {},
	"ENHANCED":            {},
	"ONLINE":              {},
	"VERIFY_CHECKSUMS":    {},
	"DETECT_DRIFT":        {},
	"VERBOSE":             {},
	"AUTO_RUN_EXTENSIONS": {},
	"AUTO_RUN_FUNCTIONS":  {},
	"AUTO_RUN_ENUMS":      {},
	"DRIFT_HANDLING":      {},
}

// boolKeys is the subset of keys whose value must be a boolean.
var boolKeys = map[string]struct{}{
	"SQLC_SUPPORT":        {},
	"ENHANCED":            {},
	"ONLINE":              {},
	"VERIFY_CHECKSUMS":    {},
	"DETECT_DRIFT":        {},
	"VERBOSE":             {},
	"AUTO_RUN_EXTENSIONS": {},
	"AUTO_RUN_FUNCTIONS":  {},
	"AUTO_RUN_ENUMS":      {},
}

// validEnvironments is the set of accepted ENVIRONMENT / ENV values.
var validEnvironments = map[string]struct{}{
	"development": {},
	"dev":         {},
	"develop":     {},
	"production":  {},
	"prod":        {},
}

// validDriftHandling is the set of accepted DRIFT_HANDLING values.
var validDriftHandling = map[string]struct{}{
	"auto":   {},
	"reject": {},
	"prompt": {},
}

// isValidBool returns true for "true", "false", "1", "0".
func isValidBool(v string) bool {
	switch strings.ToLower(v) {
	case "true", "false", "1", "0":
		return true
	}
	return false
}

// LintVMFile reads a .vm file and returns all detected issues.
// It returns (nil, nil) when the file does not exist — absence is not an error.
func LintVMFile(path string) ([]LintIssue, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("could not read %s: %w", path, err)
	}

	var issues []LintIssue
	seen := make(map[string]int) // key → first line number
	hasDatabaseURL := false

	lines := strings.Split(string(content), "\n")
	for i, rawLine := range lines {
		lineNum := i + 1
		line := strings.TrimSpace(rawLine)

		// Skip blank lines and full-line comments.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			issues = append(issues, LintIssue{
				Line:     lineNum,
				Key:      line,
				Message:  "malformed line — expected KEY=VALUE",
				Severity: LintWarning,
			})
			continue
		}

		key := strings.TrimSpace(parts[0])

		// Strip inline # comment from value.
		rawValue := parts[1]
		if idx := strings.Index(rawValue, " #"); idx != -1 {
			rawValue = rawValue[:idx]
		}
		value := strings.TrimSpace(rawValue)

		// Unknown key check.
		if _, known := knownKeys[key]; !known {
			// Suggest closest known key if there is an obvious typo.
			suggestion := closestKey(key)
			msg := "unknown key"
			if suggestion != "" {
				msg = fmt.Sprintf("unknown key — did you mean %s?", suggestion)
			}
			issues = append(issues, LintIssue{
				Line:     lineNum,
				Key:      key,
				Message:  msg,
				Severity: LintError,
			})
			continue // Don't validate value of unknown keys.
		}

		// Duplicate key check.
		if firstLine, dup := seen[key]; dup {
			issues = append(issues, LintIssue{
				Line:     lineNum,
				Key:      key,
				Message:  fmt.Sprintf("duplicate key (first defined on line %d)", firstLine),
				Severity: LintWarning,
			})
		} else {
			seen[key] = lineNum
		}

		// Boolean value check.
		if _, isBool := boolKeys[key]; isBool {
			if !isValidBool(value) {
				issues = append(issues, LintIssue{
					Line:     lineNum,
					Key:      key,
					Message:  fmt.Sprintf("invalid boolean value %q — expected true, false, 1, or 0", value),
					Severity: LintError,
				})
			}
		}

		// ENVIRONMENT / ENV enum check.
		if key == "ENVIRONMENT" || key == "ENV" {
			if _, ok := validEnvironments[strings.ToLower(value)]; !ok {
				issues = append(issues, LintIssue{
					Line:     lineNum,
					Key:      key,
					Message:  fmt.Sprintf("invalid value %q — expected development, dev, production, or prod", value),
					Severity: LintError,
				})
			}
		}

		// DRIFT_HANDLING enum check.
		if key == "DRIFT_HANDLING" {
			if _, ok := validDriftHandling[strings.ToLower(value)]; !ok {
				issues = append(issues, LintIssue{
					Line:     lineNum,
					Key:      key,
					Message:  fmt.Sprintf("invalid value %q — expected auto, prompt, or reject", value),
					Severity: LintError,
				})
			}
		}

		// DATABASE_URL presence tracking.
		if key == "DATABASE_URL" && value != "" {
			hasDatabaseURL = true
		}
	}

	// Warn when DATABASE_URL is absent — the tool cannot run without it.
	if !hasDatabaseURL {
		issues = append(issues, LintIssue{
			Line:     0,
			Key:      "DATABASE_URL",
			Message:  "DATABASE_URL is not set — the tool requires a database connection string",
			Severity: LintWarning,
		})
	}

	return issues, nil
}

// closestKey returns the known key that most closely matches the typo, using a
// simple one-character prefix + suffix heuristic. Returns "" when nothing is
// close enough to be worth suggesting.
func closestKey(typo string) string {
	upper := strings.ToUpper(typo)
	best := ""
	bestScore := 0

	for k := range knownKeys {
		score := similarity(upper, k)
		if score > bestScore {
			bestScore = score
			best = k
		}
	}

	// Only suggest when there is a meaningful overlap (>50 % of the shorter string).
	shorter := len(upper)
	if len(best) < shorter {
		shorter = len(best)
	}
	if shorter > 0 && bestScore*2 >= shorter {
		return best
	}
	return ""
}

// similarity counts common characters at matching positions (longest common
// prefix) plus a bonus for a shared suffix — good enough for short key names.
func similarity(a, b string) int {
	score := 0
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	for i := 0; i < limit; i++ {
		if a[i] == b[i] {
			score++
		}
	}
	// Bonus: shared suffix (last 3 characters)
	if len(a) >= 3 && len(b) >= 3 && a[len(a)-3:] == b[len(b)-3:] {
		score += 2
	}
	return score
}
