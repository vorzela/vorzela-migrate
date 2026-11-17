/**
 * Shared overlay helpers — body scroll lock (ref-counted) + focus trap.
 * Prevents nested Modal/Drawer from unlocking scroll too early and keeps
 * keyboard focus inside the open surface.
 */

let scrollLockCount = 0
let previousOverflow = ''

export function lockBodyScroll() {
  if (typeof document === 'undefined') return () => {}
  if (scrollLockCount === 0) {
    previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
  }
  scrollLockCount += 1
  return () => {
    scrollLockCount = Math.max(0, scrollLockCount - 1)
    if (scrollLockCount === 0) {
      document.body.style.overflow = previousOverflow
    }
  }
}

const FOCUSABLE =
  'a[href],button:not([disabled]),textarea:not([disabled]),input:not([disabled]),select:not([disabled]),[tabindex]:not([tabindex="-1"])'

export function getFocusable(root: HTMLElement): HTMLElement[] {
  return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
    (el) => !el.hasAttribute('disabled') && el.getAttribute('aria-hidden') !== 'true',
  )
}

/** Trap Tab inside `root`; returns cleanup. */
export function trapFocus(root: HTMLElement, onEscape?: () => void) {
  const previouslyFocused =
    typeof document !== 'undefined'
      ? (document.activeElement as HTMLElement | null)
      : null

  const focusFirst = () => {
    const items = getFocusable(root)
    ;(items[0] ?? root).focus({ preventScroll: true })
  }

  // Defer so portal content is mounted
  const raf = requestAnimationFrame(focusFirst)

  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Escape') {
      e.stopPropagation()
      onEscape?.()
      return
    }
    if (e.key !== 'Tab') return
    const items = getFocusable(root)
    if (items.length === 0) {
      e.preventDefault()
      root.focus({ preventScroll: true })
      return
    }
    const first = items[0]!
    const last = items[items.length - 1]!
    const active = document.activeElement as HTMLElement | null
    if (e.shiftKey && (active === first || !root.contains(active))) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && (active === last || !root.contains(active))) {
      e.preventDefault()
      first.focus()
    }
  }

  document.addEventListener('keydown', onKeyDown, true)

  return () => {
    cancelAnimationFrame(raf)
    document.removeEventListener('keydown', onKeyDown, true)
    previouslyFocused?.focus?.({ preventScroll: true })
  }
}
