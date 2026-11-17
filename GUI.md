# Vorzela Migrate GUI

`vm gui` is a local, confirmation-first GUI for the same migration engine the
CLI uses. It is a VorzelaJS client app (file router + Vorzela UI components)
served from the `vm` binary.

## Run the GUI after install

```bash
# install script (per-user, no root)
curl -fsSL https://raw.githubusercontent.com/vorzela/vorzela-migrate/main/install.sh | bash

# then, from any project directory
vm gui
```

If the shell has not picked up `~/.local/bin` yet:

```bash
~/.local/bin/vm gui
```

`vm init` now creates `vm.yaml`; `vm.yml` is also accepted. Legacy `.vm`
files remain readable and are checked last.

The command prints a one-time access URL, for example:

```text
URL:    http://127.0.0.1:7845
Access: http://127.0.0.1:7845/auth?token=…
```

Opening that URL exchanges the token for an HttpOnly session cookie and
redirects to the app. The token is consumed once. Restart `vm gui` to print a
new one.

## Version and updates

The sidebar footer and **Settings → Version & updates** show the installed
version. **Check for updates** queries the latest GitHub release; **Upgrade**
runs the same `vm upgrade` command as the terminal.

```bash
vm upgrade --check   # terminal equivalent of Check for updates
vm upgrade           # download and install the latest release
```

The running GUI process keeps using the old binary and embedded assets until
you restart it:

```bash
# Ctrl+C the running vm gui, then
vm gui
```

## Why it is safe by default

| Control | Default |
|---|---|
| Bind address | `127.0.0.1:7845` (loopback only) |
| Remote bind | refused unless `--allow-remote` **and** TLS cert/key are provided |
| Authentication | one-time 32-byte access token, then in-memory session |
| Session cookie | `HttpOnly`, `SameSite=Strict`, `Secure` under TLS |
| CSRF | per-session token + `X-VM-GUI` header + same-origin check |
| DNS rebinding | `Host` header must be `127.0.0.1`, `localhost` or `[::1]` |
| Credentials | AES-256-GCM, key in a `0600` local file |
| Child process | password passed as `DATABASE_URL` in the environment, never argv |
| Mutations | server-issued, single-use confirmation plan required |
| Destructive actions | typed project-name confirmation |
| File access | migration paths must resolve inside the project directory |
| HTTP headers | strict CSP, `nosniff`, `DENY` framing, no CORS |
| Request bodies | 4 MiB API limit, 2 MiB migration-file limit |
| Audit | local JSONL log with secrets redacted |

The GUI is not a replacement for database permissions. Use a least-privilege
database role, especially for production-like databases.

Like most localhost development servers, the GUI trusts the local user account:
a same-user process can read the config/key, and browser cookies are not
isolated by port. Run it only on a machine you trust and do not expose it to
untrusted local services.

Read-only status checks call `vm status`, which may create vm's own
`migrations` bookkeeping table on a brand-new database. No user tables are
changed by a status check.

## Flags

```bash
vm gui [flags]

--addr 127.0.0.1:7845     listen address
--store yaml|sqlite       config store (default yaml)
--config <path>           explicit config file
--config-dir <dir>        directory for config, key, backups and audit log
--vm <path>               vm binary to execute (default: the running binary)
--no-open                 do not open a browser
--allow-remote            allow a non-loopback bind (TLS is required)
--tls-cert <pem>          TLS certificate
--tls-key <pem>           TLS private key
--dev-origin <origin>     extra allowed Origin for the Vite dev server
```

Examples:

```bash
vm gui
vm gui --store sqlite
vm gui --no-open --addr 127.0.0.1:9000
vm gui --allow-remote --tls-cert cert.pem --tls-key key.pem --addr 0.0.0.0:7845
```

`--allow-remote` without TLS is rejected. Use a TLS reverse proxy in front of a
loopback listener if you cannot pass certificates directly.

## Storage

Linux/macOS:

```text
~/.config/vorzela-migrate/
├── gui.yaml          # YAML store (default)
├── gui.db            # SQLite store (--store sqlite)
├── gui.key           # 32-byte AES-256-GCM key, mode 0600
├── audit.jsonl       # local audit trail, mode 0600
└── backups/
    └── <project-id>/ # timestamped migration backups
```

Windows uses `%AppData%\vorzela-migrate\`. Override the directory with
`--config-dir`.

YAML shape (passwords are ciphertext, never plaintext):

```yaml
version: 1
active_project_id: prj_…
projects:
  - id: prj_…
    name: my-app
    path: /home/you/code/my-app
    connection_id: con_…
connections:
  - id: con_…
    name: Local Postgres
    engine: postgres
    host: 127.0.0.1
    port: 5432
    database: app_development
    username: app
    password_enc: v1:…
    sslmode: disable
settings:
  confirmation_ttl_seconds: 600
  session_ttl_minutes: 720
  prefer_runtime_credentials: true
```

The SQLite store keeps the same data in `meta`, `projects` and `connections`
tables. Switching stores is manual; the GUI does not copy data between them.
If you move the store to another machine, copy `gui.key` too (or re-enter the
passwords from the Connections page).

## Add a project and connection

1. Open **Connections** and add a target: engine, host, port, database,
   username and password. **Test connection** pings the database without
   running migrations.
2. Open **Projects** and paste the absolute project path. Select a default
   connection.
3. If the folder has no `vm.yaml`, use **Initialize vm.yaml**. By default the GUI does
   **not** write `DATABASE_URL` into the file: it injects the selected
   connection at runtime, so the password stays out of the repository. Turn on
   "Write DATABASE_URL" only when you also run `vm` from a terminal; the GUI
   then warns and requires typed confirmation.

The topbar selects the active project and the connection used for every plan.
The confirmation dialog always repeats the target database and a redacted DSN.

## Tables page

The **Tables** page shows the live schema for the selected database using the
Vorzela UI `SimpleDataTable` (backed by `@vorzelajs/table`). Each table row
shows its column count, the `create_*_table` migration that defines it and
whether that migration has run.

- **Add table** opens a modal that calls `vm make migration` with the optional
  soft-delete, trigger, belongs-to, one-to-one and many-to-many flags, then
  opens the generated file in the editor.
- **Add column** opens a form for name, type, nullability, default, unique and
  optional foreign key/on-delete/backfill.
  - If the table has an executed `create_*_table` migration and the column is
    drift-safe, the form edits that migration and applies it with
    `vm migrate --force --detect-drift`.
  - If the create migration is pending, the column is written to that file and
    applied by **Run pending**.
  - If drift cannot apply the column (NOT NULL without a default, NOT NULL
    UNIQUE, NOT NULL foreign key, or no create migration), the form creates a
    complete `add_*` follow-up migration for review.

Column types and defaults are validated against a small safe grammar before
SQL is generated. Complex expressions can still be edited directly in the
migration file.

## Create and edit migrations

- **New migration** calls `vm make migration` with optional soft-delete,
  triggers, belongs-to, one-to-one and many-to-many flags. The generated file
  opens in the editor.
- Edits are saved atomically with a timestamped backup. A changed file is
  detected with a hash; the GUI refuses to overwrite a file changed on disk.
- An already-executed migration requires typing its filename before saving.

### Drift-first edit rules

When a migration has already run, the GUI compares the old and new SQL and
explains the safest way to apply it:

| Edit | GUI guidance |
|---|---|
| Add a nullable column | **Drift safe** — `vm migrate --force --detect-drift` |
| Add `NOT NULL DEFAULT …` | **Drift safe** |
| Add `NOT NULL` without a default | **Follow-up migration** — `add_<col>_to_<table>` |
| Add `NOT NULL UNIQUE` | **Follow-up migration** |
| Add a `NOT NULL` foreign-key column | **Follow-up migration** |
| Change a type, nullability, default, primary key or unique constraint | **Follow-up migration** |
| Remove a column or table | **Destructive** — explicit forward migration |
| Add a new table to an executed file | **Follow-up migration** |

Applying an edit uses:

```bash
vm migrate --force --detect-drift --drift-handling=auto --yes
```

`--drift-handling=auto` is only reached after the GUI's own confirmation plan.
Drift applies **additive** fixes. Destructive fixes are skipped; the GUI never
passes `--allow-destructive` for drift.

The plan preparation runs a dry run first and shows the exact statements. If
the dry run reports a NOT NULL/UNIQUE advisory, the GUI refuses the drift plan
and directs you to the follow-up migration.

For a pending (not yet executed) migration, edit and save normally, then use
**Run pending migrations**.

## Confirmation plans

Every mutating endpoint returns `428`/`409` unless the browser first obtains a
server plan and then executes it. A plan is bound to the session, expires after
10 minutes, and can be used once. The browser cannot send an arbitrary command;
the server maps the action to a fixed `vm` invocation.

Risk levels:

| Risk | Examples | Confirmation |
|---|---|---|
| Low | add project, create migration file | click |
| Medium | save connection, save migration, run drift/pending | click |
| High | save an executed migration, rollback, plaintext `vm.yaml` DSN | type filename/project name |
| Critical | fresh rebuild | type project name |

Plans show the project, database, migration file, command and a preview
(dry-run SQL, diff, or Down SQL).

## Rollback and fresh

- **Rollback** previews the Down SQL with `vm rollback --dry-run`, then runs
  with `--force` after you type the project name.
- **Fresh rebuild** previews all Down SQL, then runs `vm fresh --force` after
  typed confirmation. Only use it on disposable development databases.

## Audit and backups

- `~/.config/vorzela-migrate/audit.jsonl` records plan creation, execution,
  failures, connection tests and token failures. Passwords are never included.
- Backups live under `backups/<project-id>/` and the newest 50 per project are
  kept. Restoring is manual: copy a `.bak` file back over the migration and
  re-apply drift if needed.

## Development

The GUI source is in [`gui/`](gui/). It uses the VorzelaJS packages declared in
`gui/package.json`; the checked-in build is embedded in the Go binary.

```bash
# build assets and copy them into the embed folder
make gui

# rebuild vm with the new assets
go build -o vm ./cmd/vm

# Vite dev server (run the API separately)
./vm gui --no-open --dev-origin http://localhost:5173
cd gui && npm run dev
```

The component source was copied with the Vorzela JS UI CLI (`vorzelajs-ui add`)
and can be updated the same way. See [`gui/README.md`](gui/README.md).

## Limitations

- The GUI edits timestamped migration files. Use `vm extensions|functions|enums`
  for helper SQL.
- YAML and SQLite stores are separate; there is no automatic migration between
  them.
- A saved connection is a convenience, not an authorization boundary. The
  database still enforces its own permissions.
- Drift is intentionally additive. Drops and constraint changes need a
  reviewed forward migration.
