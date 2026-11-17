import { computed, effect, signal } from '@vorzelajs/core'
import { For, Show } from '@vorzelajs/dom'
import { createFileRoute, useNavigate } from '@vorzelajs/router'

import { Alert } from '@/components/ui/alert.tsx'
import { Badge } from '@/components/ui/badge.tsx'
import { Button } from '@/components/ui/button.tsx'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card.tsx'
import { Empty } from '@/components/ui/empty.tsx'
import { Field } from '@/components/ui/field.tsx'
import { Input } from '@/components/ui/input.tsx'
import { Switch } from '@/components/ui/switch.tsx'
import {
  LayoutList,
  Play,
  Plus,
  RotateCcw,
  Trash2,
} from '@/components/ui/icons.tsx'
import { fetchOverview } from '@/lib/api.ts'
import { formatBytes, formatRelative, migrationName } from '@/lib/format.ts'
import { activeConnection, activeProject, requestAction } from '@/lib/state.ts'
import type { MigrationListItem, ProjectOverview } from '@/lib/api-types.ts'

export const Route = createFileRoute('/migrations')({
  head: () => ({
    title: 'Migrations · Vorzela Migrate',
    description:
      'Review executed and pending migrations, create files and run them.',
  }),
  component: MigrationsPage,
})

function MigrationBadge({ item }: { item: MigrationListItem }) {
  if (item.fileMissing) return <Badge variant="destructive">file missing</Badge>
  if (!item.statusKnown)
    return <Badge variant="secondary">status unknown</Badge>
  if (item.executed && item.checksumOk === false)
    return <Badge variant="destructive">edited after run</Badge>
  if (item.executed)
    return (
      <Badge variant="success">
        executed{item.batch ? ` · batch ${item.batch}` : ''}
      </Badge>
    )
  return <Badge variant="warning">pending</Badge>
}

function MigrationsPage() {
  const navigate = useNavigate()
  const [overview, setOverview] = signal<ProjectOverview | null>(null)
  const [loading, setLoading] = signal(false)
  const [error, setError] = signal('')
  const [search, setSearch] = signal('')
  const [showCreate, setShowCreate] = signal(false)
  const [name, setName] = signal('')
  const [softDelete, setSoftDelete] = signal(false)
  const [triggers, setTriggers] = signal(false)
  const [belongsTo, setBelongsTo] = signal('')
  const [oneToOne, setOneToOne] = signal('')
  const [manyToMany, setManyToMany] = signal('')
  const [steps, setSteps] = signal('1')
  const [specific, setSpecific] = signal('')
  const [renderVersion, setRenderVersion] = signal(0)
  let request = 0

  const filtered = computed(() => {
    const query = search().trim().toLowerCase()
    const list = overview()?.migrations ?? []
    if (!query) return list
    return list.filter(
      (item) =>
        item.filename.toLowerCase().includes(query) ||
        item.name.toLowerCase().includes(query),
    )
  })

  async function load(projectId: string, connectionId?: string): Promise<void> {
    if (!projectId) {
      setOverview(null)
      return
    }
    const current = ++request
    setLoading(true)
    setError('')
    try {
      const result = await fetchOverview(projectId, connectionId)
      if (current === request) {
        setOverview(result)
        setRenderVersion(renderVersion() + 1)
      }
    } catch (cause) {
      if (current === request) {
        setError(cause instanceof Error ? cause.message : String(cause))
      }
    } finally {
      if (current === request) setLoading(false)
    }
  }

  effect(() => {
    const project = activeProject()
    const connection = activeConnection()
    void load(project?.id ?? '', connection?.id)
  })

  function reload(): void {
    const project = activeProject()
    const connection = activeConnection()
    if (project) void load(project.id, connection?.id)
  }

  function createMigration(): void {
    const project = activeProject()
    const connection = activeConnection()
    if (!project) return
    const split = (value: string): string[] =>
      value
        .split(',')
        .map((part) => part.trim())
        .filter(Boolean)
    void requestAction(
      {
        action: 'migration.create',
        projectId: project.id,
        connectionId: connection?.id,
        payload: {
          name: name().trim(),
          softDelete: softDelete(),
          triggers: triggers(),
          belongsTo: split(belongsTo()),
          oneToOne: split(oneToOne()),
          manyToMany: manyToMany().trim(),
        },
      },
      (result) => {
        const created = result.result.migrationFile
        setName('')
        setShowCreate(false)
        void reload()
        if (created) {
          void navigate(`/migrations/${encodeURIComponent(created)}`)
        }
      },
    )
  }

  function runMigrations(): void {
    const project = activeProject()
    const connection = activeConnection()
    if (!project) return
    void requestAction(
      {
        action: 'migrate.run',
        projectId: project.id,
        connectionId: connection?.id,
      },
      reload,
    )
  }

  function rollback(): void {
    const project = activeProject()
    const connection = activeConnection()
    if (!project) return
    void requestAction(
      {
        action: 'rollback.run',
        projectId: project.id,
        connectionId: connection?.id,
        payload: {
          steps: specific().trim() ? '' : steps().trim() || '1',
          migration: specific().trim(),
        },
      },
      reload,
    )
  }

  function fresh(): void {
    const project = activeProject()
    const connection = activeConnection()
    if (!project) return
    void requestAction(
      {
        action: 'fresh.run',
        projectId: project.id,
        connectionId: connection?.id,
      },
      reload,
    )
  }

  return (
    <div class="page">
      <header class="page-header">
        <div>
          <p class="eyebrow">SQL history</p>
          <h2>Migrations</h2>
          <p class="muted">
            Create a file, edit it, then let the confirmation plan show exactly
            what will run. Editing an executed <code>create_*_table</code> file
            applies additive changes through drift.
          </p>
        </div>
        <div class="row">
          <Button variant="outline" onClick={reload}>
            <RotateCcw aria-hidden />
            Re-check
          </Button>
          <Button onClick={() => setShowCreate(!showCreate())}>
            <Plus aria-hidden />
            New migration
          </Button>
        </div>
      </header>

      <Show
        when={() => {
          renderVersion()
          return activeProject() !== null
        }}
        fallback={
          <Empty
            icon={<LayoutList aria-hidden />}
            title="Select a project"
            description="Add and select a project before creating or running migrations."
            action={{
              label: 'Open projects',
              onClick: () => void navigate('/projects'),
            }}
          />
        }
      >
        {() => (
          <>
            <Show when={() => showCreate()}>
              <Card>
                <CardHeader>
                  <CardTitle>Scaffold a migration</CardTitle>
                  <CardDescription>
                    <code>vm make migration</code> creates the file. Review the
                    generated SQL in the editor before running it.
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <div class="grid grid-2">
                    <Field
                      label="Name"
                      htmlFor="migration-name"
                      required
                      hint="Use snake_case. create_ and _table are added automatically."
                    >
                      <Input
                        id="migration-name"
                        placeholder="posts"
                        value={() => name()}
                        onInput={(event) =>
                          setName((event.target as HTMLInputElement).value)
                        }
                      />
                    </Field>
                    <Field
                      label="Belongs to"
                      htmlFor="migration-belongs"
                      optional
                    >
                      <Input
                        id="migration-belongs"
                        placeholder="users, categories"
                        value={() => belongsTo()}
                        onInput={(event) =>
                          setBelongsTo((event.target as HTMLInputElement).value)
                        }
                      />
                    </Field>
                    <Field label="One to one" htmlFor="migration-oto" optional>
                      <Input
                        id="migration-oto"
                        placeholder="users"
                        value={() => oneToOne()}
                        onInput={(event) =>
                          setOneToOne((event.target as HTMLInputElement).value)
                        }
                      />
                    </Field>
                    <Field label="Many to many" htmlFor="migration-mm" optional>
                      <Input
                        id="migration-mm"
                        placeholder="roles"
                        value={() => manyToMany()}
                        onInput={(event) =>
                          setManyToMany(
                            (event.target as HTMLInputElement).value,
                          )
                        }
                      />
                    </Field>
                  </div>
                  <div class="row" style={{ 'margin-top': 'var(--space-4)' }}>
                    <Switch
                      checked={() => softDelete()}
                      onCheckedChange={setSoftDelete}
                      label="Soft delete"
                    />
                    <Switch
                      checked={() => triggers()}
                      onCheckedChange={setTriggers}
                      label="Updated-at trigger"
                    />
                  </div>
                  <div class="row" style={{ 'margin-top': 'var(--space-4)' }}>
                    <Button onClick={createMigration}>
                      <Plus aria-hidden />
                      Create file
                    </Button>
                    <Button
                      variant="ghost"
                      onClick={() => setShowCreate(false)}
                    >
                      Cancel
                    </Button>
                  </div>
                </CardContent>
              </Card>
            </Show>

            <Show when={() => Boolean(overview()?.statusError)}>
              <Alert variant="warning" title="Database status unavailable">
                {overview()?.statusError}
              </Alert>
            </Show>

            <Show when={() => error() !== ''}>
              <Alert variant="error" title="Could not load migrations">
                {error()}
              </Alert>
            </Show>

            <div class="grid grid-2">
              <Card>
                <CardHeader>
                  <CardTitle>Run</CardTitle>
                  <CardDescription>
                    {() =>
                      `${overview()?.pending ?? 0} pending · ${overview()?.executed ?? 0} executed`
                    }
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <div class="stack">
                    <div class="row">
                      <Button onClick={runMigrations}>
                        <Play aria-hidden />
                        Run pending
                      </Button>
                      <Button variant="outline" onClick={reload}>
                        Re-check safety
                      </Button>
                    </div>
                    <Alert variant="info" title="Drift-first rule">
                      Editing an executed create migration uses{' '}
                      <code>vm migrate --force --detect-drift</code>. A new
                      <code>add_*</code> migration is only suggested when drift
                      cannot apply the change safely.
                    </Alert>
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle>Rollback / rebuild</CardTitle>
                  <CardDescription>
                    Both actions require the exact project name in the
                    confirmation dialog.
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <div class="grid grid-2">
                    <Field label="Batches" htmlFor="rollback-steps" optional>
                      <Input
                        id="rollback-steps"
                        placeholder="1 or all"
                        value={() => steps()}
                        onInput={(event) =>
                          setSteps((event.target as HTMLInputElement).value)
                        }
                      />
                    </Field>
                    <Field
                      label="Specific migration"
                      htmlFor="rollback-specific"
                      optional
                      hint="Substring match; overrides batches."
                    >
                      <Input
                        id="rollback-specific"
                        placeholder="create_users_table"
                        value={() => specific()}
                        onInput={(event) =>
                          setSpecific((event.target as HTMLInputElement).value)
                        }
                      />
                    </Field>
                  </div>
                  <div class="row" style={{ 'margin-top': 'var(--space-4)' }}>
                    <Button variant="outline" onClick={rollback}>
                      <RotateCcw aria-hidden />
                      Roll back
                    </Button>
                    <Button variant="destructive" onClick={fresh}>
                      <Trash2 aria-hidden />
                      Fresh rebuild
                    </Button>
                  </div>
                </CardContent>
              </Card>
            </div>

            <Card>
              <CardHeader>
                <div class="row-between">
                  <div>
                    <CardTitle>Migration files</CardTitle>
                    <CardDescription>
                      {() =>
                        `${overview()?.migrationPath ?? ''} · ${overview()?.check?.migrations ?? 0} tracked file(s)`
                      }
                    </CardDescription>
                  </div>
                  <input
                    type="search"
                    placeholder="Filter migrations…"
                    aria-label="Filter migrations"
                    value={() => search()}
                    onInput={(event) =>
                      setSearch((event.target as HTMLInputElement).value)
                    }
                  />
                </div>
              </CardHeader>
              <CardContent>
                <Show when={() => loading() && overview() === null}>
                  <p class="muted">Loading migrations…</p>
                </Show>
                <Show
                  when={() => {
                    renderVersion()
                    return (overview()?.migrations.length ?? 0) > 0
                  }}
                  fallback={
                    <p class="muted">
                      No migration files yet. Create the first one above.
                    </p>
                  }
                >
                  {() => (
                    <div class="table-wrap">
                      <table class="data-table">
                        <thead>
                          <tr>
                            <th>Migration</th>
                            <th>Status</th>
                            <th class="hide-sm">Size</th>
                            <th class="hide-sm">Modified</th>
                          </tr>
                        </thead>
                        <tbody>
                          <For each={filtered} key={(item) => item.filename}>
                            {(item) => (
                              <tr>
                                <td>
                                  <a
                                    href={`/migrations/${encodeURIComponent(item.filename)}`}
                                    class="filename"
                                  >
                                    {migrationName(item.filename)}
                                  </a>
                                  <div class="muted small">{item.filename}</div>
                                </td>
                                <td>
                                  <MigrationBadge item={item} />
                                </td>
                                <td class="hide-sm">
                                  {formatBytes(item.size)}
                                </td>
                                <td class="hide-sm">
                                  {formatRelative(item.modifiedAt)}
                                </td>
                              </tr>
                            )}
                          </For>
                        </tbody>
                      </table>
                    </div>
                  )}
                </Show>
              </CardContent>
            </Card>
          </>
        )}
      </Show>
    </div>
  )
}
