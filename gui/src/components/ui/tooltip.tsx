/**
 * components/ui/tooltip.tsx
 *
 * Viewport-aware tooltip with collision-aware placement.
 */
import { effect, onCleanup, signal } from '@vorzelajs/core'
import { Show } from '@vorzelajs/dom'
import { useId } from '~/hooks/use-id.ts'
import { cx } from '~/lib/cx.ts'
import type { ReactNode } from '~/lib/types.ts'
import { Portal } from '~/components/primitives/portal.tsx'
import styles from '~/styles/components/ui/tooltip.module.scss'

const VIEWPORT_MARGIN = 8
const GAP = 10

export type TooltipPlacement = 'top' | 'bottom' | 'left' | 'right'

interface ComputedPos {
  x: number
  y: number
  placement: TooltipPlacement
  arrowOffset: number
}

function computePos(
  trigger: HTMLElement,
  tooltip: HTMLElement,
  preferred: TooltipPlacement,
): ComputedPos {
  const tr = trigger.getBoundingClientRect()
  const tw = tooltip.offsetWidth
  const th = tooltip.offsetHeight
  const vw = window.innerWidth
  const vh = window.innerHeight
  const margin = VIEWPORT_MARGIN
  const gap = GAP

  const space = {
    top: tr.top - margin,
    bottom: vh - tr.bottom - margin,
    left: tr.left - margin,
    right: vw - tr.right - margin,
  }

  const fits: Record<TooltipPlacement, boolean> = {
    top: space.top >= th + gap,
    bottom: space.bottom >= th + gap,
    left: space.left >= tw + gap,
    right: space.right >= tw + gap,
  }

  const opposite: Record<TooltipPlacement, TooltipPlacement> = {
    top: 'bottom',
    bottom: 'top',
    left: 'right',
    right: 'left',
  }

  let placement: TooltipPlacement = preferred
  if (!fits[preferred]) {
    if (fits[opposite[preferred]]) {
      placement = opposite[preferred]
    } else {
      placement = (
        Object.entries(space).sort(([, a], [, b]) => b - a)[0] as [
          TooltipPlacement,
          number,
        ]
      )[0]
    }
  }

  let x: number
  let y: number
  let arrowOffset: number

  const ARROW_HALF = 6

  if (placement === 'top' || placement === 'bottom') {
    const ideal = tr.left + tr.width / 2 - tw / 2
    x = Math.max(margin, Math.min(ideal, vw - tw - margin))
    y = placement === 'top' ? tr.top - th - gap : tr.bottom + gap
    const arrowIdeal = tr.left + tr.width / 2 - x
    arrowOffset = Math.max(
      ARROW_HALF + 4,
      Math.min(arrowIdeal, tw - ARROW_HALF - 4),
    )
  } else {
    const ideal = tr.top + tr.height / 2 - th / 2
    y = Math.max(margin, Math.min(ideal, vh - th - margin))
    x = placement === 'left' ? tr.left - tw - gap : tr.right + gap
    const arrowIdeal = tr.top + tr.height / 2 - y
    arrowOffset = Math.max(
      ARROW_HALF + 4,
      Math.min(arrowIdeal, th - ARROW_HALF - 4),
    )
  }

  return { x, y, placement, arrowOffset }
}

export interface TooltipProps {
  content: ReactNode
  placement?: TooltipPlacement
  delayShow?: number
  delayHide?: number
  disabled?: boolean
  maxWidth?: number
  className?: string
  children: ReactNode
}

export function Tooltip({
  content,
  placement: placementPref = 'top',
  delayShow = 0,
  delayHide = 0,
  disabled,
  maxWidth = 240,
  className,
  children,
}: TooltipProps) {
  const tooltipId = useId('tooltip')
  const [trigger, setTrigger] = signal<HTMLSpanElement | null>(null)
  const [tooltip, setTooltip] = signal<HTMLDivElement | null>(null)
  const [visible, setVisible] = signal(false)
  const [pos, setPos] = signal<ComputedPos | null>(null)

  onCleanup(() => setTrigger(null))

  let showTimer: ReturnType<typeof setTimeout> | null = null
  let hideTimer: ReturnType<typeof setTimeout> | null = null

  const show = (): void => {
    if (disabled) return
    if (hideTimer) {
      clearTimeout(hideTimer)
      hideTimer = null
    }
    if (showTimer) clearTimeout(showTimer)
    showTimer = setTimeout(() => {
      showTimer = null
      setVisible(true)
    }, delayShow)
    showTimer.unref?.()
  }

  const hide = (): void => {
    if (showTimer) {
      clearTimeout(showTimer)
      showTimer = null
    }
    if (hideTimer) clearTimeout(hideTimer)
    hideTimer = setTimeout(() => {
      hideTimer = null
      setVisible(false)
      setPos(null)
    }, delayHide)
    hideTimer.unref?.()
  }

  effect(() => {
    const isVisible = visible()
    const triggerNode = trigger()
    const tooltipNode = tooltip()

    if (
      !isVisible ||
      !triggerNode ||
      !tooltipNode ||
      typeof window === 'undefined'
    ) {
      return
    }

    const update = (): void => {
      setPos(computePos(triggerNode, tooltipNode, placementPref))
    }

    const frame = requestAnimationFrame(update)
    window.addEventListener('scroll', update, { passive: true, capture: true })
    window.addEventListener('resize', update, { passive: true })
    onCleanup(() => {
      cancelAnimationFrame(frame)
      window.removeEventListener('scroll', update, { capture: true })
      window.removeEventListener('resize', update)
    })
  })

  onCleanup(() => {
    if (showTimer) clearTimeout(showTimer)
    if (hideTimer) clearTimeout(hideTimer)
    showTimer = null
    hideTimer = null
  })

  return (
    <>
      <span
        ref={(element) => setTrigger(element as HTMLSpanElement)}
        className={cx(styles.trigger, className)}
        onMouseEnter={show}
        onMouseLeave={hide}
        onFocus={show}
        onBlur={hide}
        aria-describedby={() => (visible() ? tooltipId : undefined)}
      >
        {children}
      </span>

      <Show when={visible}>
        {() => {
          const panel = (
            <Portal>
              <div
                ref={(element) => setTooltip(element as HTMLDivElement)}
                id={tooltipId}
                role="tooltip"
                style={() => ({
                  position: 'fixed',
                  left: `${pos()?.x ?? 0}px`,
                  top: `${pos()?.y ?? 0}px`,
                  maxWidth: `${maxWidth}px`,
                  visibility: pos() ? 'visible' : 'hidden',
                  opacity: pos() ? 1 : 0,
                })}
                className={styles.content}
              >
                {content}
                <span
                  aria-hidden="true"
                  className={styles.arrow}
                  data-placement={() => pos()?.placement}
                  style={() => ({
                    '--vz-arrow-offset': `${pos()?.arrowOffset ?? 0}px`,
                  })}
                />
              </div>
            </Portal>
          )

          onCleanup(() => setTooltip(null))
          return panel
        }}
      </Show>
    </>
  )
}
