import { useEffect, useRef, useState, type CSSProperties, type KeyboardEvent, type MouseEvent, type ReactNode } from 'react'
import { formatDate } from './api'

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

type CopyIconButtonProps = {
  text?: string | null
  label: string
  className?: string
  disabled?: boolean
}

export type SortDir = 'asc' | 'desc'

export type SortState<K extends string = string> = {
  key: K | null
  dir: SortDir
}

type SortHeaderProps<K extends string> = {
  label: string
  sortKey: K
  sortState: SortState<K>
  onSortChange: (key: K, dir: SortDir) => void
  defaultDir?: SortDir
  className?: string
}

type ActiveFilterChip = {
  key: string
  label: string
  onRemove: () => void
}

type ActiveFiltersBarProps = {
  resultCount: number
  resultLabel?: string
  chips: ActiveFilterChip[]
  onClearAll?: () => void
  note?: string
}

type TableFrameProps = {
  children: ReactNode
  className?: string
  stickyHeader?: boolean
  style?: CSSProperties
}

type TableEmptyStateRowProps = {
  colSpan: number
  icon?: string
  title: string
  description: string
  action?: ReactNode
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
      <div className="empty-icon" aria-hidden="true">{icon}</div>
      <div className="empty-state-copy">
        <h3>{title}</h3>
        <p>{description}</p>
      </div>
      {action ? <div className="empty-actions">{action}</div> : null}
    </div>
  )
}

export function InlineErrorState({ message, onRetry }: InlineErrorProps) {
  return (
    <div className="error-msg error-msg-rich">
      <div className="error-msg-body">
        <div className="error-msg-icon" aria-hidden="true">!</div>
        <div>
        <strong>Something went wrong.</strong>
        <div>{message}</div>
        </div>
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

export function CopyIconButton({ text, label, className = '', disabled = false }: CopyIconButtonProps) {
  const [copied, setCopied] = useState(false)
  const canCopy = !!(text || '').trim() && !disabled

  async function handleClick(event: MouseEvent<HTMLButtonElement>) {
    event.preventDefault()
    event.stopPropagation()
    if (!canCopy) return
    try {
      await copyText(text || '')
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    } catch {
      setCopied(false)
    }
  }

  return (
    <button
      className={`btn btn-outline btn-sm copy-icon-button ${className}`.trim()}
      type="button"
      onClick={(event) => void handleClick(event)}
      disabled={!canCopy}
      title={copied ? `${label} copied` : `Copy ${label}`}
      aria-label={copied ? `${label} copied` : `Copy ${label}`}
    >
      {copied ? '✓' : '⧉'}
    </button>
  )
}

export function SortHeader<K extends string>({
  label,
  sortKey,
  sortState,
  onSortChange,
  defaultDir = 'asc',
  className = '',
}: SortHeaderProps<K>) {
  const isActive = sortState.key === sortKey
  const nextDir: SortDir = !isActive ? defaultDir : sortState.dir === 'asc' ? 'desc' : 'asc'
  const ariaSort = !isActive ? 'none' : sortState.dir === 'asc' ? 'ascending' : 'descending'

  function triggerSort() {
    onSortChange(sortKey, nextDir)
  }

  function onKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
    if (event.key !== 'Enter' && event.key !== ' ') return
    event.preventDefault()
    triggerSort()
  }

  return (
    <button
      type="button"
      className={`sort-header ${isActive ? 'is-active' : ''} ${className}`.trim()}
      aria-sort={ariaSort}
      onClick={triggerSort}
      onKeyDown={onKeyDown}
      title={isActive ? `Sorted ${sortState.dir}` : 'Use backend order until you sort this page'}
    >
      <span>{label}</span>
      <span className="sort-header-carets" aria-hidden="true">
        <span className={isActive && sortState.dir === 'asc' ? 'is-active' : ''}>▲</span>
        <span className={isActive && sortState.dir === 'desc' ? 'is-active' : ''}>▼</span>
      </span>
    </button>
  )
}

export function ActiveFiltersBar({
  resultCount,
  resultLabel = 'results',
  chips,
  onClearAll,
  note,
}: ActiveFiltersBarProps) {
  return (
    <div className="active-filters-bar">
      <div className="active-filters-summary">
        <strong>
          Showing {resultCount.toLocaleString()} {resultLabel}
        </strong>
        {note ? <span className="active-filters-note">{note}</span> : null}
      </div>
      <div className="active-filters-chips">
        {chips.map(chip => (
          <button key={chip.key} className="filter-chip" type="button" onClick={chip.onRemove} title={`Remove ${chip.label}`}>
            <span>{chip.label}</span>
            <span aria-hidden="true">×</span>
          </button>
        ))}
        {chips.length > 1 && onClearAll ? (
          <button className="link-button filter-chip-clear" type="button" onClick={onClearAll}>
            Clear all
          </button>
        ) : null}
      </div>
    </div>
  )
}

export function TableFrame({ children, className = '', stickyHeader = false, style }: TableFrameProps) {
  const ref = useRef<HTMLDivElement | null>(null)
  const [hasOverflowLeft, setHasOverflowLeft] = useState(false)
  const [hasOverflowRight, setHasOverflowRight] = useState(false)

  useEffect(() => {
    const node = ref.current
    if (!node) return
    const element = node

    function updateOverflow() {
      const maxScrollLeft = Math.max(element.scrollWidth - element.clientWidth, 0)
      setHasOverflowLeft(element.scrollLeft > 1)
      setHasOverflowRight(maxScrollLeft - element.scrollLeft > 1)
    }

    updateOverflow()
    element.addEventListener('scroll', updateOverflow, { passive: true })
    window.addEventListener('resize', updateOverflow)
    const observer = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(updateOverflow) : null
    observer?.observe(element)

    return () => {
      element.removeEventListener('scroll', updateOverflow)
      window.removeEventListener('resize', updateOverflow)
      observer?.disconnect()
    }
  }, [])

  return (
    <div
      ref={ref}
      style={style}
      className={[
        'table-container',
        stickyHeader ? 'table-sticky' : '',
        hasOverflowLeft ? 'has-overflow-left' : '',
        hasOverflowRight ? 'has-overflow-right' : '',
        className,
      ].filter(Boolean).join(' ')}
    >
      {hasOverflowRight ? <div className="table-scroll-hint" aria-hidden="true">⇆ Scroll</div> : null}
      {children}
    </div>
  )
}

export function TableEmptyStateRow({ colSpan, icon, title, description, action }: TableEmptyStateRowProps) {
  return (
    <tr>
      <td colSpan={colSpan} className="table-empty-state-cell">
        <EmptyState icon={icon} title={title} description={description} action={action} />
      </td>
    </tr>
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
    if (typeof value === 'number' && !Number.isFinite(value)) return
    const text = String(value).trim()
    if (!text) return
    query.set(key, text)
  })
  const built = query.toString()
  return built ? `?${built}` : ''
}

export function compareText(a: string | null | undefined, b: string | null | undefined) {
  return (a || '').localeCompare(b || '', undefined, { sensitivity: 'base', numeric: true })
}

export function compareNumber(a: number | null | undefined, b: number | null | undefined) {
  return (a || 0) - (b || 0)
}

export function compareDate(a: string | null | undefined, b: string | null | undefined) {
  const left = a ? new Date(a).getTime() : 0
  const right = b ? new Date(b).getTime() : 0
  return left - right
}

export function applySort(result: number, dir: SortDir) {
  return dir === 'asc' ? result : -result
}

export function formatRelativeTime(value: string | null | undefined) {
  if (!value) return '—'
  const timestamp = new Date(value).getTime()
  if (Number.isNaN(timestamp)) return value

  const deltaMs = timestamp - Date.now()
  const absSeconds = Math.abs(deltaMs) / 1000
  const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })

  if (absSeconds < 60) return rtf.format(Math.round(deltaMs / 1000), 'second')
  if (absSeconds < 3600) return rtf.format(Math.round(deltaMs / 60000), 'minute')
  if (absSeconds < 86400) return rtf.format(Math.round(deltaMs / 3600000), 'hour')
  if (absSeconds < 604800) return rtf.format(Math.round(deltaMs / 86400000), 'day')
  if (absSeconds < 2629800) return rtf.format(Math.round(deltaMs / 604800000), 'week')
  if (absSeconds < 31557600) return rtf.format(Math.round(deltaMs / 2629800000), 'month')
  return rtf.format(Math.round(deltaMs / 31557600000), 'year')
}

export function formatTimeWithTitle(value: string | null | undefined) {
  return {
    label: formatRelativeTime(value),
    title: formatDate(value),
  }
}

export async function copyText(text: string) {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return
    } catch {
      // Fall back below for browsers or localhost contexts that block the async clipboard API.
    }
  }

  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.setAttribute('readonly', 'true')
  textarea.style.position = 'fixed'
  textarea.style.top = '-1000px'
  textarea.style.opacity = '0'
  document.body.appendChild(textarea)
  textarea.select()
  textarea.setSelectionRange(0, textarea.value.length)

  const succeeded = typeof document.execCommand === 'function' && document.execCommand('copy')
  document.body.removeChild(textarea)

  if (!succeeded) {
    throw new Error('Copy is not available in this browser context')
  }
}

let downloadTriggerForTests: ((link: HTMLAnchorElement) => void) | null = null

export function setDownloadTriggerForTests(handler: ((link: HTMLAnchorElement) => void) | null) {
  downloadTriggerForTests = handler
}

export function downloadBlob(blob: Blob, filename: string) {
  const objectUrl = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = objectUrl
  link.download = filename
  document.body.appendChild(link)
  if (downloadTriggerForTests) downloadTriggerForTests(link)
  else link.click()
  document.body.removeChild(link)
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
