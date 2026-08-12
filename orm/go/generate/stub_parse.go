package generate

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/vorzela/vorm/query"
)

// ModelSpec is Meta + Go type info parsed from models/*.go (generated, DO NOT EDIT).
type ModelSpec struct {
	Entity      string // Users
	TypeName    string // User
	Package     string // models
	ImportPath  string // optional full import if known
	Table       string
	Columns     []string
	Fields      []FieldSpec // db column order for Scan
	SoftDeletes bool
	PrimaryKey  string
}

// FieldSpec is one scanned field.
type FieldSpec struct {
	Name   string // Go field: Active
	Column string // db: active
	Type   string // bool, string, int, …
}

// StubFunc is one // vorm:query function.
type StubFunc struct {
	Name    string
	File    string
	Params  []ParamSpec
	Results []string // Go type strings

	Entity    string // Users or models.Users
	ModelType string // User or models.User

	// Action is the terminal builder call: Get, First, FirstOrFail, Count,
	// Exists, Paginate, Create, Update, Delete, SoftDelete, ForceDelete,
	// Restore. Empty means the chain could not be lowered.
	Action string

	Selects    []string
	Wheres     []WhereSpec
	Havings    []WhereSpec
	Joins      []JoinSpec
	GroupBy    []string
	Orders     []OrderSpec
	Limit      int
	Offset     int
	LimitExpr  string // dynamic LIMIT bound as a parameter
	OffsetExpr string
	Distinct   bool
	DistinctOn []string
	Lock       string // FOR UPDATE | FOR SHARE | FOR UPDATE SKIP LOCKED

	// WithTrashed drops the soft-delete filter for this query.
	WithTrashed bool
	// Relations requested via .With(...); they need model structs, so their
	// presence keeps a stub on the runtime builder.
	Relations []string

	CreateVals []KVSpec
	UpdateVals []KVSpec
	SoftIDExpr string // SoftDelete/ForceDelete id expr

	// Pagination arguments for the Paginate/OffsetPage terminal.
	PageExpr    string
	PerPageExpr string

	Pending    bool // body too complex to lower
	PendingWhy string
}

type ParamSpec struct {
	Name string
	Type string
}

// WhereKind distinguishes predicate shapes that compile differently.
type WhereKind string

const (
	// WhereValue is `col op $n`.
	WhereValue WhereKind = ""
	// WhereNull / WhereNotNull take no argument.
	WhereNull    WhereKind = "null"
	WhereNotNull WhereKind = "notnull"
	// WhereInList is IN over a fixed set of expressions known at generate time.
	WhereInList WhereKind = "in_list"
	// WhereInSlice is IN over a slice whose length is only known at run time.
	WhereInSlice WhereKind = "in_slice"
	// WhereSearch is a case-insensitive OR search across columns.
	WhereSearch WhereKind = "search"
	// WhereRaw is a caller-supplied fragment with bound arguments.
	WhereRaw WhereKind = "raw"
)

type WhereSpec struct {
	Kind    WhereKind
	Col     string
	Op      string
	ArgExpr string   // scalar value, search pattern, or slice expression
	Args    []string // IN list values / raw fragment arguments
	Cols    []string // search columns
	Raw     string   // raw SQL fragment
	Or      bool     // joined with OR instead of AND
	Negated bool     // NOT IN
}

type JoinSpec struct {
	Type  string // INNER JOIN | LEFT JOIN | RIGHT JOIN
	Table string
	On    string
}

type OrderSpec struct {
	Col string
	Dir string
}

type KVSpec struct {
	Col  string
	Expr string
}

// pkgConsts holds package-level string and []string declarations so a Meta
// literal can refer to them (generated models use UserTable / UserColumnList
// rather than inline literals).
type pkgConsts struct {
	strings map[string]string
	slices  map[string][]string
}

func newPkgConsts() *pkgConsts {
	return &pkgConsts{strings: map[string]string{}, slices: map[string][]string{}}
}

func (c *pkgConsts) collect(f *ast.File) {
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || (gd.Tok != token.CONST && gd.Tok != token.VAR) {
			continue
		}
		for _, sp := range gd.Specs {
			vs, ok := sp.(*ast.ValueSpec)
			if !ok || len(vs.Names) != 1 || len(vs.Values) != 1 {
				continue
			}
			name := vs.Names[0].Name
			if s, ok := litString(vs.Values[0]); ok {
				c.strings[name] = s
				continue
			}
			if vals := litStringSlice(vs.Values[0]); len(vals) > 0 {
				c.slices[name] = vals
			}
		}
	}
}

// resolveString reads a string literal or a package-level constant reference.
func (c *pkgConsts) resolveString(e ast.Expr) (string, bool) {
	if s, ok := litString(e); ok {
		return s, true
	}
	if id, ok := e.(*ast.Ident); ok && c != nil {
		s, ok := c.strings[id.Name]
		return s, ok
	}
	return "", false
}

func (c *pkgConsts) resolveStrings(e ast.Expr) []string {
	if vals := litStringSlice(e); len(vals) > 0 {
		return vals
	}
	if id, ok := e.(*ast.Ident); ok && c != nil {
		return c.slices[id.Name]
	}
	return nil
}

func parseModelsDir(dir string) (map[string]ModelSpec, error) {
	out := map[string]ModelSpec{}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return out, nil
	}

	// Two passes: package-level declarations first, since an entity in one file
	// can reference constants declared in another.
	consts := newPkgConsts()
	files := map[string]*ast.File{}
	if err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		files[path] = f
		consts.collect(f)
		return nil
	}); err != nil {
		return nil, err
	}

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		f := files[path]
		if f == nil {
			return nil
		}
		pkg := f.Name.Name
		// Find type decls + var Entity = query.Model[Type](query.Meta{...})
		types := map[string]*ast.StructType{}
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, sp := range gd.Specs {
				ts, ok := sp.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if st, ok := ts.Type.(*ast.StructType); ok {
					types[ts.Name.Name] = st
				}
			}
		}
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, sp := range gd.Specs {
				vs, ok := sp.(*ast.ValueSpec)
				if !ok || len(vs.Names) == 0 || len(vs.Values) == 0 {
					continue
				}
				entity := vs.Names[0].Name
				call, ok := vs.Values[0].(*ast.CallExpr)
				if !ok {
					continue
				}
				modelType, meta, ok := parseModelCall(call, consts)
				if !ok {
					continue
				}
				ms := ModelSpec{
					Entity:      entity,
					TypeName:    modelType,
					Package:     pkg,
					Table:       meta.Table,
					Columns:     meta.Columns,
					SoftDeletes: meta.SoftDeletes,
					PrimaryKey:  meta.PrimaryKey,
				}
				if ms.PrimaryKey == "" {
					ms.PrimaryKey = "id"
				}
				if st := types[modelType]; st != nil {
					ms.Fields = structFields(st)
				}
				out[entity] = ms
				if pkg != "" {
					out[pkg+"."+entity] = ms
				}
			}
		}
		return nil
	})
	return out, err
}

func parseModelCall(call *ast.CallExpr, consts *pkgConsts) (typeName string, meta query.Meta, ok bool) {
	var x ast.Expr
	var index ast.Expr
	switch fun := call.Fun.(type) {
	case *ast.IndexExpr:
		x, index = fun.X, fun.Index
	case *ast.IndexListExpr:
		if len(fun.Indices) != 1 {
			return "", query.Meta{}, false
		}
		x, index = fun.X, fun.Indices[0]
	default:
		return "", query.Meta{}, false
	}
	if !isSelector(x, "query", "Model") && !isIdent(x, "Model") {
		return "", query.Meta{}, false
	}
	typeName = exprString(index)
	if len(call.Args) == 0 {
		return "", query.Meta{}, false
	}
	meta, ok = parseMetaLit(call.Args[0], consts)
	return typeName, meta, ok
}

func parseMetaLit(expr ast.Expr, consts *pkgConsts) (query.Meta, bool) {
	cl, ok := expr.(*ast.CompositeLit)
	if !ok {
		return query.Meta{}, false
	}
	var m query.Meta
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		switch exprString(kv.Key) {
		case "Table":
			m.Table, _ = consts.resolveString(kv.Value)
		case "PrimaryKey":
			m.PrimaryKey, _ = consts.resolveString(kv.Value)
		case "SoftDeletes":
			m.SoftDeletes = exprString(kv.Value) == "true"
		case "Columns":
			m.Columns = consts.resolveStrings(kv.Value)
		}
	}
	return m, m.Table != "" && len(m.Columns) > 0
}

func structFields(st *ast.StructType) []FieldSpec {
	var out []FieldSpec
	if st.Fields == nil {
		return out
	}
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 {
			// embedded — expand known vorm.Model fields later via Columns
			continue
		}
		tag := ""
		if f.Tag != nil {
			tag, _ = strconv.Unquote(f.Tag.Value)
		}
		db := tagBetween(tag, "db:")
		if db == "" || db == "-" {
			continue
		}
		out = append(out, FieldSpec{
			Name:   f.Names[0].Name,
			Column: db,
			Type:   exprString(f.Type),
		})
	}
	return out
}

func tagBetween(tag, key string) string {
	// `json:"x" db:"y"`
	i := strings.Index(tag, key)
	if i < 0 {
		return ""
	}
	rest := tag[i+len(key):]
	if len(rest) == 0 || rest[0] != '"' {
		return ""
	}
	end := strings.Index(rest[1:], `"`)
	if end < 0 {
		return ""
	}
	val := rest[1 : 1+end]
	if comma := strings.Index(val, ","); comma >= 0 {
		val = val[:comma]
	}
	return val
}

func findAnnotatedStubs(queryDir string, models map[string]ModelSpec) ([]StubFunc, error) {
	var stubs []StubFunc
	seen := map[string]bool{}
	if _, err := os.Stat(queryDir); os.IsNotExist(err) {
		return nil, nil
	}
	err := filepath.WalkDir(queryDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		if strings.Contains(path, "_gen.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Name == nil || fd.Body == nil {
				continue
			}
			override, ok := vormQueryName(fd, f)
			if !ok {
				continue
			}
			name := fd.Name.Name
			if override != "" {
				name = override
			}
			if seen[name] {
				continue
			}
			seen[name] = true
			st := StubFunc{
				Name:    name,
				File:    path,
				Params:  parseParams(fd.Type),
				Results: parseResults(fd.Type),
			}
			lowerStubBody(fd, &st, models)
			stubs = append(stubs, st)
		}
		return nil
	})
	return stubs, err
}

// vormQueryName returns (nameOverride, true) if fd is marked // vorm:query.
func vormQueryName(fd *ast.FuncDecl, f *ast.File) (string, bool) {
	if fd.Doc != nil {
		for _, c := range fd.Doc.List {
			line := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(c.Text), "//"))
			if strings.HasPrefix(line, "vorm:query") {
				return parseQueryNameLine(line), true
			}
		}
	}
	var best *ast.CommentGroup
	for _, cg := range f.Comments {
		if cg.End() < fd.Pos() && (best == nil || cg.End() > best.End()) {
			best = cg
		}
	}
	if best == nil {
		return "", false
	}
	for _, c := range best.List {
		line := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(c.Text), "//"))
		if strings.HasPrefix(line, "vorm:query") {
			return parseQueryNameLine(line), true
		}
	}
	return "", false
}

func parseParams(ft *ast.FuncType) []ParamSpec {
	var out []ParamSpec
	if ft == nil || ft.Params == nil {
		return out
	}
	for _, f := range ft.Params.List {
		typ := exprString(f.Type)
		if len(f.Names) == 0 {
			out = append(out, ParamSpec{Type: typ})
			continue
		}
		for _, n := range f.Names {
			out = append(out, ParamSpec{Name: n.Name, Type: typ})
		}
	}
	return out
}

func parseResults(ft *ast.FuncType) []string {
	var out []string
	if ft == nil || ft.Results == nil {
		return out
	}
	for _, f := range ft.Results.List {
		typ := exprString(f.Type)
		n := 1
		if len(f.Names) > 1 {
			n = len(f.Names)
		}
		for i := 0; i < n; i++ {
			out = append(out, typ)
		}
	}
	return out
}

func lowerStubBody(fd *ast.FuncDecl, st *StubFunc, models map[string]ModelSpec) {
	// Only the stub's own return counts; returns inside nested closures (a
	// cursor extractor, say) belong to a different function.
	var ret *ast.ReturnStmt
	for _, stmt := range fd.Body.List {
		if r, ok := stmt.(*ast.ReturnStmt); ok {
			ret = r
			break
		}
	}
	if ret == nil || len(ret.Results) == 0 {
		st.Pending = true
		st.PendingWhy = "no return expression"
		return
	}
	call, ok := ret.Results[0].(*ast.CallExpr)
	if !ok {
		st.Pending = true
		st.PendingWhy = "return is not a call"
		return
	}
	if !lowerCallChain(call, st, models) {
		st.Pending = true
		if st.PendingWhy == "" {
			st.PendingWhy = "unsupported builder chain (Find/Paginate/Transaction/…)"
		}
	}
}

func lowerCallChain(call *ast.CallExpr, st *StubFunc, models map[string]ModelSpec) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	action := sel.Sel.Name
	switch action {
	case "Get", "First", "FirstOrFail", "Count", "Exists", "Delete", "Restore":
		st.Action = action
		return lowerBuilderChain(sel.X, st, models)
	case "OffsetPage":
		st.Action = "Paginate"
		if len(call.Args) >= 4 {
			st.PageExpr = exprString(call.Args[2])
			st.PerPageExpr = exprString(call.Args[3])
		}
		return lowerBuilderChain(sel.X, st, models)
	case "Paginate":
		st.Action = "Paginate"
		if len(call.Args) >= 3 {
			page, perPage, ok := parsePageRequest(call.Args[2])
			if !ok {
				st.PendingWhy = "Paginate needs a query.PageRequest literal (cursor pages stay on the builder)"
				return false
			}
			st.PageExpr, st.PerPageExpr = page, perPage
		}
		return lowerBuilderChain(sel.X, st, models)
	case "Create":
		st.Action = action
		if len(call.Args) >= 3 {
			st.CreateVals = parseMapLit(call.Args[2])
		}
		return lowerBuilderChain(sel.X, st, models) || lowerEntityStart(sel.X, st, models)
	case "Update":
		st.Action = action
		if len(call.Args) >= 3 {
			st.UpdateVals = parseMapLit(call.Args[2])
		}
		return lowerBuilderChain(sel.X, st, models)
	case "SoftDelete", "ForceDelete":
		st.Action = action
		if len(call.Args) >= 3 {
			st.SoftIDExpr = exprString(call.Args[2])
		}
		if len(call.Args) >= 3 {
			return lowerEntityStart(sel.X, st, models)
		}
		return lowerBuilderChain(sel.X, st, models)
	default:
		st.PendingWhy = "terminal call " + action + " is runtime-only"
		return false
	}
}

// parsePageRequest pulls Page/PerPage out of a query.PageRequest literal.
// Cursor pagination is stateful, so it stays on the runtime builder.
func parsePageRequest(expr ast.Expr) (page, perPage string, ok bool) {
	cl, isLit := expr.(*ast.CompositeLit)
	if !isLit {
		return "", "", false
	}
	for _, elt := range cl.Elts {
		kv, isKV := elt.(*ast.KeyValueExpr)
		if !isKV {
			continue
		}
		switch exprString(kv.Key) {
		case "Page":
			page = exprString(kv.Value)
		case "PerPage":
			perPage = exprString(kv.Value)
		case "Cursor", "OrderBy", "Desc":
			return "", "", false
		case "Style":
			if strings.Contains(exprString(kv.Value), "Cursor") {
				return "", "", false
			}
		}
	}
	return page, perPage, true
}

func lowerEntityStart(x ast.Expr, st *StubFunc, models map[string]ModelSpec) bool {
	name := exprString(x)
	st.Entity = name
	if ms, ok := models[name]; ok {
		st.ModelType = qualifyType(ms)
		return true
	}
	// Users without models map — try bare
	if id, ok := x.(*ast.Ident); ok {
		st.Entity = id.Name
		return true
	}
	if sel, ok := x.(*ast.SelectorExpr); ok {
		st.Entity = exprString(sel)
		return true
	}
	return false
}

func qualifyType(ms ModelSpec) string {
	if ms.Package != "" && ms.Package != "gen" {
		return ms.Package + "." + ms.TypeName
	}
	return ms.TypeName
}

func lowerBuilderChain(x ast.Expr, st *StubFunc, models map[string]ModelSpec) bool {
	switch n := x.(type) {
	case *ast.Ident:
		st.Entity = n.Name
		if ms, ok := models[n.Name]; ok {
			st.ModelType = qualifyType(ms)
		}
		return true
	case *ast.SelectorExpr:
		// models.Users or Users.New — if Sel is New/Distinct, continue on X
		switch n.Sel.Name {
		case "New", "Distinct":
			if n.Sel.Name == "Distinct" {
				st.Distinct = true
			}
			return lowerBuilderChain(n.X, st, models)
		default:
			// models.Users
			st.Entity = exprString(n)
			if ms, ok := models[st.Entity]; ok {
				st.ModelType = qualifyType(ms)
			} else if ms, ok := models[n.Sel.Name]; ok {
				st.ModelType = qualifyType(ms)
			}
			return true
		}
	case *ast.CallExpr:
		sel, ok := n.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		if !lowerBuilderCall(sel.Sel.Name, n, st) {
			return false
		}
		return lowerBuilderChain(sel.X, st, models)
	default:
		return false
	}
}

// lowerBuilderCall records one chained builder method. Calls arrive from the
// outermost inward, so clauses are prepended to keep source order.
func lowerBuilderCall(name string, call *ast.CallExpr, st *StubFunc) bool {
	args := call.Args
	switch name {
	case "New", "Dialect":
		return true

	case "Where", "OrWhere", "Having":
		w, ok := parseWhereCall(args)
		if !ok {
			st.PendingWhy = "Where form not supported (use column, [op], value)"
			return false
		}
		if name == "OrWhere" {
			w.Or = true
		}
		if name == "Having" {
			st.Havings = append([]WhereSpec{w}, st.Havings...)
			return true
		}
		st.Wheres = prependWhere(st.Wheres, w)
		return true

	case "WhereIn", "WhereNotIn":
		w, ok := parseWhereIn(args, call.Ellipsis.IsValid(), name == "WhereNotIn")
		if !ok {
			st.PendingWhy = name + " needs a column and values"
			return false
		}
		st.Wheres = prependWhere(st.Wheres, w)
		return true

	case "WhereNull", "WhereNotNull":
		if len(args) < 1 {
			return false
		}
		col, ok := litString(args[0])
		if !ok {
			st.PendingWhy = name + " needs a literal column name"
			return false
		}
		kind := WhereNull
		if name == "WhereNotNull" {
			kind = WhereNotNull
		}
		st.Wheres = prependWhere(st.Wheres, WhereSpec{Kind: kind, Col: col})
		return true

	case "WhereSearch":
		if len(args) != 2 {
			return false
		}
		cols := litStringSlice(args[0])
		if len(cols) == 0 {
			st.PendingWhy = "WhereSearch needs a literal column list"
			return false
		}
		st.Wheres = prependWhere(st.Wheres, WhereSpec{
			Kind:    WhereSearch,
			Cols:    cols,
			ArgExpr: exprString(args[1]),
		})
		return true

	case "WhereRaw":
		if len(args) < 1 {
			return false
		}
		frag, ok := litString(args[0])
		if !ok {
			st.PendingWhy = "WhereRaw needs a literal SQL fragment"
			return false
		}
		if strings.ContainsAny(frag, ";") || strings.Contains(frag, "--") || strings.Contains(frag, "/*") {
			st.PendingWhy = "WhereRaw rejects ; and SQL comments (injection risk)"
			return false
		}
		w := WhereSpec{Kind: WhereRaw, Raw: frag}
		for _, a := range args[1:] {
			w.Args = append(w.Args, exprString(a))
		}
		st.Wheres = prependWhere(st.Wheres, w)
		return true

	case "OrderBy", "OrderByDesc":
		if len(args) < 1 {
			return false
		}
		col, ok := litString(args[0])
		if !ok {
			st.PendingWhy = "OrderBy needs a literal column name"
			return false
		}
		dir := "ASC"
		if name == "OrderByDesc" {
			dir = "DESC"
		} else if len(args) >= 2 {
			if d, ok := litString(args[1]); ok && d != "" {
				dir = d
			}
		}
		st.Orders = append([]OrderSpec{{Col: col, Dir: strings.ToUpper(dir)}}, st.Orders...)
		return true

	case "GroupBy":
		var cols []string
		for _, a := range args {
			c, ok := litString(a)
			if !ok {
				st.PendingWhy = "GroupBy needs literal column names"
				return false
			}
			cols = append(cols, c)
		}
		st.GroupBy = append(cols, st.GroupBy...)
		return true

	case "Join", "InnerJoin", "LeftJoin", "RightJoin":
		if len(args) != 2 {
			return false
		}
		table, tok := litString(args[0])
		on, ook := litString(args[1])
		if !tok || !ook {
			st.PendingWhy = "joins need literal table and ON clause"
			return false
		}
		typ := "INNER JOIN"
		switch name {
		case "LeftJoin":
			typ = "LEFT JOIN"
		case "RightJoin":
			typ = "RIGHT JOIN"
		}
		st.Joins = append([]JoinSpec{{Type: typ, Table: table, On: on}}, st.Joins...)
		return true

	case "Limit", "Offset":
		if len(args) < 1 {
			return false
		}
		n, isLit := litInt(args[0])
		expr := exprString(args[0])
		if name == "Limit" {
			if isLit {
				st.Limit = n
			} else {
				st.LimitExpr = expr
			}
			return true
		}
		if isLit {
			st.Offset = n
		} else {
			st.OffsetExpr = expr
		}
		return true

	case "Select":
		for _, a := range args {
			s, ok := litString(a)
			if !ok {
				st.PendingWhy = "Select needs literal column names"
				return false
			}
			st.Selects = append(st.Selects, s)
		}
		return true

	case "Distinct":
		st.Distinct = true
		return true

	case "DistinctOn":
		st.Distinct = true
		for _, a := range args {
			s, ok := litString(a)
			if !ok {
				st.PendingWhy = "DistinctOn needs literal column names"
				return false
			}
			st.DistinctOn = append(st.DistinctOn, s)
		}
		return true

	case "WithTrashed":
		st.WithTrashed = true
		return true

	case "LockForUpdate", "ForUpdate":
		st.Lock = "FOR UPDATE"
		return true
	case "LockForShare":
		st.Lock = "FOR SHARE"
		return true
	case "SkipLocked":
		st.Lock = "FOR UPDATE SKIP LOCKED"
		return true

	case "With":
		for _, a := range args {
			if s, ok := litString(a); ok {
				st.Relations = append(st.Relations, s)
			}
		}
		st.PendingWhy = "eager loading needs model structs — call the builder directly for .With(...)"
		return false

	default:
		st.PendingWhy = "method " + name + " is runtime-only"
		return false
	}
}

func prependWhere(list []WhereSpec, w WhereSpec) []WhereSpec {
	return append([]WhereSpec{w}, list...)
}

// parseWhereIn handles WhereIn("col", a, b) and WhereIn("col", ids...). The
// spread form has a runtime length, so it compiles to a dynamic IN group.
func parseWhereIn(args []ast.Expr, spread, negated bool) (WhereSpec, bool) {
	if len(args) < 2 {
		return WhereSpec{}, false
	}
	col, ok := litString(args[0])
	if !ok {
		return WhereSpec{}, false
	}
	return parseInValues(col, args[1:], spread, negated)
}

func parseInValues(col string, vals []ast.Expr, spread, negated bool) (WhereSpec, bool) {
	if len(vals) == 0 {
		return WhereSpec{}, false
	}
	if spread && len(vals) == 1 {
		return WhereSpec{Kind: WhereInSlice, Col: col, ArgExpr: exprString(vals[0]), Negated: negated}, true
	}
	w := WhereSpec{Kind: WhereInList, Col: col, Negated: negated}
	for _, a := range vals {
		w.Args = append(w.Args, exprString(a))
	}
	return w, true
}

func parseWhereCall(args []ast.Expr) (WhereSpec, bool) {
	if len(args) == 2 {
		col, ok := litString(args[0])
		if !ok {
			return WhereSpec{}, false
		}
		if w, ok := parseOperatorHelper(col, args[1]); ok {
			return w, true
		}
		return WhereSpec{Col: col, Op: "=", ArgExpr: exprString(args[1])}, true
	}
	if len(args) == 3 {
		col, ok := litString(args[0])
		if !ok {
			return WhereSpec{}, false
		}
		op, ok := litString(args[1])
		if !ok {
			return WhereSpec{}, false
		}
		return WhereSpec{Col: col, Op: strings.ToUpper(op), ArgExpr: exprString(args[2])}, true
	}
	return WhereSpec{}, false
}

// operatorHelpers maps the query.Eq/MoreThan/… sugar to SQL operators so a
// stub written with helpers lowers to the same SQL as the explicit form.
var operatorHelpers = map[string]string{
	"Eq": "=", "Not": "<>",
	"MoreThan": ">", "MoreThanOrEqual": ">=",
	"LessThan": "<", "LessThanOrEqual": "<=",
	"Like": "LIKE", "ILike": "ILIKE",
}

// parseOperatorHelper recognises Where("age", query.MoreThan(18)) and friends.
func parseOperatorHelper(col string, arg ast.Expr) (WhereSpec, bool) {
	call, ok := arg.(*ast.CallExpr)
	if !ok {
		return WhereSpec{}, false
	}
	name := ""
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		name = fn.Sel.Name
	case *ast.Ident:
		name = fn.Name
	default:
		return WhereSpec{}, false
	}

	switch name {
	case "IsNull":
		return WhereSpec{Kind: WhereNull, Col: col}, true
	case "IsNotNull":
		return WhereSpec{Kind: WhereNotNull, Col: col}, true
	case "In":
		return parseInValues(col, call.Args, call.Ellipsis.IsValid(), false)
	case "NotIn":
		return parseInValues(col, call.Args, call.Ellipsis.IsValid(), true)
	}
	if op, ok := operatorHelpers[name]; ok && len(call.Args) == 1 {
		return WhereSpec{Col: col, Op: op, ArgExpr: exprString(call.Args[0])}, true
	}
	return WhereSpec{}, false
}

func parseMapLit(expr ast.Expr) []KVSpec {
	cl, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	var out []KVSpec
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		col, ok := litString(kv.Key)
		if !ok {
			continue
		}
		out = append(out, KVSpec{Col: col, Expr: exprString(kv.Value)})
	}
	return out
}

// --- AST helpers ---

func isSelector(e ast.Expr, pkg, name string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == pkg && sel.Sel.Name == name
}

func isIdent(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}

func exprString(e ast.Expr) string {
	if e == nil {
		return ""
	}
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return exprString(v.X) + "." + v.Sel.Name
	case *ast.StarExpr:
		return "*" + exprString(v.X)
	case *ast.ArrayType:
		return "[]" + exprString(v.Elt)
	case *ast.Ellipsis:
		return "..." + exprString(v.Elt)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.BasicLit:
		return v.Value
	case *ast.UnaryExpr:
		return v.Op.String() + exprString(v.X)
	case *ast.BinaryExpr:
		return exprString(v.X) + v.Op.String() + exprString(v.Y)
	case *ast.CallExpr:
		var args []string
		for _, a := range v.Args {
			args = append(args, exprString(a))
		}
		return exprString(v.Fun) + "(" + strings.Join(args, ", ") + ")"
	case *ast.IndexExpr:
		return exprString(v.X) + "[" + exprString(v.Index) + "]"
	case *ast.IndexListExpr:
		var idx []string
		for _, i := range v.Indices {
			idx = append(idx, exprString(i))
		}
		return exprString(v.X) + "[" + strings.Join(idx, ", ") + "]"
	case *ast.CompositeLit:
		return exprString(v.Type) + "{…}"
	case *ast.FuncType:
		return "func"
	default:
		return fmt.Sprintf("%T", e)
	}
}

func litStringSlice(e ast.Expr) []string {
	cl, ok := e.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	var out []string
	for _, elt := range cl.Elts {
		if s, ok := litString(elt); ok {
			out = append(out, s)
		}
	}
	return out
}

func litString(e ast.Expr) (string, bool) {
	bl, ok := e.(*ast.BasicLit)
	if !ok || bl.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(bl.Value)
	return s, err == nil
}

func litInt(e ast.Expr) (int, bool) {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind == token.INT {
			n, err := strconv.Atoi(v.Value)
			return n, err == nil
		}
	case *ast.Ident:
		// limit param — treat as 0 and use expr in emit for Limit? keep 0 → pending for dynamic
		return 0, false
	}
	return 0, false
}
