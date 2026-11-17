/**
 * components/ui/alert-dialog.tsx
 *
 * Confirmation dialog — focus-trapped, scroll-lock safe.
 */
import { effect, onCleanup, signal } from '@vorzelajs/core'
import { Show } from '@vorzelajs/dom'
import { accessorOf } from '~/hooks/index.ts'
import { useId } from '~/hooks/use-id.ts'
import { cx } from '~/lib/cx.ts'
import { lockBodyScroll, trapFocus } from '~/lib/overlay.ts'
import type { ReactNode } from '~/lib/types.ts'
import { Portal } from '~/components/primitives/portal.tsx'
import { Button } from './button.tsx'
import styles from '~/styles/components/ui/alert-dialog.module.scss'

export interface AlertDialogProps {
  open: boolean | (() => boolean)
  onOpenChange: (open: boolean) => void
  title?: string
  description?: string
  cancelText?: string
  actionText?: string
  onAction?: () => void
  onCancel?: () => void
  destructive?: boolean
  className?: string
  children?: ReactNode
}

export function AlertDialog({
  open,
  onOpenChange,
  title,
  description,
  cancelText = 'Cancel',
  actionText = 'Continue',
  onAction,
  onCancel,
  destructive = false,
  className,
  children,
}: AlertDialogProps) {
  const titleId = useId('alert-dialog-title')
  const descId = useId('alert-dialog-description')
  const [panel, setPanel] = signal<HTMLDivElement | null>(null)
  const isOpen = accessorOf(open)

  effect(() => {
    const node = panel()
    if (!isOpen() || !node || typeof document === 'undefined') return
    const unlock = lockBodyScroll()
    const releaseTrap = trapFocus(node, () => onOpenChange(false))
    onCleanup(() => {
      releaseTrap()
      unlock()
    })
  })

  const handleCancel = (): void => {
    onCancel?.()
    onOpenChange(false)
  }

  const handleAction = (): void => {
    onAction?.()
  }

  return (
    <Show when={isOpen}>
      {() => {
        const content = (
          <Portal>
            <div className={styles.root}>
              <div
                className={styles.overlay}
                aria-hidden="true"
                onClick={handleCancel}
              />

              <div
                ref={(element) => setPanel(element as HTMLDivElement)}
                role="alertdialog"
                aria-modal="true"
                aria-labelledby={title ? titleId : undefined}
                aria-describedby={description ? descId : undefined}
                tabIndex={-1}
                className={cx(styles.panel, className)}
                onClick={(event) => event.stopPropagation()}
              >
                <div className={styles.content}>
                  {title ? (
                    <h2 id={titleId} className={styles.title}>
                      {title}
                    </h2>
                  ) : null}

                  {description ? (
                    <p id={descId} className={styles.description}>
                      {description}
                    </p>
                  ) : null}

                  {children}
                </div>

                <div className={styles.actions}>
                  <Button variant="outline" onClick={handleCancel}>
                    {cancelText}
                  </Button>
                  <Button
                    variant={destructive ? 'destructive' : 'primary'}
                    onClick={handleAction}
                  >
                    {actionText}
                  </Button>
                </div>
              </div>
            </div>
          </Portal>
        )

        onCleanup(() => setPanel(null))
        return content
      }}
    </Show>
  )
}

export { AlertDialog as ConfirmationDialog }
