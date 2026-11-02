package migration

import (
	"strings"
	"testing"
)

func TestSplitSQLStatements(t *testing.T) {
	tests := []struct {
		name      string
		sql       string
		wantParts int
		wantSub   []string
		wantErr   bool
	}{
		{
			name:      "simple statements",
			sql:       "CREATE TABLE a (id int); CREATE TABLE b (id int);",
			wantParts: 2,
			wantSub:   []string{"CREATE TABLE a", "CREATE TABLE b"},
		},
		{
			name:      "semicolon in string",
			sql:       "INSERT INTO t VALUES ('a;b'); SELECT 1;",
			wantParts: 2,
			wantSub:   []string{"'a;b'", "SELECT 1"},
		},
		{
			name:      "line comments",
			sql:       "-- a comment; with semicolon\nSELECT 1;\n-- another\nSELECT 2;",
			wantParts: 2,
			wantSub:   []string{"SELECT 1", "SELECT 2"},
		},
		{
			name:      "block comment",
			sql:       "/* ; */ SELECT 1;",
			wantParts: 1,
			wantSub:   []string{"SELECT 1"},
		},
		{
			name: "dollar quoted function",
			sql: `CREATE OR REPLACE FUNCTION f() RETURNS trigger AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
SELECT 1;`,
			wantParts: 2,
			wantSub:   []string{"CREATE OR REPLACE FUNCTION", "SELECT 1"},
		},
		{
			name:      "tagged dollar quote",
			sql:       "DO $body$ BEGIN PERFORM 1; END $body$; SELECT 2;",
			wantParts: 2,
			wantSub:   []string{"DO $body$", "SELECT 2"},
		},
		{
			name:    "unterminated string",
			sql:     "SELECT 'broken;",
			wantErr: true,
		},
		{
			name:    "unterminated block comment",
			sql:     "SELECT 1; /* broken",
			wantErr: true,
		},
		{
			name:    "unterminated dollar quote",
			sql:     "DO $$ BEGIN PERFORM 1;",
			wantErr: true,
		},
		{
			name:      "comments only",
			sql:       "-- nothing here\n/* or here */",
			wantParts: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SplitSQLStatements(tt.sql)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got) != tt.wantParts {
				t.Fatalf("got %d statements %v, want %d", len(got), got, tt.wantParts)
			}
			for i, sub := range tt.wantSub {
				if i >= len(got) {
					break
				}
				if !strings.Contains(got[i], sub) {
					t.Errorf("statement %d = %q, want substring %q", i, got[i], sub)
				}
			}
		})
	}
}

func TestHasNoTransactionMarker(t *testing.T) {
	if !HasNoTransactionMarker("-- vm:no-transaction\nCREATE INDEX CONCURRENTLY x ON t(c);") {
		t.Fatal("expected marker to be detected")
	}
	if HasNoTransactionMarker("-- normal comment\nSELECT 1;") {
		t.Fatal("did not expect marker")
	}
}

func TestStatementTransactionSafety(t *testing.T) {
	if reason := StatementTransactionSafety("CREATE INDEX CONCURRENTLY idx ON t(c);"); reason == "" {
		t.Fatal("expected CREATE INDEX CONCURRENTLY to be transaction-hostile")
	}
	if reason := StatementTransactionSafety("CREATE INDEX idx ON t(c);"); reason != "" {
		t.Fatalf("normal CREATE INDEX should be safe, got %q", reason)
	}
}
