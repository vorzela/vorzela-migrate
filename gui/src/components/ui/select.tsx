import { effect, onCleanup, signal } from '@vorzelajs/core'
import { For, Show } from '@vorzelajs/dom'
import { accessorOf, useId } from '~/hooks/index.ts'
import { cx } from '~/lib/cx.ts'
import type { ReactNode } from '~/lib/types.ts'
import { Portal } from '~/components/primitives/portal.tsx'
import { Check, ChevronDown, Search } from './icons.tsx'
import { Label } from './label.tsx'
import styles from '~/styles/components/ui/select.module.scss'

export interface SelectOption {
  value: string
  label: string
  description?: string
}

export interface SelectProps {
  options: SelectOption[] | (() => SelectOption[])
  value?: string | (() => string)
  defaultValue?: string
  onChange?: (value: string) => void
  onBlur?: () => void
  placeholder?: string
  searchable?: boolean
  label?: string
  hint?: string
  error?: string
  required?: boolean
  disabled?: boolean
  wrapClassName?: string
  className?: string
  id?: string
  leadingNode?: ReactNode
  appearance?: 'default' | 'outlined' | 'filled'
}

interface Position {
  left: number
  top: number
  bottom: number
  width: number
  up: boolean
}

/** Searchable select with keyboard navigation and a portalled panel. */
export function Select({
  options,
  value: valueProp,
  defaultValue = '',
  onChange,
  onBlur,
  placeholder = 'Select…',
  searchable,
  label,
  hint,
  error,
  required,
  disabled,
  wrapClassName,
  className,
  id: idProp,
  leadingNode,
  appearance = 'default',
}: SelectProps) {
  const id = idProp ?? useId('select')
  const list = typeof options === 'function' ? options : () => options
  const isControlled = valueProp !== undefined
  const controlled = isControlled ? accessorOf(valueProp) : null
  const [uncontrolled, setUncontrolled] = signal(defaultValue)
  const value = (): string => (controlled ? controlled() : uncontrolled())
  const selected = () => list().find((option) => option.value === value())

  const [open, setOpen] = signal(false)
  const [search, setSearch] = signal('')
  const [active, setActive] = signal(-1)
  const [pos, setPos] = signal<Position>({
    left: 0,
    top: 0,
    bottom: 0,
    width: 0,
    up: false,
  })
  const [trigger, setTrigger] = signal<HTMLButtonElement | null>(null)
  const [panel, setPanel] = signal<HTMLDivElement | null>(null)
  const [searchInput, setSearchInput] = signal<HTMLInputElement | null>(null)
  const [listNode, setListNode] = signal<HTMLDivElement | null>(null)

  const filtered = (): SelectOption[] => {
    const query = search().trim().toLowerCase()
    if (!query || !searchable) return list()
    return list().filter(
      (option) =>
        option.label.toLowerCase().includes(query) ||
        option.value.toLowerCase().includes(query) ||
        (option.description?.toLowerCase().includes(query) ?? false),
    )
  }

  const measure = (): void => {
    const node = trigger()
    if (!node) return
    const rect = node.getBoundingClientRect()
    const spaceBelow = window.innerHeight - rect.bottom
    const spaceAbove = rect.top
    setPos({
      left: rect.left,
      top: rect.bottom,
      bottom: rect.top,
      width: rect.width,
      up: spaceBelow < 280 && spaceAbove > spaceBelow,
    })
  }

  const close = (returnFocus = true): void => {
    const wasOpen = open()
    setOpen(false)
    setSearch('')
    setActive(-1)
    if (returnFocus) {
      queueMicrotask(() => trigger()?.focus({ preventScroll: true }))
    }
    if (wasOpen) queueMicrotask(() => onBlur?.())
  }

  const openPanel = (): void => {
    if (disabled) return
    measure()
    setActive(-1)
    setOpen(true)
  }

  const choose = (next: string): void => {
    if (!isControlled) setUncontrolled(next)
    if (value() !== next) onChange?.(next)
    close()
  }

  effect(() => {
    if (!open()) return
    if (searchable) {
      queueMicrotask(() => searchInput()?.focus({ preventScroll: true }))
    } else {
      queueMicrotask(() => listNode()?.focus({ preventScroll: true }))
    }
  })

  effect(() => {
    if (!open()) return

    const onPointerDown = (event: PointerEvent): void => {
      const target = event.target as Node
      if (trigger()?.contains(target) || panel()?.contains(target)) return
      close(false)
    }
    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.key === 'Escape') {
        event.stopPropagation()
        close()
      }
    }
    const onViewportChange = (): void => measure()

    document.addEventListener('pointerdown', onPointerDown)
    document.addEventListener('keydown', onKeyDown, { capture: true })
    window.addEventListener('scroll', onViewportChange, {
      passive: true,
      capture: true,
    })
    window.addEventListener('resize', onViewportChange, { passive: true })
    onCleanup(() => {
      document.removeEventListener('pointerdown', onPointerDown)
      document.removeEventListener('keydown', onKeyDown, { capture: true })
      window.removeEventListener('scroll', onViewportChange, { capture: true })
      window.removeEventListener('resize', onViewportChange)
    })
  })

  const onPanelKeyDown = (event: KeyboardEvent): void => {
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      setActive((index) => Math.min(index + 1, filtered().length - 1))
    } else if (event.key === 'ArrowUp') {
      event.preventDefault()
      setActive((index) => Math.max(index - 1, 0))
    } else if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      const option = filtered()[active()]
      if (option) choose(option.value)
    }
  }

  return (
    <div className={cx(styles.field, wrapClassName)}>
      {label ? (
        <Label htmlFor={id} required={required} hint={hint} hintId={`${id}-hint`}>
          {label}
        </Label>
      ) : null}

      <button
        ref={(node) => setTrigger(node as HTMLButtonElement)}
        id={id}
        type="button"
        role="combobox"
        aria-expanded={open}
        aria-controls={`${id}-panel`}
        aria-haspopup="listbox"
        aria-invalid={error ? true : undefined}
        aria-describedby={
          error ? `${id}-error` : hint ? `${id}-hint` : undefined
        }
        disabled={disabled}
        className={cx(
          styles.trigger,
          appearance === 'outlined' && styles.outlined,
          appearance === 'filled' && styles.filled,
          error && styles.invalid,
          className,
        )}
        onClick={() => (open() ? close() : openPanel())}
        onKeyDown={(event) => {
          const key = (event as KeyboardEvent).key
          if (key === 'Enter' || key === ' ' || key === 'ArrowDown') {
            event.preventDefault()
            if (!open()) openPanel()
          }
        }}
      >
        {leadingNode ? <span className={styles.leading}>{leadingNode}</span> : null}
        <span className={cx(styles.value, !selected() && styles.placeholder)}>
          {selected()?.label ?? placeholder}
        </span>
        <ChevronDown className={styles.chevron} aria-hidden />
      </button>

      <Show when={open}>
        {() => (
          <Portal>
            <div
              ref={(node) => setPanel(node as HTMLDivElement)}
              id={`${id}-panel`}
              className={styles.panel}
              style={{
                left: `${pos().left}px`,
                width: `${pos().width}px`,
                ...(pos().up
                  ? { bottom: `${window.innerHeight - pos().bottom}px` }
                  : { top: `${pos().top}px` }),
              }}
              onKeyDown={onPanelKeyDown}
            >
              {searchable ? (
                <div className={styles.searchRow}>
                  <Search className={styles.searchIcon} aria-hidden />
                  <input
                    ref={(node) => setSearchInput(node as HTMLInputElement)}
                    className={styles.search}
                    value={search}
                    placeholder="Search…"
                    onInput={(event) =>
                      setSearch((event.target as HTMLInputElement).value)
                    }
                    aria-label="Search options"
                  />
                </div>
              ) : null}

              <div
                ref={(node) => setListNode(node as HTMLDivElement)}
                className={styles.list}
                role="listbox"
                tabIndex={searchable ? -1 : 0}
                aria-label={label ?? 'Options'}
              >
                <For each={filtered}>
                  {(option, index) => (
                    <button
                      type="button"
                      role="option"
                      aria-selected={option.value === value()}
                      disabled={disabled}
                      className={cx(
                        styles.option,
                        index === active() && styles.optionActive,
                        option.value === value() && styles.optionSelected,
                      )}
                      onMouseEnter={() => setActive(index)}
                      onClick={() => choose(option.value)}
                    >
                      <span className={styles.optionCopy}>
                        <span className={styles.optionLabel}>{option.label}</span>
                        {option.description ? (
                          <span className={styles.optionDescription}>
                            {option.description}
                          </span>
                        ) : null}
                      </span>
                      {option.value === value() ? (
                        <Check className={styles.check} aria-hidden />
                      ) : null}
                    </button>
                  )}
                </For>

                <Show when={() => filtered().length === 0}>
                  <p className={styles.empty}>No results</p>
                </Show>
              </div>
            </div>
          </Portal>
        )}
      </Show>

      {error ? (
        <p id={`${id}-error`} role="alert" className={cx(styles.message, styles.errorText)}>
          {error}
        </p>
      ) : null}
    </div>
  )
}
