package query

import (
	"context"
	"fmt"
	"reflect"
	"sort"
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
		q, err := QuoteIdent(b.dialect, b.qualify(c))
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
			gparts[i], _ = QuoteIdent(b.dialect, b.qualify(g))
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
			qc, _ := QuoteIdent(b.dialect, b.qualify(o.col))
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

// qualify prefixes a bare column with the base table once joins are present,
// so `id` cannot become ambiguous against a joined table's `id`.
func (b *Builder[T]) qualify(col string) string {
	if len(b.joins) == 0 || col == "" {
		return col
	}
	if strings.ContainsAny(col, ".( ") {
		return col
	}
	return b.meta.Table + "." + col
}

func (b *Builder[T]) compileWhere(argStart int) (string, []any, error) {
	sqlText, args, err := b.compilePreds(b.wheres, argStart, true)
	if err != nil {
		return "", nil, err
	}
	if !b.meta.SoftDeletes || !b.soft {
		return sqlText, args, nil
	}
	del, err := QuoteIdent(b.dialect, b.qualify("deleted_at"))
	if err != nil {
		return "", nil, err
	}
	filter := del + " IS NULL"
	switch {
	case sqlText == "":
		return filter, args, nil
	case predsContainOr(b.wheres):
		// AND binds tighter than OR, so an unparenthesised OR group would let
		// soft-deleted rows through the filter.
		return "(" + sqlText + ") AND " + filter, args, nil
	default:
		return sqlText + " AND " + filter, args, nil
	}
}

func predsContainOr(preds []pred) bool {
	for i, p := range preds {
		if i > 0 && p.or {
			return true
		}
	}
	return false
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
				qc, err := QuoteIdent(b.dialect, b.qualify(p.col))
				if err != nil {
					return "", nil, err
				}
				p.col = qc
			}
			switch op {
			case "IS NULL", "IS NOT NULL":
				clause = fmt.Sprintf("%s %s", p.col, op)
			case "IN", "NOT IN":
				vals, ok := toAnySlice(p.arg)
				if !ok {
					return "", nil, fmt.Errorf("%s expects a slice of values", op)
				}
				switch {
				case len(vals) == 0 && op == "IN":
					clause = "1 = 0" // an empty set matches nothing
				case len(vals) == 0:
					clause = "1 = 1" // …and excludes nothing
				default:
					holders := make([]string, len(vals))
					for i, v := range vals {
						holders[i] = placeholder()
						args = append(args, v)
					}
					clause = fmt.Sprintf("%s %s (%s)", p.col, op, strings.Join(holders, ", "))
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

// Get executes SELECT and maps rows into []T. It uses a RowMapper from
// WithMapper when present (generated code path) and otherwise binds result
// columns to db-tagged struct fields with a cached plan.
func (b *Builder[T]) Get(ctx context.Context, db DB) ([]T, error) {
	sqlText, args, err := b.CompileSelect()
	if err != nil {
		return nil, err
	}

	mapper := rowMapperFromContext[T](ctx)
	if mapper == nil {
		mapper, err = structScanner[T](b.selects)
		if err != nil {
			return nil, err
		}
	}

	obs := observe(ctx, "select", b.meta.Table, sqlText, args)
	rows, err := db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, obs.done(ctx, 0, wrapErr("select", b.meta.Table, sqlText, len(args), err))
	}
	defer rows.Close()

	var out []T
	if b.limit > 0 && b.limit <= 8192 {
		out = make([]T, 0, b.limit)
	}
	for rows.Next() {
		row, err := mapper(rows)
		if err != nil {
			return nil, obs.done(ctx, len(out), wrapErr("scan", b.meta.Table, sqlText, len(args), err))
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, obs.done(ctx, len(out), wrapErr("select", b.meta.Table, sqlText, len(args), err))
	}
	_ = obs.done(ctx, len(out), nil)

	if err := loadRelations(ctx, db, b.with, out); err != nil {
		return nil, err
	}
	return out, nil
}

// First returns one row, or (nil, nil) when empty. Use FirstOrFail when absence
// should be an error.
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

// FirstOrFail returns ErrNoRows when nothing matches.
func (b *Builder[T]) FirstOrFail(ctx context.Context, db DB) (*T, error) {
	row, err := b.First(ctx, db)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, &Error{Op: "select", Table: b.meta.Table, Kind: KindNoRows, Err: ErrNoRows}
	}
	return row, nil
}

// Each streams rows through fn without buffering the whole result set.
// Return a non-nil error from fn to stop early. Relations are not eager-loaded
// here — batching needs the full set.
func (b *Builder[T]) Each(ctx context.Context, db DB, fn func(T) error) error {
	sqlText, args, err := b.CompileSelect()
	if err != nil {
		return err
	}
	mapper := rowMapperFromContext[T](ctx)
	if mapper == nil {
		mapper, err = structScanner[T](b.selects)
		if err != nil {
			return err
		}
	}

	obs := observe(ctx, "select", b.meta.Table, sqlText, args)
	rows, err := db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return obs.done(ctx, 0, wrapErr("select", b.meta.Table, sqlText, len(args), err))
	}
	defer rows.Close()

	n := 0
	for rows.Next() {
		row, err := mapper(rows)
		if err != nil {
			return obs.done(ctx, n, wrapErr("scan", b.meta.Table, sqlText, len(args), err))
		}
		n++
		if err := fn(row); err != nil {
			return obs.done(ctx, n, err)
		}
	}
	if err := rows.Err(); err != nil {
		return obs.done(ctx, n, wrapErr("select", b.meta.Table, sqlText, len(args), err))
	}
	return obs.done(ctx, n, nil)
}

// Exists reports whether any row matches (SELECT 1 … LIMIT 1).
func (b *Builder[T]) Exists(ctx context.Context, db DB) (bool, error) {
	if err := b.checkWhereErrors(); err != nil {
		return false, err
	}
	tableQ, err := QuoteIdent(b.dialect, b.meta.Table)
	if err != nil {
		return false, err
	}
	whereSQL, args, err := b.compileWhere(1)
	if err != nil {
		return false, err
	}
	var sb strings.Builder
	sb.WriteString("SELECT 1 FROM ")
	sb.WriteString(tableQ)
	sb.WriteString(compileJoinsQuoted(b.joins, b.dialect))
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
	sqlText := sb.String()

	obs := observe(ctx, "exists", b.meta.Table, sqlText, args)
	var n int
	// An empty result is the answer "false", not a failure; anything else is real.
	if err := db.QueryRowContext(ctx, sqlText, args...).Scan(&n); err != nil {
		if Classify(err) == KindNoRows {
			_ = obs.done(ctx, 0, nil)
			return false, nil
		}
		return false, obs.done(ctx, 0, wrapErr("exists", b.meta.Table, sqlText, len(args), err))
	}
	_ = obs.done(ctx, 1, nil)
	return true, nil
}

// Count executes COUNT. Uses COUNT(*) or COUNT(DISTINCT col) when Distinct is set.
func (b *Builder[T]) Count(ctx context.Context, db DB) (int64, error) {
	if err := b.checkWhereErrors(); err != nil {
		return 0, err
	}
	tableQ, err := QuoteIdent(b.dialect, b.meta.Table)
	if err != nil {
		return 0, err
	}
	whereSQL, args, err := b.compileWhere(1)
	if err != nil {
		return 0, err
	}
	var sb strings.Builder
	sb.WriteString("SELECT ")
	if b.distinct && len(b.selects) == 1 {
		colQ, err := QuoteIdent(b.dialect, b.selects[0])
		if err != nil {
			return 0, err
		}
		sb.WriteString("COUNT(DISTINCT ")
		sb.WriteString(colQ)
		sb.WriteString(")")
	} else {
		sb.WriteString("COUNT(*)")
	}
	sb.WriteString(" FROM ")
	sb.WriteString(tableQ)
	sb.WriteString(compileJoinsQuoted(b.joins, b.dialect))
	if whereSQL != "" {
		sb.WriteString(" WHERE ")
		sb.WriteString(whereSQL)
	}
	sqlText := sb.String()

	obs := observe(ctx, "count", b.meta.Table, sqlText, args)
	var n int64
	if err := db.QueryRowContext(ctx, sqlText, args...).Scan(&n); err != nil {
		return 0, obs.done(ctx, 0, wrapErr("count", b.meta.Table, sqlText, len(args), err))
	}
	_ = obs.done(ctx, 1, nil)
	return n, nil
}

// sortedKeys gives map-driven statements a stable column order so the same
// logical write always produces the same SQL text (prepared-statement friendly).
func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Create inserts an explicit column map and returns the new primary key.
func (b *Builder[T]) Create(ctx context.Context, db DB, values map[string]any) (int64, error) {
	if len(values) == 0 {
		return 0, validationErr("insert", b.meta.Table, "Create requires values")
	}
	keys := sortedKeys(values)
	for _, k := range keys {
		if err := b.meta.CheckColumnValue(k, values[k]); err != nil {
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
	cols := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys))
	holders := make([]string, 0, len(keys))
	for i, k := range keys {
		qc, err := QuoteIdent(b.dialect, k)
		if err != nil {
			return 0, err
		}
		cols = append(cols, qc)
		args = append(args, values[k])
		if b.dialect == DialectMySQL {
			holders = append(holders, "?")
		} else {
			holders = append(holders, fmt.Sprintf("$%d", i+1))
		}
	}
	sqlText := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		tableQ, strings.Join(cols, ", "), strings.Join(holders, ", "))

	if b.dialect != DialectMySQL {
		sqlText += " RETURNING " + pkQ
		obs := observe(ctx, "insert", b.meta.Table, sqlText, args)
		var id int64
		if err := db.QueryRowContext(ctx, sqlText, args...).Scan(&id); err != nil {
			return 0, obs.done(ctx, 0, wrapErr("insert", b.meta.Table, sqlText, len(args), err))
		}
		_ = obs.done(ctx, 1, nil)
		return id, nil
	}

	obs := observe(ctx, "insert", b.meta.Table, sqlText, args)
	res, err := db.ExecContext(ctx, sqlText, args...)
	if err != nil {
		return 0, obs.done(ctx, 0, wrapErr("insert", b.meta.Table, sqlText, len(args), err))
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, obs.done(ctx, 0, wrapErr("insert", b.meta.Table, sqlText, len(args), err))
	}
	_ = obs.done(ctx, 1, nil)
	return id, nil
}

// CreateMany inserts several rows in one statement. Every map must carry the
// same columns; the primary keys are not returned.
func (b *Builder[T]) CreateMany(ctx context.Context, db DB, rows []map[string]any) (int64, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	keys := sortedKeys(rows[0])
	if len(keys) == 0 {
		return 0, validationErr("insert", b.meta.Table, "CreateMany requires values")
	}
	tableQ, err := QuoteIdent(b.dialect, b.meta.Table)
	if err != nil {
		return 0, err
	}
	cols := make([]string, len(keys))
	for i, k := range keys {
		qc, err := QuoteIdent(b.dialect, k)
		if err != nil {
			return 0, err
		}
		cols[i] = qc
	}

	args := make([]any, 0, len(rows)*len(keys))
	tuples := make([]string, 0, len(rows))
	n := 1
	for _, row := range rows {
		if len(row) != len(keys) {
			return 0, validationErr("insert", b.meta.Table, "CreateMany rows must all share the same columns")
		}
		holders := make([]string, len(keys))
		for i, k := range keys {
			v, ok := row[k]
			if !ok {
				return 0, validationErr("insert", b.meta.Table, "CreateMany row is missing column %q", k)
			}
			if err := b.meta.CheckColumnValue(k, v); err != nil {
				return 0, err
			}
			if b.dialect == DialectMySQL {
				holders[i] = "?"
			} else {
				holders[i] = fmt.Sprintf("$%d", n)
				n++
			}
			args = append(args, v)
		}
		tuples = append(tuples, "("+strings.Join(holders, ", ")+")")
	}
	sqlText := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s",
		tableQ, strings.Join(cols, ", "), strings.Join(tuples, ", "))

	obs := observe(ctx, "insert", b.meta.Table, sqlText, args)
	res, err := db.ExecContext(ctx, sqlText, args...)
	if err != nil {
		return 0, obs.done(ctx, 0, wrapErr("insert", b.meta.Table, sqlText, len(args), err))
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, obs.done(ctx, 0, wrapErr("insert", b.meta.Table, sqlText, len(args), err))
	}
	_ = obs.done(ctx, int(affected), nil)
	return affected, nil
}

// Update sets columns for rows matching WHERE.
func (b *Builder[T]) Update(ctx context.Context, db DB, values map[string]any) (int64, error) {
	if len(values) == 0 {
		return 0, validationErr("update", b.meta.Table, "Update requires values")
	}
	keys := sortedKeys(values)
	for _, k := range keys {
		if err := b.meta.CheckColumnValue(k, values[k]); err != nil {
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
	sets := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys))
	for _, k := range keys {
		qc, err := QuoteIdent(b.dialect, k)
		if err != nil {
			return 0, err
		}
		sets = append(sets, fmt.Sprintf("%s = %s", qc, ph()))
		args = append(args, values[k])
	}
	whereSQL, whereArgs, err := b.compileWhere(n)
	if err != nil {
		return 0, err
	}
	sqlText := fmt.Sprintf("UPDATE %s SET %s", tableQ, strings.Join(sets, ", "))
	if whereSQL != "" {
		sqlText += " WHERE " + whereSQL
		args = append(args, whereArgs...)
	}

	obs := observe(ctx, "update", b.meta.Table, sqlText, args)
	res, err := db.ExecContext(ctx, sqlText, args...)
	if err != nil {
		return 0, obs.done(ctx, 0, wrapErr("update", b.meta.Table, sqlText, len(args), err))
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, obs.done(ctx, 0, wrapErr("update", b.meta.Table, sqlText, len(args), err))
	}
	_ = obs.done(ctx, int(affected), nil)
	return affected, nil
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
	sqlText := fmt.Sprintf("UPDATE %s SET %s = CURRENT_TIMESTAMP", tableQ, delCol)
	if whereSQL != "" {
		sqlText += " WHERE " + whereSQL
	}
	return b.execAffected(ctx, db, "soft_delete", sqlText, whereArgs)
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
	sqlText := "DELETE FROM " + tableQ
	if whereSQL != "" {
		sqlText += " WHERE " + whereSQL
	}
	return b.execAffected(ctx, db, "delete", sqlText, args)
}

// Restore clears deleted_at for soft-deleted rows matching WHERE.
func (b *Builder[T]) Restore(ctx context.Context, db DB) (int64, error) {
	if !b.meta.SoftDeletes {
		return 0, validationErr("restore", b.meta.Table, "table has no deleted_at column")
	}
	tableQ, err := QuoteIdent(b.dialect, b.meta.Table)
	if err != nil {
		return 0, err
	}
	delCol, err := QuoteIdent(b.dialect, "deleted_at")
	if err != nil {
		return 0, err
	}
	cp := *b
	cp.soft = false // target the rows that ARE deleted
	whereSQL, args, err := cp.compileWhere(1)
	if err != nil {
		return 0, err
	}
	sqlText := fmt.Sprintf("UPDATE %s SET %s = NULL", tableQ, delCol)
	if whereSQL != "" {
		sqlText += " WHERE " + whereSQL
	}
	return b.execAffected(ctx, db, "restore", sqlText, args)
}

func (b *Builder[T]) execAffected(ctx context.Context, db DB, op, sqlText string, args []any) (int64, error) {
	obs := observe(ctx, op, b.meta.Table, sqlText, args)
	res, err := db.ExecContext(ctx, sqlText, args...)
	if err != nil {
		return 0, obs.done(ctx, 0, wrapErr(op, b.meta.Table, sqlText, len(args), err))
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, obs.done(ctx, 0, wrapErr(op, b.meta.Table, sqlText, len(args), err))
	}
	_ = obs.done(ctx, int(affected), nil)
	return affected, nil
}
