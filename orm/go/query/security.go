package query

import (
	"fmt"
	"strings"
)

// allowedOps is the only WHERE/HAVING operators vorm will emit (prevents op-injection).
var allowedOps = map[string]bool{
	"=": true, "<>": true, "!=": true,
	">": true, ">=": true, "<": true, "<=": true,
	"LIKE": true, "ILIKE": true, "NOT LIKE": true, "NOT ILIKE": true,
	"IN": true, "NOT IN": true, "IS NULL": true, "IS NOT NULL": true,
}

// SafeOp validates a comparison operator (whitelist).
func SafeOp(op string) error {
	op = strings.ToUpper(strings.TrimSpace(op))
	if op == "" {
		return fmt.Errorf("vorm/query: empty operator")
	}
	if !allowedOps[op] {
		return fmt.Errorf("vorm/query: operator %q not allowed (injection risk)", op)
	}
	return nil
}

// NormalizeOp returns the canonical uppercase operator or error.
func NormalizeOp(op string) (string, error) {
	op = strings.ToUpper(strings.TrimSpace(op))
	if err := SafeOp(op); err != nil {
		return "", err
	}
	return op, nil
}

// SafeOrderDir allows only ASC or DESC.
func SafeOrderDir(dir string) (string, error) {
	d := strings.ToUpper(strings.TrimSpace(dir))
	if d == "" {
		d = "ASC"
	}
	if d != "ASC" && d != "DESC" {
		return "", fmt.Errorf("vorm/query: ORDER BY direction must be ASC or DESC, got %q", dir)
	}
	return d, nil
}

// DialectPlaceholders documents binding style (never concatenate user values).
func DialectPlaceholders(d Dialect) string {
	if d == DialectMySQL {
		return "?"
	}
	return "$n"
}
