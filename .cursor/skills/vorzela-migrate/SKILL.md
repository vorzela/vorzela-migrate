---
name: vorzela-migrate
description: >-
  Operate the Vorzela `vm` migration CLI — .vm config, make/migrate/rollback,
  drift, extensions/functions/enums. Use when creating or running migrations,
  editing schema, adding columns, fixing drift, or when the user mentions vm,
  Vorzela, migrations, or LLM.md workflows. Critical: prefer edit create +
  drift over add_ migrations.
---

# Vorzela Migrate (CLI mastery)

## Source of truth

Read **[LLM.md](LLM.md)** §1.1 and §6 for the full decision tree. This skill is the short playbook.

## Non-negotiables

1. Prefer **`vm` commands** over hand-writing files.
2. The only file an agent should create from scratch is **`.vm`** (then `vm lint`).
3. Never hand-create migration SQL, `extensions.sql`, `functions.sql`, or `enums.sql` — let `vm` generate them, then edit.
4. Default database is **PostgreSQL**. Use MySQL/MariaDB only when the user asks. See skill `vorzela-dialects`.
5. **Keep migration history thin** — do not default to `add_` for schema tweaks.

## Schema change decision (memorize)

When the user asks to change an existing table (add column, index, soft-delete, etc.):

| Order | Situation | Action |
|------:|-----------|--------|
| 1 | Brand-new table | `vm make migration <table>` |
| 2 | Disposable / local DB | Edit `create_*_table.sql` → `vm fresh --force` |
| 3 | Executed create, editable history | Edit `create_*_table.sql` → `vm migrate --force --detect-drift` → **yes** |
| 4 | Drift advisory: NOT NULL w/o DEFAULT or UNIQUE | **Then** `vm make migration add_<col>_to_<table>` + backfill |
| 5 | Immutable production history | One purposeful forward `add_`/`alter` migration (still avoid many tiny files) |

**Never** jump to `vm make migration add_…` as the first reaction to “add a column”.

**`vm migrate --force`:** required after editing an already-executed migration (checksum). Pair with `--detect-drift` to sync the live DB. See LLM.md §5.1. Do not use on immutable production history.

**Avoid:** stacks of `add_*` files, keeping `fix_schema_drift` as normal history, stale create migrations while stacking alters.

## Setup checklist (new table / new project)

```
- [ ] Write .vm with DATABASE_URL (+ ENVIRONMENT if needed)
- [ ] vm lint
- [ ] PostgreSQL only: vm extensions migrate → edit → migrate again
- [ ] PostgreSQL only: vm functions migrate (needed for --triggers)
- [ ] PostgreSQL only: vm enums migrate → edit types
- [ ] vm make migration <name> [flags]   # new tables only
- [ ] Edit generated SQL if needed
- [ ] vm migrate
```

## Command map

| Goal | Command |
|------|---------|
| Validate config | `vm lint` |
| New table | `vm make migration <name>` |
| Soft delete / triggers on **new** table | put flags on the create make |
| Change existing table | edit create → `vm migrate --force --detect-drift` → yes (or fresh) |
| Apply | `vm migrate` |
| Preview | `vm migrate --dry-run` |
| Sync live ↔ migration SQL after editing create | `vm migrate --force --detect-drift` → yes |
| Checksum only (file already matches DB) | `vm migrate --force` |
| Rollback batch | `vm rollback` |
| Status | `vm status` |
| Rebuild disposable DB | `vm fresh --force` |

## Naming

- snake_case only
- `create_` + `_table` added automatically unless name starts with `add_` or `trigger_`
- `add_` = alteration scaffold — **rare** (step 4/5 only)
- Do not combine `--many-to-many` with `--belongs-to` / `--one-to-one`

## Config priority

CLI flag → `DATABASE_URL` env → `.vm` (cwd/parents) → `.env` → defaults.

## MySQL / MariaDB quick rules

- Skip `vm extensions|functions|enums`.
- Ensure `.vm` `DATABASE_URL` is set before `vm make migration` so scaffolds use MySQL types.
- `--triggers` is OK (MySQL `SET NEW.updated_at` trigger; no `functions.sql`).
- `--online` is OK for MySQL and MariaDB.
