import { cx } from '~/lib/cx.ts'
import type { HTMLAttributes, ReactNode } from '~/lib/types.ts'
import { Label } from './label.tsx'
import styles from '~/styles/components/ui/field.module.scss'

export interface FieldProps extends HTMLAttributes<HTMLDivElement> {
  label?: ReactNode
  htmlFor?: string
  hint?: ReactNode
  error?: ReactNode
  required?: boolean
  optional?: boolean
  orientation?: 'vertical' | 'horizontal'
  children?: ReactNode
}

/** Label + control + description/error in one accessible block. */
export function Field({
  label,
  htmlFor,
  hint,
  error,
  required,
  optional,
  orientation = 'vertical',
  className,
  children,
  ...props
}: FieldProps) {
  return (
    <div
      className={cx(
        styles.root,
        orientation === 'horizontal' && styles.horizontal,
        className,
      )}
      {...props}
    >
      {label || optional ? (
        <div className={styles.labelRow}>
          {label ? (
            <Label htmlFor={htmlFor} required={required}>
              {label}
            </Label>
          ) : (
            <span />
          )}
          {optional ? <span className={styles.optional}>Optional</span> : null}
        </div>
      ) : null}
      {children}
      {error ? (
        <p role="alert" className={cx(styles.message, styles.error)}>
          {error}
        </p>
      ) : hint ? (
        <p className={styles.message}>{hint}</p>
      ) : null}
    </div>
  )
}

export interface FieldGroupProps extends HTMLAttributes<HTMLDivElement> {
  children?: ReactNode
}

/** Vertical stack for a set of fields. */
export function FieldGroup({ className, children, ...props }: FieldGroupProps) {
  return (
    <div className={cx(styles.group, className)} {...props}>
      {children}
    </div>
  )
}
