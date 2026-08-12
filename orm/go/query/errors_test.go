package query

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestClassifyPostgresSQLStates(t *testing.T) {
	tests := []struct {
		state string
		want  Kind
	}{
		{"23505", KindUnique},
		{"23503", KindForeignKey},
		{"23502", KindNotNull},
		{"23514", KindCheck},
		{"40P01", KindDeadlock},
		{"40001", KindSerialization},
		{"42P01", KindUndefinedTable},
		{"42703", KindUndefinedColumn},
		{"08006", KindConnection},
		{"22001", KindDataException},
	}
	for _, tt := range tests {
		err := wrapErr("insert", "users", "INSERT …", 2, &pgconn.PgError{Code: tt.state, ConstraintName: "users_email_key"})
		if got := Classify(err); got != tt.want {
			t.Errorf("SQLSTATE %s → %s, want %s", tt.state, got, tt.want)
		}
		if Code(err) != tt.state {
			t.Errorf("Code(%s) = %q", tt.state, Code(err))
		}
	}
}

func TestUniqueViolationExposesConstraint(t *testing.T) {
	err := wrapErr("insert", "users", "INSERT …", 1, &pgconn.PgError{Code: "23505", ConstraintName: "users_email_key", ColumnName: "email"})
	if !IsUniqueViolation(err) {
		t.Fatal("expected unique violation")
	}
	if Constraint(err) != "users_email_key" {
		t.Fatalf("constraint = %q", Constraint(err))
	}
	var e *Error
	if !errors.As(err, &e) || e.Column != "email" {
		t.Fatalf("column not captured: %+v", e)
	}
}

func TestClassifyMySQLErrnos(t *testing.T) {
	err := wrapErr("insert", "users", "INSERT …", 1, &mysql.MySQLError{
		Number:  1062,
		Message: "Duplicate entry 'a@b.c' for key 'users.email_unique'",
	})
	if !IsUniqueViolation(err) {
		t.Fatalf("want unique violation, got %s", Classify(err))
	}
	if Constraint(err) != "email_unique" {
		t.Fatalf("constraint = %q", Constraint(err))
	}
	if !IsForeignKeyViolation(wrapErr("insert", "t", "", 0, &mysql.MySQLError{Number: 1452})) {
		t.Error("1452 should be a foreign key violation")
	}
	if !IsDeadlock(wrapErr("update", "t", "", 0, &mysql.MySQLError{Number: 1213})) {
		t.Error("1213 should be a deadlock")
	}
}

func TestNoRowsMatchesErrorsIs(t *testing.T) {
	err := wrapErr("select", "users", "SELECT …", 0, sql.ErrNoRows)
	if !IsNotFound(err) {
		t.Fatalf("want not found, got %s", Classify(err))
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Error("must stay compatible with errors.Is(err, sql.ErrNoRows)")
	}
	if !errors.Is(err, ErrNoRows) {
		t.Error("must match ErrNoRows")
	}
}

func TestIsRetryable(t *testing.T) {
	retryable := []string{"40001", "40P01", "55P03", "08006"}
	for _, state := range retryable {
		if !IsRetryable(wrapErr("x", "t", "", 0, &pgconn.PgError{Code: state})) {
			t.Errorf("%s should be retryable", state)
		}
	}
	if IsRetryable(wrapErr("x", "t", "", 0, &pgconn.PgError{Code: "23505"})) {
		t.Error("unique violation is not retryable")
	}
}

func TestWrapErrKeepsFirstWrap(t *testing.T) {
	inner := wrapErr("select", "users", "SELECT 1", 0, &pgconn.PgError{Code: "23505"})
	outer := wrapErr("update", "posts", "UPDATE …", 3, inner)
	var e *Error
	errors.As(outer, &e)
	if e.Op != "select" || e.Table != "users" {
		t.Fatalf("re-wrapping lost the original context: %+v", e)
	}
}

func TestErrorMessageOmitsBoundValues(t *testing.T) {
	err := wrapErr("insert", "users", "INSERT INTO users (email) VALUES ($1)", 1,
		&pgconn.PgError{Code: "23505", ConstraintName: "users_email_key"})
	msg := err.Error()
	if want := "unique_violation"; !contains(msg, want) {
		t.Fatalf("message %q missing %q", msg, want)
	}
	if contains(msg, "$1") {
		t.Fatal("statement text must not leak into the message")
	}
}

func TestValidationErrorsAreClassified(t *testing.T) {
	users := Model[scanUser](Meta{Table: "verr_users", Columns: []string{"id", "email"}})
	_, err := users.Where("nope", 1).Get(context.Background(), &fakeDB{})
	if err == nil {
		t.Fatal("unknown column must fail")
	}
	if !contains(err.Error(), "unknown column") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoggerReceivesEvents(t *testing.T) {
	users := Model[scanUser](Meta{Table: "log_users", Columns: []string{"id", "email"}})
	db := (&fakeDB{}).on("log_users", []string{"id", "email"}, []any{int64(1), "a@x.io"})

	var events []Event
	ctx := WithLogger(context.Background(), LoggerFunc(func(_ context.Context, ev Event) {
		events = append(events, ev)
	}))

	if _, err := users.New().Get(ctx, db); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Op != "select" || ev.Table != "log_users" || ev.Rows != 1 {
		t.Fatalf("unexpected event: %+v", ev)
	}
	if ev.Duration <= 0 {
		t.Error("duration should be measured")
	}
	if ev.SQL == "" {
		t.Error("SQL should be captured for logs")
	}
}

func TestLoggerReceivesFailures(t *testing.T) {
	users := Model[scanUser](Meta{Table: "logfail_users", Columns: []string{"id", "email"}})
	boom := &pgconn.PgError{Code: "42P01"}
	db := (&fakeDB{}).onError("logfail_users", boom)

	var got Event
	ctx := WithLogger(context.Background(), LoggerFunc(func(_ context.Context, ev Event) { got = ev }))
	if _, err := users.New().Get(ctx, db); err == nil {
		t.Fatal("expected error")
	}
	if got.Err == nil || Classify(got.Err) != KindUndefinedTable {
		t.Fatalf("logger did not receive the classified failure: %+v", got)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOfSub(s, sub) >= 0)
}

func indexOfSub(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
