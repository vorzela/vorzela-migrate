package migrate

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/vorzela/vorm/query"
)

// tableName is not configurable: vm and this package must read the same state.
const tableName = "migrations"

const createTablePostgres = `CREATE TABLE IF NOT EXISTS migrations (
    id SERIAL PRIMARY KEY,
    migration VARCHAR(255) NOT NULL UNIQUE,
    batch INTEGER NOT NULL,
    checksum VARCHAR(64),
    executed_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    execution_time_ms INTEGER DEFAULT 0
);`

const createTableMySQL = `CREATE TABLE IF NOT EXISTS migrations (
    id INT AUTO_INCREMENT PRIMARY KEY,
    migration VARCHAR(255) NOT NULL UNIQUE,
    batch INT NOT NULL,
    checksum VARCHAR(64),
    executed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    execution_time_ms INT DEFAULT 0
);`

// AppliedRow is one row of the migrations tracking table.
type AppliedRow struct {
	ID   int64
	Name string
	// Batch groups every migration applied by the same Up call.
	Batch int
	// Checksum is empty for rows written before checksums were recorded.
	Checksum   string
	DurationMS int
}

func createTableSQL(d query.Dialect) string {
	if isMySQL(d) {
		return createTableMySQL
	}
	return createTablePostgres
}

// placeholder returns the bind marker for the i-th (1-based) argument.
func placeholder(d query.Dialect, i int) string {
	if isMySQL(d) {
		return "?"
	}
	return "$" + strconv.Itoa(i)
}

// placeholderList returns n comma-separated bind markers.
func placeholderList(d query.Dialect, n int) string {
	markers := make([]string, n)
	for i := range markers {
		markers[i] = placeholder(d, i+1)
	}
	return strings.Join(markers, ", ")
}

// EnsureTable creates the tracking table when it does not exist yet.
func (r *Runner) EnsureTable(ctx context.Context) error {
	if err := r.ready(); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, createTableSQL(r.opts.Dialect)); err != nil {
		return fmt.Errorf("vorm/migrate: create %s table: %w", tableName, err)
	}
	return nil
}

// Applied returns the tracking table rows in apply order (batch, then id).
func (r *Runner) Applied(ctx context.Context) ([]AppliedRow, error) {
	if err := r.ready(); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, migration, batch, COALESCE(checksum, ''), COALESCE(execution_time_ms, 0) FROM "+
			tableName+" ORDER BY batch, id")
	if err != nil {
		return nil, fmt.Errorf("vorm/migrate: list applied migrations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []AppliedRow
	for rows.Next() {
		var row AppliedRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Batch, &row.Checksum, &row.DurationMS); err != nil {
			return nil, fmt.Errorf("vorm/migrate: scan applied migration: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("vorm/migrate: list applied migrations: %w", err)
	}
	return out, nil
}

func (r *Runner) nextBatch(ctx context.Context) (int, error) {
	var batch int
	q := "SELECT COALESCE(MAX(batch), 0) + 1 FROM " + tableName
	if err := r.db.QueryRowContext(ctx, q).Scan(&batch); err != nil {
		return 0, fmt.Errorf("vorm/migrate: next batch number: %w", err)
	}
	return batch, nil
}

// insertRow records an applied migration. exec is the migration's transaction
// when there is one, so the row and the DDL land together.
func (r *Runner) insertRow(ctx context.Context, exec query.DB, name string, batch int, checksum string, durationMS int64) error {
	q := fmt.Sprintf("INSERT INTO %s (migration, batch, checksum, execution_time_ms) VALUES (%s)",
		tableName, placeholderList(r.opts.Dialect, 4))
	if _, err := exec.ExecContext(ctx, q, name, batch, checksum, durationMS); err != nil {
		return fmt.Errorf("vorm/migrate: record %s: %w", name, err)
	}
	return nil
}

func (r *Runner) deleteRow(ctx context.Context, id int64) error {
	q := fmt.Sprintf("DELETE FROM %s WHERE id = %s", tableName, placeholder(r.opts.Dialect, 1))
	if _, err := r.db.ExecContext(ctx, q, id); err != nil {
		return fmt.Errorf("vorm/migrate: delete migration row %d: %w", id, err)
	}
	return nil
}

func (r *Runner) updateChecksum(ctx context.Context, name, checksum string) error {
	q := fmt.Sprintf("UPDATE %s SET checksum = %s WHERE migration = %s",
		tableName, placeholder(r.opts.Dialect, 1), placeholder(r.opts.Dialect, 2))
	if _, err := r.db.ExecContext(ctx, q, checksum, name); err != nil {
		return fmt.Errorf("vorm/migrate: update checksum for %s: %w", name, err)
	}
	return nil
}
