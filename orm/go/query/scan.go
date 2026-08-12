package query

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

var scannerType = reflect.TypeFor[sql.Scanner]()

// ColumnNamer is implemented by driver Rows that can report result column names.
// It lets ScanStructRows map arbitrary SQL (raw queries, joins) onto a struct
// without the caller restating the projection.
type ColumnNamer interface {
	Columns() ([]string, error)
}

// fieldMap maps a normalized column name to a struct field index path.
type fieldMap map[string][]int

var fieldMapCache sync.Map // reflect.Type -> fieldMap

// structFields resolves db-tagged fields for t, descending into embedded structs.
// Shallower fields win so an outer override beats an embedded default.
func structFields(t reflect.Type) fieldMap {
	if cached, ok := fieldMapCache.Load(t); ok {
		return cached.(fieldMap)
	}
	out := fieldMap{}
	collectFieldPaths(t, nil, out, 0, map[reflect.Type]bool{})
	fieldMapCache.Store(t, out)
	return out
}

func collectFieldPaths(t reflect.Type, prefix []int, out fieldMap, depth int, seen map[reflect.Type]bool) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || seen[t] {
		return
	}
	seen[t] = true
	defer delete(seen, t)

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" && !f.Anonymous {
			continue // unexported
		}
		path := append(append([]int(nil), prefix...), i)

		ft := f.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if f.Anonymous && ft.Kind() == reflect.Struct && f.Tag.Get("db") == "" && !isScannable(f.Type) {
			collectFieldPaths(f.Type, path, out, depth+1, seen)
			continue
		}

		tag := f.Tag.Get("db")
		if tag == "-" {
			continue
		}
		name := tag
		if i := strings.IndexByte(name, ','); i >= 0 {
			name = name[:i]
		}
		if name == "" {
			name = toSnakeCase(f.Name)
		}
		key := strings.ToLower(name)
		if existing, ok := out[key]; ok && len(existing) <= len(path) {
			continue // shallower binding wins
		}
		out[key] = path
	}
}

// isScannable reports whether a struct type handles its own scanning
// (sql.Scanner / time.Time-like), in which case we must not descend into it.
func isScannable(t reflect.Type) bool {
	if t.Kind() != reflect.Pointer {
		t = reflect.PointerTo(t)
	}
	return t.Implements(scannerType) || t.Elem().String() == "time.Time"
}

func toSnakeCase(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 4)
	runes := []rune(s)
	for i, r := range runes {
		isUpper := r >= 'A' && r <= 'Z'
		if isUpper {
			prevLower := i > 0 && !(runes[i-1] >= 'A' && runes[i-1] <= 'Z')
			nextLower := i+1 < len(runes) && !(runes[i+1] >= 'A' && runes[i+1] <= 'Z')
			if i > 0 && (prevLower || nextLower) {
				b.WriteByte('_')
			}
			b.WriteRune(r - 'A' + 'a')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// scanPlan is the resolved binding of one result set's columns to struct fields.
// A nil path means the column has no destination field and is scanned into a sink.
type scanPlan struct {
	paths   [][]int
	missing []string
}

type planKey struct {
	typ  reflect.Type
	cols string
}

var planCache sync.Map // planKey -> *scanPlan

func planFor(t reflect.Type, cols []string) *scanPlan {
	key := planKey{typ: t, cols: strings.Join(cols, "\x00")}
	if cached, ok := planCache.Load(key); ok {
		return cached.(*scanPlan)
	}
	fields := structFields(t)
	plan := &scanPlan{paths: make([][]int, len(cols))}
	for i, c := range cols {
		name := strings.ToLower(scanColumnKey(c))
		if path, ok := fields[name]; ok {
			plan.paths[i] = path
			continue
		}
		plan.missing = append(plan.missing, c)
	}
	planCache.Store(key, plan)
	return plan
}

// scanColumnKey reduces a projection item to the name a struct field would carry:
// strips an alias, a table qualifier, and quoting.
func scanColumnKey(col string) string {
	c := strings.TrimSpace(col)
	if i := strings.LastIndex(strings.ToLower(c), " as "); i >= 0 {
		c = strings.TrimSpace(c[i+4:])
	} else if i := strings.LastIndexByte(c, ' '); i >= 0 && !strings.ContainsAny(c, "()") {
		c = strings.TrimSpace(c[i+1:])
	}
	c = strings.Trim(c, `"'`+"`")
	if i := strings.LastIndexByte(c, '.'); i >= 0 {
		c = c[i+1:]
	}
	return strings.Trim(c, `"'`+"`")
}

// ScanStruct scans the current row of rows into a new T using cached field
// bindings for cols. T may be a struct or a pointer to one.
func ScanStruct[T any](rows Rows, cols []string) (T, error) {
	var zero T
	rt := reflect.TypeFor[T]()
	isPtr := rt.Kind() == reflect.Pointer
	base := rt
	if isPtr {
		base = rt.Elem()
	}
	if base.Kind() != reflect.Struct {
		return zero, fmt.Errorf("vorm/query: ScanStruct needs a struct type, got %s", rt)
	}

	plan := planFor(base, cols)
	ptr := reflect.New(base)
	dest := make([]any, len(cols))
	sinks := make([]any, len(cols))
	bindDest(ptr.Elem(), plan, dest, sinks)

	if err := rows.Scan(dest...); err != nil {
		return zero, err
	}
	if isPtr {
		return ptr.Interface().(T), nil
	}
	return ptr.Elem().Interface().(T), nil
}

func bindDest(sv reflect.Value, plan *scanPlan, dest []any, sinks []any) {
	for i, path := range plan.paths {
		if path == nil {
			dest[i] = &sinks[i]
			continue
		}
		dest[i] = fieldByPathAlloc(sv, path).Addr().Interface()
	}
}

// fieldByPathAlloc walks an index path, allocating nil embedded pointers on the way.
func fieldByPathAlloc(v reflect.Value, path []int) reflect.Value {
	for i, idx := range path {
		if i > 0 {
			for v.Kind() == reflect.Pointer {
				if v.IsNil() {
					v.Set(reflect.New(v.Type().Elem()))
				}
				v = v.Elem()
			}
		}
		v = v.Field(idx)
	}
	return v
}

// structScanner builds a RowMapper[T] bound to cols. It is the fallback used by
// Get/First when no explicit mapper is supplied.
func structScanner[T any](cols []string) (RowMapper[T], error) {
	rt := reflect.TypeFor[T]()
	base := rt
	isPtr := base.Kind() == reflect.Pointer
	if isPtr {
		base = base.Elem()
	}
	if base.Kind() != reflect.Struct {
		return nil, fmt.Errorf("vorm/query: cannot auto-scan %s — pass WithMapper or select into a struct", rt)
	}
	plan := planFor(base, cols)
	return func(rows Rows) (T, error) {
		var zero T
		ptr := reflect.New(base)
		dest := make([]any, len(cols))
		sinks := make([]any, len(cols))
		bindDest(ptr.Elem(), plan, dest, sinks)
		if err := rows.Scan(dest...); err != nil {
			return zero, err
		}
		if isPtr {
			return ptr.Interface().(T), nil
		}
		return ptr.Elem().Interface().(T), nil
	}, nil
}

// ScanStructRows drains rows into []T, resolving columns from the driver.
// Use it for raw SQL that vorm did not build.
func ScanStructRows[T any](rows Rows) ([]T, error) {
	namer, ok := rows.(ColumnNamer)
	if !ok {
		return nil, fmt.Errorf("vorm/query: driver rows do not expose column names — use ScanStruct with an explicit column list")
	}
	cols, err := namer.Columns()
	if err != nil {
		return nil, err
	}
	mapper, err := structScanner[T](cols)
	if err != nil {
		return nil, err
	}
	var out []T
	for rows.Next() {
		v, err := mapper(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
