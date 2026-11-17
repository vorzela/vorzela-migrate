import { computed, effect, onCleanup, signal, untrack } from '@vorzelajs/core'
import { For, Show } from '@vorzelajs/dom'
import { createFileRoute, useNavigate, useParams } from '@vorzelajs/router'

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
import {
  Play,
  Plus,
  RotateCcw,
  FileUp,
  ShieldCheck,
} from '@/components/ui/icons.tsx'
import { Spinner } from '@/components/ui/spinner.tsx'
import { analyzeMigration, fetchMigration } from '@/lib/api.ts'
import {
  formatBytes,
  formatRelative,
  verdictLabel,
  verdictTone,
} from '@/lib/format.ts'
import { activeConnection, activeProject, requestAction } from '@/lib/state.ts'
import type { EditAnalysis, MigrationDetail } from '@/lib/api-types.ts'

export const Route = createFileRoute('/migrations/$name')({
  head: () => ({
    title: 'Edit migration · Vorzela Migrate',
    description: 'Edit SQL and apply additive changes with drift.',
  }),
  component: MigrationEditor,
})

function MigrationEditor() {
  const params = useParams<{ name: string }>()
  const navigate = useNavigate()
  const [detail, setDetail] = signal<MigrationDetail | null>(null)
  const [draft, setDraft] = signal('')
  const [analysis, setAnalysis] = signal<EditAnalysis | null>(null)
  const [loading, setLoading] = signal(true)
  const [analyzing, setAnalyzing] = signal(false)
  const [error, setError] = signal('')
  let analyzeTimer: ReturnType<typeof setTimeout> | null = null
  let request = 0

  const dirty = computed(() => {
    const value = detail()
    return value ? draft() !== value.content : false
  })

  async function load(force = false): Promise<void> {
    const project = activeProject()
    if (!project) {
      setError('Select a project first.')
      setLoading(false)
      return
    }
    if (!force && dirty() && !window.confirm('Discard unsaved changes?')) {
      return
    }
    const current = ++request
    setLoading(true)
    setError('')
    try {
      const value = await fetchMigration(
        project.id,
        params.name,
        activeConnection()?.id,
      )
      if (current !== request) return
      setDetail(value)
      setDraft(value.content)
      setAnalysis(value.analysis ?? null)
    } catch (cause) {
      if (current === request) {
        setError(cause instanceof Error ? cause.message : String(cause))
      }
    } finally {
      if (current === request) setLoading(false)
    }
  }

  function scheduleAnalysis(content: string): void {
    setDraft(content)
    if (analyzeTimer) clearTimeout(analyzeTimer)
    const project = activeProject()
    if (!project || !detail()) return
    setAnalyzing(true)
    analyzeTimer = setTimeout(() => {
      analyzeTimer = null
      void analyzeMigration(
        project.id,
        params.name,
        content,
        activeConnection()?.id,
      )
        .then((value) => setAnalysis(value.analysis ?? null))
        .catch(() => {
          // Keep the last successful analysis; save still validates on the server.
        })
        .finally(() => setAnalyzing(false))
    }, 450)
  }

  effect(() => {
    const project = activeProject()
    activeConnection()
    if (!project) return
    if (
      untrack(() => dirty()) &&
      !window.confirm('Switch project or database and discard unsaved changes?')
    ) {
      return
    }
    void load(true)
  })

  effect(() => {
    if (typeof window === 'undefined') return
    const handler = (event: BeforeUnloadEvent): void => {
      if (!dirty()) return
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', handler)
    onCleanup(() => window.removeEventListener('beforeunload', handler))
  })

  function save(): void {
    const value = detail()
    const project = activeProject()
    if (!value || !project || !dirty()) return
    if (analyzeTimer) {
      clearTimeout(analyzeTimer)
      analyzeTimer = null
    }
    void requestAction(
      {
        action: 'migration.save',
        projectId: project.id,
        connectionId: activeConnection()?.id,
        payload: {
          file: value.filename,
          content: draft(),
          baseHash: value.hash,
        },
      },
      async (result) => {
        await load(true)
        if (result.result.analysis) setAnalysis(result.result.analysis)
      },
    )
  }

  function applyDrift(): void {
    const value = detail()
    const project = activeProject()
    if (!value || !project) return
    void requestAction(
      {
        action: 'migration.apply_edits',
        projectId: project.id,
        connectionId: activeConnection()?.id,
        payload: { file: value.filename },
      },
      () => void load(true),
    )
  }

  function runPending(): void {
    const project = activeProject()
    if (!project) return
    void requestAction(
      {
        action: 'migrate.run',
        projectId: project.id,
        connectionId: activeConnection()?.id,
      },
      () => void load(true),
    )
  }

  function createFollowUp(name: string): void {
    const project = activeProject()
    if (!project) return
    void requestAction(
      {
        action: 'migration.create',
        projectId: project.id,
        connectionId: activeConnection()?.id,
        payload: { name },
      },
      (result) => {
        const created = result.result.migrationFile
        if (created) {
          void navigate(`/migrations/${encodeURIComponent(created)}`)
        } else {
          void load(true)
        }
      },
    )
  }

  function handleKeyDown(event: KeyboardEvent): void {
    const target = event.target as HTMLTextAreaElement
    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 's') {
      event.preventDefault()
      save()
      return
    }
    if (event.key === 'Tab' && target?.tagName === 'TEXTAREA') {
      event.preventDefault()
      const start = target.selectionStart ?? 0
      const end = target.selectionEnd ?? 0
      const next = `${draft().slice(0, start)}  ${draft().slice(end)}`
      scheduleAnalysis(next)
      queueMicrotask(() => {
        target.selectionStart = start + 2
        target.selectionEnd = start + 2
      })
    }
  }

  const value = detail()

  return (
    <div class="page">
      <header class="page-header">
        <div>
          <p class="eyebrow">Migration editor</p>
          <h2 class="filename">{() => detail()?.filename ?? params.name}</h2>
          <p class="muted">
            Edits are written only after a confirmation plan. Executed create
            files use drift; irreversible changes get a follow-up migration.
          </p>
        </div>
        <div class="row">
          <Button variant="outline" onClick={() => void load(false)}>
            <RotateCcw aria-hidden />
            Reload
          </Button>
          <Button onClick={save}>
            <FileUp aria-hidden />
            Save file
          </Button>
        </div>
      </header>

      <Show when={() => error() !== ''}>
        <Alert variant="error" title="Could not load migration">
          {error()}
        </Alert>
      </Show>

      <Show when={() => loading() && detail() === null}>
        <div class="row">
          <Spinner />
          <span class="muted">Loading migration…</span>
        </div>
      </Show>

      <Show when={() => detail() !== null}>
        {() => (
          <>
            <div class="row">
              <Show
                when={() => Boolean(detail()?.executed)}
                fallback={
                  <Show
                    when={() => Boolean(detail()?.statusKnown)}
                    fallback={<Badge variant="secondary">status unknown</Badge>}
                  >
                    <Badge variant="warning">pending</Badge>
                  </Show>
                }
              >
                <Badge variant="success">
                  executed{value?.batch ? ` · batch ${value.batch}` : ''}
                </Badge>
              </Show>
              <Show when={() => detail()?.checksumOk === false}>
                <Badge variant="destructive">changed after execution</Badge>
              </Show>
              <span class="chip">
                <strong>size</strong> {() => formatBytes(detail()?.size ?? 0)}
              </span>
              <span class="chip">
                <strong>updated</strong>{' '}
                {() => formatRelative(detail()?.modifiedAt)}
              </span>
              <Show when={() => dirty()}>
                <Badge variant="warning">unsaved</Badge>
              </Show>
              <Show when={() => analyzing()}>
                <Badge variant="secondary">analyzing…</Badge>
              </Show>
            </div>

            <Show
              when={() =>
                Boolean(detail()?.statusKnown) && detail()?.executed === false
              }
            >
              <Alert variant="info" title="This migration has not run yet">
                Save the file, then use <strong>Run pending</strong>. Drift is
                only needed after a migration has already executed.
              </Alert>
            </Show>

            <Show when={() => detail()?.statusKnown === false}>
              <Alert variant="warning" title="Database status unknown">
                Select a working connection to verify whether this file already
                ran. Saving still asks for confirmation.
              </Alert>
            </Show>

            <div class="editor-shell">
              <div class="stack-sm">
                <div class="editor-toolbar">
                  <div class="editor-meta">
                    <span>
                      <span class="kbd">Ctrl</span> + <span class="kbd">S</span>{' '}
                      save
                    </span>
                    <span>
                      <span class="kbd">Tab</span> inserts two spaces
                    </span>
                  </div>
                  <div class="row">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setDraft(detail()?.content ?? '')}
                    >
                      Revert draft
                    </Button>
                  </div>
                </div>
                <textarea
                  class="editor"
                  spellcheck={false}
                  aria-label="Migration SQL"
                  value={() => draft()}
                  onInput={(event) =>
                    scheduleAnalysis(
                      (event.target as HTMLTextAreaElement).value,
                    )
                  }
                  onKeyDown={handleKeyDown}
                />
              </div>

              <div class="stack">
                <Show
                  when={() => analysis() !== null}
                  fallback={
                    <div class="analysis">
                      <h4>Change analysis</h4>
                      <p class="muted">
                        Start editing to see whether drift can apply the change.
                      </p>
                    </div>
                  }
                >
                  {() => {
                    const current = analysis() as EditAnalysis
                    return (
                      <div class="analysis">
                        <div class="row-between">
                          <h4>Change analysis</h4>
                          <Badge variant={verdictTone(current.verdict)}>
                            {verdictLabel(current.verdict)}
                          </Badge>
                        </div>
                        <p class="muted">{current.summary}</p>

                        {current.addedColumns &&
                        current.addedColumns.length > 0 ? (
                          <div>
                            <p class="muted small">Added columns</p>
                            <ul class="change-list">
                              <For
                                each={() => analysis()?.addedColumns ?? []}
                                key={(item) => `${item.table}.${item.column}`}
                              >
                                {(column) => (
                                  <li>
                                    <code>
                                      {column.table}.{column.column}
                                    </code>{' '}
                                    <span class="muted">{column.type}</span>{' '}
                                    <Badge
                                      variant={
                                        column.driftSafe ? 'success' : 'warning'
                                      }
                                      size="sm"
                                    >
                                      {column.driftSafe
                                        ? 'drift safe'
                                        : 'follow-up'}
                                    </Badge>
                                    {column.reason ? (
                                      <div class="muted small">
                                        {column.reason}
                                      </div>
                                    ) : null}
                                  </li>
                                )}
                              </For>
                            </ul>
                          </div>
                        ) : null}

                        {current.removedColumns &&
                        current.removedColumns.length > 0 ? (
                          <div>
                            <p class="muted small">Removed columns</p>
                            <ul class="change-list">
                              <For
                                each={() => analysis()?.removedColumns ?? []}
                                key={(item) =>
                                  `removed.${item.table}.${item.column}`
                                }
                              >
                                {(column) => (
                                  <li>
                                    <code>
                                      {column.table}.{column.column}
                                    </code>{' '}
                                    <Badge variant="destructive" size="sm">
                                      destructive
                                    </Badge>
                                  </li>
                                )}
                              </For>
                            </ul>
                          </div>
                        ) : null}

                        {current.modifiedColumns &&
                        current.modifiedColumns.length > 0 ? (
                          <div>
                            <p class="muted small">Modified columns</p>
                            <ul class="change-list">
                              <For
                                each={() => analysis()?.modifiedColumns ?? []}
                                key={(item) =>
                                  `changed.${item.table}.${item.column}`
                                }
                              >
                                {(column) => (
                                  <li>
                                    <code>
                                      {column.table}.{column.column}
                                    </code>
                                    <div class="muted small">
                                      {column.reason}
                                    </div>
                                  </li>
                                )}
                              </For>
                            </ul>
                          </div>
                        ) : null}

                        {current.followUps && current.followUps.length > 0 ? (
                          <Alert
                            variant={
                              current.verdict === 'destructive'
                                ? 'error'
                                : 'warning'
                            }
                            title="Follow-up migration may be required"
                          >
                            <div class="stack-sm">
                              {current.followUps.map((follow) => (
                                <div key={follow.name} class="row-between">
                                  <span>
                                    <code>{follow.name}</code>
                                    <div class="muted small">
                                      {follow.reason}
                                    </div>
                                  </span>
                                  <Button
                                    variant="outline"
                                    size="sm"
                                    onClick={() => createFollowUp(follow.name)}
                                  >
                                    <Plus aria-hidden />
                                    Create
                                  </Button>
                                </div>
                              ))}
                            </div>
                          </Alert>
                        ) : null}
                      </div>
                    )
                  }}
                </Show>

                <Card>
                  <CardHeader>
                    <CardTitle>Apply</CardTitle>
                    <CardDescription>
                      Every button prepares a reviewable plan first.
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <div class="stack-sm">
                      <Show when={() => Boolean(detail()?.executed)}>
                        <Button variant="outline" onClick={applyDrift}>
                          <ShieldCheck aria-hidden />
                          Apply edit with drift
                        </Button>
                      </Show>
                      <Button variant="outline" onClick={runPending}>
                        <Play aria-hidden />
                        Run pending migrations
                      </Button>
                      <p class="muted small">
                        Drift never drops columns or tables. Those edits create
                        a destructive follow-up migration instead.
                      </p>
                    </div>
                  </CardContent>
                </Card>

                <Show
                  when={() =>
                    Boolean(value?.executed && value?.checksumOk === false)
                  }
                >
                  <Alert variant="warning" title="Checksum changed">
                    This executed file was edited. Applying changes uses{' '}
                    <code>--force --detect-drift</code> so the stored checksum
                    is updated and additive schema changes are applied.
                  </Alert>
                </Show>
              </div>
            </div>
          </>
        )}
      </Show>
    </div>
  )
}
