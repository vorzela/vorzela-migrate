package migration

import (
	"errors"
	"strings"
	"testing"
)

func TestEnhanceMigrationError(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		filename       string
		wantContains   []string
		wantNotContain []string
	}{
		{
			name:     "missing auto_update_timestamp function",
			err:      errors.New("ERROR: function auto_update_timestamp() does not exist (SQLSTATE 42883)"),
			filename: "20240218_create_users_table.sql",
			wantContains: []string{
				"MISSING DATABASE FUNCTION",
				"auto_update_timestamp()",
				"vm functions migrate",
				"💡 SOLUTION:",
			},
			wantNotContain: []string{},
		},
		{
			name:     "missing protect_soft_deleted function",
			err:      errors.New("ERROR: function protect_soft_deleted() does not exist (SQLSTATE 42883)"),
			filename: "20240218_create_posts_table.sql",
			wantContains: []string{
				"MISSING DATABASE FUNCTION",
				"protect_soft_deleted()",
				"vm functions migrate",
			},
			wantNotContain: []string{},
		},
		{
			name:     "missing auto_update_with_soft_delete_protection function",
			err:      errors.New("ERROR: function auto_update_with_soft_delete_protection() does not exist"),
			filename: "migration.sql",
			wantContains: []string{
				"MISSING DATABASE FUNCTION",
				"auto_update_with_soft_delete_protection()",
				"vm functions migrate",
			},
			wantNotContain: []string{},
		},
		{
			name:     "missing prevent_hard_delete function",
			err:      errors.New("ERROR: function prevent_hard_delete() does not exist"),
			filename: "migration.sql",
			wantContains: []string{
				"MISSING DATABASE FUNCTION",
				"prevent_hard_delete()",
				"vm functions migrate",
			},
			wantNotContain: []string{},
		},
		{
			name:     "missing table/relation error",
			err:      errors.New("ERROR: relation \"users\" does not exist"),
			filename: "20240218_create_posts_table.sql",
			wantContains: []string{
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
			name:     "generic SQL error",
			err:      errors.New("ERROR: syntax error at or near \"SELECT\""),
			filename: "20240218_migration.sql",
			wantContains: []string{
				"MIGRATION FAILED",
				"Transaction automatically rolled back",
				"Fix the SQL",
			},
			wantNotContain: []string{
				"MISSING DATABASE FUNCTION",
				"vm functions migrate",
			},
		},
		{
			name:     "constraint violation error",
			err:      errors.New("ERROR: duplicate key value violates unique constraint"),
			filename: "migration.sql",
			wantContains: []string{
				"MIGRATION FAILED",
				"Transaction automatically rolled back",
			},
			wantNotContain: []string{
				"MISSING DATABASE FUNCTION",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := enhanceMigrationError(tt.err, tt.filename)

			// Check that expected strings are present
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("enhanceMigrationError() = %q\nwant to contain %q", got, want)
				}
			}

			// Check that unwanted strings are not present
			for _, notWant := range tt.wantNotContain {
				if strings.Contains(got, notWant) {
					t.Errorf("enhanceMigrationError() = %q\nshould NOT contain %q", got, notWant)
				}
			}

			// Ensure filename is mentioned
			if !strings.Contains(got, tt.filename) {
				t.Errorf("enhanceMigrationError() should mention filename %q", tt.filename)
			}
		})
	}
}

func TestEnhanceMigrationErrorAllFunctions(t *testing.T) {
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
			result := enhanceMigrationError(err, "test_migration.sql")

			if !strings.Contains(result, "MISSING DATABASE FUNCTION") {
				t.Errorf("Should detect missing function %s", funcName)
			}

			if !strings.Contains(result, funcName+"()") {
				t.Errorf("Should mention function name %s()", funcName)
			}

			if !strings.Contains(result, "vm functions migrate") {
				t.Error("Should provide solution to run 'vm functions migrate'")
			}
		})
	}
}

func TestEnhanceMigrationErrorFormat(t *testing.T) {
	// Test that the error message is properly formatted
	err := errors.New("ERROR: function auto_update_timestamp() does not exist")
	filename := "20240218_create_users_table.sql"

	result := enhanceMigrationError(err, filename)

	// Should have proper structure
	requiredElements := []string{
		"❌ MIGRATION FAILED:",
		"Reason:",
		"💡 SOLUTION:",
		"1. Run: vm functions migrate",
		"2. Then run: vm migrate",
	}

	for _, element := range requiredElements {
		if !strings.Contains(result, element) {
			t.Errorf("Error message missing required element: %q\nGot: %s", element, result)
		}
	}
}
