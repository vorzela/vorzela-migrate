import { effect, onCleanup, signal } from '@vorzelajs/core'
import { accessorOf } from '~/hooks/index.ts'
import type { ReactNode } from '~/lib/types.ts'
import styles from '~/styles/components/primitives/primitives.module.scss'

export interface DismissableLayerProps {
  children?: ReactNode
  onDismiss: () => void
  enabled?: boolean | (() => boolean)
}

/** Calls `onDismiss` on Escape or an outside pointer press. */
export function DismissableLayer({
  children,
  onDismiss,
  enabled = true,
}: DismissableLayerProps) {
  const [node, setNode] = signal<HTMLDivElement | null>(null)
  const isEnabled = accessorOf(enabled)

  effect(() => {
    if (!isEnabled()) return

    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.key !== 'Escape' || event.defaultPrevented) return
      event.preventDefault()
      onDismiss()
    }
    const onPointerDown = (event: PointerEvent): void => {
      const root = node()
      if (!root || root.contains(event.target as Node)) return
      onDismiss()
    }

    document.addEventListener('keydown', onKeyDown)
    document.addEventListener('pointerdown', onPointerDown, true)
    onCleanup(() => {
      document.removeEventListener('keydown', onKeyDown)
      document.removeEventListener('pointerdown', onPointerDown, true)
    })
  })

  return (
    <div ref={(element) => setNode(element as HTMLDivElement)} className={styles.layer}>
      {children}
    </div>
  )
}
