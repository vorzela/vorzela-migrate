/** @typedef {{ required?: boolean, type: string, values?: string[], summary: string, detail: string }} KeyDoc */

/** @type {Record<string, KeyDoc>} */
const VORM_KEYS = {
  PACKAGE: {
    type: "string (Go package)",
    summary: "Generated queries package name (default gen)",
    detail:
      "Go package name for `vorm/gen` output. Change if `gen` conflicts with another package in your module.\n\n" +
      "Example: `PACKAGE=vormgen` → import `your/module/vorm/vormgen`.\n\n" +
      "Must be a valid Go identifier (not `main` / `test`).",
  },
  OUT_DIR: {
    type: "string (path)",
    summary: "Directory for queries_gen.go",
    detail: "Default `./vorm/<PACKAGE>`. Set explicitly to pin a path when renaming PACKAGE.",
  },
  DRIVER: {
    type: "enum",
    values: ["pgx", "pq"],
    summary: "PostgreSQL client library",
    detail:
      "`pgx` (default) — jackc/pgx/v5 via `query.OpenPostgres`.\n\n" +
      "`pq` — database/sql + lib/pq via `query.WithDriver(query.PostgresPQ)`.\n\n" +
      "MySQL/MariaDB always use `query.OpenMySQL` (this key is for Postgres).",
  },
  DIALECT: {
    type: "enum",
    values: ["postgres", "postgresql", "mysql", "mariadb"],
    summary: "SQL dialect (placeholders + quoting)",
    detail:
      "`postgres` / `postgresql` → `$n` placeholders, double-quoted idents.\n\n" +
      "`mysql` / `mariadb` → `?` placeholders, backtick idents.\n\n" +
      "All dialects: values bound as parameters only (no SQL injection).",
  },
  QUERY_DIR: {
    type: "string (path)",
    summary: "Directory of // vorm:query stubs",
    detail: "Default `./queries`. Hand-written IR stubs for `vorm generate`.",
  },
  MODEL_DIR: {
    type: "string (path)",
    summary: "Directory for generated models",
    detail: "Default `./models`. **DO NOT EDIT** — regenerate with `vorm generate models`.",
  },
  SCHEMA_DIR: {
    type: "string (path)",
    summary: "Blueprint Schema.Create sources",
    detail: "Default `./schema/migrations`. Input for model generation.",
  },
  MODEL_PACKAGE: {
    type: "string (Go package)",
    summary: "Package name for models/ (default models)",
    detail: "Must be a valid Go identifier. Used when emitting `package models`.",
  },
  MODEL_IMPORT: {
    type: "string (import path)",
    summary: "Import path for models (optional)",
    detail:
      "Empty = `<module>/models` from go.mod. Query Row types are self-contained (sqlc-style); " +
      "this is used when stubs reference models.* types.",
  },
};

const VORM_KNOWN_KEYS = Object.keys(VORM_KEYS);

/**
 * @param {string} a
 * @param {string} b
 */
function levenshtein(a, b) {
  const m = a.length;
  const n = b.length;
  /** @type {number[]} */
  const dp = Array(n + 1);
  for (let j = 0; j <= n; j++) dp[j] = j;
  for (let i = 1; i <= m; i++) {
    let prev = dp[0];
    dp[0] = i;
    for (let j = 1; j <= n; j++) {
      const tmp = dp[j];
      const cost = a[i - 1].toLowerCase() === b[j - 1].toLowerCase() ? 0 : 1;
      dp[j] = Math.min(dp[j] + 1, dp[j - 1] + 1, prev + cost);
      prev = tmp;
    }
  }
  return dp[n];
}

/**
 * @param {string} key
 * @returns {string | null}
 */
function closestVormKey(key) {
  let best = null;
  let bestDist = Infinity;
  for (const k of VORM_KNOWN_KEYS) {
    const d = levenshtein(key, k);
    if (d < bestDist && d <= 3) {
      bestDist = d;
      best = k;
    }
  }
  return best;
}

/**
 * @param {string} key
 */
function hoverVormMarkdown(key) {
  const doc = VORM_KEYS[key];
  if (!doc) {
    const suggestion = closestVormKey(key);
    return suggestion
      ? `**Unknown key** \`${key}\`\n\nDid you mean \`${suggestion}\`?\n\nRun \`vorm config keys\`.`
      : `**Unknown key** \`${key}\`\n\nNot a recognised Vorzela \`.vorm\` setting.`;
  }
  const values = doc.values ? `\n\n**Allowed values:** \`${doc.values.join("` · `")}\`` : "";
  return (
    `### \`${key}\`\n\n` +
    `**Optional** (sensible defaults)\n\n` +
    `**Type:** ${doc.type}\n\n` +
    `${doc.summary}\n\n` +
    `${doc.detail}` +
    values
  );
}

/**
 * @param {string} name
 */
function isValidGoPackage(name) {
  if (!name || name === "main" || name === "test") return false;
  return /^[A-Za-z_][A-Za-z0-9_]*$/.test(name);
}

module.exports = {
  VORM_KEYS,
  VORM_KNOWN_KEYS,
  closestVormKey,
  hoverVormMarkdown,
  isValidGoPackage,
};
