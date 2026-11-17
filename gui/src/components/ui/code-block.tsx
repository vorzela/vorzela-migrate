import { onCleanup, signal } from '@vorzelajs/core'
import { cx } from '~/lib/cx.ts'
import { Check, Copy } from './icons.tsx'
import styles from '~/styles/components/ui/code-block.module.scss'

export interface CodeBlockProps {
  code: string
  language?: string
  title?: string
  copyable?: boolean
  className?: string
}

/** Read-only code block with an optional copy affordance. */
export function CodeBlock({
  code,
  language,
  title,
  copyable = true,
  className,
}: CodeBlockProps) {
  const [copied, setCopied] = signal(false)
  let timer: ReturnType<typeof setTimeout> | null = null

  onCleanup(() => {
    if (timer) clearTimeout(timer)
  })

  async function copy(): Promise<void> {
    try {
      await navigator.clipboard.writeText(code)
      setCopied(true)
      if (timer) clearTimeout(timer)
      timer = setTimeout(() => {
        setCopied(false)
        timer = null
      }, 1500)
    } catch {
      setCopied(false)
    }
  }

  return (
    <figure className={cx(styles.root, className)}>
      {title || language || copyable ? (
        <figcaption className={styles.header}>
          <span className={styles.title}>{title ?? language ?? 'Code'}</span>
          {copyable ? (
            <button
              type="button"
              className={styles.copy}
              onClick={() => void copy()}
              aria-label={copied() ? 'Copied' : 'Copy code'}
            >
              {copied() ? <Check aria-hidden /> : <Copy aria-hidden />}
            </button>
          ) : null}
        </figcaption>
      ) : null}
      <pre className={styles.pre}>
        <code className={styles.code}>{code}</code>
      </pre>
    </figure>
  )
}
