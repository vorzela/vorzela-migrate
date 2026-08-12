package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/vorzela/vorm/query"
)

// fakeDB is an in-memory query.DB that understands the handful of statements the
// runner issues against the tracking table and the lock functions. Everything
// else is recorded in log so tests can assert what a migration executed, in
// which order, and whether it ran inside a transaction.
type fakeDB struct {
	mu sync.Mutex

	rows   []trackRow
	nextID int64

	log     []string
	locks   []string
	creates int

	// failOn maps an uppercased substring to the error returned when a statement
	// contains it.
	failOn map[string]error

	// busyFor makes the first n lock attempts report that the lock is taken.
	busyFor      int
	lockAttempts int
	lockHeld     bool
}

type trackRow struct {
	id       int64
	name     string
	batch    int
	checksum string
	ms       int64
}

func newFakeDB() *fakeDB {
	return &fakeDB{failOn: map[string]error{}}
}

func (f *fakeDB) ExecContext(_ context.Context, q string, args ...any) (query.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	statement := normalizeSpace(q)
	switch upper := strings.ToUpper(statement); {
	case strings.HasPrefix(upper, "CREATE TABLE IF NOT EXISTS MIGRATIONS"):
		f.creates++
		return fakeResult{}, nil

	case strings.HasPrefix(upper, "INSERT INTO MIGRATIONS"):
		if len(args) != 4 {
			return nil, fmt.Errorf("fakeDB: insert wants 4 args, got %d", len(args))
		}
		name := fmt.Sprint(args[0])
		for _, row := range f.rows {
			if row.name == name {
				return nil, fmt.Errorf("fakeDB: duplicate key value violates unique constraint: %s", name)
			}
		}
		f.nextID++
		f.rows = append(f.rows, trackRow{
			id:       f.nextID,
			name:     name,
			batch:    int(toInt64(args[1])),
			checksum: fmt.Sprint(args[2]),
			ms:       toInt64(args[3]),
		})
		return fakeResult{affected: 1}, nil

	case strings.HasPrefix(upper, "DELETE FROM MIGRATIONS"):
		id := toInt64(args[0])
		kept := make([]trackRow, 0, len(f.rows))
		affected := int64(0)
		for _, row := range f.rows {
			if row.id == id {
				affected++
				continue
			}
			kept = append(kept, row)
		}
		f.rows = kept
		return fakeResult{affected: affected}, nil

	case strings.HasPrefix(upper, "UPDATE MIGRATIONS SET CHECKSUM"):
		checksum, name := fmt.Sprint(args[0]), fmt.Sprint(args[1])
		affected := int64(0)
		for i := range f.rows {
			if f.rows[i].name == name {
				f.rows[i].checksum = checksum
				affected++
			}
		}
		return fakeResult{affected: affected}, nil

	case strings.Contains(upper, "PG_ADVISORY_UNLOCK"), strings.Contains(upper, "RELEASE_LOCK("):
		f.locks = append(f.locks, statement)
		f.lockHeld = false
		return fakeResult{affected: 1}, nil

	default:
		f.log = append(f.log, statement)
		for substr, err := range f.failOn {
			if strings.Contains(strings.ToUpper(statement), strings.ToUpper(substr)) {
				return nil, err
			}
		}
		return fakeResult{affected: 1}, nil
	}
}

func (f *fakeDB) QueryRowContext(_ context.Context, q string, _ ...any) query.Row {
	f.mu.Lock()
	defer f.mu.Unlock()

	statement := normalizeSpace(q)
	switch upper := strings.ToUpper(statement); {
	case strings.Contains(upper, "COALESCE(MAX(BATCH), 0) + 1"):
		highest := 0
		for _, row := range f.rows {
			if row.batch > highest {
				highest = row.batch
			}
		}
		return fakeRow{values: []any{highest + 1}}

	case strings.Contains(upper, "PG_TRY_ADVISORY_LOCK"):
		f.locks = append(f.locks, statement)
		f.lockAttempts++
		if f.lockAttempts <= f.busyFor {
			return fakeRow{values: []any{false}}
		}
		f.lockHeld = true
		return fakeRow{values: []any{true}}

	case strings.Contains(upper, "GET_LOCK("):
		f.locks = append(f.locks, statement)
		f.lockAttempts++
		if f.lockAttempts <= f.busyFor {
			return fakeRow{values: []any{int64(0)}}
		}
		f.lockHeld = true
		return fakeRow{values: []any{int64(1)}}

	default:
		return fakeRow{err: fmt.Errorf("fakeDB: unexpected QueryRow: %s", statement)}
	}
}

func (f *fakeDB) QueryContext(_ context.Context, q string, _ ...any) (query.Rows, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	statement := normalizeSpace(q)
	if !strings.HasPrefix(strings.ToUpper(statement), "SELECT ID, MIGRATION, BATCH") {
		return nil, fmt.Errorf("fakeDB: unexpected Query: %s", statement)
	}

	ordered := append([]trackRow(nil), f.rows...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].batch != ordered[j].batch {
			return ordered[i].batch < ordered[j].batch
		}
		return ordered[i].id < ordered[j].id
	})

	values := make([][]any, 0, len(ordered))
	for _, row := range ordered {
		values = append(values, []any{row.id, row.name, row.batch, row.checksum, int(row.ms)})
	}
	return &fakeRows{values: values}, nil
}

func (f *fakeDB) BeginTx(_ context.Context, _ *query.TxOptions) (query.Tx, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.log = append(f.log, "BEGIN")
	return &fakeTx{db: f, rows: append([]trackRow(nil), f.rows...), nextID: f.nextID}, nil
}

// snapshot copies the tracking table so assertions do not race with the runner.
func (f *fakeDB) snapshot() []trackRow {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]trackRow(nil), f.rows...)
}

func (f *fakeDB) names() []string {
	rows := f.snapshot()
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.name)
	}
	return out
}

func (f *fakeDB) statements() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.log...)
}

func (f *fakeDB) lockCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.locks...)
}

// fakeTx applies to the parent immediately and restores the tracking table on
// rollback, so a failed migration can be asserted to leave no row behind.
type fakeTx struct {
	db     *fakeDB
	rows   []trackRow
	nextID int64
}

func (t *fakeTx) ExecContext(ctx context.Context, q string, args ...any) (query.Result, error) {
	return t.db.ExecContext(ctx, q, args...)
}

func (t *fakeTx) QueryRowContext(ctx context.Context, q string, args ...any) query.Row {
	return t.db.QueryRowContext(ctx, q, args...)
}

func (t *fakeTx) QueryContext(ctx context.Context, q string, args ...any) (query.Rows, error) {
	return t.db.QueryContext(ctx, q, args...)
}

func (t *fakeTx) Commit() error {
	t.db.mu.Lock()
	defer t.db.mu.Unlock()
	t.db.log = append(t.db.log, "COMMIT")
	return nil
}

func (t *fakeTx) Rollback() error {
	t.db.mu.Lock()
	defer t.db.mu.Unlock()
	t.db.rows = append([]trackRow(nil), t.rows...)
	t.db.nextID = t.nextID
	t.db.log = append(t.db.log, "ROLLBACK")
	return nil
}

type fakeResult struct{ affected int64 }

func (r fakeResult) RowsAffected() (int64, error) { return r.affected, nil }
func (r fakeResult) LastInsertId() (int64, error) { return 0, nil }

type fakeRow struct {
	values []any
	err    error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	return scanInto(dest, r.values)
}

type fakeRows struct {
	values [][]any
	pos    int
}

func (r *fakeRows) Next() bool {
	if r.pos >= len(r.values) {
		return false
	}
	r.pos++
	return true
}

func (r *fakeRows) Scan(dest ...any) error { return scanInto(dest, r.values[r.pos-1]) }
func (r *fakeRows) Close() error           { return nil }
func (r *fakeRows) Err() error             { return nil }

func scanInto(dest []any, values []any) error {
	if len(dest) != len(values) {
		return fmt.Errorf("fakeDB: scan wants %d destinations, got %d", len(values), len(dest))
	}
	for i, value := range values {
		switch target := dest[i].(type) {
		case *int64:
			*target = toInt64(value)
		case *int:
			*target = int(toInt64(value))
		case *string:
			s, ok := value.(string)
			if !ok {
				return fmt.Errorf("fakeDB: cannot scan %T into *string", value)
			}
			*target = s
		case *bool:
			b, ok := value.(bool)
			if !ok {
				return fmt.Errorf("fakeDB: cannot scan %T into *bool", value)
			}
			*target = b
		case *sql.NullInt64:
			if value == nil {
				*target = sql.NullInt64{}
				continue
			}
			*target = sql.NullInt64{Int64: toInt64(value), Valid: true}
		default:
			return fmt.Errorf("fakeDB: unsupported scan destination %T", dest[i])
		}
	}
	return nil
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int32:
		return int64(n)
	case int64:
		return n
	default:
		return 0
	}
}
