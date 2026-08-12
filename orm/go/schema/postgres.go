package schema

import (
	"fmt"
	"os"
	"strings"
)

// CreateExtension writes a migration that enables a PostgreSQL extension.
// Down drops it (may fail if dependent objects remain — use CASCADE carefully).
func (f *Facade) CreateExtension(name string) error {
	if d := f.resolveDialect(); d != "postgres" {
		return fmt.Errorf("schema: extensions require postgres (got %s)", d)
	}
	up := fmt.Sprintf("CREATE EXTENSION IF NOT EXISTS %q;", name)
	down := fmt.Sprintf("DROP EXTENSION IF EXISTS %q CASCADE;", name)
	path, err := f.writeMigration("enable_"+sanitizeIdent(name)+"_extension", up, down)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "vorm: wrote %s\n", path)
	return f.maybeMigrate()
}

// CreateEnum writes CREATE TYPE … AS ENUM and drops it on rollback (CASCADE).
func (f *Facade) CreateEnum(typeName string, values ...string) error {
	if d := f.resolveDialect(); d != "postgres" {
		return fmt.Errorf("schema: enums require postgres (got %s)", d)
	}
	if len(values) == 0 {
		return fmt.Errorf("schema: enum %q needs at least one value", typeName)
	}
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = "'" + strings.ReplaceAll(v, "'", "''") + "'"
	}
	up := fmt.Sprintf("CREATE TYPE %s AS ENUM (%s);", typeName, strings.Join(quoted, ", "))
	down := fmt.Sprintf("DROP TYPE IF EXISTS %s CASCADE;", typeName)
	path, err := f.writeMigration("create_"+typeName+"_enum", up, down)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "vorm: wrote %s\n", path)
	return f.maybeMigrate()
}

// CreateFunction writes a raw PostgreSQL function body (up) with DROP on down.
func (f *Facade) CreateFunction(name, upSQL string) error {
	if d := f.resolveDialect(); d != "postgres" {
		return fmt.Errorf("schema: functions helper targets postgres (got %s)", d)
	}
	down := fmt.Sprintf("DROP FUNCTION IF EXISTS %s CASCADE;", name)
	path, err := f.writeMigration("create_"+sanitizeIdent(name)+"_function", strings.TrimSpace(upSQL), down)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "vorm: wrote %s\n", path)
	return f.maybeMigrate()
}

func sanitizeIdent(s string) string {
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, ".", "_")
	s = strings.ReplaceAll(s, "(", "_")
	s = strings.ReplaceAll(s, ")", "_")
	s = strings.ReplaceAll(s, " ", "_")
	return s
}
