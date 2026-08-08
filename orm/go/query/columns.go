package query

import (
	"fmt"
	"reflect"
	"strings"
	"time"
)

// columnTypeMap is built from model struct fields (db tags) at Model[T] registration.
type columnTypeMap map[string]reflect.Type

func buildColumnTypes[T any]() columnTypeMap {
	out := columnTypeMap{}
	var zero T
	t := reflect.TypeOf(zero)
	if t == nil {
		return out
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return out
	}
	collectFields(t, out)
	return out
}

func collectFields(t reflect.Type, out columnTypeMap) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			collectFields(f.Type, out)
			continue
		}
		col := f.Tag.Get("db")
		if col == "" || col == "-" {
			col = strings.ToLower(f.Name)
			// vorm.Model uses CreatedAt → created_at via common snake; prefer explicit db tags.
			if f.Name == "ID" {
				col = "id"
			} else if f.Name == "CreatedAt" {
				col = "created_at"
			} else if f.Name == "UpdatedAt" {
				col = "updated_at"
			} else if f.Name == "DeletedAt" {
				col = "deleted_at"
			}
		}
		out[col] = f.Type
	}
}

func bareColumn(col string) string {
	if i := strings.LastIndex(col, "."); i >= 0 {
		return col[i+1:]
	}
	return col
}

// HasColumn reports whether col is in Meta.Columns (table.col or bare).
func (m Meta) HasColumn(col string) bool {
	bare := bareColumn(col)
	for _, c := range m.Columns {
		if c == col || c == bare || bareColumn(c) == bare {
			return true
		}
	}
	return false
}

// RequireColumn errors if the column is not listed on Meta (prevents typos / injection via unknown names).
// Qualified columns on other tables (joins) are allowed after SafeIdent only.
func (m Meta) RequireColumn(col string) error {
	if col == "" || strings.HasPrefix(col, "__") {
		return nil
	}
	if err := SafeIdent(col); err != nil {
		return err
	}
	if i := strings.LastIndex(col, "."); i >= 0 {
		table, bare := col[:i], col[i+1:]
		if table != m.Table {
			return nil // join / other relation
		}
		col = bare
	}
	if !m.HasColumn(col) {
		return fmt.Errorf("vorm/query: unknown column %q on table %q (known: %s)",
			col, m.Table, strings.Join(m.Columns, ", "))
	}
	return nil
}

// CheckColumnValue ensures col exists and value is assignable to the model field type (when known).
func (m Meta) CheckColumnValue(col string, value any) error {
	if err := m.RequireColumn(col); err != nil {
		return err
	}
	if value == nil || m.columnTypes == nil {
		return nil
	}
	want, ok := m.columnTypes[bareColumn(col)]
	if !ok {
		return nil
	}
	return checkAssignable(want, value, bareColumn(col))
}

func checkAssignable(want reflect.Type, value any, col string) error {
	if op, ok := value.(Operator); ok {
		if op.Value == nil && (op.Op == "IS NULL" || op.Op == "IS NOT NULL") {
			return nil
		}
		if op.Op == "IN" {
			return checkInValues(want, op.Value, col)
		}
		return checkAssignable(want, op.Value, col)
	}
	got := reflect.TypeOf(value)
	if got == nil {
		return nil
	}
	if assignableTo(want, got) {
		return nil
	}
	return fmt.Errorf("vorm/query: column %q expects %s, got %s", col, typeName(want), typeName(got))
}

func checkInValues(want reflect.Type, v any, col string) error {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return nil
	}
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return checkAssignable(want, v, col)
	}
	for i := 0; i < rv.Len(); i++ {
		if err := checkAssignable(want, rv.Index(i).Interface(), col); err != nil {
			return err
		}
	}
	return nil
}

func assignableTo(want, got reflect.Type) bool {
	if want == nil || got == nil {
		return true
	}
	if got.AssignableTo(want) || got.ConvertibleTo(want) {
		return true
	}
	// unwrap pointers
	w, g := want, got
	for w.Kind() == reflect.Pointer {
		w = w.Elem()
	}
	for g.Kind() == reflect.Pointer {
		g = g.Elem()
	}
	if g.AssignableTo(w) || g.ConvertibleTo(w) {
		return true
	}
	// common numeric widenings: int literals vs int64 fields
	if isIntLike(w) && isIntLike(g) {
		return true
	}
	if isFloatLike(w) && isFloatLike(g) {
		return true
	}
	if w == reflect.TypeOf(time.Time{}) && g == reflect.TypeOf(time.Time{}) {
		return true
	}
	// string-backed enums
	if w.Kind() == reflect.String && g.Kind() == reflect.String {
		return true
	}
	return false
}

func isIntLike(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	default:
		return false
	}
}

func isFloatLike(t reflect.Type) bool {
	k := t.Kind()
	return k == reflect.Float32 || k == reflect.Float64
}

func typeName(t reflect.Type) string {
	if t == nil {
		return "nil"
	}
	return t.String()
}
