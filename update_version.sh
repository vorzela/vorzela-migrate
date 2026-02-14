#!/bin/bash
# Automatically update version in README.md based on latest git tag

VERSION=$(git describe --tags --abbrev=0 2>/dev/null || echo "dev")

if [ "$VERSION" = "dev" ]; then
    echo "No git tags found. Please create a tag first."
    exit 1
fi

echo "Updating README.md to version $VERSION..."

# Update version in README.md title
sed -i "s/# Vorzela Migration Tool (v[0-9]\+\.[0-9]\+\.[0-9]\+)/# Vorzela Migration Tool ($VERSION)/" README.md

echo "✓ Updated README.md to $VERSION"
