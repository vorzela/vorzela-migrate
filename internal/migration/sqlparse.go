package migration

import (
	"fmt"
	"strings"
)

// SplitSQLStatements splits a SQL script into individual executable
// statements. It understands:
//
//   - single-quoted strings (including ” and backslash escapes)
//   - double-quoted identifiers and backtick identifiers
//   - -- line comments and /* block comments */
//   - PostgreSQL dollar-quoted bodies ($$ … $$ and $tag$ … $tag$)
//
// A semicolon only terminates a statement in normal (unquoted) state. Empty
// statements and statements consisting only of comments are discarded.
//
// It returns an error for unterminated strings/comments/dollar quotes so the
// safety checker can report a useful problem instead of sending broken SQL to
// the database.
func SplitSQLStatements(script string) ([]string, error) {
	var (
		statements []string
		current    strings.Builder
		state      = sqlStateNormal
		dollarTag  string
	)

	flush := func() {
		stmt := strings.TrimSpace(current.String())
		current.Reset()
		if isOnlyWhitespaceOrComments(stmt) {
			return
		}
		statements = append(statements, stmt)
	}

	for i := 0; i < len(script); i++ {
		ch := script[i]

		switch state {
		case sqlStateNormal:
			switch {
			case ch == '\'':
				state = sqlStateSingle
				current.WriteByte(ch)
			case ch == '"':
				state = sqlStateDouble
				current.WriteByte(ch)
			case ch == '`':
				state = sqlStateBacktick
				current.WriteByte(ch)
			case ch == '-' && i+1 < len(script) && script[i+1] == '-':
				state = sqlStateLineComment
				current.WriteByte(ch)
			case ch == '/' && i+1 < len(script) && script[i+1] == '*':
				state = sqlStateBlockComment
				current.WriteByte(ch)
				i++
				current.WriteByte(script[i])
			case ch == '$':
				if tag, end, ok := readDollarTag(script, i); ok {
					state = sqlStateDollar
					dollarTag = tag
					current.WriteString(script[i:end])
					i = end - 1
				} else {
					current.WriteByte(ch)
				}
			case ch == ';':
				flush()
			default:
				current.WriteByte(ch)
			}

		case sqlStateSingle:
			current.WriteByte(ch)
			if ch == '\\' && i+1 < len(script) {
				i++
				current.WriteByte(script[i])
			} else if ch == '\'' {
				if i+1 < len(script) && script[i+1] == '\'' {
					i++
					current.WriteByte(script[i])
				} else {
					state = sqlStateNormal
				}
			}

		case sqlStateDouble:
			current.WriteByte(ch)
			if ch == '\\' && i+1 < len(script) {
				i++
				current.WriteByte(script[i])
			} else if ch == '"' {
				if i+1 < len(script) && script[i+1] == '"' {
					i++
					current.WriteByte(script[i])
				} else {
					state = sqlStateNormal
				}
			}

		case sqlStateBacktick:
			current.WriteByte(ch)
			if ch == '`' {
				if i+1 < len(script) && script[i+1] == '`' {
					i++
					current.WriteByte(script[i])
				} else {
					state = sqlStateNormal
				}
			}

		case sqlStateLineComment:
			current.WriteByte(ch)
			if ch == '\n' {
				state = sqlStateNormal
			}

		case sqlStateBlockComment:
			current.WriteByte(ch)
			if ch == '*' && i+1 < len(script) && script[i+1] == '/' {
				i++
				current.WriteByte(script[i])
				state = sqlStateNormal
			}

		case sqlStateDollar:
			if strings.HasPrefix(script[i:], dollarTag) {
				current.WriteString(dollarTag)
				i += len(dollarTag) - 1
				state = sqlStateNormal
				continue
			}
			current.WriteByte(ch)
		}
	}

	switch state {
	case sqlStateSingle:
		return nil, fmt.Errorf("unterminated single-quoted string")
	case sqlStateDouble:
		return nil, fmt.Errorf("unterminated double-quoted identifier")
	case sqlStateBacktick:
		return nil, fmt.Errorf("unterminated backtick-quoted identifier")
	case sqlStateBlockComment:
		return nil, fmt.Errorf("unterminated block comment")
	case sqlStateDollar:
		return nil, fmt.Errorf("unterminated dollar-quoted string (%s)", dollarTag)
	}

	flush()
	return statements, nil
}

type sqlState int

const (
	sqlStateNormal sqlState = iota
	sqlStateSingle
	sqlStateDouble
	sqlStateBacktick
	sqlStateLineComment
	sqlStateBlockComment
	sqlStateDollar
)

// readDollarTag recognizes a PostgreSQL dollar-quote opener at i and returns
// the tag (including both $ characters) and the index just past the opener.
func readDollarTag(s string, i int) (tag string, end int, ok bool) {
	if s[i] != '$' {
		return "", i, false
	}
	j := i + 1
	for j < len(s) {
		ch := s[j]
		if ch == '$' {
			return s[i : j+1], j + 1, true
		}
		if !isSQLTagChar(ch) {
			return "", i, false
		}
		j++
	}
	return "", i, false
}

func isSQLTagChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') ||
		ch == '_'
}

// isOnlyWhitespaceOrComments reports whether a statement fragment contains no
// executable SQL (only whitespace/comments).
func isOnlyWhitespaceOrComments(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	for len(s) > 0 {
		s = strings.TrimSpace(s)
		switch {
		case strings.HasPrefix(s, "--"):
			if idx := strings.IndexByte(s, '\n'); idx >= 0 {
				s = s[idx+1:]
			} else {
				return true
			}
		case strings.HasPrefix(s, "/*"):
			idx := strings.Index(s[2:], "*/")
			if idx < 0 {
				return true
			}
			s = s[2+idx+2:]
		default:
			return false
		}
	}
	return true
}

// HasNoTransactionMarker reports whether a migration opts out of the wrapping
// transaction with:
//
//	-- vm:no-transaction
//
// This is required for statements such as PostgreSQL CREATE INDEX
// CONCURRENTLY, which cannot run inside a transaction block.
func HasNoTransactionMarker(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(strings.ToLower(line))
		if line == "-- vm:no-transaction" || line == "--vm:no-transaction" {
			return true
		}
	}
	return false
}

// StatementTransactionSafety classifies statements that must not run inside a
// transaction. The returned reason is empty when the statement is safe.
func StatementTransactionSafety(stmt string) (reason string) {
	switch {
	case StatementControlsTransaction(stmt):
		return "statement controls transactions itself"
	case StatementNeedsNoTransaction(stmt):
		return "statement cannot run inside a transaction block"
	}
	return ""
}

// StatementControlsTransaction reports explicit BEGIN/COMMIT/ROLLBACK/START
// TRANSACTION. vm manages transactions, so these must be removed rather than
// executed on a pooled connection.
func StatementControlsTransaction(stmt string) bool {
	upper := strings.ToUpper(stripSQLLineComments(stmt))
	normalized := strings.Join(strings.Fields(upper), " ")
	switch {
	case strings.HasPrefix(normalized, "BEGIN"),
		strings.HasPrefix(normalized, "COMMIT"),
		strings.HasPrefix(normalized, "ROLLBACK"),
		strings.HasPrefix(normalized, "START TRANSACTION"):
		return true
	}
	return false
}

// StatementNeedsNoTransaction reports statements such as CREATE INDEX
// CONCURRENTLY and VACUUM that cannot run inside a transaction block.
func StatementNeedsNoTransaction(stmt string) bool {
	upper := strings.ToUpper(stripSQLLineComments(stmt))
	normalized := strings.Join(strings.Fields(upper), " ")
	switch {
	case strings.Contains(normalized, "CREATE INDEX CONCURRENTLY"),
		strings.Contains(normalized, "DROP INDEX CONCURRENTLY"),
		strings.Contains(normalized, "REINDEX") && strings.Contains(normalized, "CONCURRENTLY"),
		strings.Contains(normalized, "VACUUM"):
		return true
	}
	return false
}
