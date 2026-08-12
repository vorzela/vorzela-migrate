package generate

import (
	"fmt"
	"strings"

	"github.com/vorzela/vorm/query"
)

// A plan is SQL split into literal text and the runtime-sized IN groups that
// cannot be known at generate time. When Dynamic is false the whole statement
// collapses to one const string, which is the fast path: no per-call building.
type plan struct {
	segs    []segment
	binds   []bind
	locals  []local
	dynamic bool
}

// local is a variable computed once before the statement runs, so a value bound
// to several placeholders is not recomputed per placeholder.
type local struct {
	Name string
	Expr string
}

func (p *plan) addLocal(name, expr string) string {
	p.locals = append(p.locals, local{Name: name, Expr: expr})
	return name
}

type segKind int

const (
	segText segKind = iota
	// segPlaceholder is a bind marker whose number is only known at run time,
	// because a preceding IN group shifted every later argument.
	segPlaceholder
	// segIn is an IN group sized by len(SliceExpr) at run time.
	segIn
)

type segment struct {
	Kind segKind
	Text string

	// Inline marks a placeholder whose marker is already part of the literal
	// text; only its argument still has to be appended at run time.
	Inline bool

	SliceExpr string // Go expression evaluating to a slice
	Column    string // pre-quoted column for the IN group
	Negated   bool
}

// bind is one argument in textual order. Slice binds append every element.
type bind struct {
	Expr  string
	Slice bool
}

func (p *plan) text(s string) {
	if s == "" {
		return
	}
	if n := len(p.segs); n > 0 && p.segs[n-1].Kind == segText {
		p.segs[n-1].Text += s
		return
	}
	p.segs = append(p.segs, segment{Kind: segText, Text: s})
}

func (p *plan) arg(expr string) {
	p.binds = append(p.binds, bind{Expr: expr})
}

// inGroup appends a runtime-sized IN group and the slice that fills it.
func (p *plan) inGroup(col, sliceExpr string, negated bool) {
	p.segs = append(p.segs, segment{Kind: segIn, SliceExpr: sliceExpr, Column: col, Negated: negated})
	p.binds = append(p.binds, bind{Expr: sliceExpr, Slice: true})
	p.dynamic = true
}

// placeholder writes the marker for the next argument. Before any dynamic
// segment the number is known at generate time and goes straight into the
// literal text; afterwards it depends on runtime lengths.
func (p *plan) placeholder(d query.Dialect) {
	if !p.dynamic {
		p.text(query.Placeholder(d, p.staticArgCount()+1))
		p.segs = append(p.segs, segment{Kind: segPlaceholder, Inline: true})
		return
	}
	p.segs = append(p.segs, segment{Kind: segPlaceholder})
}

// staticArgCount counts binds already emitted; only valid while !dynamic.
func (p *plan) staticArgCount() int { return len(p.binds) }

// constSQL returns the whole statement as one string. Only valid when !dynamic.
func (p *plan) constSQL() string {
	var sb strings.Builder
	for _, s := range p.segs {
		sb.WriteString(s.Text)
	}
	return sb.String()
}

// argExprs returns the Go expressions for a static plan, in bind order.
func (p *plan) argExprs() []string {
	out := make([]string, len(p.binds))
	for i, b := range p.binds {
		out[i] = b.Expr
	}
	return out
}

// planner compiles a lowered stub into a plan, mirroring query.Builder's SQL.
type planner struct {
	st      StubFunc
	ms      ModelSpec
	dialect query.Dialect
	bindFn  func(string) string
}

func newPlanner(st StubFunc, ms ModelSpec, d query.Dialect, bindFn func(string) string) *planner {
	if bindFn == nil {
		bindFn = func(s string) string { return s }
	}
	return &planner{st: st, ms: ms, dialect: d, bindFn: bindFn}
}

func (pl *planner) quote(name string) (string, error) {
	return query.QuoteIdent(pl.dialect, name)
}

// qualify mirrors Builder.qualify: once a join is present a bare column is
// prefixed with the base table so `id` cannot become ambiguous.
func (pl *planner) qualify(col string) string {
	if len(pl.st.Joins) == 0 || col == "" {
		return col
	}
	if strings.ContainsAny(col, ".( ") {
		return col
	}
	return pl.ms.Table + "." + col
}

func (pl *planner) quoteCol(col string) (string, error) {
	if err := query.SafeIdent(col); err != nil {
		return "", err
	}
	return pl.quote(pl.qualify(col))
}

// selectMode picks the projection: full rows, COUNT(*), or an existence probe.
type selectMode int

const (
	selectRows selectMode = iota
	selectCount
	selectExists
)

// selectPlan compiles the SELECT statement for Get/First/Count/Exists/Paginate.
func (pl *planner) selectPlan(cols []string, mode selectMode) (*plan, error) {
	if pl.ms.Table == "" {
		return nil, fmt.Errorf("cannot resolve model/entity %q — run vorm generate models", pl.st.Entity)
	}
	p := &plan{}
	p.text("SELECT ")
	switch {
	case mode == selectExists:
		p.text("1")
	case mode == selectCount:
		if pl.st.Distinct && len(pl.st.Selects) == 1 {
			q, err := pl.quoteCol(pl.st.Selects[0])
			if err != nil {
				return nil, err
			}
			p.text("COUNT(DISTINCT " + q + ")")
		} else {
			p.text("COUNT(*)")
		}
	default:
		if pl.st.Distinct {
			if len(pl.st.DistinctOn) > 0 && pl.dialect != query.DialectMySQL {
				parts := make([]string, len(pl.st.DistinctOn))
				for i, c := range pl.st.DistinctOn {
					q, err := pl.quoteCol(c)
					if err != nil {
						return nil, err
					}
					parts[i] = q
				}
				p.text("DISTINCT ON (" + strings.Join(parts, ", ") + ") ")
			} else {
				p.text("DISTINCT ")
			}
		}
		if len(cols) == 0 {
			return nil, fmt.Errorf("no columns selected (refusing SELECT *)")
		}
		if err := query.RejectStarInList(cols); err != nil {
			return nil, err
		}
		quoted := make([]string, len(cols))
		for i, c := range cols {
			q, err := pl.quoteCol(c)
			if err != nil {
				return nil, err
			}
			quoted[i] = q
		}
		p.text(strings.Join(quoted, ", "))
	}

	tableQ, err := pl.quote(pl.ms.Table)
	if err != nil {
		return nil, err
	}
	p.text(" FROM " + tableQ)
	if err := pl.writeJoins(p); err != nil {
		return nil, err
	}
	if err := pl.writeWhere(p); err != nil {
		return nil, err
	}
	if mode == selectExists {
		p.text(" LIMIT 1")
		if pl.st.Lock != "" {
			p.text(" " + pl.lockSQL())
		}
		return p, nil
	}
	if mode == selectCount {
		return p, nil
	}

	if len(pl.st.GroupBy) > 0 {
		parts := make([]string, len(pl.st.GroupBy))
		for i, g := range pl.st.GroupBy {
			q, err := pl.quoteCol(g)
			if err != nil {
				return nil, err
			}
			parts[i] = q
		}
		p.text(" GROUP BY " + strings.Join(parts, ", "))
	}
	if len(pl.st.Havings) > 0 {
		p.text(" HAVING ")
		if err := pl.writePreds(p, pl.st.Havings, false); err != nil {
			return nil, err
		}
	}
	if len(pl.st.Orders) > 0 {
		parts := make([]string, len(pl.st.Orders))
		for i, o := range pl.st.Orders {
			q, err := pl.quoteCol(o.Col)
			if err != nil {
				return nil, err
			}
			dir, err := query.SafeOrderDir(o.Dir)
			if err != nil {
				return nil, err
			}
			parts[i] = q + " " + dir
		}
		p.text(" ORDER BY " + strings.Join(parts, ", "))
	}
	if err := pl.writeLimitOffset(p); err != nil {
		return nil, err
	}
	if pl.st.Lock != "" {
		p.text(" " + pl.lockSQL())
	}
	return p, nil
}

func (pl *planner) lockSQL() string {
	if pl.st.Lock == "FOR SHARE" && pl.dialect == query.DialectMySQL {
		return "LOCK IN SHARE MODE"
	}
	return pl.st.Lock
}

func (pl *planner) writeLimitOffset(p *plan) error {
	switch {
	case pl.st.LimitExpr != "":
		p.text(" LIMIT ")
		p.placeholder(pl.dialect)
		p.arg(pl.bindFn(pl.st.LimitExpr))
	case pl.st.Limit > 0:
		p.text(fmt.Sprintf(" LIMIT %d", pl.st.Limit))
	}
	switch {
	case pl.st.OffsetExpr != "":
		p.text(" OFFSET ")
		p.placeholder(pl.dialect)
		p.arg(pl.bindFn(pl.st.OffsetExpr))
	case pl.st.Offset > 0:
		p.text(fmt.Sprintf(" OFFSET %d", pl.st.Offset))
	}
	return nil
}

func (pl *planner) writeJoins(p *plan) error {
	for _, j := range pl.st.Joins {
		if err := query.SafeIdent(j.Table); err != nil {
			return err
		}
		if err := query.SafeOnClause(j.On); err != nil {
			return err
		}
		tq, err := pl.quote(j.Table)
		if err != nil {
			return err
		}
		p.text(" " + j.Type + " " + tq + " ON " + j.On)
	}
	return nil
}

// writeWhere emits the WHERE clause including the soft-delete filter, matching
// Builder.compileWhere.
func (pl *planner) writeWhere(p *plan) error {
	soft := pl.ms.SoftDeletes && !pl.st.WithTrashed
	if len(pl.st.Wheres) == 0 && !soft {
		return nil
	}
	p.text(" WHERE ")
	if len(pl.st.Wheres) == 0 {
		return pl.writeSoftFilter(p)
	}
	// AND binds tighter than OR, so an unparenthesised OR group would let
	// soft-deleted rows through the filter.
	group := soft && wheresContainOr(pl.st.Wheres)
	if group {
		p.text("(")
	}
	if err := pl.writePreds(p, pl.st.Wheres, true); err != nil {
		return err
	}
	if group {
		p.text(")")
	}
	if !soft {
		return nil
	}
	p.text(" AND ")
	return pl.writeSoftFilter(p)
}

func (pl *planner) writeSoftFilter(p *plan) error {
	col, err := pl.quoteCol("deleted_at")
	if err != nil {
		return err
	}
	p.text(col + " IS NULL")
	return nil
}

func wheresContainOr(preds []WhereSpec) bool {
	for i, w := range preds {
		if i > 0 && w.Or {
			return true
		}
	}
	return false
}

func (pl *planner) writePreds(p *plan, preds []WhereSpec, allowOr bool) error {
	for i, w := range preds {
		if i > 0 {
			if w.Or && allowOr {
				p.text(" OR ")
			} else {
				p.text(" AND ")
			}
		}
		if err := pl.writePred(p, w); err != nil {
			return err
		}
	}
	return nil
}

func (pl *planner) writePred(p *plan, w WhereSpec) error {
	switch w.Kind {
	case WhereRaw:
		p.text("(" + w.Raw + ")")
		for _, a := range w.Args {
			p.placeholder(pl.dialect)
			p.arg(pl.bindFn(a))
		}
		return nil

	case WhereSearch:
		op := "ILIKE"
		if pl.dialect == query.DialectMySQL {
			op = "LIKE"
		}
		// Build the escaped pattern once, then bind it to every column.
		pattern := fmt.Sprintf("pattern%d", len(p.locals)+1)
		p.addLocal(pattern, "query.LikePattern("+pl.bindFn(w.ArgExpr)+")")
		p.text("(")
		for i, c := range w.Cols {
			if i > 0 {
				p.text(" OR ")
			}
			q, err := pl.quoteCol(c)
			if err != nil {
				return err
			}
			p.text(q + " " + op + " ")
			p.placeholder(pl.dialect)
			p.arg(pattern)
		}
		p.text(")")
		return nil
	}

	col, err := pl.quoteCol(w.Col)
	if err != nil {
		return err
	}

	switch w.Kind {
	case WhereNull:
		p.text(col + " IS NULL")
	case WhereNotNull:
		p.text(col + " IS NOT NULL")
	case WhereInList:
		if len(w.Args) == 0 {
			p.text("1 = 0")
			return nil
		}
		p.text(col + " IN (")
		for i, a := range w.Args {
			if i > 0 {
				p.text(", ")
			}
			p.placeholder(pl.dialect)
			p.arg(pl.bindFn(a))
		}
		p.text(")")
	case WhereInSlice:
		p.inGroup(col, pl.bindFn(w.ArgExpr), w.Negated)
	default:
		op := strings.ToUpper(strings.TrimSpace(w.Op))
		if op == "" {
			op = "="
		}
		if err := query.SafeOp(op); err != nil {
			return err
		}
		if (op == "ILIKE" || op == "NOT ILIKE") && pl.dialect == query.DialectMySQL {
			return fmt.Errorf("%s is PostgreSQL-only; use LIKE on MySQL/MariaDB", op)
		}
		p.text(col + " " + op + " ")
		p.placeholder(pl.dialect)
		p.arg(pl.bindFn(w.ArgExpr))
	}
	return nil
}

// deletePlan compiles DELETE FROM … WHERE …. A hard delete targets rows by
// predicate alone, so the soft-delete filter is never added.
func (pl *planner) deletePlan() (*plan, error) {
	tableQ, err := pl.quote(pl.ms.Table)
	if err != nil {
		return nil, err
	}
	p := &plan{}
	p.text("DELETE FROM " + tableQ)
	if err := pl.writeExplicitWhere(p); err != nil {
		return nil, err
	}
	return p, nil
}

// updatePlan compiles UPDATE … SET … WHERE … for Update/SoftDelete/Restore.
// applySoft keeps already soft-deleted rows out of the update, matching
// Builder.Update and Builder.SoftDelete.
func (pl *planner) updatePlan(sets []KVSpec, rawSets []string, applySoft bool) (*plan, error) {
	tableQ, err := pl.quote(pl.ms.Table)
	if err != nil {
		return nil, err
	}
	p := &plan{}
	p.text("UPDATE " + tableQ + " SET ")
	first := true
	for _, kv := range sets {
		if !first {
			p.text(", ")
		}
		first = false
		col, err := pl.quote(kv.Col)
		if err != nil {
			return nil, err
		}
		p.text(col + " = ")
		p.placeholder(pl.dialect)
		p.arg(pl.bindFn(kv.Expr))
	}
	for _, raw := range rawSets {
		if !first {
			p.text(", ")
		}
		first = false
		p.text(raw)
	}
	if applySoft {
		if err := pl.writeWhere(p); err != nil {
			return nil, err
		}
		return p, nil
	}
	if err := pl.writeExplicitWhere(p); err != nil {
		return nil, err
	}
	return p, nil
}

// writeExplicitWhere emits only the predicates the caller wrote.
func (pl *planner) writeExplicitWhere(p *plan) error {
	if len(pl.st.Wheres) == 0 {
		return nil
	}
	p.text(" WHERE ")
	return pl.writePreds(p, pl.st.Wheres, true)
}
