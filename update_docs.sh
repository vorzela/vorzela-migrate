#!/bin/bash

# Update all documentation to use vm instead of vm

files=(
    "README.md"
    "INSTALL.md"
    "SOFT_DELETE_QUICK_START.md"
    "SOFT_DELETE_AND_ERROR_HANDLING.md"
    "MIGRATION_TEMPLATE_FIX.md"
    "setup.sh"
    "build.sh"
)

echo "Updating documentation from 'vm' to 'vm'..."

for file in "${files[@]}"; do
    if [ -f "$file" ]; then
        # Replace vm command references
        sed -i 's/\bvc /vm /g' "$file"
        sed -i 's/\`vm\`/`vm`/g' "$file"
        sed -i 's/"vm"/"vm"/g' "$file"
        sed -i "s/'vm'/'vm'/g" "$file"
        sed -i 's/^vm /vm /g' "$file"
        sed -i 's/: vm /: vm /g' "$file"
        echo "✓ Updated $file"
    fi
done

echo ""
echo "✅ Documentation updated successfully!"
