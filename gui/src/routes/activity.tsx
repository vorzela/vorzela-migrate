import { effect, signal } from '@vorzelajs/core'
import { For, Show } from '@vorzelajs/dom'
import { createFileRoute } from '@vorzelajs/router'

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
import { Activity } from '@/components/ui/icons.tsx'
import { fetchActivity } from '@/lib/api.ts'
import { formatDate } from '@/lib/format.ts'
import type { AuditEntry } from '@/lib/api-types.ts'

export const Route = createFileRoute('/activity')({
  head: () => ({
    title: 'Activity · Vorzela Migrate',
    description: 'Audit trail of confirmations, runs and connection tests.',
  }),
  component: ActivityPage,
})

function resultVariant(
  result: string,
): 'success' | 'destructive' | 'warning' | 'secondary' {
  if (result === 'ok') return 'success'
  if (result === 'failed' || result === 'denied') return 'destructive'
  if (result === 'prepared') return 'warning'
  return 'secondary'
}

function ActivityPage() {
  const [entries, setEntries] = signal<AuditEntry[]>([])
  const [loading, setLoading] = signal(true)
  const [error, setError] = signal('')
  const [search, setSearch] = signal('')

  async function load(): Promise<void> {
    setLoading(true)
    setError('')
    try {
      setEntries(await fetchActivity(200))
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setLoading(false)
    }
  }

  effect(() => {
    void load()
  })

  const filtered = (): AuditEntry[] => {
    const query = search().trim().toLowerCase()
    if (!query) return entries()
    return entries().filter((entry) =>
      [
        entry.action,
        entry.result,
        entry.project,
        entry.database,
        entry.file,
        entry.detail,
      ]
        .filter(Boolean)
        .some((value) => String(value).toLowerCase().includes(query)),
    )
  }

  return (
    <div class="page">
      <header class="page-header">
        <div>
          <p class="eyebrow">Audit trail</p>
          <h2>Activity</h2>
          <p class="muted">
            Prepared confirmations, confirmed runs, connection tests and failed
            tokens are appended locally. Passwords are never written here.
          </p>
        </div>
        <Button variant="outline" onClick={() => void load()}>
          Refresh
        </Button>
      </header>

      <Show when={() => error() !== ''}>
        <Alert variant="error" title="Could not load activity">
          {error()}
        </Alert>
      </Show>

      <Card>
        <CardHeader>
          <div class="row-between">
            <div>
              <CardTitle>Recent events</CardTitle>
              <CardDescription>
                Newest first · {() => `${entries().length} event(s) in memory`}
              </CardDescription>
            </div>
            <input
              type="search"
              placeholder="Filter activity…"
              aria-label="Filter activity"
              value={() => search()}
              onInput={(event) =>
                setSearch((event.target as HTMLInputElement).value)
              }
            />
          </div>
        </CardHeader>
        <CardContent>
          <Show when={() => loading() && entries().length === 0}>
            <p class="muted">Loading activity…</p>
          </Show>
          <Show
            when={() => filtered().length > 0}
            fallback={
              <Empty
                icon={<Activity aria-hidden />}
                title="No activity yet"
                description="Confirmed actions and connection tests appear here."
              />
            }
          >
            <div class="table-wrap">
              <table class="data-table">
                <thead>
                  <tr>
                    <th>Time</th>
                    <th>Action</th>
                    <th>Result</th>
                    <th class="hide-sm">Project</th>
                    <th class="hide-sm">Database</th>
                    <th class="hide-sm">Detail</th>
                  </tr>
                </thead>
                <tbody>
                  <For
                    each={filtered}
                    key={(entry, index) =>
                      `${index}-${entry.time}-${entry.planId ?? ''}-${entry.action}-${entry.result}`
                    }
                  >
                    {(entry) => (
                      <tr>
                        <td class="small">{formatDate(entry.time)}</td>
                        <td>
                          <code>{entry.action}</code>
                          {entry.file ? (
                            <div class="muted small filename">{entry.file}</div>
                          ) : null}
                        </td>
                        <td>
                          <Badge variant={resultVariant(entry.result)}>
                            {entry.result}
                          </Badge>
                        </td>
                        <td class="hide-sm">{entry.project ?? '—'}</td>
                        <td class="hide-sm path">
                          {entry.database ?? entry.connection ?? '—'}
                        </td>
                        <td class="hide-sm small">{entry.detail ?? '—'}</td>
                      </tr>
                    )}
                  </For>
                </tbody>
              </table>
            </div>
          </Show>
        </CardContent>
      </Card>
    </div>
  )
}
