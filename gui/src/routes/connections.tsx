import { signal } from '@vorzelajs/core'
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
import { Field } from '@/components/ui/field.tsx'
import { Input } from '@/components/ui/input.tsx'
import { Layers, Pencil, Plus, Trash2 } from '@/components/ui/icons.tsx'
import { formatRelative } from '@/lib/format.ts'
import { testConnection } from '@/lib/api.ts'
import {
  activeConnection,
  appState,
  refreshState,
  requestAction,
  setActiveConnection,
  showToast,
} from '@/lib/state.ts'
import type { Connection, TestConnectionResult } from '@/lib/api-types.ts'

export const Route = createFileRoute('/connections')({
  head: () => ({
    title: 'Connections · Vorzela Migrate',
    description: 'Saved database targets with encrypted passwords.',
  }),
  component: ConnectionsPage,
})

const ENGINE_PORTS: Record<string, number> = {
  postgres: 5432,
  mysql: 3306,
  mariadb: 3306,
}

function ConnectionsPage() {
  const [editingId, setEditingId] = signal('')
  const [name, setName] = signal('')
  const [engine, setEngine] = signal('postgres')
  const [host, setHost] = signal('127.0.0.1')
  const [port, setPort] = signal(5432)
  const [database, setDatabase] = signal('')
  const [username, setUsername] = signal('')
  const [password, setPassword] = signal('')
  const [sslMode, setSslMode] = signal('prefer')
  const [testing, setTesting] = signal(false)
  const [testResult, setTestResult] = signal<TestConnectionResult | null>(null)

  function reset(): void {
    setEditingId('')
    setName('')
    setEngine('postgres')
    setHost('127.0.0.1')
    setPort(5432)
    setDatabase('')
    setUsername('')
    setPassword('')
    setSslMode('prefer')
    setTestResult(null)
  }

  function edit(connection: Connection): void {
    setEditingId(connection.id)
    setName(connection.name)
    setEngine(connection.engine)
    setHost(connection.host)
    setPort(connection.port)
    setDatabase(connection.database)
    setUsername(connection.username)
    setPassword('')
    setSslMode(connection.sslMode || 'prefer')
    setTestResult(null)
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  function changeEngine(next: string): void {
    setEngine(next)
    setPort(ENGINE_PORTS[next] ?? 5432)
    if (next !== 'postgres') setSslMode('')
  }

  function payload() {
    return {
      id: editingId() || undefined,
      name: name().trim(),
      engine: engine(),
      host: host().trim(),
      port: Number(port()),
      database: database().trim(),
      username: username().trim(),
      password: password() || undefined,
      sslMode: engine() === 'postgres' ? sslMode().trim() : '',
    }
  }

  function save(): void {
    void requestAction(
      { action: 'connection.save', payload: payload() },
      () => {
        reset()
        void refreshState()
      },
    )
  }

  async function test(): Promise<void> {
    if (testing()) return
    setTesting(true)
    setTestResult(null)
    try {
      const result = await testConnection(payload())
      setTestResult(result)
      if (result.ok)
        showToast('success', 'Connection works', `${result.latencyMs} ms`)
      else showToast('error', 'Connection failed', result.error)
      void refreshState()
    } catch (cause) {
      setTestResult({
        ok: false,
        latencyMs: 0,
        error: cause instanceof Error ? cause.message : String(cause),
      })
    } finally {
      setTesting(false)
    }
  }

  function remove(connection: Connection): void {
    void requestAction(
      {
        action: 'connection.delete',
        connectionId: connection.id,
      },
      () => void refreshState(),
    )
  }

  return (
    <div class="page">
      <header class="page-header">
        <div>
          <p class="eyebrow">Database targets</p>
          <h2>Connections</h2>
          <p class="muted">
            Passwords are encrypted with a local AES-256-GCM key and never sent
            back to the browser. Every action shows the selected target before
            it runs.
          </p>
        </div>
        <Badge variant="secondary">
          {() => `${appState()?.connections.length ?? 0} saved`}
        </Badge>
      </header>

      <Card>
        <CardHeader>
          <div class="row-between">
            <div>
              <CardTitle>
                {() => (editingId() ? 'Edit connection' : 'Add connection')}
              </CardTitle>
              <CardDescription>
                The password is only sent when you save; test uses it
                in-process.
              </CardDescription>
            </div>
            <Show when={() => editingId() !== ''}>
              <Button variant="ghost" size="sm" onClick={reset}>
                Cancel edit
              </Button>
            </Show>
          </div>
        </CardHeader>
        <CardContent>
          <form
            onSubmit={(event) => {
              event.preventDefault()
              save()
            }}
          >
            <div class="grid grid-2">
              <Field label="Name" htmlFor="connection-name" required>
                <Input
                  id="connection-name"
                  placeholder="Local Postgres"
                  value={() => name()}
                  onInput={(event) =>
                    setName((event.target as HTMLInputElement).value)
                  }
                />
              </Field>
              <Field label="Engine" htmlFor="connection-engine" required>
                <select
                  id="connection-engine"
                  class="native-select"
                  value={() => engine()}
                  onChange={(event) =>
                    changeEngine((event.target as HTMLSelectElement).value)
                  }
                >
                  <option value="postgres">PostgreSQL</option>
                  <option value="mysql">MySQL</option>
                  <option value="mariadb">MariaDB</option>
                </select>
              </Field>
              <Field label="Host" htmlFor="connection-host" required>
                <Input
                  id="connection-host"
                  placeholder="127.0.0.1"
                  value={() => host()}
                  onInput={(event) =>
                    setHost((event.target as HTMLInputElement).value)
                  }
                />
              </Field>
              <Field label="Port" htmlFor="connection-port">
                <Input
                  id="connection-port"
                  type="number"
                  min="1"
                  max="65535"
                  value={() => port()}
                  onInput={(event) =>
                    setPort(Number((event.target as HTMLInputElement).value))
                  }
                />
              </Field>
              <Field label="Database" htmlFor="connection-database" required>
                <Input
                  id="connection-database"
                  placeholder="myapp_development"
                  value={() => database()}
                  onInput={(event) =>
                    setDatabase((event.target as HTMLInputElement).value)
                  }
                />
              </Field>
              <Field label="Username" htmlFor="connection-username" optional>
                <Input
                  id="connection-username"
                  autocomplete="username"
                  value={() => username()}
                  onInput={(event) =>
                    setUsername((event.target as HTMLInputElement).value)
                  }
                />
              </Field>
              <Field
                label="Password"
                htmlFor="connection-password"
                optional
                hint={
                  editingId()
                    ? 'Leave blank to keep the saved password.'
                    : 'Stored encrypted; never returned to the browser.'
                }
              >
                <Input
                  id="connection-password"
                  type="password"
                  autocomplete="current-password"
                  value={() => password()}
                  onInput={(event) =>
                    setPassword((event.target as HTMLInputElement).value)
                  }
                />
              </Field>
              <Show when={() => engine() === 'postgres'}>
                <Field label="SSL mode" htmlFor="connection-ssl">
                  <select
                    id="connection-ssl"
                    class="native-select"
                    value={() => sslMode()}
                    onChange={(event) =>
                      setSslMode((event.target as HTMLSelectElement).value)
                    }
                  >
                    <option value="prefer">prefer</option>
                    <option value="disable">disable</option>
                    <option value="require">require</option>
                    <option value="verify-ca">verify-ca</option>
                    <option value="verify-full">verify-full</option>
                  </select>
                </Field>
              </Show>
            </div>

            <Show when={() => testResult() !== null}>
              {() => {
                const result = testResult() as TestConnectionResult
                return (
                  <Alert
                    variant={result.ok ? 'success' : 'error'}
                    title={result.ok ? 'Connection works' : 'Connection failed'}
                    style={{ 'margin-top': 'var(--space-4)' }}
                  >
                    {result.ok
                      ? `Ping completed in ${result.latencyMs} ms. Saving still asks for confirmation.`
                      : result.error}
                  </Alert>
                )
              }}
            </Show>

            <div class="row" style={{ 'margin-top': 'var(--space-4)' }}>
              <Button type="submit">
                <Plus aria-hidden />
                {() => (editingId() ? 'Save changes' : 'Add connection')}
              </Button>
              <Button
                variant="outline"
                onClick={() => void test()}
                disabled={testing()}
              >
                {() => (testing() ? 'Testing…' : 'Test connection')}
              </Button>
              <span class="muted small">
                Test connects directly and does not run migrations.
              </span>
            </div>
          </form>
        </CardContent>
      </Card>

      <Show
        when={() => (appState()?.connections.length ?? 0) > 0}
        fallback={
          <Empty
            icon={<Layers aria-hidden />}
            title="No saved connections"
            description="Add a database target above. It is stored locally with an encrypted password."
          />
        }
      >
        {() => (
          <div class="grid grid-2">
            <For
              each={() => appState()?.connections ?? []}
              key={(item) => item.id}
            >
              {(connection) => (
                <Card>
                  <CardHeader>
                    <div class="row-between">
                      <div>
                        <CardTitle>
                          {connection.name}{' '}
                          <Show
                            when={() =>
                              activeConnection()?.id === connection.id
                            }
                          >
                            <Badge variant="primary" size="sm">
                              selected
                            </Badge>
                          </Show>
                        </CardTitle>
                        <CardDescription class="path">
                          {connection.dsn}
                        </CardDescription>
                      </div>
                      <Badge variant="outline">{connection.engine}</Badge>
                    </div>
                  </CardHeader>
                  <CardContent>
                    <div class="stack-sm">
                      <div class="row">
                        <span class="chip">
                          <strong>user</strong>{' '}
                          {connection.username || 'default'}
                        </span>
                        <span class="chip">
                          <strong>password</strong>{' '}
                          {connection.hasPassword ? 'saved' : 'not set'}
                        </span>
                        {connection.lastTestedAt ? (
                          <span class="chip">
                            <strong>test</strong>{' '}
                            {connection.lastTestOk ? 'ok' : 'failed'} ·{' '}
                            {formatRelative(connection.lastTestedAt)}
                          </span>
                        ) : null}
                      </div>
                      <div class="row">
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => setActiveConnection(connection.id)}
                        >
                          Use for actions
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => edit(connection)}
                        >
                          <Pencil aria-hidden />
                          Edit
                        </Button>
                        <Button
                          variant="destructive"
                          size="sm"
                          onClick={() => remove(connection)}
                        >
                          <Trash2 aria-hidden />
                          Delete
                        </Button>
                      </div>
                    </div>
                  </CardContent>
                </Card>
              )}
            </For>
          </div>
        )}
      </Show>
    </div>
  )
}
