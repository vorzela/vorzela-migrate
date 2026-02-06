#!/bin/bash

# Setup script for Vorzela Migration Tool

set -e

echo "🚀 Setting up Vorzela Migration Tool..."
echo ""

# Check Go installation
if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed. Please install Go 1.21 or later."
    exit 1
fi

GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
echo "✓ Go version: $GO_VERSION"

# Build the tool
echo ""
echo "Building vm binary..."
go mod tidy
go build -o vm main.go
echo "✓ Build successful"

# Create migrations directory
echo ""
echo "Creating migrations directory..."
mkdir -p migrations
echo "✓ Directory created"

# Set permissions
chmod +x vc
echo "✓ Permissions set"

echo ""
echo "✅ Setup complete!"
echo ""
echo "Next steps:"
echo "1. Set your DATABASE_URL environment variable:"
echo "   export DATABASE_URL='postgres://user:password@localhost:5432/database'"
echo ""
echo "2. Create a migration:"
echo "   ./vm make migration create_users_table"
echo ""
echo "3. Edit the migration file in migrations/"
echo ""
echo "4. Run migrations:"
echo "   ./vm migrate --dsn \$DATABASE_URL"
echo ""
echo "5. Check status:"
echo "   ./vm status --dsn \$DATABASE_URL"
echo ""
echo "For global usage, run: make install"
