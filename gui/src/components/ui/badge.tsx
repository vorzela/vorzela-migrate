
import { cx } from '~/lib/cx.ts'
import styles from '~/styles/components/ui/badge.module.scss'
import type { HTMLAttributes } from '~/lib/types.ts'

export type BadgeVariant =
  | 'primary'
  | 'secondary'
  | 'outline'
  | 'destructive'
  | 'warning'
  | 'success'
  | 'soft'

export type BadgeSize = 'sm' | 'md' | 'lg'

const VARIANT: Record<BadgeVariant, string> = {
  primary: styles.primary,
  secondary: styles.secondary,
  outline: styles.outline,
  destructive: styles.destructive,
  warning: styles.warning,
  success: styles.success,
  soft: styles.soft,
}

const SIZE: Record<BadgeSize, string> = {
  sm: styles.sm,
  md: '',
  lg: styles.lg,
}

export interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  variant?: BadgeVariant
  size?: BadgeSize
}

export function Badge({
  className,
  variant = 'primary',
  size = 'md',
  ...props
}: BadgeProps) {
  return (
    <span
      className={cx(styles.root, VARIANT[variant], SIZE[size], className)}
      {...props}
    />
  )
}
