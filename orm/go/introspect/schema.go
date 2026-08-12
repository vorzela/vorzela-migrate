package introspect

import (
	"cmp"
	"slices"
	"strings"

	"github.com/vorzela/vorm/query"
)

// Schema is a snapshot of a live database, expressed as plain data. It holds no
// database handle and is safe to cache, diff or serialise.
type Schema struct {
	Dialect   query.Dialect
	Tables    []Table
	Enums     []Enum
	Functions []Function
}

// Table is a base table, partitioned table, view or materialised view.
type Table struct {
	Name        string
	Schema      string
	Comment     string
	Columns     []Column
	PrimaryKey  []string
	Indexes     []Index
	ForeignKeys []ForeignKey
	IsView      bool
}

// Column is a single attribute of a Table.
type Column struct {
	Name     string
	DBType   string
	FullType string
	Nullable bool
	// HasDefault is false for generated columns even though the catalog stores
	// their generation expression in the same place as a default.
	Default      string
	HasDefault   bool
	IsGenerated  bool
	IsIdentity   bool
	IsArray      bool
	ArrayElem    string
	CharMaxLen   int
	NumPrecision int
	NumScale     int
	EnumType     string
	Comment      string
	Position     int
}

// Index describes one index on a Table. Columns keeps catalog order, not
// alphabetical order, because index column order is semantically significant.
type Index struct {
	Name       string
	Columns    []string
	Unique     bool
	Primary    bool
	Method     string
	Partial    bool
	Predicate  string
	Expression bool
}

// ForeignKey describes one referential constraint. Columns and RefColumns are
// positionally paired.
type ForeignKey struct {
	Name       string
	Columns    []string
	RefTable   string
	RefSchema  string
	RefColumns []string
	OnDelete   string
	OnUpdate   string
}

// Enum is a PostgreSQL enum type or a MySQL inline enum column. For MySQL,
// Table and Column record where the inline definition came from and Name is
// synthesised as "{table}_{column}".
type Enum struct {
	Name   string
	Schema string
	Values []string
	Table  string
	Column string
}

// Function is a stored routine.
type Function struct {
	Name       string
	Schema     string
	Args       []FunctionArg
	ReturnType string
	ReturnsSet bool
	Language   string
	Kind       string
}

// FunctionArg is one routine parameter.
type FunctionArg struct {
	Name       string
	DBType     string
	Mode       string
	HasDefault bool
}

// Table returns the named table, matched case-insensitively.
func (s *Schema) Table(name string) (*Table, bool) {
	if s == nil {
		return nil, false
	}
	for i := range s.Tables {
		if strings.EqualFold(s.Tables[i].Name, name) {
			return &s.Tables[i], true
		}
	}
	return nil, false
}

// Enum returns the named enum, matched case-insensitively.
func (s *Schema) Enum(name string) (*Enum, bool) {
	if s == nil {
		return nil, false
	}
	for i := range s.Enums {
		if strings.EqualFold(s.Enums[i].Name, name) {
			return &s.Enums[i], true
		}
	}
	return nil, false
}

// Column returns the named column, matched case-insensitively.
func (t *Table) Column(name string) (*Column, bool) {
	if t == nil {
		return nil, false
	}
	for i := range t.Columns {
		if strings.EqualFold(t.Columns[i].Name, name) {
			return &t.Columns[i], true
		}
	}
	return nil, false
}

// HasColumn reports whether the table has the named column.
func (t *Table) HasColumn(name string) bool {
	_, ok := t.Column(name)
	return ok
}

// HasTimestamps reports whether the table carries both created_at and updated_at.
func (t *Table) HasTimestamps() bool {
	return t.HasColumn("created_at") && t.HasColumn("updated_at")
}

// HasSoftDeletes reports whether the table carries deleted_at.
func (t *Table) HasSoftDeletes() bool {
	return t.HasColumn("deleted_at")
}

// SinglePrimaryKey returns the primary key column name, or "" when the table has
// no primary key or a composite one.
func (t *Table) SinglePrimaryKey() string {
	if t == nil || len(t.PrimaryKey) != 1 {
		return ""
	}
	return t.PrimaryKey[0]
}

// sort orders every collection so that generated output is stable across runs.
// Positional slices — Columns within a table, Columns within an Index or
// ForeignKey, and enum Values — keep their catalog order.
func (s *Schema) sort() {
	slices.SortStableFunc(s.Tables, func(a, b Table) int {
		if c := cmp.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return cmp.Compare(a.Schema, b.Schema)
	})
	for i := range s.Tables {
		t := &s.Tables[i]
		slices.SortStableFunc(t.Columns, func(a, b Column) int {
			if c := cmp.Compare(a.Position, b.Position); c != 0 {
				return c
			}
			return cmp.Compare(a.Name, b.Name)
		})
		slices.SortStableFunc(t.Indexes, func(a, b Index) int {
			return cmp.Compare(a.Name, b.Name)
		})
		slices.SortStableFunc(t.ForeignKeys, func(a, b ForeignKey) int {
			return cmp.Compare(a.Name, b.Name)
		})
	}
	slices.SortStableFunc(s.Enums, func(a, b Enum) int {
		if c := cmp.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Table, b.Table); c != 0 {
			return c
		}
		return cmp.Compare(a.Column, b.Column)
	})
	// PostgreSQL allows overloads, so the argument list breaks name ties.
	slices.SortStableFunc(s.Functions, func(a, b Function) int {
		if c := cmp.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return cmp.Compare(a.signature(), b.signature())
	})
}

func (f Function) signature() string {
	parts := make([]string, len(f.Args))
	for i, a := range f.Args {
		parts[i] = a.Mode + " " + a.DBType
	}
	return strings.Join(parts, ", ")
}
