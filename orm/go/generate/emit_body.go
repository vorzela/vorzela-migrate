package generate

import (
	"fmt"
	"strings"

	"github.com/vorzela/vorm/query"
)

// stmt is the rendered form of a plan: the Go expressions holding the SQL text
// and the arguments to pass to the driver.
type stmt struct {
	SQL  string
	Args string
}

// call renders "ctx, <sql>[, args]" for a DB method call.
func (s stmt) call() string {
	if s.Args == "" {
		return "ctx, " + s.SQL
	}
	return "ctx, " + s.SQL + ", " + s.Args
}

func dialectExpr(d query.Dialect) string {
	if d == query.DialectMySQL {
		return "query.DialectMySQL"
	}
	return "query.DialectPostgres"
}

// emitStatement writes either a const SQL string with a fixed argument list —
// the fast path, one prepared statement per query — or the runtime assembly
// needed when an IN group is sized by a slice. prefix names the locals so two
// statements can live in one function.
func emitStatement(b *strings.Builder, constName, prefix string, p *plan, d query.Dialect) stmt {
	// Two statements can share one function, so locals carry the prefix too.
	for i, l := range p.locals {
		name := l.Name
		if prefix != "" {
			name = prefix + strings.ToUpper(l.Name[:1]) + l.Name[1:]
			for j := range p.binds {
				if p.binds[j].Expr == l.Name {
					p.binds[j].Expr = name
				}
			}
			p.locals[i].Name = name
		}
		fmt.Fprintf(b, "\t%s := %s\n", name, l.Expr)
	}
	if !p.dynamic {
		fmt.Fprintf(b, "\tconst %s = %s\n", constName, backquote(p.constSQL()))
		if len(p.binds) == 0 {
			return stmt{SQL: constName}
		}
		return stmt{SQL: constName, Args: strings.Join(p.argExprs(), ", ")}
	}

	argsVar, sbVar := "args", "sb"
	if prefix != "" {
		argsVar, sbVar = prefix+"Args", prefix+"SB"
	}

	fixed := 0
	var lens []string
	for _, bd := range p.binds {
		if bd.Slice {
			lens = append(lens, "len("+bd.Expr+")")
			continue
		}
		fixed++
	}
	capacity := strings.Join(lens, "+")
	switch {
	case capacity == "":
		capacity = fmt.Sprintf("%d", fixed)
	case fixed > 0:
		capacity = fmt.Sprintf("%d+%s", fixed, capacity)
	}

	fmt.Fprintf(b, "\t%s := make([]any, 0, %s)\n", argsVar, capacity)
	fmt.Fprintf(b, "\tvar %s strings.Builder\n", sbVar)
	fmt.Fprintf(b, "\t%s.Grow(%d)\n", sbVar, staticLen(p)+16*len(lens))

	bindIdx := 0
	for _, s := range p.segs {
		switch s.Kind {
		case segText:
			fmt.Fprintf(b, "\t%s.WriteString(%s)\n", sbVar, backquote(s.Text))
		case segPlaceholder:
			if !s.Inline {
				fmt.Fprintf(b, "\t%s.WriteString(query.Placeholder(%s, len(%s)+1))\n", sbVar, dialectExpr(d), argsVar)
			}
			fmt.Fprintf(b, "\t%s = append(%s, %s)\n", argsVar, argsVar, p.binds[bindIdx].Expr)
			bindIdx++
		case segIn:
			fn := "query.InClause"
			if s.Negated {
				fn = "query.NotInClause"
			}
			fmt.Fprintf(b, "\t%s.WriteString(%s(%s, %s, len(%s)+1, len(%s)))\n",
				sbVar, fn, dialectExpr(d), backquote(s.Column), argsVar, s.SliceExpr)
			fmt.Fprintf(b, "\tfor _, v := range %s {\n\t\t%s = append(%s, v)\n\t}\n", s.SliceExpr, argsVar, argsVar)
			bindIdx++
		}
	}
	return stmt{SQL: sbVar + ".String()", Args: argsVar + "..."}
}

func staticLen(p *plan) int {
	n := 0
	for _, s := range p.segs {
		n += len(s.Text)
	}
	return n
}

func emitSelectBody(b *strings.Builder, st StubFunc, ms ModelSpec, d query.Dialect, hasParams bool) error {
	// First/FirstOrFail read a single row, so stop the scan in the database.
	if st.Action != "Get" && st.Limit == 0 && st.LimitExpr == "" {
		st.Limit = 1
	}
	pl := newPlanner(st, ms, d, binder(st, hasParams))
	p, err := pl.selectPlan(resultCols(st, ms), selectRows)
	if err != nil {
		return err
	}
	rowT := rowTypeName(st.Name)
	s := emitStatement(b, constSQLName(st.Name), "", p, d)

	fmt.Fprintf(b, "\trows, err := db.QueryContext(%s)\n", s.call())
	b.WriteString("\tif err != nil {\n\t\treturn nil, err\n\t}\n\tdefer rows.Close()\n")

	switch st.Action {
	case "First", "FirstOrFail":
		b.WriteString("\tif !rows.Next() {\n\t\tif err := rows.Err(); err != nil {\n\t\t\treturn nil, err\n\t\t}\n")
		if st.Action == "FirstOrFail" {
			fmt.Fprintf(b, "\t\treturn nil, fmt.Errorf(\"%s: %%w\", query.ErrNoRows)\n\t}\n", st.Name)
		} else {
			b.WriteString("\t\treturn nil, nil\n\t}\n")
		}
		fmt.Fprintf(b, "\trow, err := scan%s(rows)\n", rowT)
		b.WriteString("\tif err != nil {\n\t\treturn nil, err\n\t}\n\treturn &row, nil\n")
	default:
		if st.Limit > 0 && st.Limit <= 8192 {
			fmt.Fprintf(b, "\tout := make([]%s, 0, %d)\n", rowT, st.Limit)
		} else {
			fmt.Fprintf(b, "\tvar out []%s\n", rowT)
		}
		b.WriteString("\tfor rows.Next() {\n")
		fmt.Fprintf(b, "\t\trow, err := scan%s(rows)\n", rowT)
		b.WriteString("\t\tif err != nil {\n\t\t\treturn nil, err\n\t\t}\n\t\tout = append(out, row)\n\t}\n")
		b.WriteString("\tif err := rows.Err(); err != nil {\n\t\treturn nil, err\n\t}\n\treturn out, nil\n")
	}
	return nil
}

func emitCountBody(b *strings.Builder, st StubFunc, ms ModelSpec, d query.Dialect, hasParams bool) error {
	pl := newPlanner(st, ms, d, binder(st, hasParams))
	mode := selectCount
	if st.Action == "Exists" {
		mode = selectExists
	}
	p, err := pl.selectPlan(nil, mode)
	if err != nil {
		return err
	}
	s := emitStatement(b, constSQLName(st.Name), "", p, d)

	if st.Action == "Exists" {
		b.WriteString("\tvar probe int\n")
		fmt.Fprintf(b, "\tif err := db.QueryRowContext(%s).Scan(&probe); err != nil {\n", s.call())
		// An empty result is the answer "false", not a failure.
		b.WriteString("\t\tif query.IsNotFound(err) {\n\t\t\treturn false, nil\n\t\t}\n\t\treturn false, err\n\t}\n")
		b.WriteString("\treturn true, nil\n")
		return nil
	}
	b.WriteString("\tvar total int64\n")
	fmt.Fprintf(b, "\tif err := db.QueryRowContext(%s).Scan(&total); err != nil {\n\t\treturn 0, err\n\t}\n", s.call())
	b.WriteString("\treturn total, nil\n")
	return nil
}

// emitPaginateBody runs the page query plus a COUNT over the same predicate.
func emitPaginateBody(b *strings.Builder, st StubFunc, ms ModelSpec, d query.Dialect, hasParams bool) error {
	bind := binder(st, hasParams)
	page, perPage := "1", "15"
	if st.PageExpr != "" {
		page = bind(st.PageExpr)
	}
	if st.PerPageExpr != "" {
		perPage = bind(st.PerPageExpr)
	}

	fmt.Fprintf(b, "\tperPage := %s\n", perPage)
	b.WriteString("\tif perPage <= 0 {\n\t\tperPage = 15\n\t}\n")
	fmt.Fprintf(b, "\tpage := %s\n", page)
	b.WriteString("\tif page <= 0 {\n\t\tpage = 1\n\t}\n")

	// LIMIT/OFFSET are bound, so every page reuses one prepared statement.
	pageStub := st
	pageStub.Limit, pageStub.Offset = 0, 0
	pageStub.LimitExpr, pageStub.OffsetExpr = "perPage", "(page-1)*perPage"
	localBind := passthroughBinder(bind, "perPage", "(page-1)*perPage")

	rowT := rowTypeName(st.Name)
	pl := newPlanner(pageStub, ms, d, localBind)
	p, err := pl.selectPlan(resultCols(st, ms), selectRows)
	if err != nil {
		return err
	}
	s := emitStatement(b, constSQLName(st.Name), "", p, d)
	fmt.Fprintf(b, "\trows, err := db.QueryContext(%s)\n", s.call())
	b.WriteString("\tif err != nil {\n\t\treturn nil, err\n\t}\n\tdefer rows.Close()\n")
	fmt.Fprintf(b, "\tout := make([]%s, 0, perPage)\n", rowT)
	b.WriteString("\tfor rows.Next() {\n")
	fmt.Fprintf(b, "\t\trow, err := scan%s(rows)\n", rowT)
	b.WriteString("\t\tif err != nil {\n\t\t\treturn nil, err\n\t\t}\n\t\tout = append(out, row)\n\t}\n")
	b.WriteString("\tif err := rows.Err(); err != nil {\n\t\treturn nil, err\n\t}\n")
	// Release the connection before the COUNT so both work on a single conn.
	b.WriteString("\trows.Close()\n")

	countStub := st
	countStub.Limit, countStub.Offset = 0, 0
	countStub.LimitExpr, countStub.OffsetExpr = "", ""
	countStub.Orders = nil
	cp, err := newPlanner(countStub, ms, d, bind).selectPlan(nil, selectCount)
	if err != nil {
		return err
	}
	cs := emitStatement(b, constSQLName(st.Name)+"Count", "count", cp, d)
	b.WriteString("\tvar total int64\n")
	fmt.Fprintf(b, "\tif err := db.QueryRowContext(%s).Scan(&total); err != nil {\n\t\treturn nil, err\n\t}\n", cs.call())

	b.WriteString("\tpages := 0\n")
	b.WriteString("\tif total > 0 {\n\t\tpages = int((total + int64(perPage) - 1) / int64(perPage))\n\t}\n")
	fmt.Fprintf(b, "\treturn &query.PageResult[%s]{\n", rowT)
	b.WriteString("\t\tData:     out,\n")
	b.WriteString("\t\tStyle:    string(query.PageOffset),\n")
	b.WriteString("\t\tPerPage:  perPage,\n")
	b.WriteString("\t\tPage:     page,\n")
	b.WriteString("\t\tPages:    pages,\n")
	b.WriteString("\t\tLastPage: pages,\n")
	b.WriteString("\t\tTotal:    &total,\n")
	b.WriteString("\t\tHasMore:  page < pages,\n")
	b.WriteString("\t}, nil\n")
	return nil
}

func emitAffectedBody(b *strings.Builder, s stmt) {
	fmt.Fprintf(b, "\tres, err := db.ExecContext(%s)\n", s.call())
	b.WriteString("\tif err != nil {\n\t\treturn 0, err\n\t}\n\treturn res.RowsAffected()\n")
}

func emitDeleteBody(b *strings.Builder, st StubFunc, ms ModelSpec, d query.Dialect, hasParams bool) error {
	pl := newPlanner(st, ms, d, binder(st, hasParams))
	p, err := pl.deletePlan()
	if err != nil {
		return err
	}
	emitAffectedBody(b, emitStatement(b, constSQLName(st.Name), "", p, d))
	return nil
}

func emitUpdateSetBody(b *strings.Builder, st StubFunc, ms ModelSpec, d query.Dialect, hasParams bool, sets []KVSpec, rawSets []string, applySoft bool) error {
	pl := newPlanner(st, ms, d, binder(st, hasParams))
	p, err := pl.updatePlan(sets, rawSets, applySoft)
	if err != nil {
		return err
	}
	emitAffectedBody(b, emitStatement(b, constSQLName(st.Name), "", p, d))
	return nil
}

// binder maps stub parameter names to fields on the generated Params struct.
func binder(st StubFunc, hasParams bool) func(string) string {
	if !hasParams {
		return func(s string) string { return s }
	}
	return func(expr string) string { return bindExpr(expr, st) }
}

// passthroughBinder leaves generated locals alone while still binding params.
func passthroughBinder(bind func(string) string, locals ...string) func(string) string {
	set := make(map[string]bool, len(locals))
	for _, l := range locals {
		set[l] = true
	}
	return func(expr string) string {
		if set[expr] {
			return expr
		}
		return bind(expr)
	}
}

// planIsDynamic reports whether a stub needs runtime SQL assembly, which
// decides whether the generated file imports strings.
func planIsDynamic(st StubFunc) bool {
	for _, w := range st.Wheres {
		if w.Kind == WhereInSlice {
			return true
		}
	}
	for _, w := range st.Havings {
		if w.Kind == WhereInSlice {
			return true
		}
	}
	return false
}
