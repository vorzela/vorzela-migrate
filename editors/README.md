# Vorzela `.vm` + `.vorm` syntax highlighting

Editor support for:

| File | Tool | Language id |
|------|------|-------------|
| `.vm` | Vorzela Migrate | `vorzela-vm` |
| `.vorm` | vorm ORM | `vorzela-vorm` |

## Install

```bash
./editors/install.sh          # all detected editors
./editors/install.sh --vscode # Cursor / VS Code only
# then: Developer: Reload Window
```

| Editor | Auto via `install.sh`? |
|--------|------------------------|
| **Cursor / VS Code / Insiders / VSCodium** | Yes |
| **Sublime Text** | Yes |
| **Vim / Neovim** | Yes (`.vm` + `.vorm`) |
| **Nano / Helix / Zed / JetBrains** | Detected / partial |

## Features (VS Code / Cursor)

### `.vm` (migrate)

Syntax · hover · live lint (unknown keys, `DATABASE_URL`) · Ctrl+Space completions

### `.vorm` (ORM)

Syntax · hover · live lint (`DRIVER`/`DIALECT`/`PACKAGE`) · completions for `pgx`/`pq` and dialects

```bash
vorm init
vorm config set PACKAGE=vormgen
```

Workspace [`.vscode/settings.json`](../.vscode/settings.json) associates both file types.

### Manual symlink (Cursor)

```bash
mkdir -p ~/.cursor/extensions
ln -sfn "$(pwd)/editors/vscode" ~/.cursor/extensions/vorzela.vorzela-vm-1.2.0
```

Extension version is **1.2.0** (`vorzela-vm` package name; includes `.vorm`).
