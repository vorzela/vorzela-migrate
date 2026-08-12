package introspect

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/vorzela/vorm/query"
)

const mysqlCurrentDatabaseQuery = `SELECT DATABASE()`

const mysqlTablesQuery = `
SELECT t.TABLE_NAME, t.TABLE_TYPE, COALESCE(t.TABLE_COMMENT, '')
FROM information_schema.tables t
WHERE t.TABLE_SCHEMA = ?
ORDER BY t.TABLE_NAME`

const mysqlColumnsQuery = `
SELECT c.TABLE_NAME,
       c.COLUMN_NAME,
       c.ORDINAL_POSITION,
       c.DATA_TYPE,
       c.COLUMN_TYPE,
       c.IS_NULLABLE,
       c.COLUMN_DEFAULT,
       COALESCE(c.EXTRA, ''),
       c.CHARACTER_MAXIMUM_LENGTH,
       c.NUMERIC_PRECISION,
       c.NUMERIC_SCALE,
       COALESCE(c.COLUMN_COMMENT, '')
FROM information_schema.columns c
WHERE c.TABLE_SCHEMA = ?
ORDER BY c.TABLE_NAME, c.ORDINAL_POSITION`

// COLUMN_NAME is NULL for functional key parts (MySQL 8.0.13+), which is how an
// expression index is recognised without reading the version-specific
// EXPRESSION column that MariaDB does not have.
const mysqlIndexesQuery = `
SELECT s.TABLE_NAME,
       s.INDEX_NAME,
       s.NON_UNIQUE,
       s.COLUMN_NAME,
       COALESCE(s.INDEX_TYPE, '')
FROM information_schema.statistics s
WHERE s.TABLE_SCHEMA = ?
ORDER BY s.TABLE_NAME, s.INDEX_NAME, s.SEQ_IN_INDEX`

const mysqlForeignKeysQuery = `
SELECT k.TABLE_NAME,
       k.CONSTRAINT_NAME,
       k.COLUMN_NAME,
       COALESCE(k.REFERENCED_TABLE_SCHEMA, ''),
       k.REFERENCED_TABLE_NAME,
       k.REFERENCED_COLUMN_NAME,
       COALESCE(r.UPDATE_RULE, ''),
       COALESCE(r.DELETE_RULE, '')
FROM information_schema.key_column_usage k
JOIN information_schema.referential_constraints r
  ON r.CONSTRAINT_SCHEMA = k.CONSTRAINT_SCHEMA
 AND r.CONSTRAINT_NAME = k.CONSTRAINT_NAME
 AND r.TABLE_NAME = k.TABLE_NAME
WHERE k.TABLE_SCHEMA = ?
  AND k.REFERENCED_TABLE_NAME IS NOT NULL
ORDER BY k.TABLE_NAME, k.CONSTRAINT_NAME, k.ORDINAL_POSITION`

// DTD_IDENTIFIER is the return type and is NULL for procedures.
const mysqlRoutinesQuery = `
SELECT r.ROUTINE_NAME,
       r.ROUTINE_TYPE,
       COALESCE(r.DTD_IDENTIFIER, ''),
       COALESCE(r.EXTERNAL_LANGUAGE, r.ROUTINE_BODY, '')
FROM information_schema.routines r
WHERE r.ROUTINE_SCHEMA = ?
ORDER BY r.ROUTINE_NAME`

// Ordinal position 0 is a function's return value rather than a parameter.
const mysqlParametersQuery = `
SELECT p.SPECIFIC_NAME,
       p.ORDINAL_POSITION,
       COALESCE(p.PARAMETER_NAME, ''),
       COALESCE(p.PARAMETER_MODE, ''),
       COALESCE(p.DTD_IDENTIFIER, '')
FROM information_schema.parameters p
WHERE p.SPECIFIC_SCHEMA = ?
ORDER BY p.SPECIFIC_NAME, p.ORDINAL_POSITION`

// MySQL reads a MySQL or MariaDB schema from information_schema. It runs six
// schema-wide queries, plus one to resolve DATABASE() when Options.SchemaName
// is empty.
func MySQL(ctx context.Context, db query.DB, opts Options) (*Schema, error) {
	if db == nil {
		return nil, errNilDB
	}
	schemaName, err := mysqlSchemaName(ctx, db, opts)
	if err != nil {
		return nil, err
	}
	args := []any{schemaName}
	filter := newTableFilter(opts)

	s := &Schema{Dialect: query.DialectMySQL}
	tables := newTableSet()

	if err := mysqlLoadTables(ctx, db, args, schemaName, opts, filter, tables); err != nil {
		return nil, err
	}
	if err := mysqlLoadColumns(ctx, db, args, schemaName, tables, s); err != nil {
		return nil, err
	}
	if err := mysqlLoadIndexes(ctx, db, args, tables); err != nil {
		return nil, err
	}
	if err := mysqlLoadForeignKeys(ctx, db, args, tables); err != nil {
		return nil, err
	}
	if err := mysqlLoadRoutines(ctx, db, args, schemaName, s); err != nil {
		return nil, err
	}

	s.Tables = tables.collect()
	applyEnumTypes(s)
	s.sort()
	return s, nil
}

func mysqlSchemaName(ctx context.Context, db query.DB, opts Options) (string, error) {
	if opts.SchemaName != "" {
		return opts.SchemaName, nil
	}
	var current sql.NullString
	if err := db.QueryRowContext(ctx, mysqlCurrentDatabaseQuery).Scan(&current); err != nil {
		return "", wrap("mysql current database", err)
	}
	if !current.Valid || current.String == "" {
		return "", errors.New("vorm/introspect: no database selected; set Options.SchemaName")
	}
	return current.String, nil
}

func mysqlLoadTables(ctx context.Context, db query.DB, args []any, schemaName string, opts Options, filter tableFilter, tables *tableSet) error {
	return queryRows(ctx, db, "mysql tables", mysqlTablesQuery, args, func(rows query.Rows) error {
		var name, tableType, comment string
		if err := rows.Scan(&name, &tableType, &comment); err != nil {
			return err
		}
		isView := strings.Contains(strings.ToUpper(tableType), "VIEW")
		if isView && !opts.IncludeViews {
			return nil
		}
		if !filter.allows(name) {
			return nil
		}
		tables.add(&Table{Name: name, Schema: schemaName, Comment: comment, IsView: isView})
		return nil
	})
}

func mysqlLoadColumns(ctx context.Context, db query.DB, args []any, schemaName string, tables *tableSet, s *Schema) error {
	return queryRows(ctx, db, "mysql columns", mysqlColumnsQuery, args, func(rows query.Rows) error {
		var (
			tableName, colName   string
			dataType, columnType string
			isNullable, extra    string
			comment              string
			position             int
			defaultValue         sql.NullString
			charMaxLen           sql.NullInt64
			numPrecision         sql.NullInt64
			numScale             sql.NullInt64
		)
		if err := rows.Scan(
			&tableName, &colName, &position, &dataType, &columnType,
			&isNullable, &defaultValue, &extra,
			&charMaxLen, &numPrecision, &numScale, &comment,
		); err != nil {
			return err
		}
		t := tables.get(tableName)
		if t == nil {
			return nil
		}
		generated := mysqlIsGenerated(extra)
		col := Column{
			Name:         colName,
			DBType:       dataType,
			FullType:     columnType,
			Nullable:     strings.EqualFold(isNullable, "YES"),
			Default:      defaultValue.String,
			HasDefault:   defaultValue.Valid && !generated,
			IsGenerated:  generated,
			IsIdentity:   mysqlIsAutoIncrement(extra),
			CharMaxLen:   int(charMaxLen.Int64),
			NumPrecision: int(numPrecision.Int64),
			NumScale:     int(numScale.Int64),
			Comment:      comment,
			Position:     position,
		}
		// MySQL enums are declared inline on the column, so each one becomes a
		// synthetic named type the generator can emit.
		if strings.EqualFold(dataType, "enum") {
			if values := parseMySQLEnumValues(columnType); values != nil {
				name := tableName + "_" + colName
				s.Enums = append(s.Enums, Enum{
					Name:   name,
					Schema: schemaName,
					Values: values,
					Table:  tableName,
					Column: colName,
				})
				col.EnumType = name
			}
		}
		t.Columns = append(t.Columns, col)
		return nil
	})
}

func mysqlLoadIndexes(ctx context.Context, db query.DB, args []any, tables *tableSet) error {
	byKey := make(map[relIndexKey]*Index)
	var order []relIndexKey

	err := queryRows(ctx, db, "mysql indexes", mysqlIndexesQuery, args, func(rows query.Rows) error {
		var (
			tableName, indexName string
			indexType            string
			nonUnique            int
			colName              sql.NullString
		)
		if err := rows.Scan(&tableName, &indexName, &nonUnique, &colName, &indexType); err != nil {
			return err
		}
		if tables.get(tableName) == nil {
			return nil
		}
		key := relIndexKey{table: tableName, index: indexName}
		idx, ok := byKey[key]
		if !ok {
			idx = &Index{
				Name:    indexName,
				Unique:  nonUnique == 0,
				Primary: indexName == "PRIMARY",
				Method:  strings.ToLower(indexType),
			}
			byKey[key] = idx
			order = append(order, key)
		}
		if colName.Valid {
			idx.Columns = append(idx.Columns, colName.String)
		} else {
			idx.Expression = true
		}
		return nil
	})
	if err != nil {
		return err
	}

	for _, key := range order {
		t := tables.get(key.table)
		if t == nil {
			continue
		}
		idx := *byKey[key]
		t.Indexes = append(t.Indexes, idx)
		if idx.Primary {
			t.PrimaryKey = append([]string(nil), idx.Columns...)
		}
	}
	return nil
}

func mysqlLoadForeignKeys(ctx context.Context, db query.DB, args []any, tables *tableSet) error {
	byKey := make(map[relIndexKey]*ForeignKey)
	var order []relIndexKey

	err := queryRows(ctx, db, "mysql foreign keys", mysqlForeignKeysQuery, args, func(rows query.Rows) error {
		var (
			tableName, name        string
			colName                string
			refSchema, refTable    string
			refColumn              string
			updateRule, deleteRule string
		)
		if err := rows.Scan(
			&tableName, &name, &colName, &refSchema, &refTable, &refColumn,
			&updateRule, &deleteRule,
		); err != nil {
			return err
		}
		if tables.get(tableName) == nil {
			return nil
		}
		key := relIndexKey{table: tableName, index: name}
		fk, ok := byKey[key]
		if !ok {
			fk = &ForeignKey{
				Name:      name,
				RefTable:  refTable,
				RefSchema: refSchema,
				OnUpdate:  normalizeFKAction(updateRule),
				OnDelete:  normalizeFKAction(deleteRule),
			}
			byKey[key] = fk
			order = append(order, key)
		}
		fk.Columns = append(fk.Columns, colName)
		fk.RefColumns = append(fk.RefColumns, refColumn)
		return nil
	})
	if err != nil {
		return err
	}

	for _, key := range order {
		if t := tables.get(key.table); t != nil {
			t.ForeignKeys = append(t.ForeignKeys, *byKey[key])
		}
	}
	return nil
}

func mysqlLoadRoutines(ctx context.Context, db query.DB, args []any, schemaName string, s *Schema) error {
	byName := make(map[string]*Function)
	var order []string

	err := queryRows(ctx, db, "mysql routines", mysqlRoutinesQuery, args, func(rows query.Rows) error {
		var name, routineType, returnType, language string
		if err := rows.Scan(&name, &routineType, &returnType, &language); err != nil {
			return err
		}
		fn := &Function{
			Name:       name,
			Schema:     schemaName,
			ReturnType: returnType,
			Language:   language,
			Kind:       strings.ToLower(routineType),
		}
		byName[name] = fn
		order = append(order, name)
		return nil
	})
	if err != nil {
		return err
	}

	err = queryRows(ctx, db, "mysql routine parameters", mysqlParametersQuery, args, func(rows query.Rows) error {
		var specificName, paramName, paramMode, dataType string
		var position int
		if err := rows.Scan(&specificName, &position, &paramName, &paramMode, &dataType); err != nil {
			return err
		}
		fn, ok := byName[specificName]
		if !ok || position == 0 {
			return nil
		}
		mode := strings.ToUpper(paramMode)
		if mode == "" {
			mode = "IN"
		}
		fn.Args = append(fn.Args, FunctionArg{Name: paramName, DBType: dataType, Mode: mode})
		return nil
	})
	if err != nil {
		return err
	}

	for _, name := range order {
		s.Functions = append(s.Functions, *byName[name])
	}
	return nil
}

// mysqlIsAutoIncrement reports whether information_schema EXTRA marks the column
// as auto_increment.
func mysqlIsAutoIncrement(extra string) bool {
	return strings.Contains(strings.ToLower(extra), "auto_increment")
}

// mysqlIsGenerated reports whether EXTRA marks a stored or virtual generated
// column. MySQL 8 also writes DEFAULT_GENERATED for expression defaults, which
// are not generated columns, so that token is removed first. MariaDB before
// 10.2 writes a bare VIRTUAL or PERSISTENT.
func mysqlIsGenerated(extra string) bool {
	rest := strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(extra)), "DEFAULT_GENERATED", "")
	if strings.Contains(rest, "GENERATED") {
		return true
	}
	switch strings.TrimSpace(rest) {
	case "VIRTUAL", "PERSISTENT":
		return true
	}
	return false
}

// parseMySQLEnumValues extracts the members of an inline enum('a','b') or
// set('a','b') COLUMN_TYPE. It returns nil when columnType is not an enum or
// set, and a possibly empty slice otherwise.
//
// MySQL renders an embedded quote as ” but a client that wrote it with a
// backslash escape can leave \' in the catalog, so both forms are decoded.
func parseMySQLEnumValues(columnType string) []string {
	s := strings.TrimSpace(columnType)
	open := strings.IndexByte(s, '(')
	if open < 0 {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(s[:open])) {
	case "enum", "set":
	default:
		return nil
	}

	values := []string{}
	for i := open + 1; i < len(s); {
		switch s[i] {
		case ' ', '\t', '\n', '\r', ',':
			i++
		case '\'':
			value, next, ok := scanMySQLQuoted(s, i)
			if !ok {
				return values
			}
			values = append(values, value)
			i = next
		default:
			// A closing paren, or a trailing charset/collation clause.
			return values
		}
	}
	return values
}

func scanMySQLQuoted(s string, start int) (value string, next int, ok bool) {
	var b strings.Builder
	for i := start + 1; i < len(s); {
		switch s[i] {
		case '\'':
			if i+1 < len(s) && s[i+1] == '\'' {
				b.WriteByte('\'')
				i += 2
				continue
			}
			return b.String(), i + 1, true
		case '\\':
			if i+1 >= len(s) {
				return b.String(), len(s), false
			}
			b.WriteByte(unescapeMySQLByte(s[i+1]))
			i += 2
		default:
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String(), len(s), false
}

func unescapeMySQLByte(c byte) byte {
	switch c {
	case '0':
		return 0
	case 'b':
		return '\b'
	case 'n':
		return '\n'
	case 'r':
		return '\r'
	case 't':
		return '\t'
	case 'Z':
		return 26
	default:
		return c
	}
}
