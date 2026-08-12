package generate

import (
	"fmt"
	"strings"
)

// resultCols returns the columns projected by a SELECT stub.
func resultCols(st StubFunc, ms ModelSpec) []string {
	if len(st.Selects) > 0 {
		return st.Selects
	}
	if len(ms.Columns) > 0 {
		return ms.Columns
	}
	return nil
}

// userParams returns stub params excluding ctx/db (and Beginner).
func userParams(st StubFunc) []ParamSpec {
	var out []ParamSpec
	for _, p := range st.Params {
		if p.Name == "ctx" || p.Name == "db" || p.Name == "tx" {
			continue
		}
		if p.Type == "context.Context" || strings.Contains(p.Type, "query.DB") || strings.Contains(p.Type, "query.Beginner") || strings.Contains(p.Type, "query.Tx") {
			continue
		}
		out = append(out, p)
	}
	return out
}

func rowTypeName(fn string) string { return fn + "Row" }

func paramsTypeName(fn string) string { return fn + "Params" }

func emitRowStruct(b *strings.Builder, st StubFunc, ms ModelSpec) {
	cols := resultCols(st, ms)
	if len(cols) == 0 {
		return
	}
	fmt.Fprintf(b, "// %s is the typed result shape for %s (sqlc-style; never SELECT *).\n", rowTypeName(st.Name), st.Name)
	fmt.Fprintf(b, "type %s struct {\n", rowTypeName(st.Name))
	for _, col := range cols {
		field, goType := fieldForColumn(ms, col)
		fmt.Fprintf(b, "\t%s %s `json:%q db:%q`\n", field, goType, bareColumnName(col), bareColumnName(col))
	}
	b.WriteString("}\n\n")
}

func emitParamsStruct(b *strings.Builder, st StubFunc) bool {
	ps := userParams(st)
	if len(ps) == 0 {
		return false
	}
	fmt.Fprintf(b, "// %s holds bound arguments for %s (type-safe; never string-concat into SQL).\n", paramsTypeName(st.Name), st.Name)
	fmt.Fprintf(b, "type %s struct {\n", paramsTypeName(st.Name))
	for _, p := range ps {
		name := p.Name
		if name == "" {
			name = "Arg"
		}
		field := exportIdent(name)
		fmt.Fprintf(b, "\t%s %s\n", field, paramFieldType(p.Type))
	}
	b.WriteString("}\n\n")
	return true
}

// paramFieldType turns a stub parameter type into a struct field type: a
// variadic `...T` becomes the slice it already is at the call site.
func paramFieldType(typ string) string {
	if rest, ok := strings.CutPrefix(strings.TrimSpace(typ), "..."); ok {
		return "[]" + rest
	}
	return rewriteType(typ)
}

// goPredeclared are the type names that never need package qualification.
var goPredeclared = map[string]bool{
	"bool": true, "string": true, "int": true, "int8": true, "int16": true,
	"int32": true, "int64": true, "uint": true, "uint8": true, "uint16": true,
	"uint32": true, "uint64": true, "uintptr": true, "byte": true, "rune": true,
	"float32": true, "float64": true, "complex64": true, "complex128": true,
	"any": true, "error": true,
}

// qualifyFieldType prefixes a model field's type when it is declared in the
// models package (a generated enum such as UserStatus), so the generated Row
// struct compiles from its own package.
func qualifyFieldType(ms ModelSpec, typ string) string {
	if ms.Package == "" || ms.Package == "gen" {
		return typ
	}
	base := typ
	var prefix string
	for {
		switch {
		case strings.HasPrefix(base, "*"):
			prefix, base = prefix+"*", base[1:]
		case strings.HasPrefix(base, "[]"):
			prefix, base = prefix+"[]", base[2:]
		default:
			if base == "" || strings.Contains(base, ".") || goPredeclared[base] {
				return typ
			}
			r := rune(base[0])
			if r < 'A' || r > 'Z' {
				return typ
			}
			return prefix + ms.Package + "." + base
		}
	}
}

func fieldForColumn(ms ModelSpec, col string) (field, goType string) {
	bare := bareColumnName(col)
	for _, f := range ms.Fields {
		if f.Column == bare {
			return f.Name, qualifyFieldType(ms, f.Type)
		}
	}
	switch bare {
	case "id":
		return "ID", "int64"
	case "created_at":
		return "CreatedAt", "time.Time"
	case "updated_at":
		return "UpdatedAt", "time.Time"
	case "deleted_at":
		return "DeletedAt", "*time.Time"
	default:
		return exportIdent(bare), "string"
	}
}

func bareColumnName(col string) string {
	if i := strings.LastIndex(col, "."); i >= 0 {
		return col[i+1:]
	}
	return col
}

func needsTimeImport(st StubFunc, ms ModelSpec) bool {
	for _, col := range resultCols(st, ms) {
		_, goType := fieldForColumn(ms, col)
		if strings.Contains(goType, "time.Time") {
			return true
		}
	}
	return false
}
