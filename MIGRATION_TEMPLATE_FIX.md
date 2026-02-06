# Migration Template Fix - Test Results

## Issue Fixed
The migration template was incorrectly using the full migration name as the table name.

### Before (Wrong):
```sql
CREATE TABLE IF NOT EXISTS create_users_table (...)
DROP TABLE IF EXISTS create_users_table CASCADE;
```

### After (Correct):
```sql
CREATE TABLE IF NOT EXISTS users (...)
DROP TABLE IF EXISTS users CASCADE;
```

## Solution
Added `extractTableName()` function that:
- Strips `create_` prefix if present
- Strips `_table` suffix if present
- Leaves other migration names unchanged

## Test Results

### Test 1: create_users_table
```
Migration: create_users_table
Table name: users ✓
```

### Test 2: create_orders_table
```
Migration: create_orders_table
Table name: orders ✓
```

### Test 3: add_email_to_users
```
Migration: add_email_to_users
Table name: add_email_to_users ✓ (no stripping needed)
```

### Test 4: create_articles_and_comments_table
```
Migration: create_articles_and_comments_table
Table name: articles_and_comments ✓
```

## PostgreSQL Configuration

**DSN Format:**
```
postgres://user:password@localhost:5432/myapp
```

**Generated SQL:**
```sql
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

DROP TABLE IF EXISTS users CASCADE;
```

**Key Features:**
- Uses `SERIAL` for auto-increment
- Uses `CASCADE` for foreign key drops
- Uses `CURRENT_TIMESTAMP`
- `INTEGER` type for batch column

## MySQL/MariaDB Configuration

**DSN Format (URL style):**
```
mysql://user:password@localhost:3306/myapp
```

**DSN Format (Traditional style):**
```
user:password@tcp(localhost:3306)/myapp
```

**Generated SQL:**
```sql
CREATE TABLE IF NOT EXISTS users (
    id INT AUTO_INCREMENT PRIMARY KEY,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

DROP TABLE IF EXISTS users;
```

**Key Features:**
- Uses `INT AUTO_INCREMENT` instead of `SERIAL`
- Uses `TIMESTAMP DEFAULT CURRENT_TIMESTAMP`
- `INT` type for batch column (instead of `INTEGER`)
- No `CASCADE` support (MySQL)

## Database Type Auto-Detection

The tool automatically detects database type from DSN:

```go
if strings.HasPrefix(dsn, "mysql://") || strings.Contains(dsn, "@tcp") {
    // Use MySQL adapter
} else {
    // Use PostgreSQL adapter (default)
}
```

| DSN Pattern | Database |
|-------------|----------|
| `postgres://` | PostgreSQL |
| `postgresql://` | PostgreSQL |
| `mysql://` | MySQL |
| `user@tcp(...)` | MySQL |
| Default | PostgreSQL |

## Testing Commands

### Test PostgreSQL
```bash
# Set PostgreSQL DSN
export DATABASE_URL="postgres://user:pass@localhost:5432/mydb"

# Create and run migration
./vm make migration create_users_table
./vm migrate
./vm status
```

### Test MySQL
```bash
# Set MySQL DSN
export DATABASE_URL="mysql://user:pass@localhost:3306/mydb"

# Create and run migration
./vm make migration create_users_table
./vm migrate
./vm status
```

## Build Status
✅ `go build -o vm .` - SUCCESS (no errors)
✅ Template extraction - WORKING
✅ PostgreSQL format - VERIFIED
✅ MySQL format - AUTO-DETECTED

## Files Modified
- `internal/migration/create.go` - Added `extractTableName()` function
- Template now uses extracted table name instead of full migration name

## Code Changes

```go
// New function added:
func extractTableName(migrationName string) string {
	name := migrationName
	
	// Remove "create_" prefix if present
	if strings.HasPrefix(name, "create_") {
		name = strings.TrimPrefix(name, "create_")
	}
	
	// Remove "_table" suffix if present
	if strings.HasSuffix(name, "_table") {
		name = strings.TrimSuffix(name, "_table")
	}
	
	return name
}

// Updated template generation:
func generateMigrationTemplate(name string) string {
	upperName := strings.ToUpper(name)
	tableName := extractTableName(name)  // Now uses extracted name
	// ... rest of template using tableName instead of name
}
```
