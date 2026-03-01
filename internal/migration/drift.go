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

// IndexInfo represents a database index
type IndexInfo struct {
	Name      string
	TableName string
	Columns   []string
	IsUnique  bool
	IndexType string // btree, hash, gin, gist, etc.
}

// TriggerInfo represents a database trigger
type TriggerInfo struct {
	Name      string
	TableName string
	Event     string // INSERT, UPDATE, DELETE
	Timing    string // BEFORE, AFTER, INSTEAD OF
	Statement string // trigger body / procedure reference
}

// TableSchema represents the schema of a database table
type TableSchema struct {
	Name    string
	Columns []ColumnInfo
}

// SchemaDrift represents detected schema differences
type SchemaDrift struct {
	Table           string
	AddedColumns    []ColumnInfo // columns in DB not defined in any migration
	MissingColumns  []ColumnInfo // columns defined in migrations but absent from DB
	ModifiedColumns []ColumnModification
	MissingIndexes  []IndexInfo
	MissingTriggers []TriggerInfo
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

// GetTableIndexes retrieves all indexes for a table from the database.
func (si *SchemaInspector) GetTableIndexes(tableName string) ([]IndexInfo, error) {
	switch si.dialect {
	case PostgreSQL:
		return si.getPostgresTableIndexes(tableName)
	case MySQL, MariaDB:
		return si.getMySQLTableIndexes(tableName)
	default:
		return nil, fmt.Errorf("unsupported dialect for index inspection: %s", si.dialect)
	}
}

func (si *SchemaInspector) getPostgresTableIndexes(tableName string) ([]IndexInfo, error) {
	query := `
		SELECT
			i.relname AS index_name,
			ix.indisunique AS is_unique,
			am.amname AS index_type,
			array_agg(a.attname ORDER BY a.attnum) AS columns
		FROM
			pg_class t
			JOIN pg_index ix ON t.oid = ix.indrelid
			JOIN pg_class i ON i.oid = ix.indexrelid
			JOIN pg_am am ON am.oid = i.relam
			JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(ix.indkey)
		WHERE
			t.relname = $1
			AND t.relkind = 'r'
			AND ix.indisprimary = false
		GROUP BY i.relname, ix.indisunique, am.amname
		ORDER BY i.relname
	`
	rows, err := si.db.Query(query, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to query indexes: %w", err)
	}
	defer rows.Close()

	var indexes []IndexInfo
	for rows.Next() {
		var idx IndexInfo
		var colArray string
		idx.TableName = tableName
		if err := rows.Scan(&idx.Name, &idx.IsUnique, &idx.IndexType, &colArray); err != nil {
			return nil, err
		}
		// colArray arrives as "{col1,col2,...}" from PostgreSQL
		colArray = strings.Trim(colArray, "{}")
		for _, c := range strings.Split(colArray, ",") {
			if c = strings.TrimSpace(c); c != "" {
				idx.Columns = append(idx.Columns, c)
			}
		}
		indexes = append(indexes, idx)
	}
	return indexes, rows.Err()
}

func (si *SchemaInspector) getMySQLTableIndexes(tableName string) ([]IndexInfo, error) {
	query := `
		SELECT
			INDEX_NAME,
			NON_UNIQUE,
			INDEX_TYPE,
			GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX) AS columns
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = ?
		  AND INDEX_NAME != 'PRIMARY'
		GROUP BY INDEX_NAME, NON_UNIQUE, INDEX_TYPE
		ORDER BY INDEX_NAME
	`
	rows, err := si.db.Query(query, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to query indexes: %w", err)
	}
	defer rows.Close()

	var indexes []IndexInfo
	for rows.Next() {
		var idx IndexInfo
		var nonUnique int
		var colList string
		idx.TableName = tableName
		if err := rows.Scan(&idx.Name, &nonUnique, &idx.IndexType, &colList); err != nil {
			return nil, err
		}
		idx.IsUnique = nonUnique == 0
		for _, c := range strings.Split(colList, ",") {
			if c = strings.TrimSpace(c); c != "" {
				idx.Columns = append(idx.Columns, c)
			}
		}
		indexes = append(indexes, idx)
	}
	return indexes, rows.Err()
}

// GetTableTriggers retrieves all triggers for a table from the database.
func (si *SchemaInspector) GetTableTriggers(tableName string) ([]TriggerInfo, error) {
	switch si.dialect {
	case PostgreSQL:
		return si.getPostgresTableTriggers(tableName)
	case MySQL, MariaDB:
		return si.getMySQLTableTriggers(tableName)
	default:
		return nil, fmt.Errorf("unsupported dialect for trigger inspection: %s", si.dialect)
	}
}

func (si *SchemaInspector) getPostgresTableTriggers(tableName string) ([]TriggerInfo, error) {
	query := `
		SELECT
			trigger_name,
			event_manipulation,
			action_timing,
			action_statement
		FROM information_schema.triggers
		WHERE event_object_table = $1
		  AND trigger_schema = 'public'
		ORDER BY trigger_name
	`
	rows, err := si.db.Query(query, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to query triggers: %w", err)
	}
	defer rows.Close()

	var triggers []TriggerInfo
	for rows.Next() {
		var trig TriggerInfo
		trig.TableName = tableName
		if err := rows.Scan(&trig.Name, &trig.Event, &trig.Timing, &trig.Statement); err != nil {
			return nil, err
		}
		triggers = append(triggers, trig)
	}
	return triggers, rows.Err()
}

func (si *SchemaInspector) getMySQLTableTriggers(tableName string) ([]TriggerInfo, error) {
	query := `
		SELECT
			TRIGGER_NAME,
			EVENT_MANIPULATION,
			ACTION_TIMING,
			ACTION_STATEMENT
		FROM information_schema.TRIGGERS
		WHERE EVENT_OBJECT_SCHEMA = DATABASE()
		  AND EVENT_OBJECT_TABLE = ?
		ORDER BY TRIGGER_NAME
	`
	rows, err := si.db.Query(query, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to query triggers: %w", err)
	}
	defer rows.Close()

	var triggers []TriggerInfo
	for rows.Next() {
		var trig TriggerInfo
		trig.TableName = tableName
		if err := rows.Scan(&trig.Name, &trig.Event, &trig.Timing, &trig.Statement); err != nil {
			return nil, err
		}
		triggers = append(triggers, trig)
	}
	return triggers, rows.Err()
}

// DetectDrift compares expected schema with current schema.
// It checks BOTH drift directions:
//   - AddedColumns:   columns that exist in the DB but are not in any migration file
//   - MissingColumns: columns that migrations define but are absent from the DB
//
// expectedColumns must carry full ColumnInfo (including Type) so that
// ALTER TABLE ADD COLUMN statements can be generated for MissingColumns.
func (si *SchemaInspector) DetectDrift(tableName string, expectedColumns []ColumnInfo) (*SchemaDrift, error) {
	currentSchema, err := si.GetTableSchema(tableName)
	if err != nil {
		return nil, err
	}

	drift := &SchemaDrift{
		Table:        tableName,
		AddedColumns: []ColumnInfo{},
	}

	// Build a set of columns currently in the DB (keyed by lower-case name)
	dbColMap := make(map[string]bool, len(currentSchema.Columns))
	for _, col := range currentSchema.Columns {
		dbColMap[strings.ToLower(col.Name)] = true
	}

	// Build a set of columns expected from migration files (keyed by lower-case name)
	expectedMap := make(map[string]ColumnInfo, len(expectedColumns))
	for _, col := range expectedColumns {
		expectedMap[strings.ToLower(col.Name)] = col
	}

	// Standard columns that are always auto-added and should be ignored in both directions
	autoColumns := map[string]bool{
		"id": true, "created_at": true, "updated_at": true, "deleted_at": true,
	}

	// Direction 1: columns in DB not defined in any migration → "added outside migration"
	for _, col := range currentSchema.Columns {
		name := strings.ToLower(col.Name)
		if autoColumns[name] {
			continue
		}
		if _, ok := expectedMap[name]; !ok {
			drift.AddedColumns = append(drift.AddedColumns, col)
		}
	}

	// Direction 2: columns defined in migrations but not present in DB → "missing column"
	for _, col := range expectedColumns {
		name := strings.ToLower(col.Name)
		if autoColumns[name] {
			continue
		}
		if !dbColMap[name] {
			drift.MissingColumns = append(drift.MissingColumns, col)
		}
	}

	return drift, nil
}

// GenerateAlterStatements generates ALTER TABLE statements for drift.
// It handles both AddedColumns (columns extra in DB) and MissingColumns
// (columns defined in migrations but absent from DB — these need ADD COLUMN).
func (si *SchemaInspector) GenerateAlterStatements(drift *SchemaDrift) []string {
	var statements []string

	// AddedColumns: columns that appeared in the DB outside of migrations.
	// These were the original drift-fix targets (ALTER TABLE … ADD COLUMN based on DB introspection).
	for _, col := range drift.AddedColumns {
		stmt := si.generateAddColumnStatement(drift.Table, col)
		statements = append(statements, stmt)
	}

	// MissingColumns: columns defined in migration files but absent from the DB.
	// Generate ALTER TABLE … ADD COLUMN using the type from the migration definition.
	for _, col := range drift.MissingColumns {
		if col.Type == "" {
			// Type unknown — emit a comment as a reminder instead of a broken statement.
			statements = append(statements,
				fmt.Sprintf("-- MISSING COLUMN: %s.%s — type unknown, please add manually.",
					drift.Table, col.Name))
			continue
		}
		statements = append(statements,
			fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s %s;",
				drift.Table, col.Name, col.Type))
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

// GenerateCreateIndexStatements generates CREATE INDEX statements for missing indexes.
func (si *SchemaInspector) GenerateCreateIndexStatements(drift *SchemaDrift) []string {
	if len(drift.MissingIndexes) == 0 {
		return nil
	}

	var statements []string
	for _, idx := range drift.MissingIndexes {
		stmt := generateCreateIndexSQL(idx, si.dialect)
		statements = append(statements, stmt)
	}
	return statements
}

// generateCreateIndexSQL builds a CREATE INDEX statement for the given IndexInfo.
// It is shared between the live executor and the migration file generator.
func generateCreateIndexSQL(idx IndexInfo, dialect Dialect) string {
	unique := ""
	if idx.IsUnique {
		unique = "UNIQUE "
	}
	cols := strings.Join(idx.Columns, ", ")

	// USING <type> goes BEFORE the column list in PostgreSQL:
	//   CREATE INDEX name ON table USING gist (col);
	// Omit it when it is empty or is the default "btree".
	usingClause := ""
	if t := strings.ToLower(strings.TrimSpace(idx.IndexType)); t != "" && t != "btree" {
		usingClause = fmt.Sprintf("USING %s ", t)
	}

	if dialect == PostgreSQL {
		return fmt.Sprintf("CREATE %sINDEX IF NOT EXISTS %s ON %s %s(%s);",
			unique, idx.Name, idx.TableName, usingClause, cols)
	}
	// MySQL / MariaDB — no USING clause for standard indexes
	if idx.IsUnique {
		return fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s (%s);",
			idx.Name, idx.TableName, cols)
	}
	return fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s (%s);",
		idx.Name, idx.TableName, cols)
}

// GenerateCreateTriggerStatements returns a comment for each missing trigger
// reminding the user to recreate it, since trigger bodies require the original SQL.
func (si *SchemaInspector) GenerateCreateTriggerStatements(drift *SchemaDrift) []string {
	if len(drift.MissingTriggers) == 0 {
		return nil
	}

	var statements []string
	for _, trig := range drift.MissingTriggers {
		// Emit a reminder comment — trigger bodies can be complex and dialect-specific;
		// the DBA should re-apply the original migration or write a targeted fix migration.
		stmt := fmt.Sprintf(
			"-- MISSING TRIGGER: %s (%s %s ON %s) — re-apply or create a new migration to restore it.",
			trig.Name, trig.Timing, trig.Event, trig.TableName,
		)
		statements = append(statements, stmt)
	}
	return statements
}

// GenerateAllStatements returns all SQL statements needed to fix drift:
// ALTER TABLE for missing columns, CREATE INDEX for missing indexes,
// and advisory comments for missing triggers.
func (si *SchemaInspector) GenerateAllStatements(drift *SchemaDrift) []string {
	var all []string
	all = append(all, si.GenerateAlterStatements(drift)...)
	all = append(all, si.GenerateCreateIndexStatements(drift)...)
	all = append(all, si.GenerateCreateTriggerStatements(drift)...)
	return all
}

// GetAllTables returns all user-managed tables in the database.
// For PostgreSQL, tables that were created by an installed extension
// (e.g. PostGIS's spatial_ref_sys) are excluded so that drift detection
// never reports false positives against them.
func (si *SchemaInspector) GetAllTables() ([]string, error) {
	var query string

	switch si.dialect {
	case PostgreSQL:
		// Exclude the migration bookkeeping tables and any table that is owned
		// by a pg_extension (deptype = 'e'), which covers PostGIS tables such as
		// spatial_ref_sys, geometry_columns, geography_columns, etc.
		query = `
			SELECT c.relname AS tablename
			FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = 'public'
			  AND c.relkind = 'r'
			  AND c.relname NOT IN ('migrations', 'migrations_lock')
			  AND NOT EXISTS (
			      SELECT 1
			      FROM pg_depend d
			      JOIN pg_extension e ON e.oid = d.refobjid
			      WHERE d.classid = 'pg_class'::regclass
			        AND d.objid = c.oid
			        AND d.deptype = 'e'
			  )
			ORDER BY c.relname
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
