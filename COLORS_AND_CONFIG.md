# Colored Output & Config File Enhancement

## What's New

### 🎨 Colorized Terminal Output

All migration operations now display with beautiful, easy-to-read colors:

- ✅ **Green** - Successful operations (`Migrated`, `Rolled back`, `Created`)
- ⚠️ **Yellow** - Warnings (`Pending migrations`, `Missing sections`)
- ❌ **Red** - Errors (`Failed operations`, `Connection errors`)
- ℹ️ **Cyan** - Informational messages (`Running migrations`, `Status`)

**Example Output:**
```
✓ Migrated: 1707123456_create_users_table.sql
✓ Migrated: 1707123457_create_posts_table.sql
✓ Successfully ran 2 migration(s)

⚠ No UP section found in migration test_table.sql, skipping

🐘 Migration Status [dev]
────────────────────────────────────────────────────────────────────────────────
Migration                                | Status
────────────────────────────────────────────────────────────────────────────────
1707123456_create_users_table.sql        | ✓ Batch 1
1707123457_create_posts_table.sql        | ✓ Batch 1
1707123458_add_column.sql                | ⏳ Pending
────────────────────────────────────────────────────────────────────────────────

Summary: 2 executed, 1 pending
```

### ⚙️ Configuration Files

No more typing `--dsn` every time! Use configuration files instead.

**Create `.vorzela` file:**
```ini
DATABASE_URL=postgres://localhost:5432/myapp
VORZELA_ENV=dev
MIGRATION_PATH=./migrations
```

**Then simply:**
```bash
vm migrate
vm status
vm rollback
```

### 📋 Priority System

Configuration is loaded in this priority order:

1. **CLI Flags** (Highest)
   ```bash
    vm migrate --dsn "postgres://override/db"
   ```

2. **Environment Variables**
   ```bash
   export DATABASE_URL="postgres://localhost/myapp"
    vm migrate
   ```

3. **`.vorzela` Config File**
   ```ini
   DATABASE_URL=postgres://localhost/myapp
   ```

4. **`.env` File**
   ```env
   DATABASE_URL=postgres://localhost/myapp
   ```

5. **Defaults** (Lowest)
   - `VORZELA_ENV=dev`
   - `MIGRATION_PATH=./migrations`

## Features

### ✅ Multiple Configuration Methods

**Method 1: Config File (.vorzela)**
```ini
# .vorzela - Check into git
DATABASE_URL=postgres://localhost/myapp_dev
VORZELA_ENV=dev
```

```bash
vm migrate
```

**Method 2: Environment Variables**
```bash
export DATABASE_URL="postgres://localhost/myapp"
vm migrate
```

**Method 3: CLI Flags**
```bash
vm migrate --dsn "postgres://localhost/myapp"
```

### ✅ .vorzela Configuration File

**Purpose:**
- Project-specific configuration
- Share with team members (checked into git)
- Local database URLs and settings

**Example:**
```ini
# .vorzela
DATABASE_URL=postgres://localhost:5432/myapp
VORZELA_ENV=dev
MIGRATION_PATH=./migrations
```

**Benefits:**
- No environment variables needed
- Works in subdirectories (searches parent dirs)
- Team consistency

### ✅ .env Support

**Purpose:**
- Local machine overrides (NOT checked into git)
- Sensitive data (passwords, keys)
- CI/CD integration

**Example:**
```env
# .env - Add to .gitignore!
DATABASE_URL=postgres://user:secure_password@localhost/myapp
```

### ✅ Colored Output

All commands now produce colored output for better readability:

**Migration Execution:**
```bash
$  vm migrate
✓ Migrated: 1707123456_create_users_table.sql
✓ Migrated: 1707123457_create_posts_table.sql
✓ Successfully ran 2 migration(s)
```

**Status Display:**
```bash
$  vm status
🐘 Migration Status [dev]
────────────────────────────────────────────────────────────────────────────────
Migration                          | Status
────────────────────────────────────────────────────────────────────────────────
1707123456_create_users_table.sql  | ✓ Batch 1
1707123457_create_posts_table.sql  | ✓ Batch 2
1707123458_add_column.sql          | ⏳ Pending
────────────────────────────────────────────────────────────────────────────────

Summary: 2 executed, 1 pending
```

**Error Messages:**
```bash
$  vm migrate
✗ database URL is required. Set DATABASE_URL env var, create .vorzela config file, or use --dsn flag
```

## Usage Examples

### Local Development (Config File)

**Setup once:**
```bash
# Create .vorzela
cat > .vorzela << EOF
DATABASE_URL=postgres://localhost:5432/myapp
VORZELA_ENV=dev
EOF
```

**Use anytime:**
```bash
vm make migration create_users_table
vm migrate
vm status
vm rollback
```

### CI/CD (Environment Variables)

```yaml
# GitHub Actions
jobs:
  migrate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - run: go build -o vm main.go
      - run:  vm migrate
        env:
          DATABASE_URL: ${{ secrets.DATABASE_URL }}
          VORZELA_ENV: prod
```

### Production (CLI Flags with Env Vars)

```bash
# Combined approach
export VORZELA_ENV=server

vm migrate --dsn "postgres://secure_url/prod_db"
vm status --dsn "postgres://secure_url/prod_db"
```

## Migration Examples

### Example 1: Basic Setup

```bash
# Create .vorzela
cat > .vorzela << EOF
DATABASE_URL=postgres://localhost/myapp
VORZELA_ENV=dev
EOF

# Create migration
vm make migration create_users_table

# Edit migrations/dev/TIMESTAMP_create_users_table.sql
# Add your SQL

# Run migration
vm migrate

# Check status
vm status

# Rollback if needed
vm rollback
```

### Example 2: Multiple Environments

**Development (.vorzela):**
```ini
DATABASE_URL=postgres://localhost/myapp_dev
VORZELA_ENV=dev
```

**Production (Environment variables):**
```bash
export DATABASE_URL="postgres://prod_user:password@prod.db/myapp"
export VORZELA_ENV=server

vm migrate
```

### Example 3: Docker

```dockerfile
FROM golang:1.21

WORKDIR /app
COPY . .
RUN go build -o vm main.go

ENV DATABASE_URL=postgres://db:5432/myapp
ENV VORZELA_ENV=server

ENTRYPOINT ["./vm", "migrate"]
```

## Configuration Files

### .vorzela (Project Configuration)
```ini
# Check into git ✓
# Shared with team
# Project-specific settings

DATABASE_URL=postgres://localhost/myapp_dev
VORZELA_ENV=dev
MIGRATION_PATH=./migrations
```

### .vorzela.example (Template)
```bash
cp .vorzela.example .vorzela
# Edit with your local values
```

### .env (Local Secrets)
```env
# Do NOT check into git ✗
# Local machine only
# Sensitive data (passwords, tokens)

DATABASE_URL=postgres://user:password@localhost/myapp
```

### Environment Variables (CI/CD)
```bash
# Set in CI/CD system (GitHub Actions, GitLab CI, etc.)
# Keep sensitive values in secrets management
# Override locally if needed

DATABASE_URL="postgres://secure/password@prod/db"
VORZELA_ENV=server
```

## Best Practices

### ✅ Do This

1. **Use .vorzela for team development**
   ```ini
   DATABASE_URL=postgres://localhost/myapp
   VORZELA_ENV=dev
   ```

2. **Use env vars for CI/CD and production**
   ```bash
   export DATABASE_URL="postgres://secure/password@prod/db"
   ```

3. **Keep .env in .gitignore**
   ```
   # .gitignore
   .env
   .env.local
   ```

4. **Document configuration**
   - Check in `.vorzela.example`
   - Provide setup instructions

### ❌ Don't Do This

1. **Don't hardcode database URLs in code**
2. **Don't check in `.env` with secrets**
3. **Don't use `--dsn` for every command**
4. **Don't commit production passwords**

## Troubleshooting

### "database URL is required"

**Problem:** No configuration found

**Solution:** Use one of:
```bash
# 1. Create .vorzela
echo 'DATABASE_URL=postgres://localhost/myapp' > .vorzela
vm migrate

# 2. Set environment variable
export DATABASE_URL="postgres://localhost/myapp"
vm migrate

# 3. Use CLI flag
vm migrate --dsn "postgres://localhost/myapp"
```

### Colors not showing

**Problem:** Colors not displaying in terminal

**Solution:**
- Colors work in most modern terminals
- Force color output (if supported by terminal):
  ```bash
  FORCE_COLOR=1  vm migrate
  ```
- Fall back to CLI flag method

### Config file not found

**Problem:** `.vorzela` file not being loaded

**Solution:**
- File must be named exactly `.vorzela`
- Must be in current or parent directory
- Tool searches up to 5 parent directories
- Check file exists:
  ```bash
  ls -la .vorzela
  cat .vorzela
  ```

## Migration Path

### From --dsn to .vorzela

**Before:**
```bash
vm migrate --dsn "postgres://localhost/myapp"
vm status --dsn "postgres://localhost/myapp"
vm rollback --dsn "postgres://localhost/myapp"
```

**After:**
```bash
# Create once
echo 'DATABASE_URL=postgres://localhost/myapp' > .vorzela

# Use many times
vm migrate
vm status
vm rollback
```

## Summary

| Feature | Before | After |
|---------|--------|-------|
| Config | CLI flags only | Files + Env vars + Flags |
| Output | Plain text | 🎨 Colored & beautiful |
| DSN Repetition | Every command | Once in config |
| Team Sharing | Difficult | Easy (.vorzela) |
| CI/CD Setup | Multiple flags | Env vars + config |

Now enjoy faster, more colorful migrations! 🚀
