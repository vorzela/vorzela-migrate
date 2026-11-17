import { effect, onCleanup } from '@vorzelajs/core'

export function useIsomorphicLayoutEffect(fn: () => void | (() => void)): void {
  effect(() => {
    const cleanup = fn()
    if (typeof cleanup === 'function') onCleanup(cleanup)
  })
}
