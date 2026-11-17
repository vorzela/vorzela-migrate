import { computed } from '@vorzelajs/core'
import { For } from '@vorzelajs/dom'
import { accessorOf } from '~/hooks/index.ts'
import { cx } from '~/lib/cx.ts'
import styles from '~/styles/components/ui/diff-viewer.module.scss'

type Op = {
  type: 'same' | 'add' | 'remove'
  text: string
  oldLine?: number
  newLine?: number
}

function diffLines(oldText: string, newText: string): Op[] {
  const oldLines = oldText.split('\n')
  const newLines = newText.split('\n')
  const n = oldLines.length
  const m = newLines.length

  // Bound the DP to avoid pathological memory/time on large files.
  if (n * m > 250_000) {
    return [
      ...oldLines.map((text, index) => ({
        type: 'remove' as const,
        text,
        oldLine: index + 1,
      })),
      ...newLines.map((text, index) => ({
        type: 'add' as const,
        text,
        newLine: index + 1,
      })),
    ]
  }

  const dp = Array.from({ length: n + 1 }, () => new Uint16Array(m + 1))
  for (let i = n - 1; i >= 0; i -= 1) {
    for (let j = m - 1; j >= 0; j -= 1) {
      dp[i][j] =
        oldLines[i] === newLines[j]
          ? dp[i + 1][j + 1] + 1
          : Math.max(dp[i + 1][j], dp[i][j + 1])
    }
  }

  const ops: Op[] = []
  let i = 0
  let j = 0
  while (i < n && j < m) {
    if (oldLines[i] === newLines[j]) {
      ops.push({
        type: 'same',
        text: oldLines[i] as string,
        oldLine: i + 1,
        newLine: j + 1,
      })
      i += 1
      j += 1
    } else if (dp[i + 1][j] >= dp[i][j + 1]) {
      ops.push({ type: 'remove', text: oldLines[i] as string, oldLine: i + 1 })
      i += 1
    } else {
      ops.push({ type: 'add', text: newLines[j] as string, newLine: j + 1 })
      j += 1
    }
  }
  while (i < n) {
    ops.push({ type: 'remove', text: oldLines[i] as string, oldLine: i + 1 })
    i += 1
  }
  while (j < m) {
    ops.push({ type: 'add', text: newLines[j] as string, newLine: j + 1 })
    j += 1
  }
  return ops
}

export interface DiffViewerProps {
  oldValue: string | (() => string)
  newValue: string | (() => string)
  label?: string
  className?: string
}

export function DiffViewer({
  oldValue,
  newValue,
  label = 'Diff',
  className,
}: DiffViewerProps) {
  const oldValueAccessor = accessorOf(oldValue)
  const newValueAccessor = accessorOf(newValue)
  const ops = computed(() =>
    diffLines(oldValueAccessor(), newValueAccessor()),
  )
  const added = (): number =>
    ops().filter((op) => op.type === 'add').length
  const removed = (): number =>
    ops().filter((op) => op.type === 'remove').length

  return (
    <div
      role="group"
      aria-label={() => `${label}: ${added()} added, ${removed()} removed`}
      className={cx(styles.root, className)}
    >
      <div className={styles.header}>
        <span>{label}</span>
        <span className={styles.counts}>
          <span className={styles.added}>+{added}</span>{' '}
          <span className={styles.removed}>−{removed}</span>
        </span>
      </div>
      <div className={styles.body}>
        <For each={ops}>
          {(op) => (
            <div
              className={() =>
                cx(
                  styles.row,
                  op.type === 'add' && styles.rowAdd,
                  op.type === 'remove' && styles.rowRemove,
                )
              }
            >
              <span className={styles.lineNo}>{op.oldLine ?? ''}</span>
              <span className={styles.lineNo}>{op.newLine ?? ''}</span>
              <span className={styles.text}>
                <span className={styles.marker}>
                  {op.type === 'add' ? '+' : op.type === 'remove' ? '−' : ' '}
                </span>{' '}
                {op.text || ' '}
              </span>
            </div>
          )}
        </For>
      </div>
    </div>
  )
}
