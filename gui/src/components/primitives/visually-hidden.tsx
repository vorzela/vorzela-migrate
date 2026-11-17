import { cx } from '~/lib/cx.ts'
import type { HTMLAttributes } from '~/lib/types.ts'
import styles from '~/styles/components/primitives/primitives.module.scss'

/** Screen-reader-only content. */
export function VisuallyHidden({
  className,
  ...props
}: HTMLAttributes<HTMLSpanElement>) {
  return <span className={cx(styles.sr, className)} {...props} />
}
