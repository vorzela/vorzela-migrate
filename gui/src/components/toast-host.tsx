import { Show } from '@vorzelajs/dom'

import { Alert } from '@/components/ui/alert.tsx'
import { dismissToast, toast } from '@/lib/state.ts'

const VARIANT = {
  success: 'success',
  error: 'error',
  warning: 'warning',
  info: 'info',
} as const

export function ToastHost() {
  return (
    <Show when={() => toast() !== null}>
      {() => {
        const value = toast()
        if (!value) return null
        return (
          <div class="toast-host" aria-live="polite">
            <Alert
              variant={VARIANT[value.tone]}
              title={value.title}
              onDismiss={dismissToast}
            >
              {value.message ? <span>{value.message}</span> : null}
            </Alert>
          </div>
        )
      }}
    </Show>
  )
}
