package migrate

import (
	"reflect"
	"testing"

	"github.com/vorzela/vorm/query"
)

func TestSplitStatements(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want []string
	}{
		{
			name: "two statements on one line",
			sql:  "SELECT 1; SELECT 2;",
			want: []string{"SELECT 1;", "SELECT 2;"},
		},
		{
			name: "trailing fragment without a semicolon",
			sql:  "SELECT 1;\nSELECT 2",
			want: []string{"SELECT 1;", "SELECT 2"},
		},
		{
			name: "semicolon inside a single quoted literal",
			sql:  "INSERT INTO t (v) VALUES ('a;b');\nSELECT 1;",
			want: []string{"INSERT INTO t (v) VALUES ('a;b');", "SELECT 1;"},
		},
		{
			name: "doubled single quotes stay inside the literal",
			sql:  "INSERT INTO t (v) VALUES ('it''s; fine');",
			want: []string{"INSERT INTO t (v) VALUES ('it''s; fine');"},
		},
		{
			name: "semicolon inside a quoted identifier",
			sql:  `CREATE TABLE "we;ird" (id INT);`,
			want: []string{`CREATE TABLE "we;ird" (id INT);`},
		},
		{
			name: "dollar quoted function body is one statement",
			sql: "CREATE FUNCTION touch() RETURNS trigger AS $$\n" +
				"BEGIN\n" +
				"  NEW.updated_at = NOW();\n" +
				"  RETURN NEW;\n" +
				"END;\n" +
				"$$ LANGUAGE plpgsql;\n" +
				"SELECT 1;\n",
			want: []string{
				"CREATE FUNCTION touch() RETURNS trigger AS $$\n" +
					"BEGIN\n" +
					"  NEW.updated_at = NOW();\n" +
					"  RETURN NEW;\n" +
					"END;\n" +
					"$$ LANGUAGE plpgsql;",
				"SELECT 1;",
			},
		},
		{
			name: "tagged dollar quoting",
			sql: "CREATE FUNCTION f() RETURNS void AS $body$\n" +
				"  SELECT 1; SELECT 2;\n" +
				"$body$ LANGUAGE sql;\n",
			want: []string{
				"CREATE FUNCTION f() RETURNS void AS $body$\n" +
					"  SELECT 1; SELECT 2;\n" +
					"$body$ LANGUAGE sql;",
			},
		},
		{
			name: "comment lines inside a dollar quoted body are kept",
			sql: "CREATE FUNCTION f() RETURNS void AS $$\n" +
				"-- keep me\n" +
				"SELECT 1;\n" +
				"$$ LANGUAGE sql;\n",
			want: []string{
				"CREATE FUNCTION f() RETURNS void AS $$\n" +
					"-- keep me\n" +
					"SELECT 1;\n" +
					"$$ LANGUAGE sql;",
			},
		},
		{
			name: "a differently tagged dollar quote inside a body does not close it",
			sql:  "SELECT $outer$ a $inner$ b; c $outer$;\nSELECT 1;",
			want: []string{"SELECT $outer$ a $inner$ b; c $outer$;", "SELECT 1;"},
		},
		{
			name: "positional parameters are not dollar quotes",
			sql:  "INSERT INTO t (a, b) VALUES ($1, $2);\nSELECT 1;",
			want: []string{"INSERT INTO t (a, b) VALUES ($1, $2);", "SELECT 1;"},
		},
		{
			name: "comment lines are dropped",
			sql:  "-- create it\nCREATE TABLE t (id INT);\n-- drop it\nDROP TABLE t;",
			want: []string{"CREATE TABLE t (id INT);", "DROP TABLE t;"},
		},
		{
			name: "trailing inline comment does not become a statement",
			sql:  "CREATE TABLE t (id INT); -- done\n",
			want: []string{"CREATE TABLE t (id INT);"},
		},
		{
			name: "comment only input",
			sql:  "-- nothing here\n-- CREATE EXTENSION IF NOT EXISTS citext;\n",
			want: nil,
		},
		{
			name: "empty input",
			sql:  "",
			want: nil,
		},
		{
			name: "stray semicolons are dropped",
			sql:  ";;\nSELECT 1;\n;",
			want: []string{"SELECT 1;"},
		},
		{
			name: "unterminated dollar quote does not lose the tail",
			sql:  "CREATE FUNCTION f() AS $$ SELECT 1;",
			want: []string{"CREATE FUNCTION f() AS $$ SELECT 1;"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SplitStatements(tc.sql)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("SplitStatements()\n got: %#v\nwant: %#v", got, tc.want)
			}
		})
	}
}

func TestRequiresNoTransaction(t *testing.T) {
	tests := []struct {
		name    string
		sql     string
		dialect query.Dialect
		want    bool
	}{
		{"create index concurrently", "CREATE INDEX CONCURRENTLY idx ON t (a);", query.DialectPostgres, true},
		{"lower case", "create index concurrently idx on t (a);", query.DialectPostgres, true},
		{"unique index", "CREATE UNIQUE INDEX CONCURRENTLY idx ON t (a);", query.DialectPostgres, true},
		{"drop index concurrently", "DROP INDEX CONCURRENTLY idx;", query.DialectPostgres, true},
		{"wrapped across lines", "CREATE INDEX\n  CONCURRENTLY idx ON t (a);", query.DialectPostgres, true},
		{"plain create index", "CREATE INDEX idx ON t (a);", query.DialectPostgres, false},
		{"unrelated ddl", "ALTER TABLE t ADD COLUMN a INT;", query.DialectPostgres, false},
		{"mysql has no concurrent index", "CREATE INDEX CONCURRENTLY idx ON t (a);", query.DialectMySQL, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := requiresNoTransaction(SplitStatements(tc.sql), tc.dialect); got != tc.want {
				t.Errorf("requiresNoTransaction() = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestRequiresNoTransactionFindsLaterStatement(t *testing.T) {
	stmts := SplitStatements("ALTER TABLE t ADD COLUMN a INT;\nCREATE INDEX CONCURRENTLY idx ON t (a);")
	if len(stmts) != 2 {
		t.Fatalf("split produced %d statements, want 2", len(stmts))
	}
	if !requiresNoTransaction(stmts, query.DialectPostgres) {
		t.Error("requiresNoTransaction() = false, want true when any statement is concurrent")
	}
}
