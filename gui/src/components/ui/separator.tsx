
import { cx } from '~/lib/cx.ts'
import styles from '~/styles/components/ui/separator.module.scss'
import type { HTMLAttributes } from '~/lib/types.ts'

export interface SeparatorProps extends HTMLAttributes<HTMLDivElement> {
  orientation?: 'horizontal' | 'vertical'
  /** Optional centred caption for horizontal rules. */
  label?: string
}

export function Separator({
  orientation = 'horizontal',
  label,
  className,
  ...props
}: SeparatorProps) {
  if (label && orientation === 'horizontal') {
    return (
      <div
        role="separator"
        aria-orientation="horizontal"
        className={cx(styles.root, styles.withLabel, className)}
        {...props}
      >
        <span className={styles.line} />
        <span className={styles.label}>{label}</span>
        <span className={styles.line} />
      </div>
    )
  }

  return (
    <div
      role="separator"
      aria-orientation={orientation}
      className={cx(
        styles.root,
        orientation === 'vertical' ? styles.vertical : styles.horizontal,
        className,
      )}
      {...props}
    />
  )
}
