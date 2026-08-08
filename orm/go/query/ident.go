package query

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// identifier: table, column, or table.column — no quotes, spaces, or SQL metacharacters.
var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`)

// SafeIdent validates a SQL identifier (prevents injection via column/table names).
func SafeIdent(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("vorm/query: empty identifier")
	}
	if name == "*" {
		return fmt.Errorf("vorm/query: refusing SELECT * (list columns explicitly)")
	}
	if strings.ContainsAny(name, ";'\"`--") || strings.Contains(name, "/*") {
		return fmt.Errorf("vorm/query: unsafe identifier %q", name)
	}
	if !identRe.MatchString(name) {
		// Allow simple aggregate forms used intentionally: count(id), etc.
		if safeExpr(name) {
			return nil
		}
		return fmt.Errorf("vorm/query: invalid identifier %q (use letters/digits/_ and optional table.column)", name)
	}
	return nil
}

var safeExprRe = regexp.MustCompile(`(?i)^(count|sum|avg|min|max)\(\s*[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?\s*\)$`)

func safeExpr(s string) bool {
	return safeExprRe.MatchString(strings.TrimSpace(s))
}

// QuoteIdent dialect-quotes a validated identifier (table or table.column).
func QuoteIdent(dialect Dialect, name string) (string, error) {
	if err := SafeIdent(name); err != nil {
		return "", err
	}
	if safeExpr(name) {
		return name, nil // already validated expression
	}
	parts := strings.Split(name, ".")
	for i, p := range parts {
		parts[i] = quoteOne(dialect, p)
	}
	return strings.Join(parts, "."), nil
}

func quoteOne(dialect Dialect, p string) string {
	// Escape embedded quote characters (defense in depth; SafeIdent already rejects them).
	if dialect == DialectMySQL {
		return "`" + strings.ReplaceAll(p, "`", "``") + "`"
	}
	return `"` + strings.ReplaceAll(p, `"`, `""`) + `"`
}

// SafeOnClause allows only identifier/operator join conditions (no bound user strings).
// Values must never appear here — use Where for filters.
func SafeOnClause(on string) error {
	on = strings.TrimSpace(on)
	if on == "" {
		return fmt.Errorf("vorm/query: empty JOIN ON")
	}
	if strings.ContainsAny(on, ";'`") || strings.Contains(on, "--") || strings.Contains(on, "/*") {
		return fmt.Errorf("vorm/query: unsafe JOIN ON %q", on)
	}
	// Only allow idents, dots, spaces, =<>!, and AND/OR parentheses
	for _, r := range on {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.' || r == ' ' ||
			r == '=' || r == '<' || r == '>' || r == '!' || r == '(' || r == ')' {
			continue
		}
		return fmt.Errorf("vorm/query: unsafe character in JOIN ON")
	}
	return nil
}

// RejectStarInList errors if any select column is *.
func RejectStarInList(cols []string) error {
	for _, c := range cols {
		if strings.TrimSpace(c) == "*" || strings.HasSuffix(strings.TrimSpace(c), ".*") {
			return fmt.Errorf("vorm/query: refusing SELECT *")
		}
		if err := SafeIdent(c); err != nil && !safeExpr(c) {
			return err
		}
	}
	return nil
}
