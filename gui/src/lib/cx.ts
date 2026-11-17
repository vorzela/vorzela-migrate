export type ClassValue =
  | string
  | number
  | bigint
  | false
  | null
  | undefined
  | ClassValue[]

/**
 * Join class names, dropping falsy values. CSS Modules never produce
 * conflicting utilities, so no merge layer is needed.
 */
export function cx(...inputs: ClassValue[]): string {
  const out: string[] = []

  for (const input of inputs) {
    if (!input) continue

    if (Array.isArray(input)) {
      const nested = cx(...input)
      if (nested) out.push(nested)
    } else {
      out.push(String(input))
    }
  }

  return out.join(' ')
}
