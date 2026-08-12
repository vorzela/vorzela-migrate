package introspect

import (
	"context"
	"database/sql"
	"strings"

	"github.com/vorzela/vorm/query"
)

// relkind values: r = table, p = partitioned table, v = view, m = materialised view.
const pgTablesQuery = `
SELECT c.relname::text,
       n.nspname::text,
       c.relkind::text,
       COALESCE(obj_description(c.oid, 'pg_class'), '')
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = $1
  AND c.relkind IN ('r', 'p', 'v', 'm')
  AND NOT c.relispartition
ORDER BY c.relname`

// attidentity and attgenerated are "char" columns holding a zero byte when the
// column is neither, so they are compared in SQL rather than scanned.
// typmod carries the declared length: varchar/bpchar store length+4, numeric
// packs precision in the high 16 bits and scale in the low 16 bits of typmod-4.
const pgColumnsQuery = `
SELECT c.relname::text,
       a.attname::text,
       a.attnum::int,
       t.typname::text,
       format_type(a.atttypid, a.atttypmod),
       a.attnotnull,
       a.atthasdef,
       pg_get_expr(d.adbin, d.adrelid),
       (a.attidentity <> ''),
       (a.attgenerated <> ''),
       t.typcategory::text,
       COALESCE(et.typname::text, ''),
       CASE
         WHEN t.typname IN ('varchar', 'bpchar') AND a.atttypmod > 4 THEN a.atttypmod - 4
         WHEN t.typname IN ('bit', 'varbit') AND a.atttypmod > 0 THEN a.atttypmod
         ELSE 0
       END,
       CASE WHEN t.typname = 'numeric' AND a.atttypmod > 4 THEN ((a.atttypmod - 4) >> 16) & 65535 ELSE 0 END,
       CASE WHEN t.typname = 'numeric' AND a.atttypmod > 4 THEN (a.atttypmod - 4) & 65535 ELSE 0 END,
       COALESCE(col_description(c.oid, a.attnum), '')
FROM pg_catalog.pg_attribute a
JOIN pg_catalog.pg_class c ON c.oid = a.attrelid
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
JOIN pg_catalog.pg_type t ON t.oid = a.atttypid
LEFT JOIN pg_catalog.pg_type et ON et.oid = t.typelem AND t.typcategory = 'A'
LEFT JOIN pg_catalog.pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
WHERE n.nspname = $1
  AND c.relkind IN ('r', 'p', 'v', 'm')
  AND a.attnum > 0
  AND NOT a.attisdropped
ORDER BY c.relname, a.attnum`

// indkey holds one entry per indexed attribute; entries past indnkeyatts are
// INCLUDE payload rather than key columns, and an entry of 0 marks an
// expression, which has no pg_attribute row.
const pgIndexesQuery = `
SELECT c.relname::text,
       ic.relname::text,
       i.indisunique,
       i.indisprimary,
       am.amname::text,
       (i.indpred IS NOT NULL),
       COALESCE(pg_get_expr(i.indpred, i.indrelid), ''),
       (i.indexprs IS NOT NULL),
       a.attname::text
FROM pg_catalog.pg_index i
JOIN pg_catalog.pg_class c ON c.oid = i.indrelid
JOIN pg_catalog.pg_class ic ON ic.oid = i.indexrelid
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
JOIN pg_catalog.pg_am am ON am.oid = ic.relam
CROSS JOIN LATERAL unnest(i.indkey) WITH ORDINALITY AS k(attnum, ord)
LEFT JOIN pg_catalog.pg_attribute a
       ON a.attrelid = i.indrelid AND a.attnum = k.attnum AND NOT a.attisdropped
WHERE n.nspname = $1
  AND c.relkind IN ('r', 'p', 'm')
  AND k.ord <= i.indnkeyatts
ORDER BY c.relname, ic.relname, k.ord`

// conkey and confkey are parallel 1-based arrays, so a shared subscript pairs
// each referencing column with the column it references.
const pgForeignKeysQuery = `
SELECT rel.relname::text,
       con.conname::text,
       att.attname::text,
       fnsp.nspname::text,
       frel.relname::text,
       fatt.attname::text,
       con.confupdtype::text,
       con.confdeltype::text
FROM pg_catalog.pg_constraint con
JOIN pg_catalog.pg_class rel ON rel.oid = con.conrelid
JOIN pg_catalog.pg_namespace nsp ON nsp.oid = rel.relnamespace
JOIN pg_catalog.pg_class frel ON frel.oid = con.confrelid
JOIN pg_catalog.pg_namespace fnsp ON fnsp.oid = frel.relnamespace
CROSS JOIN LATERAL generate_subscripts(con.conkey, 1) AS k(ord)
JOIN pg_catalog.pg_attribute att
     ON att.attrelid = con.conrelid AND att.attnum = con.conkey[k.ord]
JOIN pg_catalog.pg_attribute fatt
     ON fatt.attrelid = con.confrelid AND fatt.attnum = con.confkey[k.ord]
WHERE con.contype = 'f'
  AND nsp.nspname = $1
ORDER BY rel.relname, con.conname, k.ord`

const pgEnumsQuery = `
SELECT n.nspname::text,
       t.typname::text,
       e.enumlabel::text
FROM pg_catalog.pg_type t
JOIN pg_catalog.pg_namespace n ON n.oid = t.typnamespace
JOIN pg_catalog.pg_enum e ON e.enumtypid = t.oid
WHERE t.typtype = 'e'
  AND n.nspname = $1
ORDER BY t.typname, e.enumsortorder`

// pg_get_function_result is NULL for procedures, which have no return type.
const pgFunctionsQuery = `
SELECT n.nspname::text,
       p.proname::text,
       pg_get_function_arguments(p.oid),
       pg_get_function_result(p.oid),
       p.proretset,
       l.lanname::text,
       p.prokind::text
FROM pg_catalog.pg_proc p
JOIN pg_catalog.pg_namespace n ON n.oid = p.pronamespace
JOIN pg_catalog.pg_language l ON l.oid = p.prolang
WHERE n.nspname = $1
  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
ORDER BY p.proname, p.oid`

// Postgres reads a PostgreSQL schema from pg_catalog. It runs six schema-wide
// queries regardless of how many tables exist.
func Postgres(ctx context.Context, db query.DB, opts Options) (*Schema, error) {
	if db == nil {
		return nil, errNilDB
	}
	schemaName := opts.SchemaName
	if schemaName == "" {
		schemaName = DefaultSchemaName
	}
	args := []any{schemaName}
	filter := newTableFilter(opts)

	s := &Schema{Dialect: query.DialectPostgres}
	tables := newTableSet()

	if err := pgLoadTables(ctx, db, args, opts, filter, tables); err != nil {
		return nil, err
	}
	if err := pgLoadColumns(ctx, db, args, tables); err != nil {
		return nil, err
	}
	if err := pgLoadIndexes(ctx, db, args, tables); err != nil {
		return nil, err
	}
	if err := pgLoadForeignKeys(ctx, db, args, tables); err != nil {
		return nil, err
	}
	if err := pgLoadEnums(ctx, db, args, s); err != nil {
		return nil, err
	}
	if err := pgLoadFunctions(ctx, db, args, s); err != nil {
		return nil, err
	}

	s.Tables = tables.collect()
	applyEnumTypes(s)
	s.sort()
	return s, nil
}

func pgLoadTables(ctx context.Context, db query.DB, args []any, opts Options, filter tableFilter, tables *tableSet) error {
	return queryRows(ctx, db, "postgres tables", pgTablesQuery, args, func(rows query.Rows) error {
		var name, namespace, relkind, comment string
		if err := rows.Scan(&name, &namespace, &relkind, &comment); err != nil {
			return err
		}
		isView := relkind == "v" || relkind == "m"
		if isView && !opts.IncludeViews {
			return nil
		}
		if !filter.allows(name) {
			return nil
		}
		tables.add(&Table{Name: name, Schema: namespace, Comment: comment, IsView: isView})
		return nil
	})
}

func pgLoadColumns(ctx context.Context, db query.DB, args []any, tables *tableSet) error {
	return queryRows(ctx, db, "postgres columns", pgColumnsQuery, args, func(rows query.Rows) error {
		var (
			tableName, colName      string
			dbType, fullType        string
			typeCategory, elemType  string
			comment                 string
			position                int
			charMaxLen              int
			numPrecision, numScale  int
			notNull, hasDefault     bool
			isIdentity, isGenerated bool
			defaultExpr             sql.NullString
		)
		if err := rows.Scan(
			&tableName, &colName, &position, &dbType, &fullType,
			&notNull, &hasDefault, &defaultExpr, &isIdentity, &isGenerated,
			&typeCategory, &elemType, &charMaxLen, &numPrecision, &numScale, &comment,
		); err != nil {
			return err
		}
		t := tables.get(tableName)
		if t == nil {
			return nil
		}
		col := Column{
			Name:         colName,
			DBType:       dbType,
			FullType:     fullType,
			Nullable:     !notNull,
			Default:      defaultExpr.String,
			HasDefault:   hasDefault && !isGenerated,
			IsGenerated:  isGenerated,
			IsArray:      typeCategory == "A" && elemType != "",
			CharMaxLen:   charMaxLen,
			NumPrecision: numPrecision,
			NumScale:     numScale,
			Comment:      comment,
			Position:     position,
		}
		if col.IsArray {
			col.ArrayElem = elemType
		}
		// serial columns are ordinary defaults over a sequence rather than
		// catalog identity columns.
		col.IsIdentity = isIdentity || strings.HasPrefix(col.Default, "nextval(")
		t.Columns = append(t.Columns, col)
		return nil
	})
}

type relIndexKey struct {
	table string
	index string
}

func pgLoadIndexes(ctx context.Context, db query.DB, args []any, tables *tableSet) error {
	byKey := make(map[relIndexKey]*Index)
	var order []relIndexKey

	err := queryRows(ctx, db, "postgres indexes", pgIndexesQuery, args, func(rows query.Rows) error {
		var (
			tableName, indexName string
			method, predicate    string
			unique, primary      bool
			partial, expression  bool
			colName              sql.NullString
		)
		if err := rows.Scan(
			&tableName, &indexName, &unique, &primary, &method,
			&partial, &predicate, &expression, &colName,
		); err != nil {
			return err
		}
		if tables.get(tableName) == nil {
			return nil
		}
		key := relIndexKey{table: tableName, index: indexName}
		idx, ok := byKey[key]
		if !ok {
			idx = &Index{
				Name:       indexName,
				Unique:     unique,
				Primary:    primary,
				Method:     method,
				Partial:    partial,
				Predicate:  predicate,
				Expression: expression,
			}
			byKey[key] = idx
			order = append(order, key)
		}
		if colName.Valid {
			idx.Columns = append(idx.Columns, colName.String)
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

func pgLoadForeignKeys(ctx context.Context, db query.DB, args []any, tables *tableSet) error {
	byKey := make(map[relIndexKey]*ForeignKey)
	var order []relIndexKey

	err := queryRows(ctx, db, "postgres foreign keys", pgForeignKeysQuery, args, func(rows query.Rows) error {
		var (
			tableName, name            string
			colName                    string
			refSchema, refTable        string
			refColumn                  string
			updateAction, deleteAction string
		)
		if err := rows.Scan(
			&tableName, &name, &colName, &refSchema, &refTable, &refColumn,
			&updateAction, &deleteAction,
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
				OnUpdate:  pgFKAction(updateAction),
				OnDelete:  pgFKAction(deleteAction),
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

func pgLoadEnums(ctx context.Context, db query.DB, args []any, s *Schema) error {
	byName := make(map[string]*Enum)
	var order []string

	err := queryRows(ctx, db, "postgres enums", pgEnumsQuery, args, func(rows query.Rows) error {
		var namespace, name, label string
		if err := rows.Scan(&namespace, &name, &label); err != nil {
			return err
		}
		e, ok := byName[name]
		if !ok {
			e = &Enum{Name: name, Schema: namespace}
			byName[name] = e
			order = append(order, name)
		}
		e.Values = append(e.Values, label)
		return nil
	})
	if err != nil {
		return err
	}

	for _, name := range order {
		s.Enums = append(s.Enums, *byName[name])
	}
	return nil
}

func pgLoadFunctions(ctx context.Context, db query.DB, args []any, s *Schema) error {
	return queryRows(ctx, db, "postgres functions", pgFunctionsQuery, args, func(rows query.Rows) error {
		var (
			namespace, name    string
			language, prokind  string
			returnsSet         bool
			argText, resultTxt sql.NullString
		)
		if err := rows.Scan(&namespace, &name, &argText, &resultTxt, &returnsSet, &language, &prokind); err != nil {
			return err
		}
		returnType := strings.TrimSpace(resultTxt.String)
		if rest, ok := cutWordPrefix(returnType, "SETOF"); ok {
			returnType = rest
		}
		s.Functions = append(s.Functions, Function{
			Name:       name,
			Schema:     namespace,
			Args:       parsePGFunctionArgs(argText.String),
			ReturnType: returnType,
			ReturnsSet: returnsSet,
			Language:   language,
			Kind:       pgFunctionKind(prokind),
		})
		return nil
	})
}

// pgFKAction maps pg_constraint.confupdtype / confdeltype codes.
func pgFKAction(code string) string {
	switch code {
	case "r":
		return "RESTRICT"
	case "c":
		return "CASCADE"
	case "n":
		return "SET NULL"
	case "d":
		return "SET DEFAULT"
	default:
		return fkNoAction
	}
}

func pgFunctionKind(prokind string) string {
	switch prokind {
	case "p":
		return "procedure"
	case "a":
		return "aggregate"
	case "w":
		return "window"
	default:
		return "function"
	}
}

// parsePGFunctionArgs splits the rendered argument list from
// pg_get_function_arguments, e.g.
//
//	IN a integer, b numeric(10,2) DEFAULT 1.5, VARIADIC rest text[]
func parsePGFunctionArgs(signature string) []FunctionArg {
	parts := splitTopLevel(signature, ',')
	args := make([]FunctionArg, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		args = append(args, parsePGFunctionArg(part))
	}
	if len(args) == 0 {
		return nil
	}
	return args
}

func parsePGFunctionArg(s string) FunctionArg {
	arg := FunctionArg{Mode: "IN"}
	// INOUT and VARIADIC must be tested before IN and OUT.
	for _, mode := range []string{"INOUT", "VARIADIC", "OUT", "IN"} {
		if rest, ok := cutWordPrefix(s, mode); ok {
			arg.Mode = mode
			s = rest
			break
		}
	}
	if at := indexTopLevelWord(s, "DEFAULT"); at >= 0 {
		arg.HasDefault = true
		s = strings.TrimSpace(s[:at])
	}
	arg.Name, arg.DBType = splitArgNameAndType(s)
	return arg
}

// pgMultiWordTypeLeads are the words that can start a multi-word type name.
// PostgreSQL quotes any argument named after one of them, so an unquoted
// leading match means the argument is unnamed.
var pgMultiWordTypeLeads = map[string]bool{
	"bit":       true,
	"character": true,
	"double":    true,
	"interval":  true,
	"national":  true,
	"time":      true,
	"timestamp": true,
}

func splitArgNameAndType(s string) (name, dbType string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	if s[0] == '"' {
		quoted, rest := cutQuotedIdent(s)
		return quoted, strings.TrimSpace(rest)
	}
	space := indexTopLevel(s, ' ')
	if space < 0 {
		return "", s
	}
	first := s[:space]
	if pgMultiWordTypeLeads[strings.ToLower(first)] {
		return "", s
	}
	return first, strings.TrimSpace(s[space+1:])
}

func cutQuotedIdent(s string) (ident, rest string) {
	i := 1
	var b strings.Builder
	for i < len(s) {
		if s[i] == '"' {
			if i+1 < len(s) && s[i+1] == '"' {
				b.WriteByte('"')
				i += 2
				continue
			}
			return b.String(), s[i+1:]
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String(), ""
}

// cutWordPrefix strips a leading keyword, matched case-insensitively and only
// on a word boundary.
func cutWordPrefix(s, word string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) < len(word) || !strings.EqualFold(s[:len(word)], word) {
		return s, false
	}
	if len(s) == len(word) {
		return "", true
	}
	if !isSpaceByte(s[len(word)]) {
		return s, false
	}
	return strings.TrimSpace(s[len(word):]), true
}

// indexTopLevelWord finds a keyword outside quotes, parentheses and brackets.
func indexTopLevelWord(s, word string) int {
	scan := newSQLScanner(s)
	for scan.next() {
		if !scan.topLevel() {
			continue
		}
		i := scan.pos
		if i+len(word) > len(s) || !strings.EqualFold(s[i:i+len(word)], word) {
			continue
		}
		if i > 0 && !isSpaceByte(s[i-1]) {
			continue
		}
		if i+len(word) < len(s) && !isSpaceByte(s[i+len(word)]) {
			continue
		}
		return i
	}
	return -1
}

func indexTopLevel(s string, sep byte) int {
	scan := newSQLScanner(s)
	for scan.next() {
		if scan.topLevel() && s[scan.pos] == sep {
			return scan.pos
		}
	}
	return -1
}

// splitTopLevel splits on sep outside quotes, parentheses and brackets, so that
// "numeric(10,2)" and "DEFAULT 'a,b'::text" survive intact.
func splitTopLevel(s string, sep byte) []string {
	var (
		out   []string
		start int
	)
	scan := newSQLScanner(s)
	for scan.next() {
		if scan.topLevel() && s[scan.pos] == sep {
			out = append(out, s[start:scan.pos])
			start = scan.pos + 1
		}
	}
	return append(out, s[start:])
}

// sqlScanner walks a SQL fragment byte by byte, tracking nesting depth and
// string/identifier quoting so callers can act only on top-level positions.
type sqlScanner struct {
	s       string
	pos     int
	started bool
	depth   int
	inQuote byte
}

func newSQLScanner(s string) *sqlScanner {
	return &sqlScanner{s: s}
}

func (sc *sqlScanner) next() bool {
	if sc.started {
		sc.advance()
	}
	sc.started = true
	return sc.pos < len(sc.s)
}

func (sc *sqlScanner) advance() {
	if sc.pos >= len(sc.s) {
		return
	}
	c := sc.s[sc.pos]
	switch {
	case sc.inQuote != 0:
		if c == sc.inQuote {
			// A doubled quote is an escaped quote, not a terminator.
			if sc.pos+1 < len(sc.s) && sc.s[sc.pos+1] == sc.inQuote {
				sc.pos += 2
				return
			}
			sc.inQuote = 0
		}
	case c == '\'' || c == '"':
		sc.inQuote = c
	case c == '(' || c == '[':
		sc.depth++
	case c == ')' || c == ']':
		if sc.depth > 0 {
			sc.depth--
		}
	}
	sc.pos++
}

func (sc *sqlScanner) topLevel() bool {
	return sc.depth == 0 && sc.inQuote == 0
}

func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
