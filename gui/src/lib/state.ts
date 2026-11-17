import { computed, signal } from '@vorzelajs/core'

import {
  ApiError,
  errorResult,
  executePlan,
  loadAppState,
  preparePlan,
} from './api.ts'
import type {
  ActionResult,
  AppState,
  Connection,
  Plan,
  Project,
} from './api-types.ts'

const PROJECT_KEY = 'vm-gui.active-project'
const CONNECTION_KEY = 'vm-gui.active-connection'
const THEME_KEY = 'vm-gui.theme'

function readLocal(key: string): string {
  try {
    return localStorage.getItem(key) ?? ''
  } catch {
    return ''
  }
}

function writeLocal(key: string, value: string): void {
  try {
    if (value) localStorage.setItem(key, value)
    else localStorage.removeItem(key)
  } catch {
    // localStorage can be disabled; the GUI still works per page load.
  }
}

export const [appState, setAppState] = signal<AppState | null>(null)
export const [stateLoading, setStateLoading] = signal(true)
export const [activeProjectId, setActiveProjectIdSignal] = signal(
  readLocal(PROJECT_KEY),
)
export const [activeConnectionId, setActiveConnectionIdSignal] = signal(
  readLocal(CONNECTION_KEY),
)
export const [busy, setBusy] = signal(false)
export const [pendingPlan, setPendingPlan] = signal<Plan | null>(null)
export const [lastResult, setLastResult] = signal<ActionResult | null>(null)

export interface Toast {
  tone: 'success' | 'error' | 'warning' | 'info'
  title: string
  message?: string
}

export const [toast, setToast] = signal<Toast | null>(null)
let toastTimer: ReturnType<typeof setTimeout> | null = null

export function showToast(
  tone: Toast['tone'],
  title: string,
  message?: string,
): void {
  setToast({ tone, title, message })
  if (toastTimer) clearTimeout(toastTimer)
  toastTimer = setTimeout(() => {
    setToast(null)
    toastTimer = null
  }, 6500)
}

export function dismissToast(): void {
  if (toastTimer) clearTimeout(toastTimer)
  toastTimer = null
  setToast(null)
}

export function setActiveProject(id: string): void {
  setActiveProjectIdSignal(id)
  writeLocal(PROJECT_KEY, id)
}

export function setActiveConnection(id: string): void {
  setActiveConnectionIdSignal(id)
  writeLocal(CONNECTION_KEY, id)
}

export const activeProject = computed<Project | null>(() => {
  const state = appState()
  if (!state) return null
  const id = activeProjectId()
  const match = state.projects.find((project) => project.id === id)
  if (match) return match
  return state.projects[0] ?? null
})

export const activeConnection = computed<Connection | null>(() => {
  const state = appState()
  if (!state) return null
  const id = activeConnectionId()
  const selected = state.connections.find((connection) => connection.id === id)
  if (selected) return selected
  const project = activeProject()
  if (project?.connectionId) {
    const byProject = state.connections.find(
      (connection) => connection.id === project.connectionId,
    )
    if (byProject) return byProject
  }
  return state.connections[0] ?? null
})

export const hasProject = computed(() => (appState()?.projects.length ?? 0) > 0)

export function applyState(state: AppState): void {
  setAppState(state)
  if (state.projects.length === 0) {
    setActiveProject('')
    return
  }
  const projectId = activeProjectId()
  if (!state.projects.some((project) => project.id === projectId)) {
    setActiveProject(state.activeProjectId ?? state.projects[0].id)
  }
  const connectionId = activeConnectionId()
  if (
    connectionId &&
    !state.connections.some((connection) => connection.id === connectionId)
  ) {
    setActiveConnection('')
  }
}

export async function refreshState(): Promise<AppState | null> {
  try {
    const state = await loadAppState()
    applyState(state)
    return state
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) {
      window.dispatchEvent(new CustomEvent('vm-auth-lost'))
      return null
    }
    showToast('error', 'Could not refresh state', String(error))
    return null
  } finally {
    setStateLoading(false)
  }
}

export async function bootstrapState(): Promise<boolean> {
  setStateLoading(true)
  const state = await refreshState()
  return state !== null
}

let planCallback: ((result: ActionResult) => void | Promise<void>) | null = null
let planCancel: (() => void) | null = null

export interface PlanRequest {
  action: string
  projectId?: string
  connectionId?: string
  payload?: unknown
}

export async function requestAction(
  input: PlanRequest,
  onDone?: (result: ActionResult) => void | Promise<void>,
  onCancel?: () => void,
): Promise<Plan | null> {
  setBusy(true)
  try {
    const plan = await preparePlan(input)
    planCallback = onDone ?? null
    planCancel = onCancel ?? null
    setPendingPlan(plan)
    return plan
  } catch (error) {
    onCancel?.()
    const message =
      error instanceof Error ? error.message : 'Could not prepare the action'
    showToast('error', 'Action blocked', message)
    return null
  } finally {
    setBusy(false)
  }
}

export async function confirmPendingPlan(
  phrase = '',
): Promise<ActionResult | null> {
  const plan = pendingPlan()
  if (!plan) return null
  setBusy(true)
  try {
    const result = await executePlan(plan.id, phrase)
    setPendingPlan(null)
    setLastResult(result)
    planCallback?.(result)
    planCallback = null
    planCancel = null
    showToast(
      result.result.ok ? 'success' : 'error',
      result.result.ok ? 'Done' : 'Action failed',
      result.result.message ?? result.result.stderr ?? undefined,
    )
    return result
  } catch (error) {
    const result = errorResult(error)
    setPendingPlan(null)
    if (result) {
      const wrapped: ActionResult = { plan, result }
      setLastResult(wrapped)
    }
    planCallback = null
    planCancel?.()
    planCancel = null
    showToast(
      'error',
      'Action failed',
      error instanceof Error ? error.message : String(error),
    )
    return null
  } finally {
    setBusy(false)
  }
}

export function cancelPendingPlan(): void {
  setPendingPlan(null)
  planCallback = null
  planCancel?.()
  planCancel = null
}

export function dismissResult(): void {
  setLastResult(null)
}

export function setTheme(theme: 'light' | 'dark'): void {
  writeLocal(THEME_KEY, theme)
  if (typeof document !== 'undefined') {
    document.documentElement.classList.toggle('dark', theme === 'dark')
    document.documentElement.dataset.theme = theme
  }
}

export function initialTheme(): 'light' | 'dark' {
  const saved = readLocal(THEME_KEY)
  if (saved === 'light' || saved === 'dark') return saved
  if (
    typeof window !== 'undefined' &&
    window.matchMedia?.('(prefers-color-scheme: dark)').matches
  ) {
    return 'dark'
  }
  return 'light'
}
