package query

import (
	"context"
	"fmt"
	"reflect"
	"strings"
)

// CompileSelect builds SELECT SQL + args. Always uses an explicit column list.
// Values are bound as parameters ($n / ?); identifiers are validated (no injection via names).
func (b *Builder[T]) CompileSelect() (sql string, args []any, err error) {
	if err := b.checkWhereErrors(); err != nil {
		return "", nil, err
	}
	cols := b.selects
	if len(cols) == 0 {
		return "", nil, fmt.Errorf("vorm/query: no columns selected (refusing SELECT *)")
	}
	if err := RejectStarInList(cols); err != nil {
		return "", nil, err
	}
	if err := SafeIdent(b.meta.Table); err != nil {
		return "", nil, err
	}
	for _, j := range b.joins {
		if err := SafeIdent(j.table); err != nil {
			return "", nil, err
		}
		if err := SafeOnClause(j.on); err != nil {
			return "", nil, err
		}
	}
	for _, g := range b.groupBy {
		if err := SafeIdent(g); err != nil {
			return "", nil, err
		}
	}
	for _, o := range b.orderBy {
		if err := SafeIdent(o.col); err != nil {
			return "", nil, err
		}
	}

	quotedCols := make([]string, len(cols))
	for i, c := range cols {
		q, err := QuoteIdent(b.dialect, c)
		if err != nil {
			return "", nil, err
		}
		quotedCols[i] = q
	}
	tableQ, err := QuoteIdent(b.dialect, b.meta.Table)
	if err != nil {
		return "", nil, err
	}

	var sb strings.Builder
	sb.WriteString("SELECT ")
	if b.distinct {
		if len(b.distinctOn) > 0 && b.dialect != DialectMySQL {
			sb.WriteString("DISTINCT ON (")
			onParts := make([]string, len(b.distinctOn))
			for i, c := range b.distinctOn {
				if err := SafeIdent(c); err != nil {
					return "", nil, err
				}
				onParts[i], err = QuoteIdent(b.dialect, c)
				if err != nil {
					return "", nil, err
				}
			}
			sb.WriteString(strings.Join(onParts, ", "))
			sb.WriteString(") ")
		} else {
			sb.WriteString("DISTINCT ")
		}
	}
	sb.WriteString(strings.Join(quotedCols, ", "))
	sb.WriteString(" FROM ")
	sb.WriteString(tableQ)
	sb.WriteString(compileJoinsQuoted(b.joins, b.dialect))

	whereSQL, whereArgs, err := b.compileWhere(1)
	if err != nil {
		return "", nil, err
	}
	if whereSQL != "" {
		sb.WriteString(" WHERE ")
		sb.WriteString(whereSQL)
	}
	args = append(args, whereArgs...)
	n := 1 + len(whereArgs)

	if len(b.groupBy) > 0 {
		sb.WriteString(" GROUP BY ")
		gparts := make([]string, len(b.groupBy))
		for i, g := range b.groupBy {
			gparts[i], _ = QuoteIdent(b.dialect, g)
		}
		sb.WriteString(strings.Join(gparts, ", "))
	}
	if len(b.havings) > 0 {
		havSQL, havArgs, err := b.compilePreds(b.havings, n, false)
		if err != nil {
			return "", nil, err
		}
		if havSQL != "" {
			sb.WriteString(" HAVING ")
			sb.WriteString(havSQL)
			args = append(args, havArgs...)
		}
	}
	if len(b.orderBy) > 0 {
		sb.WriteString(" ORDER BY ")
		parts := make([]string, len(b.orderBy))
		for i, o := range b.orderBy {
			qc, _ := QuoteIdent(b.dialect, o.col)
			parts[i] = qc + " " + o.dir
		}
		sb.WriteString(strings.Join(parts, ", "))
	}
	if b.limit > 0 {
		sb.WriteString(fmt.Sprintf(" LIMIT %d", b.limit))
	}
	if b.offset > 0 {
		sb.WriteString(fmt.Sprintf(" OFFSET %d", b.offset))
	}
	if b.lock != "" {
		sb.WriteString(" ")
		if b.lock == "FOR SHARE" && b.dialect == DialectMySQL {
			sb.WriteString("LOCK IN SHARE MODE")
		} else {
			sb.WriteString(b.lock)
		}
	}
	return sb.String(), args, nil
}

func compileJoinsQuoted(joins []joinClause, dialect Dialect) string {
	if len(joins) == 0 {
		return ""
	}
	var b strings.Builder
	for _, j := range joins {
		tq, _ := QuoteIdent(dialect, j.table)
		b.WriteString(" ")
		b.WriteString(j.typ)
		b.WriteString(" ")
		b.WriteString(tq)
		b.WriteString(" ON ")
		b.WriteString(j.on)
	}
	return b.String()
}

func (b *Builder[T]) compileWhere(argStart int) (string, []any, error) {
	preds := append([]pred(nil), b.wheres...)
	if b.meta.SoftDeletes && b.soft {
		col := "deleted_at"
		if len(b.joins) > 0 {
			col = b.meta.Table + ".deleted_at"
		}
		preds = append(preds, pred{col: col, op: "IS NULL", arg: nil})
	}
	return b.compilePreds(preds, argStart, true)
}

func (b *Builder[T]) compilePreds(preds []pred, argStart int, allowOr bool) (string, []any, error) {
	var parts []string
	var args []any
	n := argStart

	placeholder := func() string {
		if b.dialect == DialectMySQL {
			return "?"
		}
		s := fmt.Sprintf("$%d", n)
		n++
		return s
	}

	for i, p := range preds {
		if p.col == "__error__" {
			return "", nil, fmt.Errorf("%v", p.arg)
		}
		var clause string
		switch p.col {
		case "__search__", "__raw__":
			var tmpParts []string
			if err := appendSearchOrRaw(b.dialect, p, &tmpParts, &args, placeholder); err != nil {
				return "", nil, err
			}
			clause = tmpParts[0]
		case "__exists__":
			clause = "EXISTS (" + p.op + ")"
			if raw, ok := p.arg.([]any); ok {
				args = append(args, raw...)
			}
		case "__not_exists__":
			clause = "NOT EXISTS (" + p.op + ")"
			if raw, ok := p.arg.([]any); ok {
				args = append(args, raw...)
			}
		default:
			op := strings.ToUpper(p.op)
			if op == "" {
				op = "="
			}
			if err := SafeOp(op); err != nil {
				return "", nil, err
			}
			if p.col != "" && !strings.HasPrefix(p.col, "__") {
				if err := SafeIdent(p.col); err != nil {
					return "", nil, err
				}
				qc, err := QuoteIdent(b.dialect, p.col)
				if err != nil {
					return "", nil, err
				}
				p.col = qc
			}
			switch op {
			case "IS NULL", "IS NOT NULL":
				clause = fmt.Sprintf("%s %s", p.col, op)
			case "IN":
				vals, ok := toAnySlice(p.arg)
				if !ok {
					return "", nil, fmt.Errorf("IN expects a slice of values")
				}
				if len(vals) == 0 {
					clause = "1=0"
				} else {
					holders := make([]string, len(vals))
					for i, v := range vals {
						holders[i] = placeholder()
						args = append(args, v)
					}
					clause = fmt.Sprintf("%s IN (%s)", p.col, strings.Join(holders, ", "))
				}
			default:
				clause = fmt.Sprintf("%s %s %s", p.col, op, placeholder())
				args = append(args, p.arg)
			}
		}
		if i == 0 || !allowOr || !p.or {
			if i == 0 {
				parts = append(parts, clause)
			} else if p.or && allowOr {
				parts = append(parts, "OR "+clause)
			} else {
				parts = append(parts, "AND "+clause)
			}
		} else {
			parts = append(parts, "OR "+clause)
		}
	}
	return strings.Join(parts, " "), args, nil
}

func toAnySlice(v any) ([]any, bool) {
	if v == nil {
		return nil, false
	}
	if s, ok := v.([]any); ok {
		return s, true
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, false
	}
	out := make([]any, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		out[i] = rv.Index(i).Interface()
	}
	return out, true
}

func (b *Builder[T]) checkWhereErrors() error {
	for _, p := range append(b.wheres, b.havings...) {
		if p.col == "__error__" {
			return fmt.Errorf("%v", p.arg)
		}
	}
	return nil
}

// Get executes SELECT. Requires a RowMapper via WithMapper (or generated code).
func (b *Builder[T]) Get(ctx context.Context, db DB) ([]T, error) {
	sql, args, err := b.CompileSelect()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	mapper := rowMapperFromContext[T](ctx)
	if mapper == nil {
		return nil, fmt.Errorf("vorm/query: Get needs WithMapper[T] (or // vorm:query generate) — refusing untyped scans")
	}
	var out []T
	for rows.Next() {
		row, err := mapper(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// First returns one row, or (nil, nil) when empty.
func (b *Builder[T]) First(ctx context.Context, db DB) (*T, error) {
	cp := *b
	if cp.limit == 0 {
		cp.limit = 1
	}
	rows, err := cp.Get(ctx, db)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// Exists returns whether any row matches (SELECT 1 … LIMIT 1) — cheap existence check.
func (b *Builder[T]) Exists(ctx context.Context, db DB) (bool, error) {
	if err := b.checkWhereErrors(); err != nil {
		return false, err
	}
	whereSQL, args, err := b.compileWhere(1)
	if err != nil {
		return false, err
	}
	var sb strings.Builder
	sb.WriteString("SELECT 1 FROM ")
	sb.WriteString(b.meta.Table)
	sb.WriteString(compileJoins(b.joins))
	if whereSQL != "" {
		sb.WriteString(" WHERE ")
		sb.WriteString(whereSQL)
	}
	sb.WriteString(" LIMIT 1")
	if b.lock != "" {
		sb.WriteString(" ")
		if b.lock == "FOR SHARE" && b.dialect == DialectMySQL {
			sb.WriteString("LOCK IN SHARE MODE")
		} else {
			sb.WriteString(b.lock)
		}
	}
	row := db.QueryRowContext(ctx, sb.String(), args...)
	var n int
	if err := row.Scan(&n); err != nil {
		return false, nil // no rows / scan miss → not exists
	}
	return true, nil
}

// Count executes COUNT. Uses COUNT(*) or COUNT(DISTINCT col) when Distinct is set.
func (b *Builder[T]) Count(ctx context.Context, db DB) (int64, error) {
	if err := b.checkWhereErrors(); err != nil {
		return 0, err
	}
	whereSQL, args, err := b.compileWhere(1)
	if err != nil {
		return 0, err
	}
	var sb strings.Builder
	sb.WriteString("SELECT ")
	if b.distinct && len(b.selects) == 1 {
		sb.WriteString("COUNT(DISTINCT ")
		sb.WriteString(b.selects[0])
		sb.WriteString(")")
	} else {
		sb.WriteString("COUNT(*)")
	}
	sb.WriteString(" FROM ")
	sb.WriteString(b.meta.Table)
	sb.WriteString(compileJoins(b.joins))
	if whereSQL != "" {
		sb.WriteString(" WHERE ")
		sb.WriteString(whereSQL)
	}
	row := db.QueryRowContext(ctx, sb.String(), args...)
	var n int64
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// Create inserts an explicit column map.
func (b *Builder[T]) Create(ctx context.Context, db DB, values map[string]any) (int64, error) {
	if len(values) == 0 {
		return 0, fmt.Errorf("vorm/query: Create requires values")
	}
	for k, v := range values {
		if err := b.meta.CheckColumnValue(k, v); err != nil {
			return 0, err
		}
	}
	tableQ, err := QuoteIdent(b.dialect, b.meta.Table)
	if err != nil {
		return 0, err
	}
	pkQ, err := QuoteIdent(b.dialect, b.meta.PrimaryKey)
	if err != nil {
		return 0, err
	}
	cols := make([]string, 0, len(values))
	args := make([]any, 0, len(values))
	for k, v := range values {
		qc, err := QuoteIdent(b.dialect, k)
		if err != nil {
			return 0, err
		}
		cols = append(cols, qc)
		args = append(args, v)
	}
	holders := make([]string, len(args))
	for i := range args {
		if b.dialect == DialectMySQL {
			holders[i] = "?"
		} else {
			holders[i] = fmt.Sprintf("$%d", i+1)
		}
	}
	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		tableQ, strings.Join(cols, ", "), strings.Join(holders, ", "))
	if b.dialect != DialectMySQL {
		sql += " RETURNING " + pkQ
		row := db.QueryRowContext(ctx, sql, args...)
		var id int64
		if err := row.Scan(&id); err != nil {
			return 0, err
		}
		return id, nil
	}
	res, err := db.ExecContext(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Update sets columns for rows matching WHERE.
func (b *Builder[T]) Update(ctx context.Context, db DB, values map[string]any) (int64, error) {
	if len(values) == 0 {
		return 0, fmt.Errorf("vorm/query: Update requires values")
	}
	for k, v := range values {
		if err := b.meta.CheckColumnValue(k, v); err != nil {
			return 0, err
		}
	}
	tableQ, err := QuoteIdent(b.dialect, b.meta.Table)
	if err != nil {
		return 0, err
	}
	n := 1
	ph := func() string {
		if b.dialect == DialectMySQL {
			return "?"
		}
		s := fmt.Sprintf("$%d", n)
		n++
		return s
	}
	sets := make([]string, 0, len(values))
	args := make([]any, 0, len(values))
	for k, v := range values {
		qc, err := QuoteIdent(b.dialect, k)
		if err != nil {
			return 0, err
		}
		sets = append(sets, fmt.Sprintf("%s = %s", qc, ph()))
		args = append(args, v)
	}
	whereSQL, whereArgs, err := b.compileWhere(n)
	if err != nil {
		return 0, err
	}
	sql := fmt.Sprintf("UPDATE %s SET %s", tableQ, strings.Join(sets, ", "))
	if whereSQL != "" {
		sql += " WHERE " + whereSQL
		args = append(args, whereArgs...)
	}
	res, err := db.ExecContext(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Delete soft-deletes when Meta.SoftDeletes is set; otherwise hard-deletes.
func (b *Builder[T]) Delete(ctx context.Context, db DB) (int64, error) {
	if b.meta.SoftDeletes && b.soft {
		return b.SoftDelete(ctx, db)
	}
	return b.ForceDelete(ctx, db)
}

// SoftDelete sets deleted_at = CURRENT_TIMESTAMP.
func (b *Builder[T]) SoftDelete(ctx context.Context, db DB) (int64, error) {
	tableQ, err := QuoteIdent(b.dialect, b.meta.Table)
	if err != nil {
		return 0, err
	}
	delCol, err := QuoteIdent(b.dialect, "deleted_at")
	if err != nil {
		return 0, err
	}
	whereSQL, whereArgs, err := b.compileWhere(1)
	if err != nil {
		return 0, err
	}
	sql := fmt.Sprintf("UPDATE %s SET %s = CURRENT_TIMESTAMP", tableQ, delCol)
	if whereSQL != "" {
		sql += " WHERE " + whereSQL
	}
	res, err := db.ExecContext(ctx, sql, whereArgs...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ForceDelete always issues DELETE FROM.
func (b *Builder[T]) ForceDelete(ctx context.Context, db DB) (int64, error) {
	tableQ, err := QuoteIdent(b.dialect, b.meta.Table)
	if err != nil {
		return 0, err
	}
	cp := *b
	cp.soft = false
	cp.meta.SoftDeletes = false
	whereSQL, args, err := cp.compileWhere(1)
	if err != nil {
		return 0, err
	}
	sql := "DELETE FROM " + tableQ
	if whereSQL != "" {
		sql += " WHERE " + whereSQL
	}
	res, err := db.ExecContext(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
