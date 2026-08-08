package query

import (
	"fmt"
	"strings"
)

// WhereSearch adds a case-insensitive OR search across columns (ILIKE on Postgres, LIKE on MySQL).
// Empty term is a no-op.
//
//	Users.WhereSearch([]string{"name", "email"}, "ada").Limit(20)
func (b *Builder[T]) WhereSearch(columns []string, term string) *Builder[T] {
	term = strings.TrimSpace(term)
	if term == "" || len(columns) == 0 {
		return b
	}
	for _, c := range columns {
		if err := b.meta.RequireColumn(c); err != nil {
			b.wheres = append(b.wheres, pred{col: "__error__", op: "=", arg: err.Error()})
			return b
		}
	}
	op := "ILIKE"
	if b.dialect == DialectMySQL {
		op = "LIKE"
	}
	pattern := "%" + escapeLike(term) + "%"
	// Encode as a special pred consumed by compileWhere
	b.wheres = append(b.wheres, pred{
		col: "__search__",
		op:  op,
		arg: searchArg{Columns: columns, Pattern: pattern},
	})
	return b
}

// WhereRaw adds a raw SQL fragment with bound args only.
// SECURITY: fragment must not embed user input — pass values via args.
// Rejects semicolons and comment markers.
func (b *Builder[T]) WhereRaw(fragment string, args ...any) *Builder[T] {
	if strings.ContainsAny(fragment, ";") || strings.Contains(fragment, "--") || strings.Contains(fragment, "/*") {
		b.wheres = append(b.wheres, pred{col: "__error__", op: "=", arg: "vorm/query: WhereRaw rejects ; or SQL comments (injection risk)"})
		return b
	}
	b.wheres = append(b.wheres, pred{
		col: "__raw__",
		op:  fragment,
		arg: args,
	})
	return b
}

type searchArg struct {
	Columns []string
	Pattern string
}

func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// compileWhere is extended in exec.go — patch search/raw there via helpers used from compileWhere.
func appendSearchOrRaw(dialect Dialect, p pred, parts *[]string, args *[]any, placeholder func() string) error {
	switch p.col {
	case "__search__":
		sa, ok := p.arg.(searchArg)
		if !ok {
			return fmt.Errorf("invalid search arg")
		}
		ors := make([]string, len(sa.Columns))
		for i, c := range sa.Columns {
			qc, err := QuoteIdent(dialect, c)
			if err != nil {
				return err
			}
			// Values bound via placeholder — never concat the search term into SQL.
			ors[i] = fmt.Sprintf("%s %s %s", qc, p.op, placeholder())
			*args = append(*args, sa.Pattern)
		}
		*parts = append(*parts, "("+strings.Join(ors, " OR ")+")")
		return nil
	case "__raw__":
		*parts = append(*parts, "("+p.op+")")
		if rawArgs, ok := p.arg.([]any); ok {
			*args = append(*args, rawArgs...)
		}
		return nil
	default:
		return fmt.Errorf("not special")
	}
}
