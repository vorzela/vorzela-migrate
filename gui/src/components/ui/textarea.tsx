import { effect, signal } from '@vorzelajs/core'

import { accessorOf, useId } from '~/hooks/index.ts'
import { cx } from '~/lib/cx.ts'
import { forwardRef } from '~/lib/types.ts'
import type { ChangeEvent, HTMLAttributes } from '~/lib/types.ts'
import { CircleAlert, CircleCheck } from './icons.tsx'
import { Label } from './label.tsx'
import styles from '~/styles/components/ui/textarea.module.scss'

export type TextareaProps = HTMLAttributes<HTMLTextAreaElement> & {
  label?: string
  hint?: string
  error?: string
  success?: string
  /** Show a live character count. Needs `maxLength` to be meaningful. */
  showCount?: boolean
  wrapClassName?: string
  value?: string | (() => string)
  defaultValue?: string | number
  required?: boolean
  disabled?: boolean
  rows?: number
  cols?: number
  maxLength?: number
  placeholder?: string
  name?: string
  onChange?: (event: ChangeEvent<HTMLTextAreaElement>) => void
}

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(
  function Textarea(
    {
      label,
      hint,
      error,
      success,
      showCount = false,
      wrapClassName,
      id: idProp,
      className,
      required,
      disabled,
      rows = 4,
      maxLength,
      value: valueProp,
      defaultValue,
      onChange,
      ...props
    },
    ref,
  ) {
    const generatedId = useId('textarea')
    const id = idProp ?? generatedId
    const controlled =
      valueProp === undefined ? null : accessorOf<string>(valueProp)

    const initial = controlled
      ? controlled()
      : String(defaultValue ?? '')
    const [count, setCount] = signal(initial.length)

    // A controlled value can change without a DOM event (reset, server
    // refresh), so the counter follows the accessor.
    effect(() => {
      if (controlled) setCount(controlled().length)
    })

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

    const atLimit = (): boolean =>
      maxLength !== undefined && count() >= maxLength

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
            hasError ? styles.invalid : hasSuccess ? styles.valid : undefined,
            disabled && styles.disabled,
          )}
        >
          <textarea
            {...props}
            ref={ref}
            id={id}
            rows={rows}
            required={required}
            disabled={disabled}
            maxLength={maxLength}
            value={controlled ?? defaultValue}
            aria-invalid={hasError ? 'true' : undefined}
            aria-describedby={describedBy}
            className={cx(styles.control, className)}
            onChange={(event) => {
              setCount(
                (event.currentTarget as HTMLTextAreaElement).value.length,
              )
              onChange?.(event as ChangeEvent<HTMLTextAreaElement>)
            }}
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
        </div>

        {hasError || hasSuccess || (showCount && maxLength !== undefined) ? (
          <div className={styles.message}>
            {hasError ? (
              <p id={`${id}-error`} role="alert" className={styles.errorText}>
                {error}
              </p>
            ) : hasSuccess ? (
              <p id={`${id}-success`} className={styles.successText}>
                {success}
              </p>
            ) : null}

            {showCount && maxLength !== undefined ? (
              <p
                className={() =>
                  cx(styles.counter, atLimit() && styles.counterAtLimit)
                }
              >
                {() => `${count()} / ${maxLength}`}
              </p>
            ) : null}
          </div>
        ) : null}
      </div>
    )
  },
)

Textarea.displayName = 'Textarea'
