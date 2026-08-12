package query

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

// classify fills Kind/Code/Constraint/Column/Detail from the underlying driver error.
func classify(e *Error) {
	err := e.Err
	if err == nil {
		return
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		e.Code = pgErr.Code
		e.Kind = kindForSQLState(pgErr.Code)
		e.Constraint = pgErr.ConstraintName
		e.Column = pgErr.ColumnName
		e.Detail = pgErr.Detail
		if e.Table == "" {
			e.Table = pgErr.TableName
		}
		return
	}

	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		e.Code = string(pqErr.Code)
		e.Kind = kindForSQLState(string(pqErr.Code))
		e.Constraint = pqErr.Constraint
		e.Column = pqErr.Column
		e.Detail = pqErr.Detail
		if e.Table == "" {
			e.Table = pqErr.Table
		}
		return
	}

	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) {
		e.Code = strconv.Itoa(int(myErr.Number))
		e.Kind = kindForMySQLErrno(myErr.Number)
		e.Detail = myErr.Message
		if e.Kind == KindUnique || e.Kind == KindForeignKey {
			e.Constraint = mysqlConstraintName(myErr.Message)
		}
		return
	}

	switch {
	case errors.Is(err, sql.ErrNoRows), errors.Is(err, pgx.ErrNoRows):
		e.Kind = KindNoRows
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		e.Kind = KindTimeout
	default:
		e.Kind = KindUnknown
	}
}

// mysqlConstraintName extracts the quoted key name MySQL embeds in the message,
// e.g. Duplicate entry 'a' for key 'users.email_unique'.
func mysqlConstraintName(msg string) string {
	const marker = "for key '"
	i := strings.Index(msg, marker)
	if i < 0 {
		return ""
	}
	rest := msg[i+len(marker):]
	j := strings.IndexByte(rest, '\'')
	if j < 0 {
		return ""
	}
	name := rest[:j]
	if k := strings.LastIndexByte(name, '.'); k >= 0 {
		name = name[k+1:]
	}
	return name
}
