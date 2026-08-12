package migrate

import (
	"strings"
	"testing"

	"github.com/vorzela/vorm/query"
)

func TestDetectDialect(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want query.Dialect
	}{
		{"postgres url", "postgres://user:pass@localhost:5432/app?sslmode=disable", query.DialectPostgres},
		{"postgresql url", "postgresql://user@localhost/app", query.DialectPostgres},
		{"unix socket", "host=/var/run/postgresql dbname=app", query.DialectPostgres},
		{"empty", "", query.DialectPostgres},
		{"mysql url", "mysql://user:pass@localhost:3306/app", query.DialectMySQL},
		{"mysql dsn with tcp", "user:pass@tcp(localhost:3306)/app?parseTime=true", query.DialectMySQL},
		{"tcp without at", "tcp(127.0.0.1:3306)/app", query.DialectMySQL},
		{"mariadb", "mariadb://user@localhost/app", query.DialectMySQL},
		{"uppercase mysql url", "MYSQL://user@localhost/app", query.DialectMySQL},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DetectDialect(tc.dsn); got != tc.want {
				t.Errorf("DetectDialect(%q) = %q, want %q", tc.dsn, got, tc.want)
			}
		})
	}
}

func TestIsMySQL(t *testing.T) {
	tests := []struct {
		dialect query.Dialect
		want    bool
	}{
		{query.DialectMySQL, true},
		{query.Dialect("mariadb"), true},
		{query.Dialect("MariaDB"), true},
		{query.DialectPostgres, false},
		{query.Dialect(""), false},
	}
	for _, tc := range tests {
		t.Run(string(tc.dialect), func(t *testing.T) {
			if got := isMySQL(tc.dialect); got != tc.want {
				t.Errorf("isMySQL(%q) = %t, want %t", tc.dialect, got, tc.want)
			}
		})
	}
}

func TestPlaceholders(t *testing.T) {
	if got := placeholder(query.DialectPostgres, 3); got != "$3" {
		t.Errorf("placeholder(postgres, 3) = %q, want $3", got)
	}
	if got := placeholder(query.DialectMySQL, 3); got != "?" {
		t.Errorf("placeholder(mysql, 3) = %q, want ?", got)
	}
	if got := placeholderList(query.DialectPostgres, 4); got != "$1, $2, $3, $4" {
		t.Errorf("placeholderList(postgres, 4) = %q", got)
	}
	if got := placeholderList(query.DialectMySQL, 4); got != "?, ?, ?, ?" {
		t.Errorf("placeholderList(mysql, 4) = %q", got)
	}
	if got := placeholderList(query.DialectPostgres, 0); got != "" {
		t.Errorf("placeholderList(postgres, 0) = %q, want empty", got)
	}
}

func TestCreateTableSQL(t *testing.T) {
	pg := createTableSQL(query.DialectPostgres)
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS migrations",
		"id SERIAL PRIMARY KEY",
		"migration VARCHAR(255) NOT NULL UNIQUE",
		"batch INTEGER NOT NULL",
		"checksum VARCHAR(64)",
		"executed_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP",
		"execution_time_ms INTEGER DEFAULT 0",
	} {
		if !strings.Contains(pg, want) {
			t.Errorf("postgres DDL missing %q:\n%s", want, pg)
		}
	}

	my := createTableSQL(query.DialectMySQL)
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS migrations",
		"id INT AUTO_INCREMENT PRIMARY KEY",
		"batch INT NOT NULL",
		"executed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP",
		"execution_time_ms INT DEFAULT 0",
	} {
		if !strings.Contains(my, want) {
			t.Errorf("mysql DDL missing %q:\n%s", want, my)
		}
	}
	if strings.Contains(my, "SERIAL") || strings.Contains(my, "TIMESTAMPTZ") {
		t.Errorf("mysql DDL contains PostgreSQL types:\n%s", my)
	}
	if createTableSQL(query.Dialect("mariadb")) != my {
		t.Error("mariadb should use the MySQL DDL")
	}
}
