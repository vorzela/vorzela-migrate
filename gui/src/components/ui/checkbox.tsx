import { effect, signal } from '@vorzelajs/core'
import { accessorOf, useId } from '~/hooks/index.ts'
import { cx } from '~/lib/cx.ts'
import type { ReactNode } from '~/lib/types.ts'
import { Check, Minus } from './icons.tsx'
import styles from '~/styles/components/ui/checkbox.module.scss'

export interface CheckboxProps {
  checked?: boolean | (() => boolean)
  defaultChecked?: boolean
  indeterminate?: boolean | (() => boolean)
  onChange?: (checked: boolean) => void
  onCheckedChange?: (checked: boolean) => void
  disabled?: boolean
  label?: ReactNode
  hint?: string
  error?: string
  required?: boolean
  id?: string
  name?: string
  className?: string
  inputClassName?: string
  'aria-label'?: string
}

/**
 * Accessible checkbox. Controlled `checked` may be a signal accessor; the
 * native property and `indeterminate` flag update without re-running the
 * component.
 */
export function Checkbox({
  checked: checkedProp,
  defaultChecked = false,
  indeterminate = false,
  onChange,
  onCheckedChange,
  disabled,
  label,
  hint,
  error,
  required,
  id: idProp,
  name,
  className,
  inputClassName,
  'aria-label': ariaLabel,
}: CheckboxProps) {
  const id = idProp ?? useId('checkbox')
  const inputRef: { current: HTMLInputElement | null } = { current: null }
  const [uncontrolled, setUncontrolled] = signal(defaultChecked)
  const isControlled = checkedProp !== undefined
  const controlled = isControlled ? accessorOf(checkedProp) : null
  const checked = () => (controlled ? controlled() : uncontrolled())
  const isIndeterminate = accessorOf(indeterminate)

  effect(() => {
    const node = inputRef.current
    if (node) node.indeterminate = isIndeterminate()
  })

  const notify = (next: boolean): void => {
    if (!isControlled) setUncontrolled(next)
    onChange?.(next)
    onCheckedChange?.(next)
  }

  return (
    <div className={cx(styles.field, className)}>
      <label
        className={cx(styles.row, disabled && styles.rowDisabled)}
        data-disabled={disabled ? 'true' : undefined}
      >
        <span className={styles.controlWrap}>
          <input
            ref={inputRef}
            type="checkbox"
            id={id}
            name={name}
            checked={checked}
            disabled={disabled}
            required={required}
            aria-label={ariaLabel}
            aria-checked={() => (isIndeterminate() ? 'mixed' : checked())}
            aria-invalid={error ? true : undefined}
            aria-describedby={
              error ? `${id}-error` : hint ? `${id}-hint` : undefined
            }
            onChange={(event) =>
              notify((event.currentTarget as HTMLInputElement).checked)
            }
            className={cx(styles.control, inputClassName)}
          />
          <span className={styles.box} aria-hidden>
            <Check className={styles.check} />
            <Minus className={styles.dash} />
          </span>
        </span>

        {label ? (
          <span className={styles.label}>
            {label}
            {required ? (
              <span className={styles.required} aria-hidden>
                *
              </span>
            ) : null}
          </span>
        ) : null}
      </label>

      {hint && !error ? (
        <p id={`${id}-hint`} className={styles.hint}>
          {hint}
        </p>
      ) : null}

      {error ? (
        <p id={`${id}-error`} role="alert" className={styles.error}>
          {error}
        </p>
      ) : null}
    </div>
  )
}
