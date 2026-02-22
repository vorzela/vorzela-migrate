package migration

import (
	"strings"
	"testing"
)

func TestParseEnumValues(t *testing.T) {
	tests := []struct {
		name string
		stmt string
		want []string
	}{
		{
			name: "simple single line",
			stmt: "CREATE TYPE user_status AS ENUM ('active', 'inactive', 'suspended');",
			want: []string{"active", "inactive", "suspended"},
		},
		{
			name: "multi-line statement",
			stmt: `CREATE TYPE order_status AS ENUM (
				'pending',
				'processing',
				'shipped',
				'delivered',
				'cancelled'
			);`,
			want: []string{"pending", "processing", "shipped", "delivered", "cancelled"},
		},
		{
			name: "single value",
			stmt: "CREATE TYPE flag AS ENUM ('yes');",
			want: []string{"yes"},
		},
		{
			name: "values with spaces",
			stmt: "CREATE TYPE priority AS ENUM ('very high', 'high', 'low');",
			want: []string{"very high", "high", "low"},
		},
		{
			name: "empty parens",
			stmt: "CREATE TYPE empty AS ENUM ();",
			want: nil,
		},
		{
			name: "no parens",
			stmt: "CREATE TYPE broken",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseEnumValues(tt.stmt)
			if len(got) != len(tt.want) {
				t.Errorf("ParseEnumValues() got %v, want %v", got, tt.want)
				return
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("ParseEnumValues()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestGenerateEnumSyncSQL(t *testing.T) {
	t.Run("creates type when values provided", func(t *testing.T) {
		sql := GenerateEnumSyncSQL("user_status", []string{"active", "inactive"})
		if sql == "" {
			t.Fatal("expected non-empty SQL")
		}
		if !strings.Contains(sql, "CREATE TYPE user_status AS ENUM") {
			t.Errorf("SQL should contain CREATE TYPE; got:\n%s", sql)
		}
		if !strings.Contains(sql, "ADD VALUE IF NOT EXISTS 'active'") {
			t.Errorf("SQL should contain ADD VALUE for 'active'; got:\n%s", sql)
		}
		if !strings.Contains(sql, "ADD VALUE IF NOT EXISTS 'inactive'") {
			t.Errorf("SQL should contain ADD VALUE for 'inactive'; got:\n%s", sql)
		}
	})

	t.Run("empty values returns empty string", func(t *testing.T) {
		sql := GenerateEnumSyncSQL("empty_type", nil)
		if sql != "" {
			t.Errorf("expected empty SQL for nil values, got: %s", sql)
		}
	})

	t.Run("escapes single quotes in values", func(t *testing.T) {
		sql := GenerateEnumSyncSQL("tricky", []string{"it's"})
		if !strings.Contains(sql, "'it''s'") {
			t.Errorf("expected escaped single quote in SQL; got:\n%s", sql)
		}
	})

	t.Run("contains both CREATE and ADD VALUE branches", func(t *testing.T) {
		sql := GenerateEnumSyncSQL("status", []string{"a", "b", "c"})
		if !strings.Contains(sql, "IF NOT EXISTS") {
			t.Errorf("SQL should include existence check; got:\n%s", sql)
		}
		if !strings.Contains(sql, "ELSE") {
			t.Errorf("SQL should have ELSE branch for ALTER TYPE; got:\n%s", sql)
		}
	})
}

// TestParseAllEnumNames_SyncBehaviour verifies that editing an enum (adding a
// new value by changing the line in enums.sql) results in the type staying in
// the enabled list rather than being treated as disabled.
func TestParseAllEnumNames_SyncBehaviour(t *testing.T) {
	// User edits enums.sql: old line commented out, new line active
	content := `
-- original with 2 values (now replaced)
-- CREATE TYPE user_status AS ENUM ('active', 'inactive');

-- updated with 3 values (active)
CREATE TYPE user_status AS ENUM ('active', 'inactive', 'suspended');
`
	enabled, disabled := ParseAllEnumNames(content)

	// user_status should appear in enabled (the active CREATE TYPE)
	foundEnabled := false
	for _, n := range enabled {
		if n == "user_status" {
			foundEnabled = true
		}
	}
	if !foundEnabled {
		t.Errorf("user_status should be enabled; enabled=%v, disabled=%v", enabled, disabled)
	}

	// user_status may also appear in disabled (old commented line), but since
	// the active line takes precedence for syncing this is acceptable.
	// The important thing is it's not ONLY in disabled.
}

// TestExtractEnumStatement_MultiLine verifies multi-line CREATE TYPE is captured.
func TestExtractEnumStatement_MultiLine(t *testing.T) {
	content := `
CREATE TYPE order_status AS ENUM (
    'pending',
    'processing',
    'shipped',
    'delivered'
);
`
	stmt := ExtractEnumStatement(content, "order_status")
	if stmt == "" {
		t.Fatal("expected non-empty statement")
	}
	vals := ParseEnumValues(stmt)
	wantCount := 4
	if len(vals) != wantCount {
		t.Errorf("expected %d values, got %d: %v", wantCount, len(vals), vals)
	}
}
