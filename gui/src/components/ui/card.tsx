
import { cx } from '~/lib/cx.ts'
import styles from '~/styles/components/ui/card.module.scss'
import type { HTMLAttributes } from '~/lib/types.ts'

export interface CardProps extends HTMLAttributes<HTMLDivElement> {
  /** Remove elevation for a quieter, bordered surface. */
  flat?: boolean
  /** Adds hover lift — use when the whole card is a link. */
  interactive?: boolean
  /** Apply the shared inner padding (for simple cards without sections). */
  padded?: boolean
}

export function Card({
  flat = false,
  interactive = false,
  padded = false,
  className,
  ...props
}: CardProps) {
  return (
    <div
      className={cx(
        styles.root,
        flat && styles.flat,
        interactive && styles.interactive,
        padded && styles.padded,
        className,
      )}
      {...props}
    />
  )
}

export function CardHeader({
  className,
  ...props
}: HTMLAttributes<HTMLDivElement>) {
  return <div className={cx(styles.header, className)} {...props} />
}

export function CardTitle({
  className,
  ...props
}: HTMLAttributes<HTMLHeadingElement>) {
  return <h3 className={cx(styles.title, className)} {...props} />
}

export function CardDescription({
  className,
  ...props
}: HTMLAttributes<HTMLParagraphElement>) {
  return <p className={cx(styles.description, className)} {...props} />
}

export function CardContent({
  className,
  ...props
}: HTMLAttributes<HTMLDivElement>) {
  return <div className={cx(styles.content, className)} {...props} />
}

export function CardFooter({
  className,
  ...props
}: HTMLAttributes<HTMLDivElement>) {
  return <div className={cx(styles.footer, className)} {...props} />
}
