
import { Header } from './header.tsx'
import type { ReactNode } from '~/lib/types.ts'

export interface TopbarProps {
  title?: ReactNode
  onMenuClick?: () => void
  actions?: ReactNode
  className?: string
}

/** Compatibility alias for `Header`; kept for the legacy app-shell API. */
export function Topbar({ title, onMenuClick, actions, className }: TopbarProps) {
  return (
    <Header
      title={title}
      onMenuClick={onMenuClick}
      actions={actions}
      className={className}
      sticky={false}
    />
  )
}
