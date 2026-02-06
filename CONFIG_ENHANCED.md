# Configuration Guide - Enhanced

## Three Ways to Configure

The Vorzela migration tool supports three configuration methods with clear priority:

### 1. CLI Flags (Highest Priority)
```bash
vm migrate --dsn "postgres://localhost/myapp" --env dev
vm migrate -d "postgres://localhost/myapp" -e server
```

### 2. Environment Variables
```bash
export DATABASE_URL="postgres://localhost/myapp"
export VORZELA_ENV=dev
vm migrate
```

### 3. Configuration File (Lowest Priority)
Create a `.vorzela` file in your project root:
```ini
DATABASE_URL=postgres://localhost/myapp
VORZELA_ENV=dev
MIGRATION_PATH=./migrations
```

Then simply run:
```bash
vm migrate
```

## Configuration Priority (Highest to Lowest)

1. **CLI Flags** - `--dsn`, `--env`, `--path`
2. **Environment Variables** - `DATABASE_URL`, `VORZELA_ENV`
3. **.vorzela Config File** - `.vorzela` in current or parent directory
4. **.env File** - Loaded automatically if present
5. **Defaults** - `VORZELA_ENV=dev`, `MIGRATION_PATH=./migrations`

## Configuration Methods in Detail

### Using CLI Flags (Always Works)

```bash
# Required DSN when not configured elsewhere
vm migrate --dsn $DATABASE_URL

# Specify environment
vm migrate --dsn $DATABASE_URL --env server

# Specify migration path
vm migrate --dsn $DATABASE_URL --path ./db/migrations
```

### Using Environment Variables (Recommended for CI/CD)

```bash
# Single setup, multiple commands
export DATABASE_URL="postgres://localhost/myapp"
export VORZELA_ENV=dev

vm migrate
vm status
vm rollback
```

### Using .vorzela Config File (Recommended for Local Development)

**Creating a .vorzela file:**

```ini
# .vorzela
DATABASE_URL=postgres://localhost/myapp
VORZELA_ENV=dev
MIGRATION_PATH=./migrations
```

**Benefits:**
- No need to set environment variables every time
- Can be checked into git (project-specific configuration)
- Works from any subdirectory in the project

**Example:**

```bash
# After creating .vorzela file, simply run:
vm migrate
vm status
vm rollback

# No --dsn flag needed!
```

### Using .env File (Legacy Support)

The tool automatically loads `.env` files:

```env
# .env
DATABASE_URL=postgres://localhost/myapp
VORZELA_ENV=dev
```

**Note:** Add `.env` to `.gitignore` for sensitive data. Use `.vorzela` for shared project config.

## Complete Configuration Example

### Project Structure
```
myproject/
├── .vorzela              # Shared project configuration
├── .env                  # Local secrets (in .gitignore)
├── migrations/
│   ├── dev/
│   └── server/
├── Makefile
└── main.go
```

### .vorzela File
```ini
DATABASE_URL=postgres://localhost/myapp_dev
VORZELA_ENV=dev
MIGRATION_PATH=./migrations
```

### .env File (Local, in .gitignore)
```env
# .env - Local overrides, NOT checked in
DATABASE_URL=postgres://user:password@localhost:5432/myapp
```

### Usage
```bash
# Uses .vorzela configuration by default
vm migrate

# Override with environment variable
DATABASE_URL=postgres://prod/myapp vm migrate --env server

# Override with flag (highest priority)
vm migrate --dsn "postgres://override/myapp"
```

## Environment-Specific Configuration

### Development (.vorzela)
```ini
DATABASE_URL=postgres://localhost/myapp
VORZELA_ENV=dev
```

### Production (Environment Variables)
```bash
export DATABASE_URL="postgres://prod-user:pass@prod-server:5432/myapp"
export VORZELA_ENV=server
vm migrate
```

### Staging (CLI Flags)
```bash
vm migrate \
  --dsn "postgres://staging-user:pass@staging:5432/myapp" \
  --env server
```

## Connection String Format

### PostgreSQL Connection String Formats

**Local (default):**
```
postgres://localhost/myapp
```

**With credentials:**
```
postgres://user:password@localhost:5432/myapp
```

**With SSL (production):**
```
postgres://user:password@prod-server:5432/myapp?sslmode=require
```

**With multiple options:**
```
postgres://user:password@host:5432/myapp?sslmode=require&connect_timeout=5&application_name=vm
```

## Configuration for CI/CD

### GitHub Actions
```yaml
jobs:
  migrate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - uses: actions/setup-go@v2
      - run: go build -o vm main.go
      - run: vm migrate
        env:
          DATABASE_URL: ${{ secrets.DATABASE_URL }}
          VORZELA_ENV: dev
```

### Docker
```dockerfile
FROM golang:1.21

WORKDIR /app
COPY . .

RUN go build -o vm main.go

ENV DATABASE_URL=postgres://db:5432/myapp
ENV VORZELA_ENV=server

ENTRYPOINT ["./vm"]
```

### Kubernetes
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: vm-config
data:
  DATABASE_URL: "postgres://db-service:5432/myapp"
  VORZELA_ENV: "server"
```

## Best Practices

### ✅ DO:

1. **Use .vorzela for development**
   ```ini
   DATABASE_URL=postgres://localhost/myapp
   ```

2. **Use environment variables for production**
   ```bash
   export DATABASE_URL="postgres://secure/password@prod/myapp"
   ```

3. **Keep sensitive data out of version control**
   - Add `.env` to `.gitignore`
   - Check in `.vorzela.example` instead

4. **Document configuration requirements**
   ```bash
   cp .vorzela.example .vorzela
   # Edit .vorzela with your local database
   ```

### ❌ DON'T:

1. **Don't hardcode DSN in code**
   ```go
   // BAD
   dsn := "postgres://localhost/myapp"
   ```

2. **Don't check in .env with secrets**
   ```
   # .gitignore
   .env
   .env.local
   ```

3. **Don't commit production passwords**
   ```ini
   # Bad - Don't do this!
   DATABASE_URL=postgres://prod_user:production_password@prod:5432/myapp
   ```

## Migrating Between Configuration Methods

### From CLI Flags to .vorzela

**Before:**
```bash
vm migrate --dsn "postgres://localhost/myapp" --env dev
```

**After:**
```bash
# Create .vorzela
echo 'DATABASE_URL=postgres://localhost/myapp' > .vorzela
echo 'VORZELA_ENV=dev' >> .vorzela

# Now just run:
vm migrate
```

### From .env to .vorzela

**Before:**
```bash
# .env
DATABASE_URL=postgres://localhost/myapp
VORZELA_ENV=dev

vm migrate  # Loads from .env
```

**After:**
```bash
# .vorzela (checked into git, project-specific)
DATABASE_URL=postgres://localhost/myapp
VORZELA_ENV=dev

vm migrate  # Loads from .vorzela
```

## Troubleshooting Configuration

### "database URL is required"

**Problem:** No DATABASE_URL found

**Solution:** Set via one of these methods:
```bash
# 1. CLI flag
vm migrate --dsn "postgres://localhost/myapp"

# 2. Environment variable
export DATABASE_URL="postgres://localhost/myapp"
vm migrate

# 3. .vorzela file
echo 'DATABASE_URL=postgres://localhost/myapp' > .vorzela
vm migrate
```

### "Can't find .vorzela file"

**Note:** This is not an error. The tool checks:
1. Current directory
2. Parent directories (walks up the tree)
3. Falls back to environment variables

If you want to see where it's loading config from, check the current working directory.

### "Configuration conflict"

**Problem:** Multiple configuration sources with different values

**Solution:** Remember the priority:
1. CLI flags (highest) - wins
2. Environment variables
3. .vorzela file
4. .env file
5. Defaults (lowest)

Use the override that matches your use case.

## Configuration Files Reference

### .vorzela
**Purpose:** Project-specific configuration  
**Check in git:** Yes  
**Sensitive data:** No  
**Example:**
```ini
DATABASE_URL=postgres://localhost/myapp
VORZELA_ENV=dev
MIGRATION_PATH=./migrations
```

### .vorzela.example
**Purpose:** Template for .vorzela  
**Check in git:** Yes  
**Purpose:** Help team members set up .vorzela
```bash
cp .vorzela.example .vorzela
# Edit with your local values
```

### .env
**Purpose:** Local machine overrides  
**Check in git:** No (add to .gitignore)  
**Sensitive data:** Yes (passwords, keys)
```env
DATABASE_URL=postgres://user:password@localhost/myapp
```

### Environment Variables
**Purpose:** CI/CD, Docker, production  
**Check in git:** Only in CI config (as secrets)  
**Sensitive data:** Yes (via secrets management)

## Summary

| Method | Priority | Use Case | Example |
|--------|----------|----------|---------|
| CLI Flags | Highest | One-off commands | `vm migrate --dsn $URL` |
| Env Vars | High | CI/CD, Docker | `export DATABASE_URL=...` |
| .vorzela | Medium | Local development | `DATABASE_URL=postgres://...` |
| .env | Low | Local secrets | Added to .gitignore |
| Defaults | Lowest | Fallback values | `VORZELA_ENV=dev` |

Choose the method that best fits your workflow!
