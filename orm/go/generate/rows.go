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
		fmt.Fprintf(b, "\t%s %s\n", field, rewriteType(p.Type))
	}
	b.WriteString("}\n\n")
	return true
}

func fieldForColumn(ms ModelSpec, col string) (field, goType string) {
	bare := bareColumnName(col)
	for _, f := range ms.Fields {
		if f.Column == bare {
			return f.Name, f.Type
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
