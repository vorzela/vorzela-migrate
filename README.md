# Vorzela Migration Tool (v1.1.2)


## ✨ Features

- 🎨 **Colorized Output** - Beautiful, easy-to-read colored terminal output
- ⚙️ **Multiple Configuration Methods** - `.vm` config files, `.env` files, or environment variables
- 🚀 **No DSN Flag Required** - Use config files instead of repeating `--dsn` flag
 - 🐘 **Multi-Database Support** - PostgreSQL and MySQL/MariaDB with automatic detection
- � **Batch Tracking** - Organized rollback with batch numbers
- 🔒 **Transaction Safety** - All-or-nothing migration execution
- ⚠️ **Warning System** - Alerts for missing migration sections
- 🌍 **Global CLI** - Install and use from anywhere
- 📚 **Comprehensive Docs** - Full documentation and examples

## Requirements

- **Go** 1.16+ (uses modern `os` package APIs, no deprecated `io/ioutil`)
- **PostgreSQL** 10+ or **MySQL** 5.7+ or **MariaDB** 10.3+

## Installation

```bash
go mod download
go build -o vm main.go
```

### Installers (Recommended)

**Linux/macOS (bash):**

```bash
curl -fsSL https://raw.githubusercontent.com/vorzela/vorzela-migrate/main/install.sh | bash
```

**Windows (PowerShell):**

```powershell
iex (New-Object Net.WebClient).DownloadString('https://raw.githubusercontent.com/vorzela/vorzela-migrate/main/install.ps1')
```

Note: The installers try to add the install directory to your PATH. On Linux/macOS this updates your shell profile (best effort). On Windows this updates your user PATH. Restart your shell if the `vm` command is not found right away.

For more options and platform notes, see [INSTALL.md](INSTALL.md).

## Supported Databases

- **PostgreSQL** 10+ (via pgx v5)
- **MySQL** 5.7+ (via go-sql-driver/mysql)
- **MariaDB** 10.3+ (via go-sql-driver/mysql)

Database type is automatically detected from the DSN URL.

## Usage

### Quick Start (No --dsn Needed!)

Create `.vm` file:

**PostgreSQL:**
```ini
DATABASE_URL=postgres://user:password@localhost:5432/myapp
```

**MySQL/MariaDB:**
```ini
DATABASE_URL=mysql://user:password@localhost:3306/myapp
```

Then simply:
```bash
vm migrate
vm status
vm rollback
```

### Create a new migration

```bash
# PostgreSQL/MySQL (default)
vm make migration create_users_table
vm make migration add_email_to_users
vm make migration create_posts_table

```

The migration file will be created in the `migrations/` directory with a timestamp prefix.

### Run migrations

**Option 1: Using .vm config file** (Recommended for local development)
```bash
vm migrate
```

**Option 2: Using environment variables** (Recommended for CI/CD)
```bash
export DATABASE_URL="postgres://user:pass@localhost:5432/db"
vm migrate
```

Or for MySQL:
```bash
export DATABASE_URL="mysql://user:pass@localhost:3306/db"
vm migrate
```

**Option 3: Using CLI flags** (For one-off commands)

PostgreSQL:
```bash
vm migrate --dsn "postgres://user:pass@localhost:5432/db"
```

MySQL:
```bash
vm migrate --dsn "mysql://user:pass@localhost:3306/db"
```

```bash
```

### Check migration status

```bash
vm status
```

Or with explicit DSN:
```bash
vm status --dsn "postgres://user:pass@localhost:5432/db"
```

### Rollback migrations

```bash
# Rollback last batch
vm rollback

# Rollback last 3 batches
vm rollback --steps 3
```

### Refresh (rollback all and re-run)

```bash
vm refresh --dsn "postgres://user:pass@localhost:5432/db"
```

## Global Installation

To make `vm` available globally:

```bash
# Build the binary
go build -o vm main.go

# Copy to a location in your PATH
sudo cp vm /usr/local/bin/vm
chmod +x /usr/local/bin/vm

# Now you can use from anywhere
vm make migration create_table_name_table
vm migrate --dsn "postgres://user:pass@localhost:5432/db"
```

## Migration File Format

Migrations are SQL files with a special format. The tool automatically extracts the UP and DOWN sections:

```sql
-- Migration: CREATE_USERS_TABLE
-- Created at: 2026-02-05 10:30:45

-- ⬆ Up (Run when migrating forward)
BEGIN;

CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMIT;

-- ⬇ Down (Run when rolling back)
BEGIN;

DROP TABLE IF EXISTS users;

COMMIT;
```

## Configuration

### Three Ways to Configure

**1. Configuration File (.vm)** - Recommended for local development
```ini
DATABASE_URL=postgres://localhost:5432/myapp
MIGRATION_PATH=./migrations
```

**2. Environment Variables** - Recommended for CI/CD and production
```bash
export DATABASE_URL="postgres://user:pass@localhost:5432/db"
```

**3. CLI Flags** - For one-off commands
```bash
vm migrate --dsn "postgres://user:pass@localhost:5432/db"
```

### Priority (Highest to Lowest)
1. CLI flags (`--dsn`, `--path`)
2. Environment variables (`DATABASE_URL`)
3. `.vm` config file
4. `.env` file
5. Default values

See [CONFIG_ENHANCED.md](CONFIG_ENHANCED.md) for detailed configuration guide.

## Migration File Format

| Command | Description |
|---------|-------------|
| `make migration <name>` | Create a new migration file |
| `migrate` | Run all pending migrations |
| `rollback [--steps N]` | Rollback last N batches |
| `refresh` | Rollback all and re-run all migrations |
| `status` | Show migration status |

## Flags

- `-d, --dsn` - Database connection string
- `-p, --path` - Path to migrations directory (default: migrations)
- `--steps` - Number of batches to rollback (only for rollback command)

## Examples

### Development workflow

```bash
# Create migrations
vm make migration create_users_table
vm make migration add_posts_table
vm make migration add_indexes

# Run migrations
vm migrate --dsn "postgres://user:pass@localhost:5432/myapp"

# Check status
vm status --dsn "postgres://user:pass@localhost:5432/myapp"

# Rollback if needed
vm rollback --dsn "postgres://user:pass@localhost:5432/myapp"
```

### Production deployment

```bash
# Run migrations on production database
vm migrate --dsn "postgres://user:pass@prod:5432/myapp"

# Check production status
vm status --dsn "postgres://user:pass@prod:5432/myapp"
```

## Error Handling

The tool provides helpful warnings and errors:

- ⚠️ Missing UP section in migration file
- ⚠️ Missing DOWN section in migration file
- ❌ Database connection failures
- ❌ Invalid migration names
- ❌ SQL execution errors with detailed messages

## Project Structure

```
vorzela-migrate/
├── main.go
├── go.mod
├── README.md
├── cmd/
│   ├── make.go
│   ├── migrate.go
│   ├── rollback.go
│   ├── refresh.go
│   ├── fresh.go
│   └── status.go
├── internal/
│   ├── config/
│   │   └── config.go
│   ├── database/
│   │   └── connection.go
│   ├── db/
│   │   ├── db.go
│   │   ├── postgres.go
│   │   └── mysql.go
│   └── migration/
│       ├── types.go
│       ├── create.go
│       ├── executor.go
│       ├── status.go
│       └── dialect.go
└── migrations/
    ├── .gitkeep
    └── 1707123456_create_users_table.sql
```

## Tips & Best Practices

1. **Always use snake_case** for migration names: `create_users_table`, `add_email_column`
2. **Use transactions** in your migrations (BEGIN/COMMIT) for safety
3. **Test rollbacks** to ensure your DOWN sections work correctly
4. **Separate concerns** - one migration per table/feature
5. **Use descriptive names** that clearly indicate what the migration does
6. **Keep DOWN migrations reversible** - don't lose data unless intentional

## Troubleshooting

### "migration file already exists"
The migration was already created. Check the `migrations/` directory.

### "failed to connect to database"
Check your DATABASE_URL or --dsn flag. Ensure PostgreSQL is running.

### "invalid migration name"
Use snake_case with only lowercase letters, numbers, and underscores.

### "No UP section found"
Ensure your migration file has the proper format with `-- ⬆ Up` marker.

## License

MIT
