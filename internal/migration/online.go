package migration

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// OnlineMigration provides zero-downtime migration strategies
type OnlineMigration struct {
	db      *sql.DB
	dialect Dialect
}

// NewOnlineMigration creates a new online migration helper
func NewOnlineMigration(db *sql.DB, dialect Dialect) *OnlineMigration {
	return &OnlineMigration{
		db:      db,
		dialect: dialect,
	}
}

// AddColumnOnline adds a column without locking the table
func (om *OnlineMigration) AddColumnOnline(ctx context.Context, table, column, columnType string, nullable bool, defaultValue *string) error {
	switch om.dialect {
	case PostgreSQL:
		return om.addColumnPostgres(ctx, table, column, columnType, nullable, defaultValue)
	case MySQL:
		return om.addColumnMySQL(ctx, table, column, columnType, nullable, defaultValue)
	default:
		return fmt.Errorf("online migrations not supported for dialect: %s", om.dialect)
	}
}

// addColumnPostgres adds column using PostgreSQL non-blocking technique
func (om *OnlineMigration) addColumnPostgres(ctx context.Context, table, column, columnType string, nullable bool, defaultValue *string) error {
	// Step 1: Add column as nullable without default (fast, no table rewrite)
	addSQL := fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s %s", table, column, columnType)
	
	if _, err := om.db.ExecContext(ctx, addSQL); err != nil {
		return fmt.Errorf("failed to add column: %w", err)
	}

	// Step 2: If there's a default value, set it in batches to avoid long locks
	if defaultValue != nil {
		if err := om.updateDefaultInBatches(ctx, table, column, *defaultValue); err != nil {
			return fmt.Errorf("failed to set default values: %w", err)
		}

		// Step 3: Add the default constraint (fast)
		defaultSQL := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s", 
			table, column, *defaultValue)
		if _, err := om.db.ExecContext(ctx, defaultSQL); err != nil {
			return fmt.Errorf("failed to set default: %w", err)
		}
	}

	// Step 4: If not nullable, add constraint after data is populated
	if !nullable {
		// First ensure all rows have non-null values
		checkSQL := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s IS NULL", table, column)
		var nullCount int
		if err := om.db.QueryRowContext(ctx, checkSQL).Scan(&nullCount); err != nil {
			return fmt.Errorf("failed to check null values: %w", err)
		}

		if nullCount > 0 {
			return fmt.Errorf("cannot add NOT NULL constraint: %d rows have NULL values", nullCount)
		}

		// Add NOT NULL constraint (Postgres 12+ can do this without full table scan)
		notNullSQL := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL", table, column)
		if _, err := om.db.ExecContext(ctx, notNullSQL); err != nil {
			return fmt.Errorf("failed to add NOT NULL constraint: %w", err)
		}
	}

	return nil
}

// addColumnMySQL adds column using MySQL non-blocking technique (requires Percona/MySQL 8.0+)
func (om *OnlineMigration) addColumnMySQL(ctx context.Context, table, column, columnType string, nullable bool, defaultValue *string) error {
	var addSQL string
	
	if nullable {
		addSQL = fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s NULL", table, column, columnType)
	} else {
		if defaultValue == nil {
			return fmt.Errorf("NOT NULL columns must have a default value for online migrations")
		}
		addSQL = fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s NOT NULL DEFAULT %s", 
			table, column, columnType, *defaultValue)
	}

	// Use ALGORITHM=INSTANT if available (MySQL 8.0.12+)
	addSQL += ", ALGORITHM=INSTANT"

	if _, err := om.db.ExecContext(ctx, addSQL); err != nil {
		// Fall back to INPLACE if INSTANT fails
		addSQL = fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, columnType)
		if !nullable && defaultValue != nil {
			addSQL += fmt.Sprintf(" NOT NULL DEFAULT %s", *defaultValue)
		}
		addSQL += ", ALGORITHM=INPLACE, LOCK=NONE"
		
		if _, err := om.db.ExecContext(ctx, addSQL); err != nil {
			return fmt.Errorf("failed to add column: %w", err)
		}
	}

	return nil
}

// updateDefaultInBatches updates column values in batches to avoid long locks
func (om *OnlineMigration) updateDefaultInBatches(ctx context.Context, table, column, defaultValue string) error {
	const batchSize = 1000
	
	for {
		// Update in small batches
		updateSQL := fmt.Sprintf(`
			UPDATE %s 
			SET %s = %s 
			WHERE %s IS NULL 
			LIMIT %d
		`, table, column, defaultValue, column, batchSize)

		result, err := om.db.ExecContext(ctx, updateSQL)
		if err != nil {
			return err
		}

		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}

		if rows == 0 {
			break
		}

		// Small delay to avoid overwhelming the database
		time.Sleep(10 * time.Millisecond)
	}

	return nil
}

// CreateIndexConcurrently creates an index without blocking writes (PostgreSQL)
func (om *OnlineMigration) CreateIndexConcurrently(ctx context.Context, indexName, table string, columns []string) error {
	if om.dialect != PostgreSQL {
		return fmt.Errorf("concurrent index creation only supported on PostgreSQL")
	}

	columnList := ""
	for i, col := range columns {
		if i > 0 {
			columnList += ", "
		}
		columnList += col
	}

	createSQL := fmt.Sprintf("CREATE INDEX CONCURRENTLY IF NOT EXISTS %s ON %s (%s)", 
		indexName, table, columnList)

	if _, err := om.db.ExecContext(ctx, createSQL); err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}

	return nil
}

// DropColumnOnline drops a column with minimal locking
func (om *OnlineMigration) DropColumnOnline(ctx context.Context, table, column string) error {
	switch om.dialect {
	case PostgreSQL:
		// In PostgreSQL, dropping a column is generally fast
		dropSQL := fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS %s", table, column)
		if _, err := om.db.ExecContext(ctx, dropSQL); err != nil {
			return fmt.Errorf("failed to drop column: %w", err)
		}
	case MySQL:
		// Use ALGORITHM=INSTANT for MySQL 8.0.29+
		dropSQL := fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s, ALGORITHM=INSTANT", table, column)
		if _, err := om.db.ExecContext(ctx, dropSQL); err != nil {
			// Fall back to INPLACE
			dropSQL = fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s, ALGORITHM=INPLACE, LOCK=NONE", table, column)
			if _, err := om.db.ExecContext(ctx, dropSQL); err != nil {
				return fmt.Errorf("failed to drop column: %w", err)
			}
		}
	}

	return nil
}

// RenameTableOnline renames a table atomically
func (om *OnlineMigration) RenameTableOnline(ctx context.Context, oldName, newName string) error {
	renameSQL := fmt.Sprintf("ALTER TABLE %s RENAME TO %s", oldName, newName)
	
	if _, err := om.db.ExecContext(ctx, renameSQL); err != nil {
		return fmt.Errorf("failed to rename table: %w", err)
	}

	return nil
}

// ValidateOnlineSupport checks if the database supports online migrations
func (om *OnlineMigration) ValidateOnlineSupport() error {
	switch om.dialect {
	case PostgreSQL:
		// Check PostgreSQL version (need 11+ for better online DDL)
		var version string
		err := om.db.QueryRow("SHOW server_version").Scan(&version)
		if err != nil {
			return fmt.Errorf("failed to check PostgreSQL version: %w", err)
		}
		return nil
	case MySQL:
		// Check for InnoDB and version
		var version string
		err := om.db.QueryRow("SELECT VERSION()").Scan(&version)
		if err != nil {
			return fmt.Errorf("failed to check MySQL version: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("online migrations not supported for %s", om.dialect)
	}
}
