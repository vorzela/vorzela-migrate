import { computed, effect, signal } from '@vorzelajs/core'
import { For, Show } from '@vorzelajs/dom'
import { createFileRoute, useNavigate } from '@vorzelajs/router'

import { Alert } from '@/components/ui/alert.tsx'
import { AlertDialog } from '@/components/ui/alert-dialog.tsx'
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
import { Plus, Table2 } from '@/components/ui/icons.tsx'
import { SimpleDataTable } from '@/components/ui/simple-data-table.tsx'
import { Switch } from '@/components/ui/switch.tsx'
import type { AppColumnDef } from '@/lib/table.ts'
import type {
  ProjectSchema,
  SchemaColumn,
  SchemaTable,
} from '@/lib/api-types.ts'
import { fetchSchema } from '@/lib/api.ts'
import { formatRelative } from '@/lib/format.ts'
import { activeConnection, activeProject, requestAction } from '@/lib/state.ts'

export const Route = createFileRoute('/tables')({
  head: () => ({
    title: 'Tables · Vorzela Migrate',
    description: 'Live tables, columns and migration coverage.',
  }),
  component: TablesPage,
})

const POSTGRES_TYPES = [
  'BIGSERIAL',
  'BIGINT',
  'INTEGER',
  'SMALLINT',
  'VARCHAR(255)',
  'TEXT',
  'BOOLEAN',
  'TIMESTAMPTZ',
  'DATE',
  'UUID',
  'JSONB',
  'NUMERIC(10,2)',
]

const MYSQL_TYPES = [
  'BIGINT',
  'INT',
  'SMALLINT',
  'VARCHAR(255)',
  'TEXT',
  'BOOLEAN',
  'TIMESTAMP',
  'DATETIME',
  'DATE',
  'JSON',
  'CHAR(36)',
  'DECIMAL(10,2)',
]

function TablesPage() {
  const navigate = useNavigate()
  const [schema, setSchema] = signal<ProjectSchema | null>(null)
  const [loading, setLoading] = signal(false)
  const [error, setError] = signal('')
  const [selectedName, setSelectedName] = signal('')
  const [showAddTable, setShowAddTable] = signal(false)
  const [columnTarget, setColumnTarget] = signal<SchemaTable | null>(null)
  let request = 0

  // Add table form
  const [tableName, setTableName] = signal('')
  const [belongsTo, setBelongsTo] = signal('')
  const [oneToOne, setOneToOne] = signal('')
  const [manyToMany, setManyToMany] = signal('')
  const [softDelete, setSoftDelete] = signal(false)
  const [triggers, setTriggers] = signal(false)

  // Add column form
  const [columnName, setColumnName] = signal('')
  const [columnType, setColumnType] = signal('VARCHAR(255)')
  const [columnNullable, setColumnNullable] = signal(true)
  const [columnDefault, setColumnDefault] = signal('')
  const [columnUnique, setColumnUnique] = signal(false)
  const [refTable, setRefTable] = signal('')
  const [refColumn, setRefColumn] = signal('')
  const [onDelete, setOnDelete] = signal('')
  const [backfill, setBackfill] = signal('')

  const selectedTable = computed(() => {
    const current = schema()
    if (!current) return null
    const name = selectedName()
    return current.tables.find((table) => table.name === name) ?? null
  })

  const typeSuggestions = computed(() =>
    schema()?.dialect === 'postgres' ? POSTGRES_TYPES : MYSQL_TYPES,
  )

  async function loadSchema(
    projectId: string,
    connectionId?: string,
  ): Promise<void> {
    if (!projectId || !connectionId) {
      setSchema(null)
      setLoading(false)
      return
    }
    const current = ++request
    setLoading(true)
    setError('')
    try {
      const result = await fetchSchema(projectId, connectionId)
      if (current !== request) return
      setSchema(result)
      const selected = selectedName()
      if (!result.tables.some((table) => table.name === selected)) {
        setSelectedName(result.tables[0]?.name ?? '')
      }
    } catch (cause) {
      if (current === request) {
        setError(cause instanceof Error ? cause.message : String(cause))
        setSchema(null)
      }
    } finally {
      if (current === request) setLoading(false)
    }
  }

  effect(() => {
    const project = activeProject()
    const connection = activeConnection()
    void loadSchema(project?.id ?? '', connection?.id)
  })

  function reload(): void {
    const project = activeProject()
    const connection = activeConnection()
    if (project && connection) void loadSchema(project.id, connection.id)
  }

  function openAddTable(): void {
    setTableName('')
    setBelongsTo('')
    setOneToOne('')
    setManyToMany('')
    setSoftDelete(false)
    setTriggers(false)
    setShowAddTable(true)
  }

  function splitTables(value: string): string[] {
    return value
      .split(',')
      .map((part) => part.trim())
      .filter(Boolean)
  }

  async function createTable(): Promise<void> {
    const project = activeProject()
    const connection = activeConnection()
    if (!project) return
    const plan = await requestAction(
      {
        action: 'migration.create',
        projectId: project.id,
        connectionId: connection?.id,
        payload: {
          name: tableName().trim(),
          softDelete: softDelete(),
          triggers: triggers(),
          belongsTo: splitTables(belongsTo()),
          oneToOne: splitTables(oneToOne()),
          manyToMany: manyToMany().trim(),
        },
      },
      (result) => {
        setShowAddTable(false)
        const created = result.result.migrationFile
        if (created) {
          void navigate(`/migrations/${encodeURIComponent(created)}`)
        } else {
          reload()
        }
      },
    )
    if (plan) setShowAddTable(false)
  }

  function openColumn(table: SchemaTable): void {
    setColumnTarget(table)
    setColumnName('')
    setColumnType(
      schema()?.dialect === 'postgres' ? 'VARCHAR(255)' : 'VARCHAR(255)',
    )
    setColumnNullable(true)
    setColumnDefault('')
    setColumnUnique(false)
    setRefTable('')
    setRefColumn('')
    setOnDelete('')
    setBackfill('')
  }

  async function submitColumn(): Promise<void> {
    const target = columnTarget()
    const project = activeProject()
    const connection = activeConnection()
    if (!target || !project) return
    const plan = await requestAction(
      {
        action: 'column.add',
        projectId: project.id,
        connectionId: connection?.id,
        payload: {
          table: target.name,
          name: columnName().trim(),
          type: columnType().trim(),
          nullable: columnNullable(),
          default: columnDefault().trim(),
          unique: columnUnique(),
          referencesTable: refTable().trim(),
          referencesColumn: refColumn().trim(),
          onDelete: onDelete(),
          backfill: backfill().trim(),
        },
      },
      (result) => {
        setColumnTarget(null)
        const created = result.result.migrationFile
        if (created) {
          void navigate(`/migrations/${encodeURIComponent(created)}`)
        } else {
          reload()
        }
      },
    )
    if (plan) setColumnTarget(null)
  }

  const tableColumns: AppColumnDef<SchemaTable>[] = [
    {
      accessorKey: 'name',
      header: 'Table',
      cell: ({ row }) => (
        <Button
          variant="link"
          size="sm"
          onClick={() => setSelectedName(row.original.name)}
        >
          {row.original.name}
        </Button>
      ),
    },
    {
      accessorKey: 'columnCount',
      header: 'Columns',
    },
    {
      accessorKey: 'migrationFile',
      header: 'Create migration',
      cell: ({ row }) =>
        row.original.migrationFile ? (
          <a
            class="filename"
            href={`/migrations/${encodeURIComponent(row.original.migrationFile)}`}
          >
            {row.original.migrationFile}
          </a>
        ) : (
          <span class="muted">—</span>
        ),
    },
    {
      accessorKey: 'executed',
      header: 'Migration status',
      cell: ({ row }) =>
        row.original.migrationFile ? (
          <Badge
            variant={
              schema()?.statusKnown
                ? row.original.executed
                  ? 'success'
                  : 'warning'
                : 'secondary'
            }
          >
            {schema()?.statusKnown
              ? row.original.executed
                ? 'executed'
                : 'pending'
              : 'unknown'}
          </Badge>
        ) : (
          <Badge variant="secondary">no create file</Badge>
        ),
    },
    {
      id: 'actions',
      header: '',
      enableSorting: false,
      cell: ({ row }) => (
        <Button
          variant="outline"
          size="sm"
          onClick={() => openColumn(row.original)}
        >
          <Plus aria-hidden />
          Add column
        </Button>
      ),
    },
  ]

  const columnColumns: AppColumnDef<SchemaColumn>[] = [
    {
      accessorKey: 'name',
      header: 'Column',
      cell: ({ row }) => <code>{row.original.name}</code>,
    },
    {
      accessorKey: 'type',
      header: 'Type',
      cell: ({ row }) => <span class="filename">{row.original.type}</span>,
    },
    {
      accessorKey: 'nullable',
      header: 'Nullable',
      cell: ({ row }) =>
        row.original.nullable ? (
          <Badge variant="secondary">NULL</Badge>
        ) : (
          <Badge variant="outline">NOT NULL</Badge>
        ),
    },
    {
      accessorKey: 'default',
      header: 'Default',
      cell: ({ row }) => (
        <span class="filename muted">{row.original.default ?? '—'}</span>
      ),
    },
    {
      id: 'keys',
      header: 'Keys',
      enableSorting: false,
      cell: ({ row }) => (
        <span class="row">
          {row.original.primary ? (
            <Badge variant="primary" size="sm">
              PK
            </Badge>
          ) : null}
          {row.original.unique ? (
            <Badge variant="secondary" size="sm">
              unique
            </Badge>
          ) : null}
          {row.original.foreignKey ? (
            <Badge variant="outline" size="sm">
              FK → {row.original.foreignKey}
            </Badge>
          ) : null}
          {!row.original.primary &&
          !row.original.unique &&
          !row.original.foreignKey ? (
            <span class="muted">—</span>
          ) : null}
        </span>
      ),
    },
  ]

  const project = activeProject()
  const connection = activeConnection()

  return (
    <div class="page">
      <header class="page-header">
        <div>
          <p class="eyebrow">Schema</p>
          <h2>Tables</h2>
          <p class="muted">
            Live tables from the selected database, their columns and the
            migration that created them. Add table uses{' '}
            <code>vm make migration</code>; Add column uses the drift-first
            rule.
          </p>
        </div>
        <div class="row">
          <Button
            variant="outline"
            onClick={() => void navigate('/migrations')}
          >
            All migrations
          </Button>
          <Button onClick={reload}>
            <Table2 aria-hidden />
            Refresh schema
          </Button>
        </div>
      </header>

      <Show when={() => !project}>
        <Empty
          icon={<Table2 aria-hidden />}
          title="Select a project"
          description="Add and select a project before inspecting tables."
          action={{
            label: 'Open projects',
            onClick: () => void navigate('/projects'),
          }}
        />
      </Show>

      <Show when={() => Boolean(project) && !connection}>
        <Alert variant="warning" title="Select a database connection">
          <p>
            The Tables page reads the live schema. Choose a connection in the
            topbar or add one first.
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

      <Show when={() => error() !== ''}>
        <Alert variant="error" title="Could not read the schema">
          {error()}
        </Alert>
      </Show>

      <Show when={() => Boolean(schema()) && schema()?.statusKnown === false}>
        <Alert variant="warning" title="Migration status unknown">
          Tables are readable, but the add-column decision needs a working
          database connection and a readable <code>migrations</code> table.
        </Alert>
      </Show>

      <Show when={() => Boolean(schema())}>
        {() => {
          const current = schema() as ProjectSchema
          return (
            <Card>
              <CardHeader>
                <div class="row-between">
                  <div>
                    <CardTitle>Database tables</CardTitle>
                    <CardDescription>
                      {current.database} · {current.tables.length} table(s) ·
                      refreshed {formatRelative(current.checkedAt)}
                    </CardDescription>
                  </div>
                </div>
              </CardHeader>
              <CardContent>
                <SimpleDataTable
                  data={() => schema()?.tables ?? []}
                  columns={tableColumns}
                  loading={() => loading()}
                  searchPlaceholder="Search tables…"
                  initialPageSize={10}
                  emptyMessage="No tables found in the selected database."
                  toolbarActions={
                    <div class="row">
                      <Button size="sm" onClick={openAddTable}>
                        <Plus aria-hidden />
                        Add table
                      </Button>
                    </div>
                  }
                />
              </CardContent>
            </Card>
          )
        }}
      </Show>

      <Show
        when={() => selectedTable() !== null}
        fallback={
          <Empty
            icon={<Table2 aria-hidden />}
            title="Select a table"
            description="Choose a table above to inspect its columns."
          />
        }
      >
        {() => {
          const table = selectedTable() as SchemaTable
          return (
            <Card>
              <CardHeader>
                <div class="row-between">
                  <div>
                    <CardTitle>
                      <code>{table.name}</code>{' '}
                      {table.migrationFile ? (
                        <Badge
                          variant={
                            schema()?.statusKnown
                              ? table.executed
                                ? 'success'
                                : 'warning'
                              : 'secondary'
                          }
                        >
                          {schema()?.statusKnown
                            ? table.executed
                              ? 'executed'
                              : 'pending'
                            : 'unknown'}
                        </Badge>
                      ) : (
                        <Badge variant="secondary">no create file</Badge>
                      )}
                    </CardTitle>
                    <CardDescription>
                      {table.columnCount} column(s)
                      {table.migrationFile ? (
                        <>
                          {' · '}
                          <a
                            href={`/migrations/${encodeURIComponent(table.migrationFile)}`}
                          >
                            {table.migrationFile}
                          </a>
                        </>
                      ) : null}
                    </CardDescription>
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => openColumn(table)}
                  >
                    <Plus aria-hidden />
                    Add column
                  </Button>
                </div>
              </CardHeader>
              <CardContent>
                <SimpleDataTable
                  data={() => selectedTable()?.columns ?? []}
                  columns={columnColumns}
                  loading={() => loading()}
                  searchPlaceholder="Search columns…"
                  pagination={false}
                  emptyMessage="No columns found."
                />
              </CardContent>
            </Card>
          )
        }}
      </Show>

      <Show when={() => showAddTable()}>
        {() => (
          <AlertDialog
            open={() => true}
            onOpenChange={(open) => {
              if (!open) setShowAddTable(false)
            }}
            className="plan-dialog"
            title="Add table"
            description="Scaffold a new table with vm make migration. You review and run the generated SQL from the editor."
            cancelText="Cancel"
            actionText="Create migration"
            onCancel={() => setShowAddTable(false)}
            onAction={() => void createTable()}
          >
            <div class="stack">
              <Field label="Table name" htmlFor="new-table-name" required>
                <Input
                  id="new-table-name"
                  placeholder="posts"
                  value={() => tableName()}
                  onInput={(event) =>
                    setTableName((event.target as HTMLInputElement).value)
                  }
                />
              </Field>
              <Field label="Belongs to" htmlFor="new-table-belongs" optional>
                <Input
                  id="new-table-belongs"
                  placeholder="users, categories"
                  value={() => belongsTo()}
                  onInput={(event) =>
                    setBelongsTo((event.target as HTMLInputElement).value)
                  }
                />
              </Field>
              <div class="grid grid-2">
                <Field label="One to one" htmlFor="new-table-oto" optional>
                  <Input
                    id="new-table-oto"
                    placeholder="users"
                    value={() => oneToOne()}
                    onInput={(event) =>
                      setOneToOne((event.target as HTMLInputElement).value)
                    }
                  />
                </Field>
                <Field
                  label="Many to many"
                  htmlFor="new-table-mm"
                  optional
                  hint="Cannot be combined with belongs-to/one-to-one."
                >
                  <Input
                    id="new-table-mm"
                    placeholder="roles"
                    value={() => manyToMany()}
                    onInput={(event) =>
                      setManyToMany((event.target as HTMLInputElement).value)
                    }
                  />
                </Field>
              </div>
              <div class="row">
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
            </div>
          </AlertDialog>
        )}
      </Show>

      <Show when={() => columnTarget() !== null}>
        {() => {
          const target = columnTarget() as SchemaTable
          return (
            <AlertDialog
              open={() => true}
              onOpenChange={(open) => {
                if (!open) setColumnTarget(null)
              }}
              className="plan-dialog"
              title={`Add column to ${target.name}`}
              description="Drift handles safe additive columns. NOT NULL without a default, NOT NULL UNIQUE and NOT NULL foreign keys become a reviewed follow-up migration."
              cancelText="Cancel"
              actionText="Review plan"
              onCancel={() => setColumnTarget(null)}
              onAction={() => void submitColumn()}
            >
              <div class="stack">
                <Show
                  when={() => Boolean(target.migrationFile) && target.executed}
                  fallback={
                    <Alert variant="info" title="Migration path">
                      {target.migrationFile
                        ? 'The create migration is pending. The column is written to that file and applied by Run pending.'
                        : 'No create_*_table migration was found. A follow-up add_* migration will be created.'}
                    </Alert>
                  }
                >
                  <Alert variant="info" title="Drift-first path">
                    The create migration is executed. Safe columns are added to{' '}
                    <code>{target.migrationFile}</code> and applied with{' '}
                    <code>vm migrate --force --detect-drift</code>.
                  </Alert>
                </Show>

                <Field label="Column name" htmlFor="new-column-name" required>
                  <Input
                    id="new-column-name"
                    placeholder="email"
                    value={() => columnName()}
                    onInput={(event) =>
                      setColumnName((event.target as HTMLInputElement).value)
                    }
                  />
                </Field>

                <Field
                  label="Type"
                  htmlFor="new-column-type"
                  required
                  hint="Simple SQL types. Edit the migration for enum types or complex defaults."
                >
                  <Input
                    id="new-column-type"
                    list="column-type-options"
                    value={() => columnType()}
                    onInput={(event) =>
                      setColumnType((event.target as HTMLInputElement).value)
                    }
                  />
                </Field>
                <datalist id="column-type-options">
                  <For each={() => typeSuggestions()} key={(type) => type}>
                    {(type) => <option value={type} />}
                  </For>
                </datalist>

                <div class="row">
                  <Switch
                    checked={() => columnNullable()}
                    onCheckedChange={setColumnNullable}
                    label="Nullable"
                    description="Nullable columns are drift-safe; NOT NULL may need a backfill."
                  />
                  <Switch
                    checked={() => columnUnique()}
                    onCheckedChange={setColumnUnique}
                    label="Unique"
                  />
                </div>

                <div class="grid grid-2">
                  <Field
                    label="Default"
                    htmlFor="new-column-default"
                    optional
                    hint="Literal or supported function."
                  >
                    <Input
                      id="new-column-default"
                      placeholder="CURRENT_TIMESTAMP"
                      value={() => columnDefault()}
                      onInput={(event) =>
                        setColumnDefault(
                          (event.target as HTMLInputElement).value,
                        )
                      }
                    />
                  </Field>
                  <Show when={() => !columnNullable()}>
                    <Field
                      label="Backfill"
                      htmlFor="new-column-backfill"
                      optional
                      hint="Used by the follow-up migration. Defaults to the DEFAULT value."
                    >
                      <Input
                        id="new-column-backfill"
                        placeholder="0 or id or gen_random_uuid()"
                        value={() => backfill()}
                        onInput={(event) =>
                          setBackfill((event.target as HTMLInputElement).value)
                        }
                      />
                    </Field>
                  </Show>
                </div>

                <div class="grid grid-2">
                  <Field
                    label="References table"
                    htmlFor="new-column-ref-table"
                    optional
                  >
                    <Input
                      id="new-column-ref-table"
                      placeholder="users"
                      value={() => refTable()}
                      onInput={(event) =>
                        setRefTable((event.target as HTMLInputElement).value)
                      }
                    />
                  </Field>
                  <Field
                    label="References column"
                    htmlFor="new-column-ref-column"
                    optional
                  >
                    <Input
                      id="new-column-ref-column"
                      placeholder="id"
                      value={() => refColumn()}
                      onInput={(event) =>
                        setRefColumn((event.target as HTMLInputElement).value)
                      }
                    />
                  </Field>
                </div>

                <Show when={() => refTable().trim() !== ''}>
                  <Field
                    label="On delete"
                    htmlFor="new-column-on-delete"
                    optional
                  >
                    <select
                      id="new-column-on-delete"
                      class="native-select"
                      value={() => onDelete()}
                      onChange={(event) =>
                        setOnDelete((event.target as HTMLSelectElement).value)
                      }
                    >
                      <option value="">NO ACTION</option>
                      <option value="CASCADE">CASCADE</option>
                      <option value="RESTRICT">RESTRICT</option>
                      <option value="SET NULL">SET NULL</option>
                      <option value="SET DEFAULT">SET DEFAULT</option>
                    </select>
                  </Field>
                </Show>

                <p class="muted small">
                  The confirmation plan shows the exact file diff or the
                  generated follow-up SQL before anything is written.
                </p>
              </div>
            </AlertDialog>
          )
        }}
      </Show>
    </div>
  )
}
