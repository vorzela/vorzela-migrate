export interface Settings {
  confirmationTtlSeconds: number
  sessionTtlMinutes: number
  preferRuntimeCredentials: boolean
}

export interface Project {
  id: string
  name: string
  path: string
  connectionId?: string
  hasVm: boolean
  createdAt: string
  updatedAt: string
}

export interface Connection {
  id: string
  name: string
  engine: 'postgres' | 'mysql' | 'mariadb' | string
  host: string
  port: number
  database: string
  username: string
  sslMode?: string
  hasPassword: boolean
  createdAt: string
  updatedAt: string
  lastTestedAt?: string
  lastTestOk?: boolean
  dsn: string
}

export interface AppState {
  version: string
  storeKind: 'yaml' | 'sqlite'
  storePath: string
  configPath: string
  activeProjectId?: string
  projects: Project[]
  connections: Connection[]
  settings: Settings
  vmPath: string
  sessionExpires: string
  csrfToken: string
}

export interface SafetyIssue {
  file?: string
  section?: string
  line?: number
  code: string
  severity: 'error' | 'warning' | 'info'
  message: string
  statement?: string
  destructive?: boolean
}

export interface CheckReport {
  path: string
  files: number
  migrations: number
  issues: SafetyIssue[]
  checked_files?: string[]
}

export interface StatusEntry {
  filename: string
  name: string
  timestamp?: number
  executed: boolean
  batch?: number
  checksum_ok?: boolean
  file_missing?: boolean
}

export interface StatusReport {
  path: string
  dialect: string
  executed: number
  pending: number
  missing_files: number
  entries: StatusEntry[]
  batches: number
  last_batch?: number
}

export interface MigrationListItem {
  filename: string
  name: string
  timestamp?: number
  executed: boolean
  batch?: number
  checksumOk?: boolean
  fileMissing?: boolean
  size: number
  modifiedAt: string
  hash: string
  statusKnown: boolean
}

export interface ProjectOverview {
  project: Project
  migrationPath: string
  dialect?: string
  status?: StatusReport
  statusError?: string
  check?: CheckReport
  checkError?: string
  migrations: MigrationListItem[]
  pending: number
  executed: number
  errors: number
  warnings: number
  statusKnown: boolean
}

export interface ColumnChange {
  table: string
  column: string
  type?: string
  definition?: string
  driftSafe?: boolean
  reason?: string
}

export interface FollowUp {
  table?: string
  column?: string
  name: string
  reason: string
}

export interface EditAnalysis {
  executed?: boolean
  checksumOk?: boolean
  verdict:
    | 'no_changes'
    | 'pending'
    | 'drift_safe'
    | 'needs_migration'
    | 'destructive'
    | 'unknown'
  summary: string
  addedTables?: string[]
  removedTables?: string[]
  addedColumns?: ColumnChange[]
  removedColumns?: ColumnChange[]
  modifiedColumns?: ColumnChange[]
  addedIndexes?: string[]
  removedIndexes?: string[]
  warnings?: string[]
  followUps?: FollowUp[]
}

export interface SchemaColumn {
  name: string
  type: string
  nullable: boolean
  default?: string
  primary: boolean
  unique: boolean
  foreignKey?: string
}

export interface SchemaTable {
  name: string
  columns: SchemaColumn[]
  columnCount: number
  migrationFile?: string
  executed: boolean
  fileMissing?: boolean
}

export interface ProjectSchema {
  project: Project
  dialect: string
  tables: SchemaTable[]
  database: string
  checkedAt: string
  statusKnown: boolean
}

export interface MigrationDetail {
  filename: string
  path: string
  content: string
  hash: string
  executed?: boolean
  batch?: number
  checksumOk?: boolean
  statusKnown: boolean
  size: number
  modifiedAt: string
  analysis?: EditAnalysis
}

export interface PlanTarget {
  projectId?: string
  projectName?: string
  projectPath?: string
  connectionId?: string
  connectionName?: string
  database?: string
  file?: string
}

export interface PlanPreview {
  kind: 'sql' | 'text' | 'diff'
  title?: string
  content?: string
  old?: string
  new?: string
}

export type PlanRisk = 'low' | 'medium' | 'high' | 'critical'

export interface Plan {
  id: string
  action: string
  title: string
  summary: string
  risk: PlanRisk
  requiresTypedConfirmation: boolean
  typingPhrase?: string
  command?: string
  target: PlanTarget
  warnings?: string[]
  preview?: PlanPreview
  analysis?: EditAnalysis
  expiresAt: string
  destructive: boolean
}

export interface ExecutionResult {
  ok: boolean
  action: string
  message?: string
  command?: string
  exitCode: number
  stdout?: string
  stderr?: string
  durationMs: number
  warnings?: string[]
  analysis?: EditAnalysis
  file?: string
  migrationFile?: string
}

export interface ActionResult {
  plan: Plan
  result: ExecutionResult
}

export interface AuditEntry {
  time: string
  action: string
  result: string
  project?: string
  connection?: string
  database?: string
  file?: string
  planId?: string
  detail?: string
  remote?: string
}

export interface UpdateInfo {
  current: string
  latest?: string
  available: boolean
  error?: string
  checkedAt: string
}

export interface TestConnectionResult {
  ok: boolean
  latencyMs: number
  error?: string
}
