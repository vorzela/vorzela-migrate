import type {
  ActionResult,
  AppState,
  AuditEntry,
  ExecutionResult,
  MigrationDetail,
  Plan,
  ProjectSchema,
  UpdateInfo,
  ProjectOverview,
  TestConnectionResult,
} from './api-types.ts'

let csrfToken = ''

export class ApiError extends Error {
  status: number
  payload: unknown

  constructor(message: string, status: number, payload: unknown) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.payload = payload
  }
}

function headersFor(method: string): Record<string, string> {
  const headers: Record<string, string> = {
    Accept: 'application/json',
    'X-VM-GUI': '1',
  }
  if (method !== 'GET' && method !== 'HEAD') {
    headers['Content-Type'] = 'application/json'
    if (csrfToken) headers['X-VM-CSRF'] = csrfToken
  }
  return headers
}

export async function request<T>(
  path: string,
  options: { method?: string; body?: unknown } = {},
): Promise<T> {
  const method = options.method ?? 'GET'
  const response = await fetch(path, {
    method,
    credentials: 'same-origin',
    headers: headersFor(method),
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
  })

  const text = await response.text()
  let payload: unknown = null
  if (text) {
    try {
      payload = JSON.parse(text)
    } catch {
      payload = { error: text }
    }
  }

  if (!response.ok) {
    const message =
      payload && typeof payload === 'object' && 'error' in payload
        ? String((payload as { error: unknown }).error)
        : `Request failed (${response.status})`
    if (response.status === 401) {
      window.dispatchEvent(new CustomEvent('vm-auth-lost'))
    }
    throw new ApiError(message, response.status, payload)
  }
  return payload as T
}

export function setCSRFToken(token: string): void {
  csrfToken = token
}

export function getCSRFToken(): string {
  return csrfToken
}

export async function loadAppState(): Promise<AppState> {
  const state = await request<AppState>('/api/state')
  setCSRFToken(state.csrfToken)
  return state
}

export async function exchangeToken(token: string): Promise<void> {
  await request<{ authenticated: boolean }>('/api/auth/token', {
    method: 'POST',
    body: { token },
  })
}

export async function logout(): Promise<void> {
  await request<{ ok: boolean }>('/api/auth/logout', { method: 'POST' })
}

export async function preparePlan(input: {
  action: string
  projectId?: string
  connectionId?: string
  payload?: unknown
}): Promise<Plan> {
  return request<Plan>('/api/plans', { method: 'POST', body: input })
}

export async function executePlan(
  planId: string,
  phrase = '',
): Promise<ActionResult> {
  return request<ActionResult>(
    `/api/plans/${encodeURIComponent(planId)}/execute`,
    {
      method: 'POST',
      body: { phrase },
    },
  )
}

export async function fetchOverview(
  projectId: string,
  connectionId?: string,
): Promise<ProjectOverview> {
  const query = connectionId
    ? `?connectionId=${encodeURIComponent(connectionId)}`
    : ''
  return request<ProjectOverview>(
    `/api/projects/${encodeURIComponent(projectId)}/overview${query}`,
  )
}

export async function fetchSchema(
  projectId: string,
  connectionId?: string,
): Promise<ProjectSchema> {
  const query = connectionId
    ? `?connectionId=${encodeURIComponent(connectionId)}`
    : ''
  return request<ProjectSchema>(
    `/api/projects/${encodeURIComponent(projectId)}/schema${query}`,
  )
}

export async function fetchMigration(
  projectId: string,
  filename: string,
  connectionId?: string,
): Promise<MigrationDetail> {
  const query = connectionId
    ? `?connectionId=${encodeURIComponent(connectionId)}`
    : ''
  return request<MigrationDetail>(
    `/api/projects/${encodeURIComponent(projectId)}/migrations/${encodeURIComponent(filename)}${query}`,
  )
}

export async function analyzeMigration(
  projectId: string,
  file: string,
  content: string,
  connectionId?: string,
): Promise<MigrationDetail> {
  const query = connectionId
    ? `?connectionId=${encodeURIComponent(connectionId)}`
    : ''
  return request<MigrationDetail>(
    `/api/projects/${encodeURIComponent(projectId)}/migrations/analyze${query}`,
    { method: 'POST', body: { file, content } },
  )
}

export async function testConnection(input: {
  id?: string
  name?: string
  engine?: string
  host?: string
  port?: number
  database?: string
  username?: string
  password?: string
  sslMode?: string
}): Promise<TestConnectionResult> {
  return request<TestConnectionResult>('/api/connections/test', {
    method: 'POST',
    body: input,
  })
}

export async function fetchUpdateCheck(force = true): Promise<UpdateInfo> {
  return request<UpdateInfo>(`/api/updates/check${force ? '?force=1' : ''}`)
}

export async function fetchActivity(limit = 100): Promise<AuditEntry[]> {
  const response = await request<{ entries: AuditEntry[] }>(
    `/api/activity?limit=${limit}`,
  )
  return response.entries
}

export function errorResult(error: unknown): ExecutionResult | null {
  if (!(error instanceof ApiError)) return null
  const payload = error.payload
  if (!payload || typeof payload !== 'object') return null
  const result = (payload as { result?: ExecutionResult }).result
  return result ?? null
}
