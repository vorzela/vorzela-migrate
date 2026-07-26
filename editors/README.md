# Vorzela `.vm` syntax highlighting

Editor support for Vorzela Migrate config files (`.vm` / `*.vm`).

## Does it install automatically?

**No on clone** — but one script detects editors on your machine and installs:

```bash
./editors/install.sh          # all detected editors
./editors/install.sh --list   # dry-run
./editors/install.sh --vscode # Cursor / VS Code / VSCodium only
```

| Editor | Auto via `install.sh`? | Notes |
|--------|------------------------|-------|
| **Cursor / VS Code / Insiders / VSCodium** | Yes | Symlinks `editors/vscode` extension |
| **Sublime Text** | Yes | Symlinks `.sublime-syntax` |
| **Vim / Neovim** | Yes | Symlinks `ftdetect` + `syntax` |
| **Nano** | Yes | Symlinks nanorc + appends `~/.nanorc` include |
| **Helix** | Yes | Appends `vorzela-vm` to `languages.toml` |
| **Zed** | Detected | Prints settings snippet (Ini map) |
| **JetBrains** | Detected | Prints TextMate bundle path to add |

`editors/vscode/install.sh` is a thin wrapper around `./editors/install.sh --vscode`.

Workspace [`.vscode/settings.json`](../.vscode/settings.json) maps `.vm` → `vorzela-vm` once the extension is linked.

## Features (VS Code / Cursor)

| Feature | Behaviour |
|---------|-----------|
| **Syntax colors** | Keys, booleans, enums, DSNs, comments |
| **Hover docs** | Hover any key for type, required/optional, allowed values, and usage |
| **Live lint** | Unknown keys → **error** (red); invalid bool/enum → **error**; duplicates → warning; missing `DATABASE_URL` → warning |
| **Completions** | **Ctrl+Space** (or start typing a key) lists all known keys — Required first; marks already-set. After `=` offers allowed values |

### Required vs optional keys

| Key | Required? | Notes |
|-----|-----------|--------|
| `DATABASE_URL` | **Yes** | Without it, `vm` cannot connect. Hover explains DSN formats. Empty value is an error; missing key is a warning. |
| Everything else | Optional | Sensible defaults from `ENVIRONMENT` / built-ins |

Unknown keys (typos like `VM_ENV`, `DATABSE_URL`) are highlighted as errors with a “did you mean …?” suggestion when possible — same rules as `vm lint`.

---

Highlights (all editors with grammar support):
- `#` comments
- Known keys (`DATABASE_URL`, `ENVIRONMENT`, `DRIFT_HANDLING`, `AUTO_RUN_*`, …)
- `=` assignment
- Booleans (`true` / `false` / `1` / `0`)
- Environment & drift enums
- `postgres://` / `mysql://` / `@tcp(...)` DSNs

## VS Code / Cursor (recommended)

Extension lives in [`vscode/`](vscode/). Includes highlighting **plus** hover + live diagnostics + Ctrl+Space keys.

### One-shot install (from repo root)

```bash
./editors/install.sh              # detect all editors
# or:
./editors/vscode/install.sh       # Cursor / VS Code family only
# then: Developer: Reload Window
```

### Manual symlink

```bash
# Cursor
mkdir -p ~/.cursor/extensions
ln -sfn "$(pwd)/editors/vscode" ~/.cursor/extensions/vorzela.vorzela-vm-1.1.1

# VS Code
mkdir -p ~/.vscode/extensions
ln -sfn "$(pwd)/editors/vscode" ~/.vscode/extensions/vorzela.vorzela-vm-1.1.1
```

Reload the window (`Developer: Reload Window`). Open `.vm` — language mode **Vorzela VM**.

- **Hover** a key for docs (required vs optional)
- **Ctrl+Space** on a blank line (or while typing) to pick a forgotten key
- **Problems** panel for unknown / invalid keys

### Install as VSIX

```bash
cd editors/vscode
npx --yes @vscode/vsce package --allow-missing-repository
# then: Extensions → … → Install from VSIX
```

### Workspace association

This repository’s [`.vscode/settings.json`](../.vscode/settings.json) maps `.vm` → `vorzela-vm`.

## Sublime Text 3 / 4

Copy or symlink:

```bash
# Linux
mkdir -p ~/.config/sublime-text/Packages/Vorzela
ln -sfn "$(pwd)/editors/sublime/Vorzela VM.sublime-syntax" \
  ~/.config/sublime-text/Packages/Vorzela/

# macOS
mkdir -p ~/Library/Application\ Support/Sublime\ Text/Packages/Vorzela
ln -sfn "$(pwd)/editors/sublime/Vorzela VM.sublime-syntax" \
  "$HOME/Library/Application Support/Sublime Text/Packages/Vorzela/"
```

## Vim / Neovim

```bash
# Vim
mkdir -p ~/.vim/{ftdetect,syntax}
ln -sfn "$(pwd)/editors/vim/ftdetect/vorzela.vim" ~/.vim/ftdetect/
ln -sfn "$(pwd)/editors/vim/syntax/vorzela.vim" ~/.vim/syntax/

# Neovim
mkdir -p ~/.config/nvim/{ftdetect,syntax}
ln -sfn "$(pwd)/editors/vim/ftdetect/vorzela.vim" ~/.config/nvim/ftdetect/
ln -sfn "$(pwd)/editors/vim/syntax/vorzela.vim" ~/.config/nvim/syntax/
```

## Nano

```bash
mkdir -p ~/.nano
ln -sfn "$(pwd)/editors/nano/vorzela.nanorc" ~/.nano/
# add to ~/.nanorc:
echo 'include "~/.nano/vorzela.nanorc"' >> ~/.nanorc
```

## Helix

Merge [`helix/languages.toml`](helix/languages.toml) into `~/.config/helix/languages.toml`, or map `.vm` to `ini`:

```toml
[[language]]
name = "ini"
file-types = ["ini", "conf", { glob = ".vm" }, "vm"]
```

## JetBrains (IntelliJ / GoLand / PhpStorm)

1. Install the **TextMate Bundles** plugin (bundled in many IDEs).
2. Settings → Editor → TextMate Bundles → `+` → select [`textmate/`](textmate/).
3. Or: Settings → Editor → File Types → add `*.vm` / `.vm` under **Ini** / **Properties** for basic coloring until the TextMate bundle is loaded.

## Zed

See [`zed/README.md`](zed/README.md). Quick fallback — treat `.vm` as Ini:

```json
{
  "file_types": {
    "Ini": ["vm", ".vm"]
  }
}
```

## Grammar source of truth

| Editor | Path |
|--------|------|
| TextMate / VS Code / Cursor | `vscode/syntaxes/vorzela-vm.tmLanguage.json` |
| Sublime | `sublime/Vorzela VM.sublime-syntax` |
| Vim | `vim/syntax/vorzela.vim` |
| Nano | `nano/vorzela.nanorc` |
| TextMate bundle (JetBrains) | `textmate/` |
