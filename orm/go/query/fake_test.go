package query

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// recorded is one statement the fake DB saw.
type recorded struct {
	SQL  string
	Args []any
}

// fakeRows replays canned values through the Rows interface.
type fakeRows struct {
	cols   []string
	values [][]any
	pos    int
	err    error
	closed bool
}

func (r *fakeRows) Next() bool {
	if r.err != nil {
		return false
	}
	r.pos++
	return r.pos <= len(r.values)
}

func (r *fakeRows) Scan(dest ...any) error {
	if r.pos < 1 || r.pos > len(r.values) {
		return fmt.Errorf("fake: scan out of range")
	}
	row := r.values[r.pos-1]
	if len(dest) != len(row) {
		return fmt.Errorf("fake: scan wants %d dest, row has %d", len(dest), len(row))
	}
	for i, d := range dest {
		if err := assign(d, row[i]); err != nil {
			return fmt.Errorf("fake: column %d (%s): %w", i, r.colName(i), err)
		}
	}
	return nil
}

func (r *fakeRows) colName(i int) string {
	if i < len(r.cols) {
		return r.cols[i]
	}
	return "?"
}

func (r *fakeRows) Close() error               { r.closed = true; return nil }
func (r *fakeRows) Err() error                 { return r.err }
func (r *fakeRows) Columns() ([]string, error) { return r.cols, nil }

func assign(dest, val any) error {
	dv := reflect.ValueOf(dest)
	if dv.Kind() != reflect.Pointer || dv.IsNil() {
		return fmt.Errorf("dest must be a non-nil pointer, got %T", dest)
	}
	target := dv.Elem()
	if val == nil {
		target.Set(reflect.Zero(target.Type()))
		return nil
	}
	vv := reflect.ValueOf(val)
	switch {
	case target.Kind() == reflect.Interface:
		target.Set(vv)
	case vv.Type().AssignableTo(target.Type()):
		target.Set(vv)
	case vv.Type().ConvertibleTo(target.Type()):
		target.Set(vv.Convert(target.Type()))
	case target.Kind() == reflect.Pointer && vv.Type().AssignableTo(target.Type().Elem()):
		p := reflect.New(target.Type().Elem())
		p.Elem().Set(vv)
		target.Set(p)
	case target.Kind() == reflect.Pointer && vv.Type().ConvertibleTo(target.Type().Elem()):
		p := reflect.New(target.Type().Elem())
		p.Elem().Set(vv.Convert(target.Type().Elem()))
		target.Set(p)
	default:
		return fmt.Errorf("cannot assign %T to %s", val, target.Type())
	}
	return nil
}

type fakeResult struct {
	affected int64
	lastID   int64
	err      error
}

func (r fakeResult) RowsAffected() (int64, error) { return r.affected, r.err }
func (r fakeResult) LastInsertId() (int64, error) { return r.lastID, r.err }

// fakeDB answers queries from handlers matched on a SQL substring.
type fakeDB struct {
	mu       sync.Mutex
	seen     []recorded
	handlers []handler
	execRes  fakeResult
	execErr  error
	rowErr   error
}

type handler struct {
	match string
	rows  *fakeRows
	err   error
}

func (d *fakeDB) on(match string, cols []string, values ...[]any) *fakeDB {
	d.handlers = append(d.handlers, handler{match: match, rows: &fakeRows{cols: cols, values: values}})
	return d
}

func (d *fakeDB) onError(match string, err error) *fakeDB {
	d.handlers = append(d.handlers, handler{match: match, err: err})
	return d
}

func (d *fakeDB) record(sqlText string, args []any) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seen = append(d.seen, recorded{SQL: sqlText, Args: args})
}

func (d *fakeDB) statements() []recorded {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]recorded(nil), d.seen...)
}

func (d *fakeDB) count() int { return len(d.statements()) }

func (d *fakeDB) lookup(sqlText string) (*fakeRows, error) {
	for _, h := range d.handlers {
		if strings.Contains(sqlText, h.match) {
			if h.err != nil {
				return nil, h.err
			}
			clone := *h.rows
			clone.pos = 0
			clone.err = d.rowErr
			return &clone, nil
		}
	}
	return &fakeRows{}, nil
}

func (d *fakeDB) QueryContext(ctx context.Context, sqlText string, args ...any) (Rows, error) {
	d.record(sqlText, args)
	return d.lookup(sqlText)
}

func (d *fakeDB) QueryRowContext(ctx context.Context, sqlText string, args ...any) Row {
	d.record(sqlText, args)
	rows, err := d.lookup(sqlText)
	if err != nil {
		return errRow{err}
	}
	if !rows.Next() {
		return errRow{ErrNoRows}
	}
	return singleRow{rows}
}

func (d *fakeDB) ExecContext(ctx context.Context, sqlText string, args ...any) (Result, error) {
	d.record(sqlText, args)
	if d.execErr != nil {
		return nil, d.execErr
	}
	for _, h := range d.handlers {
		if strings.Contains(sqlText, h.match) && h.err != nil {
			return nil, h.err
		}
	}
	return d.execRes, nil
}

type errRow struct{ err error }

func (r errRow) Scan(dest ...any) error { return r.err }

type singleRow struct{ rows *fakeRows }

func (r singleRow) Scan(dest ...any) error { return r.rows.Scan(dest...) }
