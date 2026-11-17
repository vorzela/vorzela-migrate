export function formatDate(value?: string): string {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return date.toLocaleString(undefined, {
    year: 'numeric',
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export function formatRelative(value?: string): string {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  const seconds = Math.round((date.getTime() - Date.now()) / 1000)
  const abs = Math.abs(seconds)
  const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })
  if (abs < 60) return rtf.format(seconds, 'second')
  if (abs < 3600) return rtf.format(Math.round(seconds / 60), 'minute')
  if (abs < 86400) return rtf.format(Math.round(seconds / 3600), 'hour')
  return rtf.format(Math.round(seconds / 86400), 'day')
}

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  const index = Math.min(
    units.length - 1,
    Math.floor(Math.log(bytes) / Math.log(1024)),
  )
  const value = bytes / Math.pow(1024, index)
  return `${value.toFixed(index === 0 ? 0 : 1)} ${units[index]}`
}

export function migrationName(filename: string): string {
  const index = filename.indexOf('_')
  return (index >= 0 ? filename.slice(index + 1) : filename).replace(
    /\.sql$/i,
    '',
  )
}

export function riskLabel(risk: string): string {
  switch (risk) {
    case 'critical':
      return 'Critical'
    case 'high':
      return 'High risk'
    case 'medium':
      return 'Needs review'
    default:
      return 'Low risk'
  }
}

export function verdictLabel(verdict?: string): string {
  switch (verdict) {
    case 'drift_safe':
      return 'Drift can apply'
    case 'needs_migration':
      return 'Follow-up migration needed'
    case 'destructive':
      return 'Destructive'
    case 'pending':
      return 'Not run yet'
    case 'unknown':
      return 'Status unknown'
    default:
      return 'No schema change'
  }
}

export function verdictTone(
  verdict?: string,
): 'success' | 'warning' | 'destructive' | 'secondary' {
  switch (verdict) {
    case 'drift_safe':
      return 'success'
    case 'needs_migration':
      return 'warning'
    case 'destructive':
      return 'destructive'
    default:
      return 'secondary'
  }
}

export function copyText(text: string): void {
  if (navigator.clipboard) {
    void navigator.clipboard.writeText(text)
  }
}
