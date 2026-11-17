package gui

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// EditAnalysis explains whether an edited migration can be applied with drift
// or needs a follow-up migration. This is advisory; vm remains the source of
// truth and its dry-run output is shown in the confirmation plan.
type EditAnalysis struct {
	Executed        *bool          `json:"executed,omitempty"`
	ChecksumOK      *bool          `json:"checksumOk,omitempty"`
	Verdict         string         `json:"verdict"` // no_changes, pending, drift_safe, needs_migration, destructive, unknown
	Summary         string         `json:"summary"`
	AddedTables     []string       `json:"addedTables,omitempty"`
	RemovedTables   []string       `json:"removedTables,omitempty"`
	AddedColumns    []ColumnChange `json:"addedColumns,omitempty"`
	RemovedColumns  []ColumnChange `json:"removedColumns,omitempty"`
	ModifiedColumns []ColumnChange `json:"modifiedColumns,omitempty"`
	AddedIndexes    []string       `json:"addedIndexes,omitempty"`
	RemovedIndexes  []string       `json:"removedIndexes,omitempty"`
	Warnings        []string       `json:"warnings,omitempty"`
	FollowUps       []FollowUp     `json:"followUps,omitempty"`
}

// ColumnChange is one column-level difference between two versions of a file.
type ColumnChange struct {
	Table      string `json:"table"`
	Column     string `json:"column"`
	Type       string `json:"type,omitempty"`
	Definition string `json:"definition,omitempty"`
	DriftSafe  bool   `json:"driftSafe"`
	Reason     string `json:"reason,omitempty"`
}

// FollowUp suggests a migration name when drift cannot apply a change.
type FollowUp struct {
	Table  string `json:"table,omitempty"`
	Column string `json:"column,omitempty"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type parsedColumn struct {
	Name       string
	Definition string
	Type       string
	Nullable   bool
	HasDefault bool
	Unique     bool
	Primary    bool
	Default    string
	References string
}

type parsedTable struct {
	Name    string
	Columns map[string]parsedColumn
}

type parsedSchema struct {
	Tables  map[string]*parsedTable
	Indexes map[string]string
}

func (s *parsedSchema) table(name string) *parsedTable {
	name = strings.ToLower(strings.Trim(name, "`\"[]"))
	if s.Tables == nil {
		s.Tables = map[string]*parsedTable{}
	}
	if t, ok := s.Tables[name]; ok {
		return t
	}
	t := &parsedTable{Name: name, Columns: map[string]parsedColumn{}}
	s.Tables[name] = t
	return t
}

var (
	createTableEditRe = regexp.MustCompile(`(?is)\bCREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?(?:(?:[\w"` + "`" + `]+)\.)?([\w"` + "`" + `]+)\s*\(`)
	dropTableEditRe   = regexp.MustCompile(`(?is)\bDROP\s+TABLE\s+(?:IF\s+EXISTS\s+)?(?:(?:[\w"` + "`" + `]+)\.)?([\w"` + "`" + `]+)`)
	createIndexEditRe = regexp.MustCompile(`(?is)\bCREATE\s+(?:UNIQUE\s+)?INDEX\s+(?:IF\s+NOT\s+EXISTS\s+)?([\w"` + "`" + `]+)`)
	alterAddEditRe    = regexp.MustCompile(`(?is)\bALTER\s+TABLE\s+(?:IF\s+EXISTS\s+)?(?:(?:[\w"` + "`" + `]+)\.)?([\w"` + "`" + `]+)\s+ADD\s+(?:COLUMN\s+)?(?:IF\s+NOT\s+EXISTS\s+)?([\w"` + "`" + `]+)\s+([^;]+)`)
	alterDropEditRe   = regexp.MustCompile(`(?is)\bALTER\s+TABLE\s+(?:IF\s+EXISTS\s+)?(?:(?:[\w"` + "`" + `]+)\.)?([\w"` + "`" + `]+)\s+DROP\s+(?:COLUMN\s+)?(?:IF\s+EXISTS\s+)?([\w"` + "`" + `]+)`)
)

// AnalyzeEdit compares two versions of a migration file. executed reports
// whether the file is already in the database (nil when unknown).
func AnalyzeEdit(oldContent, newContent string, executed *bool, checksumOK *bool) EditAnalysis {
	oldSchema := parseSchemaSQL(oldContent)
	newSchema := parseSchemaSQL(newContent)
	analysis := EditAnalysis{Executed: executed, ChecksumOK: checksumOK}

	for name := range newSchema.Tables {
		if _, ok := oldSchema.Tables[name]; !ok {
			analysis.AddedTables = append(analysis.AddedTables, name)
		}
	}
	for name := range oldSchema.Tables {
		if _, ok := newSchema.Tables[name]; !ok {
			analysis.RemovedTables = append(analysis.RemovedTables, name)
		}
	}
	sort.Strings(analysis.AddedTables)
	sort.Strings(analysis.RemovedTables)

	tableNames := make([]string, 0, len(newSchema.Tables))
	for name := range newSchema.Tables {
		tableNames = append(tableNames, name)
	}
	sort.Strings(tableNames)

	for _, tableName := range tableNames {
		oldTable, oldOK := oldSchema.Tables[tableName]
		newTable, newOK := newSchema.Tables[tableName]
		if !oldOK || !newOK {
			continue
		}
		colNames := make([]string, 0, len(newTable.Columns))
		for name := range newTable.Columns {
			colNames = append(colNames, name)
		}
		sort.Strings(colNames)
		for _, colName := range colNames {
			newCol := newTable.Columns[colName]
			oldCol, exists := oldTable.Columns[colName]
			if !exists {
				change := ColumnChange{
					Table:      tableName,
					Column:     colName,
					Type:       newCol.Type,
					Definition: newCol.Definition,
				}
				change.DriftSafe, change.Reason = classifyAddedColumn(newCol)
				analysis.AddedColumns = append(analysis.AddedColumns, change)
				if !change.DriftSafe {
					analysis.FollowUps = append(analysis.FollowUps, FollowUp{
						Table:  tableName,
						Column: colName,
						Name:   "add_" + colName + "_to_" + tableName,
						Reason: change.Reason,
					})
				}
				continue
			}
			if reason := columnChanged(oldCol, newCol); reason != "" {
				analysis.ModifiedColumns = append(analysis.ModifiedColumns, ColumnChange{
					Table:      tableName,
					Column:     colName,
					Type:       newCol.Type,
					Definition: newCol.Definition,
					Reason:     reason,
				})
				analysis.FollowUps = append(analysis.FollowUps, FollowUp{
					Table:  tableName,
					Column: colName,
					Name:   "alter_" + colName + "_on_" + tableName,
					Reason: reason,
				})
			}
		}
		for colName, oldCol := range oldTable.Columns {
			if _, exists := newTable.Columns[colName]; exists {
				continue
			}
			analysis.RemovedColumns = append(analysis.RemovedColumns, ColumnChange{
				Table:      tableName,
				Column:     colName,
				Type:       oldCol.Type,
				Definition: oldCol.Definition,
				Reason:     "column was removed from the migration; drift will not drop columns automatically",
			})
			analysis.FollowUps = append(analysis.FollowUps, FollowUp{
				Table:  tableName,
				Column: colName,
				Name:   "alter_drop_" + colName + "_from_" + tableName,
				Reason: "destructive column drop needs an explicit forward migration",
			})
		}
	}

	for name := range newSchema.Indexes {
		if _, ok := oldSchema.Indexes[name]; !ok {
			analysis.AddedIndexes = append(analysis.AddedIndexes, name)
		}
	}
	for name := range oldSchema.Indexes {
		if _, ok := newSchema.Indexes[name]; !ok {
			analysis.RemovedIndexes = append(analysis.RemovedIndexes, name)
			analysis.Warnings = append(analysis.Warnings, fmt.Sprintf("index %s was removed from the file; drift does not drop indexes automatically", name))
		}
	}
	sort.Strings(analysis.AddedIndexes)
	sort.Strings(analysis.RemovedIndexes)
	sort.Slice(analysis.AddedColumns, func(i, j int) bool {
		if analysis.AddedColumns[i].Table == analysis.AddedColumns[j].Table {
			return analysis.AddedColumns[i].Column < analysis.AddedColumns[j].Column
		}
		return analysis.AddedColumns[i].Table < analysis.AddedColumns[j].Table
	})
	sort.Slice(analysis.RemovedColumns, func(i, j int) bool {
		if analysis.RemovedColumns[i].Table == analysis.RemovedColumns[j].Table {
			return analysis.RemovedColumns[i].Column < analysis.RemovedColumns[j].Column
		}
		return analysis.RemovedColumns[i].Table < analysis.RemovedColumns[j].Table
	})
	sort.Slice(analysis.ModifiedColumns, func(i, j int) bool {
		if analysis.ModifiedColumns[i].Table == analysis.ModifiedColumns[j].Table {
			return analysis.ModifiedColumns[i].Column < analysis.ModifiedColumns[j].Column
		}
		return analysis.ModifiedColumns[i].Table < analysis.ModifiedColumns[j].Table
	})

	analysis.Verdict, analysis.Summary = summarizeAnalysis(analysis, executed)
	return analysis
}

func classifyAddedColumn(col parsedColumn) (bool, string) {
	if col.References != "" && !col.Nullable {
		return false, "NOT NULL foreign-key column: drift cannot add it to a populated table without a value"
	}
	if !col.Nullable && !col.HasDefault {
		return false, "NOT NULL without a DEFAULT: drift cannot add it to a populated table"
	}
	if col.Unique && !col.Nullable {
		return false, "NOT NULL UNIQUE: drift cannot add it without backfilling unique values"
	}
	if col.HasDefault {
		return true, "has a DEFAULT; drift can add it non-destructively"
	}
	return true, "nullable; drift can add it non-destructively"
}

func columnChanged(oldCol, newCol parsedColumn) string {
	var reasons []string
	if !strings.EqualFold(strings.TrimSpace(oldCol.Type), strings.TrimSpace(newCol.Type)) {
		reasons = append(reasons, fmt.Sprintf("type changed from %s to %s", oldCol.Type, newCol.Type))
	}
	if oldCol.Nullable != newCol.Nullable {
		if oldCol.Nullable {
			reasons = append(reasons, "nullability changed to NOT NULL")
		} else {
			reasons = append(reasons, "nullability changed to NULL")
		}
	}
	if !strings.EqualFold(strings.TrimSpace(oldCol.Default), strings.TrimSpace(newCol.Default)) {
		reasons = append(reasons, "DEFAULT changed")
	}
	if oldCol.Primary != newCol.Primary {
		reasons = append(reasons, "PRIMARY KEY changed")
	}
	if oldCol.Unique != newCol.Unique {
		reasons = append(reasons, "UNIQUE constraint changed")
	}
	if !strings.EqualFold(oldCol.References, newCol.References) {
		reasons = append(reasons, "foreign-key reference changed")
	}
	return strings.Join(reasons, "; ")
}

func summarizeAnalysis(a EditAnalysis, executed *bool) (string, string) {
	hasChanges := len(a.AddedTables)+len(a.RemovedTables)+len(a.AddedColumns)+len(a.RemovedColumns)+len(a.ModifiedColumns)+len(a.AddedIndexes)+len(a.RemovedIndexes) > 0
	if !hasChanges {
		return "no_changes", "No schema changes were detected."
	}
	if executed == nil {
		return "unknown", "Schema changes detected, but database status is unknown. Select a working connection to decide how to apply them."
	}
	if !*executed {
		return "pending", "This migration has not run yet. The edits will be applied by the next migrate."
	}
	if len(a.RemovedTables) > 0 || len(a.RemovedColumns) > 0 {
		return "destructive", "The edit removes schema. Drift will not drop tables or columns automatically; create an explicit forward migration."
	}
	if len(a.ModifiedColumns) > 0 || len(a.RemovedIndexes) > 0 {
		return "needs_migration", "This migration already ran. Drift cannot alter existing columns or drop indexes; create a follow-up migration."
	}
	if len(a.AddedTables) > 0 {
		return "needs_migration", "This migration already ran. Drift only repairs existing tables, so a new table needs its own migration."
	}
	for _, col := range a.AddedColumns {
		if !col.DriftSafe {
			return "needs_migration", "This migration already ran. Drift cannot safely add one or more columns; create a follow-up migration."
		}
	}
	if len(a.AddedColumns) > 0 || len(a.AddedIndexes) > 0 {
		return "drift_safe", "This migration already ran. Drift can apply the additive changes to the live database."
	}
	return "no_changes", "No schema changes were detected."
}

func parseSchemaSQL(content string) *parsedSchema {
	sql := stripSQLComments(extractUpSection(content))
	schema := &parsedSchema{Tables: map[string]*parsedTable{}, Indexes: map[string]string{}}

	for _, match := range createTableEditRe.FindAllStringSubmatchIndex(sql, -1) {
		tableName := unquoteIdent(sql[match[2]:match[3]])
		open := sql[match[1]-1]
		if open != '(' {
			continue
		}
		bodyEnd := findMatchingParen(sql, match[1]-1)
		if bodyEnd < 0 {
			continue
		}
		body := sql[match[1]:bodyEnd]
		table := schema.table(tableName)
		for _, item := range splitTopLevel(body, ',') {
			col, ok := parseColumnDefinition(item)
			if !ok {
				continue
			}
			table.Columns[strings.ToLower(col.Name)] = col
		}
	}
	for _, match := range alterAddEditRe.FindAllStringSubmatch(sql, -1) {
		tableName := unquoteIdent(match[1])
		col, ok := parseColumnDefinition(match[2] + " " + match[3])
		if !ok {
			continue
		}
		schema.table(tableName).Columns[strings.ToLower(col.Name)] = col
	}
	for _, match := range alterDropEditRe.FindAllStringSubmatch(sql, -1) {
		tableName := unquoteIdent(match[1])
		colName := strings.ToLower(unquoteIdent(match[2]))
		if t, ok := schema.Tables[tableName]; ok {
			delete(t.Columns, colName)
		}
	}
	for _, match := range dropTableEditRe.FindAllStringSubmatch(sql, -1) {
		delete(schema.Tables, unquoteIdent(match[1]))
	}
	for _, match := range createIndexEditRe.FindAllStringSubmatch(sql, -1) {
		name := unquoteIdent(match[1])
		schema.Indexes[name] = match[0]
	}
	return schema
}

var columnConstraintStart = map[string]bool{
	"NOT": true, "NULL": true, "DEFAULT": true, "UNIQUE": true, "PRIMARY": true,
	"REFERENCES": true, "CHECK": true, "CONSTRAINT": true, "GENERATED": true,
	"COLLATE": true, "ON": true, "DEFERRABLE": true, "INITIALLY": true,
}

func parseColumnDefinition(raw string) (parsedColumn, bool) {
	raw = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(raw), ","))
	if raw == "" {
		return parsedColumn{}, false
	}
	upper := strings.ToUpper(raw)
	for _, prefix := range []string{"CONSTRAINT ", "PRIMARY KEY", "FOREIGN KEY", "UNIQUE ", "CHECK ", "KEY ", "INDEX "} {
		if strings.HasPrefix(upper, prefix) {
			return parsedColumn{}, false
		}
	}
	name, rest, ok := readIdentifier(raw)
	if !ok {
		return parsedColumn{}, false
	}
	col := parsedColumn{Name: name, Definition: strings.TrimSpace(rest)}
	col.Type = extractColumnType(col.Definition)
	col.Nullable = !regexp.MustCompile(`(?i)\bNOT\s+NULL\b`).MatchString(col.Definition)
	col.Unique = regexp.MustCompile(`(?i)\bUNIQUE\b`).MatchString(col.Definition)
	col.Primary = regexp.MustCompile(`(?i)\bPRIMARY\s+KEY\b`).MatchString(col.Definition)
	col.Default = extractDefaultValue(col.Definition)
	col.HasDefault = col.Default != ""
	if m := regexp.MustCompile(`(?is)\bREFERENCES\s+([\w"` + "`" + `]+)\s*\(([^)]*)\)`).FindStringSubmatch(col.Definition); m != nil {
		col.References = unquoteIdent(m[1]) + "(" + strings.TrimSpace(m[2]) + ")"
	}
	return col, true
}

var defaultEditRe = regexp.MustCompile(`(?is)\bDEFAULT\s+(.+?)(?:\s+(?:NOT|NULL|UNIQUE|PRIMARY|REFERENCES|CHECK|CONSTRAINT|GENERATED|COLLATE|ON|DEFERRABLE|INITIALLY)\b|$)`)

func extractDefaultValue(def string) string {
	match := defaultEditRe.FindStringSubmatch(def)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func extractColumnType(def string) string {
	var tokens []string
	for _, tok := range splitTopLevelWords(def) {
		if columnConstraintStart[strings.ToUpper(tok)] {
			break
		}
		tokens = append(tokens, tok)
	}
	return strings.Join(tokens, " ")
}

func readIdentifier(s string) (name, rest string, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", false
	}
	switch s[0] {
	case '"', '`', '[':
		closeCh := s[0]
		if closeCh == '[' {
			closeCh = ']'
		}
		for i := 1; i < len(s); i++ {
			if s[i] == closeCh {
				return s[1:i], s[i+1:], true
			}
		}
		return "", "", false
	default:
		for i := 0; i < len(s); i++ {
			ch := s[i]
			if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '(' || ch == ',' {
				return s[:i], s[i:], true
			}
		}
		return s, "", true
	}
}

func unquoteIdent(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '`' && s[len(s)-1] == '`') || (s[0] == '[' && s[len(s)-1] == ']') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func splitTopLevelWords(s string) []string {
	var out []string
	var current strings.Builder
	depth := 0
	inString := false
	stringChar := byte(0)
	flush := func() {
		if current.Len() > 0 {
			out = append(out, current.String())
			current.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inString {
			current.WriteByte(ch)
			if ch == '\\' && i+1 < len(s) {
				i++
				current.WriteByte(s[i])
			} else if ch == stringChar {
				inString = false
			}
			continue
		}
		switch ch {
		case '\'', '"':
			inString = true
			stringChar = ch
			current.WriteByte(ch)
		case '(':
			depth++
			current.WriteByte(ch)
		case ')':
			if depth > 0 {
				depth--
			}
			current.WriteByte(ch)
		case ' ', '\t', '\n', '\r':
			if depth == 0 {
				flush()
			} else {
				current.WriteByte(ch)
			}
		default:
			current.WriteByte(ch)
		}
	}
	flush()
	return out
}

func splitTopLevel(s string, sep byte) []string {
	var out []string
	start := 0
	depth := 0
	inString := false
	stringChar := byte(0)
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inString {
			if ch == '\\' && i+1 < len(s) {
				i++
			} else if ch == stringChar {
				inString = false
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
			if depth > 0 {
				depth--
			}
		default:
			if ch == sep && depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	if start < len(s) {
		out = append(out, strings.TrimSpace(s[start:]))
	}
	return out
}

func findMatchingParen(s string, open int) int {
	depth := 0
	inString := false
	stringChar := byte(0)
	for i := open; i < len(s); i++ {
		ch := s[i]
		if inString {
			if ch == '\\' && i+1 < len(s) {
				i++
			} else if ch == stringChar {
				inString = false
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

func extractUpSection(content string) string {
	lines := strings.Split(content, "\n")
	var out []string
	inUp := false
	foundMarker := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		isUp := strings.Contains(lower, "up (run when migrating forward)") || lower == "-- +goose up" || lower == "-- migrate:up" || strings.HasPrefix(lower, "-- ⬆ up")
		isDown := strings.Contains(lower, "down (run when rolling back)") || lower == "-- +goose down" || lower == "-- migrate:down" || strings.HasPrefix(lower, "-- ⬇ down")
		if isUp {
			inUp = true
			foundMarker = true
			continue
		}
		if inUp && isDown {
			break
		}
		if inUp {
			out = append(out, line)
		}
	}
	if !foundMarker {
		return content
	}
	return strings.Join(out, "\n")
}

func stripSQLComments(sql string) string {
	var b strings.Builder
	inString := false
	stringChar := byte(0)
	for i := 0; i < len(sql); i++ {
		ch := sql[i]
		if inString {
			b.WriteByte(ch)
			if ch == '\\' && i+1 < len(sql) {
				i++
				b.WriteByte(sql[i])
			} else if ch == stringChar {
				inString = false
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			inString = true
			stringChar = ch
			b.WriteByte(ch)
			continue
		}
		if ch == '-' && i+1 < len(sql) && sql[i+1] == '-' {
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			if i < len(sql) {
				b.WriteByte('\n')
			}
			continue
		}
		if ch == '/' && i+1 < len(sql) && sql[i+1] == '*' {
			i += 2
			for i < len(sql) {
				if sql[i] == '*' && i+1 < len(sql) && sql[i+1] == '/' {
					i++
					break
				}
				if sql[i] == '\n' {
					b.WriteByte('\n')
				}
				i++
			}
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}
