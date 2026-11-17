import { getHost, jsx, type Component, type Renderable } from '@vorzelajs/dom'
import { cx } from '~/lib/cx.ts'
import type { ReactNode } from '~/lib/types.ts'
import styles from '~/styles/components/ui/icon.module.scss'

export type IconNode = [tag: string, attrs: Record<string, string | number | undefined>][]

export interface IconProps {
  /** Lucide icon node. Omit when composing with children. */
  node?: IconNode
  children?: ReactNode
  label?: string
  size?: number | string | 'sm' | 'md' | 'lg'
  strokeWidth?: number
  class?: string
  className?: string
  'aria-hidden'?: boolean | 'true' | 'false'
  'aria-label'?: string
  [key: string]: unknown
}

export interface IconComponent extends Component<IconProps> {
  readonly iconNode?: IconNode
}

const SVG_NAMESPACE = 'http://www.w3.org/2000/svg'

/**
 * The JSX runtime creates elements with `document.createElement`, which puts
 * `<svg>` in the HTML namespace and leaves it unrendered. Build icon nodes with
 * `createElementNS` on the client and fall back to the host `jsx` path during
 * SSR, where the server renderer owns element creation.
 */
function setSvgAttributes(
  element: SVGElement,
  attrs: Record<string, unknown>,
): void {
  for (const [rawName, value] of Object.entries(attrs)) {
    if (value === undefined || value === null || value === false) continue

    if (rawName === 'style' && typeof value === 'object') {
      for (const [property, propertyValue] of Object.entries(
        value as Record<string, unknown>,
      )) {
        if (typeof propertyValue === 'string' || typeof propertyValue === 'number') {
          element.style.setProperty(property, String(propertyValue))
        }
      }
      continue
    }

    if (typeof value === 'function') {
      if (rawName.startsWith('on') && rawName.length > 2) {
        element.addEventListener(
          rawName.slice(2).toLowerCase(),
          value as EventListener,
        )
      }
      continue
    }

    if (typeof value === 'object') continue
    const name = rawName === 'className' ? 'class' : rawName
    element.setAttribute(name, String(value))
  }
}

function createSvgElement(
  tag: string,
  attrs: Record<string, unknown>,
): SVGElement {
  const element = document.createElementNS(SVG_NAMESPACE, tag) as SVGElement
  setSvgAttributes(element, attrs)
  return element
}

/**
 * Renders a Lucide icon node as an inline SVG. Icons inherit `currentColor`
 * and are decorative by default.
 */
export function Icon(props: IconProps): Renderable {
  const {
    node,
    children,
    label,
    size = '1em',
    strokeWidth = 2,
    class: className,
    className: camelClassName,
    'aria-hidden': ariaHidden,
    ...rest
  } = props

  if (!node) {
    const boxSize =
      size === 'sm' || size === 'md' || size === 'lg' ? size : 'md'
    return jsx('span', {
      class: cx(styles.root, styles[boxSize], camelClassName ?? className),
      role: label ? 'img' : undefined,
      'aria-label': label,
      'aria-hidden': label ? undefined : (ariaHidden ?? true),
      ...rest,
      children,
    }) as Renderable
  }

  // Icons follow their text by default; callers pass a number when they need
  // a fixed pixel size. The named sizes match the composed-child path.
  const resolvedSize =
    size === 'sm' ? 16 : size === 'md' ? 20 : size === 'lg' ? 24 : size

  const svgAttributes: Record<string, unknown> = {
    xmlns: 'http://www.w3.org/2000/svg',
    width: resolvedSize,
    height: resolvedSize,
    viewBox: '0 0 24 24',
    fill: 'none',
    stroke: 'currentColor',
    'stroke-width': strokeWidth,
    'stroke-linecap': 'round',
    'stroke-linejoin': 'round',
    'aria-hidden': label ? undefined : (ariaHidden ?? true),
    role: label ? 'img' : undefined,
    'aria-label': label,
    class: camelClassName ?? className,
    ...rest,
  }

  // `jsdom` exposes `document` in SSR tests, so branch on the active host
  // instead of the global. The default DOM host is the only one that needs
  // `createElementNS`; every other host serialises through `jsx`.
  if (getHost().name !== 'dom') {
    return jsx('svg', {
      ...svgAttributes,
      children: node.map(([tag, attrs]) => jsx(tag, attrs)),
    }) as Renderable
  }

  const svg = createSvgElement('svg', svgAttributes)
  for (const [tag, attrs] of node) {
    svg.appendChild(createSvgElement(tag, attrs))
  }
  return svg as Renderable
}

/** Builds a named icon component from a Lucide icon node. */
export function createIcon(node: IconNode): IconComponent {
  const component = ((props: IconProps) => Icon({ ...props, node })) as IconComponent
  Object.defineProperty(component, 'iconNode', { value: node })
  return component
}

export type LucideIcon = IconComponent
