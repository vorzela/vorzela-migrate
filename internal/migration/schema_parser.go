package migration

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// buildExpectedSchemaFromFiles parses executed migration SQL files and returns
// a map of tableName -> []columnName representing what each table should contain.
//
// Only the Up section of each migration file is parsed.  Reading the full file
// (including the Down section) would cause ADD COLUMN statements in Up and the
// corresponding DROP COLUMN statements in Down to cancel each other out, making
// those columns disappear from the expected schema and triggering false-positive
// drift warnings.
func buildExpectedSchemaFromFiles(migrationPath string, executedFiles []string) map[string][]string {
	expected := make(map[string][]string)

	for _, filename := range executedFiles {
		content, err := os.ReadFile(filepath.Join(migrationPath, filename))
		if err != nil {
			continue
		}

		// Parse only the Up section so that Down-section DROP/ALTER statements
		// do not cancel out columns that were added in the Up section.
		sqlContent := extractSection(string(content), "Up")
		if sqlContent == "" {
			// No section markers found – treat the whole file as Up SQL.
			sqlContent = string(content)
		}

		cols := extractColumnsFromSQL(sqlContent)
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

// -----------------------------------------------------------------------
// Index + trigger extraction from migration SQL
// -----------------------------------------------------------------------

// createIndexRe matches: CREATE [UNIQUE] INDEX [IF NOT EXISTS] name ON table (cols)
var createIndexRe = regexp.MustCompile(
	`(?is)CREATE\s+(UNIQUE\s+)?INDEX\s+(?:IF\s+NOT\s+EXISTS\s+)?([` + "`" + `"\w]+)\s+ON\s+([` + "`" + `"\w]+)\s*\(([^)]+)\)`)

// createTriggerRe matches: CREATE [OR REPLACE] TRIGGER name {BEFORE|AFTER} event ON table
var createTriggerRe = regexp.MustCompile(
	`(?is)CREATE\s+(?:OR\s+REPLACE\s+)?TRIGGER\s+([` + "`" + `"\w]+)\s+(BEFORE|AFTER|INSTEAD\s+OF)\s+(INSERT|UPDATE|DELETE|TRUNCATE)\s+ON\s+([` + "`" + `"\w]+)`)

// buildExpectedIndexesFromFiles parses executed migration files and returns
// a map of tableName -> []IndexInfo listing indexes that should exist.
// Only the Up section of each file is parsed to avoid false negatives from
// DROP INDEX statements in the Down section.
func buildExpectedIndexesFromFiles(migrationPath string, executedFiles []string) map[string][]IndexInfo {
	expected := make(map[string][]IndexInfo)
	seen := make(map[string]bool) // deduplicate by index name

	for _, filename := range executedFiles {
		content, err := os.ReadFile(filepath.Join(migrationPath, filename))
		if err != nil {
			continue
		}
		sqlContent := extractSection(string(content), "Up")
		if sqlContent == "" {
			sqlContent = string(content)
		}
		for _, idx := range extractIndexesFromSQL(sqlContent) {
			key := strings.ToLower(idx.TableName) + "." + strings.ToLower(idx.Name)
			if seen[key] {
				continue
			}
			seen[key] = true
			tbl := strings.ToLower(idx.TableName)
			expected[tbl] = append(expected[tbl], idx)
		}
	}
	return expected
}

// buildExpectedTriggersFromFiles parses executed migration files and returns
// a map of tableName -> []TriggerInfo listing triggers that should exist.
// Only the Up section is parsed so that DOWN-section DROP TRIGGER statements
// do not shadow triggers that should be present.
func buildExpectedTriggersFromFiles(migrationPath string, executedFiles []string) map[string][]TriggerInfo {
	expected := make(map[string][]TriggerInfo)
	seen := make(map[string]bool)

	for _, filename := range executedFiles {
		content, err := os.ReadFile(filepath.Join(migrationPath, filename))
		if err != nil {
			continue
		}
		sqlContent := extractSection(string(content), "Up")
		if sqlContent == "" {
			sqlContent = string(content)
		}
		for _, trig := range extractTriggersFromSQL(sqlContent) {
			key := strings.ToLower(trig.TableName) + "." + strings.ToLower(trig.Name)
			if seen[key] {
				continue
			}
			seen[key] = true
			tbl := strings.ToLower(trig.TableName)
			expected[tbl] = append(expected[tbl], trig)
		}
	}
	return expected
}

// extractIndexesFromSQL parses CREATE INDEX statements from a SQL string.
func extractIndexesFromSQL(sql string) []IndexInfo {
	var indexes []IndexInfo
	for _, m := range createIndexRe.FindAllStringSubmatch(sql, -1) {
		isUnique := strings.TrimSpace(m[1]) != ""
		name := stripQuotes(m[2])
		table := stripQuotes(m[3])
		rawCols := m[4]

		var cols []string
		for _, c := range strings.Split(rawCols, ",") {
			c = strings.TrimSpace(c)
			// Strip any index-direction keyword or parenthesised length — keep identifier only
			c = strings.Fields(c)[0]
			c = stripQuotes(c)
			if c != "" {
				cols = append(cols, strings.ToLower(c))
			}
		}

		indexes = append(indexes, IndexInfo{
			Name:      strings.ToLower(name),
			TableName: strings.ToLower(table),
			Columns:   cols,
			IsUnique:  isUnique,
		})
	}
	return indexes
}

// extractTriggersFromSQL parses CREATE TRIGGER statements from a SQL string.
func extractTriggersFromSQL(sql string) []TriggerInfo {
	var triggers []TriggerInfo
	for _, m := range createTriggerRe.FindAllStringSubmatch(sql, -1) {
		name := stripQuotes(m[1])
		timing := strings.ToUpper(strings.TrimSpace(m[2]))
		event := strings.ToUpper(strings.TrimSpace(m[3]))
		table := stripQuotes(m[4])

		triggers = append(triggers, TriggerInfo{
			Name:      strings.ToLower(name),
			TableName: strings.ToLower(table),
			Event:     event,
			Timing:    timing,
		})
	}
	return triggers
}
