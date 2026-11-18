# Vorzela `vm.yaml`/`vm.yml` + `.vorm` editor support

`vm.yaml` / `vm.yml` are real YAML files and use your editor's built-in YAML
support. The Vorzela extensions in this directory add syntax highlighting,
hover docs and completions for the **legacy `.vm`** format and for vorm
`.vorm` files.

| File | Syntax | Language id |
|------|--------|-------------|
| `vm.yaml` / `vm.yml` | Built-in YAML | `yaml` |
| `.vm` (legacy) | Vorzela Migrate | `vorzela-vm` |
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

### `vm.yaml` / `vm.yml` (migrate)

Use the built-in YAML language support. `vm lint` validates keys and values:

```bash
vm lint
```

### `.vm` (legacy migrate config)

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
ln -sfn "$(pwd)/editors/vscode" ~/.cursor/extensions/vorzela.vorzela-vm-1.3.0
```

Extension version is **1.3.0** (`vorzela-vm` package name; includes `.vorm`).
