package migration

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// buildExpectedSchemaFromFiles parses executed migration SQL files and returns
// a map of tableName -> []columnName representing what each table should contain.
func buildExpectedSchemaFromFiles(migrationPath string, executedFiles []string) map[string][]string {
	expected := make(map[string][]string)

	for _, filename := range executedFiles {
		content, err := os.ReadFile(filepath.Join(migrationPath, filename))
		if err != nil {
			continue
		}

		cols := extractColumnsFromSQL(string(content))
		for table, columns := range cols {
			expected[table] = append(expected[table], columns...)
		}
	}

	// Deduplicate columns per table
	for table, cols := range expected {
		expected[table] = uniqueStrings(cols)
	}

	return expected
}

var createTableRe = regexp.MustCompile(`(?is)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?(?:[\w"` + "`" + `]+\.)?([` + "`" + `"\w]+)\s*\(`)
var alterTableAddRe = regexp.MustCompile(`(?i)ALTER\s+TABLE\s+(?:(?:IF\s+EXISTS\s+)?(?:[\w"` + "`" + `]+\.)?([` + "`" + `"\w]+))\s+ADD\s+(?:COLUMN\s+)?(?:IF\s+NOT\s+EXISTS\s+)?([` + "`" + `"\w]+)`)
var alterTableDropRe = regexp.MustCompile(`(?i)ALTER\s+TABLE\s+(?:IF\s+EXISTS\s+)?(?:[\w"` + "`" + `]+\.)?([` + "`" + `"\w]+)\s+DROP\s+(?:COLUMN\s+)?(?:IF\s+EXISTS\s+)?([` + "`" + `"\w]+)`)

// extractColumnsFromSQL parses SQL and returns a map of table -> []columnName.
func extractColumnsFromSQL(sql string) map[string][]string {
	result := make(map[string][]string)

	// Find all CREATE TABLE statements
	allMatches := createTableRe.FindAllStringSubmatchIndex(sql, -1)
	for _, match := range allMatches {
		tableName := strings.ToLower(stripQuotes(sql[match[2]:match[3]]))

		// The opening paren ends at match[1]-1; find its matching close paren
		openPos := strings.LastIndex(sql[:match[1]], "(")
		if openPos == -1 {
			continue
		}
		bodyEnd := findMatchingParen(sql, openPos)
		if bodyEnd == -1 {
			continue
		}

		body := sql[openPos+1 : bodyEnd]
		cols := parseCreateTableColumns(body)
		result[tableName] = append(result[tableName], cols...)
	}

	// Find ALTER TABLE ... ADD COLUMN statements
	for _, m := range alterTableAddRe.FindAllStringSubmatch(sql, -1) {
		tableName := strings.ToLower(stripQuotes(m[1]))
		colName := strings.ToLower(stripQuotes(m[2]))
		if tableName != "" && colName != "" && !isConstraintKeyword(colName) {
			result[tableName] = append(result[tableName], colName)
		}
	}

	// Remove dropped columns
	for _, m := range alterTableDropRe.FindAllStringSubmatch(sql, -1) {
		tableName := strings.ToLower(stripQuotes(m[1]))
		colName := strings.ToLower(stripQuotes(m[2]))
		if tableName != "" && colName != "" {
			cols := result[tableName]
			filtered := cols[:0]
			for _, c := range cols {
				if c != colName {
					filtered = append(filtered, c)
				}
			}
			result[tableName] = filtered
		}
	}

	return result
}

// findMatchingParen finds the closing ) that matches the ( at openPos.
func findMatchingParen(s string, openPos int) int {
	depth := 0
	inString := false
	stringChar := byte(0)

	for i := openPos; i < len(s); i++ {
		ch := s[i]

		if inString {
			switch ch {
			case stringChar:
				inString = false
			case '\\':
				i++ // skip escaped char
			}
			continue
		}

		switch ch {
		case '\'', '"':
			inString = true
			stringChar = ch
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

var columnSkipRe = regexp.MustCompile(`(?i)^\s*(CONSTRAINT|PRIMARY\s+KEY|FOREIGN\s+KEY|UNIQUE\s+KEY|UNIQUE\s+INDEX|INDEX|CHECK|KEY)\b`)

// parseCreateTableColumns splits a CREATE TABLE body by comma (at depth 0)
// and extracts column names, ignoring constraint/key lines.
func parseCreateTableColumns(body string) []string {
	var cols []string
	depth := 0
	start := 0
	inString := false
	stringChar := byte(0)

	for i := 0; i < len(body); i++ {
		ch := body[i]

		if inString {
			switch ch {
			case stringChar:
				inString = false
			case '\\':
				i++
			}
			continue
		}

		switch ch {
		case '\'', '"':
			inString = true
			stringChar = ch
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				part := strings.TrimSpace(body[start:i])
				if col := extractColumnName(part); col != "" {
					cols = append(cols, col)
				}
				start = i + 1
			}
		}
	}

	// Last part
	part := strings.TrimSpace(body[start:])
	if col := extractColumnName(part); col != "" {
		cols = append(cols, col)
	}

	return cols
}

// extractColumnName returns the column name from a single column definition line.
func extractColumnName(part string) string {
	part = strings.TrimSpace(part)
	if part == "" {
		return ""
	}
	// Skip constraint/key lines
	if columnSkipRe.MatchString(part) {
		return ""
	}
	// First token is the column name (may be quoted)
	fields := strings.Fields(part)
	if len(fields) == 0 {
		return ""
	}
	name := stripQuotes(fields[0])
	if name == "" || isConstraintKeyword(name) {
		return ""
	}
	return strings.ToLower(name)
}

// stripQuotes removes surrounding backtick or double-quote from an identifier.
func stripQuotes(s string) string {
	s = strings.Trim(s, "`\"")
	return s
}

var constraintKeywords = map[string]bool{
	"constraint": true, "primary": true, "foreign": true,
	"unique": true, "index": true, "check": true, "key": true,
	"like": true,
}

func isConstraintKeyword(s string) bool {
	return constraintKeywords[strings.ToLower(s)]
}

// uniqueStrings deduplicates a string slice preserving order.
func uniqueStrings(s []string) []string {
	seen := make(map[string]bool)
	out := s[:0]
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
