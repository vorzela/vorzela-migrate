package migrate

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseExtensionsSplitsEnabledFromCommented(t *testing.T) {
	content := `-- header
CREATE EXTENSION IF NOT EXISTS citext;
-- CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION "uuid-ossp";
--   CREATE EXTENSION pgcrypto;
-- just a comment
`
	enabled, disabled := ParseExtensions(content)
	if want := []string{"citext", "uuid-ossp"}; !reflect.DeepEqual(enabled, want) {
		t.Errorf("enabled = %v, want %v", enabled, want)
	}
	if want := []string{"postgis", "pgcrypto"}; !reflect.DeepEqual(disabled, want) {
		t.Errorf("disabled = %v, want %v", disabled, want)
	}
}

func TestParseEnumsReadsValuesAcrossLines(t *testing.T) {
	content := `CREATE TYPE user_status AS ENUM (
    'active',
    'invited'
);
-- CREATE TYPE order_status AS ENUM ('pending', 'paid');
`
	enabled, disabled := ParseEnums(content)
	if len(enabled) != 1 {
		t.Fatalf("enabled = %+v, want one type", enabled)
	}
	if enabled[0].Name != "user_status" {
		t.Errorf("name = %q", enabled[0].Name)
	}
	if want := []string{"active", "invited"}; !reflect.DeepEqual(enabled[0].Values, want) {
		t.Errorf("values = %v, want %v", enabled[0].Values, want)
	}
	if want := []string{"order_status"}; !reflect.DeepEqual(disabled, want) {
		t.Errorf("disabled = %v, want %v", disabled, want)
	}
}

func TestEnumSyncSQLIsRerunnable(t *testing.T) {
	sql := EnumSyncSQL(EnumSpec{Name: "user_status", Values: []string{"active", "it's on"}})

	for _, want := range []string{
		`CREATE TYPE "user_status" AS ENUM ('active', 'it''s on')`,
		`ALTER TYPE "user_status" ADD VALUE IF NOT EXISTS 'active';`,
		`ALTER TYPE "user_status" ADD VALUE IF NOT EXISTS 'it''s on';`,
		`WHERE t.typname = 'user_status'`,
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("sync SQL is missing %q:\n%s", want, sql)
		}
	}
}

func TestDropEnumIfUnusedSQLGuardsOnColumns(t *testing.T) {
	sql := DropEnumIfUnusedSQL("user_status")
	if !strings.Contains(sql, "NOT EXISTS") || !strings.Contains(sql, "pg_attribute") {
		t.Errorf("drop SQL should be guarded by column usage:\n%s", sql)
	}
	if strings.Contains(sql, "CASCADE") {
		t.Errorf("dropping an enum must never cascade:\n%s", sql)
	}
}

func TestPrereqStatementsMakeFilesRerunnable(t *testing.T) {
	enums := PrereqStatements(EnumsFile, "CREATE TYPE status AS ENUM ('on', 'off');\n")
	if len(enums) != 1 {
		t.Fatalf("enum statements = %v, want one", enums)
	}
	if strings.HasPrefix(strings.TrimSpace(enums[0]), "CREATE TYPE") {
		t.Errorf("a bare CREATE TYPE fails on the second run:\n%s", enums[0])
	}

	exts := PrereqStatements(ExtensionsFile, "CREATE EXTENSION citext;\n")
	if len(exts) != 1 || !strings.Contains(exts[0], "IF NOT EXISTS") {
		t.Errorf("extensions = %v, want IF NOT EXISTS added", exts)
	}

	fns := PrereqStatements(FunctionsFile, "CREATE OR REPLACE FUNCTION touch() RETURNS trigger AS $$\nBEGIN\n  RETURN NEW;\nEND;\n$$ LANGUAGE plpgsql;\n")
	if len(fns) != 1 {
		t.Errorf("a dollar-quoted body must stay in one statement: %v", fns)
	}
}

func TestEnsurePrereqFileNeverOverwrites(t *testing.T) {
	dir := t.TempDir()

	path, created, err := EnsurePrereqFile(dir, EnumsFile)
	if err != nil || !created {
		t.Fatalf("EnsurePrereqFile = %v, %v", created, err)
	}
	if err := os.WriteFile(path, []byte("CREATE TYPE mine AS ENUM ('a');\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, created, err = EnsurePrereqFile(dir, EnumsFile); err != nil || created {
		t.Fatalf("second call = %v, %v, want no create", created, err)
	}
	content, err := os.ReadFile(filepath.Join(dir, EnumsFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "CREATE TYPE mine") {
		t.Error("the template overwrote hand-written content")
	}
}

func TestPrereqTemplatesShipDisabled(t *testing.T) {
	// A fresh extensions.sql or enums.sql must not create anything by itself:
	// the first sync should be a no-op until the user opts in.
	for _, name := range []string{ExtensionsFile, EnumsFile} {
		dir := t.TempDir()
		if _, _, err := EnsurePrereqFile(dir, name); err != nil {
			t.Fatal(err)
		}
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if stmts := PrereqStatements(name, string(content)); len(stmts) != 0 {
			t.Errorf("%s template would run %d statement(s): %v", name, len(stmts), stmts)
		}
	}
}
