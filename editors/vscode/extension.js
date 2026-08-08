const vscode = require("vscode");
const { KEYS, BOOL_KEYS, KNOWN_KEYS, closestKey, hoverMarkdown } = require("./keys");
const {
  VORM_KEYS,
  VORM_KNOWN_KEYS,
  closestVormKey,
  hoverVormMarkdown,
  isValidGoPackage,
} = require("./vorm-keys");

const ENV_VALUES = new Set(["development", "dev", "develop", "production", "prod"]);
const DRIFT_VALUES = new Set(["auto", "prompt", "reject"]);
const DRIVER_VALUES = new Set(["pgx", "pq"]);
const DIALECT_VALUES = new Set(["postgres", "postgresql", "mysql", "mariadb"]);

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
  const vmCollection = vscode.languages.createDiagnosticCollection("vorzela-vm");
  const vormCollection = vscode.languages.createDiagnosticCollection("vorzela-vorm");
  context.subscriptions.push(vmCollection, vormCollection);

  /** @param {vscode.TextDocument | undefined} doc */
  const refresh = (doc) => {
    if (!doc) return;
    if (doc.languageId === "vorzela-vm") {
      vmCollection.set(doc.uri, lintDocument(doc));
    } else if (doc.languageId === "vorzela-vorm") {
      vormCollection.set(doc.uri, lintVormDocument(doc));
    }
  };

  if (vscode.window.activeTextEditor) {
    refresh(vscode.window.activeTextEditor.document);
  }

  context.subscriptions.push(
    vscode.workspace.onDidOpenTextDocument(refresh),
    vscode.workspace.onDidChangeTextDocument((e) => refresh(e.document)),
    vscode.workspace.onDidCloseTextDocument((doc) => {
      vmCollection.delete(doc.uri);
      vormCollection.delete(doc.uri);
    }),
    vscode.window.onDidChangeActiveTextEditor((ed) => ed && refresh(ed.document))
  );

  registerHoverAndCompletions(context, "vorzela-vm", KEYS, KNOWN_KEYS, hoverMarkdown, collectUsedKeys);
  registerHoverAndCompletions(
    context,
    "vorzela-vorm",
    VORM_KEYS,
    VORM_KNOWN_KEYS,
    hoverVormMarkdown,
    collectUsedVormKeys
  );
}

/**
 * @param {vscode.ExtensionContext} context
 * @param {string} languageId
 * @param {Record<string, any>} keys
 * @param {string[]} knownKeys
 * @param {(k: string) => string} hoverFn
 * @param {(doc: vscode.TextDocument) => Set<string>} usedFn
 */
function registerHoverAndCompletions(context, languageId, keys, knownKeys, hoverFn, usedFn) {
  context.subscriptions.push(
    vscode.languages.registerHoverProvider(languageId, {
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
          if (position.character > keyEnd && keyPart in keys) {
            return new vscode.Hover(new vscode.MarkdownString(hoverFn(keyPart)));
          }
          return null;
        }
        const md = new vscode.MarkdownString(hoverFn(keyPart));
        md.isTrusted = true;
        return new vscode.Hover(md, new vscode.Range(position.line, keyStart, position.line, keyEnd));
      },
    })
  );

  context.subscriptions.push(
    vscode.languages.registerCompletionItemProvider(
      languageId,
      {
        provideCompletionItems(document, position) {
          const line = document.lineAt(position.line);
          const before = line.text.slice(0, position.character);
          const trimmedBefore = before.trimStart();
          if (trimmedBefore.startsWith("#")) {
            return undefined;
          }
          const eqInBefore = before.indexOf("=");
          if (eqInBefore !== -1) {
            const key = before.slice(0, eqInBefore).trim();
            const doc = keys[key];
            if (!doc?.values?.length) {
              return undefined;
            }
            const afterEq = before.slice(eqInBefore + 1);
            if (afterEq.includes(" #")) {
              return undefined;
            }
            const valuePrefix = afterEq.trimStart();
            const valueStart = eqInBefore + 1 + (afterEq.length - afterEq.trimStart().length);
            const range = new vscode.Range(position.line, valueStart, position.line, position.character);
            return doc.values
              .filter((v) => v.toLowerCase().startsWith(valuePrefix.toLowerCase()) || valuePrefix === "")
              .map((v) => {
                const item = new vscode.CompletionItem(v, vscode.CompletionItemKind.EnumMember);
                item.detail = `${key} value`;
                item.range = range;
                return item;
              });
          }
          const keyPrefixMatch = before.match(/([A-Za-z_][A-Za-z0-9_]*)?$/);
          const prefix = keyPrefixMatch?.[1] ?? "";
          const startChar = position.character - prefix.length;
          const range = new vscode.Range(position.line, startChar, position.line, position.character);
          const used = usedFn(document);
          /** @type {vscode.CompletionItem[]} */
          const items = [];
          for (const key of knownKeys) {
            const doc = keys[key];
            const already = used.has(key);
            const item = new vscode.CompletionItem(key, vscode.CompletionItemKind.Property);
            item.detail = already ? "Optional · already set" : "Optional";
            item.documentation = new vscode.MarkdownString(hoverFn(key));
            item.range = range;
            item.insertText = new vscode.SnippetString(`${key}=$0`);
            item.sortText = `${already ? "1" : "0"}_${key}`;
            items.push(item);
          }
          return new vscode.CompletionList(items, false);
        },
      },
      ..."ABCDEFGHIJKLMNOPQRSTUVWXYZ_=".split("")
    )
  );
}

/**
 * Live lint for `.vorm` (PACKAGE, DRIVER, DIALECT, …).
 * @param {vscode.TextDocument} document
 */
function lintVormDocument(document) {
  /** @type {vscode.Diagnostic[]} */
  const diagnostics = [];
  const seen = new Map();

  for (let i = 0; i < document.lineCount; i++) {
    const line = document.lineAt(i);
    const trimmed = line.text.trim();
    if (trimmed === "" || trimmed.startsWith("#")) continue;

    const eq = trimmed.indexOf("=");
    if (eq === -1) {
      diagnostics.push(makeVormDiag(line.range, "Malformed line — expected KEY=VALUE", vscode.DiagnosticSeverity.Warning, "malformed"));
      continue;
    }

    const leading = line.text.match(/^\s*/)?.[0].length ?? 0;
    const keyPart = trimmed.slice(0, eq).trim();
    const rawValue = trimmed.slice(eq + 1);
    const valueNoComment = rawValue.includes(" #") ? rawValue.slice(0, rawValue.indexOf(" #")) : rawValue;
    const value = valueNoComment.trim().replace(/^["']|["']$/g, "");

    const keyStart = leading + trimmed.indexOf(keyPart);
    const keyRange = new vscode.Range(i, keyStart, i, keyStart + keyPart.length);
    const valueStartInTrimmed = eq + 1 + (rawValue.length - rawValue.trimStart().length);
    const valueStart = leading + valueStartInTrimmed;
    const valueRange =
      value.length > 0
        ? new vscode.Range(i, valueStart, i, valueStart + value.length)
        : new vscode.Range(i, leading + eq + 1, i, line.text.length);

    if (!(keyPart in VORM_KEYS)) {
      const suggestion = closestVormKey(keyPart);
      const msg = suggestion
        ? `Unknown key — did you mean ${suggestion}?`
        : "Unknown key — not a recognised Vorzela .vorm setting";
      diagnostics.push(makeVormDiag(keyRange, msg, vscode.DiagnosticSeverity.Error, "unknown-key"));
      continue;
    }

    if (seen.has(keyPart)) {
      diagnostics.push(
        makeVormDiag(keyRange, `Duplicate key (first defined on line ${seen.get(keyPart)})`, vscode.DiagnosticSeverity.Warning, "duplicate")
      );
    } else {
      seen.set(keyPart, i + 1);
    }

    if (keyPart === "DRIVER" && value !== "" && !DRIVER_VALUES.has(value.toLowerCase())) {
      diagnostics.push(
        makeVormDiag(valueRange, `Invalid DRIVER ${JSON.stringify(value)} — expected pgx or pq`, vscode.DiagnosticSeverity.Error, "invalid-driver")
      );
    }
    if (keyPart === "DIALECT" && value !== "" && !DIALECT_VALUES.has(value.toLowerCase())) {
      diagnostics.push(
        makeVormDiag(
          valueRange,
          `Invalid DIALECT ${JSON.stringify(value)} — expected postgres, mysql, or mariadb`,
          vscode.DiagnosticSeverity.Error,
          "invalid-dialect"
        )
      );
    }
    if ((keyPart === "PACKAGE" || keyPart === "MODEL_PACKAGE") && value !== "" && !isValidGoPackage(value)) {
      diagnostics.push(
        makeVormDiag(
          valueRange,
          `Invalid Go package name ${JSON.stringify(value)} — use letters/digits/_ (not main/test)`,
          vscode.DiagnosticSeverity.Error,
          "invalid-package"
        )
      );
    }
  }

  return diagnostics;
}

/**
 * @param {vscode.Range} range
 * @param {string} message
 * @param {vscode.DiagnosticSeverity} severity
 * @param {string} code
 */
function makeVormDiag(range, message, severity, code) {
  const d = new vscode.Diagnostic(range, message, severity);
  d.source = "vorzela-vorm";
  d.code = code;
  return d;
}

/**
 * @param {vscode.TextDocument} document
 * @returns {Set<string>}
 */
function collectUsedKeys(document) {
  const used = new Set();
  for (let i = 0; i < document.lineCount; i++) {
    const trimmed = document.lineAt(i).text.trim();
    if (!trimmed || trimmed.startsWith("#") || !trimmed.includes("=")) continue;
    const key = trimmed.slice(0, trimmed.indexOf("=")).trim();
    if (key in KEYS) used.add(key);
  }
  return used;
}

/**
 * @param {vscode.TextDocument} document
 * @returns {Set<string>}
 */
function collectUsedVormKeys(document) {
  const used = new Set();
  for (let i = 0; i < document.lineCount; i++) {
    const trimmed = document.lineAt(i).text.trim();
    if (!trimmed || trimmed.startsWith("#") || !trimmed.includes("=")) continue;
    const key = trimmed.slice(0, trimmed.indexOf("=")).trim();
    if (key in VORM_KEYS) used.add(key);
  }
  return used;
}

function deactivate() {}

module.exports = { activate, deactivate, lintDocument, lintVormDocument };
