package query

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ErrGeneratePending is returned for stubs vorm generate could not fully lower yet.
// Prefer gen.* for lowered queries.
var ErrGeneratePending = errors.New("vorm: query not fully generated — simplify stub or use models.* builders")

// ErrNoRows reports an empty result where exactly one row was required.
// It wraps sql.ErrNoRows so existing errors.Is checks keep working.
var ErrNoRows = fmt.Errorf("vorm: no rows in result set: %w", sql.ErrNoRows)

// Kind classifies a database failure independently of the driver in use.
type Kind string

const (
	KindUnknown          Kind = "unknown"
	KindNoRows           Kind = "no_rows"
	KindUnique           Kind = "unique_violation"
	KindForeignKey       Kind = "foreign_key_violation"
	KindNotNull          Kind = "not_null_violation"
	KindCheck            Kind = "check_violation"
	KindDeadlock         Kind = "deadlock"
	KindSerialization    Kind = "serialization_failure"
	KindLockNotAvailable Kind = "lock_not_available"
	KindUndefinedTable   Kind = "undefined_table"
	KindUndefinedColumn  Kind = "undefined_column"
	KindDataException    Kind = "data_exception"
	KindConnection       Kind = "connection"
	KindSyntax           Kind = "syntax"
	KindPermission       Kind = "permission_denied"
	KindTimeout          Kind = "timeout"
	KindValidation       Kind = "validation"
)

// Error is a database failure enriched with the statement that produced it.
// SQL is retained for logs; bound values are never embedded in the message.
type Error struct {
	Op         string // select, insert, update, delete, count, exists, migrate…
	Table      string
	Kind       Kind
	Code       string // SQLSTATE (Postgres) or errno (MySQL)
	Constraint string
	Column     string
	Detail     string
	SQL        string
	ArgCount   int
	Err        error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	var b strings.Builder
	b.WriteString("vorm/query: ")
	if e.Op != "" {
		b.WriteString(e.Op)
	}
	if e.Table != "" {
		b.WriteString(" ")
		b.WriteString(e.Table)
	}
	if e.Kind != "" && e.Kind != KindUnknown {
		b.WriteString(" [")
		b.WriteString(string(e.Kind))
		if e.Code != "" {
			b.WriteString(" ")
			b.WriteString(e.Code)
		}
		b.WriteString("]")
	}
	if e.Constraint != "" {
		b.WriteString(" constraint=")
		b.WriteString(e.Constraint)
	}
	if e.Column != "" {
		b.WriteString(" column=")
		b.WriteString(e.Column)
	}
	if e.Err != nil {
		b.WriteString(": ")
		b.WriteString(e.Err.Error())
	}
	if e.Detail != "" {
		b.WriteString(" (")
		b.WriteString(e.Detail)
		b.WriteString(")")
	}
	return b.String()
}

func (e *Error) Unwrap() error { return e.Err }

// Is lets errors.Is(err, ErrNoRows) match a wrapped no-rows failure.
func (e *Error) Is(target error) bool {
	if e == nil {
		return false
	}
	if e.Kind == KindNoRows && (target == ErrNoRows || errors.Is(target, sql.ErrNoRows)) {
		return true
	}
	return false
}

// wrapErr attaches statement context and driver classification to err.
func wrapErr(op, table, sqlText string, argCount int, err error) error {
	if err == nil {
		return nil
	}
	var already *Error
	if errors.As(err, &already) {
		return err
	}
	e := &Error{
		Op:       op,
		Table:    table,
		SQL:      sqlText,
		ArgCount: argCount,
		Err:      err,
	}
	classify(e)
	return e
}

// validationErr reports a vorm-side rejection (unknown column, bad operator,
// refused SELECT *) that never reached the database.
func validationErr(op, table string, format string, args ...any) error {
	return &Error{
		Op:    op,
		Table: table,
		Kind:  KindValidation,
		Err:   fmt.Errorf(format, args...),
	}
}

// Classify reports the portable Kind for any error returned by vorm.
func Classify(err error) Kind {
	if err == nil {
		return KindUnknown
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	if errors.Is(err, sql.ErrNoRows) {
		return KindNoRows
	}
	probe := &Error{Err: err}
	classify(probe)
	return probe.Kind
}

// Code returns the driver-native error code (SQLSTATE or MySQL errno), if any.
func Code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	probe := &Error{Err: err}
	classify(probe)
	return probe.Code
}

// Constraint returns the violated constraint name, if the driver reported one.
func Constraint(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Constraint
	}
	probe := &Error{Err: err}
	classify(probe)
	return probe.Constraint
}

func IsNotFound(err error) bool             { return Classify(err) == KindNoRows }
func IsUniqueViolation(err error) bool      { return Classify(err) == KindUnique }
func IsForeignKeyViolation(err error) bool  { return Classify(err) == KindForeignKey }
func IsNotNullViolation(err error) bool     { return Classify(err) == KindNotNull }
func IsCheckViolation(err error) bool       { return Classify(err) == KindCheck }
func IsDeadlock(err error) bool             { return Classify(err) == KindDeadlock }
func IsSerializationFailure(err error) bool { return Classify(err) == KindSerialization }
func IsValidationError(err error) bool      { return Classify(err) == KindValidation }

// IsRetryable reports whether retrying the transaction may succeed.
func IsRetryable(err error) bool {
	switch Classify(err) {
	case KindDeadlock, KindSerialization, KindLockNotAvailable, KindConnection:
		return true
	default:
		return false
	}
}

// kindForSQLState maps a Postgres SQLSTATE to a portable Kind.
func kindForSQLState(state string) Kind {
	switch state {
	case "23505":
		return KindUnique
	case "23503":
		return KindForeignKey
	case "23502":
		return KindNotNull
	case "23514":
		return KindCheck
	case "40P01":
		return KindDeadlock
	case "40001":
		return KindSerialization
	case "55P03":
		return KindLockNotAvailable
	case "42P01":
		return KindUndefinedTable
	case "42703":
		return KindUndefinedColumn
	case "42601":
		return KindSyntax
	case "42501":
		return KindPermission
	case "57014":
		return KindTimeout
	}
	switch {
	case strings.HasPrefix(state, "08"):
		return KindConnection
	case strings.HasPrefix(state, "22"):
		return KindDataException
	case strings.HasPrefix(state, "23"):
		return KindCheck
	case strings.HasPrefix(state, "42"):
		return KindSyntax
	default:
		return KindUnknown
	}
}

// kindForMySQLErrno maps a MySQL/MariaDB error number to a portable Kind.
func kindForMySQLErrno(n uint16) Kind {
	switch n {
	case 1062, 1169, 1586:
		return KindUnique
	case 1216, 1217, 1451, 1452:
		return KindForeignKey
	case 1048, 1364:
		return KindNotNull
	case 3819, 4025:
		return KindCheck
	case 1213:
		return KindDeadlock
	case 1205:
		return KindLockNotAvailable
	case 1146:
		return KindUndefinedTable
	case 1054:
		return KindUndefinedColumn
	case 1064:
		return KindSyntax
	case 1044, 1045, 1142, 1143:
		return KindPermission
	case 1040, 1042, 1043, 1129, 2002, 2003, 2006, 2013:
		return KindConnection
	default:
		return KindUnknown
	}
}
