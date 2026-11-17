
import { cx } from '~/lib/cx.ts'
import styles from '~/styles/components/ui/button.module.scss'
import { forwardRef } from '~/lib/types.ts'
import type { ButtonHTMLAttributes } from '~/lib/types.ts'

export type ButtonVariant =
  | 'primary'
  | 'secondary'
  | 'outline'
  | 'ghost'
  | 'destructive'
  | 'link'

export type ButtonSize = 'sm' | 'md' | 'lg' | 'icon'

const VARIANT: Record<ButtonVariant, string> = {
  primary: styles.primary,
  secondary: styles.secondary,
  outline: styles.outline,
  ghost: styles.ghost,
  destructive: styles.destructive,
  link: styles.link,
}

const SIZE: Record<ButtonSize, string> = {
  sm: styles.sm,
  md: '',
  lg: styles.lg,
  icon: styles.icon,
}

export interface ButtonProps
  extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant
  size?: ButtonSize
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = 'primary', size = 'md', type = 'button', ...props }, ref) => (
    <button
      ref={ref}
      type={type}
      className={cx(styles.root, VARIANT[variant], SIZE[size], className)}
      {...props}
    />
  ),
)

Button.displayName = 'Button'
