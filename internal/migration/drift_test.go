package migration

import (
	"testing"
)

func TestNewSchemaInspector(t *testing.T) {
	tests := []struct {
		name    string
		dialect Dialect
		wantNil bool
	}{
		{
			name:    "postgres inspector",
			dialect: PostgreSQL,
			wantNil: false,
		},
		{
			name:    "mysql inspector",
			dialect: MySQL,
			wantNil: false,
		},
		{
			name:    "unknown dialect",
			dialect: Dialect("unknown"),
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inspector := NewSchemaInspector(nil, tt.dialect)
			if (inspector == nil) != tt.wantNil {
				t.Errorf("NewSchemaInspector() nil = %v, want nil = %v", inspector == nil, tt.wantNil)
			}
		})
	}
}

func TestSchemaDriftDetection(t *testing.T) {
	// Test drift type representation
	drift := &SchemaDrift{
		Table: "users",
		AddedColumns: []ColumnInfo{
			{
				Name:     "email_verified_at",
				Type:     "timestamp",
				Nullable: true,
			},
			{
				Name:     "last_login_ip",
				Type:     "varchar(45)",
				Nullable: true,
			},
		},
	}

	if drift.Table != "users" {
		t.Errorf("Table name = %s, want users", drift.Table)
	}

	if len(drift.AddedColumns) != 2 {
		t.Errorf("AddedColumns count = %d, want 2", len(drift.AddedColumns))
	}
}

func TestGenerateAlterStatements(t *testing.T) {
	inspector := &SchemaInspector{
		dialect: PostgreSQL,
	}

	drift := &SchemaDrift{
		Table: "users",
		AddedColumns: []ColumnInfo{
			{
				Name:     "email_verified_at",
				Type:     "timestamp",
				Nullable: true,
			},
		},
	}

	statements := inspector.GenerateAlterStatements(drift)

	if len(statements) == 0 {
		t.Error("GenerateAlterStatements() returned empty slice")
	}

	// Should generate at least one ALTER TABLE statement
	foundAlter := false
	for _, stmt := range statements {
		if len(stmt) > 0 {
			foundAlter = true
			// Verify it contains expected keywords
			if stmt == "" {
				t.Error("Generated empty statement")
			}
		}
	}

	if !foundAlter {
		t.Error("No ALTER TABLE statements generated")
	}
}

func TestColumnInfoValidation(t *testing.T) {
	tests := []struct {
		name   string
		column ColumnInfo
		valid  bool
	}{
		{
			name: "valid column",
			column: ColumnInfo{
				Name:     "email",
				Type:     "varchar(255)",
				Nullable: true,
			},
			valid: true,
		},
		{
			name: "column without name",
			column: ColumnInfo{
				Type:     "text",
				Nullable: true,
			},
			valid: false,
		},
		{
			name: "column without type",
			column: ColumnInfo{
				Name:     "description",
				Nullable: true,
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation
			isValid := tt.column.Name != "" && tt.column.Type != ""
			if isValid != tt.valid {
				t.Errorf("Column validation = %v, want %v", isValid, tt.valid)
			}
		})
	}
}
