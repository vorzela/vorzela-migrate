import { Menu } from './icons.tsx'

import { cx } from '~/lib/cx.ts'
import styles from '~/styles/components/ui/header.module.scss'
import type { ReactNode } from '~/lib/types.ts'

export interface HeaderProps {
  title?: ReactNode
  /** Shown on small screens — typically opens the app drawer. */
  onMenuClick?: () => void
  /** Hide the menu button entirely. */
  showMenuButton?: boolean
  actions?: ReactNode
  /** Leading slot (logo, back button) — replaces the menu button when set. */
  leading?: ReactNode
  /** Subtitle under the title. */
  description?: ReactNode
  sticky?: boolean
  className?: string
  /** Accessible name when the title is not a string. */
  'aria-label'?: string
}

/** Page header: optional menu trigger, title, description and trailing actions. */
export function Header({
  title = 'Dashboard',
  onMenuClick,
  showMenuButton = true,
  actions,
  leading,
  description,
  sticky = true,
  className,
  'aria-label': ariaLabel,
}: HeaderProps) {
  return (
    <header
      aria-label={ariaLabel}
      className={cx(styles.root, sticky && styles.sticky, className)}
    >
      {leading ??
        (showMenuButton && onMenuClick ? (
          <button
            type="button"
            className={cx(styles.menu, styles.menuAuto)}
            onClick={onMenuClick}
            aria-label="Open menu"
          >
            <Menu aria-hidden />
          </button>
        ) : null)}

      <div className={styles.titleWrap}>
        <h1 className={styles.title}>{title}</h1>
        {description ? <p className={styles.description}>{description}</p> : null}
      </div>

      {actions ? <div className={styles.actions}>{actions}</div> : null}
    </header>
  )
}
