import { signal, type Accessor } from '@vorzelajs/core'
import { createContext, useContext } from '@vorzelajs/dom'
import { accessorOf, useId } from '~/hooks/index.ts'
import { cx } from '~/lib/cx.ts'
import styles from '~/styles/components/ui/tabs.module.scss'
import type {
  ButtonHTMLAttributes,
  HTMLAttributes,
  KeyboardEvent,
  MouseEvent,
  ReactNode,
} from '~/lib/types.ts'

type Orientation = 'horizontal' | 'vertical'

interface TabsContextValue {
  value: Accessor<string>
  setValue: (next: string) => void
  baseId: string
  orientation: Orientation
}

const TabsContext = createContext<TabsContextValue | null>(null, 'tabs')

function useTabs(component: string): TabsContextValue {
  const context = useContext(TabsContext)
  if (!context) {
    throw new Error(`<${component}> must be rendered inside <Tabs>.`)
  }
  return context
}

/** Pass accessors where the runtime must update the DOM in place. */
function reactive<T>(value: () => T): T {
  return value as unknown as T
}

/**
 * Tab group. Pass children as a function so triggers and panels are created
 * inside the shared context:
 * `<Tabs>{() => <><TabsList …/><TabsContent …/></>}</Tabs>`.
 */
export interface TabsProps extends HTMLAttributes<HTMLDivElement> {
  value?: string | (() => string)
  defaultValue?: string
  onValueChange?: (value: string) => void
  orientation?: Orientation
  children?: ReactNode | (() => ReactNode)
}

export function Tabs({
  value: valueProp,
  defaultValue = '',
  onValueChange,
  orientation = 'horizontal',
  className,
  children,
  ...props
}: TabsProps) {
  const [internal, setInternal] = signal(defaultValue)
  const controlled = valueProp === undefined ? null : accessorOf(valueProp)
  const value: Accessor<string> = () =>
    controlled ? controlled() : internal()

  const setValue = (next: string): void => {
    if (!controlled) setInternal(next)
    onValueChange?.(next)
  }

  const baseId = useId('tabs')
  const context: TabsContextValue = { value, setValue, baseId, orientation }

  return (
    <TabsContext.Provider value={context}>
      {() => {
        const content =
          typeof children === 'function'
            ? (children as () => ReactNode)()
            : children

        return (
          <div
            className={cx(styles.root, className)}
            data-orientation={orientation}
            {...props}
          >
            {content}
          </div>
        )
      }}
    </TabsContext.Provider>
  )
}

export interface TabsListProps extends HTMLAttributes<HTMLDivElement> {
  onKeyDown?: (event: KeyboardEvent<HTMLDivElement>) => void
}

export function TabsList({
  className,
  onKeyDown,
  children,
  ...props
}: TabsListProps) {
  const { orientation } = useTabs('TabsList')

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>): void => {
    onKeyDown?.(event)
    if (event.defaultPrevented) return

    const forward = orientation === 'horizontal' ? 'ArrowRight' : 'ArrowDown'
    const backward = orientation === 'horizontal' ? 'ArrowLeft' : 'ArrowUp'
    const navigation = [forward, backward, 'Home', 'End']

    if (!navigation.includes(event.key)) return

    const tabs = Array.from(
      event.currentTarget.querySelectorAll<HTMLButtonElement>(
        '[role="tab"]:not([disabled])',
      ),
    )
    if (tabs.length === 0) return

    event.preventDefault()

    const current = tabs.indexOf(event.target as HTMLButtonElement)
    let next = current

    if (event.key === forward) next = (current + 1) % tabs.length
    if (event.key === backward) next = (current - 1 + tabs.length) % tabs.length
    if (event.key === 'Home') next = 0
    if (event.key === 'End') next = tabs.length - 1

    tabs[next]?.focus()
    tabs[next]?.click()
  }

  return (
    <div
      role="tablist"
      aria-orientation={orientation}
      className={cx(
        styles.list,
        orientation === 'vertical' && styles.vertical,
        className,
      )}
      onKeyDown={
        handleKeyDown as unknown as (event: globalThis.KeyboardEvent) => void
      }
      {...props}
    >
      {children}
    </div>
  )
}

export interface TabsTriggerProps
  extends ButtonHTMLAttributes<HTMLButtonElement> {
  value: string
  onClick?: (event: MouseEvent<HTMLButtonElement>) => void
}

export function TabsTrigger({
  value,
  className,
  children,
  onClick,
  ...props
}: TabsTriggerProps) {
  const tabs = useTabs('TabsTrigger')
  const selected = (): boolean => tabs.value() === value

  return (
    <button
      {...props}
      type="button"
      role="tab"
      id={`${tabs.baseId}-tab-${value}`}
      aria-selected={() => (selected() ? 'true' : 'false')}
      aria-controls={`${tabs.baseId}-panel-${value}`}
      tabIndex={reactive(() => (selected() ? 0 : -1))}
      className={cx(styles.trigger, className)}
      onClick={(event) => {
        onClick?.(event as unknown as MouseEvent<HTMLButtonElement>)
        if (!event.defaultPrevented) tabs.setValue(value)
      }}
    >
      {children}
    </button>
  )
}

export interface TabsContentProps extends HTMLAttributes<HTMLDivElement> {
  value: string
}

export function TabsContent({
  value,
  className,
  children,
  ...props
}: TabsContentProps) {
  const tabs = useTabs('TabsContent')
  const selected = (): boolean => tabs.value() === value

  return (
    <div
      role="tabpanel"
      id={`${tabs.baseId}-panel-${value}`}
      aria-labelledby={`${tabs.baseId}-tab-${value}`}
      tabIndex={0}
      hidden={reactive(() => !selected())}
      className={cx(styles.content, className)}
      {...props}
    >
      {children}
    </div>
  )
}
