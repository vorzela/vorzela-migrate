import { cx } from '~/lib/cx.ts'
import { useId } from '~/hooks/use-id.ts'
import type { LabelHTMLAttributes, ReactNode } from '~/lib/types.ts'
import styles from '~/styles/components/ui/label.module.scss'

export interface LabelProps extends LabelHTMLAttributes<HTMLLabelElement> {
  required?: boolean
  hint?: string
  hintId?: string
  children?: ReactNode
}

export function Label({
  required,
  hint,
  hintId: hintIdProp,
  className,
  children,
  ...rest
}: LabelProps) {
  const generatedHintId = useId('hint')
  const hintId = hintIdProp ?? (hint ? generatedHintId : undefined)

  return (
    <div className={styles.wrapper}>
      <label {...rest} className={cx(styles.label, className)}>
        {children}
        {required ? (
          <span className={styles.required} aria-hidden>
            *
          </span>
        ) : null}
        {required ? <span className={styles.srOnly}> (required)</span> : null}
      </label>
      {hint ? (
        <p id={hintId} className={styles.hint}>
          {hint}
        </p>
      ) : null}
    </div>
  )
}
