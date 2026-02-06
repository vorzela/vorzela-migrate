package migration

import "testing"

func TestDetectDialect(t *testing.T) {
	cases := map[string]Dialect{
		"postgres://user:pass@localhost:5432/db": PostgreSQL,
		"mysql://user:pass@localhost:3306/db":    MySQL,
		"cassandra://127.0.0.1:9042/keyspace":    Cassandra,
		"scylla://127.0.0.1:9042/keyspace":       Cassandra,
	}

	for input, want := range cases {
		got := DetectDialect(input)
		if got != want {
			t.Fatalf("DetectDialect(%s) = %v, want %v", input, got, want)
		}
	}
}

func TestCreateMigrationTableSQL_Cassandra(t *testing.T) {
	sql := CreateMigrationTableSQL(Cassandra)
	if sql == "" {
		t.Fatal("expected non-empty SQL for Cassandra")
	}
	if !contains(sql, "PRIMARY KEY") || !contains(sql, "batch") {
		t.Fatalf("unexpected cassandra migrations SQL: %s", sql)
	}
}

// small helper to avoid importing strings in test
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (len(s) == len(sub) && s == sub || len(s) > len(sub) && (indexOf(s, sub) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
