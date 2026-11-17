package gui

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vorzela/vorzela-migrate/v2/internal/database"
	"github.com/vorzela/vorzela-migrate/v2/internal/migration"
)

// SchemaColumn is one live database column.
type SchemaColumn struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Nullable   bool   `json:"nullable"`
	Default    string `json:"default,omitempty"`
	Primary    bool   `json:"primary"`
	Unique     bool   `json:"unique"`
	ForeignKey string `json:"foreignKey,omitempty"`
}

// SchemaTable is one live database table plus the migration that created it.
type SchemaTable struct {
	Name          string         `json:"name"`
	Columns       []SchemaColumn `json:"columns"`
	ColumnCount   int            `json:"columnCount"`
	MigrationFile string         `json:"migrationFile,omitempty"`
	Executed      bool           `json:"executed"`
	FileMissing   bool           `json:"fileMissing,omitempty"`
}

// ProjectSchema is the response for the Tables page.
type ProjectSchema struct {
	Project     PublicProject `json:"project"`
	Dialect     string        `json:"dialect"`
	Tables      []SchemaTable `json:"tables"`
	Database    string        `json:"database"`
	CheckedAt   string        `json:"checkedAt"`
	StatusKnown bool          `json:"statusKnown"`
}

func primaryKeyColumns(db *sql.DB, dialect migration.Dialect, table string) map[string]bool {
	out := map[string]bool{}
	var query string
	var args []any
	switch dialect {
	case migration.PostgreSQL:
		query = `SELECT a.attname
			FROM pg_index i
			JOIN pg_class t ON t.oid = i.indrelid
			JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(i.indkey)
			WHERE t.relname = $1 AND i.indisprimary`
		args = []any{table}
	case migration.MySQL, migration.MariaDB:
		query = `SELECT COLUMN_NAME
			FROM information_schema.KEY_COLUMN_USAGE
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND CONSTRAINT_NAME = 'PRIMARY'`
		args = []any{table}
	default:
		return out
	}
	rows, err := db.Query(query, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			out[strings.ToLower(name)] = true
		}
	}
	return out
}

func dialectForEngine(engine string) (migration.Dialect, error) {
	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "postgres", "postgresql", "pg", "":
		return migration.PostgreSQL, nil
	case "mysql":
		return migration.MySQL, nil
	case "mariadb":
		return migration.MariaDB, nil
	default:
		return "", fmt.Errorf("unsupported engine %q", engine)
	}
}

func (s *Server) projectSchema(ctx context.Context, project Project, conn *Connection, password string) (ProjectSchema, error) {
	if conn == nil {
		return ProjectSchema{}, fmt.Errorf("select a database connection first")
	}
	dir, _, err := s.migrationDir(ctx, project, conn, password)
	if err != nil {
		return ProjectSchema{}, err
	}
	dsn, err := buildDSN(conn, password)
	if err != nil {
		return ProjectSchema{}, err
	}
	sqlDB, err := database.GetSQLDB(dsn)
	if err != nil {
		return ProjectSchema{}, err
	}
	defer sqlDB.Close()

	dialect, err := dialectForEngine(conn.Engine)
	if err != nil {
		return ProjectSchema{}, err
	}
	inspector := migration.NewSchemaInspector(sqlDB, dialect)
	tables, err := inspector.GetAllTables()
	if err != nil {
		return ProjectSchema{}, fmt.Errorf("list tables: %w", err)
	}
	statusReport, _ := s.migrationStatus(ctx, project, conn, password)
	executedMap := statusEntryMap(statusReport)
	fileMap, _ := migrationFileMap(dir)

	out := ProjectSchema{
		Project:     s.publicProject(project),
		Dialect:     string(dialect),
		Tables:      make([]SchemaTable, 0, len(tables)),
		Database:    redactedDSN(conn),
		CheckedAt:   time.Now().UTC().Format(time.RFC3339),
		StatusKnown: executedMap != nil,
	}
	for _, tableName := range tables {
		schema, err := inspector.GetTableSchema(tableName)
		if err != nil {
			return ProjectSchema{}, fmt.Errorf("inspect table %s: %w", tableName, err)
		}
		constraints, _ := inspector.GetTableConstraints(tableName)
		fkByColumn := map[string]string{}
		for _, constraint := range constraints {
			if len(constraint.Columns) != 1 {
				continue
			}
			fkByColumn[constraint.Columns[0]] = fmt.Sprintf("%s(%s)", constraint.RefTable, strings.Join(constraint.RefColumns, ", "))
		}
		indexes, _ := inspector.GetTableIndexes(tableName)
		uniqueByColumn := map[string]bool{}
		for _, index := range indexes {
			if index.IsUnique && len(index.Columns) == 1 {
				uniqueByColumn[index.Columns[0]] = true
			}
		}
		primaryColumns := primaryKeyColumns(sqlDB, dialect, tableName)

		table := SchemaTable{Name: tableName}
		for _, column := range schema.Columns {
			item := SchemaColumn{
				Name:       column.Name,
				Type:       column.Type,
				Nullable:   column.Nullable,
				Primary:    primaryColumns[strings.ToLower(column.Name)],
				Unique:     uniqueByColumn[column.Name],
				ForeignKey: fkByColumn[column.Name],
			}
			if column.Default.Valid {
				item.Default = column.Default.String
			}
			table.Columns = append(table.Columns, item)
		}
		table.ColumnCount = len(table.Columns)
		if file, ok := fileMap[strings.ToLower(tableName)]; ok {
			table.MigrationFile = file
			if executedMap != nil {
				if entry, ok := executedMap[file]; ok {
					table.Executed = entry.Executed
					table.FileMissing = entry.FileMissing
				}
			}
		}
		out.Tables = append(out.Tables, table)
	}
	sort.Slice(out.Tables, func(i, j int) bool { return out.Tables[i].Name < out.Tables[j].Name })
	return out, nil
}

// migrationFileMap maps table names to the migration file that defines them.
// A create_*_table file wins over later ALTER migrations.
func migrationFileMap(dir string) (map[string]string, error) {
	files, err := listMigrationFiles(dir)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, filename := range files {
		data, err := os.ReadFile(filepath.Join(dir, filename))
		if err != nil {
			continue
		}
		for table := range createTablesInContent(string(data)) {
			existing, ok := out[table]
			if !ok || (strings.Contains(filename, "create_") && !strings.Contains(existing, "create_")) {
				out[table] = filename
			}
		}
	}
	return out, nil
}

// columnDefinedInMigrations reports whether any migration already defines the
// column for the table, including pending ALTER-only files.
func columnDefinedInMigrations(dir, table, column string) (string, bool) {
	files, err := listMigrationFiles(dir)
	if err != nil {
		return "", false
	}
	for _, filename := range files {
		data, err := os.ReadFile(filepath.Join(dir, filename))
		if err != nil {
			continue
		}
		schema := parseSchemaSQL(string(data))
		t := schema.Tables[strings.ToLower(table)]
		if t == nil {
			continue
		}
		if _, ok := t.Columns[strings.ToLower(column)]; ok {
			return filename, true
		}
	}
	return "", false
}

// createTablesInContent returns tables that have an actual CREATE TABLE in the
// Up section. An ALTER-only migration must not count as the table definition.
func createTablesInContent(content string) map[string]bool {
	sql := stripSQLComments(extractUpSection(content))
	out := map[string]bool{}
	for _, match := range createTableEditRe.FindAllStringSubmatchIndex(sql, -1) {
		out[strings.ToLower(unquoteIdent(sql[match[2]:match[3]]))] = true
	}
	return out
}
