package introspect

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"

	"github.com/vorzela/vorm/query"
)

// canned is one replayed catalog result. match is a substring unique to the
// query it answers.
type canned struct {
	match string
	rows  [][]any
	err   error
}

// fakeDB replays canned catalog rows so the assembly logic can be exercised
// without a server. Any query without a canned result fails loudly, so adding
// a query to the introspector without extending a test is caught immediately.
type fakeDB struct {
	results  []canned
	rowValue []any
	rowErr   error
	seen     []string
}

func (d *fakeDB) QueryContext(_ context.Context, sqlText string, _ ...any) (query.Rows, error) {
	d.seen = append(d.seen, sqlText)
	for _, r := range d.results {
		if !strings.Contains(sqlText, r.match) {
			continue
		}
		if r.err != nil {
			return nil, r.err
		}
		return &fakeRows{rows: r.rows}, nil
	}
	return nil, fmt.Errorf("fakeDB: no canned result for query: %s", sqlText)
}

func (d *fakeDB) QueryRowContext(_ context.Context, sqlText string, _ ...any) query.Row {
	d.seen = append(d.seen, sqlText)
	return &fakeRow{values: d.rowValue, err: d.rowErr}
}

func (d *fakeDB) ExecContext(_ context.Context, sqlText string, _ ...any) (query.Result, error) {
	return nil, fmt.Errorf("fakeDB: unexpected Exec: %s", sqlText)
}

// ran reports whether a query containing match was executed.
func (d *fakeDB) ran(match string) bool {
	for _, s := range d.seen {
		if strings.Contains(s, match) {
			return true
		}
	}
	return false
}

type fakeRow struct {
	values []any
	err    error
}

func (r *fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	return assignRow(dest, r.values)
}

type fakeRows struct {
	rows   [][]any
	cursor int
	cur    []any
	closed bool
	err    error
}

func (r *fakeRows) Next() bool {
	if r.err != nil || r.cursor >= len(r.rows) {
		return false
	}
	r.cur = r.rows[r.cursor]
	r.cursor++
	return true
}

func (r *fakeRows) Scan(dest ...any) error { return assignRow(dest, r.cur) }
func (r *fakeRows) Close() error           { r.closed = true; return nil }
func (r *fakeRows) Err() error             { return r.err }

func assignRow(dest []any, values []any) error {
	if len(dest) != len(values) {
		return fmt.Errorf("scan arity mismatch: %d destinations, %d values", len(dest), len(values))
	}
	for i := range dest {
		if err := assign(dest[i], values[i]); err != nil {
			return fmt.Errorf("column %d: %w", i, err)
		}
	}
	return nil
}

func assign(dst, src any) error {
	if scanner, ok := dst.(sql.Scanner); ok {
		return scanner.Scan(src)
	}
	dv := reflect.ValueOf(dst)
	if dv.Kind() != reflect.Pointer || dv.IsNil() {
		return fmt.Errorf("destination %T is not a non-nil pointer", dst)
	}
	elem := dv.Elem()
	if src == nil {
		elem.SetZero()
		return nil
	}
	sv := reflect.ValueOf(src)
	switch {
	case sv.Type().AssignableTo(elem.Type()):
		elem.Set(sv)
	case sv.Type().ConvertibleTo(elem.Type()):
		elem.Set(sv.Convert(elem.Type()))
	default:
		return fmt.Errorf("cannot assign %T to %T", src, dst)
	}
	return nil
}

// selectArity counts the top-level expressions in a query's SELECT list so a
// test can assert the scan destinations line up with the catalog columns.
func selectArity(sqlText string) int {
	upper := strings.ToUpper(sqlText)
	start := strings.Index(upper, "SELECT ")
	if start < 0 {
		return -1
	}
	start += len("SELECT ")
	end := strings.Index(upper[start:], "\nFROM ")
	if end < 0 {
		return -1
	}
	return len(splitTopLevel(sqlText[start:start+end], ','))
}
