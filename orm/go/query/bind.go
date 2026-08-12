package query

import (
	"strconv"
	"strings"
)

// pgHolders caches "$1".."$64" so generated code that renders placeholders per
// call does not allocate for the common case.
var pgHolders = func() [65]string {
	var a [65]string
	for i := 1; i < len(a); i++ {
		a[i] = "$" + strconv.Itoa(i)
	}
	return a
}()

// Placeholder returns the bind marker for the n-th argument (1-based): "$n" on
// PostgreSQL, "?" on MySQL/MariaDB. Generated code uses it when the statement
// shape depends on runtime slice lengths.
func Placeholder(d Dialect, n int) string {
	if d == DialectMySQL {
		return "?"
	}
	if n > 0 && n < len(pgHolders) {
		return pgHolders[n]
	}
	return "$" + strconv.Itoa(n)
}

// InPlaceholders renders n comma-separated markers starting at argument index
// start (1-based): "$3, $4, $5" or "?, ?, ?". It returns "" for n <= 0.
func InPlaceholders(d Dialect, start, n int) string {
	if n <= 0 {
		return ""
	}
	if d == DialectMySQL {
		switch n {
		case 1:
			return "?"
		case 2:
			return "?, ?"
		case 3:
			return "?, ?, ?"
		}
		return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
	}
	var sb strings.Builder
	sb.Grow(n * 5)
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(Placeholder(d, start+i))
	}
	return sb.String()
}

// InClause renders `expr IN ($3, $4)` for n bound values. With no values it
// renders a never-true predicate: `IN ()` is a syntax error in both dialects,
// and an empty set can never match.
//
// expr must already be quoted — pass the output of QuoteIdent, not user input.
func InClause(d Dialect, expr string, start, n int) string {
	if n <= 0 {
		return "1 = 0"
	}
	return expr + " IN (" + InPlaceholders(d, start, n) + ")"
}

// NotInClause is InClause for NOT IN. With no values every row qualifies, so it
// renders an always-true predicate.
func NotInClause(d Dialect, expr string, start, n int) string {
	if n <= 0 {
		return "1 = 1"
	}
	return expr + " NOT IN (" + InPlaceholders(d, start, n) + ")"
}

// LikePattern builds a contains-search pattern for LIKE/ILIKE, escaping the
// wildcards in term so user input cannot widen the match.
//
//	WHERE "name" ILIKE query.LikePattern(arg.Q)
func LikePattern(term string) string {
	return "%" + escapeLike(strings.TrimSpace(term)) + "%"
}

// PrefixPattern builds a prefix-search pattern ("term%"), which can use a
// standard B-tree index where LikePattern cannot.
func PrefixPattern(term string) string {
	return escapeLike(strings.TrimSpace(term)) + "%"
}
