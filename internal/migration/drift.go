package migration

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// ColumnInfo represents database column metadata
type ColumnInfo struct {
	Name     string
	Type     string
	Nullable bool
	Default  sql.NullString
}

// TableSchema represents the schema of a database table
type TableSchema struct {
	Name    string
	Columns []ColumnInfo
}

// SchemaDrift represents detected schema differences
type SchemaDrift struct {
	Table           string
	AddedColumns    []ColumnInfo
	ModifiedColumns []ColumnModification
}

// ColumnModification represents a changed column
type ColumnModification struct {
	Name    string
	OldType string
	NewType string
	Changed string // "type", "nullable", "default"
}

// SchemaInspector inspects database schema for drift detection
type SchemaInspector struct {
	db      *sql.DB
	dialect Dialect
}

// NewSchemaInspector creates a new schema inspector
func NewSchemaInspector(db *sql.DB, dialect Dialect) *SchemaInspector {
	return &SchemaInspector{
		db:      db,
		dialect: dialect,
	}
}

// GetTableSchema retrieves the current schema of a table
func (si *SchemaInspector) GetTableSchema(tableName string) (*TableSchema, error) {
	switch si.dialect {
	case PostgreSQL:
		return si.getPostgresTableSchema(tableName)
	case MySQL, MariaDB:
		return si.getMySQLTableSchema(tableName)
	default:
		return nil, fmt.Errorf("unsupported dialect: %s", si.dialect)
	}
}

// getPostgresTableSchema retrieves schema from PostgreSQL
func (si *SchemaInspector) getPostgresTableSchema(tableName string) (*TableSchema, error) {
	query := `
		SELECT 
			column_name,
			data_type,
			is_nullable,
			column_default
		FROM information_schema.columns
		WHERE table_name = $1
		AND table_schema = 'public'
		ORDER BY ordinal_position
	`

	rows, err := si.db.Query(query, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to query schema: %w", err)
	}
	defer rows.Close()

	schema := &TableSchema{Name: tableName}

	for rows.Next() {
		var col ColumnInfo
		var nullable string

		err := rows.Scan(&col.Name, &col.Type, &nullable, &col.Default)
		if err != nil {
			return nil, fmt.Errorf("failed to scan column: %w", err)
		}

		col.Nullable = (nullable == "YES")
		schema.Columns = append(schema.Columns, col)
	}

	return schema, nil
}

// getMySQLTableSchema retrieves schema from MySQL/MariaDB
func (si *SchemaInspector) getMySQLTableSchema(tableName string) (*TableSchema, error) {
	query := `
		SELECT 
			COLUMN_NAME,
			COLUMN_TYPE,
			IS_NULLABLE,
			COLUMN_DEFAULT
		FROM information_schema.COLUMNS
		WHERE TABLE_NAME = ?
		AND TABLE_SCHEMA = DATABASE()
		ORDER BY ORDINAL_POSITION
	`

	rows, err := si.db.Query(query, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to query schema: %w", err)
	}
	defer rows.Close()

	schema := &TableSchema{Name: tableName}

	for rows.Next() {
		var col ColumnInfo
		var nullable string

		err := rows.Scan(&col.Name, &col.Type, &nullable, &col.Default)
		if err != nil {
			return nil, fmt.Errorf("failed to scan column: %w", err)
		}

		col.Nullable = (nullable == "YES")
		schema.Columns = append(schema.Columns, col)
	}

	return schema, nil
}

// DetectDrift compares expected schema with current schema
func (si *SchemaInspector) DetectDrift(tableName string, expectedColumns []string) (*SchemaDrift, error) {
	currentSchema, err := si.GetTableSchema(tableName)
	if err != nil {
		return nil, err
	}

	drift := &SchemaDrift{
		Table:        tableName,
		AddedColumns: []ColumnInfo{},
	}

	// Create a map of expected columns
	expectedMap := make(map[string]bool)
	for _, col := range expectedColumns {
		expectedMap[col] = true
	}

	// Find columns that exist in DB but not in expected schema
	for _, col := range currentSchema.Columns {
		if !expectedMap[col.Name] {
			// Skip standard columns that are auto-added
			if col.Name == "id" || col.Name == "created_at" ||
				col.Name == "updated_at" || col.Name == "deleted_at" {
				continue
			}
			drift.AddedColumns = append(drift.AddedColumns, col)
		}
	}

	return drift, nil
}

// GenerateAlterStatements generates ALTER TABLE statements for drift
func (si *SchemaInspector) GenerateAlterStatements(drift *SchemaDrift) []string {
	if len(drift.AddedColumns) == 0 {
		return nil
	}

	var statements []string

	for _, col := range drift.AddedColumns {
		stmt := si.generateAddColumnStatement(drift.Table, col)
		statements = append(statements, stmt)
	}

	return statements
}

// generateAddColumnStatement creates ALTER TABLE ADD COLUMN statement
func (si *SchemaInspector) generateAddColumnStatement(table string, col ColumnInfo) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s %s",
		table, col.Name, col.Type))

	if !col.Nullable {
		parts = append(parts, "NOT NULL")
	}

	if col.Default.Valid {
		parts = append(parts, fmt.Sprintf("DEFAULT %s", col.Default.String))
	}

	return strings.Join(parts, " ") + ";"
}

// GetAllTables returns all user tables in the database
func (si *SchemaInspector) GetAllTables() ([]string, error) {
	var query string

	switch si.dialect {
	case PostgreSQL:
		query = `
			SELECT tablename 
			FROM pg_tables 
			WHERE schemaname = 'public' 
			AND tablename != 'migrations'
			AND tablename != 'migrations_lock'
			ORDER BY tablename
		`
	case MySQL, MariaDB:
		query = `
			SELECT TABLE_NAME 
			FROM information_schema.TABLES 
			WHERE TABLE_SCHEMA = DATABASE()
			AND TABLE_NAME != 'migrations'
			AND TABLE_NAME != 'migrations_lock'
			ORDER BY TABLE_NAME
		`
	default:
		return nil, fmt.Errorf("unsupported dialect")
	}

	rows, err := si.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}

	return tables, nil
}

// CompareSchemas compares two table schemas and returns differences
func CompareSchemas(expected, current *TableSchema) *SchemaDrift {
	drift := &SchemaDrift{
		Table:           current.Name,
		AddedColumns:    []ColumnInfo{},
		ModifiedColumns: []ColumnModification{},
	}

	// Create maps for easier comparison
	expectedMap := make(map[string]ColumnInfo)
	for _, col := range expected.Columns {
		expectedMap[col.Name] = col
	}

	currentMap := make(map[string]ColumnInfo)
	for _, col := range current.Columns {
		currentMap[col.Name] = col
	}

	// Find added columns
	for name, currentCol := range currentMap {
		if _, exists := expectedMap[name]; !exists {
			drift.AddedColumns = append(drift.AddedColumns, currentCol)
		}
	}

	// Sort for consistent output
	sort.Slice(drift.AddedColumns, func(i, j int) bool {
		return drift.AddedColumns[i].Name < drift.AddedColumns[j].Name
	})

	return drift
}
