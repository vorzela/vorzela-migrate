import type { Component } from '@vorzelajs/dom'

export type ReactNode = unknown
export type ReactElement = unknown
export type Key = string | number
export type CSSProperties = Record<string, string | number>
// Ported React components use this type for forwarded refs. The DOM host
// accepts callback and object refs; `any` keeps the port's render signatures
// compatible without leaking into app-facing component props.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type Ref<_T = Element> = any

export interface HTMLAttributes<T = Element> {
  class?: string
  className?: string
  style?: CSSProperties
  children?: unknown
  ref?: ((element: T | Element) => void) | { current: T | Element | null }
  id?: string
  role?: string
  title?: unknown
  hidden?: boolean
  tabIndex?: number
  'aria-label'?: string
  'aria-hidden'?: boolean | 'true' | 'false'
  onClick?: (...args: any[]) => void
  onDoubleClick?: (...args: any[]) => void
  onInput?: (...args: any[]) => void
  onChange?: (...args: any[]) => void
  onSubmit?: (...args: any[]) => void
  onKeyDown?: (...args: any[]) => void
  onKeyUp?: (...args: any[]) => void
  onKeyPress?: (...args: any[]) => void
  onFocus?: (...args: any[]) => void
  onBlur?: (...args: any[]) => void
  onPointerDown?: (...args: any[]) => void
  onPointerMove?: (...args: any[]) => void
  onPointerUp?: (...args: any[]) => void
  onPointerEnter?: (...args: any[]) => void
  onPointerLeave?: (...args: any[]) => void
  onMouseDown?: (...args: any[]) => void
  onMouseUp?: (...args: any[]) => void
  onMouseMove?: (...args: any[]) => void
  onMouseEnter?: (...args: any[]) => void
  onMouseLeave?: (...args: any[]) => void
  onContextMenu?: (...args: any[]) => void
  onDragStart?: (...args: any[]) => void
  onDragOver?: (...args: any[]) => void
  onDrop?: (...args: any[]) => void
  onScroll?: (...args: any[]) => void
  onWheel?: (...args: any[]) => void
  onTouchStart?: (...args: any[]) => void
  onTouchMove?: (...args: any[]) => void
  onTouchEnd?: (...args: any[]) => void
  [key: string]: unknown
}

export type ButtonHTMLAttributes<T = HTMLButtonElement> = HTMLAttributes<T> & {
  type?: 'button' | 'submit' | 'reset'
  disabled?: boolean
  form?: string
  name?: string
  value?: string | number
}

export type AnchorHTMLAttributes<T = HTMLAnchorElement> = HTMLAttributes<T> & {
  href?: string
  target?: string
  rel?: string
  download?: string | boolean
}

export type InputHTMLAttributes<T = HTMLInputElement> = HTMLAttributes<T> & {
  type?: string
  name?: string
  value?: string | number | (() => string | number)
  defaultValue?: string | number
  placeholder?: string
  required?: boolean
  disabled?: boolean | (() => boolean)
  readOnly?: boolean
  checked?: boolean | (() => boolean)
  defaultChecked?: boolean
  min?: string | number
  max?: string | number
  step?: string | number
  minLength?: number
  maxLength?: number
  pattern?: string
  multiple?: boolean
  accept?: string
  autoComplete?: string
  inputMode?: string
}

export type TextareaHTMLAttributes<T = HTMLTextAreaElement> = InputHTMLAttributes<T> & {
  rows?: number
  cols?: number
}

export type SelectHTMLAttributes<T = HTMLSelectElement> = HTMLAttributes<T> & {
  name?: string
  value?: string | number | (() => string | number)
  defaultValue?: string | number
  required?: boolean | (() => boolean)
  disabled?: boolean | (() => boolean)
  multiple?: boolean
  size?: number
}

export type LabelHTMLAttributes<T = HTMLLabelElement> = HTMLAttributes<T> & {
  htmlFor?: string
  for?: string
  required?: boolean
}

export type ImgHTMLAttributes<T = HTMLImageElement> = HTMLAttributes<T> & {
  src?: string
  srcSet?: string
  sizes?: string
  alt?: string
  width?: number | string
  height?: number | string
  loading?: 'eager' | 'lazy'
  decoding?: 'async' | 'sync' | 'auto'
  crossOrigin?: 'anonymous' | 'use-credentials'
}

export type TdHTMLAttributes<T = HTMLTableCellElement> = HTMLAttributes<T> & {
  colSpan?: number
  rowSpan?: number
  headers?: string
}

export type ThHTMLAttributes<T = HTMLTableCellElement> = TdHTMLAttributes<T> & {
  scope?: string
  abbr?: string
}

export type LiHTMLAttributes<T = HTMLLIElement> = HTMLAttributes<T>

export type ComponentProps<T> = HTMLAttributes<
  T extends keyof HTMLElementTagNameMap ? HTMLElementTagNameMap[T] : Element
>

export type ComponentPropsWithoutRef<T> = ComponentProps<T>

export type ChangeEvent<T = HTMLInputElement> = Event & {
  currentTarget: T
  target: T
}

export type MouseEvent<T = Element> = globalThis.MouseEvent & {
  currentTarget: T
  target: EventTarget | null
}

export type KeyboardEvent<T = Element> = globalThis.KeyboardEvent & {
  currentTarget: T
  target: EventTarget | null
}

export type PointerEvent<T = Element> = globalThis.PointerEvent & {
  currentTarget: T
  target: EventTarget | null
}

export type ClipboardEvent<T = Element> = globalThis.ClipboardEvent & {
  currentTarget: T
  target: EventTarget | null
}

export type DragEvent<T = Element> = globalThis.DragEvent & {
  currentTarget: T
  target: EventTarget | null
}

export type SyntheticEvent<T = Element> = Event & {
  currentTarget: T
  target: EventTarget | null
}

export type MutableRefObject<T> = { current: T }

/**
 * React-shaped forwardRef adapter. Vorzela passes `ref` as a prop; this helper
 * keeps the familiar two-argument render signature for ported components.
 */
export function forwardRef<T, P>(
  render: (props: P, ref: Ref<T>) => unknown,
): Component<P & { ref?: ((element: Element) => void) | { current: Element | null } }> & {
  displayName?: string
} {
  return ((props: P & { ref?: unknown }) =>
    render(props, props.ref ?? null)) as Component<
    P & { ref?: ((element: Element) => void) | { current: Element | null } }
  > & { displayName?: string }
}

export function isValidElement(value: unknown): boolean {
  return !!value && typeof value === 'object' && 'nodeType' in value
}
