import { signal } from '@vorzelajs/core'
import { Show } from '@vorzelajs/dom'
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
import { Download, RotateCcw, ShieldCheck } from '@/components/ui/icons.tsx'
import { fetchUpdateCheck, logout } from '@/lib/api.ts'
import type { UpdateInfo } from '@/lib/api-types.ts'
import { formatDate } from '@/lib/format.ts'
import { appState, requestAction, showToast } from '@/lib/state.ts'

export const Route = createFileRoute('/settings')({
  head: () => ({
    title: 'Settings · Vorzela Migrate',
    description: 'Local GUI storage, security and confirmation policy.',
  }),
  component: SettingsPage,
})

async function lock(): Promise<void> {
  try {
    await logout()
    window.location.reload()
  } catch {
    showToast('error', 'Could not lock the GUI')
  }
}

function SettingsPage() {
  const [updateInfo, setUpdateInfo] = signal<UpdateInfo | null>(null)
  const [checkingUpdates, setCheckingUpdates] = signal(false)
  const [upgradeDone, setUpgradeDone] = signal(false)

  async function checkUpdates(): Promise<void> {
    if (checkingUpdates()) return
    setCheckingUpdates(true)
    try {
      const info = await fetchUpdateCheck(true)
      setUpdateInfo(info)
      if (info.error) showToast('warning', 'Update check failed', info.error)
    } catch (cause) {
      showToast(
        'error',
        'Update check failed',
        cause instanceof Error ? cause.message : String(cause),
      )
    } finally {
      setCheckingUpdates(false)
    }
  }

  function upgrade(): void {
    void requestAction({ action: 'system.upgrade' }, (result) => {
      if (result.result.ok) {
        setUpgradeDone(true)
        setUpdateInfo(null)
      }
    })
  }

  return (
    <div class="page">
      <header class="page-header">
        <div>
          <p class="eyebrow">Local configuration</p>
          <h2>Settings</h2>
          <p class="muted">
            The GUI stores project references and encrypted credentials only.
            Migration SQL stays in your project directory.
          </p>
        </div>
        <Badge variant="secondary">{() => appState()?.version ?? 'dev'}</Badge>
      </header>

      <div class="grid grid-2">
        <Card>
          <CardHeader>
            <CardTitle>Storage</CardTitle>
            <CardDescription>
              Choose <code>yaml</code> or <code>sqlite</code> at launch.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <dl class="plan-target">
              <dt>Store</dt>
              <dd>{() => appState()?.storeKind ?? '—'}</dd>
              <dt>Store path</dt>
              <dd class="path">{() => appState()?.storePath ?? '—'}</dd>
              <dt>Config path</dt>
              <dd class="path">{() => appState()?.configPath ?? '—'}</dd>
              <dt>vm binary</dt>
              <dd class="path">{() => appState()?.vmPath ?? '—'}</dd>
              <dt>Session ends</dt>
              <dd>
                {() =>
                  appState()?.sessionExpires
                    ? formatDate(appState()?.sessionExpires)
                    : '—'
                }
              </dd>
            </dl>
            <p class="muted small" style={{ 'margin-top': 'var(--space-3)' }}>
              Switch stores with <code>vm gui --store sqlite</code> or{' '}
              <code>vm gui --store yaml</code>. Existing data is not migrated
              automatically.
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Security posture</CardTitle>
            <CardDescription>
              Local-only defaults with defense in depth.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <ul class="issue-list">
              <li class="issue">
                <span>
                  Binds to <code>127.0.0.1</code> and rejects unknown Host
                  headers.
                </span>
              </li>
              <li class="issue">
                <span>
                  One-time access token; HttpOnly, SameSite=Strict session
                  cookie.
                </span>
              </li>
              <li class="issue">
                <span>
                  CSRF token + custom header + same-origin check on every write.
                </span>
              </li>
              <li class="issue">
                <span>
                  Database passwords encrypted with AES-256-GCM; key file mode
                  0600.
                </span>
              </li>
              <li class="issue">
                <span>
                  Passwords are injected into <code>vm</code> through the
                  environment, never argv.
                </span>
              </li>
              <li class="issue">
                <span>
                  Strict CSP, no CORS, request body limits and migration-path
                  containment.
                </span>
              </li>
              <li class="issue">
                <span>
                  Every write requires a server-issued, single-use confirmation
                  plan.
                </span>
              </li>
            </ul>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Confirmation policy</CardTitle>
            <CardDescription>
              The browser cannot call a mutating action directly.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div class="stack-sm">
              <p>
                <strong>Normal writes</strong> show the project, database, file
                and exact command. Confirm with one click.
              </p>
              <p>
                <strong>Executed migration edits</strong> require typing the
                filename. Applying them uses <code>--force --detect-drift</code>
                .
              </p>
              <p>
                <strong>Rollback and fresh rebuild</strong> require typing the
                project name and show the Down SQL preview.
              </p>
              <p class="muted small">
                Confirmations expire after{' '}
                {() => appState()?.settings.confirmationTtlSeconds ?? 600}{' '}
                seconds and are consumed once.
              </p>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Drift policy</CardTitle>
            <CardDescription>
              How the GUI keeps migration history thin.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div class="stack-sm">
              <p>
                <Badge variant="success">Preferred</Badge> Edit the existing{' '}
                <code>create_*_table</code> file, then apply with drift.
                Additive nullable/default columns are synced without a new
                migration.
              </p>
              <p>
                <Badge variant="warning">Only when necessary</Badge> A follow-up{' '}
                <code>add_*</code> migration is suggested for NOT NULL columns
                without a default, NOT NULL UNIQUE columns, NOT NULL foreign
                keys and column modifications.
              </p>
              <p>
                <Badge variant="destructive">Explicit</Badge> Drops and
                destructive changes are never applied by drift. They need a
                forward migration you review and confirm.
              </p>
            </div>
          </CardContent>
        </Card>
      </div>

      <Show when={() => appState()?.settings.preferRuntimeCredentials}>
        <Card>
          <CardHeader>
            <CardTitle>Runtime credentials</CardTitle>
            <CardDescription>
              The selected connection is injected into <code>vm</code> as{' '}
              <code>DATABASE_URL</code>.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <p class="muted">
              This preference is on. Project <code>vm.yaml</code> files can omit{' '}
              <code>DATABASE_URL</code>; the GUI supplies it for every action.
              That keeps the password out of source control.
            </p>
          </CardContent>
        </Card>
      </Show>

      <Card>
        <CardHeader>
          <div class="row-between">
            <div>
              <CardTitle>Version & updates</CardTitle>
              <CardDescription>
                Installed version and the latest GitHub release for the per-user
                vm binary.
              </CardDescription>
            </div>
            <Badge variant="secondary">
              {() => appState()?.version ?? 'dev'}
            </Badge>
          </div>
        </CardHeader>
        <CardContent>
          <div class="stack">
            <dl class="plan-target">
              <dt>Installed</dt>
              <dd>{() => appState()?.version ?? 'dev'}</dd>
              <dt>vm binary</dt>
              <dd class="path">{() => appState()?.vmPath ?? '—'}</dd>
              <dt>Latest release</dt>
              <dd>{() => updateInfo()?.latest ?? 'Not checked yet'}</dd>
              <dt>Last checked</dt>
              <dd>
                {() =>
                  updateInfo()?.checkedAt
                    ? formatDate(updateInfo()?.checkedAt)
                    : '—'
                }
              </dd>
            </dl>
            <div class="row">
              <Button variant="outline" onClick={() => void checkUpdates()}>
                <RotateCcw aria-hidden />
                {() => (checkingUpdates() ? 'Checking…' : 'Check for updates')}
              </Button>
              <Show
                when={() => Boolean(updateInfo()?.available) && !upgradeDone()}
              >
                <Button onClick={upgrade}>
                  <Download aria-hidden />
                  Upgrade to {() => updateInfo()?.latest ?? 'latest'}
                </Button>
              </Show>
            </div>

            <Show when={() => Boolean(updateInfo()?.error)}>
              <Alert variant="warning" title="Update check failed">
                {() => updateInfo()?.error}
              </Alert>
            </Show>

            <Show
              when={() => Boolean(updateInfo()?.available) && !upgradeDone()}
            >
              <Alert variant="info" title="Update available">
                {() =>
                  `${updateInfo()?.current} → ${updateInfo()?.latest}. The upgrade runs the same vm upgrade command as the terminal.`
                }
              </Alert>
            </Show>

            <Show
              when={() =>
                Boolean(updateInfo()) &&
                !updateInfo()?.available &&
                !updateInfo()?.error
              }
            >
              <Alert variant="success" title="You're up to date">
                {() => `Installed ${updateInfo()?.current}.`}
              </Alert>
            </Show>

            <Show when={() => upgradeDone()}>
              <Alert variant="success" title="Upgrade installed">
                Restart <code>vm gui</code> to use the new binary and embedded
                GUI. The running process keeps the old version until then.
              </Alert>
            </Show>

            <p class="muted small">
              The GUI checks the GitHub release API through{' '}
              <code>vm upgrade</code>. If the check fails, update from a
              terminal with <code>vm upgrade</code>.
            </p>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Session</CardTitle>
          <CardDescription>
            Locking revokes this in-memory session. Restart <code>vm gui</code>{' '}
            for a new one-time token.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Button variant="outline" onClick={() => void lock()}>
            <ShieldCheck aria-hidden />
            Lock GUI now
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}
