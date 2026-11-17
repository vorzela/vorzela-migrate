import { signal } from '@vorzelajs/core'
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
import { Folder, Plus, Trash2 } from '@/components/ui/icons.tsx'
import { Input } from '@/components/ui/input.tsx'
import { Switch } from '@/components/ui/switch.tsx'
import { formatRelative } from '@/lib/format.ts'
import {
  activeProject,
  appState,
  refreshState,
  requestAction,
  setActiveConnection,
  setActiveProject,
} from '@/lib/state.ts'
import type { Project } from '@/lib/api-types.ts'

export const Route = createFileRoute('/projects')({
  head: () => ({
    title: 'Projects · Vorzela Migrate',
    description: 'Add project folders and initialize vm.yaml configuration.',
  }),
  component: ProjectsPage,
})

function ProjectsPage() {
  const navigate = useNavigate()
  const [path, setPath] = signal('')
  const [name, setName] = signal('')
  const [connectionId, setConnectionId] = signal('')
  const [initId, setInitId] = signal('')
  const [writeDsn, setWriteDsn] = signal(false)
  const [migrationPath, setMigrationPath] = signal('./migrations')

  function addProject(): void {
    void requestAction(
      {
        action: 'project.add',
        payload: {
          path: path().trim(),
          name: name().trim(),
          connectionId: connectionId() || undefined,
        },
      },
      () => {
        setPath('')
        setName('')
        void refreshState()
      },
    )
  }

  function initProject(project: Project): void {
    void requestAction(
      {
        action: 'project.init',
        projectId: project.id,
        connectionId: project.connectionId || connectionId() || undefined,
        payload: {
          writeDsn: writeDsn(),
          migrationPath: migrationPath().trim() || './migrations',
          environment: 'development',
        },
      },
      () => {
        setInitId('')
        setWriteDsn(false)
        void refreshState()
      },
    )
  }

  function updateConnection(project: Project, next: string): void {
    if (next === (project.connectionId ?? '')) return
    void requestAction(
      {
        action: 'project.update',
        projectId: project.id,
        payload: { name: project.name, connectionId: next },
      },
      () => void refreshState(),
      () => void refreshState(),
    )
  }

  function removeProject(project: Project): void {
    void requestAction(
      { action: 'project.remove', projectId: project.id },
      () => void refreshState(),
    )
  }

  return (
    <div class="page">
      <header class="page-header">
        <div>
          <p class="eyebrow">Workspace</p>
          <h2>Projects</h2>
          <p class="muted">
            The GUI only edits migration files inside the project folder. It
            never copies or uploads your code.
          </p>
        </div>
        <Badge variant="secondary">
          {() => `${appState()?.projects.length ?? 0} project(s)`}
        </Badge>
      </header>

      <Show when={() => (appState()?.connections.length ?? 0) === 0}>
        <Alert variant="warning" title="Add a database connection first">
          <p>
            Connections keep the username and password (encrypted) and are
            selected per action. You can still add a project without one.
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

      <Card>
        <CardHeader>
          <CardTitle>Add project folder</CardTitle>
          <CardDescription>
            Paste an absolute path. The server resolves symlinks and refuses
            migration folders outside the project.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div class="grid grid-2">
            <Field label="Project folder" htmlFor="project-path">
              <Input
                id="project-path"
                placeholder="/home/you/code/my-app"
                value={() => path()}
                onInput={(event) =>
                  setPath((event.target as HTMLInputElement).value)
                }
              />
            </Field>
            <Field label="Display name" htmlFor="project-name" optional>
              <Input
                id="project-name"
                placeholder="my-app"
                value={() => name()}
                onInput={(event) =>
                  setName((event.target as HTMLInputElement).value)
                }
              />
            </Field>
            <Field
              label="Default connection"
              htmlFor="project-connection"
              optional
            >
              <select
                id="project-connection"
                class="native-select"
                value={() => connectionId()}
                onChange={(event) =>
                  setConnectionId((event.target as HTMLSelectElement).value)
                }
              >
                <option value="">None (choose later)</option>
                <For
                  each={() => appState()?.connections ?? []}
                  key={(item) => item.id}
                >
                  {(item) => (
                    <option
                      value={item.id}
                      selected={item.id === connectionId()}
                    >
                      {item.name}
                    </option>
                  )}
                </For>
              </select>
            </Field>
            <div class="row" style={{ 'align-items': 'flex-end' }}>
              <Button onClick={addProject}>
                <Plus aria-hidden />
                Add project
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      <Show
        when={() => (appState()?.projects.length ?? 0) > 0}
        fallback={
          <Empty
            icon={<Folder aria-hidden />}
            title="No projects yet"
            description="Add the folder that contains your vm.yaml file and migrations directory."
          />
        }
      >
        {() => (
          <div class="stack">
            <For
              each={() => appState()?.projects ?? []}
              key={(item) => item.id}
            >
              {(project) => (
                <Card>
                  <CardHeader>
                    <div class="row-between">
                      <div>
                        <CardTitle>
                          {project.name}{' '}
                          <Show when={() => activeProject()?.id === project.id}>
                            <Badge variant="primary" size="sm">
                              active
                            </Badge>
                          </Show>
                        </CardTitle>
                        <CardDescription class="path">
                          {project.path}
                        </CardDescription>
                      </div>
                      <div class="row">
                        <Badge variant={project.hasVm ? 'success' : 'warning'}>
                          {project.hasVm ? 'config present' : 'config missing'}
                        </Badge>
                        <Badge variant="secondary">
                          updated {formatRelative(project.updatedAt)}
                        </Badge>
                      </div>
                    </div>
                  </CardHeader>
                  <CardContent>
                    <div class="stack">
                      <div class="row">
                        <Button
                          variant={
                            activeProject()?.id === project.id
                              ? 'secondary'
                              : 'outline'
                          }
                          size="sm"
                          onClick={() => {
                            setActiveProject(project.id)
                            if (project.connectionId) {
                              setActiveConnection(project.connectionId)
                            }
                          }}
                        >
                          Use this project
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => {
                            setActiveProject(project.id)
                            if (project.connectionId) {
                              setActiveConnection(project.connectionId)
                            }
                            void navigate('/migrations')
                          }}
                        >
                          Migrations
                        </Button>
                        {!project.hasVm ? (
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() =>
                              setInitId(
                                initId() === project.id ? '' : project.id,
                              )
                            }
                          >
                            Initialize vm.yaml
                          </Button>
                        ) : null}
                        <Button
                          variant="destructive"
                          size="sm"
                          onClick={() => removeProject(project)}
                        >
                          <Trash2 aria-hidden />
                          Remove from GUI
                        </Button>
                      </div>

                      <div class="grid grid-2">
                        <Field
                          label="Default connection"
                          htmlFor={`project-conn-${project.id}`}
                          hint="Changing this requires a confirmation plan."
                        >
                          <select
                            id={`project-conn-${project.id}`}
                            class="native-select"
                            value={() => project.connectionId ?? ''}
                            onChange={(event) =>
                              updateConnection(
                                project,
                                (event.target as HTMLSelectElement).value,
                              )
                            }
                          >
                            <option value="">None</option>
                            <For
                              each={() => appState()?.connections ?? []}
                              key={(item) => item.id}
                            >
                              {(item) => (
                                <option
                                  value={item.id}
                                  selected={
                                    item.id === (project.connectionId ?? '')
                                  }
                                >
                                  {item.name}
                                </option>
                              )}
                            </For>
                          </select>
                        </Field>
                      </div>

                      <Show when={() => initId() === project.id}>
                        <div class="notice">
                          <div class="stack-sm">
                            <strong>Create vm.yaml</strong>
                            <Field
                              label="Migrations directory"
                              htmlFor={`project-migrations-${project.id}`}
                            >
                              <Input
                                id={`project-migrations-${project.id}`}
                                value={() => migrationPath()}
                                onInput={(event) =>
                                  setMigrationPath(
                                    (event.target as HTMLInputElement).value,
                                  )
                                }
                              />
                            </Field>
                            <Switch
                              checked={() => writeDsn()}
                              onCheckedChange={setWriteDsn}
                              label="Write DATABASE_URL into vm.yaml"
                              description="Off by default: vm gui injects the connection at runtime, so the password stays encrypted in the GUI store. Turn this on only if you also run vm in a terminal."
                            />
                            <Show when={() => writeDsn()}>
                              <Alert
                                variant="warning"
                                title="Plaintext warning"
                              >
                                The selected database password will be written
                                to <code>vm.yaml</code> with mode 0600.
                              </Alert>
                            </Show>
                            <div class="row">
                              <Button
                                size="sm"
                                onClick={() => initProject(project)}
                              >
                                Create vm.yaml
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => setInitId('')}
                              >
                                Cancel
                              </Button>
                            </div>
                          </div>
                        </div>
                      </Show>
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
