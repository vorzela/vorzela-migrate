import { signal } from '@vorzelajs/core'

import { accessorOf, useId } from '~/hooks/index.ts'
import { cx } from '~/lib/cx.ts'
import type { ReactNode } from '~/lib/types.ts'
import styles from '~/styles/components/ui/switch.module.scss'

export type SwitchSize = 'sm' | 'md' | 'lg'

const SIZE: Record<SwitchSize, string> = {
  sm: styles.sm,
  md: '',
  lg: styles.lg,
}

export interface SwitchProps {
  checked?: boolean | (() => boolean)
  defaultChecked?: boolean
  onCheckedChange?: (checked: boolean) => void
  label?: ReactNode
  description?: string
  /** Put the switch after the label instead of before it. */
  reverse?: boolean
  size?: SwitchSize
  disabled?: boolean
  id?: string
  name?: string
  value?: string
  className?: string
}

export function Switch({
  checked: checkedProp,
  defaultChecked = false,
  onCheckedChange,
  label,
  description,
  reverse = false,
  size = 'md',
  disabled = false,
  id: idProp,
  name,
  value,
  className,
}: SwitchProps) {
  const generatedId = useId('switch')
  const id = idProp ?? generatedId
  const labelId = `${id}-label`
  const descriptionId = description ? `${id}-description` : undefined

  const [uncontrolled, setUncontrolled] = signal(defaultChecked)
  const controlled =
    checkedProp === undefined ? null : accessorOf<boolean>(checkedProp)
  const checked = (): boolean => (controlled ? controlled() : uncontrolled())

  const toggle = (): void => {
    const next = !checked()
    if (!controlled) setUncontrolled(next)
    onCheckedChange?.(next)
  }

  const control = (
    <button
      type="button"
      role="switch"
      id={id}
      name={name}
      value={value}
      aria-checked={() => (checked() ? 'true' : 'false')}
      aria-describedby={descriptionId}
      aria-label={label ? undefined : 'Toggle'}
      aria-labelledby={label ? labelId : undefined}
      disabled={disabled}
      className={cx(styles.switch, SIZE[size], className)}
      onClick={toggle}
    >
      <span className={styles.thumb} />
    </button>
  )

  if (!label && !description) return control

  const text = (
    <span className={styles.text}>
      {label ? (
        <label htmlFor={id} id={labelId} className={styles.label}>
          {label}
        </label>
      ) : null}
      {description ? (
        <p id={descriptionId} className={styles.description}>
          {description}
        </p>
      ) : null}
    </span>
  )

  return (
    <div className={styles.field} data-reverse={reverse ? 'true' : undefined}>
      {reverse ? (
        <>
          {text}
          {control}
        </>
      ) : (
        <>
          {control}
          {text}
        </>
      )}
    </div>
  )
}
