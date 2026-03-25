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
	IsUnique bool // column-level UNIQUE constraint declared in the migration
}

// ConstraintInfo represents a foreign-key constraint on a table.
type ConstraintInfo struct {
	Name       string // constraint name (e.g. "fk_orders_user_id")
	TableName  string
	Columns    []string // local columns involved in the FK
	RefTable   string   // referenced table
	RefColumns []string // referenced columns
	OnDelete   string   // NO ACTION, RESTRICT, CASCADE, SET NULL, SET DEFAULT (empty = unspecified)
	OnUpdate   string
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
	Table              string
	AddedColumns       []ColumnInfo // columns in DB not defined in any migration
	MissingColumns     []ColumnInfo // columns defined in migrations but absent from DB
	ModifiedColumns    []ColumnModification
	MissingIndexes     []IndexInfo // indexes defined in migrations but absent from DB
	ExtraIndexes       []IndexInfo // indexes in DB that back orphaned/dropped columns
	MissingTriggers    []TriggerInfo
	MissingConstraints []ConstraintInfo // FK constraints in migrations but absent from DB
	ExtraConstraints   []ConstraintInfo // FK constraints in DB not defined in any migration
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

// GetTableConstraints retrieves all foreign-key constraints for a table from the DB.
func (si *SchemaInspector) GetTableConstraints(tableName string) ([]ConstraintInfo, error) {
	switch si.dialect {
	case PostgreSQL:
		return si.getPostgresTableConstraints(tableName)
	case MySQL, MariaDB:
		return si.getMySQLTableConstraints(tableName)
	default:
		return nil, fmt.Errorf("unsupported dialect for constraint inspection: %s", si.dialect)
	}
}

func (si *SchemaInspector) getPostgresTableConstraints(tableName string) ([]ConstraintInfo, error) {
	query := `
		SELECT
			c.conname AS constraint_name,
			(SELECT string_agg(a.attname, ',' ORDER BY u.pos)
			 FROM unnest(c.conkey) WITH ORDINALITY AS u(attnum, pos)
			 JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = u.attnum) AS columns,
			f.relname AS ref_table,
			(SELECT string_agg(af.attname, ',' ORDER BY uf.pos)
			 FROM unnest(c.confkey) WITH ORDINALITY AS uf(attnum, pos)
			 JOIN pg_attribute af ON af.attrelid = c.confrelid AND af.attnum = uf.attnum) AS ref_columns,
			CASE c.confdeltype
				WHEN 'a' THEN 'NO ACTION'
				WHEN 'r' THEN 'RESTRICT'
				WHEN 'c' THEN 'CASCADE'
				WHEN 'n' THEN 'SET NULL'
				WHEN 'd' THEN 'SET DEFAULT'
			END AS on_delete,
			CASE c.confupdtype
				WHEN 'a' THEN 'NO ACTION'
				WHEN 'r' THEN 'RESTRICT'
				WHEN 'c' THEN 'CASCADE'
				WHEN 'n' THEN 'SET NULL'
				WHEN 'd' THEN 'SET DEFAULT'
			END AS on_update
		FROM pg_constraint c
		JOIN pg_class t ON t.oid = c.conrelid
		JOIN pg_namespace n ON n.oid = t.relnamespace
		JOIN pg_class f ON f.oid = c.confrelid
		WHERE t.relname = $1
		  AND n.nspname = 'public'
		  AND c.contype = 'f'
		ORDER BY c.conname
	`
	rows, err := si.db.Query(query, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to query FK constraints: %w", err)
	}
	defer rows.Close()

	var constraints []ConstraintInfo
	for rows.Next() {
		var ci ConstraintInfo
		var colList, refColList string
		ci.TableName = tableName
		if err := rows.Scan(&ci.Name, &colList, &ci.RefTable, &refColList, &ci.OnDelete, &ci.OnUpdate); err != nil {
			return nil, err
		}
		ci.Columns = splitCommaList(colList)
		ci.RefColumns = splitCommaList(refColList)
		constraints = append(constraints, ci)
	}
	return constraints, rows.Err()
}

func (si *SchemaInspector) getMySQLTableConstraints(tableName string) ([]ConstraintInfo, error) {
	query := `
		SELECT
			kcu.CONSTRAINT_NAME,
			GROUP_CONCAT(kcu.COLUMN_NAME ORDER BY kcu.ORDINAL_POSITION SEPARATOR ','),
			kcu.REFERENCED_TABLE_NAME,
			GROUP_CONCAT(kcu.REFERENCED_COLUMN_NAME ORDER BY kcu.ORDINAL_POSITION SEPARATOR ','),
			rc.DELETE_RULE,
			rc.UPDATE_RULE
		FROM information_schema.KEY_COLUMN_USAGE kcu
		JOIN information_schema.REFERENTIAL_CONSTRAINTS rc
			ON rc.CONSTRAINT_SCHEMA = kcu.TABLE_SCHEMA
			AND rc.CONSTRAINT_NAME = kcu.CONSTRAINT_NAME
		WHERE kcu.TABLE_SCHEMA = DATABASE()
		  AND kcu.TABLE_NAME = ?
		  AND kcu.REFERENCED_TABLE_NAME IS NOT NULL
		GROUP BY kcu.CONSTRAINT_NAME, kcu.REFERENCED_TABLE_NAME, rc.DELETE_RULE, rc.UPDATE_RULE
		ORDER BY kcu.CONSTRAINT_NAME
	`
	rows, err := si.db.Query(query, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to query FK constraints: %w", err)
	}
	defer rows.Close()

	var constraints []ConstraintInfo
	for rows.Next() {
		var ci ConstraintInfo
		var colList, refColList string
		ci.TableName = tableName
		if err := rows.Scan(&ci.Name, &colList, &ci.RefTable, &refColList, &ci.OnDelete, &ci.OnUpdate); err != nil {
			return nil, err
		}
		ci.Columns = splitCommaList(colList)
		ci.RefColumns = splitCommaList(refColList)
		constraints = append(constraints, ci)
	}
	return constraints, rows.Err()
}

// splitCommaList splits a comma-separated string into a trimmed slice.
func splitCommaList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// constraintKey returns a normalised string key for a ConstraintInfo that can
// be used to match expected vs actual constraints regardless of name differences
// (Postgres auto-names inline REFERENCES differently from our fk_ convention).
func constraintKey(ci ConstraintInfo) string {
	cols := make([]string, len(ci.Columns))
	copy(cols, ci.Columns)
	sort.Strings(cols)
	return strings.ToLower(ci.TableName) + "|" +
		strings.Join(cols, ",") + "|" +
		strings.ToLower(ci.RefTable)
}

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
// It handles both AddedColumns (columns extra in DB that should be dropped) and
// MissingColumns (columns defined in migrations but absent from DB — these need ADD COLUMN).
// ExtraIndexes (indexes on orphaned columns) are dropped first so the subsequent
// DROP COLUMN statements are not blocked by dependent indexes.
func (si *SchemaInspector) GenerateAlterStatements(drift *SchemaDrift) []string {
	var statements []string

	// ExtraIndexes: drop indexes that cover orphaned columns before dropping the columns.
	for _, idx := range drift.ExtraIndexes {
		statements = append(statements,
			fmt.Sprintf("DROP INDEX IF EXISTS %s; -- index on orphaned column(s) %s",
				idx.Name, strings.Join(idx.Columns, ", ")))
	}

	// AddedColumns: columns that exist in the DB but are not defined in any migration.
	// These are unexpected / orphaned columns — drop them to bring the DB in line with
	// the migration-defined schema.
	for _, col := range drift.AddedColumns {
		statements = append(statements,
			fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS %s;",
				drift.Table, col.Name))
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

		// Non-nullable UNIQUE columns cannot be safely auto-added to a populated
		// table because existing rows may violate the constraint. The user must
		// create a targeted migration file (e.g. add_<col>_to_<table>) that
		// backfills unique values before enforcing the constraint.
		// Nullable UNIQUE columns are safe: NULLs are never compared by UNIQUE.
		if col.IsUnique && !col.Nullable {
			statements = append(statements,
				fmt.Sprintf(
					"-- UNIQUE COLUMN: %s.%s — cannot safely auto-add to a populated table.\n"+
						"-- Create an add_%s_to_%s migration file to backfill values and add the UNIQUE constraint manually.",
					drift.Table, col.Name, col.Name, drift.Table))
			continue
		}

		// Build the column definition, preserving NOT NULL and DEFAULT so that pgx
		// cannot scan a NULL into a non-pointer Go field after the column is added.
		colDef := col.Type
		if !col.Nullable {
			if col.Default.Valid {
				colDef += " NOT NULL DEFAULT " + col.Default.String
			} else {
				// Adding a NOT NULL column to a populated table without a DEFAULT
				// would fail. Tell the user to handle it in a dedicated migration.
				statements = append(statements,
					fmt.Sprintf(
						"-- NOT NULL COLUMN: %s.%s (%s) has no DEFAULT value.\n"+
							"-- Create an add_%s_to_%s migration file to supply a DEFAULT before enforcing NOT NULL.",
						drift.Table, col.Name, col.Type, col.Name, drift.Table))
				continue
			}
		}

		statements = append(statements,
			fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s %s;",
				drift.Table, col.Name, colDef))
	}

	return statements
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
// ADD CONSTRAINT for missing FK constraints, and advisory comments for
// missing triggers.
func (si *SchemaInspector) GenerateAllStatements(drift *SchemaDrift) []string {
	var all []string
	all = append(all, si.GenerateAlterStatements(drift)...)
	all = append(all, si.GenerateCreateIndexStatements(drift)...)
	all = append(all, si.GenerateAddConstraintStatements(drift)...)
	all = append(all, si.GenerateCreateTriggerStatements(drift)...)
	return all
}

// GenerateAddConstraintStatements generates ALTER TABLE … ADD CONSTRAINT …
// FOREIGN KEY statements for each missing FK in the drift report.
func (si *SchemaInspector) GenerateAddConstraintStatements(drift *SchemaDrift) []string {
	if len(drift.MissingConstraints) == 0 {
		return nil
	}
	var stmts []string
	for _, ci := range drift.MissingConstraints {
		name := ci.Name
		if name == "" {
			name = generateConstraintName(ci.TableName, ci.Columns)
		}
		cols := strings.Join(ci.Columns, ", ")
		refCols := strings.Join(ci.RefColumns, ", ")
		stmt := fmt.Sprintf(
			"ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)",
			ci.TableName, name, cols, ci.RefTable, refCols)
		if ci.OnDelete != "" && ci.OnDelete != "NO ACTION" {
			stmt += " ON DELETE " + ci.OnDelete
		}
		if ci.OnUpdate != "" && ci.OnUpdate != "NO ACTION" {
			stmt += " ON UPDATE " + ci.OnUpdate
		}
		stmt += ";"
		stmts = append(stmts, stmt)
	}
	return stmts
}

// GenerateDropConstraintStatements generates ALTER TABLE … DROP CONSTRAINT
// statements for each extra FK in the drift report (FK in DB but not in migrations).
func (si *SchemaInspector) GenerateDropConstraintStatements(drift *SchemaDrift) []string {
	if len(drift.ExtraConstraints) == 0 {
		return nil
	}
	var stmts []string
	for _, ci := range drift.ExtraConstraints {
		var stmt string
		if si.dialect == MySQL || si.dialect == MariaDB {
			stmt = fmt.Sprintf("ALTER TABLE %s DROP FOREIGN KEY %s;", ci.TableName, ci.Name)
		} else {
			stmt = fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s;", ci.TableName, ci.Name)
		}
		stmts = append(stmts, stmt)
	}
	return stmts
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

// indexCoveredByOrphans reports whether every column in idx is present in the
// orphanedSet (i.e. a set of column names that are about to be dropped).
// Such indexes must be dropped before the DROP COLUMN statements are executed.
func indexCoveredByOrphans(idx IndexInfo, orphanedSet map[string]bool) bool {
	if len(idx.Columns) == 0 {
		return false
	}
	for _, c := range idx.Columns {
		if !orphanedSet[strings.ToLower(c)] {
			return false
		}
	}
	return true
}
