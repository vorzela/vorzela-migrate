package generate

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// TableSpec is extracted from Schema.Create Blueprint callbacks.
type TableSpec struct {
	Table       string
	Columns     []ColSpec
	SoftDeletes bool
	Timestamps  bool
	HasID       bool
	Enums       []EnumSpec
}

// ColSpec is one Blueprint column.
type ColSpec struct {
	Name     string
	Kind     string
	GoType   string
	EnumType string
	Nullable bool
}

// EnumSpec is an enum type used by the table.
type EnumSpec struct {
	Column string
	Type   string
	Values []string
}

// ParseSchemaDir walks schema migration Go files and extracts Create(...) Blueprints.
func ParseSchemaDir(dir string) ([]TableSpec, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil
	}
	var out []TableSpec
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		specs, err := ParseSchemaFile(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		out = append(out, specs...)
		return nil
	})
	return out, err
}

// ParseSchemaFile extracts TableSpecs from one migration Go file.
func ParseSchemaFile(path string) ([]TableSpec, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, err
	}
	var specs []TableSpec
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		table, build := matchCreateCall(call)
		if table == "" || build == nil {
			return true
		}
		spec := TableSpec{Table: table}
		for _, c := range outerCalls(build.Body) {
			applyBlueprintCall(c, &spec)
		}
		specs = append(specs, spec)
		return true
	})
	return specs, nil
}

func matchCreateCall(call *ast.CallExpr) (table string, build *ast.FuncLit) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil || sel.Sel.Name != "Create" {
		return "", nil
	}
	if len(call.Args) < 2 {
		return "", nil
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", nil
	}
	table = strings.Trim(lit.Value, `"`)
	fl, ok := call.Args[1].(*ast.FuncLit)
	if !ok {
		return "", nil
	}
	return table, fl
}

func outerCalls(body *ast.BlockStmt) []*ast.CallExpr {
	var all []*ast.CallExpr
	inner := map[*ast.CallExpr]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		all = append(all, call)
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if ic, ok := sel.X.(*ast.CallExpr); ok {
				inner[ic] = true
			}
		}
		return true
	})
	var out []*ast.CallExpr
	for _, c := range all {
		if !inner[c] {
			out = append(out, c)
		}
	}
	return out
}

func applyBlueprintCall(call *ast.CallExpr, spec *TableSpec) {
	methods, rootName, rootArgs := flattenChain(call)
	flags := map[string]bool{}
	for _, m := range methods {
		flags[m] = true
	}
	switch rootName {
	case "ID", "Id":
		spec.HasID = true
	case "Timestamps":
		spec.Timestamps = true
	case "SoftDeletes":
		spec.SoftDeletes = true
	case "String":
		addCol(spec, stringArg(rootArgs, 0), "string", "string", flags)
	case "Text":
		addCol(spec, stringArg(rootArgs, 0), "text", "string", flags)
		if len(spec.Columns) > 0 {
			spec.Columns[len(spec.Columns)-1].Nullable = true
		}
	case "Boolean":
		addCol(spec, stringArg(rootArgs, 0), "bool", "bool", flags)
	case "Integer":
		addCol(spec, stringArg(rootArgs, 0), "int", "int", flags)
	case "BigInteger", "ForeignID", "ForeignId":
		addCol(spec, stringArg(rootArgs, 0), "bigint", "int64", flags)
	case "UUID":
		addCol(spec, stringArg(rootArgs, 0), "uuid", "string", flags)
	case "Enum":
		col := stringArg(rootArgs, 0)
		if col == "" {
			return
		}
		vals := stringArgs(rootArgs[1:])
		typeName := exportIdent(spec.Table) + exportIdent(col)
		spec.Enums = append(spec.Enums, EnumSpec{Column: col, Type: typeName, Values: vals})
		addCol(spec, col, "enum", typeName, flags)
		spec.Columns[len(spec.Columns)-1].EnumType = typeName
	case "BelongsTo":
		col := stringArg(rootArgs, 0)
		addCol(spec, col, "bigint", "int64", flags)
	}
}

func addCol(spec *TableSpec, name, kind, goType string, flags map[string]bool) {
	if name == "" {
		return
	}
	c := ColSpec{Name: name, Kind: kind, GoType: goType}
	if flags["Nullable"] {
		c.Nullable = true
		if goType == "int" || goType == "int64" || goType == "bool" {
			c.GoType = "*" + goType
		}
	}
	spec.Columns = append(spec.Columns, c)
}

// flattenChain returns outermost→innermost method names; root is innermost column method.
func flattenChain(call *ast.CallExpr) (methods []string, rootName string, rootArgs []ast.Expr) {
	var chain []struct {
		name string
		args []ast.Expr
	}
	cur := call
	for cur != nil {
		sel, ok := cur.Fun.(*ast.SelectorExpr)
		if !ok {
			break
		}
		chain = append(chain, struct {
			name string
			args []ast.Expr
		}{sel.Sel.Name, cur.Args})
		next, ok := sel.X.(*ast.CallExpr)
		if !ok {
			break
		}
		cur = next
	}
	if len(chain) == 0 {
		return nil, "", nil
	}
	for _, c := range chain {
		methods = append(methods, c.name)
	}
	inner := chain[len(chain)-1]
	return methods, inner.name, inner.args
}

func stringArg(args []ast.Expr, i int) string {
	if i >= len(args) {
		return ""
	}
	lit, ok := args[i].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	return strings.Trim(lit.Value, `"`)
}

func stringArgs(args []ast.Expr) []string {
	var out []string
	for i := range args {
		if s := stringArg(args, i); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func exportIdent(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || unicode.IsSpace(r)
	})
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]))
		if len(p) > 1 {
			b.WriteString(p[1:])
		}
	}
	return b.String()
}
