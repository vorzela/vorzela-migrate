# VM Binary Name and Upgrade Feature

## Binary Name Change

The Vorzela Migrate binary has been renamed from `vc` to `vm` to better match the project name.

### Old vs New
```bash
# Old
./vc --version
./vc make migration create_users_table

# New
./vm --version
./vm make migration create_users_table
```

## Automatic Upgrade Checking

Vorzela automatically checks for new versions after running commands. When a new version is available, you'll see:

```
⚠️  A new version is available: vm 1.1.0 (current: 1.0.0)
   Run 'vm upgrade' to update
```

## Manual Upgrade

### Automatic Upgrade Command

The simplest way to upgrade:
```bash
vm upgrade
```

This command will:
1. Check for the latest version on GitHub
2. Attempt to upgrade using the appropriate method for your OS:
   - **macOS**: Try Homebrew if installed
   - **Linux**: Try apt-get if installed
   - **Fallback**: Download and run the installation script for your OS
3. Verify the installation

### Manual Installation Methods

#### macOS with Homebrew
```bash
brew install --upgrade vorzela/vorzela/vorzela-migrate
vm --version
```

#### Linux with apt
```bash
sudo apt-get update
sudo apt-get install -y vorzela-migrate
vm --version
```

#### Windows with PowerShell
```powershell
powershell -ExecutionPolicy Bypass -Command "Invoke-WebRequest -Uri 'https://github.com/vorzela/vorzela-migrate/raw/main/install.ps1' -OutFile './install.ps1'; ./install.ps1"
vm --version
```

#### Linux/macOS with curl
```bash
curl -sSL https://github.com/vorzela/vorzela-migrate/raw/main/install.sh | bash
vm --version
```

#### Build from Source
```bash
git clone https://github.com/vorzela/vorzela-migrate.git
cd vorzela-migrate
./build.sh
sudo cp vm /usr/local/bin/
vm --version
```

## Building Locally

To build the vm binary locally:

```bash
# Clone the repository
git clone https://github.com/vorzela/vorzela-migrate.git
cd vorzela-migrate

# Build using the build script
./build.sh

# Or build manually
go build -o vm main.go

# Verify
./vm --version
```

## Build Script (build.sh)

The new `build.sh` script:
- Compiles the Go source code
- Creates the `vm` binary
- Sets proper permissions
- Shows usage instructions

```bash
./build.sh
# Output:
# 🔨 Building Vorzela Migrate...
# ✅ Build successful!
# Binary created: vm
```

## Verifying the Installation

After upgrade or installation:

```bash
# Check version
vm --version
# Output: vm version 1.0.0

# Check for updates
vm upgrade
# Output: You're already on the latest version (1.0.0)

# Show all commands
vm --help
```

## Troubleshooting Upgrades

### Upgrade fails with "permission denied"

If you installed globally with `sudo`, you may need elevated privileges to upgrade:

```bash
sudo vm upgrade
```

Or upgrade using apt/Homebrew:
```bash
brew upgrade vorzela-migrate
# or
sudo apt-get upgrade vorzela-migrate
```

### Upgrade reports "Could not fetch latest version"

This can happen if:
- No internet connection
- GitHub API is temporarily unavailable
- Your network blocks access to api.github.com

Try again later or upgrade manually from the [GitHub releases page](https://github.com/vorzela/vorzela-migrate/releases).

### Binary is still showing as "vc"

If you upgraded from a version that used `vc`:

1. Remove the old binary
   ```bash
   rm /usr/local/bin/vc
   ```

2. Reinstall using the new binary name
   ```bash
   vm upgrade
   ```

Or manually copy the new binary:
```bash
sudo cp vm /usr/local/bin/vm
chmod +x /usr/local/bin/vm
```

## Commands Available

All Vorzela commands work with the `vm` binary:

```bash
vm make migration create_users_table     # Create migration
vm --soft-delete migration create_posts_table  # Create with soft delete
vm migrate                               # Run migrations
vm status                                # Show status
vm rollback                              # Rollback last batch
vm rollback --steps 3                    # Rollback 3 batches
vm fresh                                 # Rollback all and re-run
vm refresh                               # Rollback all and re-run
vm upgrade                               # Upgrade to latest version
```

## What's New

✨ **New in this version:**
- Binary renamed from `vc` to `vm` (matches project name)
- Automatic version update checking
- New `vm upgrade` command
- Version notice shown after commands
- Build script for easy local compilation

## Version History

- **1.0.0** - Initial release with vm binary and upgrade feature
- Earlier versions used `vc` binary name

## Next Steps

1. **Upgrade now**: `vm upgrade`
2. **Check version**: `vm --version`
3. **Read docs**: `vm --help`
4. **Report issues**: GitHub Issues

---

For more information, visit [GitHub Repository](https://github.com/vorzela/vorzela-migrate)
