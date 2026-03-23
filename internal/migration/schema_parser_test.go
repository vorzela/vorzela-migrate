package migration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractColumnsFromSQL_CreateTable(t *testing.T) {
	sql := `
CREATE TABLE IF NOT EXISTS locations (
    id BIGSERIAL PRIMARY KEY,
    country_code VARCHAR(10) NOT NULL,
    parent_id BIGINT,
    type VARCHAR(50) NOT NULL,
    code VARCHAR(50) NOT NULL,
    name VARCHAR(100) NOT NULL,
    level INTEGER NOT NULL,
    is_active BOOLEAN DEFAULT true,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    deleted_at TIMESTAMP
);
`
	result := extractColumnsFromSQL(sql)
	cols, ok := result["locations"]
	if !ok {
		t.Fatal("expected 'locations' table in result")
	}

	expected := []string{"id", "country_code", "parent_id", "type", "code", "name", "level", "is_active", "metadata", "created_at", "updated_at", "deleted_at"}
	colSet := make(map[string]bool)
	for _, c := range cols {
		colSet[c] = true
	}
	for _, want := range expected {
		if !colSet[want] {
			t.Errorf("missing expected column %q in result %v", want, cols)
		}
	}
}

func TestExtractColumnsFromSQL_AlterTableAdd(t *testing.T) {
	sql := `
ALTER TABLE users ADD COLUMN phone VARCHAR(20);
ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar TEXT;
ALTER TABLE roles ADD is_system BOOLEAN NOT NULL DEFAULT false;
`
	result := extractColumnsFromSQL(sql)

	userCols, ok := result["users"]
	if !ok {
		t.Fatal("expected 'users' table in result")
	}
	userSet := make(map[string]bool)
	for _, c := range userCols {
		userSet[c] = true
	}
	if !userSet["phone"] {
		t.Errorf("missing 'phone' in users columns: %v", userCols)
	}
	if !userSet["avatar"] {
		t.Errorf("missing 'avatar' in users columns: %v", userCols)
	}

	roleCols, ok := result["roles"]
	if !ok {
		t.Fatal("expected 'roles' table in result")
	}
	roleSet := make(map[string]bool)
	for _, c := range roleCols {
		roleSet[c] = true
	}
	if !roleSet["is_system"] {
		t.Errorf("missing 'is_system' in roles columns: %v", roleCols)
	}
}

func TestExtractColumnsFromSQL_SkipsConstraints(t *testing.T) {
	sql := `
CREATE TABLE permissions (
    id BIGSERIAL,
    resource VARCHAR(100) NOT NULL,
    module VARCHAR(100) NOT NULL,
    description TEXT,
    PRIMARY KEY (id),
    UNIQUE (resource, module),
    CONSTRAINT chk_resource CHECK (resource <> ''),
    FOREIGN KEY (module) REFERENCES modules(name)
);
`
	result := extractColumnsFromSQL(sql)
	cols, ok := result["permissions"]
	if !ok {
		t.Fatal("expected 'permissions' table in result")
	}
	colSet := make(map[string]bool)
	for _, c := range cols {
		colSet[c] = true
	}

	expectedPresent := []string{"id", "resource", "module", "description"}
	for _, want := range expectedPresent {
		if !colSet[want] {
			t.Errorf("missing expected column %q", want)
		}
	}

	constraintNames := []string{"primary", "unique", "constraint", "foreign"}
	for _, bad := range constraintNames {
		if colSet[bad] {
			t.Errorf("constraint keyword %q should not appear as a column", bad)
		}
	}
}

func TestExtractColumnsFromSQL_MultipleTablesInOneMigration(t *testing.T) {
	sql := `
CREATE TABLE roles (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    slug VARCHAR(100) NOT NULL UNIQUE,
    is_system BOOLEAN NOT NULL DEFAULT false
);

CREATE TABLE permission_role (
    permission_id BIGINT NOT NULL,
    role_id BIGINT NOT NULL,
    PRIMARY KEY (permission_id, role_id)
);
`
	result := extractColumnsFromSQL(sql)

	if _, ok := result["roles"]; !ok {
		t.Fatal("missing 'roles' table")
	}
	if _, ok := result["permission_role"]; !ok {
		t.Fatal("missing 'permission_role' table")
	}

	rolesSet := make(map[string]bool)
	for _, c := range result["roles"] {
		rolesSet[c] = true
	}
	for _, want := range []string{"name", "slug", "is_system"} {
		if !rolesSet[want] {
			t.Errorf("missing %q in roles", want)
		}
	}

	prSet := make(map[string]bool)
	for _, c := range result["permission_role"] {
		prSet[c] = true
	}
	for _, want := range []string{"permission_id", "role_id"} {
		if !prSet[want] {
			t.Errorf("missing %q in permission_role", want)
		}
	}
}

func TestExtractColumnsFromSQL_AlterTableDrop(t *testing.T) {
	sql := `
ALTER TABLE users ADD COLUMN temp_col TEXT;
ALTER TABLE users DROP COLUMN temp_col;
`
	result := extractColumnsFromSQL(sql)
	for _, c := range result["users"] {
		if c == "temp_col" {
			t.Error("dropped column 'temp_col' should not appear in expected columns")
		}
	}
}

// ---------------------------------------------------------------------------
// Tests for NOT NULL / DEFAULT / UNIQUE constraint extraction
// ---------------------------------------------------------------------------

func TestParseColumnConstraints_Nullable(t *testing.T) {
	nullable, defVal, isUnique := parseColumnConstraints("bio TEXT")
	if !nullable {
		t.Error("expected nullable=true for column without NOT NULL")
	}
	if defVal.Valid {
		t.Errorf("expected no default, got %q", defVal.String)
	}
	if isUnique {
		t.Error("expected isUnique=false")
	}
}

func TestParseColumnConstraints_NotNullWithDefault(t *testing.T) {
	nullable, defVal, isUnique := parseColumnConstraints("is_system BOOLEAN NOT NULL DEFAULT false")
	if nullable {
		t.Error("expected nullable=false for NOT NULL column")
	}
	if !defVal.Valid || defVal.String != "false" {
		t.Errorf("expected default 'false', got valid=%v value=%q", defVal.Valid, defVal.String)
	}
	if isUnique {
		t.Error("expected isUnique=false")
	}
}

func TestParseColumnConstraints_NotNullNoDefault(t *testing.T) {
	nullable, defVal, _ := parseColumnConstraints("name VARCHAR(100) NOT NULL")
	if nullable {
		t.Error("expected nullable=false")
	}
	if defVal.Valid {
		t.Errorf("expected no default, got %q", defVal.String)
	}
}

func TestParseColumnConstraints_UniqueColumn(t *testing.T) {
	nullable, _, isUnique := parseColumnConstraints("slug VARCHAR(100) NOT NULL UNIQUE")
	if nullable {
		t.Error("expected nullable=false")
	}
	if !isUnique {
		t.Error("expected isUnique=true")
	}
}

func TestParseColumnConstraints_DefaultWithParens(t *testing.T) {
	_, defVal, _ := parseColumnConstraints("created_at TIMESTAMP DEFAULT NOW()")
	if !defVal.Valid || defVal.String != "NOW()" {
		t.Errorf("expected default 'NOW()', got valid=%v value=%q", defVal.Valid, defVal.String)
	}
}

func TestParseColumnConstraints_DefaultJsonb(t *testing.T) {
	_, defVal, _ := parseColumnConstraints("metadata JSONB DEFAULT '{}'")
	if !defVal.Valid || defVal.String != "'{}'" {
		t.Errorf("expected default \"'{}'\", got valid=%v value=%q", defVal.Valid, defVal.String)
	}
}

func TestParseCreateTableColumnDefs_ConstraintsPreserved(t *testing.T) {
	body := `
    id BIGSERIAL PRIMARY KEY,
    email VARCHAR(255) NOT NULL UNIQUE,
    is_active BOOLEAN NOT NULL DEFAULT false,
    bio TEXT,
    score INTEGER DEFAULT 0
`
	cols := parseCreateTableColumnDefs(body)
	byName := make(map[string]ColumnInfo)
	for _, c := range cols {
		byName[c.Name] = c
	}

	// email: NOT NULL UNIQUE
	if c, ok := byName["email"]; ok {
		if c.Nullable {
			t.Error("email: expected nullable=false")
		}
		if !c.IsUnique {
			t.Error("email: expected isUnique=true")
		}
	} else {
		t.Error("email column not parsed")
	}

	// is_active: NOT NULL DEFAULT false
	if c, ok := byName["is_active"]; ok {
		if c.Nullable {
			t.Error("is_active: expected nullable=false")
		}
		if !c.Default.Valid || c.Default.String != "false" {
			t.Errorf("is_active: expected default 'false', got %v/%q", c.Default.Valid, c.Default.String)
		}
	} else {
		t.Error("is_active column not parsed")
	}

	// bio: nullable, no default
	if c, ok := byName["bio"]; ok {
		if !c.Nullable {
			t.Error("bio: expected nullable=true")
		}
		if c.Default.Valid {
			t.Errorf("bio: expected no default, got %q", c.Default.String)
		}
	} else {
		t.Error("bio column not parsed")
	}

	// score: nullable with DEFAULT 0
	if c, ok := byName["score"]; ok {
		if !c.Default.Valid || c.Default.String != "0" {
			t.Errorf("score: expected default '0', got %v/%q", c.Default.Valid, c.Default.String)
		}
	} else {
		t.Error("score column not parsed")
	}
}

func TestExtractDefaultValueStr_SimpleValues(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"false", "false"},
		{"true", "true"},
		{"0", "0"},
		{"'{}' NOT NULL", "'{}'"},
		{"NOW() NOT NULL", "NOW()"},
		{"nextval('seq'::regclass)", "nextval('seq'::regclass)"},
	}
	for _, tt := range tests {
		got := extractDefaultValueStr(tt.input)
		if got != tt.want {
			t.Errorf("extractDefaultValueStr(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Tests for FK constraint parsing (extractFKsFromSQL, buildExpectedConstraintsFromFiles)
// ---------------------------------------------------------------------------

func TestExtractFKsFromSQL_CreateTable_TableLevel(t *testing.T) {
	sql := `
CREATE TABLE orders (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    CONSTRAINT fk_orders_user FOREIGN KEY (user_id) REFERENCES users(id)
);
`
	fks := extractFKsFromSQL(sql)
	if len(fks) != 1 {
		t.Fatalf("want 1 FK, got %d: %+v", len(fks), fks)
	}
	fk := fks[0]
	if fk.Name != "fk_orders_user" {
		t.Errorf("Name = %q, want fk_orders_user", fk.Name)
	}
	if fk.TableName != "orders" {
		t.Errorf("TableName = %q, want orders", fk.TableName)
	}
	if len(fk.Columns) != 1 || fk.Columns[0] != "user_id" {
		t.Errorf("Columns = %v, want [user_id]", fk.Columns)
	}
	if fk.RefTable != "users" {
		t.Errorf("RefTable = %q, want users", fk.RefTable)
	}
	if len(fk.RefColumns) != 1 || fk.RefColumns[0] != "id" {
		t.Errorf("RefColumns = %v, want [id]", fk.RefColumns)
	}
}

func TestExtractFKsFromSQL_CreateTable_InlineReferences(t *testing.T) {
	sql := `
CREATE TABLE order_items (
    id BIGSERIAL PRIMARY KEY,
    order_id BIGINT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES products(id)
);
`
	fks := extractFKsFromSQL(sql)
	if len(fks) != 2 {
		t.Fatalf("want 2 FKs, got %d: %+v", len(fks), fks)
	}
	byCol := make(map[string]ConstraintInfo)
	for _, fk := range fks {
		if len(fk.Columns) > 0 {
			byCol[fk.Columns[0]] = fk
		}
	}

	// order_id → orders with ON DELETE CASCADE
	if fk, ok := byCol["order_id"]; ok {
		if fk.RefTable != "orders" {
			t.Errorf("order_id RefTable = %q, want orders", fk.RefTable)
		}
		if fk.OnDelete != "CASCADE" {
			t.Errorf("order_id OnDelete = %q, want CASCADE", fk.OnDelete)
		}
	} else {
		t.Error("FK for order_id not found")
	}

	// product_id → products, no ON DELETE
	if fk, ok := byCol["product_id"]; ok {
		if fk.RefTable != "products" {
			t.Errorf("product_id RefTable = %q, want products", fk.RefTable)
		}
		if fk.OnDelete != "" {
			t.Errorf("product_id OnDelete = %q, want empty", fk.OnDelete)
		}
	} else {
		t.Error("FK for product_id not found")
	}
}

func TestExtractFKsFromSQL_AlterTableAddFK(t *testing.T) {
	sql := `
ALTER TABLE orders ADD CONSTRAINT fk_orders_user_id FOREIGN KEY (user_id) REFERENCES users(id);
`
	fks := extractFKsFromSQL(sql)
	if len(fks) != 1 {
		t.Fatalf("want 1 FK, got %d: %+v", len(fks), fks)
	}
	fk := fks[0]
	if fk.Name != "fk_orders_user_id" {
		t.Errorf("Name = %q, want fk_orders_user_id", fk.Name)
	}
	if fk.TableName != "orders" {
		t.Errorf("TableName = %q, want orders", fk.TableName)
	}
}

func TestExtractFKsFromSQL_AlterTableAddFK_Unnamed(t *testing.T) {
	sql := `ALTER TABLE orders ADD FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE SET NULL;`
	fks := extractFKsFromSQL(sql)
	if len(fks) != 1 {
		t.Fatalf("want 1 FK, got %d: %+v", len(fks), fks)
	}
	fk := fks[0]
	// Auto-generated name
	if fk.Name != "fk_orders_customer_id" {
		t.Errorf("auto-generated Name = %q, want fk_orders_customer_id", fk.Name)
	}
	if fk.OnDelete != "SET NULL" {
		t.Errorf("OnDelete = %q, want SET NULL", fk.OnDelete)
	}
}

func TestExtractFKsFromSQL_CompositeFK(t *testing.T) {
	sql := `
CREATE TABLE order_items (
    order_id BIGINT NOT NULL,
    line_no  INTEGER NOT NULL,
    FOREIGN KEY (order_id, line_no) REFERENCES orders(id, line_no)
);
`
	fks := extractFKsFromSQL(sql)
	if len(fks) != 1 {
		t.Fatalf("want 1 FK, got %d: %+v", len(fks), fks)
	}
	fk := fks[0]
	if len(fk.Columns) != 2 {
		t.Errorf("Columns = %v, want 2 elements", fk.Columns)
	}
	if len(fk.RefColumns) != 2 {
		t.Errorf("RefColumns = %v, want 2 elements", fk.RefColumns)
	}
}

func TestGenerateConstraintName(t *testing.T) {
	tests := []struct {
		table string
		cols  []string
		want  string
	}{
		{"orders", []string{"user_id"}, "fk_orders_user_id"},
		{"order_items", []string{"order_id", "product_id"}, "fk_order_items_order_id_product_id"},
		{"Users", []string{"Role_Id"}, "fk_users_role_id"},
	}
	for _, tt := range tests {
		got := generateConstraintName(tt.table, tt.cols)
		if got != tt.want {
			t.Errorf("generateConstraintName(%q, %v) = %q, want %q", tt.table, tt.cols, got, tt.want)
		}
	}
}

func TestBuildExpectedConstraintsFromFiles_Basic(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "001_create_orders.sql", `
-- Up
CREATE TABLE orders (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    CONSTRAINT fk_orders_user FOREIGN KEY (user_id) REFERENCES users(id)
);
-- Down
DROP TABLE orders;
`)

	result := buildExpectedConstraintsFromFiles(dir, []string{"001_create_orders.sql"})
	fks, ok := result["orders"]
	if !ok {
		t.Fatal("expected FKs for 'orders' table")
	}
	if len(fks) != 1 {
		t.Fatalf("want 1 FK, got %d: %+v", len(fks), fks)
	}
	if fks[0].Name != "fk_orders_user" {
		t.Errorf("Name = %q, want fk_orders_user", fks[0].Name)
	}
}

func TestBuildExpectedConstraintsFromFiles_DropConstraintRemovesFK(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "001_create_orders.sql", `
-- Up
CREATE TABLE orders (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    CONSTRAINT fk_orders_user FOREIGN KEY (user_id) REFERENCES users(id)
);
-- Down
DROP TABLE orders;
`)
	writeFile(t, dir, "002_remove_fk.sql", `
-- Up
ALTER TABLE orders DROP CONSTRAINT fk_orders_user;
-- Down
ALTER TABLE orders ADD CONSTRAINT fk_orders_user FOREIGN KEY (user_id) REFERENCES users(id);
`)

	result := buildExpectedConstraintsFromFiles(dir, []string{"001_create_orders.sql", "002_remove_fk.sql"})
	fks := result["orders"]
	for _, fk := range fks {
		if fk.Name == "fk_orders_user" {
			t.Errorf("FK fk_orders_user should have been removed by DROP CONSTRAINT, but it is still present")
		}
	}
}

func TestBuildExpectedConstraintsFromFiles_OnlyUpSection(t *testing.T) {
	dir := t.TempDir()

	// The Down section drops the FK — it must NOT cancel the Up section's ADD.
	writeFile(t, dir, "001_add_fk.sql", `
-- Up
ALTER TABLE orders ADD CONSTRAINT fk_orders_user FOREIGN KEY (user_id) REFERENCES users(id);
-- Down
ALTER TABLE orders DROP CONSTRAINT fk_orders_user;
`)

	result := buildExpectedConstraintsFromFiles(dir, []string{"001_add_fk.sql"})
	fks := result["orders"]
	found := false
	for _, fk := range fks {
		if fk.Name == "fk_orders_user" {
			found = true
		}
	}
	if !found {
		t.Error("FK should be present — Down section DROP must not cancel the Up section ADD")
	}
}

// writeFile is a test helper that writes content to a file inside dir.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("writeFile: %v", err)
	}
}
