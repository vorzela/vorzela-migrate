package migration

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/vorzela/vorzela-migrate/internal/db"
	"github.com/vorzela/vorzela-migrate/internal/output"
)

// Mock DB for testing
type mockDB struct{}

func (m *mockDB) Exec(ctx context.Context, query string, args ...interface{}) error {
	return nil
}

func (m *mockDB) Query(ctx context.Context, query string, args ...interface{}) (db.Rows, error) {
	return nil, nil
}

func (m *mockDB) QueryRow(ctx context.Context, query string, args ...interface{}) db.Row {
	return nil
}

func (m *mockDB) Ping(ctx context.Context) error {
	return nil
}

func (m *mockDB) Close() {
	// No-op for mock
}

func TestEnhanceError(t *testing.T) {
	// Create a mock enhanced executor
	executor := &EnhancedExecutor{
		conn:          &mockDB{},
		sqlDB:         &sql.DB{},
		dsn:           "postgres://test",
		migrationPath: "/test/migrations",
		dialect:       PostgreSQL,
		logger:        output.NewMigrationLogger(false),
	}

	tests := []struct {
		name           string
		err            error
		statementNum   int
		stmt           string
		wantContains   []string
		wantNotContain []string
	}{
		{
			name:         "missing auto_update_timestamp function",
			err:          errors.New("ERROR: function auto_update_timestamp() does not exist (SQLSTATE 42883)"),
			statementNum: 3,
			stmt:         "CREATE TRIGGER trigger_users_auto_update BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION auto_update_timestamp();",
			wantContains: []string{
				"statement 3 failed",
				"MISSING DATABASE FUNCTION",
				"auto_update_timestamp()",
				"vm functions migrate",
				"💡 SOLUTION:",
			},
			wantNotContain: []string{},
		},
		{
			name:         "missing protect_soft_deleted function",
			err:          errors.New("ERROR: function protect_soft_deleted() does not exist"),
			statementNum: 2,
			stmt:         "CREATE TRIGGER ... EXECUTE FUNCTION protect_soft_deleted();",
			wantContains: []string{
				"statement 2 failed",
				"MISSING DATABASE FUNCTION",
				"protect_soft_deleted()",
				"vm functions migrate",
			},
			wantNotContain: []string{},
		},
		{
			name:         "missing auto_update_with_soft_delete_protection function",
			err:          errors.New("function auto_update_with_soft_delete_protection() does not exist"),
			statementNum: 1,
			stmt:         "CREATE TRIGGER ...",
			wantContains: []string{
				"MISSING DATABASE FUNCTION",
				"auto_update_with_soft_delete_protection()",
			},
			wantNotContain: []string{},
		},
		{
			name:         "missing prevent_hard_delete function",
			err:          errors.New("FUNCTION prevent_hard_delete does not exist"),
			statementNum: 4,
			stmt:         "CREATE TRIGGER ...",
			wantContains: []string{
				"MISSING DATABASE FUNCTION",
				"prevent_hard_delete()",
			},
			wantNotContain: []string{},
		},
		{
			name:         "missing table error",
			err:          errors.New("ERROR: relation \"categories\" does not exist"),
			statementNum: 2,
			stmt:         "ALTER TABLE posts ADD CONSTRAINT fk_category FOREIGN KEY (category_id) REFERENCES categories(id);",
			wantContains: []string{
				"statement 2 failed",
				"💡 TIP:",
				"migration that hasn't run yet",
				"migration order",
			},
			wantNotContain: []string{
				"MISSING DATABASE FUNCTION",
				"vm functions migrate",
			},
		},
		{
			name:         "generic error",
			err:          errors.New("ERROR: syntax error at or near \"FROM\""),
			statementNum: 1,
			stmt:         "SELECT * FROM",
			wantContains: []string{
				"statement 1 failed",
				"Statement: SELECT * FROM",
			},
			wantNotContain: []string{
				"MISSING DATABASE FUNCTION",
				"vm functions migrate",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := executor.enhanceError(tt.err, tt.statementNum, tt.stmt)
			resultStr := result.Error()

			// Check that expected strings are present
			for _, want := range tt.wantContains {
				if !strings.Contains(resultStr, want) {
					t.Errorf("enhanceError() = %q\nwant to contain %q", resultStr, want)
				}
			}

			// Check that unwanted strings are not present
			for _, notWant := range tt.wantNotContain {
				if strings.Contains(resultStr, notWant) {
					t.Errorf("enhanceError() = %q\nshould NOT contain %q", resultStr, notWant)
				}
			}

			// Ensure statement number is mentioned
			if !strings.Contains(resultStr, "statement") {
				t.Error("enhanceError() should mention statement number")
			}
		})
	}
}

func TestEnhanceErrorAllTriggerFunctions(t *testing.T) {
	executor := &EnhancedExecutor{
		conn:          &mockDB{},
		sqlDB:         &sql.DB{},
		dsn:           "postgres://test",
		migrationPath: "/test/migrations",
		dialect:       PostgreSQL,
		logger:        output.NewMigrationLogger(false),
	}

	// Test that all known trigger functions are detected
	functions := []string{
		"auto_update_timestamp",
		"protect_soft_deleted",
		"auto_update_with_soft_delete_protection",
		"prevent_hard_delete",
	}

	for _, funcName := range functions {
		t.Run(funcName, func(t *testing.T) {
			err := errors.New("ERROR: function " + funcName + "() does not exist (SQLSTATE 42883)")
			result := executor.enhanceError(err, 1, "CREATE TRIGGER test ...")
			resultStr := result.Error()

			if !strings.Contains(resultStr, "MISSING DATABASE FUNCTION") {
				t.Errorf("Should detect missing function %s", funcName)
			}

			if !strings.Contains(resultStr, funcName+"()") {
				t.Errorf("Should mention function name %s()", funcName)
			}

			if !strings.Contains(resultStr, "vm functions migrate") {
				t.Error("Should provide solution to run 'vm functions migrate'")
			}

			if !strings.Contains(resultStr, "💡 SOLUTION:") {
				t.Error("Should include solution section")
			}
		})
	}
}

func TestEnhanceErrorCaseInsensitive(t *testing.T) {
	executor := &EnhancedExecutor{
		conn:          &mockDB{},
		sqlDB:         &sql.DB{},
		dsn:           "postgres://test",
		migrationPath: "/test/migrations",
		dialect:       PostgreSQL,
		logger:        output.NewMigrationLogger(false),
	}

	// Test different case variations
	testCases := []struct {
		name string
		err  error
	}{
		{"lowercase function", errors.New("function auto_update_timestamp() does not exist")},
		{"uppercase FUNCTION", errors.New("FUNCTION auto_update_timestamp() does not exist")},
		{"mixed case Function", errors.New("Function auto_update_timestamp() does not exist")},
		{"lowercase relation", errors.New("relation \"users\" does not exist")},
		{"uppercase RELATION", errors.New("RELATION \"users\" does not exist")},
		{"lowercase table", errors.New("table \"users\" does not exist")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := executor.enhanceError(tc.err, 1, "test statement")
			resultStr := result.Error()

			// Should detect and enhance the error regardless of case
			if !strings.Contains(resultStr, "statement 1 failed") {
				t.Error("Should detect error regardless of case")
			}

			// Should provide helpful tips
			hasHelpfulTip := strings.Contains(resultStr, "💡") ||
				strings.Contains(resultStr, "SOLUTION") ||
				strings.Contains(resultStr, "TIP")

			if !hasHelpfulTip {
				t.Error("Should provide helpful tips for detected errors")
			}
		})
	}
}
