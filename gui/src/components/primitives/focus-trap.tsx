import { effect, onCleanup, signal } from '@vorzelajs/core'
import { accessorOf } from '~/hooks/index.ts'
import { cx } from '~/lib/cx.ts'
import type { ReactNode } from '~/lib/types.ts'
import styles from '~/styles/components/primitives/primitives.module.scss'

const FOCUSABLE = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
  '[contenteditable="true"]',
].join(',')

export interface FocusTrapProps {
  children?: ReactNode
  class?: string
  className?: string
  active?: boolean | (() => boolean)
  restoreFocus?: boolean
}

/** Traps Tab focus and restores focus when the branch is disposed. */
export function FocusTrap({
  children,
  className,
  active = true,
  restoreFocus = true,
}: FocusTrapProps) {
  const [node, setNode] = signal<HTMLDivElement | null>(null)
  const isActive = accessorOf(active)

  effect(() => {
    const root = node()
    if (!root || !isActive()) return

    const previous = document.activeElement as HTMLElement | null
    const items = root.querySelectorAll<HTMLElement>(FOCUSABLE)
    ;(items[0] ?? root).focus()

    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.key !== 'Tab') return
      const focusable = Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE))
      if (focusable.length === 0) {
        event.preventDefault()
        return
      }
      const first = focusable[0] as HTMLElement
      const last = focusable[focusable.length - 1] as HTMLElement
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault()
        last.focus()
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault()
        first.focus()
      }
    }

    root.addEventListener('keydown', onKeyDown)
    onCleanup(() => {
      root.removeEventListener('keydown', onKeyDown)
      if (restoreFocus) previous?.focus()
    })
  })

  return (
    <div
      ref={(element) => setNode(element as HTMLDivElement)}
      className={cx(styles.trap, className)}
      tabIndex={-1}
    >
      {children}
    </div>
  )
}
