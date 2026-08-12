package migrate

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// The prerequisite files are declarative: a commented-out CREATE line means the
// object is disabled, an uncommented one means it is wanted. That is the same
// contract vm uses, so the files are interchangeable between the two tools.
const (
	ExtensionsFile = "extensions.sql"
	FunctionsFile  = "functions.sql"
	EnumsFile      = "enums.sql"
)

var (
	createExtensionRe = regexp.MustCompile(`(?i)^CREATE\s+EXTENSION\s+(?:IF\s+NOT\s+EXISTS\s+)?["']?([\w-]+)["']?`)
	createTypeRe      = regexp.MustCompile(`(?i)^CREATE\s+TYPE\s+["']?([\w.]+)["']?\s+AS\s+ENUM`)
	createFunctionRe  = regexp.MustCompile(`(?i)^CREATE\s+(?:OR\s+REPLACE\s+)?FUNCTION\s+["']?([\w.]+)["']?\s*\(`)
	enumValueRe       = regexp.MustCompile(`'((?:[^']|'')*)'`)
)

// EnumSpec is one enum type declared in enums.sql.
type EnumSpec struct {
	Name   string
	Values []string
}

// ParseExtensions splits extensions.sql into the extensions that are enabled
// and the ones that are commented out.
func ParseExtensions(content string) (enabled, disabled []string) {
	return parseToggles(content, createExtensionRe)
}

// ParseFunctions splits functions.sql into enabled and commented-out function
// names. Only the CREATE line matters; the body is applied verbatim.
func ParseFunctions(content string) (enabled, disabled []string) {
	return parseToggles(content, createFunctionRe)
}

func parseToggles(content string, re *regexp.Regexp) (enabled, disabled []string) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "--"); ok {
			if m := re.FindStringSubmatch(strings.TrimSpace(strings.TrimLeft(rest, "-"))); m != nil {
				disabled = append(disabled, unquote(m[1]))
			}
			continue
		}
		if m := re.FindStringSubmatch(line); m != nil {
			enabled = append(enabled, unquote(m[1]))
		}
	}
	return enabled, disabled
}

// ParseEnums reads enums.sql. Enabled entries carry their values so they can be
// synced idempotently; disabled entries are names only.
func ParseEnums(content string) (enabled []EnumSpec, disabled []string) {
	for _, stmt := range SplitStatements(content) {
		if m := createTypeRe.FindStringSubmatch(strings.TrimSpace(stmt)); m != nil {
			enabled = append(enabled, EnumSpec{Name: unquote(m[1]), Values: enumValues(stmt)})
		}
	}
	// Commented-out types never survive SplitStatements, so scan the raw lines.
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		rest, ok := strings.CutPrefix(line, "--")
		if !ok {
			continue
		}
		if m := createTypeRe.FindStringSubmatch(strings.TrimSpace(strings.TrimLeft(rest, "-"))); m != nil {
			disabled = append(disabled, unquote(m[1]))
		}
	}
	return enabled, disabled
}

func enumValues(stmt string) []string {
	open := strings.Index(stmt, "(")
	if open < 0 {
		return nil
	}
	var out []string
	for _, m := range enumValueRe.FindAllStringSubmatch(stmt[open:], -1) {
		out = append(out, strings.ReplaceAll(m[1], "''", "'"))
	}
	return out
}

func unquote(s string) string { return strings.Trim(strings.TrimSpace(s), `"'`) }

// EnumSyncSQL creates the type when it is missing and otherwise appends any
// values it does not have yet. Enum values cannot be removed by PostgreSQL, so
// this is the whole of what a sync can do.
func EnumSyncSQL(e EnumSpec) string {
	if e.Name == "" || len(e.Values) == 0 {
		return ""
	}
	quoted := make([]string, 0, len(e.Values))
	var adds strings.Builder
	for _, v := range e.Values {
		lit := quoteLiteral(v)
		quoted = append(quoted, lit)
		fmt.Fprintf(&adds, "    ALTER TYPE %s ADD VALUE IF NOT EXISTS %s;\n", quoteIdent(e.Name), lit)
	}
	return fmt.Sprintf(`DO $vorm$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_type t
    JOIN pg_namespace n ON n.oid = t.typnamespace
    WHERE t.typname = %s AND n.nspname = current_schema() AND t.typtype = 'e'
  ) THEN
    CREATE TYPE %s AS ENUM (%s);
  ELSE
%s  END IF;
END $vorm$;`, quoteLiteral(e.Name), quoteIdent(e.Name), strings.Join(quoted, ", "), adds.String())
}

// DropEnumIfUnusedSQL drops the type only when no column still uses it, so a
// disabled enum never takes a table down with it.
func DropEnumIfUnusedSQL(name string) string {
	return fmt.Sprintf(`DO $vorm$
BEGIN
  IF EXISTS (
    SELECT 1 FROM pg_type t
    JOIN pg_namespace n ON n.oid = t.typnamespace
    WHERE t.typname = %s AND n.nspname = current_schema() AND t.typtype = 'e'
  ) AND NOT EXISTS (
    SELECT 1 FROM pg_attribute a
    JOIN pg_type t ON t.oid = a.atttypid
    WHERE t.typname = %s AND a.attisdropped = false
  ) THEN
    DROP TYPE %s;
  END IF;
END $vorm$;`, quoteLiteral(name), quoteLiteral(name), quoteIdent(name))
}

func quoteLiteral(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func quoteIdent(s string) string {
	if strings.Contains(s, ".") {
		parts := strings.SplitN(s, ".", 2)
		return quoteIdent(parts[0]) + "." + quoteIdent(parts[1])
	}
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// normalizeExtension makes a CREATE EXTENSION statement re-runnable.
func normalizeExtension(stmt string) string {
	trimmed := strings.TrimSpace(stmt)
	if !createExtensionRe.MatchString(trimmed) {
		return stmt
	}
	if regexp.MustCompile(`(?i)^CREATE\s+EXTENSION\s+IF\s+NOT\s+EXISTS`).MatchString(trimmed) {
		return stmt
	}
	return regexp.MustCompile(`(?i)^CREATE\s+EXTENSION\s+`).ReplaceAllString(trimmed, "CREATE EXTENSION IF NOT EXISTS ")
}

// EnsurePrereqFile writes the template for name when the file does not exist.
// It never touches an existing file. The returned bool reports a create.
func EnsurePrereqFile(dir, name string) (string, bool, error) {
	template, ok := prereqTemplates[name]
	if !ok {
		return "", false, fmt.Errorf("vorm/migrate: no template for %q", name)
	}
	if dir == "" {
		dir = DefaultDir
	}
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err == nil {
		return path, false, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return path, false, err
	}
	if err := os.WriteFile(path, []byte(template), 0o644); err != nil {
		return path, false, err
	}
	return path, true, nil
}

var prereqTemplates = map[string]string{
	ExtensionsFile: extensionsTemplate,
	EnumsFile:      enumsTemplate,
	FunctionsFile:  functionsTemplate,
}

const extensionsTemplate = `-- PostgreSQL extensions (declarative)
--
-- Uncomment what the project needs, then: vorm extensions
-- Commenting a line out again drops the extension on the next
-- ` + "`vorm extensions --drop-disabled`" + ` run.

-- CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
-- CREATE EXTENSION IF NOT EXISTS pgcrypto;
-- CREATE EXTENSION IF NOT EXISTS citext;
-- CREATE EXTENSION IF NOT EXISTS pg_trgm;
-- CREATE EXTENSION IF NOT EXISTS unaccent;
-- CREATE EXTENSION IF NOT EXISTS btree_gin;
-- CREATE EXTENSION IF NOT EXISTS postgis;

-- Project-specific extensions below.
`

const enumsTemplate = `-- PostgreSQL enum types (declarative)
--
-- Uncomment or add types, then: vorm enums
-- Adding a value to an existing type is applied with ALTER TYPE ... ADD VALUE;
-- PostgreSQL cannot remove enum values, so removing one here is not synced.
-- vorm generate models turns each type into a Go string type with constants.

-- CREATE TYPE user_status AS ENUM ('active', 'invited', 'suspended', 'banned');
-- CREATE TYPE order_status AS ENUM ('pending', 'paid', 'shipped', 'cancelled');

-- Project-specific enums below.
`

const functionsTemplate = `-- Database functions and trigger helpers (declarative)
--
-- Applied with CREATE OR REPLACE on every change: vorm functions
-- Everything in this file is applied verbatim, so it is the right place for
-- trigger helpers that migrations attach to tables.

CREATE OR REPLACE FUNCTION auto_update_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION protect_soft_deleted()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.deleted_at IS NOT NULL AND NEW.deleted_at IS NOT NULL THEN
        RAISE EXCEPTION 'row % is soft deleted and cannot be updated', OLD.id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Custom functions below.
`
