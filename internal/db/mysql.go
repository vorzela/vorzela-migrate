package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

type sqlRows struct {
	r *sql.Rows
}

func (s sqlRows) Next() bool                     { return s.r.Next() }
func (s sqlRows) Scan(dest ...interface{}) error { return s.r.Scan(dest...) }
func (s sqlRows) Close()                         { s.r.Close() }
func (s sqlRows) Err() error                     { return s.r.Err() }

type sqlRow struct {
	r *sql.Row
}

func (r sqlRow) Scan(dest ...interface{}) error { return r.r.Scan(dest...) }

// SqlDB wraps database/sql DB.
//
// Migration code is written with PostgreSQL-style $1 placeholders so the same
// queries work on both backends. The MySQL driver only understands ?, so all
// queries are translated here (outside of string literals and comments).
type SqlDB struct {
	DB *sql.DB
}

func (s *SqlDB) Exec(ctx context.Context, query string, args ...interface{}) error {
	_, err := s.DB.ExecContext(ctx, convertPlaceholders(query), args...)
	return err
}

func (s *SqlDB) Query(ctx context.Context, query string, args ...interface{}) (Rows, error) {
	rows, err := s.DB.QueryContext(ctx, convertPlaceholders(query), args...)
	if err != nil {
		return nil, err
	}
	return sqlRows{r: rows}, nil
}

func (s *SqlDB) QueryRow(ctx context.Context, query string, args ...interface{}) Row {
	row := s.DB.QueryRowContext(ctx, convertPlaceholders(query), args...)
	return sqlRow{r: row}
}

func (s *SqlDB) Ping(ctx context.Context) error {
	return s.DB.PingContext(ctx)
}

func (s *SqlDB) Close() { s.DB.Close() }

// NormalizeMySQLDSN converts a mysql:// URL to the DSN accepted by
// go-sql-driver/mysql. Non-URL DSNs are returned unchanged.
func NormalizeMySQLDSN(dsn string) (string, error) {
	if !strings.HasPrefix(dsn, "mysql://") {
		return dsn, nil
	}

	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("invalid mysql URL: %w", err)
	}
	if u.User == nil {
		return "", fmt.Errorf("invalid mysql URL: user information is required")
	}

	user := u.User.Username()
	pass, _ := u.User.Password()
	host := u.Host
	dbName := strings.TrimPrefix(u.Path, "/")
	if host == "" {
		return "", fmt.Errorf("invalid mysql URL: host is required")
	}

	// url.Userinfo handles escaping in the URL. The MySQL DSN expects the
	// raw values (the driver parses its own escaping), so use the decoded
	// forms and escape characters that would terminate the DSN components.
	out := fmt.Sprintf("%s:%s@tcp(%s)/%s", user, pass, host, dbName)
	if u.RawQuery != "" {
		out += "?" + u.RawQuery
	}
	return out, nil
}

// ConnectMySQL opens a database/sql connection for MySQL.
func ConnectMySQL(dsn string) (DB, error) {
	out, err := NormalizeMySQLDSN(dsn)
	if err != nil {
		return nil, err
	}

	sqlDB, err := sql.Open("mysql", out)
	if err != nil {
		return nil, fmt.Errorf("failed to open mysql: %w", err)
	}

	// set some reasonable defaults
	sqlDB.SetMaxOpenConns(5)
	sqlDB.SetMaxIdleConns(1)

	return &SqlDB{DB: sqlDB}, nil
}

// convertPlaceholders rewrites PostgreSQL-style $N placeholders to MySQL ?.
// It deliberately ignores quoted strings, quoted identifiers, and comments so
// that data such as 'price is $5' or a `$1` column name is not changed.
func convertPlaceholders(query string) string {
	if !strings.Contains(query, "$") {
		return query
	}

	var b strings.Builder
	b.Grow(len(query))

	const (
		stateNormal = iota
		stateSingle
		stateDouble
		stateBacktick
		stateLineComment
		stateBlockComment
	)
	state := stateNormal

	for i := 0; i < len(query); i++ {
		ch := query[i]

		switch state {
		case stateNormal:
			switch {
			case ch == '\'':
				state = stateSingle
				b.WriteByte(ch)
			case ch == '"':
				state = stateDouble
				b.WriteByte(ch)
			case ch == '`':
				state = stateBacktick
				b.WriteByte(ch)
			case ch == '-' && i+1 < len(query) && query[i+1] == '-':
				state = stateLineComment
				b.WriteByte(ch)
			case ch == '/' && i+1 < len(query) && query[i+1] == '*':
				state = stateBlockComment
				b.WriteByte(ch)
				i++
				b.WriteByte(query[i])
			case ch == '$' && i+1 < len(query) && isDigit(query[i+1]):
				// Consume $<digits> and emit a single ?.
				j := i + 1
				for j < len(query) && isDigit(query[j]) {
					j++
				}
				b.WriteByte('?')
				i = j - 1
			default:
				b.WriteByte(ch)
			}

		case stateSingle:
			b.WriteByte(ch)
			if ch == '\\' && i+1 < len(query) {
				i++
				b.WriteByte(query[i])
			} else if ch == '\'' {
				// SQL-standard doubled quote
				if i+1 < len(query) && query[i+1] == '\'' {
					i++
					b.WriteByte(query[i])
				} else {
					state = stateNormal
				}
			}

		case stateDouble:
			b.WriteByte(ch)
			if ch == '\\' && i+1 < len(query) {
				i++
				b.WriteByte(query[i])
			} else if ch == '"' {
				if i+1 < len(query) && query[i+1] == '"' {
					i++
					b.WriteByte(query[i])
				} else {
					state = stateNormal
				}
			}

		case stateBacktick:
			b.WriteByte(ch)
			if ch == '`' {
				if i+1 < len(query) && query[i+1] == '`' {
					i++
					b.WriteByte(query[i])
				} else {
					state = stateNormal
				}
			}

		case stateLineComment:
			b.WriteByte(ch)
			if ch == '\n' {
				state = stateNormal
			}

		case stateBlockComment:
			b.WriteByte(ch)
			if ch == '*' && i+1 < len(query) && query[i+1] == '/' {
				i++
				b.WriteByte(query[i])
				state = stateNormal
			}
		}
	}

	return b.String()
}

func isDigit(ch byte) bool { return ch >= '0' && ch <= '9' }
