package migration

import (
	"database/sql"
	"strings"
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

	// AddedColumns (in DB but not in migrations) must produce DROP COLUMN statements.
	foundDrop := false
	for _, stmt := range statements {
		if strings.Contains(strings.ToUpper(stmt), "DROP COLUMN") {
			foundDrop = true
		}
	}

	if !foundDrop {
		t.Errorf("Expected DROP COLUMN statement for AddedColumns, got: %v", statements)
	}
}

func TestGenerateAlterStatements_ExtraIndexesDroppedFirst(t *testing.T) {
	inspector := &SchemaInspector{dialect: PostgreSQL}

	drift := &SchemaDrift{
		Table: "users",
		ExtraIndexes: []IndexInfo{
			{Name: "idx_users_email_verified_at", TableName: "users", Columns: []string{"email_verified_at"}, IsUnique: false, IndexType: "btree"},
		},
		AddedColumns: []ColumnInfo{
			{Name: "email_verified_at", Type: "timestamp", Nullable: true},
		},
	}

	statements := inspector.GenerateAlterStatements(drift)

	if len(statements) < 2 {
		t.Fatalf("Expected at least 2 statements (DROP INDEX + DROP COLUMN), got %d: %v", len(statements), statements)
	}

	// First statement must be DROP INDEX.
	if !strings.Contains(strings.ToUpper(statements[0]), "DROP INDEX") {
		t.Errorf("Expected first statement to be DROP INDEX, got: %s", statements[0])
	}
	// Second must be DROP COLUMN.
	if !strings.Contains(strings.ToUpper(statements[1]), "DROP COLUMN") {
		t.Errorf("Expected second statement to be DROP COLUMN, got: %s", statements[1])
	}
}

func TestIndexCoveredByOrphans(t *testing.T) {
	orphanedSet := map[string]bool{"old_col": true, "extra_col": true}

	tests := []struct {
		name string
		idx  IndexInfo
		want bool
	}{
		{
			name: "all columns orphaned",
			idx:  IndexInfo{Columns: []string{"old_col", "extra_col"}},
			want: true,
		},
		{
			name: "partial match — not fully orphaned",
			idx:  IndexInfo{Columns: []string{"old_col", "keep_col"}},
			want: false,
		},
		{
			name: "no columns in index",
			idx:  IndexInfo{Columns: []string{}},
			want: false,
		},
		{
			name: "single orphaned column",
			idx:  IndexInfo{Columns: []string{"old_col"}},
			want: true,
		},
		{
			name: "case insensitive match",
			idx:  IndexInfo{Columns: []string{"OLD_COL"}},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := indexCoveredByOrphans(tt.idx, orphanedSet)
			if got != tt.want {
				t.Errorf("indexCoveredByOrphans() = %v, want %v", got, tt.want)
			}
		})
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

// ---------------------------------------------------------------------------
// Tests for NOT NULL / DEFAULT / UNIQUE aware MissingColumns generation
// ---------------------------------------------------------------------------

func TestGenerateAlterStatements_MissingColumn_NullableNoDefault(t *testing.T) {
	inspector := &SchemaInspector{dialect: PostgreSQL}
	drift := &SchemaDrift{
		Table: "users",
		MissingColumns: []ColumnInfo{
			{Name: "bio", Type: "text", Nullable: true},
		},
	}
	stmts := inspector.GenerateAlterStatements(drift)
	if len(stmts) != 1 {
		t.Fatalf("want 1 statement, got %d: %v", len(stmts), stmts)
	}
	stmt := stmts[0]
	if !strings.Contains(strings.ToUpper(stmt), "ADD COLUMN") {
		t.Errorf("expected ADD COLUMN, got: %s", stmt)
	}
	if strings.Contains(strings.ToUpper(stmt), "NOT NULL") {
		t.Errorf("should not contain NOT NULL for nullable column, got: %s", stmt)
	}
}

func TestGenerateAlterStatements_MissingColumn_NotNullWithDefault(t *testing.T) {
	inspector := &SchemaInspector{dialect: PostgreSQL}
	drift := &SchemaDrift{
		Table: "roles",
		MissingColumns: []ColumnInfo{
			{Name: "is_system", Type: "boolean", Nullable: false,
				Default: sql.NullString{String: "false", Valid: true}},
		},
	}
	stmts := inspector.GenerateAlterStatements(drift)
	if len(stmts) != 1 {
		t.Fatalf("want 1 statement, got %d: %v", len(stmts), stmts)
	}
	stmt := stmts[0]
	if !strings.Contains(strings.ToUpper(stmt), "NOT NULL") {
		t.Errorf("expected NOT NULL in statement, got: %s", stmt)
	}
	if !strings.Contains(stmt, "DEFAULT false") {
		t.Errorf("expected DEFAULT false in statement, got: %s", stmt)
	}
}

func TestGenerateAlterStatements_MissingColumn_NotNullNoDefault(t *testing.T) {
	inspector := &SchemaInspector{dialect: PostgreSQL}
	drift := &SchemaDrift{
		Table: "orders",
		MissingColumns: []ColumnInfo{
			{Name: "status", Type: "varchar(50)", Nullable: false},
		},
	}
	stmts := inspector.GenerateAlterStatements(drift)
	if len(stmts) != 1 {
		t.Fatalf("want 1 advisory comment, got %d: %v", len(stmts), stmts)
	}
	stmt := stmts[0]
	if !strings.HasPrefix(strings.TrimSpace(stmt), "--") {
		t.Errorf("expected advisory comment for NOT NULL without DEFAULT, got: %s", stmt)
	}
	if !strings.Contains(stmt, "add_") {
		t.Errorf("expected add_ migration hint in advisory, got: %s", stmt)
	}
}

func TestGenerateAlterStatements_MissingColumn_Unique(t *testing.T) {
	// Nullable UNIQUE columns are safe to auto-add: NULLs never violate UNIQUE.
	inspector := &SchemaInspector{dialect: PostgreSQL}
	drift := &SchemaDrift{
		Table: "users",
		MissingColumns: []ColumnInfo{
			{Name: "email", Type: "varchar(255)", Nullable: true, IsUnique: true},
		},
	}
	stmts := inspector.GenerateAlterStatements(drift)
	if len(stmts) != 1 {
		t.Fatalf("want 1 ALTER TABLE statement, got %d: %v", len(stmts), stmts)
	}
	stmt := stmts[0]
	if strings.HasPrefix(strings.TrimSpace(stmt), "--") {
		t.Errorf("expected ALTER TABLE ADD COLUMN for nullable UNIQUE column, got advisory: %s", stmt)
	}
	if !strings.Contains(stmt, "ADD COLUMN") {
		t.Errorf("expected ADD COLUMN statement for nullable UNIQUE column, got: %s", stmt)
	}
}

func TestGenerateAlterStatements_MissingColumn_UniqueNotNull(t *testing.T) {
	// Non-nullable UNIQUE columns cannot be safely auto-added to a populated
	// table — an advisory comment must be emitted instead.
	inspector := &SchemaInspector{dialect: PostgreSQL}
	drift := &SchemaDrift{
		Table: "users",
		MissingColumns: []ColumnInfo{
			{Name: "username", Type: "varchar(100)", Nullable: false, IsUnique: true},
		},
	}
	stmts := inspector.GenerateAlterStatements(drift)
	if len(stmts) != 1 {
		t.Fatalf("want 1 advisory comment, got %d: %v", len(stmts), stmts)
	}
	stmt := stmts[0]
	if !strings.HasPrefix(strings.TrimSpace(stmt), "--") {
		t.Errorf("expected advisory comment for non-nullable UNIQUE column, got: %s", stmt)
	}
	if !strings.Contains(stmt, "add_") {
		t.Errorf("expected add_ migration hint in advisory, got: %s", stmt)
	}
}

// ---------------------------------------------------------------------------
// Tests for FK constraint generation (GenerateAddConstraintStatements)
// ---------------------------------------------------------------------------

func TestGenerateAddConstraintStatements_Basic(t *testing.T) {
	inspector := &SchemaInspector{dialect: PostgreSQL}
	drift := &SchemaDrift{
		Table: "orders",
		MissingConstraints: []ConstraintInfo{
			{
				Name:       "fk_orders_user_id",
				TableName:  "orders",
				Columns:    []string{"user_id"},
				RefTable:   "users",
				RefColumns: []string{"id"},
			},
		},
	}
	stmts := inspector.GenerateAddConstraintStatements(drift)
	if len(stmts) != 1 {
		t.Fatalf("want 1 statement, got %d: %v", len(stmts), stmts)
	}
	stmt := stmts[0]
	if !strings.Contains(strings.ToUpper(stmt), "ADD CONSTRAINT") {
		t.Errorf("expected ADD CONSTRAINT, got: %s", stmt)
	}
	if !strings.Contains(strings.ToUpper(stmt), "FOREIGN KEY") {
		t.Errorf("expected FOREIGN KEY, got: %s", stmt)
	}
	if !strings.Contains(stmt, "fk_orders_user_id") {
		t.Errorf("expected constraint name fk_orders_user_id, got: %s", stmt)
	}
	if !strings.Contains(stmt, "users") {
		t.Errorf("expected references to users table, got: %s", stmt)
	}
}

func TestGenerateAddConstraintStatements_WithOnDelete(t *testing.T) {
	inspector := &SchemaInspector{dialect: PostgreSQL}
	drift := &SchemaDrift{
		Table: "order_items",
		MissingConstraints: []ConstraintInfo{
			{
				Name:       "fk_order_items_order_id",
				TableName:  "order_items",
				Columns:    []string{"order_id"},
				RefTable:   "orders",
				RefColumns: []string{"id"},
				OnDelete:   "CASCADE",
			},
		},
	}
	stmts := inspector.GenerateAddConstraintStatements(drift)
	if len(stmts) != 1 {
		t.Fatalf("want 1 statement, got %d: %v", len(stmts), stmts)
	}
	stmt := stmts[0]
	if !strings.Contains(stmt, "ON DELETE CASCADE") {
		t.Errorf("expected ON DELETE CASCADE, got: %s", stmt)
	}
}

func TestGenerateAddConstraintStatements_OnDeleteNoAction_Omitted(t *testing.T) {
	inspector := &SchemaInspector{dialect: PostgreSQL}
	drift := &SchemaDrift{
		Table: "orders",
		MissingConstraints: []ConstraintInfo{
			{
				Name:       "fk_orders_user_id",
				TableName:  "orders",
				Columns:    []string{"user_id"},
				RefTable:   "users",
				RefColumns: []string{"id"},
				OnDelete:   "NO ACTION",
				OnUpdate:   "NO ACTION",
			},
		},
	}
	stmts := inspector.GenerateAddConstraintStatements(drift)
	if len(stmts) != 1 {
		t.Fatalf("want 1 statement, got %d: %v", len(stmts), stmts)
	}
	stmt := stmts[0]
	// NO ACTION is the default — should be omitted from the output
	if strings.Contains(stmt, "NO ACTION") {
		t.Errorf("NO ACTION should be omitted (it is the default), got: %s", stmt)
	}
}

func TestGenerateAddConstraintStatements_MultiColumnFK(t *testing.T) {
	inspector := &SchemaInspector{dialect: PostgreSQL}
	drift := &SchemaDrift{
		Table: "order_items",
		MissingConstraints: []ConstraintInfo{
			{
				Name:       "fk_order_items_composite",
				TableName:  "order_items",
				Columns:    []string{"order_id", "product_id"},
				RefTable:   "products",
				RefColumns: []string{"id", "variant_id"},
			},
		},
	}
	stmts := inspector.GenerateAddConstraintStatements(drift)
	if len(stmts) != 1 {
		t.Fatalf("want 1 statement, got %d: %v", len(stmts), stmts)
	}
	stmt := stmts[0]
	if !strings.Contains(stmt, "order_id, product_id") {
		t.Errorf("expected composite local columns, got: %s", stmt)
	}
	if !strings.Contains(stmt, "id, variant_id") {
		t.Errorf("expected composite referenced columns, got: %s", stmt)
	}
}

func TestGenerateAddConstraintStatements_AutoName(t *testing.T) {
	inspector := &SchemaInspector{dialect: PostgreSQL}
	drift := &SchemaDrift{
		Table: "orders",
		MissingConstraints: []ConstraintInfo{
			{
				// Name intentionally empty — should be auto-generated
				TableName:  "orders",
				Columns:    []string{"customer_id"},
				RefTable:   "customers",
				RefColumns: []string{"id"},
			},
		},
	}
	stmts := inspector.GenerateAddConstraintStatements(drift)
	if len(stmts) != 1 {
		t.Fatalf("want 1 statement, got %d: %v", len(stmts), stmts)
	}
	stmt := stmts[0]
	// Auto-generated name follows fk_<table>_<col> convention
	if !strings.Contains(stmt, "fk_orders_customer_id") {
		t.Errorf("expected auto-generated name fk_orders_customer_id, got: %s", stmt)
	}
}

func TestGenerateAddConstraintStatements_Empty(t *testing.T) {
	inspector := &SchemaInspector{dialect: PostgreSQL}
	drift := &SchemaDrift{Table: "users"}
	stmts := inspector.GenerateAddConstraintStatements(drift)
	if stmts != nil {
		t.Errorf("expected nil for empty MissingConstraints, got: %v", stmts)
	}
}

func TestGenerateDropConstraintStatements_Postgres(t *testing.T) {
	inspector := &SchemaInspector{dialect: PostgreSQL}
	drift := &SchemaDrift{
		Table: "orders",
		ExtraConstraints: []ConstraintInfo{
			{Name: "fk_orders_user_id", TableName: "orders"},
		},
	}
	stmts := inspector.GenerateDropConstraintStatements(drift)
	if len(stmts) != 1 {
		t.Fatalf("want 1 statement, got %d: %v", len(stmts), stmts)
	}
	stmt := stmts[0]
	if !strings.Contains(strings.ToUpper(stmt), "DROP CONSTRAINT") {
		t.Errorf("expected DROP CONSTRAINT (Postgres), got: %s", stmt)
	}
	if strings.Contains(strings.ToUpper(stmt), "FOREIGN KEY") {
		t.Errorf("Postgres should use DROP CONSTRAINT, not DROP FOREIGN KEY, got: %s", stmt)
	}
}

func TestGenerateDropConstraintStatements_MySQL(t *testing.T) {
	inspector := &SchemaInspector{dialect: MySQL}
	drift := &SchemaDrift{
		Table: "orders",
		ExtraConstraints: []ConstraintInfo{
			{Name: "fk_orders_user_id", TableName: "orders"},
		},
	}
	stmts := inspector.GenerateDropConstraintStatements(drift)
	if len(stmts) != 1 {
		t.Fatalf("want 1 statement, got %d: %v", len(stmts), stmts)
	}
	stmt := stmts[0]
	if !strings.Contains(strings.ToUpper(stmt), "DROP FOREIGN KEY") {
		t.Errorf("expected DROP FOREIGN KEY (MySQL), got: %s", stmt)
	}
}

func TestConstraintKey_Normalisation(t *testing.T) {
	ci1 := ConstraintInfo{TableName: "Orders", Columns: []string{"user_id", "product_id"}, RefTable: "Users"}
	ci2 := ConstraintInfo{TableName: "orders", Columns: []string{"product_id", "user_id"}, RefTable: "users"}

	// Keys should be equal regardless of column order or case
	if constraintKey(ci1) != constraintKey(ci2) {
		t.Errorf("constraintKey mismatch: %q vs %q", constraintKey(ci1), constraintKey(ci2))
	}
}

func TestGenerateAllStatements_IncludesConstraints(t *testing.T) {
	inspector := &SchemaInspector{dialect: PostgreSQL}
	drift := &SchemaDrift{
		Table: "orders",
		MissingColumns: []ColumnInfo{
			{Name: "note", Type: "text", Nullable: true},
		},
		MissingConstraints: []ConstraintInfo{
			{
				Name: "fk_orders_user_id", TableName: "orders",
				Columns: []string{"user_id"}, RefTable: "users", RefColumns: []string{"id"},
			},
		},
	}
	stmts := inspector.GenerateAllStatements(drift)

	hasAddColumn := false
	hasAddConstraint := false
	for _, s := range stmts {
		up := strings.ToUpper(s)
		if strings.Contains(up, "ADD COLUMN") {
			hasAddColumn = true
		}
		if strings.Contains(up, "ADD CONSTRAINT") {
			hasAddConstraint = true
		}
	}
	if !hasAddColumn {
		t.Errorf("expected ADD COLUMN statement in GenerateAllStatements output: %v", stmts)
	}
	if !hasAddConstraint {
		t.Errorf("expected ADD CONSTRAINT statement in GenerateAllStatements output: %v", stmts)
	}
}

func TestSchemaDrift_ConstraintFields(t *testing.T) {
	drift := &SchemaDrift{
		Table: "orders",
		MissingConstraints: []ConstraintInfo{
			{Name: "fk_a", TableName: "orders", Columns: []string{"user_id"}, RefTable: "users", RefColumns: []string{"id"}},
		},
		ExtraConstraints: []ConstraintInfo{
			{Name: "fk_b", TableName: "orders", Columns: []string{"old_ref_id"}, RefTable: "old_table", RefColumns: []string{"id"}},
		},
	}
	if len(drift.MissingConstraints) != 1 {
		t.Errorf("MissingConstraints count = %d, want 1", len(drift.MissingConstraints))
	}
	if len(drift.ExtraConstraints) != 1 {
		t.Errorf("ExtraConstraints count = %d, want 1", len(drift.ExtraConstraints))
	}
	if drift.MissingConstraints[0].Name != "fk_a" {
		t.Errorf("MissingConstraints[0].Name = %q, want fk_a", drift.MissingConstraints[0].Name)
	}
}
