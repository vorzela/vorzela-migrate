package gui

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/vorzela/vorzela-migrate/v2/internal/migration"
)

const (
	columnModeDrift    = "drift"
	columnModePending  = "pending"
	columnModeFollowUp = "followup"
)

type columnAddPayload struct {
	Table            string `json:"table"`
	Name             string `json:"name"`
	Type             string `json:"type"`
	Nullable         bool   `json:"nullable"`
	Default          string `json:"default,omitempty"`
	Unique           bool   `json:"unique"`
	ReferencesTable  string `json:"referencesTable,omitempty"`
	ReferencesColumn string `json:"referencesColumn,omitempty"`
	OnDelete         string `json:"onDelete,omitempty"`
	Backfill         string `json:"backfill,omitempty"`

	// Server-populated fields used by the confirmation plan.
	Mode         string `json:"mode,omitempty"`
	BaseHash     string `json:"baseHash,omitempty"`
	CreateFile   string `json:"createFile,omitempty"`
	FollowUpName string `json:"followUpName,omitempty"`
}

var (
	sqlIdentifierRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,62}$`)
	sqlColumnRe     = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
	sqlTypeRe       = regexp.MustCompile(`(?i)^[a-z][a-z0-9_ ]{0,60}(\(\s*\d{1,9}(\s*,\s*\d{1,9})?\s*\))?(\s+UNSIGNED)?(\s+ZEROFILL)?(\[\])?$`)
	enumTypeRe      = regexp.MustCompile(`(?is)^(ENUM|SET)\s*\(\s*'(?:[^']|'')*'(?:\s*,\s*'(?:[^']|'')*')*\s*\)$`)
	numericValueRe  = regexp.MustCompile(`^-?\d+(\.\d+)?$`)
	quotedValueRe   = regexp.MustCompile(`^'(?:[^']|'')*'$`)
	castValueRe     = regexp.MustCompile(`(?is)^'(?:[^']|'')*'::[A-Za-z_][A-Za-z0-9_ ]*$`)
	functionValueRe = regexp.MustCompile(`(?i)^(CURRENT_TIMESTAMP|CURRENT_DATE|CURRENT_TIME|NOW\(\)|gen_random_uuid\(\)|uuid_generate_v4\(\))$`)
	boolValueRe     = regexp.MustCompile(`(?i)^(TRUE|FALSE|NULL)$`)
	backfillIdentRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	backfillFnRe    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\(\)$`)
)

func validateColumnPayload(payload *columnAddPayload) error {
	payload.Table = strings.TrimSpace(payload.Table)
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Type = collapseSpaces(strings.TrimSpace(payload.Type))
	payload.Default = strings.TrimSpace(payload.Default)
	payload.Backfill = strings.TrimSpace(payload.Backfill)
	payload.ReferencesTable = strings.TrimSpace(payload.ReferencesTable)
	payload.ReferencesColumn = strings.TrimSpace(payload.ReferencesColumn)
	payload.OnDelete = strings.ToUpper(strings.TrimSpace(payload.OnDelete))

	if !sqlIdentifierRe.MatchString(payload.Table) {
		return errors.New("table name contains unsupported characters")
	}
	if !sqlColumnRe.MatchString(payload.Name) {
		return errors.New("column name must be lowercase snake_case (letters, digits, underscores)")
	}
	if !validSQLType(payload.Type) {
		return errors.New("unsupported column type; use a simple SQL type such as VARCHAR(255), TEXT, BIGINT, BOOLEAN, TIMESTAMPTZ or NUMERIC(10,2)")
	}
	if payload.Default != "" {
		if err := validateSQLValue(payload.Default, false); err != nil {
			return fmt.Errorf("default value: %w", err)
		}
	}
	if payload.Backfill != "" {
		if err := validateSQLValue(payload.Backfill, true); err != nil {
			return fmt.Errorf("backfill value: %w", err)
		}
	}
	if (payload.ReferencesTable == "") != (payload.ReferencesColumn == "") {
		return errors.New("foreign key requires both a referenced table and column")
	}
	if payload.ReferencesTable != "" {
		if !sqlIdentifierRe.MatchString(payload.ReferencesTable) || !sqlIdentifierRe.MatchString(payload.ReferencesColumn) {
			return errors.New("referenced table or column contains unsupported characters")
		}
	}
	switch payload.OnDelete {
	case "", "NO ACTION", "RESTRICT", "CASCADE", "SET NULL", "SET DEFAULT":
	default:
		return errors.New("on delete must be NO ACTION, RESTRICT, CASCADE, SET NULL or SET DEFAULT")
	}
	if payload.OnDelete == "SET NULL" && !payload.Nullable {
		return errors.New("ON DELETE SET NULL cannot be combined with NOT NULL")
	}
	return nil
}

func collapseSpaces(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func validSQLType(value string) bool {
	if value == "" || len(value) > 80 {
		return false
	}
	if strings.ContainsAny(value, ";\x00") || strings.Contains(value, "--") || strings.Contains(value, "/*") {
		return false
	}
	if enumTypeRe.MatchString(value) {
		return true
	}
	upper := " " + strings.ToUpper(collapseSpaces(value)) + " "
	for _, keyword := range []string{" NOT NULL ", " NULL ", " DEFAULT ", " UNIQUE ", " REFERENCES ", " PRIMARY ", " CHECK ", " CONSTRAINT "} {
		if strings.Contains(upper, keyword) {
			return false
		}
	}
	return sqlTypeRe.MatchString(value)
}

// validateSQLValue accepts a deliberately small set of literals/functions so a
// form value cannot inject additional SQL into a generated migration.
func validateSQLValue(value string, allowIdentifier bool) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if boolValueRe.MatchString(value) || numericValueRe.MatchString(value) ||
		quotedValueRe.MatchString(value) || castValueRe.MatchString(value) ||
		functionValueRe.MatchString(value) {
		return nil
	}
	if allowIdentifier && (backfillIdentRe.MatchString(value) || backfillFnRe.MatchString(value)) {
		return nil
	}
	return errors.New("use a literal (number, string, TRUE/FALSE/NULL) or a supported function; edit the migration for complex expressions")
}

func columnAddDriftSafe(payload columnAddPayload) bool {
	if !payload.Nullable && payload.Default == "" {
		return false
	}
	if payload.Unique && !payload.Nullable {
		return false
	}
	if payload.Unique && payload.Default != "" {
		// A DEFAULT is written to every existing row, so a UNIQUE constraint
		// would fail unless the values happen to differ.
		return false
	}
	if payload.ReferencesTable != "" && !payload.Nullable {
		return false
	}
	return true
}

func quoteIdent(name string, dialect migration.Dialect) string {
	if dialect == migration.PostgreSQL {
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	}
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

func columnDefinitionSQL(payload columnAddPayload, dialect migration.Dialect) string {
	def := quoteIdent(payload.Name, dialect) + " " + payload.Type
	if !payload.Nullable {
		def += " NOT NULL"
	}
	if payload.Default != "" {
		def += " DEFAULT " + payload.Default
	}
	if payload.Unique {
		def += " UNIQUE"
	}
	if payload.ReferencesTable != "" {
		def += " REFERENCES " + quoteIdent(payload.ReferencesTable, dialect) +
			" (" + quoteIdent(payload.ReferencesColumn, dialect) + ")"
		if payload.OnDelete != "" {
			def += " ON DELETE " + payload.OnDelete
		}
	}
	return def
}

// insertColumnIntoCreate adds a column definition to the matching CREATE TABLE.
func insertColumnIntoCreate(content string, payload columnAddPayload, dialect migration.Dialect) (string, error) {
	matches := createTableEditRe.FindAllStringSubmatchIndex(content, -1)
	for _, match := range matches {
		table := strings.ToLower(unquoteIdent(content[match[2]:match[3]]))
		if table != strings.ToLower(payload.Table) {
			continue
		}
		open := match[1] - 1
		if open < 0 || content[open] != '(' {
			continue
		}
		end := findMatchingParen(content, open)
		if end < 0 {
			return "", errors.New("could not find the end of the CREATE TABLE statement")
		}
		body := content[open+1 : end]
		if hasColumnDefinition(body, payload.Name) {
			return "", fmt.Errorf("column %s is already defined in %s", payload.Name, payload.Table)
		}
		colDef := columnDefinitionSQL(payload, dialect)
		constraintAt := firstTableConstraintOffset(content, open+1, end)
		if constraintAt >= 0 {
			insertAt := constraintAt
			for insertAt < end && isSpaceByte(content[insertAt]) {
				insertAt++
			}
			return content[:insertAt] + colDef + ",\n    " + content[insertAt:], nil
		}
		last := end - 1
		for last > open && isSpaceByte(content[last]) {
			last--
		}
		insertAt := last + 1
		separator := ",\n    "
		if last == open || content[last] == ',' {
			separator = "\n    "
		}
		if strings.TrimSpace(body) == "" {
			separator = "\n    "
		}
		return content[:insertAt] + separator + colDef + content[insertAt:], nil
	}
	return "", fmt.Errorf("no CREATE TABLE migration found for table %s", payload.Table)
}

func isSpaceByte(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

func hasColumnDefinition(body, column string) bool {
	for _, item := range splitTopLevel(body, ',') {
		col, ok := parseColumnDefinition(item)
		if ok && strings.EqualFold(col.Name, column) {
			return true
		}
	}
	return false
}

func firstTableConstraintOffset(sql string, start, end int) int {
	depth := 0
	inString := false
	stringChar := byte(0)
	itemStart := start
	for i := start; i < end; i++ {
		ch := sql[i]
		if inString {
			if ch == '\\' && i+1 < end {
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
			if ch == ',' && depth == 0 {
				if isTableConstraint(strings.TrimSpace(sql[itemStart:i])) {
					return itemStart
				}
				itemStart = i + 1
			}
		}
	}
	if isTableConstraint(strings.TrimSpace(sql[itemStart:end])) {
		return itemStart
	}
	return -1
}

func isTableConstraint(item string) bool {
	upper := strings.ToUpper(strings.TrimSpace(item))
	for _, prefix := range []string{"CONSTRAINT ", "PRIMARY KEY", "FOREIGN KEY", "UNIQUE ", "CHECK ", "KEY ", "INDEX "} {
		if strings.HasPrefix(upper, prefix) {
			return true
		}
	}
	return false
}

func safeConstraintName(prefix, table, column string) string {
	name := prefix + table + "_" + column
	if len(name) > 60 {
		name = name[:60]
	}
	return name
}

// followUpMigrationSQL builds the complete add_* template for the cases where
// drift cannot safely alter the live table.
func followUpMigrationSQL(payload columnAddPayload, dialect migration.Dialect) string {
	table := quoteIdent(payload.Table, dialect)
	column := quoteIdent(payload.Name, dialect)
	var up []string

	add := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, payload.Type)
	if dialect == migration.PostgreSQL {
		add = fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s %s", table, column, payload.Type)
	}
	if payload.Default != "" {
		add += " DEFAULT " + payload.Default
	}
	add += ";"
	up = append(up, add)

	backfill := payload.Backfill
	if backfill == "" {
		backfill = payload.Default
	}
	if !payload.Nullable {
		if backfill != "" {
			up = append(up, fmt.Sprintf("UPDATE %s SET %s = %s WHERE %s IS NULL;", table, column, backfill, column))
		} else {
			up = append(up, fmt.Sprintf("-- TODO: backfill %s before enforcing NOT NULL (existing rows contain NULL).", payload.Name))
		}
		if dialect == migration.PostgreSQL {
			up = append(up, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL;", table, column))
		} else {
			modify := fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s %s", table, column, payload.Type)
			if payload.Default != "" {
				modify += " DEFAULT " + payload.Default
			}
			modify += " NOT NULL;"
			up = append(up, modify)
		}
	}
	if payload.Unique {
		name := safeConstraintName("uq_", payload.Table, payload.Name)
		if dialect == migration.PostgreSQL {
			up = append(up, fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s UNIQUE (%s);", table, quoteIdent(name, dialect), column))
		} else {
			up = append(up, fmt.Sprintf("ALTER TABLE %s ADD UNIQUE KEY %s (%s);", table, quoteIdent(name, dialect), column))
		}
	}
	if payload.ReferencesTable != "" {
		name := safeConstraintName("fk_", payload.Table, payload.Name)
		stmt := fmt.Sprintf(
			"ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)",
			table, quoteIdent(name, dialect), column,
			quoteIdent(payload.ReferencesTable, dialect), quoteIdent(payload.ReferencesColumn, dialect),
		)
		if payload.OnDelete != "" && payload.OnDelete != "NO ACTION" {
			stmt += " ON DELETE " + payload.OnDelete
		}
		stmt += ";"
		up = append(up, stmt)
	}

	down := fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", table, column)
	if dialect == migration.PostgreSQL {
		down = fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS %s;", table, column)
	} else {
		if payload.ReferencesTable != "" {
			name := safeConstraintName("fk_", payload.Table, payload.Name)
			down = fmt.Sprintf("ALTER TABLE %s DROP FOREIGN KEY %s;\n", table, quoteIdent(name, dialect)) + down
		}
		if payload.Unique {
			name := safeConstraintName("uq_", payload.Table, payload.Name)
			down = fmt.Sprintf("ALTER TABLE %s DROP INDEX %s;\n", table, quoteIdent(name, dialect)) + down
		}
	}

	return "-- ⬆ Up (Run when migrating forward)\n" +
		"-- Generated by vm gui from the Add column form. Review before running.\n" +
		strings.Join(up, "\n") +
		"\n\n-- ⬇ Down (Run when rolling back)\n" + down + "\n"
}
