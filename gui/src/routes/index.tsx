import { effect, signal } from '@vorzelajs/core'
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
import { LayoutList, Play, Plus } from '@/components/ui/icons.tsx'
import { Skeleton } from '@/components/ui/skeleton.tsx'
import { fetchOverview } from '@/lib/api.ts'
import { formatRelative, migrationName } from '@/lib/format.ts'
import {
  activeConnection,
  activeProject,
  appState,
  requestAction,
} from '@/lib/state.ts'
import type { MigrationListItem, ProjectOverview } from '@/lib/api-types.ts'

export const Route = createFileRoute('/')({
  head: () => ({
    title: 'Dashboard · Vorzela Migrate',
    description: 'Project migration status, safety checks and quick actions.',
  }),
  component: Dashboard,
})

function StatusBadge({ item }: { item: MigrationListItem }) {
  if (item.fileMissing) return <Badge variant="destructive">file missing</Badge>
  if (!item.statusKnown) return <Badge variant="secondary">unknown</Badge>
  if (item.executed && item.checksumOk === false)
    return <Badge variant="destructive">checksum changed</Badge>
  if (item.executed) return <Badge variant="success">executed</Badge>
  return <Badge variant="warning">pending</Badge>
}

function Dashboard() {
  const navigate = useNavigate()
  const [overview, setOverview] = signal<ProjectOverview | null>(null)
  const [loading, setLoading] = signal(true)
  const [error, setError] = signal('')
  const [renderVersion, setRenderVersion] = signal(0)
  let request = 0

  async function load(projectId: string, connectionId?: string): Promise<void> {
    if (!projectId) {
      setOverview(null)
      setLoading(false)
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
        setOverview(null)
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
      () => void load(project.id, connection?.id),
    )
  }

  function rebuild(): void {
    const project = activeProject()
    const connection = activeConnection()
    if (!project) return
    void requestAction(
      {
        action: 'fresh.run',
        projectId: project.id,
        connectionId: connection?.id,
      },
      () => void load(project.id, connection?.id),
    )
  }

  return (
    <div class="page">
      <Show
        when={() => {
          renderVersion()
          return activeProject() !== null
        }}
        fallback={
          <Empty
            icon={<LayoutList aria-hidden />}
            title="No project yet"
            description="Add the folder that contains your vm.yaml file and migrations. The GUI edits files in place and never copies your project."
            action={{
              label: 'Add a project',
              onClick: () => void navigate('/projects'),
            }}
          />
        }
      >
        {() => {
          const project = activeProject()
          const connection = activeConnection()
          return (
            <>
              <section class="hero">
                <p class="eyebrow">Selected project</p>
                <h2>{() => project?.name ?? ''}</h2>
                <p class="muted path">{() => project?.path ?? ''}</p>
                <div class="hero-meta">
                  <span class="chip">
                    <strong>DB</strong>
                    {() => connection?.name ?? 'No connection selected'}
                  </span>
                  <span class="chip">
                    <strong>version</strong>{' '}
                    {() => appState()?.version ?? 'dev'}
                  </span>
                  <Show when={() => Boolean(connection?.dsn)}>
                    <span class="chip path">{() => connection?.dsn}</span>
                  </Show>
                  <Show when={() => Boolean(overview()?.dialect)}>
                    <span class="chip">
                      <strong>{() => overview()?.dialect}</strong>
                    </span>
                  </Show>
                  <Show
                    when={() => Boolean(project?.hasVm)}
                    fallback={<span class="chip">config missing</span>}
                  >
                    <span class="chip">config present</span>
                  </Show>
                </div>
                <div class="row" style={{ 'margin-top': 'var(--space-5)' }}>
                  <Button onClick={runMigrations}>
                    <Play aria-hidden />
                    Run pending
                  </Button>
                  <Button
                    variant="outline"
                    onClick={() => void navigate('/migrations')}
                  >
                    <LayoutList aria-hidden />
                    Review migrations
                  </Button>
                  <Button
                    variant="outline"
                    onClick={() => void navigate('/migrations')}
                  >
                    <Plus aria-hidden />
                    New migration
                  </Button>
                  <Button variant="destructive" onClick={rebuild}>
                    Fresh rebuild
                  </Button>
                </div>
              </section>

              <Show when={() => loading() && overview() === null}>
                <div class="grid grid-3">
                  <Skeleton />
                  <Skeleton />
                  <Skeleton />
                </div>
              </Show>

              <Show when={() => error() !== ''}>
                <Alert variant="error" title="Could not load project">
                  {error()}
                </Alert>
              </Show>

              <Show when={() => overview() !== null}>
                {() => {
                  const data = overview() as ProjectOverview
                  return (
                    <>
                      <div class="grid grid-3">
                        <div class="stat">
                          <span class="stat-label">Executed</span>
                          <span class="stat-value">{data.executed}</span>
                          <span class="stat-note">
                            {data.status?.batches ?? 0} batch(es)
                          </span>
                        </div>
                        <div class="stat">
                          <span class="stat-label">Pending</span>
                          <span class="stat-value">{data.pending}</span>
                          <span class="stat-note">
                            {data.pending === 0
                              ? 'Database history is caught up'
                              : 'Not applied to the selected DB'}
                          </span>
                        </div>
                        <div class="stat">
                          <span class="stat-label">Safety</span>
                          <span class="stat-value">
                            {data.errors === 0
                              ? 'Clean'
                              : `${data.errors} error(s)`}
                          </span>
                          <span class="stat-note">
                            {data.warnings} warning(s)
                          </span>
                        </div>
                      </div>

                      <Show when={() => Boolean(data.statusError)}>
                        <Alert
                          variant="warning"
                          title="Database status unavailable"
                        >
                          <p>
                            {data.statusError} Actions that change the database
                            stay disabled until the selected connection works.
                          </p>
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => void navigate('/connections')}
                          >
                            Open connections
                          </Button>
                        </Alert>
                      </Show>

                      <Show when={() => Boolean(data.checkError)}>
                        <Alert variant="warning" title="Static check failed">
                          {data.checkError}
                        </Alert>
                      </Show>

                      <div class="grid grid-2">
                        <Card>
                          <CardHeader>
                            <div class="row-between">
                              <div>
                                <CardTitle>Safety check</CardTitle>
                                <CardDescription>
                                  {data.check?.migrations ?? 0} migration
                                  file(s) checked before anything runs.
                                </CardDescription>
                              </div>
                              <Button
                                variant="outline"
                                size="sm"
                                onClick={() =>
                                  void load(project?.id ?? '', connection?.id)
                                }
                              >
                                Re-check
                              </Button>
                            </div>
                          </CardHeader>
                          <CardContent>
                            {data.check && data.check.issues.length > 0 ? (
                              <ul class="issue-list">
                                {data.check.issues.slice(0, 8).map((issue) => (
                                  <li
                                    class={`issue ${issue.severity}`}
                                    key={
                                      issue.code +
                                      (issue.file ?? '') +
                                      (issue.line ?? 0)
                                    }
                                  >
                                    <span>
                                      {issue.file ? (
                                        <strong>{issue.file}</strong>
                                      ) : null}{' '}
                                      {issue.message}
                                    </span>
                                  </li>
                                ))}
                              </ul>
                            ) : (
                              <p class="muted">
                                No static issues. Hard errors still block every
                                run.
                              </p>
                            )}
                          </CardContent>
                        </Card>

                        <Card>
                          <CardHeader>
                            <CardTitle>Why every action asks</CardTitle>
                            <CardDescription>
                              The server refuses writes without a confirmation
                              plan.
                            </CardDescription>
                          </CardHeader>
                          <CardContent>
                            <ul class="issue-list">
                              <li class="issue">
                                <span>
                                  <strong>Plan</strong> reviews the exact
                                  project, database, file and command.
                                </span>
                              </li>
                              <li class="issue">
                                <span>
                                  <strong>Drift first</strong> — editing an
                                  executed
                                  <code>create_*_table</code> file syncs with{' '}
                                  <code>vm migrate --force --detect-drift</code>
                                  .
                                </span>
                              </li>
                              <li class="issue">
                                <span>
                                  <strong>Follow-up only when necessary</strong>{' '}
                                  — NOT NULL, UNIQUE or FK columns that drift
                                  cannot backfill get an <code>add_*</code>{' '}
                                  migration.
                                </span>
                              </li>
                            </ul>
                          </CardContent>
                        </Card>
                      </div>

                      <Card>
                        <CardHeader>
                          <div class="row-between">
                            <div>
                              <CardTitle>Recent migrations</CardTitle>
                              <CardDescription>
                                {data.migrations.length} file(s) in{' '}
                                <code>{data.migrationPath}</code>
                              </CardDescription>
                            </div>
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => void navigate('/migrations')}
                            >
                              View all
                            </Button>
                          </div>
                        </CardHeader>
                        <CardContent>
                          {data.migrations.length === 0 ? (
                            <p class="muted">
                              No migration files yet. Create one from Projects
                              or Migrations.
                            </p>
                          ) : (
                            <div class="table-wrap">
                              <table class="data-table">
                                <thead>
                                  <tr>
                                    <th>Migration</th>
                                    <th>Status</th>
                                    <th class="hide-sm">Batch</th>
                                    <th class="hide-sm">Modified</th>
                                  </tr>
                                </thead>
                                <tbody>
                                  <For
                                    each={() =>
                                      (overview()?.migrations ?? []).slice(0, 6)
                                    }
                                    key={(item) => item.filename}
                                  >
                                    {(item) => (
                                      <tr>
                                        <td>
                                          <a
                                            href={`/migrations/${encodeURIComponent(item.filename)}`}
                                            class="filename"
                                          >
                                            {migrationName(item.filename)}
                                          </a>
                                          <div class="muted small">
                                            {item.filename}
                                          </div>
                                        </td>
                                        <td>
                                          <StatusBadge item={item} />
                                        </td>
                                        <td class="hide-sm">
                                          {item.batch ?? '—'}
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
                        </CardContent>
                      </Card>
                    </>
                  )
                }}
              </Show>
            </>
          )
        }}
      </Show>
    </div>
  )
}
