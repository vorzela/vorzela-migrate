import { cx } from '~/lib/cx.ts'
import { useId } from '~/hooks/use-id.ts'
import { forwardRef } from '~/lib/types.ts'
import type { InputHTMLAttributes, ReactNode } from '~/lib/types.ts'
import { CircleAlert, CircleCheck } from './icons.tsx'
import { Label } from './label.tsx'
import styles from '~/styles/components/ui/input.module.scss'

export type InputSize = 'sm' | 'md' | 'lg'

const SIZE: Record<InputSize, string> = {
  sm: styles.sm,
  md: '',
  lg: styles.lg,
}

export interface InputProps
  extends InputHTMLAttributes<HTMLInputElement> {
  label?: string
  hint?: string
  error?: string
  success?: string
  size?: InputSize
  leading?: ReactNode
  trailing?: ReactNode
  leadingNode?: ReactNode
  trailingNode?: ReactNode
  appearance?: 'default' | 'outlined' | 'filled'
  wrapClassName?: string
}

export const Input = forwardRef<HTMLInputElement, InputProps>(
  function Input(
    {
      label,
      hint,
      error,
      success,
      size = 'md',
      leading,
      trailing,
      leadingNode,
      trailingNode,
      appearance = 'default',
      wrapClassName,
      id: idProp,
      className,
      required,
      disabled,
      ...props
    },
    ref,
  ) {
    const generatedId = useId('input')
    const id = idProp ?? generatedId
    const lead = leadingNode ?? leading
    const trail = trailingNode ?? trailing

    const hasError = Boolean(error)
    const hasSuccess = Boolean(success) && !hasError
    const describedBy =
      [
        hint ? `${id}-hint` : null,
        hasError ? `${id}-error` : null,
        hasSuccess ? `${id}-success` : null,
      ]
        .filter(Boolean)
        .join(' ') || undefined

    return (
      <div className={cx(styles.field, wrapClassName)}>
        {label ? (
          <Label
            htmlFor={id}
            required={required}
            hint={hint}
            hintId={`${id}-hint`}
          >
            {label}
          </Label>
        ) : null}

        <div
          className={cx(
            styles.shell,
            SIZE[size],
            appearance === 'outlined' && styles.outlined,
            appearance === 'filled' && styles.filled,
            hasError ? styles.invalid : hasSuccess ? styles.valid : undefined,
            disabled && styles.disabled,
          )}
        >
          {lead ? <span className={styles.affix}>{lead}</span> : null}

          <input
            {...props}
            ref={ref}
            id={id}
            required={required}
            disabled={disabled}
            aria-invalid={hasError || undefined}
            aria-describedby={describedBy}
            className={cx(styles.control, className)}
          />

          {hasError ? (
            <span className={cx(styles.status, styles.statusError)}>
              <CircleAlert aria-hidden />
            </span>
          ) : hasSuccess ? (
            <span className={cx(styles.status, styles.statusSuccess)}>
              <CircleCheck aria-hidden />
            </span>
          ) : null}

          {trail ? <span className={styles.affix}>{trail}</span> : null}
        </div>

        {hasError ? (
          <p id={`${id}-error`} role="alert" className={cx(styles.message, styles.errorText)}>
            {error}
          </p>
        ) : hasSuccess ? (
          <p id={`${id}-success`} className={cx(styles.message, styles.successText)}>
            {success}
          </p>
        ) : null}
      </div>
    )
  },
)

export const BareInput = forwardRef<
  HTMLInputElement,
  InputHTMLAttributes<HTMLInputElement>
>(function BareInput({ className, ...props }, ref) {
  return <input ref={ref} className={cx(styles.bare, className)} {...props} />
})

export { Input as InputField }
