package query

import (
	"context"
	"fmt"
	"strings"
)

// DB is the minimal executor interface (database/sql or pgx adapters).
type DB interface {
	QueryRowContext(ctx context.Context, query string, args ...any) Row
	QueryContext(ctx context.Context, query string, args ...any) (Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (Result, error)
}

// Row scans a single row.
type Row interface {
	Scan(dest ...any) error
}

// Rows iterates multiple rows.
type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

// Result is Exec result.
type Result interface {
	RowsAffected() (int64, error)
	LastInsertId() (int64, error)
}

// Dialect for placeholders and soft-delete SQL.
type Dialect string

const (
	DialectPostgres Dialect = "postgres"
	DialectMySQL    Dialect = "mysql"
)

// Builder is a fluent query IR that also compiles/runs performant SQL
// with explicit column lists (never SELECT *).
type Builder[T any] struct {
	meta       Meta
	dialect    Dialect
	selects    []string
	wheres     []pred
	havings    []pred
	orderBy    []order
	joins      []joinClause
	groupBy    []string
	distinct   bool
	distinctOn []string
	lock       string
	limit      int
	offset     int
	soft       bool // respect soft deletes when meta.SoftDeletes
}

type pred struct {
	col string
	op  string
	arg any
	or  bool // join with OR instead of AND
}

type order struct {
	col string
	dir string
}

func newBuilder[T any](meta Meta) *Builder[T] {
	return &Builder[T]{
		meta:    meta,
		dialect: DialectPostgres,
		selects: append([]string(nil), meta.Columns...),
		soft:    meta.SoftDeletes,
	}
}

// Dialect sets postgres (default) or mysql placeholders.
func (b *Builder[T]) Dialect(d Dialect) *Builder[T] {
	b.dialect = d
	return b
}

// Select narrows projected columns (must exist on Meta.Columns).
func (b *Builder[T]) Select(cols ...string) *Builder[T] {
	if len(cols) == 0 {
		return b
	}
	for _, c := range cols {
		if err := b.meta.RequireColumn(c); err != nil {
			b.wheres = append(b.wheres, pred{col: "__error__", op: "=", arg: err.Error()})
			return b
		}
	}
	b.selects = cols
	return b
}

// Where adds a predicate.
//
//	.Where("active", true)
//	.Where("age", ">", 18)
//	.Where("age", query.MoreThan(18))
//
// Column must exist on Meta.Columns; value type must match the model field when known.
func (b *Builder[T]) Where(args ...any) *Builder[T] {
	p, err := parseWhere(args...)
	if err != nil {
		b.wheres = append(b.wheres, pred{col: "__error__", op: "=", arg: err.Error()})
		return b
	}
	if err := b.validatePred(p); err != nil {
		b.wheres = append(b.wheres, pred{col: "__error__", op: "=", arg: err.Error()})
		return b
	}
	b.wheres = append(b.wheres, p)
	return b
}

func (b *Builder[T]) validatePred(p pred) error {
	if strings.HasPrefix(p.col, "__") {
		return nil
	}
	if err := b.meta.RequireColumn(p.col); err != nil {
		return err
	}
	op := strings.ToUpper(p.op)
	if op == "" {
		op = "="
	}
	if err := SafeOp(op); err != nil {
		return err
	}
	// ILIKE is Postgres-only; on MySQL/MariaDB map or reject at compile — reject here for safety.
	if (op == "ILIKE" || op == "NOT ILIKE") && b.dialect == DialectMySQL {
		return fmt.Errorf("vorm/query: %s is PostgreSQL-only; use LIKE on MySQL/MariaDB", op)
	}
	switch op {
	case "IS NULL", "IS NOT NULL":
		return nil
	case "IN":
		return b.meta.CheckColumnValue(p.col, Operator{Op: "IN", Value: p.arg})
	default:
		return b.meta.CheckColumnValue(p.col, p.arg)
	}
}

func parseWhere(args ...any) (pred, error) {
	switch len(args) {
	case 2:
		col, ok := args[0].(string)
		if !ok {
			return pred{}, fmt.Errorf("where: first arg must be column string")
		}
		if op, ok := args[1].(Operator); ok {
			return pred{col: col, op: op.Op, arg: op.Value}, nil
		}
		return pred{col: col, op: "=", arg: args[1]}, nil
	case 3:
		col, ok := args[0].(string)
		if !ok {
			return pred{}, fmt.Errorf("where: first arg must be column string")
		}
		op, ok := args[1].(string)
		if !ok {
			return pred{}, fmt.Errorf("where: second arg must be operator string")
		}
		return pred{col: col, op: strings.ToUpper(op), arg: args[2]}, nil
	default:
		return pred{}, fmt.Errorf("where: want 2 or 3 args, got %d", len(args))
	}
}

// OrderBy appends ORDER BY col [ASC|DESC]. Column must exist on Meta.
func (b *Builder[T]) OrderBy(col string, dir ...string) *Builder[T] {
	if err := b.meta.RequireColumn(col); err != nil {
		b.wheres = append(b.wheres, pred{col: "__error__", op: "=", arg: err.Error()})
		return b
	}
	d := "ASC"
	if len(dir) > 0 && dir[0] != "" {
		d = dir[0]
	}
	nd, err := SafeOrderDir(d)
	if err != nil {
		b.wheres = append(b.wheres, pred{col: "__error__", op: "=", arg: err.Error()})
		return b
	}
	b.orderBy = append(b.orderBy, order{col: col, dir: nd})
	return b
}

// Limit sets LIMIT.
func (b *Builder[T]) Limit(n int) *Builder[T] {
	b.limit = n
	return b
}

// Offset sets OFFSET.
func (b *Builder[T]) Offset(n int) *Builder[T] {
	b.offset = n
	return b
}

// WithTrashed includes soft-deleted rows.
func (b *Builder[T]) WithTrashed() *Builder[T] {
	b.soft = false
	return b
}

// ApplyFind merges TypeORM-style FindOptions into the builder.
func (b *Builder[T]) ApplyFind(opts FindOptions) *Builder[T] {
	if len(opts.Select) > 0 {
		b.Select(opts.Select...)
	}
	for col, v := range opts.Where {
		b.Where(col, v)
	}
	for col, dir := range opts.Order {
		switch d := dir.(type) {
		case Sort:
			b.OrderBy(col, string(d))
		case string:
			b.OrderBy(col, d)
		default:
			b.OrderBy(col, "ASC")
		}
	}
	if opts.Take > 0 {
		b.Limit(opts.Take)
	}
	if opts.Skip > 0 {
		b.Offset(opts.Skip)
	}
	return b
}

// Find is sugar for ApplyFind + Get.
func (b *Builder[T]) Find(ctx context.Context, db DB, opts FindOptions) ([]T, error) {
	return b.ApplyFind(opts).Get(ctx, db)
}

// Annotation documents the // vorm:query comment format for generators.
const Annotation = `// vorm:query name=<FuncName>`
