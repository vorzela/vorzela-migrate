package generate

import (
	"fmt"
	"strings"

	"github.com/vorzela/vorm/introspect"
	"github.com/vorzela/vorm/query"
)

// renderFunctions emits typed wrappers for stored routines. Routines whose
// signature cannot be expressed in Go (OUT parameters, record/table returns,
// trigger functions) are skipped and listed in a trailing comment.
func renderFunctions(opts SchemaOptions, mapper *TypeMapper) (string, bool) {
	var body strings.Builder
	var skipped []string
	imports := importSet{"context": true, "github.com/vorzela/vorm/query": true}
	taken := map[string]bool{}
	emitted := 0

	for _, fn := range opts.Schema.Functions {
		src, needs, err := renderFunction(opts.Dialect, mapper, fn, taken)
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("%s: %s", fn.Name, err))
			continue
		}
		for _, imp := range needs {
			imports.add(imp)
		}
		body.WriteString(src)
		emitted++
	}
	if emitted == 0 {
		return "", false
	}

	var b strings.Builder
	b.WriteString(doNotEdit)
	fmt.Fprintf(&b, "\npackage %s\n\n", opts.Package)
	b.WriteString(imports.block())
	b.WriteString(body.String())
	if len(skipped) > 0 {
		b.WriteString("\n// Not generated (signature has no direct Go form):\n")
		for _, s := range skipped {
			fmt.Fprintf(&b, "//   - %s\n", s)
		}
	}
	return b.String(), true
}

func renderFunction(dialect query.Dialect, mapper *TypeMapper, fn introspect.Function, taken map[string]bool) (string, []string, error) {
	switch strings.ToLower(fn.Language) {
	case "internal", "c":
		return "", nil, fmt.Errorf("built-in routine")
	}
	if k := strings.ToLower(fn.Kind); k == "aggregate" || k == "window" {
		return "", nil, fmt.Errorf("%s routines are not callable this way", k)
	}

	var imports []string
	var params, args []string
	for i, a := range fn.Args {
		switch strings.ToUpper(strings.TrimSpace(a.Mode)) {
		case "", "IN":
		default:
			return "", nil, fmt.Errorf("%s parameter %q", a.Mode, a.Name)
		}
		gt := mapper.Resolve(introspect.Column{DBType: a.DBType, FullType: a.DBType})
		if gt.Name == "any" {
			return "", nil, fmt.Errorf("parameter %q has unmapped type %s", a.Name, a.DBType)
		}
		imports = append(imports, gt.Import)
		name := paramName(a.Name, i)
		params = append(params, fmt.Sprintf("%s %s", name, gt.Name))
		args = append(args, name)
	}

	goName := uniqueName(GoName(fn.Name), taken)
	call := functionCallSQL(dialect, fn, len(args))
	isProcedure := strings.EqualFold(fn.Kind, "procedure")
	returns := strings.ToLower(strings.TrimSpace(fn.ReturnType))

	var b strings.Builder
	signature := strings.Join(params, ", ")
	if signature != "" {
		signature = ", " + signature
	}
	argList := ""
	if len(args) > 0 {
		argList = ", " + strings.Join(args, ", ")
	}

	switch {
	case isProcedure || returns == "void":
		fmt.Fprintf(&b, "\n// %s calls the database routine %s.\n", goName, fn.Name)
		fmt.Fprintf(&b, "func %s(ctx context.Context, db query.DB%s) error {\n", goName, signature)
		fmt.Fprintf(&b, "\tconst stmt = %s\n", backquote(call))
		fmt.Fprintf(&b, "\t_, err := db.ExecContext(ctx, stmt%s)\n\treturn err\n}\n", argList)
		return b.String(), imports, nil

	case returns == "", returns == "record", returns == "trigger", returns == "void",
		strings.HasPrefix(returns, "table("), strings.Contains(returns, "record"):
		return "", nil, fmt.Errorf("returns %s", fn.ReturnType)
	}

	gt := mapper.Resolve(introspect.Column{DBType: fn.ReturnType, FullType: fn.ReturnType})
	if gt.Name == "any" {
		return "", nil, fmt.Errorf("unmapped return type %s", fn.ReturnType)
	}
	imports = append(imports, gt.Import)

	if fn.ReturnsSet {
		if dialect == query.DialectMySQL {
			return "", nil, fmt.Errorf("set-returning routines are PostgreSQL-only")
		}
		fmt.Fprintf(&b, "\n// %s calls the set-returning database function %s.\n", goName, fn.Name)
		fmt.Fprintf(&b, "func %s(ctx context.Context, db query.DB%s) ([]%s, error) {\n", goName, signature, gt.Name)
		fmt.Fprintf(&b, "\tconst stmt = %s\n", backquote(call))
		fmt.Fprintf(&b, "\trows, err := db.QueryContext(ctx, stmt%s)\n", argList)
		b.WriteString("\tif err != nil {\n\t\treturn nil, err\n\t}\n\tdefer rows.Close()\n\n")
		fmt.Fprintf(&b, "\tvar out []%s\n\tfor rows.Next() {\n\t\tvar v %s\n", gt.Name, gt.Name)
		b.WriteString("\t\tif err := rows.Scan(&v); err != nil {\n\t\t\treturn nil, err\n\t\t}\n\t\tout = append(out, v)\n\t}\n\treturn out, rows.Err()\n}\n")
		return b.String(), imports, nil
	}

	fmt.Fprintf(&b, "\n// %s calls the database function %s.\n", goName, fn.Name)
	fmt.Fprintf(&b, "func %s(ctx context.Context, db query.DB%s) (%s, error) {\n", goName, signature, gt.Name)
	fmt.Fprintf(&b, "\tconst stmt = %s\n", backquote(call))
	fmt.Fprintf(&b, "\tvar out %s\n", gt.Name)
	fmt.Fprintf(&b, "\terr := db.QueryRowContext(ctx, stmt%s).Scan(&out)\n\treturn out, err\n}\n", argList)
	return b.String(), imports, nil
}

func functionCallSQL(dialect query.Dialect, fn introspect.Function, argc int) string {
	holders := make([]string, argc)
	for i := range holders {
		if dialect == query.DialectMySQL {
			holders[i] = "?"
		} else {
			holders[i] = fmt.Sprintf("$%d", i+1)
		}
	}
	name, _ := query.QuoteIdent(dialect, fn.Name)
	if name == "" {
		name = fn.Name
	}
	call := fmt.Sprintf("%s(%s)", name, strings.Join(holders, ", "))

	switch {
	case strings.EqualFold(fn.Kind, "procedure"):
		return "CALL " + call
	case fn.ReturnsSet:
		return "SELECT * FROM " + call
	default:
		return "SELECT " + call
	}
}

func paramName(name string, i int) string {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return fmt.Sprintf("arg%d", i+1)
	}
	parts := splitIdent(clean)
	if len(parts) == 0 {
		return fmt.Sprintf("arg%d", i+1)
	}
	var b strings.Builder
	b.WriteString(strings.ToLower(parts[0]))
	for _, p := range parts[1:] {
		b.WriteString(strings.ToUpper(p[:1]))
		if len(p) > 1 {
			b.WriteString(strings.ToLower(p[1:]))
		}
	}
	out := b.String()
	if isGoKeyword(out) {
		return out + "_"
	}
	return out
}

var goKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
	"ctx": true, "db": true, "out": true, "err": true, "stmt": true, "rows": true,
}

func isGoKeyword(s string) bool { return goKeywords[s] }

// backquote renders SQL as a Go raw string literal, falling back to a quoted
// literal when the statement itself contains a backquote.
func backquote(s string) string {
	if strings.Contains(s, "`") {
		return fmt.Sprintf("%q", s)
	}
	return "`" + s + "`"
}
