
import { cx } from '~/lib/cx.ts'
import styles from '~/styles/components/ui/spinner.module.scss'
import type { CSSProperties } from '~/lib/types.ts'

export type SpinnerSize = 'sm' | 'md' | 'lg' | 'xl'

const SIZE: Record<SpinnerSize, string> = {
  sm: styles.sm,
  md: '',
  lg: styles.lg,
  xl: styles.xl,
}

export interface SpinnerProps {
  size?: SpinnerSize
  /** Announced to assistive tech. Pass `null` when a visible label exists. */
  label?: string | null
  /** Use on filled primary surfaces. */
  onPrimary?: boolean
  /** Pixel override for one-off cases. */
  pixels?: number
  className?: string
}

export function Spinner({
  size = 'md',
  label = 'Loading',
  onPrimary = false,
  pixels,
  className,
}: SpinnerProps) {
  return (
    <span
      role={label ? 'status' : undefined}
      aria-label={label ?? undefined}
      aria-hidden={label ? undefined : true}
      className={cx(
        styles.root,
        SIZE[size],
        onPrimary && styles.onPrimary,
        className,
      )}
      style={
        pixels
          ? ({ '--spinner-size': `${pixels}px` } as CSSProperties)
          : undefined
      }
    />
  )
}
