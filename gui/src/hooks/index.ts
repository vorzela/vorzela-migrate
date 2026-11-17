import {
  effect,
  onCleanup,
  signal,
  type Accessor,
} from '@vorzelajs/core'

export type SetState<T> = (next: T | ((prev: T) => T)) => void

/** Accepts a plain value or an accessor; components use it for reactive props. */
export function accessorOf<T>(value: T | Accessor<T>): Accessor<T> {
  return typeof value === 'function' ? (value as Accessor<T>) : () => value
}

let nextGeneratedId = 0

/**
 * Stable id for label/aria wiring. Pass an explicit id when SSR and client
 * markup must match exactly; generated ids follow render order.
 */
export function useId(prefix = 'vz'): string {
  nextGeneratedId += 1
  return `${prefix}-${nextGeneratedId}`
}

/**
 * Controlled/uncontrolled state. Reads are accessors so bindings track updates.
 * Pass `value` when the parent owns the state.
 */
export function useControllableState<T>({
  value,
  defaultValue,
  onChange,
}: {
  value?: T
  defaultValue: T
  onChange?: (value: T) => void
}): readonly [Accessor<T>, SetState<T>] {
  const [internal, setInternal] = signal(defaultValue)
  const current: Accessor<T> = () =>
    value !== undefined ? value : internal()
  const setValue: SetState<T> = (next) => {
    const resolved =
      typeof next === 'function'
        ? (next as (prev: T) => T)(current())
        : next
    if (value === undefined) setInternal(resolved)
    onChange?.(resolved)
  }
  return [current, setValue] as const
}

/** `false` during SSR and the first client pass, then true after mount. */
export function useMounted(): Accessor<boolean> {
  const [mounted, setMounted] = signal(false)
  effect(() => {
    setMounted(true)
  })
  return mounted
}

/** SSR-safe media query subscription. Server snapshot is false. */
export function useMediaQuery(query: string): Accessor<boolean> {
  const [matches, setMatches] = signal(false)

  effect(() => {
    if (typeof window === 'undefined' || !window.matchMedia) return
    const media = window.matchMedia(query)
    const update = () => setMatches(media.matches)
    update()
    media.addEventListener('change', update)
    onCleanup(() => media.removeEventListener('change', update))
  })

  return matches
}

/** True when the browser reports a network connection. */
export function useOnlineStatus(): Accessor<boolean> {
  const [online, setOnline] = signal(
    typeof navigator === 'undefined' ? true : navigator.onLine,
  )

  effect(() => {
    if (typeof window === 'undefined') return
    const update = () => setOnline(navigator.onLine)
    update()
    window.addEventListener('online', update)
    window.addEventListener('offline', update)
    onCleanup(() => {
      window.removeEventListener('online', update)
      window.removeEventListener('offline', update)
    })
  })

  return online
}

const REDUCED_MOTION = '(prefers-reduced-motion: reduce)'

/** True when the user asks for reduced motion. SSR-safe. */
export function useReducedMotion(): Accessor<boolean> {
  return useMediaQuery(REDUCED_MOTION)
}

export function useIsomorphicLayoutEffect(fn: () => void | (() => void)): void {
  effect(() => {
    const cleanup = fn()
    if (typeof cleanup === 'function') onCleanup(cleanup)
  })
}

/** Latest value without tracking; accepts a value or an accessor. */
export function useLatest<T>(value: T | Accessor<T>): Accessor<T> {
  return () =>
    typeof value === 'function' ? (value as Accessor<T>)() : (value as T)
}

export type CopyState = 'idle' | 'copied' | 'error'

/** Clipboard copy with a self-clearing status and no leaked timers. */
export function useCopyToClipboard(resetAfterMs = 1800): {
  state: Accessor<CopyState>
  copy: (value: string) => Promise<void>
  reset: () => void
} {
  const [state, setState] = signal<CopyState>('idle')
  let timer: ReturnType<typeof setTimeout> | null = null

  const clearTimer = (): void => {
    if (timer) {
      clearTimeout(timer)
      timer = null
    }
  }

  onCleanup(clearTimer)

  const copy = async (value: string): Promise<void> => {
    clearTimer()
    try {
      if (!navigator.clipboard?.writeText) throw new Error('unsupported')
      await navigator.clipboard.writeText(value)
      setState('copied')
    } catch {
      setState('error')
    }
    timer = setTimeout(() => {
      setState('idle')
      timer = null
    }, resetAfterMs)
    ;(timer as { unref?: () => void }).unref?.()
  }

  return {
    state,
    copy,
    reset: () => setState('idle'),
  }
}

export type Hotkey = {
  key: string
  meta?: boolean
  ctrl?: boolean
  alt?: boolean
  shift?: boolean
}

function isEditable(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return (
    target.isContentEditable ||
    ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)
  )
}

/** Global hotkeys. Editable targets are ignored unless `allowInInputs`. */
export function useHotkeys(
  hotkeys: Array<Hotkey & { handler: (event: KeyboardEvent) => void }>,
  options: { allowInInputs?: boolean; enabled?: boolean } = {},
): void {
  effect(() => {
    if (typeof window === 'undefined' || options.enabled === false) return

    const onKeyDown = (event: KeyboardEvent): void => {
      if (!options.allowInInputs && isEditable(event.target)) return
      for (const hotkey of hotkeys) {
        if (event.key.toLowerCase() !== hotkey.key.toLowerCase()) continue
        if (!!hotkey.meta !== event.metaKey) continue
        if (!!hotkey.ctrl !== event.ctrlKey) continue
        if (!!hotkey.alt !== event.altKey) continue
        if (!!hotkey.shift !== event.shiftKey) continue
        event.preventDefault()
        hotkey.handler(event)
        break
      }
    }

    window.addEventListener('keydown', onKeyDown)
    onCleanup(() => window.removeEventListener('keydown', onKeyDown))
  })
}

export type ContainerSize = {
  width: number
  height: number
}

/**
 * Observe a container's size. The observer and resize listener are disconnected
 * with the owner, not left to component lifetime.
 */
export function useContainerSize<T extends HTMLElement = HTMLDivElement>(): {
  ref: (node: T | null) => void
  size: Accessor<ContainerSize>
  width: Accessor<number>
  height: Accessor<number>
  matches: (px: number) => boolean
} {
  const [size, setSize] = signal<ContainerSize>({ width: 0, height: 0 })
  let observer: ResizeObserver | null = null
  let observed: T | null = null

  const measure = (): void => {
    if (!observed) return
    const rect = observed.getBoundingClientRect()
    setSize((previous) =>
      previous.width === rect.width && previous.height === rect.height
        ? previous
        : { width: rect.width, height: rect.height },
    )
  }

  const disconnect = (): void => {
    observer?.disconnect()
    observer = null
    if (typeof window !== 'undefined') {
      window.removeEventListener('resize', measure)
    }
  }

  const ref = (node: T | null): void => {
    disconnect()
    observed = node
    if (!node || typeof window === 'undefined') return

    measure()
    observer =
      typeof ResizeObserver === 'undefined'
        ? null
        : new ResizeObserver((entries) => {
            const entry = entries[0]
            if (!entry) return
            const box = entry.contentRect
            setSize((previous) =>
              previous.width === box.width && previous.height === box.height
                ? previous
                : { width: box.width, height: box.height },
            )
          })
    observer?.observe(node)
    window.addEventListener('resize', measure)
  }

  onCleanup(disconnect)

  return {
    ref,
    size,
    width: () => size().width,
    height: () => size().height,
    matches: (px) => size().width >= px,
  }
}

export function useContainerQuery(px: number): {
  ref: (node: HTMLDivElement | null) => void
  width: Accessor<number>
  matches: Accessor<boolean>
} {
  const { ref, width } = useContainerSize()
  return { ref, width, matches: () => width() >= px }
}
