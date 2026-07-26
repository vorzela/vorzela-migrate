/** @typedef {{ required?: boolean, type: string, values?: string[], summary: string, detail: string }} KeyDoc */

/** @type {Record<string, KeyDoc>} */
const KEYS = {
  DATABASE_URL: {
    required: true,
    type: "string (DSN)",
    summary: "Database connection string (required)",
    detail:
      "Required. Connection URL used by every `vm` command that talks to the DB.\n\n" +
      "**PostgreSQL (default):** `postgres://user:pass@host:5432/db`\n\n" +
      "**MySQL:** `mysql://user:pass@host:3306/db` or `user:pass@tcp(host:3306)/db`\n\n" +
      "**MariaDB:** same as MySQL, include `mariadb` in host or path for dialect detection.\n\n" +
      "Also overridable via `--dsn` / env `DATABASE_URL`.",
  },
  MIGRATION_PATH: {
    type: "string (path)",
    summary: "Directory of migration SQL files",
    detail: "Optional. Default `./migrations`. Override with `--path`.",
  },
  SQLC_SUPPORT: {
    type: "boolean",
    values: ["true", "false", "1", "0"],
    summary: "Add +goose Up/Down markers for sqlc/goose",
    detail: "Optional. Default `false`. When `true`, generated migrations include goose directional markers.",
  },
  ENVIRONMENT: {
    type: "enum",
    values: ["development", "dev", "develop", "production", "prod"],
    summary: "Environment profile — auto-applies enhanced/online/verbose defaults",
    detail:
      "Optional (defaults to development behaviour when unset).\n\n" +
      "| Value | ENHANCED | ONLINE | VERIFY_CHECKSUMS | DETECT_DRIFT | VERBOSE |\n" +
      "|-------|----------|--------|------------------|--------------|----------|\n" +
      "| development / dev | true | false | true | true | true |\n" +
      "| production / prod | true | true | true | true | false |\n\n" +
      "Explicit keys in `.vm` override these defaults.",
  },
  ENV: {
    type: "enum",
    values: ["development", "dev", "develop", "production", "prod"],
    summary: "Alias for ENVIRONMENT",
    detail: "Same as `ENVIRONMENT`. Prefer `ENVIRONMENT` for clarity.",
  },
  ENHANCED: {
    type: "boolean",
    values: ["true", "false", "1", "0"],
    summary: "Enable checksums + locking + drift as a group",
    detail: "Optional. Overrides ENVIRONMENT default for the enhanced feature set.",
  },
  ONLINE: {
    type: "boolean",
    values: ["true", "false", "1", "0"],
    summary: "Zero-downtime DDL strategies (PG + MySQL 8+ / MariaDB)",
    detail: "Optional. Production default is `true`; development default is `false`.",
  },
  VERIFY_CHECKSUMS: {
    type: "boolean",
    values: ["true", "false", "1", "0"],
    summary: "Detect modified already-executed migration files",
    detail:
      "Optional. When an executed file changes, migrate fails unless you pass `vm migrate --force` " +
      "(then pair with `--detect-drift` to sync the live schema).",
  },
  DETECT_DRIFT: {
    type: "boolean",
    values: ["true", "false", "1", "0"],
    summary: "Compare live schema to migration SQL before/after migrate",
    detail: "Optional. Prefer editing `create_*_table.sql` + `vm migrate --force --detect-drift` over `add_` migrations.",
  },
  VERBOSE: {
    type: "boolean",
    values: ["true", "false", "1", "0"],
    summary: "Coloured output with per-migration timing",
    detail: "Optional. Development default `true`; production default `false`.",
  },
  AUTO_RUN_EXTENSIONS: {
    type: "boolean",
    values: ["true", "false", "1", "0"],
    summary: "Run extensions.sql before migrate (PostgreSQL only)",
    detail: "Optional. Default `true`. Silently skipped on MySQL/MariaDB.",
  },
  AUTO_RUN_FUNCTIONS: {
    type: "boolean",
    values: ["true", "false", "1", "0"],
    summary: "Run functions.sql before migrate (PostgreSQL only)",
    detail: "Optional. Default `true`. Required for `--triggers` on PostgreSQL. Skipped on MySQL/MariaDB.",
  },
  AUTO_RUN_ENUMS: {
    type: "boolean",
    values: ["true", "false", "1", "0"],
    summary: "Run enums.sql before migrate (PostgreSQL only)",
    detail: "Optional. Default `true`. Silently skipped on MySQL/MariaDB.",
  },
  DRIFT_HANDLING: {
    type: "enum",
    values: ["auto", "prompt", "reject"],
    summary: "What to do when schema drift is found",
    detail:
      "Optional. Default `prompt`.\n\n" +
      "- `auto` — apply drift fixes silently\n" +
      "- `prompt` — ask before applying (recommended)\n" +
      "- `reject` — fail migrate when drift is detected (production-safe)",
  },
};

const BOOL_KEYS = new Set(
  Object.entries(KEYS)
    .filter(([, d]) => d.type === "boolean")
    .map(([k]) => k)
);

const KNOWN_KEYS = Object.keys(KEYS);

/**
 * @param {string} typo
 * @returns {string}
 */
function closestKey(typo) {
  const upper = typo.toUpperCase();
  let best = "";
  let bestDist = Infinity;
  for (const key of KNOWN_KEYS) {
    const d = levenshtein(upper, key);
    if (d < bestDist && d <= Math.max(2, Math.floor(key.length / 4))) {
      bestDist = d;
      best = key;
    }
  }
  return best;
}

/**
 * @param {string} a
 * @param {string} b
 */
function levenshtein(a, b) {
  const m = a.length;
  const n = b.length;
  const dp = Array.from({ length: m + 1 }, () => new Array(n + 1).fill(0));
  for (let i = 0; i <= m; i++) dp[i][0] = i;
  for (let j = 0; j <= n; j++) dp[0][j] = j;
  for (let i = 1; i <= m; i++) {
    for (let j = 1; j <= n; j++) {
      const cost = a[i - 1] === b[j - 1] ? 0 : 1;
      dp[i][j] = Math.min(dp[i - 1][j] + 1, dp[i][j - 1] + 1, dp[i - 1][j - 1] + cost);
    }
  }
  return dp[m][n];
}

/**
 * @param {string} key
 * @returns {string}
 */
function hoverMarkdown(key) {
  const doc = KEYS[key];
  if (!doc) {
    const suggestion = closestKey(key);
    return suggestion
      ? `**Unknown key** \`${key}\`\n\nDid you mean \`${suggestion}\`?\n\nRun \`vm lint\` for the full key list.`
      : `**Unknown key** \`${key}\`\n\nNot a recognised Vorzela \`.vm\` setting. Run \`vm lint\`.`;
  }

  const req = doc.required
    ? "**Required** — migrations and DB commands will fail without this."
    : "**Optional**";
  const values = doc.values ? `\n\n**Allowed values:** \`${doc.values.join("` · `")}\`` : "";
  return (
    `### \`${key}\`\n\n` +
    `${req}\n\n` +
    `**Type:** ${doc.type}\n\n` +
    `${doc.summary}\n\n` +
    `${doc.detail}` +
    values
  );
}

module.exports = {
  KEYS,
  BOOL_KEYS,
  KNOWN_KEYS,
  closestKey,
  hoverMarkdown,
};
