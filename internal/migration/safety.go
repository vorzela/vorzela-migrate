package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// SafetySeverity classifies a migration safety finding.
type SafetySeverity string

const (
	SafetyError   SafetySeverity = "error"
	SafetyWarning SafetySeverity = "warning"
	SafetyInfo    SafetySeverity = "info"
)

// SafetyIssue is one finding produced by the migration safety checker.
type SafetyIssue struct {
	File        string         `json:"file,omitempty"`
	Section     string         `json:"section,omitempty"`
	Line        int            `json:"line,omitempty"`
	Code        string         `json:"code"`
	Severity    SafetySeverity `json:"severity"`
	Message     string         `json:"message"`
	Statement   string         `json:"statement,omitempty"`
	Destructive bool           `json:"destructive,omitempty"`
}

func (i SafetyIssue) String() string {
	loc := i.File
	if i.Section != "" {
		loc += " [" + i.Section + "]"
	}
	if i.Line > 0 {
		loc += ":" + strconv.Itoa(i.Line)
	}
	return fmt.Sprintf("%s: %s: %s", loc, i.Severity, i.Message)
}

// SafetyReport is the complete result of checking a migrations directory.
type SafetyReport struct {
	Path         string        `json:"path"`
	Files        int           `json:"files"`
	Migrations   int           `json:"migrations"`
	Issues       []SafetyIssue `json:"issues"`
	CheckedFiles []string      `json:"checked_files,omitempty"`
}

// Errors returns the number of error-level issues.
func (r *SafetyReport) Errors() int {
	n := 0
	for _, i := range r.Issues {
		if i.Severity == SafetyError {
			n++
		}
	}
	return n
}

// Warnings returns the number of warning-level issues.
func (r *SafetyReport) Warnings() int {
	n := 0
	for _, i := range r.Issues {
		if i.Severity == SafetyWarning {
			n++
		}
	}
	return n
}

// DestructiveCount returns the number of destructive statements found.
func (r *SafetyReport) DestructiveCount() int {
	n := 0
	for _, i := range r.Issues {
		if i.Destructive {
			n++
		}
	}
	return n
}

// HasErrors reports whether the report contains any hard errors.
func (r *SafetyReport) HasErrors() bool { return r.Errors() > 0 }

// Add appends an issue to the report.
func (r *SafetyReport) Add(issue SafetyIssue) { r.Issues = append(r.Issues, issue) }

// Sort orders issues deterministically: file, line, severity.
func (r *SafetyReport) Sort() {
	sort.SliceStable(r.Issues, func(i, j int) bool {
		a, b := r.Issues[i], r.Issues[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Severity != b.Severity {
			return severityRank(a.Severity) < severityRank(b.Severity)
		}
		return a.Code < b.Code
	})
}

func severityRank(s SafetySeverity) int {
	switch s {
	case SafetyError:
		return 0
	case SafetyWarning:
		return 1
	default:
		return 2
	}
}

// helperSQLFiles are managed by vm extensions/functions/enums commands and are
// not executed as migrations.
var helperSQLFiles = map[string]bool{
	"extensions.sql": true,
	"functions.sql":  true,
	"enums.sql":      true,
}

var timestampPrefixRe = regexp.MustCompile(`^(\d+)_`)

// CheckMigrationFiles performs static, database-free safety checks on a
// migrations directory. It is the engine behind `vm check`.
func CheckMigrationFiles(migrationPath string) *SafetyReport {
	report := &SafetyReport{Path: migrationPath, Issues: []SafetyIssue{}}

	entries, err := os.ReadDir(migrationPath)
	if err != nil {
		if os.IsNotExist(err) {
			report.Add(SafetyIssue{
				Code:     "migration_dir_missing",
				Severity: SafetyWarning,
				Message:  fmt.Sprintf("migrations directory %q does not exist (vm init creates it)", migrationPath),
			})
			return report
		}
		report.Add(SafetyIssue{
			Code:     "migration_dir_read",
			Severity: SafetyError,
			Message:  fmt.Sprintf("could not read migrations directory: %v", err),
		})
		return report
	}

	timestamps := make(map[string][]string)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".sql") {
			continue
		}

		name := entry.Name()
		fullPath := filepath.Join(migrationPath, name)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			report.Add(SafetyIssue{
				File:     name,
				Code:     "read_error",
				Severity: SafetyError,
				Message:  fmt.Sprintf("could not read file: %v", err),
			})
			continue
		}
		report.Files++
		report.CheckedFiles = append(report.CheckedFiles, name)
		text := string(content)

		if helperSQLFiles[strings.ToLower(name)] {
			checkHelperFile(report, name, text)
			continue
		}

		// Files without a timestamp prefix are silently ignored by the runner.
		// Surface that loudly: users often create 0001_name.sql and wonder why
		// nothing happened.
		if m := timestampPrefixRe.FindStringSubmatch(name); m != nil {
			timestamps[m[1]] = append(timestamps[m[1]], name)
			report.Migrations++
		} else {
			report.Add(SafetyIssue{
				File:     name,
				Code:     "missing_timestamp",
				Severity: SafetyWarning,
				Message:  "filename has no numeric timestamp prefix; vm ignores this file during migrate",
			})
			continue
		}

		checkMigrationFile(report, name, text)
	}

	for ts, files := range timestamps {
		if len(files) > 1 {
			sort.Strings(files)
			report.Add(SafetyIssue{
				File:     files[0],
				Code:     "duplicate_timestamp",
				Severity: SafetyWarning,
				Message:  fmt.Sprintf("timestamp %s is shared by %d migrations (%s); ordering is ambiguous", ts, len(files), strings.Join(files, ", ")),
			})
		}
	}

	report.Sort()
	return report
}

func checkHelperFile(report *SafetyReport, name, content string) {
	lower := strings.ToLower(name)
	switch lower {
	case "extensions.sql":
		if containsFunctionDefinition(content) {
			report.Add(SafetyIssue{
				File:     name,
				Code:     "function_in_extensions",
				Severity: SafetyError,
				Message:  "extensions.sql contains a CREATE FUNCTION — move it to functions.sql",
			})
		}
	case "functions.sql":
		if containsExtensionCreation(content) {
			report.Add(SafetyIssue{
				File:     name,
				Code:     "extension_in_functions",
				Severity: SafetyError,
				Message:  "functions.sql contains CREATE EXTENSION — move it to extensions.sql",
			})
		}
	case "enums.sql":
		if containsExtensionCreation(content) {
			report.Add(SafetyIssue{
				File:     name,
				Code:     "extension_in_enums",
				Severity: SafetyError,
				Message:  "enums.sql contains CREATE EXTENSION — move it to extensions.sql",
			})
		}
	}
}

func checkMigrationFile(report *SafetyReport, name, content string) {
	hasUp := false
	hasDown := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if isUpMarker(trimmed) {
			hasUp = true
		}
		if isDownMarker(trimmed) {
			hasDown = true
		}
	}

	if !hasUp {
		report.Add(SafetyIssue{
			File:     name,
			Code:     "missing_up_marker",
			Severity: SafetyError,
			Message:  "no Up section marker found (-- Up, -- ⬆ Up, -- +goose Up, or -- migrate:up); vm would skip this migration",
		})
		return
	}
	if !hasDown {
		report.Add(SafetyIssue{
			File:     name,
			Code:     "missing_down_marker",
			Severity: SafetyWarning,
			Message:  "no Down section marker found; this migration cannot be rolled back",
		})
	}

	upSQL := extractSection(content, "Up")
	downSQL := extractSection(content, "Down")
	if strings.TrimSpace(upSQL) == "" {
		report.Add(SafetyIssue{
			File:     name,
			Section:  "Up",
			Code:     "empty_up",
			Severity: SafetyError,
			Message:  "Up section contains no executable SQL",
		})
	}
	if hasDown && strings.TrimSpace(downSQL) == "" {
		report.Add(SafetyIssue{
			File:     name,
			Section:  "Down",
			Code:     "empty_down",
			Severity: SafetyWarning,
			Message:  "Down section contains no executable SQL; rollback will be a no-op or fail",
		})
	}

	noTx := HasNoTransactionMarker(content)
	checkSQLForSafety(report, name, "Up", upSQL, noTx, true)
	if downSQL != "" {
		// Down SQL is not executed by `vm migrate`; do not make normal
		// DROP TABLE rollbacks fail a strict migrate safety check.
		checkSQLForSafety(report, name, "Down", downSQL, noTx, false)
	}
}

// AnalyzeSQL exposes the statement-level checks for callers that already have
// SQL text (for example drift-generated migrations).
func AnalyzeSQL(file, section, sql string, noTransaction bool) []SafetyIssue {
	report := &SafetyReport{}
	checkSQLForSafety(report, file, section, sql, noTransaction, true)
	report.Sort()
	return report.Issues
}

func checkSQLForSafety(report *SafetyReport, file, section, sql string, noTransaction bool, reportDestructive bool) {
	statements, err := SplitSQLStatements(sql)
	if err != nil {
		report.Add(SafetyIssue{
			File:     file,
			Section:  section,
			Code:     "sql_parse",
			Severity: SafetyError,
			Message:  err.Error(),
		})
		return
	}
	if len(statements) == 0 {
		return
	}

	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		line := lineOfStatement(sql, stmt)

		if containsFunctionDefinition(stmt) {
			report.Add(SafetyIssue{
				File:      file,
				Section:   section,
				Line:      line,
				Code:      "function_in_migration",
				Severity:  SafetyError,
				Message:   "migration contains CREATE FUNCTION — put it in functions.sql and run 'vm functions migrate'",
				Statement: shortenStatement(stmt),
			})
			continue
		}
		if containsExtensionCreation(stmt) {
			report.Add(SafetyIssue{
				File:      file,
				Section:   section,
				Line:      line,
				Code:      "extension_in_migration",
				Severity:  SafetyError,
				Message:   "migration contains CREATE EXTENSION — put it in extensions.sql and run 'vm extensions migrate'",
				Statement: shortenStatement(stmt),
			})
			continue
		}

		if StatementControlsTransaction(stmt) {
			report.Add(SafetyIssue{
				File:      file,
				Section:   section,
				Line:      line,
				Code:      "transaction_control",
				Severity:  SafetyError,
				Message:   "remove explicit BEGIN/COMMIT/ROLLBACK — vm manages transactions",
				Statement: shortenStatement(stmt),
			})
		} else if StatementNeedsNoTransaction(stmt) && !noTransaction {
			report.Add(SafetyIssue{
				File:      file,
				Section:   section,
				Line:      line,
				Code:      "transaction_hostile",
				Severity:  SafetyWarning,
				Message:   "statement cannot run inside a transaction block; add '-- vm:no-transaction' to the migration",
				Statement: shortenStatement(stmt),
			})
		}

		if destructive, reason := IsDestructiveStatement(stmt); destructive && reportDestructive {
			report.Add(SafetyIssue{
				File:        file,
				Section:     section,
				Line:        line,
				Code:        "destructive_sql",
				Severity:    SafetyWarning,
				Message:     "destructive statement (" + reason + ")",
				Statement:   shortenStatement(stmt),
				Destructive: true,
			})
		}

		if code, msg := unbalancedParens(stmt); code != "" {
			report.Add(SafetyIssue{
				File:      file,
				Section:   section,
				Line:      line,
				Code:      code,
				Severity:  SafetyError,
				Message:   msg,
				Statement: shortenStatement(stmt),
			})
		}
	}
}

// IsDestructiveStatement reports whether a statement can remove data or schema
// objects. The returned reason is suitable for display.
func IsDestructiveStatement(stmt string) (bool, string) {
	normalized := strings.ToUpper(strings.Join(strings.Fields(stripSQLLineComments(stmt)), " "))

	prefixes := []struct {
		prefix string
		reason string
	}{
		{"DROP TABLE", "drops a table and its data"},
		{"DROP SCHEMA", "drops a schema"},
		{"DROP DATABASE", "drops a database"},
		{"TRUNCATE", "removes all rows"},
		{"DROP COLUMN", "drops a column and its data"},
		{"DROP TYPE", "drops a type"},
		{"DROP EXTENSION", "drops an extension and dependent objects"},
		{"DROP FUNCTION", "drops a function and dependent objects"},
	}
	for _, p := range prefixes {
		if strings.HasPrefix(normalized, p.prefix) {
			return true, p.reason
		}
	}

	// Only dropping a column (or a partition) loses data. Dropping a
	// constraint/index/default is structural and often part of an idempotent
	// ADD CONSTRAINT workflow, so it should not fail safety checks.
	if strings.HasPrefix(normalized, "ALTER TABLE ") &&
		(strings.Contains(normalized, " DROP COLUMN ") || strings.Contains(normalized, " DROP PARTITION ")) {
		return true, "drops a column/partition and its data"
	}
	if strings.HasPrefix(normalized, "ALTER TYPE ") && strings.Contains(normalized, " DROP ") {
		return true, "alters an enum type destructively"
	}

	if strings.HasPrefix(normalized, "DELETE ") && !hasTopLevelWhere(normalized) {
		return true, "deletes rows without a WHERE clause"
	}
	if strings.HasPrefix(normalized, "UPDATE ") && !hasTopLevelWhere(normalized) {
		return true, "updates every row (no WHERE clause)"
	}

	return false, ""
}

func hasTopLevelWhere(normalized string) bool {
	return strings.Contains(" "+normalized+" ", " WHERE ")
}

func unbalancedParens(stmt string) (code, msg string) {
	depth := 0
	state := sqlStateNormal
	var dollarTag string
	for i := 0; i < len(stmt); i++ {
		ch := stmt[i]
		switch state {
		case sqlStateNormal:
			switch {
			case ch == '\'':
				state = sqlStateSingle
			case ch == '"':
				state = sqlStateDouble
			case ch == '`':
				state = sqlStateBacktick
			case ch == '-' && i+1 < len(stmt) && stmt[i+1] == '-':
				state = sqlStateLineComment
			case ch == '/' && i+1 < len(stmt) && stmt[i+1] == '*':
				state = sqlStateBlockComment
				i++
			case ch == '$':
				if tag, end, ok := readDollarTag(stmt, i); ok {
					state = sqlStateDollar
					dollarTag = tag
					i = end - 1
				}
			case ch == '(':
				depth++
			case ch == ')':
				depth--
				if depth < 0 {
					return "unbalanced_parens", "unexpected ')' in statement"
				}
			}
		case sqlStateSingle:
			if ch == '\\' && i+1 < len(stmt) {
				i++
			} else if ch == '\'' {
				if i+1 < len(stmt) && stmt[i+1] == '\'' {
					i++
				} else {
					state = sqlStateNormal
				}
			}
		case sqlStateDouble:
			if ch == '\\' && i+1 < len(stmt) {
				i++
			} else if ch == '"' {
				if i+1 < len(stmt) && stmt[i+1] == '"' {
					i++
				} else {
					state = sqlStateNormal
				}
			}
		case sqlStateBacktick:
			if ch == '`' {
				state = sqlStateNormal
			}
		case sqlStateLineComment:
			if ch == '\n' {
				state = sqlStateNormal
			}
		case sqlStateBlockComment:
			if ch == '*' && i+1 < len(stmt) && stmt[i+1] == '/' {
				i++
				state = sqlStateNormal
			}
		case sqlStateDollar:
			if strings.HasPrefix(stmt[i:], dollarTag) {
				i += len(dollarTag) - 1
				state = sqlStateNormal
			}
		}
	}
	if depth != 0 {
		return "unbalanced_parens", "unbalanced parentheses in statement"
	}
	return "", ""
}

func lineOfStatement(sql, stmt string) int {
	idx := strings.Index(sql, stmt)
	if idx < 0 {
		return 0
	}
	return strings.Count(sql[:idx], "\n") + 1
}

func shortenStatement(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 160 {
		return s[:157] + "..."
	}
	return s
}
