import { signal } from '@vorzelajs/core'
import { For, Show } from '@vorzelajs/dom'
import { createRootRoute, useLocation } from '@vorzelajs/router'

import { PlanDialog, ResultDialog } from '@/components/plan-dialog.tsx'
import { ToastHost } from '@/components/toast-host.tsx'
import { AppShell } from '@/components/ui/app-shell.tsx'
import { Button } from '@/components/ui/button.tsx'
import {
  Activity,
  Folder,
  Home,
  Layers,
  LayoutList,
  Moon,
  Table2,
  ShieldCheck,
  Sun,
} from '@/components/ui/icons.tsx'
import type { SidebarNavGroup } from '@/components/ui/sidebar.tsx'
import type { ReactNode } from '@/lib/types.ts'
import { logout } from '@/lib/api.ts'
import {
  activeConnection,
  activeProject,
  appState,
  initialTheme,
  setActiveConnection,
  setActiveProject,
  setTheme,
  showToast,
} from '@/lib/state.ts'

export const Route = createRootRoute({
  head: () => ({
    title: 'Vorzela Migrate',
    description:
      'Local GUI for editing SQL migrations, selecting a saved database and confirming every change.',
  }),
  component: RootLayout,
})

const GROUPS: SidebarNavGroup[] = [
  {
    group: 'Workspace',
    items: [
      { label: 'Dashboard', href: '/', icon: <Home aria-hidden /> },
      {
        label: 'Tables',
        href: '/tables',
        icon: <Table2 aria-hidden />,
      },
      {
        label: 'Migrations',
        href: '/migrations',
        icon: <LayoutList aria-hidden />,
      },
      { label: 'Projects', href: '/projects', icon: <Folder aria-hidden /> },
    ],
  },
  {
    group: 'Database',
    items: [
      {
        label: 'Connections',
        href: '/connections',
        icon: <Layers aria-hidden />,
      },
      { label: 'Activity', href: '/activity', icon: <Activity aria-hidden /> },
      {
        label: 'Settings',
        href: '/settings',
        icon: <ShieldCheck aria-hidden />,
      },
    ],
  },
]

function pageTitle(pathname: string): string {
  if (pathname.startsWith('/migrations/')) return 'Edit migration'
  if (pathname.startsWith('/migrations')) return 'Migrations'
  if (pathname.startsWith('/tables')) return 'Tables'
  if (pathname.startsWith('/projects')) return 'Projects'
  if (pathname.startsWith('/connections')) return 'Connections'
  if (pathname.startsWith('/activity')) return 'Activity'
  if (pathname.startsWith('/settings')) return 'Settings'
  return 'Dashboard'
}

function ThemeToggle() {
  const [theme, setThemeValue] = signal<'light' | 'dark'>(initialTheme())

  function toggle(): void {
    const next = theme() === 'dark' ? 'light' : 'dark'
    setThemeValue(next)
    setTheme(next)
  }

  return (
    <Button
      variant="outline"
      size="icon"
      aria-label="Toggle theme"
      title="Toggle light/dark theme"
      onClick={toggle}
    >
      <Show when={() => theme() === 'dark'} fallback={<Moon aria-hidden />}>
        <Sun aria-hidden />
      </Show>
    </Button>
  )
}

function RootLayout({ children }: { children: ReactNode }) {
  const location = useLocation()

  const state = appState()

  function switchProject(id: string): void {
    setActiveProject(id)
    const next = appState()?.projects.find((item) => item.id === id)
    if (next?.connectionId) setActiveConnection(next.connectionId)
  }

  async function lock(): Promise<void> {
    try {
      await logout()
      window.location.reload()
    } catch {
      showToast('error', 'Could not lock the GUI')
    }
  }

  return (
    <AppShell
      title={() => pageTitle(location.pathname)}
      groups={GROUPS}
      brand="Vorzela Migrate"
      subtitle="Local, confirmation-first"
      version={state?.version?.replace(/^v/, '')}
      currentPath={(() => location.pathname) as unknown as string}
      footer={
        <span class="small muted">
          Loopback only · one-time token · encrypted credentials
        </span>
      }
      actions={
        <div class="row">
          <Show when={() => (appState()?.projects.length ?? 0) > 0}>
            <>
              <select
                class="topbar-select"
                aria-label="Active project"
                value={() => activeProject()?.id ?? ''}
                onChange={(event) =>
                  switchProject((event.target as HTMLSelectElement).value)
                }
              >
                <For
                  each={() => appState()?.projects ?? []}
                  key={(item) => item.id}
                >
                  {(item) => (
                    <option
                      value={item.id}
                      selected={item.id === (activeProject()?.id ?? '')}
                    >
                      {item.name}
                    </option>
                  )}
                </For>
              </select>
              <select
                class="topbar-select"
                aria-label="Database connection"
                value={() => activeConnection()?.id ?? ''}
                onChange={(event) =>
                  setActiveConnection((event.target as HTMLSelectElement).value)
                }
              >
                <For
                  each={() => appState()?.connections ?? []}
                  key={(item) => item.id}
                >
                  {(item) => (
                    <option
                      value={item.id}
                      selected={item.id === (activeConnection()?.id ?? '')}
                    >
                      {item.name}
                    </option>
                  )}
                </For>
              </select>
            </>
          </Show>
          <ThemeToggle />
          <Button variant="outline" size="sm" onClick={() => void lock()}>
            Lock
          </Button>
        </div>
      }
    >
      {children}
      <PlanDialog />
      <ResultDialog />
      <ToastHost />
    </AppShell>
  )
}
