package introspect

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/vorzela/vorm/query"
)

func TestTableFilter(t *testing.T) {
	cases := []struct {
		name  string
		opts  Options
		allow []string
		deny  []string
	}{
		{
			name:  "bookkeeping excluded by default",
			opts:  Options{},
			allow: []string{"users", "posts"},
			deny:  []string{"migrations", "migrations_lock"},
		},
		{
			name:  "include list restricts everything else",
			opts:  Options{IncludeTables: []string{"users"}},
			allow: []string{"users"},
			deny:  []string{"posts", "migrations"},
		},
		{
			name:  "include list re-enables a bookkeeping table",
			opts:  Options{IncludeTables: []string{"users", "migrations"}},
			allow: []string{"users", "migrations"},
			deny:  []string{"posts", "migrations_lock"},
		},
		{
			name:  "exclude wins over include",
			opts:  Options{IncludeTables: []string{"users", "posts"}, ExcludeTables: []string{"posts"}},
			allow: []string{"users"},
			deny:  []string{"posts"},
		},
		{
			name:  "matching is case-insensitive",
			opts:  Options{ExcludeTables: []string{"AuditLog"}},
			allow: []string{"users"},
			deny:  []string{"auditlog", "AUDITLOG", "Migrations"},
		},
		{
			name:  "blank entries are ignored",
			opts:  Options{IncludeTables: []string{"", "  "}},
			allow: []string{"users", "posts"},
			deny:  []string{"migrations"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newTableFilter(tc.opts)
			for _, name := range tc.allow {
				if !f.allows(name) {
					t.Errorf("allows(%q) = false, want true", name)
				}
			}
			for _, name := range tc.deny {
				if f.allows(name) {
					t.Errorf("allows(%q) = true, want false", name)
				}
			}
		})
	}
}

func TestNormalizeDialect(t *testing.T) {
	cases := map[query.Dialect]query.Dialect{
		"":           query.DialectPostgres,
		"postgres":   query.DialectPostgres,
		"PostgreSQL": query.DialectPostgres,
		"pgx":        query.DialectPostgres,
		"pq":         query.DialectPostgres,
		"mysql":      query.DialectMySQL,
		"MariaDB":    query.DialectMySQL,
		" mysql ":    query.DialectMySQL,
		"sqlite":     "",
	}
	for in, want := range cases {
		if got := normalizeDialect(in); got != want {
			t.Errorf("normalizeDialect(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIntrospectDispatch(t *testing.T) {
	ctx := context.Background()

	for _, dialect := range []query.Dialect{"", "postgres", "postgresql"} {
		s, err := Introspect(ctx, postgresFixture(), Options{Dialect: dialect})
		if err != nil {
			t.Fatalf("Introspect(%q): %v", dialect, err)
		}
		if s.Dialect != query.DialectPostgres {
			t.Errorf("Introspect(%q) produced dialect %q", dialect, s.Dialect)
		}
	}

	for _, dialect := range []query.Dialect{"mysql", "mariadb"} {
		s, err := Introspect(ctx, mysqlFixture(), Options{Dialect: dialect})
		if err != nil {
			t.Fatalf("Introspect(%q): %v", dialect, err)
		}
		if s.Dialect != query.DialectMySQL {
			t.Errorf("Introspect(%q) produced dialect %q", dialect, s.Dialect)
		}
	}

	if _, err := Introspect(ctx, postgresFixture(), Options{Dialect: "sqlite"}); err == nil {
		t.Fatal("Introspect with an unknown dialect should fail")
	} else if !strings.Contains(err.Error(), "unsupported dialect") {
		t.Errorf("error = %q, want it to name the unsupported dialect", err)
	}
}

func TestIntrospectNilDB(t *testing.T) {
	for _, dialect := range []query.Dialect{"postgres", "mysql"} {
		if _, err := Introspect(context.Background(), nil, Options{Dialect: dialect}); err == nil {
			t.Errorf("Introspect(%q, nil) should fail", dialect)
		}
	}
}

func TestApplyEnumTypes(t *testing.T) {
	s := &Schema{
		Enums: []Enum{{Name: "user_status", Schema: "public", Values: []string{"a"}}},
		Tables: []Table{{
			Name: "users",
			Columns: []Column{
				{Name: "status", DBType: "user_status"},
				{Name: "history", DBType: "_user_status", IsArray: true, ArrayElem: "user_status"},
				{Name: "email", DBType: "varchar"},
				{Name: "kind", DBType: "USER_STATUS"},
			},
		}},
	}
	applyEnumTypes(s)

	table := &s.Tables[0]
	for _, name := range []string{"status", "history", "kind"} {
		col, _ := table.Column(name)
		if col.EnumType != "user_status" {
			t.Errorf("%s EnumType = %q, want user_status", name, col.EnumType)
		}
	}
	if col, _ := table.Column("email"); col.EnumType != "" {
		t.Errorf("email EnumType = %q, want empty", col.EnumType)
	}
}

func TestQueryRowsClosesRows(t *testing.T) {
	rows := &fakeRows{rows: [][]any{{"a"}, {"b"}}}
	db := &closeTrackingDB{rows: rows}

	var seen []string
	err := queryRows(context.Background(), db, "step", "SELECT 1", nil, func(r query.Rows) error {
		var v string
		if err := r.Scan(&v); err != nil {
			return err
		}
		seen = append(seen, v)
		return nil
	})
	if err != nil {
		t.Fatalf("queryRows: %v", err)
	}
	if !reflect.DeepEqual(seen, []string{"a", "b"}) {
		t.Errorf("scanned %v, want [a b]", seen)
	}
	if !rows.closed {
		t.Error("queryRows must close the result set")
	}
}

func TestQueryRowsReportsIterationError(t *testing.T) {
	rows := &fakeRows{rows: [][]any{{"a"}}, err: errIteration}
	db := &closeTrackingDB{rows: rows}

	err := queryRows(context.Background(), db, "step", "SELECT 1", nil, func(query.Rows) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "vorm/introspect: step:") {
		t.Fatalf("error = %v, want a wrapped rows.Err failure", err)
	}
	if !rows.closed {
		t.Error("queryRows must close the result set even when iteration fails")
	}
}

var errIteration = errorString("iteration failed")

type errorString string

func (e errorString) Error() string { return string(e) }

type closeTrackingDB struct {
	rows *fakeRows
}

func (d *closeTrackingDB) QueryContext(context.Context, string, ...any) (query.Rows, error) {
	return d.rows, nil
}

func (d *closeTrackingDB) QueryRowContext(context.Context, string, ...any) query.Row {
	return &fakeRow{}
}

func (d *closeTrackingDB) ExecContext(context.Context, string, ...any) (query.Result, error) {
	return nil, errIteration
}
