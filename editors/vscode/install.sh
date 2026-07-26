#!/usr/bin/env bash
# Install Vorzela .vm highlighting into Cursor and/or VS Code (symlink from this repo).
# Does NOT publish to the marketplace — one-time per machine.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
EXT_ID="vorzela.vorzela-vm"

install_into() {
  local dir="$1"
  local label="$2"
  mkdir -p "$dir"
  # Remove older versioned symlinks for this extension
  find "$dir" -maxdepth 1 -name "${EXT_ID}-*" -exec rm -f {} + 2>/dev/null || true
  local target="${dir}/${EXT_ID}-1.1.1"
  ln -sfn "$ROOT" "$target"
  echo "✓ Linked $label → $target"
}

INSTALLED=0
if [[ -d "$HOME/.cursor" ]] || command -v cursor >/dev/null 2>&1; then
  install_into "$HOME/.cursor/extensions" "Cursor"
  INSTALLED=1
fi
if [[ -d "$HOME/.vscode" ]] || command -v code >/dev/null 2>&1; then
  install_into "$HOME/.vscode/extensions" "VS Code"
  INSTALLED=1
fi

if [[ "$INSTALLED" -eq 0 ]]; then
  echo "No Cursor (~/.cursor) or VS Code (~/.vscode) found."
  echo "Create one of those dirs, or pass a path:"
  echo "  ln -sfn \"$ROOT\" \"\$HOME/.cursor/extensions/${EXT_ID}-1.1.1\""
  exit 1
fi

echo
echo "Reload the editor window: Developer: Reload Window"
echo "Then open a .vm file — language mode should be \"Vorzela VM\"."
echo "Ctrl+Space lists keys; hover shows docs."
