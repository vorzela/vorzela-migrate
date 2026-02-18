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
