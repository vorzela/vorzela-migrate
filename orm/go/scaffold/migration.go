package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// MigrationDirs holds output locations for make migration.
type MigrationDirs struct {
	SchemaDir string // default ./schema/migrations
	ModelDir  string // default ./models
	QueryDir  string // default ./queries
}

func (d *MigrationDirs) defaults() {
	if d.SchemaDir == "" {
		d.SchemaDir = "./schema/migrations"
	}
	if d.ModelDir == "" {
		d.ModelDir = "./models"
	}
	if d.QueryDir == "" {
		d.QueryDir = "./queries"
	}
}

// MakeResult lists files created by MakeMigration.
type MakeResult struct {
	Table         string
	MigrationFile string
	ModelFile     string
	QueryFile     string
	FuncName      string
}

// MakeMigration scaffolds Blueprint migration + model + query stub.
//
//	vorm make migration posts
//	vorm make migration post_user
func MakeMigration(rawName string, dirs MigrationDirs) (*MakeResult, error) {
	dirs.defaults()
	table, soft, pivot := normalizeTableName(rawName)
	funcName := "Create" + exportName(table) + "Table"
	modelName := singularExport(table)
	entityName := pluralExport(table)

	for _, dir := range []string{dirs.SchemaDir, dirs.ModelDir, dirs.QueryDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	migPath := filepath.Join(dirs.SchemaDir, "create_"+table+".go")
	modelPath := filepath.Join(dirs.ModelDir, singularFile(table)+".go")
	queryPath := filepath.Join(dirs.QueryDir, table+".go")

	if err := os.WriteFile(migPath, []byte(migrationSource(table, funcName, soft, pivot)), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(modelPath, []byte(modelSource(table, modelName, entityName, soft, pivot)), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(queryPath, []byte(queryStubSource(table, modelName, entityName)), 0o644); err != nil {
		return nil, err
	}

	return &MakeResult{
		Table:         table,
		MigrationFile: migPath,
		ModelFile:     modelPath,
		QueryFile:     queryPath,
		FuncName:      funcName,
	}, nil
}

func normalizeTableName(raw string) (table string, softDeletes bool, pivot *pivotHint) {
	name := strings.TrimSpace(raw)
	name = strings.TrimSuffix(name, ".go")
	name = strings.TrimPrefix(name, "create_")
	name = strings.TrimSuffix(name, "_table")
	softDeletes = true
	parts := strings.Split(name, "_")
	if len(parts) == 2 && !strings.HasSuffix(parts[0], "s") {
		// post_user / post_tag → pivot
		left, right := parts[0], parts[1]
		pivot = &pivotHint{
			LeftCol:    left + "_id",
			LeftTable:  pluralize(left),
			RightCol:   right + "_id",
			RightTable: pluralize(right),
		}
		softDeletes = false // pivots usually no soft deletes
	}
	return name, softDeletes, pivot
}

type pivotHint struct {
	LeftCol, LeftTable, RightCol, RightTable string
}

func migrationSource(table, funcName string, soft bool, pivot *pivotHint) string {
	var body strings.Builder
	body.WriteString("\t\tt.ID()\n")
	if pivot != nil {
		fmt.Fprintf(&body, "\t\tt.ForeignId(%q).Constrained(%q).CascadeOnDelete()\n", pivot.LeftCol, pivot.LeftTable)
		fmt.Fprintf(&body, "\t\tt.ForeignId(%q).Constrained(%q).CascadeOnDelete()\n", pivot.RightCol, pivot.RightTable)
		fmt.Fprintf(&body, "\t\tt.Unique(%q, %q)\n", pivot.LeftCol, pivot.RightCol)
		fmt.Fprintf(&body, "\t\tt.Index(%q)\n", pivot.LeftCol)
		fmt.Fprintf(&body, "\t\tt.Index(%q)\n", pivot.RightCol)
	} else {
		body.WriteString("\t\t// t.String(\"title\")\n")
		body.WriteString("\t\t// t.Text(\"body\")\n")
		body.WriteString("\t\t// t.Enum(\"status\", \"draft\", \"published\", \"archived\")\n")
		body.WriteString("\t\t// t.ForeignId(\"user_id\").Constrained(\"users\").CascadeOnDelete()\n")
		body.WriteString("\t\t// t.Boolean(\"active\").Default(true)\n")
		body.WriteString("\t\t// t.Integer(\"age\").Nullable()\n")
	}
	body.WriteString("\t\tt.Timestamps()\n")
	if soft {
		body.WriteString("\t\tt.SoftDeletes()\n")
	}

	return fmt.Sprintf(`package migrations

import "github.com/vorzela/vorm/schema"

// %s — Laravel-style Schema::create in Go.
// Edit the Blueprint, then call %s(nil) (or pass a Facade inside Schema.Batch).
// That writes migrations/*.sql and applies it in-process when AutoMigrate is true.
func %s(s *schema.Facade) error {
	if s == nil {
		s = schema.Default
	}
	return s.Create(%q, func(t *schema.Blueprint) {
%s	})
}
`, funcName, funcName, funcName, table, body.String())
}

func modelSource(table, modelName, entityName string, soft bool, pivot *pivotHint) string {
	var fields strings.Builder
	var cols []string
	cols = append(cols, "id")
	if pivot != nil {
		fmt.Fprintf(&fields, "\t%s int64 `json:%q db:%q`\n", exportName(pivot.LeftCol), pivot.LeftCol, pivot.LeftCol)
		fmt.Fprintf(&fields, "\t%s int64 `json:%q db:%q`\n", exportName(pivot.RightCol), pivot.RightCol, pivot.RightCol)
		cols = append(cols, pivot.LeftCol, pivot.RightCol)
	}
	cols = append(cols, "created_at", "updated_at")
	if soft {
		cols = append(cols, "deleted_at")
	}
	colLit := `"` + strings.Join(cols, `", "`) + `"`

	softLit := "false"
	if soft {
		softLit = "true"
	}

	return fmt.Sprintf(`// Code generated by vorm; DO NOT EDIT.
// Regenerate after Blueprint changes: vorm generate models
package models

import (
	"github.com/vorzela/vorm"
	"github.com/vorzela/vorm/query"
)

// %s is generated from schema/migrations create_%s.
type %s struct {
	vorm.Model
%s}

// %s is the typed query entrypoint (explicit columns — no SELECT *).
var %s = query.Model[%s](query.Meta{
	Table:       %q,
	Columns:     []string{%s},
	SoftDeletes: %s,
})
`, modelName, table, modelName, fields.String(), entityName, entityName, modelName, table, colLit, softLit)
}

func queryStubSource(table, modelName, entityName string) string {
	return fmt.Sprintf(`package queries

import (
	"context"

	"your/module/models" // TODO: fix import path

	"github.com/vorzela/vorm/query"
)

// Write // vorm:query stubs here, then: vorm generate
//
// vorm:query name=List%s
func List%s(ctx context.Context, db query.DB) ([]models.%s, error) {
	ctx = query.WithMapper(ctx, scan%s)
	return models.%s.OrderBy("id").Limit(50).Get(ctx, db)
}

func scan%s(rows query.Rows) (models.%s, error) {
	var row models.%s
	// Align Scan order with models.%s Meta.Columns after vorm generate models
	err := rows.Scan(&row.ID, &row.CreatedAt, &row.UpdatedAt, &row.DeletedAt)
	return row, err
}
`, entityName, entityName, modelName, modelName, entityName, modelName, modelName, modelName, entityName)
}

func singularFile(table string) string {
	s := singular(table)
	return s
}

func singularExport(table string) string {
	return exportName(singular(table))
}

func pluralExport(table string) string {
	return exportName(table)
}

func singular(table string) string {
	if strings.HasSuffix(table, "ies") && len(table) > 3 {
		return table[:len(table)-3] + "y"
	}
	if strings.HasSuffix(table, "ses") || strings.HasSuffix(table, "xes") || strings.HasSuffix(table, "zes") {
		return table[:len(table)-2]
	}
	if strings.HasSuffix(table, "s") && len(table) > 1 {
		return table[:len(table)-1]
	}
	return table
}

func pluralize(word string) string {
	if strings.HasSuffix(word, "y") && len(word) > 1 {
		return word[:len(word)-1] + "ies"
	}
	if strings.HasSuffix(word, "s") {
		return word
	}
	return word + "s"
}

// GoMigration is kept for callers that only want the schema file.
func GoMigration(dir, name string) (string, error) {
	res, err := MakeMigration(name, MigrationDirs{SchemaDir: dir, ModelDir: filepath.Join(dir, "..", "..", "models"), QueryDir: filepath.Join(dir, "..", "..", "queries")})
	if err != nil {
		return "", err
	}
	return res.MigrationFile, nil
}

func exportName(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '-' || unicode.IsSpace(r)
	})
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]))
		if len(p) > 1 {
			b.WriteString(p[1:])
		}
	}
	s := b.String()
	if s == "" {
		return "Up"
	}
	return s
}

func tableFromMigrationName(name string) string {
	n := name
	n = strings.TrimPrefix(n, "create_")
	n = strings.TrimSuffix(n, "_table")
	if n == "" {
		return name
	}
	return n
}
