#!/usr/bin/env bash
# Auto-install Vorzela .vm syntax support into editors detected on this machine.
#
# Usage (from repo root or editors/):
#   ./editors/install.sh              # detect & install all found editors
#   ./editors/install.sh --list       # only print what would be installed
#   ./editors/install.sh --vscode     # Cursor + VS Code family only
#   ./editors/install.sh --help
#
# Does not publish to marketplaces. Safe to re-run (idempotent symlinks).
set -euo pipefail

EDITORS_ROOT="$(cd "$(dirname "$0")" && pwd)"
VSCODE_EXT="$EDITORS_ROOT/vscode"
EXT_ID="vorzela.vorzela-vm"
EXT_VER="1.1.1"
LIST_ONLY=0
FILTER=""

usage() {
  cat <<EOF
Vorzela .vm editor installer

Usage: $0 [options]

  (default)     Detect available editors and install .vm highlighting
  --list        Detect only — print what would be installed
  --vscode      Only Cursor / VS Code / VSCodium / Code - Insiders
  --help        Show this help

Detected targets may include: Cursor, VS Code, Sublime Text, Vim, Neovim,
Nano, Helix, Zed. JetBrains IDEs are detected and print manual steps.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --list) LIST_ONLY=1 ;;
    --vscode) FILTER="vscode" ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1"; usage; exit 1 ;;
  esac
  shift
done

log()  { printf '%s\n' "$*"; }
ok()   { printf '✓ %s\n' "$*"; }
skip() { printf '· %s\n' "$*"; }
warn() { printf '! %s\n' "$*"; }

want_vscode() { [[ -z "$FILTER" || "$FILTER" == "vscode" ]]; }
skip_if_vscode_only() { [[ "$FILTER" == "vscode" ]]; }

link_file() {
  local src="$1" dest="$2"
  mkdir -p "$(dirname "$dest")"
  ln -sfn "$src" "$dest"
}

# ── Cursor / VS Code family ──────────────────────────────────────────────────
install_vscode_family() {
  local dir="$1" label="$2"
  if [[ "$LIST_ONLY" -eq 1 ]]; then
    ok "Would install → $label ($dir)"
    return
  fi
  mkdir -p "$dir"
  find "$dir" -maxdepth 1 -name "${EXT_ID}-*" -exec rm -f {} + 2>/dev/null || true
  local target="${dir}/${EXT_ID}-${EXT_VER}"
  ln -sfn "$VSCODE_EXT" "$target"
  ok "Linked $label → $target"
}

detect_vscode_family() {
  want_vscode || return 0
  local found=0

  if [[ -d "$HOME/.cursor" ]] || command -v cursor >/dev/null 2>&1; then
    install_vscode_family "$HOME/.cursor/extensions" "Cursor"
    found=1
  fi
  if [[ -d "$HOME/.vscode" ]] || command -v code >/dev/null 2>&1; then
    install_vscode_family "$HOME/.vscode/extensions" "VS Code"
    found=1
  fi
  if [[ -d "$HOME/.vscode-insiders" ]] || command -v code-insiders >/dev/null 2>&1; then
    install_vscode_family "$HOME/.vscode-insiders/extensions" "VS Code Insiders"
    found=1
  fi
  if [[ -d "$HOME/.vscodium" ]] || [[ -d "$HOME/.vscode-oss" ]] || command -v codium >/dev/null 2>&1; then
    local codium_dir="$HOME/.vscode-oss/extensions"
    [[ -d "$HOME/.vscodium/extensions" ]] && codium_dir="$HOME/.vscodium/extensions"
    install_vscode_family "$codium_dir" "VSCodium"
    found=1
  fi

  if [[ "$found" -eq 0 && -z "$FILTER" ]]; then
    skip "Cursor / VS Code not detected"
  elif [[ "$found" -eq 0 && "$FILTER" == "vscode" ]]; then
    warn "No Cursor / VS Code / VSCodium install found"
    return 1
  fi
}

# ── Sublime Text ─────────────────────────────────────────────────────────────
detect_sublime() {
  skip_if_vscode_only && return 0
  local dest=""
  if [[ -d "$HOME/.config/sublime-text/Packages" ]]; then
    dest="$HOME/.config/sublime-text/Packages/Vorzela"
  elif [[ -d "$HOME/Library/Application Support/Sublime Text/Packages" ]]; then
    dest="$HOME/Library/Application Support/Sublime Text/Packages/Vorzela"
  elif command -v subl >/dev/null 2>&1; then
    mkdir -p "$HOME/.config/sublime-text/Packages"
    dest="$HOME/.config/sublime-text/Packages/Vorzela"
  else
    skip "Sublime Text not detected"
    return 0
  fi

  if [[ "$LIST_ONLY" -eq 1 ]]; then
    ok "Would install → Sublime Text ($dest)"
    return
  fi
  mkdir -p "$dest"
  link_file "$EDITORS_ROOT/sublime/Vorzela VM.sublime-syntax" "$dest/Vorzela VM.sublime-syntax"
  ok "Linked Sublime Text → $dest"
}

# ── Vim / Neovim ─────────────────────────────────────────────────────────────
install_vim_runtime() {
  local base="$1" label="$2"
  if [[ "$LIST_ONLY" -eq 1 ]]; then
    ok "Would install → $label ($base)"
    return
  fi
  mkdir -p "$base/ftdetect" "$base/syntax"
  link_file "$EDITORS_ROOT/vim/ftdetect/vorzela.vim" "$base/ftdetect/vorzela.vim"
  link_file "$EDITORS_ROOT/vim/syntax/vorzela.vim" "$base/syntax/vorzela.vim"
  ok "Linked $label → $base"
}

detect_vim() {
  skip_if_vscode_only && return 0
  local found=0
  if [[ -d "$HOME/.vim" ]] || command -v vim >/dev/null 2>&1; then
    install_vim_runtime "$HOME/.vim" "Vim"
    found=1
  fi
  if [[ -d "$HOME/.config/nvim" ]] || command -v nvim >/dev/null 2>&1; then
    install_vim_runtime "$HOME/.config/nvim" "Neovim"
    found=1
  fi
  [[ "$found" -eq 0 ]] && skip "Vim / Neovim not detected"
  return 0
}

# ── Nano ─────────────────────────────────────────────────────────────────────
detect_nano() {
  skip_if_vscode_only && return 0
  if ! command -v nano >/dev/null 2>&1; then
    skip "Nano not detected"
    return 0
  fi
  local nanorc_dir="$HOME/.nano"
  local include_line="include \"$nanorc_dir/vorzela.nanorc\""
  if [[ "$LIST_ONLY" -eq 1 ]]; then
    ok "Would install → Nano (~/.nano + ~/.nanorc)"
    return
  fi
  mkdir -p "$nanorc_dir"
  link_file "$EDITORS_ROOT/nano/vorzela.nanorc" "$nanorc_dir/vorzela.nanorc"
  touch "$HOME/.nanorc"
  if ! grep -Fq 'vorzela.nanorc' "$HOME/.nanorc" 2>/dev/null; then
    printf '\n# Vorzela .vm syntax\n%s\n' "$include_line" >> "$HOME/.nanorc"
    ok "Linked Nano + appended include to ~/.nanorc"
  else
    ok "Linked Nano (~/.nanorc already includes vorzela.nanorc)"
  fi
}

# ── Helix ────────────────────────────────────────────────────────────────────
detect_helix() {
  skip_if_vscode_only && return 0
  if [[ ! -d "$HOME/.config/helix" ]] && ! command -v hx >/dev/null 2>&1; then
    skip "Helix not detected"
    return 0
  fi
  local dest="$HOME/.config/helix"
  if [[ "$LIST_ONLY" -eq 1 ]]; then
    ok "Would merge Helix languages.toml → $dest"
    return
  fi
  mkdir -p "$dest"
  local marker="# --- vorzela-vm (managed by editors/install.sh) ---"
  touch "$dest/languages.toml"
  if grep -Fq 'name = "vorzela-vm"' "$dest/languages.toml" 2>/dev/null; then
    ok "Helix already has vorzela-vm language entry"
  else
    {
      echo
      echo "$marker"
      grep -v '^#' "$EDITORS_ROOT/helix/languages.toml" | grep -v '^$' || true
      echo "# --- end vorzela-vm ---"
    } >> "$dest/languages.toml"
    ok "Appended vorzela-vm to $dest/languages.toml"
    warn "Run: hx --grammar fetch && hx --grammar build  (uses ini grammar)"
  fi
}

# ── Zed ──────────────────────────────────────────────────────────────────────
detect_zed() {
  skip_if_vscode_only && return 0
  if [[ ! -d "$HOME/.config/zed" ]] && ! command -v zed >/dev/null 2>&1; then
    skip "Zed not detected"
    return 0
  fi
  local settings="$HOME/.config/zed/settings.json"
  if [[ "$LIST_ONLY" -eq 1 ]]; then
    ok "Would note Zed Ini mapping (see editors/zed/README.md)"
    return
  fi
  mkdir -p "$HOME/.config/zed"
  if [[ -f "$settings" ]] && grep -Fq '"Ini"' "$settings" 2>/dev/null && grep -Fq '.vm' "$settings" 2>/dev/null; then
    ok "Zed settings already map .vm (Ini)"
  else
    warn "Zed detected — map .vm to Ini in $settings (see editors/zed/README.md)"
    warn '  Example: "file_types": { "Ini": ["vm", ".vm"] }'
  fi
}

# ── JetBrains ────────────────────────────────────────────────────────────────
detect_jetbrains() {
  skip_if_vscode_only && return 0
  local jb=0
  if [[ -d "$HOME/.config/JetBrains" ]] || [[ -d "$HOME/Library/Application Support/JetBrains" ]]; then
    jb=1
  fi
  for b in idea goland phpstorm webstorm pycharm rider clion; do
    if command -v "$b" >/dev/null 2>&1; then jb=1; break; fi
  done
  if [[ "$jb" -eq 0 ]]; then
    skip "JetBrains IDEs not detected"
    return 0
  fi
  if [[ "$LIST_ONLY" -eq 1 ]]; then
    ok "Would print JetBrains TextMate steps (editors/textmate)"
    return
  fi
  warn "JetBrains detected — add TextMate bundle manually:"
  warn "  Settings → Editor → TextMate Bundles → + → $EDITORS_ROOT/textmate"
  warn "  Or map *.vm / .vm under File Types → Ini / Properties"
}

# ── Main ─────────────────────────────────────────────────────────────────────
log "Vorzela .vm editor installer"
log "Source: $EDITORS_ROOT"
log

detect_vscode_family
detect_sublime
detect_vim
detect_nano
detect_helix
detect_zed
detect_jetbrains

log
if [[ "$LIST_ONLY" -eq 1 ]]; then
  log "Dry run complete. Re-run without --list to install."
else
  log "Done. Reload open editors to pick up changes."
  log "Cursor/VS Code: Developer: Reload Window — then Ctrl+Space in .vm for keys."
fi
