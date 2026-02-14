#!/bin/bash

# Build script for Vorzela Migrate
# Compiles the binary and names it appropriately

set -e

echo "🔨 Building Vorzela Migrate..."

# Detect version from git tags, fallback to "dev"
VERSION=$(git describe --tags --abbrev=0 2>/dev/null || echo "dev")
echo "Version: $VERSION"

# Build the binary with version
go build -ldflags "-X 'github.com/vorzela/vorzela-migrate/internal/version.CurrentVersion=$VERSION'" -o vm main.go

# Verify the build
if [ ! -f ./vm ]; then
    echo "❌ Build failed!"
    exit 1
fi

# Make it executable
chmod +x vm

echo "✅ Build successful!"
echo ""
echo "Binary created: vm"
echo ""
echo "Usage:"
echo "  ./vm --help              # Show help"
echo "  ./vm --version           # Show version"
echo "  ./vm make migration <name>  # Create migration"
echo "  ./vm migrate             # Run migrations"
echo "  ./vm status              # Show status"
echo "  ./vm rollback            # Rollback migrations"
echo "  ./vm upgrade             # Check and install updates"
echo ""
echo "For global installation, run:"
echo "  sudo cp vm /usr/local/bin/"
echo "  chmod +x /usr/local/bin/vm"
echo ""
