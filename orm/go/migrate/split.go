package migrate

import (
	"strings"
	"unicode"

	"github.com/vorzela/vorm/query"
)

// SplitStatements splits a SQL section into individual statements on semicolons
// that sit outside string literals, quoted identifiers and PostgreSQL
// dollar-quoted bodies. A trailing fragment without a semicolon is returned as a
// statement, and comment-only lines are dropped.
//
// Dollar-quoted bodies ($$ ... $$ or $tag$ ... $tag$) are copied verbatim, which
// is what keeps CREATE FUNCTION bodies in one piece.
func SplitStatements(sql string) []string {
	var (
		out       []string
		cur       strings.Builder
		inSingle  bool
		inDouble  bool
		dollarTag string
	)

	flush := func() {
		stmt := strings.TrimSpace(cur.String())
		cur.Reset()
		if stmt == "" || isCommentOnly(stmt) {
			return
		}
		out = append(out, stmt)
	}

	for _, line := range strings.Split(sql, "\n") {
		quoted := inSingle || inDouble || dollarTag != ""
		if !quoted && strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}

		runes := []rune(line)
		for i := 0; i < len(runes); i++ {
			c := runes[i]
			switch {
			case dollarTag != "":
				if c == '$' {
					if tag, ok := dollarTagAt(runes, i); ok && tag == dollarTag {
						cur.WriteString(tag)
						i += len([]rune(tag)) - 1
						dollarTag = ""
						continue
					}
				}
			case inSingle:
				// Doubled quotes ('') close and immediately reopen the literal,
				// which lands on the same state as treating them as an escape.
				if c == '\'' {
					inSingle = false
				}
			case inDouble:
				if c == '"' {
					inDouble = false
				}
			default:
				switch c {
				case '\'':
					inSingle = true
				case '"':
					inDouble = true
				case '$':
					if tag, ok := dollarTagAt(runes, i); ok {
						cur.WriteString(tag)
						i += len([]rune(tag)) - 1
						dollarTag = tag
						continue
					}
				case ';':
					cur.WriteRune(c)
					flush()
					continue
				}
			}
			cur.WriteRune(c)
		}
		cur.WriteRune('\n')
	}
	flush()
	return out
}

// dollarTagAt reports the dollar-quote tag opening at runes[i], including both
// delimiters, e.g. "$$" or "$body$". Positional parameters such as $1 are not
// tags, so they are left alone.
func dollarTagAt(runes []rune, i int) (string, bool) {
	if runes[i] != '$' {
		return "", false
	}
	for j := i + 1; j < len(runes); j++ {
		c := runes[j]
		if c == '$' {
			return string(runes[i : j+1]), true
		}
		if c == '_' || unicode.IsLetter(c) {
			continue
		}
		if j > i+1 && unicode.IsDigit(c) {
			continue
		}
		return "", false
	}
	return "", false
}

// isCommentOnly reports whether a fragment carries no executable SQL, which
// happens for the tail left behind by a trailing "-- note" after a semicolon.
func isCommentOnly(stmt string) bool {
	for _, line := range strings.Split(stmt, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed == ";" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		return false
	}
	return true
}

// requiresNoTransaction reports whether stmts must run outside a transaction.
// PostgreSQL rejects CREATE/DROP INDEX CONCURRENTLY inside a transaction block,
// so such a migration is applied statement by statement instead.
func requiresNoTransaction(stmts []string, d query.Dialect) bool {
	if isMySQL(d) {
		return false
	}
	for _, stmt := range stmts {
		if strings.Contains(normalizeSpace(strings.ToUpper(stmt)), "INDEX CONCURRENTLY") {
			return true
		}
	}
	return false
}

func normalizeSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
