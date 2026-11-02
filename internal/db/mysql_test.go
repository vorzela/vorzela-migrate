package db

import (
	"strings"
	"testing"
)

func TestNormalizeMySQLDSN(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		err  bool
	}{
		{
			name: "url form",
			in:   "mysql://user:pass@localhost:3306/mydb",
			want: "user:pass@tcp(localhost:3306)/mydb",
		},
		{
			name: "url form with query",
			in:   "mysql://user:pass@localhost:3306/mydb?parseTime=true",
			want: "user:pass@tcp(localhost:3306)/mydb?parseTime=true",
		},
		{
			name: "native dsn untouched",
			in:   "user:pass@tcp(localhost:3306)/mydb",
			want: "user:pass@tcp(localhost:3306)/mydb",
		},
		{
			name: "missing user",
			in:   "mysql://localhost:3306/mydb",
			err:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeMySQLDSN(tt.in)
			if tt.err {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConvertPlaceholders(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "basic positional",
			in:   "INSERT INTO t (a,b) VALUES ($1,$2)",
			want: "INSERT INTO t (a,b) VALUES (?,?)",
		},
		{
			name: "multi digit",
			in:   "SELECT $10",
			want: "SELECT ?",
		},
		{
			name: "inside single quote",
			in:   "SELECT '$1 is not a placeholder'",
			want: "SELECT '$1 is not a placeholder'",
		},
		{
			name: "inside double quote",
			in:   `SELECT "$1" FROM t`,
			want: `SELECT "$1" FROM t`,
		},
		{
			name: "inside comment",
			in:   "SELECT 1 -- keep $1\nFROM t WHERE a=$2",
			want: "SELECT 1 -- keep $1\nFROM t WHERE a=?",
		},
		{
			name: "dollar amount",
			in:   "SELECT '$5.00'",
			want: "SELECT '$5.00'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertPlaceholders(tt.in)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
			if strings.Count(got, "?") != strings.Count(tt.want, "?") {
				t.Fatalf("placeholder count changed: %q", got)
			}
		})
	}
}
