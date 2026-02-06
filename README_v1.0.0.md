# Vorzela Migration Tool v1.0.0 - Complete Release

**Status**: Ready for Production ✅  
**Release Date**: February 5, 2026  
**Version**: 1.0.0

---

## 🎯 What is Vorzela?

**Vorzela Migration Tool** is a Laravel-inspired database migration tool for Go developers using PostgreSQL.

Think of it as **Laravel migrations, but for Go** - with simple commands like:

```bash
vm make migration create_users_table
vm migrate
vm status
vm rollback
```

---

## 🚀 Key Features

### ✅ Laravel-Style Commands
```bash
vm make migration create_users_table   # Create migration
vm migrate                               # Run pending migrations
vm status                                # Show migration status
vm rollback                              # Rollback migrations
vm fresh                                 # Reset & re-run (with warnings)
vm refresh                               # Reset & re-run (no warnings)
```

### ✅ Enhanced Templates
All generated migrations include:
- **Timestamps** - Track when migrations were created
- **CREATE TABLE template** - With id, created_at, updated_at columns
- **DROP TABLE template** - For safe rollback

### ✅ Flexible Rollback
```bash
vm rollback              # Last 1 batch (default)
vm rollback --steps=2    # Last 2 batches
vm rollback --steps=all  # All migrations
```

### ✅ Safe Fresh Command
```bash
vm fresh                 # Interactive with warnings
vm fresh --force         # For CI/CD (no prompts)
```

### ✅ Colored Output
- 🟢 **Green** - Success messages
- 🔴 **Red** - Errors
- 🟡 **Yellow** - Warnings
- 🔵 **Cyan** - Information

### ✅ Configuration System
No more `--dsn` every time! Use:
- `.vorzela` file (checked in)
- `.env` file (local machine)
- Environment variables (CI/CD)
- CLI flags (overrides)

### ✅ Strong Naming Conventions
Clear, descriptive migration names:
```bash
vm make migration create_users_table
vm make migration add_email_to_users
vm make migration add_phone_to_customers
vm make migration create_index_on_users_email
```

Status shows readable names:
```
Migration Name                    | Status
create users table                | ✓ Batch 1
add email to users                | ✓ Batch 1
create posts table                | ⏳ Pending
```

---

## 💻 Installation

### For Non-Developers (Easiest)
Download pre-built binary for your OS:
- **Windows**: `vm.exe`
- **macOS**: `vm` (Intel or Apple Silicon)
- **Linux**: `vm`

Then add to PATH. See [INSTALL.md](INSTALL.md) for detailed steps.

### For Developers
```bash
git clone https://github.com/vorzela/vorzela-migrate.git
cd vorzela-migrate
go build -o vm main.go
sudo mv vm /usr/local/bin/
```

---

## 📚 Documentation

| Document | Purpose |
|----------|---------|
| [INSTALL.md](INSTALL.md) | Setup for Windows, macOS, Linux, Docker |
| [QUICK_REFERENCE.md](QUICK_REFERENCE.md) | Quick start guide |
| [NAMING_CONVENTIONS.md](NAMING_CONVENTIONS.md) | Migration naming patterns |
| [CONFIG_ENHANCED.md](CONFIG_ENHANCED.md) | Configuration system |
| [COLORS_AND_CONFIG.md](COLORS_AND_CONFIG.md) | Colors & config features |
| [ARCHITECTURE.md](ARCHITECTURE.md) | System design |
| [TROUBLESHOOTING.md](TROUBLESHOOTING.md) | Common issues |

---

## 🎬 Quick Start

### 1. Install
```bash
# Download binary or build from source
# See INSTALL.md for detailed instructions
vm --version
```

### 2. Setup Project
```bash
# Create configuration
echo 'DATABASE_URL=postgres://localhost/myapp' > .vorzela
echo 'VORZELA_ENV=dev' >> .vorzela

# Create migrations directory
mkdir -p migrations/{dev,server}
```

### 3. Create First Migration
```bash
vm make migration create_users_table
```

### 4. Edit & Run
```bash
# Edit the generated SQL file
vim migrations/dev/TIMESTAMP_create_users_table.sql

# Run migration
vm migrate

# Check status
vm status
```

---

## 📖 Usage Examples

### Creating Migrations (Strong Naming!)

```bash
# Table operations
vm make migration create_users_table
vm make migration create_posts_table
vm make migration drop_legacy_users_table

# Column operations
vm make migration add_email_to_users
vm make migration add_phone_to_customers
vm make migration remove_deprecated_field_from_products

# Index/Constraint operations
vm make migration create_index_on_users_email
vm make migration add_foreign_key_author_id_to_posts
```

### Running Migrations

```bash
# Run all pending migrations
vm migrate

# Check what will run
vm status

# Rollback if needed
vm rollback               # Last batch
vm rollback --steps=2     # Last 2 batches
vm rollback --steps=all   # Everything
```

### Fresh Database

```bash
# Development (with confirmation)
vm fresh
# Output:
# ⚠️  CAUTION: This will rollback ALL migrations and re-run them
# ⚠️  This may cause data loss in your database!
# Environment: dev | Database: postgres://localhost/myapp
# 
# Do you want to continue? (yes/no): yes
# ✓ Rolled back 5 migrations
# ✓ Successfully ran 5 migrations
# ✨ Fresh completed successfully!

# CI/CD (automated)
vm fresh --force
```

---

## 🔧 Commands Reference

### make
```bash
vm make migration create_users_table
# Creates: migrations/dev/TIMESTAMP_create_users_table.sql
# With: CREATE TABLE template, DROP TABLE template
```

### migrate
```bash
vm migrate
# Runs all pending migrations
# Tracks in database migration table
```

### rollback
```bash
vm rollback              # Rollback 1 batch
vm rollback --steps=1    # Explicit: 1 batch
vm rollback --steps=2    # Rollback 2 batches
vm rollback --steps=all  # Rollback all
```

### fresh
```bash
vm fresh                 # Interactive (asks for confirmation)
vm fresh --force         # Skip confirmation (for automation)
```

### refresh
```bash
vm refresh               # Rollback all + re-run (no warnings)
```

### status
```bash
vm status                # Shows executed & pending migrations
# Output includes readable migration names!
```

---

## 📁 Project Structure

```
migrations/
├── dev/
│   ├── 1707123456_create_users_table.sql
│   ├── 1707123457_add_email_to_users.sql
│   └── 1707123458_create_posts_table.sql
└── server/
    ├── 1707123456_create_users_table.sql
    └── 1707123457_add_email_to_users.sql

.vorzela                    # Configuration (checked in)
.env                        # Local secrets (in .gitignore)
go.mod                      # Go module
main.go                     # CLI entry point
cmd/                        # Commands (make, migrate, etc.)
internal/                   # Internal packages
├── config/                 # Configuration system
├── database/               # Database connection
├── migration/              # Migration logic
└── output/                 # Colored output
```

---

## ⚙️ Configuration

### Priority System

1. **CLI Flags** (highest)
   ```bash
    vm migrate --dsn "postgres://override/db"
   ```

2. **Environment Variables**
   ```bash
   DATABASE_URL=postgres://localhost/myapp  vm migrate
   ```

3. **`.vorzela` Config File**
   ```ini
   DATABASE_URL=postgres://localhost/myapp
   VORZELA_ENV=dev
   ```

4. **`.env` File**
   ```env
   DATABASE_URL=postgres://localhost/myapp
   ```

5. **Defaults** (lowest)

### Example `.vorzela` File

```ini
# PostgreSQL connection (required)
DATABASE_URL=postgres://user:password@localhost:5432/database

# Environment: dev or server
VORZELA_ENV=dev

# Migrations path (optional, default:  migrations)
MIGRATION_PATH=./migrations
```

---

## 🎨 Naming Conventions

### Migration Naming Pattern

```
<action>_<details>_<target>
```

### Examples

**Table Operations:**
```
create_users_table
create_posts_table
drop_legacy_users_table
```

**Column Operations (Strong Naming!):**
```
add_email_to_users
add_phone_to_customers
remove_deprecated_field_from_products
rename_user_name_to_full_name
```

**Index/Constraint Operations:**
```
create_index_on_users_email
add_foreign_key_author_id_to_posts
add_unique_constraint_to_users_email
```

### Why Strong Naming?

✅ Self-documenting code  
✅ Easier to understand migrations at a glance  
✅ Better for team collaboration  
✅ Readable status output  

---

## 🐘 PostgreSQL Support

- **Minimum**: PostgreSQL 13
- **Recommended**: PostgreSQL 15+
- **Driver**: pgx v5 (official PostgreSQL driver)
- **Features**: Connection pooling (5 connections), transaction support

---

## 🖥️ Platform Support

| OS | Binary | Status |
|----|--------|--------|
| Windows | vm.exe | ✅ Full support |
| macOS (Intel) | vm-macos-intel | ✅ Full support |
| macOS (Apple Silicon) | vm-macos-arm | ✅ Full support |
| Linux (x86-64) | vm-linux | ✅ Full support |
| Linux (ARM64) | vm-linux-arm | ✅ Full support |
| Docker | vorzela:latest | ✅ Full support |

All platforms include **zero external dependencies** when using pre-built binaries!

---

## 🔒 Safety Features

✅ **Confirmation Prompts** - Fresh command asks before destructive operations  
✅ **Atomic Transactions** - All migrations wrapped in BEGIN/COMMIT  
✅ **Rollback Support** - Undo migrations easily  
✅ **Batch Tracking** - Know which migrations were run together  
✅ **Configuration Validation** - Validates DATABASE_URL before running  

---

## 🚀 Best Practices

1. **Use strong naming conventions**
   ```bash
    vm make migration add_email_to_users  # Good!
    vm make migration update_table        # Bad!
   ```

2. **Create `.vorzela` file**
   ```bash
   echo 'DATABASE_URL=postgres://localhost/myapp' > .vorzela
   ```

3. **Use `.vorzela.example` for team**
   ```bash
   cp .vorzela .vorzela.example
   git add .vorzela.example
   ```

4. **Check before running**
   ```bash
    vm status        # See what will run
    vm migrate       # Run migrations
    vm status        # Verify they ran
   ```

5. **Backup before destructive ops**
   ```bash
   pg_dump myapp > backup.sql
    vm fresh
   ```

---

## 🐛 Troubleshooting

**"vm: command not found"**
- Add to PATH (see INSTALL.md)

**"database URL is required"**
- Create `.vorzela` file with DATABASE_URL

**"failed to connect to database"**
- Check DATABASE_URL format: `postgres://user:pass@host:port/db`
- Verify database exists: `psql postgres://...`

**"migration table not found"**
- Normal on first run! Table is created automatically

See [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for more.

---

## 📦 What's Included

### 6 Commands
- `make` - Create migrations
- `migrate` - Run pending migrations
- `rollback` - Undo migrations (1, 2, or all)
- `fresh` - Reset with warnings
- `refresh` - Reset without warnings
- `status` - Show migration status

### Features
- ✅ Colored output (green, red, yellow, cyan)
- ✅ Configuration system (.vorzela, .env, env vars)
- ✅ Templates with timestamps
- ✅ Strong naming conventions
- ✅ Connection pooling
- ✅ Transaction support
- ✅ Cross-platform (Windows, macOS, Linux, Docker)

### Documentation
- ✅ 10+ markdown guides
- ✅ Installation instructions
- ✅ Naming conventions
- ✅ Configuration examples
- ✅ Troubleshooting guide
- ✅ Architecture documentation

---

## 🎓 Learning Path

1. **Get Started** → [INSTALL.md](INSTALL.md)
2. **Quick Commands** → [QUICK_REFERENCE.md](QUICK_REFERENCE.md)
3. **Naming Patterns** → [NAMING_CONVENTIONS.md](NAMING_CONVENTIONS.md)
4. **Configuration** → [CONFIG_ENHANCED.md](CONFIG_ENHANCED.md)
5. **Colors & Features** → [COLORS_AND_CONFIG.md](COLORS_AND_CONFIG.md)
6. **System Design** → [ARCHITECTURE.md](ARCHITECTURE.md)
7. **Issues** → [TROUBLESHOOTING.md](TROUBLESHOOTING.md)

---

## ✨ Why Choose Vorzela?

| Feature | Vorzela | Others |
|---------|---------|--------|
| Laravel-style syntax | ✅ | ❌ |
| Colored output | ✅ | ❌ |
| Config files | ✅ | ❌ |
| Strong naming guide | ✅ | ❌ |
| Windows support | ✅ | ✓ |
| No dependencies | ✅ | ❌ |
| Flexible rollback | ✅ | ✓ |
| Fresh with warnings | ✅ | ❌ |

---

## 📄 License

(Specify your license here)

---

## 🤝 Contributing

We welcome contributions! 

---

## 📞 Support

- **Documentation**: Read the .md files in this project
- **Issues**: Check TROUBLESHOOTING.md
- **Questions**: See CONFIG_ENHANCED.md or NAMING_CONVENTIONS.md

---

## 🎉 Summary

**Vorzela v1.0.0** is a production-ready, Laravel-inspired migration tool for Go developers.

- 🟢 **Easy to install** - Pre-built binaries for all platforms
- 🟢 **Easy to use** - Simple Laravel-style commands
- 🟢 **Easy to configure** - Config files instead of CLI flags
- 🟢 **Easy to name** - Strong naming conventions guide
- 🟢 **Easy to extend** - Clean, modular Go code

**Get started in 5 minutes!** See [INSTALL.md](INSTALL.md).

---

**Version**: 1.0.0  
**Status**: Production Ready ✅  
**Released**: February 5, 2026
