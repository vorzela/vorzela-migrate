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
	Entity     string   // Users
	TypeName   string   // User
	Package    string   // models
	ImportPath string   // optional full import if known
	Table      string
	Columns    []string
	Fields     []FieldSpec // db column order for Scan
	SoftDeletes bool
	PrimaryKey string
}

// FieldSpec is one scanned field.
type FieldSpec struct {
	Name   string // Go field: Active
	Column string // db: active
	Type   string // bool, string, int, …
}

// StubFunc is one // vorm:query function.
type StubFunc struct {
	Name       string
	File       string
	Params     []ParamSpec
	Results    []string // Go type strings
	Entity     string   // Users or models.Users
	ModelType  string   // User or models.User
	Action     string   // Get|First|Create|Update|SoftDelete|ForceDelete|""
	Selects    []string
	Wheres     []WhereSpec
	Orders     []OrderSpec
	Limit      int
	Offset     int
	Distinct   bool
	CreateVals []KVSpec
	UpdateVals []KVSpec
	SoftIDExpr string // SoftDelete/ForceDelete id expr
	Pending    bool   // body too complex to lower
	PendingWhy string
}

type ParamSpec struct {
	Name string
	Type string
}

type WhereSpec struct {
	Col     string
	Op      string
	ArgExpr string // Go source: true, 18, email
}

type OrderSpec struct {
	Col string
	Dir string
}

type KVSpec struct {
	Col string
	Expr string
}

func parseModelsDir(dir string) (map[string]ModelSpec, error) {
	out := map[string]ModelSpec{}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return out, nil
	}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
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
				modelType, meta, ok := parseModelCall(call)
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

func parseModelCall(call *ast.CallExpr) (typeName string, meta query.Meta, ok bool) {
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
	meta, ok = parseMetaLit(call.Args[0])
	return typeName, meta, ok
}

func parseMetaLit(expr ast.Expr) (query.Meta, bool) {
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
		key := exprString(kv.Key)
		switch key {
		case "Table":
			m.Table, _ = litString(kv.Value)
		case "PrimaryKey":
			m.PrimaryKey, _ = litString(kv.Value)
		case "SoftDeletes":
			m.SoftDeletes = exprString(kv.Value) == "true"
		case "Columns":
			m.Columns = litStringSlice(kv.Value)
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
	// Find return <call chain>
	var ret *ast.ReturnStmt
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		if r, ok := n.(*ast.ReturnStmt); ok && ret == nil {
			ret = r
		}
		return true
	})
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
	// Peel terminal action: Get/First/Create/Update/SoftDelete/ForceDelete
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	action := sel.Sel.Name
	switch action {
	case "Get", "First":
		st.Action = action
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
		return lowerEntityStart(sel.X, st, models)
	default:
		st.PendingWhy = "action " + action
		return false
	}
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
		name := sel.Sel.Name
		switch name {
		case "Where":
			if w, ok := parseWhereCall(n.Args); ok {
				st.Wheres = append([]WhereSpec{w}, st.Wheres...)
			} else {
				return false
			}
			return lowerBuilderChain(sel.X, st, models)
		case "OrderBy":
			if len(n.Args) >= 1 {
				col, _ := litString(n.Args[0])
				dir := "ASC"
				if len(n.Args) >= 2 {
					dir, _ = litString(n.Args[1])
					if dir == "" {
						dir = "ASC"
					}
				}
				st.Orders = append([]OrderSpec{{Col: col, Dir: strings.ToUpper(dir)}}, st.Orders...)
			}
			return lowerBuilderChain(sel.X, st, models)
		case "Limit":
			if len(n.Args) >= 1 {
				if lim, ok := litInt(n.Args[0]); ok {
					st.Limit = lim
				} else {
					st.PendingWhy = "dynamic Limit (use literal or runtime builder)"
					return false
				}
			}
			return lowerBuilderChain(sel.X, st, models)
		case "Offset":
			if len(n.Args) >= 1 {
				if off, ok := litInt(n.Args[0]); ok {
					st.Offset = off
				} else {
					st.PendingWhy = "dynamic Offset"
					return false
				}
			}
			return lowerBuilderChain(sel.X, st, models)
		case "Select":
			for _, a := range n.Args {
				if s, ok := litString(a); ok {
					st.Selects = append(st.Selects, s)
				}
			}
			return lowerBuilderChain(sel.X, st, models)
		case "Distinct":
			st.Distinct = true
			return lowerBuilderChain(sel.X, st, models)
		case "WhereSearch":
			return false // pending for now — complex
		case "WhereIn":
			return false
		case "New":
			return lowerBuilderChain(sel.X, st, models)
		default:
			st.PendingWhy = "method " + name
			return false
		}
	default:
		return false
	}
}

func parseWhereCall(args []ast.Expr) (WhereSpec, bool) {
	if len(args) == 2 {
		col, ok := litString(args[0])
		if !ok {
			return WhereSpec{}, false
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
