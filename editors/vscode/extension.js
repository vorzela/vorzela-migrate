const vscode = require("vscode");
const { KEYS, BOOL_KEYS, KNOWN_KEYS, closestKey, hoverMarkdown } = require("./keys");

const ENV_VALUES = new Set(["development", "dev", "develop", "production", "prod"]);
const DRIFT_VALUES = new Set(["auto", "prompt", "reject"]);

/**
 * @param {vscode.TextDocument} document
 * @returns {vscode.Diagnostic[]}
 */
function lintDocument(document) {
  /** @type {vscode.Diagnostic[]} */
  const diagnostics = [];
  const seen = new Map();
  let hasDatabaseURL = false;

  for (let i = 0; i < document.lineCount; i++) {
    const line = document.lineAt(i);
    const trimmed = line.text.trim();
    if (trimmed === "" || trimmed.startsWith("#")) {
      continue;
    }

    const eq = trimmed.indexOf("=");
    if (eq === -1) {
      diagnostics.push(
        makeDiagnostic(
          line.range,
          "Malformed line — expected KEY=VALUE",
          vscode.DiagnosticSeverity.Warning,
          "malformed"
        )
      );
      continue;
    }

    // Map trimmed key back onto the full line for accurate ranges.
    const leading = line.text.match(/^\s*/)?.[0].length ?? 0;
    const keyPart = trimmed.slice(0, eq).trim();
    const rawValue = trimmed.slice(eq + 1);
    const valueNoComment = rawValue.includes(" #")
      ? rawValue.slice(0, rawValue.indexOf(" #"))
      : rawValue;
    const value = valueNoComment.trim();

    const keyStart = leading + trimmed.indexOf(keyPart);
    const keyRange = new vscode.Range(i, keyStart, i, keyStart + keyPart.length);

    if (!(keyPart in KEYS)) {
      const suggestion = closestKey(keyPart);
      const msg = suggestion
        ? `Unknown key — did you mean ${suggestion}?`
        : "Unknown key — not a recognised Vorzela .vm setting";
      diagnostics.push(
        makeDiagnostic(keyRange, msg, vscode.DiagnosticSeverity.Error, "unknown-key")
      );
      continue;
    }

    if (seen.has(keyPart)) {
      diagnostics.push(
        makeDiagnostic(
          keyRange,
          `Duplicate key (first defined on line ${seen.get(keyPart)})`,
          vscode.DiagnosticSeverity.Warning,
          "duplicate"
        )
      );
    } else {
      seen.set(keyPart, i + 1);
    }

    const valueStartInTrimmed = eq + 1 + (rawValue.length - rawValue.trimStart().length);
    const valueStart = leading + valueStartInTrimmed;
    const valueRange =
      value.length > 0
        ? new vscode.Range(i, valueStart, i, valueStart + value.length)
        : new vscode.Range(i, leading + eq + 1, i, line.text.length);

    if (BOOL_KEYS.has(keyPart) && value !== "" && !isValidBool(value)) {
      diagnostics.push(
        makeDiagnostic(
          valueRange,
          `Invalid boolean ${JSON.stringify(value)} — expected true, false, 1, or 0`,
          vscode.DiagnosticSeverity.Error,
          "invalid-bool"
        )
      );
    }

    if ((keyPart === "ENVIRONMENT" || keyPart === "ENV") && value !== "") {
      if (!ENV_VALUES.has(value.toLowerCase())) {
        diagnostics.push(
          makeDiagnostic(
            valueRange,
            `Invalid value ${JSON.stringify(value)} — expected development, dev, production, or prod`,
            vscode.DiagnosticSeverity.Error,
            "invalid-env"
          )
        );
      }
    }

    if (keyPart === "DRIFT_HANDLING" && value !== "") {
      if (!DRIFT_VALUES.has(value.toLowerCase())) {
        diagnostics.push(
          makeDiagnostic(
            valueRange,
            `Invalid value ${JSON.stringify(value)} — expected auto, prompt, or reject`,
            vscode.DiagnosticSeverity.Error,
            "invalid-drift"
          )
        );
      }
    }

    if (keyPart === "DATABASE_URL" && value !== "") {
      hasDatabaseURL = true;
    }

    if (keyPart === "DATABASE_URL" && value === "") {
      diagnostics.push(
        makeDiagnostic(
          keyRange,
          "DATABASE_URL is required but empty — provide a postgres:// or mysql:// connection string",
          vscode.DiagnosticSeverity.Error,
          "required-empty"
        )
      );
    }
  }

  if (!hasDatabaseURL) {
    const range = document.lineAt(0).range;
    diagnostics.push(
      makeDiagnostic(
        range,
        "Required key DATABASE_URL is missing — the tool cannot connect without a database URL",
        vscode.DiagnosticSeverity.Warning,
        "required-missing"
      )
    );
  }

  return diagnostics;
}

/**
 * @param {vscode.Range} range
 * @param {string} message
 * @param {vscode.DiagnosticSeverity} severity
 * @param {string} code
 */
function makeDiagnostic(range, message, severity, code) {
  const d = new vscode.Diagnostic(range, message, severity);
  d.source = "vorzela-vm";
  d.code = code;
  return d;
}

function isValidBool(v) {
  switch (v.toLowerCase()) {
    case "true":
    case "false":
    case "1":
    case "0":
      return true;
    default:
      return false;
  }
}

/**
 * @param {vscode.ExtensionContext} context
 */
function activate(context) {
  const collection = vscode.languages.createDiagnosticCollection("vorzela-vm");
  context.subscriptions.push(collection);

  /** @param {vscode.TextDocument | undefined} doc */
  const refresh = (doc) => {
    if (!doc || doc.languageId !== "vorzela-vm") {
      return;
    }
    collection.set(doc.uri, lintDocument(doc));
  };

  if (vscode.window.activeTextEditor) {
    refresh(vscode.window.activeTextEditor.document);
  }

  context.subscriptions.push(
    vscode.workspace.onDidOpenTextDocument(refresh),
    vscode.workspace.onDidChangeTextDocument((e) => refresh(e.document)),
    vscode.workspace.onDidCloseTextDocument((doc) => collection.delete(doc.uri)),
    vscode.window.onDidChangeActiveTextEditor((ed) => ed && refresh(ed.document))
  );

  context.subscriptions.push(
    vscode.languages.registerHoverProvider("vorzela-vm", {
      provideHover(document, position) {
        const line = document.lineAt(position.line);
        const trimmed = line.text.trim();
        if (trimmed.startsWith("#") || !trimmed.includes("=")) {
          return null;
        }
        const leading = line.text.match(/^\s*/)?.[0].length ?? 0;
        const eq = trimmed.indexOf("=");
        const keyPart = trimmed.slice(0, eq).trim();
        const keyStart = leading + trimmed.indexOf(keyPart);
        const keyEnd = keyStart + keyPart.length;
        if (position.character < keyStart || position.character > keyEnd) {
          // Allow hover on value for required-key tip when key is DATABASE_URL
          if (position.character > keyEnd && keyPart in KEYS) {
            return new vscode.Hover(new vscode.MarkdownString(hoverMarkdown(keyPart)));
          }
          return null;
        }
        const md = new vscode.MarkdownString(hoverMarkdown(keyPart));
        md.isTrusted = true;
        return new vscode.Hover(md, new vscode.Range(position.line, keyStart, position.line, keyEnd));
      },
    })
  );

  context.subscriptions.push(
    vscode.languages.registerCompletionItemProvider(
      "vorzela-vm",
      {
        provideCompletionItems(document, position) {
          const line = document.lineAt(position.line);
          const before = line.text.slice(0, position.character);
          const trimmedBefore = before.trimStart();

          // Inside a comment — no suggestions
          if (trimmedBefore.startsWith("#")) {
            return undefined;
          }

          const eqInBefore = before.indexOf("=");

          // ── Value side (after =) ──────────────────────────────────────────
          if (eqInBefore !== -1) {
            const key = before.slice(0, eqInBefore).trim();
            const doc = KEYS[key];
            if (!doc?.values?.length) {
              return undefined;
            }
            const afterEq = before.slice(eqInBefore + 1);
            // Skip if an inline comment has started
            if (afterEq.includes(" #")) {
              return undefined;
            }
            const valuePrefix = afterEq.trimStart();
            const valueStart = eqInBefore + 1 + (afterEq.length - afterEq.trimStart().length);
            const range = new vscode.Range(
              position.line,
              valueStart,
              position.line,
              position.character
            );
            return doc.values
              .filter((v) => v.toLowerCase().startsWith(valuePrefix.toLowerCase()) || valuePrefix === "")
              .map((v) => {
                const item = new vscode.CompletionItem(v, vscode.CompletionItemKind.EnumMember);
                item.detail = `${key} value`;
                item.range = range;
                item.sortText = v;
                return item;
              });
          }

          // ── Key side (Ctrl+Space / typing) — list all known keys ──────────
          // Prefix: trailing KEY fragment, or empty on a blank line
          const keyPrefixMatch = before.match(/([A-Za-z_][A-Za-z0-9_]*)?$/);
          const prefix = keyPrefixMatch?.[1] ?? "";
          const startChar = position.character - prefix.length;
          const range = new vscode.Range(position.line, startChar, position.line, position.character);
          const used = collectUsedKeys(document);

          /** @type {vscode.CompletionItem[]} */
          const items = [];
          for (const key of KNOWN_KEYS) {
            const doc = KEYS[key];
            const already = used.has(key);
            const item = new vscode.CompletionItem(
              key,
              doc.required ? vscode.CompletionItemKind.Keyword : vscode.CompletionItemKind.Property
            );
            item.detail = doc.required
              ? already
                ? "Required · already set"
                : "Required"
              : already
                ? "Optional · already set"
                : "Optional";
            item.documentation = new vscode.MarkdownString(
              hoverMarkdown(key) +
                (already ? "\n\n_Already present in this file — choosing again creates a duplicate._" : "")
            );
            item.range = range;
            item.insertText = new vscode.SnippetString(`${key}=$0`);
            item.filterText = key;
            item.sortText = `${doc.required ? "0" : "1"}${already ? "1" : "0"}_${key}`;
            item.preselect = Boolean(doc.required && !already && !prefix);
            items.push(item);
          }

          return new vscode.CompletionList(items, /* isIncomplete */ false);
        },
      },
      // Trigger while typing key names + after = for values
      ..."ABCDEFGHIJKLMNOPQRSTUVWXYZ_=".split("")
    )
  );
}

/**
 * @param {vscode.TextDocument} document
 * @returns {Set<string>}
 */
function collectUsedKeys(document) {
  const used = new Set();
  for (let i = 0; i < document.lineCount; i++) {
    const trimmed = document.lineAt(i).text.trim();
    if (!trimmed || trimmed.startsWith("#") || !trimmed.includes("=")) {
      continue;
    }
    const key = trimmed.slice(0, trimmed.indexOf("=")).trim();
    if (key in KEYS) {
      used.add(key);
    }
  }
  return used;
}

function deactivate() {}

module.exports = { activate, deactivate, lintDocument };
