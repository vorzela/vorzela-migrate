# Installation Guide - Vorzela Migration Tool

## ⚡ Fastest Installation (All Platforms)

### Automatic Installation with curl/powershell

**macOS/Linux:**
```bash
curl -fsSL https://raw.githubusercontent.com/vorzela/vorzela-migrate/main/install.sh | bash
```

**Windows (PowerShell):**
```powershell
iex (New-Object Net.WebClient).DownloadString('https://raw.githubusercontent.com/vorzela/vorzela-migrate/main/install.ps1')
```

That's it! The script will:
- Detect your OS and architecture
- Download the latest version
- Add to PATH automatically
- Verify the installation

Then run:
```bash
vm --version
vm --help
```

---

## Quick Start (All Platforms)

### 1. **Download Binary** (Manual)
Visit the releases page and download the pre-built binary for your OS:
- **Windows**: `vc.exe`
- **macOS**: `vc` (Intel or Apple Silicon)
- **Linux**: `vc`

### 2. **Make it Executable & Add to PATH**

#### Windows
```bash
# 1. Download vc.exe
# 2. Move to a folder in your PATH, e.g., C:\Program Files\vorzela\
mkdir "C:\Program Files\vorzela"
move vc.exe "C:\Program Files\vorzela\"

# 3. Add to PATH via System Environment Variables
# Or use PowerShell:
---

## Database Configuration

Vorzela supports PostgreSQL, MySQL, and MariaDB. The database type is **auto-detected** from your DSN.

### PostgreSQL (Default)

**Connection String Format:**
```
postgres://user:password@host:port/database
```

**Example `.vorzela` config:**
```ini
DATABASE_URL=postgres://user:mypass@localhost:5432/myapp
VORZELA_ENV=dev
```

### MySQL / MariaDB

**Connection String Format (URL style):**
```
mysql://user:password@localhost:3306/database
```

**Connection String Format (DSN style):**
```
user:password@tcp(localhost:3306)/database
```

**Example `.vorzela` config:**
```ini
DATABASE_URL=mysql://user:mypass@localhost:3306/myapp
VORZELA_ENV=dev
```

### Connection String Equivalents

| DB | Format |
|---|---|
| PostgreSQL | `postgres://user:pass@localhost:5432/db` |
| MySQL | `mysql://user:pass@localhost:3306/db` or `user:pass@tcp(localhost:3306)/db` |
| MariaDB | `mysql://user:pass@localhost:3306/db` |

**Vorzela auto-detects MySQL/MariaDB by looking for:**
- `mysql://` prefix in URL
- `@tcp(` or `tcp(` in DSN string

If neither pattern matches, it assumes PostgreSQL.

---

## Installation Methods

### Method 1: Pre-built Binaries (RECOMMENDED)

**Easiest for non-developers. Works on any OS without Go installed.**

```bash
# Download from releases
# Extract the binary for your OS
# Move to system PATH (see Quick Start above)
# Done! Use: vm --help
```

**Advantages:**
- ✅ No installation needed
- ✅ Single executable file
- ✅ Works immediately
- ✅ No dependencies

**Disadvantages:**
- ❌ Need to download updates manually

---

### Method 2: Build from Source

**For developers who want the latest code.**

#### Prerequisites
- **Go 1.18+** - [Download Go](https://golang.org/dl/)
- **Git** - [Download Git](https://git-scm.com/)
- **PostgreSQL** - For running migrations (psql optional)

#### Installation Steps

**Windows (PowerShell)**
```powershell
# 1. Clone repository
git clone https://github.com/vorzela/vorzela-migrate.git
cd vorzela-migrate

# 2. Build binary
go build -o vc.exe main.go

# 3. Add to PATH
# Move to C:\Program Files\vorzela\ or add current folder to PATH
$env:Path += ";$PWD"

# 4. Verify
vm --version
```

**macOS/Linux (Bash)**
```bash
# 1. Clone repository
git clone https://github.com/vorzela/vorzela-migrate.git
cd vorzela-migrate

# 2. Build binary
go build -o vm main.go

# 3. Make executable & add to PATH
chmod +x vc
sudo mv vm /usr/local/bin/

# 4. Verify
vm --version
```

---

### Method 3: Install via Package Manager

**Coming soon for popular package managers:**

```bash
# Homebrew (macOS)
brew install vorzela/tap/vc

# Windows (Chocolatey)
choco install vc

# Linux (Snap)
snap install vorzela-migrate

# Arch Linux (AUR)
yay -S vorzela-migrate
```

---

## Setup Your Project

### 1. **Create Configuration File**

Create `.vorzela` in your project root:

```bash
# Windows (PowerShell)
@"
DATABASE_URL=postgres://localhost:5432/myapp
VORZELA_ENV=dev
MIGRATION_PATH=./migrations
"@ | Out-File -Encoding utf8 .vorzela

# macOS/Linux (Bash)
cat > .vorzela << EOF
DATABASE_URL=postgres://localhost:5432/myapp
VORZELA_ENV=dev
MIGRATION_PATH=./migrations
EOF
```

Or manually create `.vorzela` file with:
```ini
DATABASE_URL=postgres://localhost:5432/myapp
VORZELA_ENV=dev
MIGRATION_PATH=./migrations
```

### 2. **Create Migrations Directory**

```bash
# Windows
mkdir migrations\dev
mkdir migrations\server

# macOS/Linux
mkdir -p migrations/{dev,server}
```

### 3. **Create First Migration**

```bash
vm make migration create_users_table
```

### 4. **Edit Migration File**

Open the generated file in `migrations/dev/` and add your SQL

### 5. **Run Migration**

```bash
vm migrate
```

---

## Verify Installation

### Check Version
```bash
vm --version
# Output: vm version 1.0.0
```

### Check Help
```bash
vm --help
# Shows all available commands
```

### Test Database Connection
```bash
vm status
# If configured correctly, shows migration status
```

---

## Troubleshooting

### "vc: command not found" (macOS/Linux)

**Problem:** Binary not in PATH

**Solution:**
```bash
# Check if binary exists
which vc

# If not found, move to PATH
sudo mv  vm /usr/local/bin/

# Verify
vm --version
```

### "vm is not recognized" (Windows)

**Problem:** Binary not in PATH

**Solution:**
```powershell
# 1. Check current PATH
$env:Path -split ";"

# 2. Add folder to PATH (permanent)
[Environment]::SetEnvironmentVariable(
  "Path",
  "$env:Path;C:\Program Files\vorzela",
  [EnvironmentVariableTarget]::User
)

# 3. Restart PowerShell and test
vm --version
```

### "failed to connect to database"

**Problem:** Database connection not working

**Solution:**
```bash
# 1. Check .vorzela file exists
cat .vorzela
# or on Windows: type .vorzela

# 2. Check DATABASE_URL format
# Should be: postgres://user:password@host:port/database

# 3. Test database connection
psql postgres://user:password@host:port/database

# 4. Update DATABASE_URL if needed
# Or use environment variable:
export DATABASE_URL="postgres://user:password@host:port/database"
vm status
```

### "migration table not found"

**Problem:** First time running migrations

**Solution:**
```bash
# This is normal! The tool creates the table automatically
vm migrate
# The migration table will be created

# Verify
vm status
```

---

## Platform-Specific Notes

### Windows

**Requirements:**
- Windows 7 or later
- No external dependencies

**Tips:**
- Use PowerShell for better compatibility
- Add to `C:\Program Files\vorzela\` for system-wide access
- Environment variables work via Control Panel > System > Environment Variables

**Example Setup:**
```powershell
# Download vc.exe
# Create folder
mkdir "C:\Program Files\vorzela"
move vc.exe "C:\Program Files\vorzela\"

# Add to PATH permanently
[Environment]::SetEnvironmentVariable(
  "Path",
  "$env:Path;C:\Program Files\vorzela",
  [EnvironmentVariableTarget]::Machine
)

# Restart computer or PowerShell
vm --version
```

### macOS

**Requirements:**
- macOS 10.12 or later
- No external dependencies for binary version

**Tips:**
- Use `/usr/local/bin/` for user-installed tools
- Grant execute permission with `chmod +x`
- Works on both Intel and Apple Silicon Macs

**Example Setup:**
```bash
# Download vc
chmod +x vc
sudo mv vm /usr/local/bin/
vm --version
```

### Linux

**Requirements:**
- Linux kernel 3.10 or later
- No external dependencies for binary version

**Tips:**
- Use `/usr/local/bin/` for system-wide access
- Make executable with `chmod +x`
- Works on all major distributions

**Example Setup:**
```bash
# Download vc
chmod +x vc
sudo mv vm /usr/local/bin/
vm --version
```

---

## Docker Installation

**Use Vorzela in Docker containers:**

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o vm main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /apvc .
ENV DATABASE_URL=postgres://db:5432/myapp
ENV VORZELA_ENV=server

ENTRYPOINT ["./vc"]
CMD ["status"]
```

**Run migrations in container:**
```bash
docker build -t vorzela .
docker run vorzela migrate
docker run vorzela status
```

---

## Environment Variables

### All Supported Variables

```bash
# Database connection
DATABASE_URL=postgres://user:password@host:port/database

# Environment type (dev or server)
VORZELA_ENV=dev

# Path to migrations directory
MIGRATION_PATH=./migrations
```

### Set Permanently

**Windows (PowerShell):**
```powershell
[Environment]::SetEnvironmentVariable(
  "DATABASE_URL",
  "postgres://localhost/myapp",
  [EnvironmentVariableTarget]::User
)
```

**macOS/Linux (Bash):**
```bash
echo 'export DATABASE_URL=postgres://localhost/myapp' >> ~/.bashrc
source ~/.bashrc
```

---

## Next Steps

1. **Create first migration**
   ```bash
   vm make migration create_users_table
   ```

2. **Read naming conventions**
   ```bash
   cat NAMING_CONVENTIONS.md
   ```

3. **Run migrations**
   ```bash
   vm migrate
   ```

4. **Check status**
   ```bash
   vm status
   ```

---

## Getting Help

- **Quick Reference:** Read [QUICK_REFERENCE.md](QUICK_REFERENCE.md)
- **Configuration:** See [CONFIG_ENHANCED.md](CONFIG_ENHANCED.md)
- **Issues:** Check [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
- **Naming:** Follow [NAMING_CONVENTIONS.md](NAMING_CONVENTIONS.md)

---

## Supported Platforms

| OS | Arch | Status | Binary |
|----|----|--------|--------|
| Windows | x86-64 | ✅ | `vc.exe` |
| macOS | Intel (x86-64) | ✅ | `vc-macos-intel` |
| macOS | Apple Silicon (ARM64) | ✅ | `vc-macos-arm` |
| Linux | x86-64 | ✅ | `vc-linux` |
| Linux | ARM64 | ✅ | `vc-linux-arm` |

All binaries are fully featured with zero dependencies!
