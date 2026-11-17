
import { cx } from '~/lib/cx.ts'
import { Button } from './button.tsx'
import styles from '~/styles/components/ui/empty.module.scss'
import type { ReactNode } from '~/lib/types.ts'

export interface EmptyProps {
  icon?: ReactNode
  title: string
  description?: string
  action?: { label: string; onClick: () => void; className?: string }
  className?: string
  iconClassName?: string
  titleClassName?: string
  descriptionClassName?: string
}

export function Empty({
  icon,
  title,
  description,
  action,
  className,
  iconClassName,
  titleClassName,
  descriptionClassName,
}: EmptyProps) {
  return (
    <div className={cx(styles.root, className)}>
      {icon ? (
        <div className={cx(styles.icon, iconClassName)}>{icon}</div>
      ) : null}
      <h3 className={cx(styles.title, titleClassName)}>{title}</h3>
      {description ? (
        <p className={cx(styles.description, descriptionClassName)}>
          {description}
        </p>
      ) : null}
      {action ? (
        <Button
          className={cx(styles.action, action.className)}
          size="sm"
          onClick={action.onClick}
        >
          {action.label}
        </Button>
      ) : null}
    </div>
  )
}
