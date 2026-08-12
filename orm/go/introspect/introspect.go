// Package introspect reads a live PostgreSQL, MySQL or MariaDB schema from the
// database's own catalogs and returns it as plain data.
//
// It exists so the vorm code generator can work from the real schema instead of
// re-deriving it from Blueprint source. The catalogs carry everything the AST
// walker cannot see: nullability, defaults, identity and generated columns,
// arrays, indexes (including partial and expression indexes), foreign keys with
// their referential actions, enum types and stored routines.
//
//	s, err := introspect.Introspect(ctx, db, introspect.Options{
//	    Dialect: query.DialectPostgres,
//	})
//
// Each dialect is read with a fixed, small number of schema-wide queries; the
// per-table wiring happens in Go. Results are fully sorted before they are
// returned so that generated output is byte-stable.
//
// PostgreSQL 12 or newer is required (attgenerated, indnkeyatts, prokind).
package introspect

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/vorzela/vorm/query"
)

// DefaultSchemaName is the PostgreSQL schema read when Options.SchemaName is
// empty. MySQL falls back to DATABASE() instead.
const DefaultSchemaName = "public"

// fkNoAction is the SQL default referential action.
const fkNoAction = "NO ACTION"

// defaultExcludedTables are the vm/vorm migration bookkeeping tables. They are
// dropped from Schema.Tables unless the caller names them in IncludeTables.
var defaultExcludedTables = []string{"migrations", "migrations_lock"}

var errNilDB = errors.New("vorm/introspect: nil database handle")

// Options controls what Introspect reads.
type Options struct {
	Dialect query.Dialect
	// SchemaName is the PostgreSQL schema ("public" by default) or the MySQL
	// database (the connection's current database by default).
	SchemaName string
	// IncludeTables, when non-empty, restricts the result to those tables and
	// also re-enables any default-excluded bookkeeping table it names.
	IncludeTables []string
	// ExcludeTables always wins over IncludeTables.
	ExcludeTables []string
	// IncludeViews adds views and materialised views, flagged with Table.IsView.
	IncludeViews bool
}

// Introspect reads the schema using the driver matching opts.Dialect. An empty
// dialect means PostgreSQL, matching the query package default.
func Introspect(ctx context.Context, db query.DB, opts Options) (*Schema, error) {
	switch normalizeDialect(opts.Dialect) {
	case query.DialectPostgres:
		return Postgres(ctx, db, opts)
	case query.DialectMySQL:
		return MySQL(ctx, db, opts)
	default:
		return nil, fmt.Errorf("vorm/introspect: unsupported dialect %q (want postgres or mysql)", opts.Dialect)
	}
}

func normalizeDialect(d query.Dialect) query.Dialect {
	switch query.Dialect(strings.ToLower(strings.TrimSpace(string(d)))) {
	case "", query.DialectPostgres, "postgresql", "pgx", "pq":
		return query.DialectPostgres
	case query.DialectMySQL, "mariadb":
		return query.DialectMySQL
	default:
		return ""
	}
}

// tableFilter applies Options.IncludeTables / ExcludeTables plus the default
// bookkeeping exclusions. Names are matched case-insensitively because MySQL
// table names may be stored in either case depending on the server platform.
type tableFilter struct {
	include map[string]bool
	exclude map[string]bool
}

func newTableFilter(opts Options) tableFilter {
	f := tableFilter{
		include: foldSet(opts.IncludeTables),
		exclude: foldSet(opts.ExcludeTables),
	}
	for _, name := range defaultExcludedTables {
		if !f.include[name] {
			f.exclude[name] = true
		}
	}
	return f
}

func (f tableFilter) allows(name string) bool {
	key := strings.ToLower(name)
	if len(f.include) > 0 && !f.include[key] {
		return false
	}
	return !f.exclude[key]
}

func foldSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		set[strings.ToLower(n)] = true
	}
	return set
}

// tableSet keeps discovery order while allowing lookups by name. Tables are
// held by pointer so that later queries can attach columns, indexes and foreign
// keys without the slice reallocating underneath them.
type tableSet struct {
	order  []*Table
	byName map[string]*Table
}

func newTableSet() *tableSet {
	return &tableSet{byName: make(map[string]*Table)}
}

func (ts *tableSet) add(t *Table) {
	ts.order = append(ts.order, t)
	ts.byName[t.Name] = t
}

func (ts *tableSet) get(name string) *Table {
	return ts.byName[name]
}

func (ts *tableSet) collect() []Table {
	out := make([]Table, 0, len(ts.order))
	for _, t := range ts.order {
		out = append(out, *t)
	}
	return out
}

// queryRows runs one catalog query and feeds every row to scan, wrapping any
// failure with the step name.
func queryRows(ctx context.Context, db query.DB, step, sql string, args []any, scan func(query.Rows) error) error {
	rows, err := db.QueryContext(ctx, sql, args...)
	if err != nil {
		return wrap(step, err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		if err := scan(rows); err != nil {
			return wrap(step, err)
		}
	}
	if err := rows.Err(); err != nil {
		return wrap(step, err)
	}
	return nil
}

func wrap(step string, err error) error {
	return fmt.Errorf("vorm/introspect: %s: %w", step, err)
}

// applyEnumTypes tags every column whose type resolves to a discovered enum.
// Array columns are matched on their element type.
func applyEnumTypes(s *Schema) {
	if len(s.Enums) == 0 {
		return
	}
	known := make(map[string]string, len(s.Enums))
	for _, e := range s.Enums {
		known[strings.ToLower(e.Name)] = e.Name
	}
	for i := range s.Tables {
		for j := range s.Tables[i].Columns {
			col := &s.Tables[i].Columns[j]
			if col.EnumType != "" {
				continue
			}
			typeName := col.DBType
			if col.IsArray && col.ArrayElem != "" {
				typeName = col.ArrayElem
			}
			if name, ok := known[strings.ToLower(typeName)]; ok {
				col.EnumType = name
			}
		}
	}
}

// normalizeFKAction canonicalises an information_schema referential action.
func normalizeFKAction(s string) string {
	n := strings.Join(strings.Fields(strings.ToUpper(s)), " ")
	if n == "" {
		return fkNoAction
	}
	return n
}
