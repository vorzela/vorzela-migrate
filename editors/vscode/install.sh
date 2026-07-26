#!/usr/bin/env bash
# Thin wrapper — Cursor / VS Code family only.
# Prefer:  ./editors/install.sh   (auto-detects all editors)
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
exec "$ROOT/install.sh" --vscode "$@"
