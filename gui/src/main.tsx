import { signal } from '@vorzelajs/core'
import { Show, render } from '@vorzelajs/dom'

import './styles/global.scss'
import './styles/app.scss'

import { exchangeToken } from '@/lib/api.ts'
import { bootstrapState, initialTheme, setTheme } from '@/lib/state.ts'

function LockScreen() {
  const [token, setToken] = signal('')
  const [error, setError] = signal('')
  const [sending, setSending] = signal(false)

  async function submit(): Promise<void> {
    if (!token().trim()) {
      setError('Enter the access token printed by `vm gui`.')
      return
    }
    setSending(true)
    setError('')
    try {
      await exchangeToken(token().trim())
      window.location.reload()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setSending(false)
    }
  }

  return (
    <main class="lock-page">
      <section class="lock-card">
        <p class="eyebrow">Vorzela Migrate</p>
        <h1>GUI is locked</h1>
        <p class="muted">
          This server only listens on loopback and requires a one-time access
          token. Restart <code>vm gui</code> in your terminal if the token has
          already been used.
        </p>
        <form
          class="lock-form"
          onSubmit={(event) => {
            event.preventDefault()
            void submit()
          }}
        >
          <label for="access-token">Access token</label>
          <input
            id="access-token"
            autocomplete="off"
            spellcheck={false}
            placeholder="Paste the token from the terminal"
            value={() => token()}
            onInput={(event) =>
              setToken((event.target as HTMLInputElement).value)
            }
          />
          <button type="submit" disabled={() => sending()}>
            {() => (sending() ? 'Unlocking…' : 'Unlock')}
          </button>
        </form>
        <Show when={() => error() !== ''}>
          <p class="lock-error" role="alert">
            {() => error()}
          </p>
        </Show>
        <p class="muted small">
          Tip: the terminal prints a full URL containing <code>?token=…</code>.
          Opening that URL unlocks the GUI automatically.
        </p>
      </section>
    </main>
  )
}

async function boot(): Promise<void> {
  setTheme(initialTheme())
  const container = document.getElementById('app')
  if (!container) throw new Error('Missing #app mount point')

  if (import.meta.env.DEV) {
    const { installBrowserDevtools } =
      await import('@vorzelajs/devtools/browser')
    installBrowserDevtools()
  }

  const authenticated = await bootstrapState()
  if (!authenticated) {
    render(<LockScreen />, container)
    return
  }

  window.addEventListener('vm-auth-lost', () => {
    window.location.reload()
  })

  const { router } = await import('@/runtime/router')
  router.start({ container })
}

void boot()
