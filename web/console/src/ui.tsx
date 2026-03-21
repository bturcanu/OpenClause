import type { ReactNode } from 'react'

type PageHeaderProps = {
  title: string
  description: string
  actions?: ReactNode
}

type EmptyStateProps = {
  icon?: string
  title: string
  description: string
  action?: ReactNode
}

type InlineErrorProps = {
  message: string
  onRetry?: () => void
}

type StatCardProps = {
  label: string
  value: ReactNode
  tone?: 'default' | 'green' | 'red' | 'yellow' | 'blue'
  hint?: string
}

export function PageHeaderBlock({ title, description, actions }: PageHeaderProps) {
  return (
    <div className="page-hero">
      <div className="page-hero-copy">
        <p className="eyebrow">OpenClause Console</p>
        <h2>{title}</h2>
        <p>{description}</p>
      </div>
      {actions ? <div className="page-hero-actions">{actions}</div> : null}
    </div>
  )
}

export function EmptyState({ icon = '○', title, description, action }: EmptyStateProps) {
  return (
    <div className="empty-state">
      <div className="empty-icon">{icon}</div>
      <h3>{title}</h3>
      <p>{description}</p>
      {action ? <div className="empty-actions">{action}</div> : null}
    </div>
  )
}

export function InlineErrorState({ message, onRetry }: InlineErrorProps) {
  return (
    <div className="error-msg error-msg-rich">
      <div>
        <strong>Something went wrong.</strong>
        <div>{message}</div>
      </div>
      {onRetry ? (
        <button className="btn btn-outline btn-sm" type="button" onClick={onRetry}>
          Retry
        </button>
      ) : null}
    </div>
  )
}

export function StatCard({ label, value, tone = 'default', hint }: StatCardProps) {
  return (
    <div className="stat-card">
      <div className="stat-label">{label}</div>
      <div className={`stat-value ${tone !== 'default' ? `tone-${tone}` : ''}`}>{value}</div>
      {hint ? <div className="stat-hint">{hint}</div> : null}
    </div>
  )
}

export function TableSkeleton({ columns, rows = 5 }: { columns: number; rows?: number }) {
  return (
    <>
      {Array.from({ length: rows }).map((_, rowIdx) => (
        <tr key={rowIdx}>
          {Array.from({ length: columns }).map((__, colIdx) => (
            <td key={`${rowIdx}-${colIdx}`}>
              <div className="skeleton-line" />
            </td>
          ))}
        </tr>
      ))}
    </>
  )
}

export function shortID(value: string | null | undefined, head = 10): string {
  const text = (value || '').trim()
  if (!text) return '(none)'
  return text.length <= head ? text : `${text.slice(0, head)}…`
}

export function noneText(value: string | null | undefined, fallback = '(none)'): string {
  const text = (value || '').trim()
  return text || fallback
}

export function formatActor(userID?: string | null, userName?: string | null, userEmail?: string | null): string {
  const name = (userName || '').trim()
  const email = (userEmail || '').trim()
  const id = (userID || '').trim()
  if (name && email) return `${name} (${email})`
  if (name) return name
  if (email) return email
  if (id) return id
  return '(unknown user)'
}

export function formatRequester(userID?: string | null, userName?: string | null, userEmail?: string | null, agentID?: string | null): string {
  const actor = formatActor(userID, userName, userEmail)
  const agent = noneText(agentID)
  if (actor === '(unknown user)' && agent === '(none)') return 'Requested without user or agent attribution'
  if (actor === '(unknown user)') return `Requested via ${agent}`
  if (agent === '(none)') return `Requested by ${actor}`
  return `Requested by ${actor} via ${agent}`
}

export function buildQuery(params: Record<string, string | number | null | undefined>) {
  const query = new URLSearchParams()
  Object.entries(params).forEach(([key, value]) => {
    if (value === null || value === undefined) return
    const text = String(value).trim()
    if (!text) return
    query.set(key, text)
  })
  const built = query.toString()
  return built ? `?${built}` : ''
}

export async function copyText(text: string) {
  await navigator.clipboard.writeText(text)
}

export function downloadBlob(blob: Blob, filename: string) {
  const objectUrl = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = objectUrl
  link.download = filename
  link.click()
  URL.revokeObjectURL(objectUrl)
}

export function decisionTone(decision?: string | null) {
  switch ((decision || '').toLowerCase()) {
    case 'allow':
    case 'approved':
    case 'success':
    case 'active':
      return 'green'
    case 'deny':
    case 'denied':
    case 'error':
    case 'revoked':
      return 'red'
    case 'approve':
    case 'pending':
    case 'timeout':
      return 'yellow'
    default:
      return 'gray'
  }
}
