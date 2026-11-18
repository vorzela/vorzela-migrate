# Zed editor — add to ~/.config/zed/settings.json (or project .zed/settings.json)
#
# vm.yaml/vm.yml use Zed's built-in YAML support; no mapping is needed.
# Map only the legacy .vm format to Ini until a tree-sitter grammar exists:
#
# {
#   "file_types": {
#     "Ini": ["vm", ".vm"]
#   }
# }
#
# Or use the VS Code / TextMate grammar via a community Zed TextMate bridge if available.
# Source grammar: ../vscode/syntaxes/vorzela-vm.tmLanguage.json
