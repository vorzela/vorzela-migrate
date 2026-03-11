package migration

import (
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
