import { effect, onCleanup, signal } from '@vorzelajs/core'
import { For, Show } from '@vorzelajs/dom'
import {
  accessorOf,
  useMediaQuery,
  useMounted,
} from '~/hooks/index.ts'
import { cx } from '~/lib/cx.ts'
import { lockBodyScroll, trapFocus } from '~/lib/overlay.ts'
import { ChevronDown, ChevronLeft, ChevronRight, X } from './icons.tsx'
import styles from '~/styles/components/ui/sidebar.module.scss'
import type { ReactNode } from '~/lib/types.ts'

export type SidebarNavChild = {
  label: string
  href: string
  icon?: ReactNode
  permission?: string
}

export type SidebarNavItem = {
  label: string
  href?: string
  icon?: ReactNode
  permission?: string
  children?: SidebarNavChild[]
}

export type SidebarNavGroup = {
  group: string
  items: SidebarNavItem[]
  /** Allow the whole group to be collapsed from its heading. */
  collapsible?: boolean
  defaultOpen?: boolean
}

export interface SidebarProps {
  groups: SidebarNavGroup[]
  brand?: ReactNode
  /** Small square mark shown before the brand text. */
  brandMark?: ReactNode
  subtitle?: string
  mobileOpen?: boolean | (() => boolean)
  onMobileClose?: () => void
  footer?: ReactNode
  version?: string
  hasPermission?: (permission: string) => boolean
  defaultCollapsed?: boolean
  drawerSide?: 'left' | 'right'
  /** Render in-flow inside a frame (docs, settings previews). */
  contained?: boolean
  /** Current route. Pass `location.pathname` or your hash path. */
  currentPath?: string | (() => string)
}

function pathMatches(pathname: string, href: string): boolean {
  return pathname === href || pathname.startsWith(`${href}/`)
}

function isHrefActive(
  pathname: string,
  href: string,
  siblingHrefs: string[],
): boolean {
  if (pathname === href) return true
  if (!pathname.startsWith(`${href}/`)) return false
  return !siblingHrefs.some(
    (other) =>
      other !== href &&
      other.length > href.length &&
      (pathname === other || pathname.startsWith(`${other}/`)),
  )
}

/** Pass accessors where the runtime must update the DOM in place. */
function reactive<T>(value: () => T): T {
  return value as unknown as T
}

function NavIcon({
  icon,
  collapsed,
  label,
}: {
  icon?: ReactNode
  collapsed: () => boolean
  label: string
}) {
  if (icon) {
    return <span className={styles.itemIcon}>{icon}</span>
  }
  return (
    <Show when={collapsed}>
      <span
        className={cx(styles.itemIcon, styles.itemFallback)}
        aria-hidden
      >
        {String(label).slice(0, 1)}
      </span>
    </Show>
  )
}

/** Responsive navigation with a collapsible desktop rail and nested groups. */
export function Sidebar({
  groups,
  brand = 'Vorzela UI',
  brandMark,
  subtitle,
  mobileOpen = false,
  onMobileClose,
  footer,
  version,
  hasPermission,
  defaultCollapsed = false,
  drawerSide = 'left',
  contained = false,
  currentPath,
}: SidebarProps) {
  const [collapsedDesktop, setCollapsedDesktop] = signal(defaultCollapsed)
  const [collapsedMobile, setCollapsedMobile] = signal(false)
  const [openItems, setOpenItems] = signal<Record<string, boolean>>({})
  const [closedGroups, setClosedGroups] = signal<Record<string, boolean>>({})
  const isDesktop = useMediaQuery('(min-width: 64rem)')
  const isMobileOpen = accessorOf(mobileOpen)
  const collapsed = (): boolean =>
    isDesktop() ? collapsedDesktop() : collapsedMobile()
  const isRight = drawerSide === 'right'
  const [aside, setAside] = signal<HTMLElement | null>(null)
  const mounted = useMounted()
  const controlledPath = currentPath === undefined ? null : accessorOf(currentPath)

  // Resolve the route only after hydration. Reading `window.location` during
  // the first render would mark a different item active than the server did.
  const resolvedPath = (): string => {
    if (controlledPath) return controlledPath()
    if (typeof window === 'undefined' || !mounted()) return ''
    return `${window.location.pathname}${window.location.hash}`
  }

  const closeMobile = (): void => {
    setCollapsedMobile(false)
    onMobileClose?.()
  }

  effect(() => {
    if (!isMobileOpen() || typeof document === 'undefined') return
    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.key === 'Escape') closeMobile()
    }
    document.addEventListener('keydown', onKeyDown)
    onCleanup(() => document.removeEventListener('keydown', onKeyDown))
  })

  // Close the drawer when the viewport becomes desktop-sized.
  effect(() => {
    if (isDesktop() && isMobileOpen()) closeMobile()
  })

  // Keep the page behind the mobile drawer still and trap keyboard focus.
  effect(() => {
    const node = aside()
    if (!isMobileOpen() || !node || typeof document === 'undefined') return
    const release = lockBodyScroll()
    const untrap = trapFocus(node, closeMobile)
    onCleanup(() => {
      release()
      untrap()
    })
  })

  const can = (permission: string): boolean =>
    hasPermission?.(permission) ?? true

  const visibleGroups = groups
    .map((group) => ({
      ...group,
      items: group.items
        .filter((item) => !item.permission || can(item.permission))
        .map((item) => ({
          ...item,
          children: item.children?.filter(
            (child) => !child.permission || can(child.permission),
          ),
        }))
        .filter(
          (item) => !item.children || item.children.length > 0 || item.href,
        ),
    }))
    .filter((group) => group.items.length > 0)

  const allHrefs: string[] = []
  for (const group of visibleGroups) {
    for (const item of group.items) {
      if (item.href) allHrefs.push(item.href)
      for (const child of item.children ?? []) allHrefs.push(child.href)
    }
  }

  const isItemOpenByRoute = (item: SidebarNavItem): boolean =>
    Boolean(item.children?.some((child) => pathMatches(resolvedPath(), child.href)))

  const groupHasActiveItem = (group: SidebarNavGroup): boolean =>
    group.items.some(
      (item) =>
        (item.href && isHrefActive(resolvedPath(), item.href, allHrefs)) ||
        item.children?.some((child) =>
          pathMatches(resolvedPath(), child.href),
        ),
    )

  // Kept lazy: reading the route eagerly while `For` renders its rows would
  // make the list re-run on navigation and dispose the row bindings.
  const groupOpen = (group: SidebarNavGroup): boolean =>
    closedGroups()[group.group] ??
    (groupHasActiveItem(group) ? true : (group.defaultOpen ?? true))

  const toggleCollapsed = (): void => {
    if (isDesktop()) {
      setCollapsedDesktop((value) => !value)
      return
    }
    setCollapsedMobile((value) => !value)
  }

  const handleParent = (item: SidebarNavItem): void => {
    if (collapsed()) {
      setCollapsedDesktop(false)
      setCollapsedMobile(false)
      setOpenItems((open) => ({ ...open, [item.label]: true }))
      return
    }
    setOpenItems((open) => ({
      ...open,
      [item.label]: !(open[item.label] ?? isItemOpenByRoute(item)),
    }))
  }

  return (
    <>
      <Show when={isMobileOpen}>
        {() => (
          <button
            type="button"
            aria-label="Close navigation"
            className={styles.overlay}
            onClick={closeMobile}
          />
        )}
      </Show>

      <aside
        ref={(node) => setAside(node as HTMLElement)}
        className={reactive(() =>
          cx(
            styles.root,
            collapsed() && styles.collapsed,
            isRight && styles.right,
            isMobileOpen() && styles.open,
            contained && styles.contained,
          ),
        )}
        aria-label="Main navigation"
      >
        <div className={styles.header}>
          {brandMark ? (
            <span className={styles.brandMark}>{brandMark}</span>
          ) : null}
          <Show when={() => !collapsed()}>
            {() => (
              <div className={styles.brand}>
                <span className={styles.brandName}>{brand}</span>
                {subtitle ? (
                  <p className={styles.brandSubtitle}>{subtitle}</p>
                ) : null}
              </div>
            )}
          </Show>

          <button
            type="button"
            className={styles.close}
            onClick={closeMobile}
            aria-label="Close menu"
          >
            <X aria-hidden />
          </button>
        </div>

        <nav className={styles.nav}>
          <For each={() => visibleGroups} key={(group) => group.group}>
            {(group) => {
              return (
                <div className={styles.group}>
                  <Show when={() => !collapsed()}>
                    {() =>
                      group.collapsible ? (
                        <button
                          type="button"
                          className={styles.groupHeadButton}
                          aria-expanded={() =>
                            groupOpen(group) ? 'true' : 'false'
                          }
                          onClick={() =>
                            setClosedGroups((state) => ({
                              ...state,
                              [group.group]: !groupOpen(group),
                            }))
                          }
                        >
                          <span className={styles.groupLabel}>
                            {group.group}
                          </span>
                          <span className={styles.groupChevron} aria-hidden>
                            <ChevronDown />
                          </span>
                        </button>
                      ) : (
                        <div className={styles.groupHead}>
                          <span className={styles.groupLabel}>
                            {group.group}
                          </span>
                        </div>
                      )
                    }
                  </Show>

                  <Show when={() => groupOpen(group)}>
                    {() => (
                      <For each={() => group.items} key={(item) => item.label}>
                        {(item) => {
                          const hasChildren = Boolean(item.children?.length)
                          const itemOpen = (): boolean =>
                            openItems()[item.label] ??
                            isItemOpenByRoute(item)
                          const active = (): boolean =>
                            item.href
                              ? isHrefActive(
                                  resolvedPath(),
                                  item.href,
                                  allHrefs,
                                )
                              : Boolean(
                                  item.children?.some((child) =>
                                    pathMatches(resolvedPath(), child.href),
                                  ),
                                )

                          if (!hasChildren) {
                            return (
                              <a
                                href={item.href}
                                className={reactive(() =>
                                  cx(
                                    styles.item,
                                    active() && styles.itemActive,
                                  ),
                                )}
                                aria-current={() =>
                                  active() ? 'page' : undefined
                                }
                                aria-label={reactive(() =>
                                  collapsed() ? item.label : undefined,
                                )}
                                title={reactive(() =>
                                  collapsed() ? item.label : undefined,
                                )}
                                data-label={reactive(() =>
                                  collapsed() ? item.label : undefined,
                                )}
                                onClick={() => {
                                  if (isMobileOpen()) closeMobile()
                                }}
                              >
                                <NavIcon icon={item.icon} collapsed={collapsed} label={item.label} />
                                <Show when={() => !collapsed()}>
                                  {() => (
                                    <span className={styles.itemLabel}>
                                      {item.label}
                                    </span>
                                  )}
                                </Show>
                              </a>
                            )
                          }

                          return (
                            <div>
                              <button
                                type="button"
                                className={reactive(() =>
                                  cx(
                                    styles.item,
                                    active() && styles.itemActive,
                                    itemOpen() && styles.itemOpen,
                                  ),
                                )}
                                aria-expanded={reactive(() =>
                                  collapsed()
                                    ? undefined
                                    : itemOpen()
                                      ? 'true'
                                      : 'false',
                                )}
                                aria-label={reactive(() =>
                                  collapsed() ? item.label : undefined,
                                )}
                                title={reactive(() =>
                                  collapsed() ? item.label : undefined,
                                )}
                                data-label={reactive(() =>
                                  collapsed() ? item.label : undefined,
                                )}
                                onClick={() => handleParent(item)}
                              >
                                <NavIcon icon={item.icon} collapsed={collapsed} label={item.label} />
                                <Show when={() => !collapsed()}>
                                  {() => (
                                    <>
                                      <span className={styles.itemLabel}>
                                        {item.label}
                                      </span>
                                      <span className={styles.chevron}>
                                        <ChevronDown aria-hidden />
                                      </span>
                                    </>
                                  )}
                                </Show>
                              </button>

                              <Show when={() => !collapsed()}>
                                {() => (
                                  <div
                                    className={styles.childrenWrap}
                                    data-open={() =>
                                      itemOpen() ? 'true' : 'false'
                                    }
                                    inert={() =>
                                      itemOpen() ? undefined : true
                                    }
                                  >
                                    <div className={styles.childrenInner}>
                                      <div className={styles.children}>
                                        <For
                                          each={() => item.children ?? []}
                                          key={(child) => child.href}
                                        >
                                          {(child) => {
                                            const childActive = (): boolean =>
                                              isHrefActive(
                                                resolvedPath(),
                                                child.href,
                                                allHrefs,
                                              )

                                            return (
                                              <a
                                                href={child.href}
                                                className={reactive(() =>
                                                  cx(
                                                    styles.item,
                                                    styles.child,
                                                    childActive() &&
                                                      styles.itemActive,
                                                  ),
                                                )}
                                                aria-current={() =>
                                                  childActive()
                                                    ? 'page'
                                                    : undefined
                                                }
                                                onClick={() => {
                                                  if (isMobileOpen()) {
                                                    closeMobile()
                                                  }
                                                }}
                                              >
                                                {child.icon ? (
                                                  <span
                                                    className={styles.itemIcon}
                                                  >
                                                    {child.icon}
                                                  </span>
                                                ) : null}
                                                <span
                                                  className={styles.itemLabel}
                                                >
                                                  {child.label}
                                                </span>
                                              </a>
                                            )
                                          }}
                                        </For>
                                      </div>
                                    </div>
                                  </div>
                                )}
                              </Show>
                            </div>
                          )
                        }}
                      </For>
                    )}
                  </Show>
                </div>
              )
            }}
          </For>
        </nav>

        <div className={styles.footer}>
          {version ? <p className={styles.version}>v{version}</p> : null}
          <Show when={() => !collapsed() && Boolean(footer)}>
            {() => <div className={styles.footerContent}>{footer}</div>}
          </Show>
          <button
            type="button"
            className={styles.collapse}
            onClick={toggleCollapsed}
            aria-label={reactive(() =>
              collapsed() ? 'Expand sidebar' : 'Collapse sidebar',
            )}
            aria-expanded={reactive(() =>
              collapsed() ? 'false' : 'true',
            )}
            title={reactive(() =>
              collapsed() ? 'Expand sidebar' : 'Collapse sidebar',
            )}
            data-label={reactive(() =>
              collapsed() ? 'Expand sidebar' : undefined,
            )}
          >
            <Show when={collapsed} fallback={<ChevronLeft aria-hidden />}>
              {() => <ChevronRight aria-hidden />}
            </Show>
            <Show when={() => !collapsed()}>
              {() => <span>Collapse</span>}
            </Show>
          </button>
        </div>
      </aside>
    </>
  )
}
