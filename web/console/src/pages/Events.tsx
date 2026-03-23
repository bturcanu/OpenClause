import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { useSearchParams } from 'react-router-dom'
import { APIClientError, api, toLocalDateTimeInput, toQueryTimestamp } from '../api'
import {
  ActiveFiltersBar,
  CopyIconButton,
  InlineErrorState,
  PageHeaderBlock,
  SortHeader,
  TableEmptyStateRow,
  TableFrame,
  TableSkeleton,
  applySort,
  buildQuery,
  compareDate,
  compareNumber,
  compareText,
  decisionTone,
  downloadBlob,
  formatRequester,
  formatTimeWithTitle,
  shortID,
  type SortState,
} from '../ui'

type Event = {
  event_id: string
  tool: string
  action: string
  decision: string
  risk_score: number
  tenant_id: string
  session_id: string
  agent_id: string
  user_id?: string
  user_name?: string
  user_email?: string
  trace_id?: string
  received_at: string
}

type EventFilters = {
  tenant_id: string
  user_id: string
  agent_id: string
  trace_id: string
  tool: string
  action: string
  decision: string
  session_id: string
  risk_min: string
  risk_max: string
  since: string
  until: string
}

const defaultFilters: EventFilters = {
  tenant_id: '',
  user_id: '',
  agent_id: '',
  trace_id: '',
  tool: '',
  action: '',
  decision: '',
  session_id: '',
  risk_min: '',
  risk_max: '',
  since: '',
  until: '',
}

function filtersFromSearchParams(searchParams: URLSearchParams): EventFilters {
  return {
    ...defaultFilters,
    tenant_id: searchParams.get('tenant_id') || '',
    user_id: searchParams.get('user_id') || '',
    agent_id: searchParams.get('agent_id') || '',
    trace_id: searchParams.get('trace_id') || '',
    tool: searchParams.get('tool') || '',
    action: searchParams.get('action') || '',
    decision: searchParams.get('decision') || '',
    session_id: searchParams.get('session_id') || '',
    risk_min: searchParams.get('risk_min') || '',
    risk_max: searchParams.get('risk_max') || '',
    since: toLocalDateTimeInput(searchParams.get('since')),
    until: toLocalDateTimeInput(searchParams.get('until')),
  }
}

export default function Events() {
  const [searchParams, setSearchParams] = useSearchParams()
  const [events, setEvents] = useState<Event[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [filters, setFilters] = useState<EventFilters>(() => filtersFromSearchParams(searchParams))
  const fetchSeq = useRef(0)
  const [page, setPage] = useState(0)
  const [sortState, setSortState] = useState<SortState<'received_at' | 'risk_score' | 'decision'>>({ key: null, dir: 'desc' })
  const limit = 25

  const selectedTenant = filters.tenant_id.trim()
  const hasActiveFilters = useMemo(
    () => Object.values(filters).some(value => value.trim() !== ''),
    [filters],
  )
  const resolvedSince = useMemo(() => toQueryTimestamp(filters.since), [filters.since])
  const resolvedUntil = useMemo(() => toQueryTimestamp(filters.until), [filters.until])
  const isPlatformAdmin = useMemo(() => {
    const token = localStorage.getItem('oc_token')
    if (!token) return false
    try {
      const payload = token.split('.')[1]
      const base64 = payload.replace(/-/g, '+').replace(/_/g, '/')
      const padded = base64.padEnd(base64.length + (4 - (base64.length % 4)) % 4, '=')
      const decoded = JSON.parse(atob(padded))
      const roles: string[] = Array.isArray(decoded?.roles) ? decoded.roles : []
      return roles.includes('platform_admin')
    } catch {
      return false
    }
  }, [])

  const fetchEvents = useCallback(async () => {
    const seq = ++fetchSeq.current
    setLoading(true)
    setError('')
    try {
      const query = buildQuery({
        ...filters,
        since: toQueryTimestamp(filters.since),
        until: toQueryTimestamp(filters.until),
        limit,
        offset: page * limit,
      })
      const data = await api.get(`/admin/events${query}`)
      if (seq !== fetchSeq.current) return
      setEvents(Array.isArray(data) ? data : data?.events || [])
    } catch (err: any) {
      if (seq === fetchSeq.current) setError(err.message)
    } finally {
      if (seq === fetchSeq.current) setLoading(false)
    }
  }, [filters, page])

  useEffect(() => {
    void fetchEvents()
  }, [fetchEvents])

  useEffect(() => {
    const nextFilters = filtersFromSearchParams(searchParams)
    setFilters(current => {
      const matches = (Object.keys(nextFilters) as Array<keyof EventFilters>).every(key => current[key] === nextFilters[key])
      return matches ? current : nextFilters
    })
    setPage(0)
  }, [searchParams])

  function syncFiltersToURL(nextFilters: EventFilters) {
    const next = new URLSearchParams()
    ;(Object.entries(nextFilters) as Array<[keyof EventFilters, string]>).forEach(([key, value]) => {
      const trimmed = value.trim()
      if (!trimmed) return
      if (key === 'since' || key === 'until') {
        const resolved = toQueryTimestamp(trimmed)
        if (resolved) next.set(key, resolved)
        return
      }
      next.set(key, trimmed)
    })
    setSearchParams(next)
  }

  function updateFilter(key: keyof EventFilters, value: string) {
    const nextFilters = { ...filters, [key]: value }
    setFilters(nextFilters)
    syncFiltersToURL(nextFilters)
    setPage(0)
  }

  function clearFilters() {
    setFilters(defaultFilters)
    setSearchParams(new URLSearchParams())
    setPage(0)
  }

  const visibleEvents = useMemo(() => {
    if (!sortState.key) return events
    return [...events].sort((left, right) => {
      switch (sortState.key) {
        case 'received_at':
          return applySort(compareDate(left.received_at, right.received_at), sortState.dir)
        case 'risk_score':
          return applySort(compareNumber(left.risk_score, right.risk_score), sortState.dir)
        case 'decision':
          return applySort(compareText(left.decision, right.decision), sortState.dir)
        default:
          return 0
      }
    })
  }, [events, sortState])

  const activeFilterChips = useMemo(() => (
    (Object.entries(filters) as Array<[keyof EventFilters, string]>)
      .filter(([, value]) => value.trim() !== '')
      .map(([key, value]) => ({
        key,
        label: `${key.replace(/_/g, ' ')}: ${value.trim()}`,
        onRemove: () => updateFilter(key, ''),
      }))
  ), [filters])

  function exportQuery() {
    return buildQuery({
      ...(selectedTenant ? { tenant_id: selectedTenant } : {}),
      since: toQueryTimestamp(filters.since),
      until: toQueryTimestamp(filters.until),
    })
  }

  function exportErrorMessage(err: unknown, kind: 'csv' | 'bundle') {
    if (err instanceof APIClientError) {
      const reason = typeof err.details === 'object' && err.details !== null
        ? String((err.details as Record<string, unknown>).reason || '')
        : ''
      if (kind === 'bundle' && err.status === 400 && reason === 'range_too_large') {
        return 'Evidence bundle exports are limited to 10,000 events. Narrow the time window and try again.'
      }
      return err.message
    }
    return err instanceof Error ? err.message : `Failed to export ${kind === 'csv' ? 'CSV' : 'evidence bundle'}`
  }

  async function exportCSV() {
    try {
      if (isPlatformAdmin && !selectedTenant) {
        setError('Select a tenant before exporting CSV')
        return
      }
      const blob = await api.getBlob(`/admin/events/export/csv${exportQuery()}`)
      downloadBlob(blob, 'events.csv')
    } catch (err) {
      setError(exportErrorMessage(err, 'csv'))
    }
  }

  async function exportBundle() {
    try {
      if (isPlatformAdmin && !selectedTenant) {
        setError('Select a tenant before exporting the evidence bundle')
        return
      }
      const blob = await api.getBlob(`/admin/reports/export/bundle${exportQuery()}`)
      downloadBlob(blob, 'audit-bundle.json')
    } catch (err) {
      setError(exportErrorMessage(err, 'bundle'))
    }
  }

  function updateSort(key: 'received_at' | 'risk_score' | 'decision', dir: 'asc' | 'desc') {
    setSortState({ key, dir })
  }

  return (
    <div>
      <PageHeaderBlock
        title="Audit Trail"
        description="Inspect every governed tool event with clear user, agent, session, and trace attribution."
        actions={
          <details className="action-menu">
            <summary className="btn btn-outline" aria-disabled={loading}>
              Export ▾
            </summary>
            <div className="action-menu-list">
              <button className="action-menu-item" type="button" onClick={exportCSV} disabled={loading}>
                Export CSV
              </button>
              <button className="action-menu-item" type="button" onClick={exportBundle} disabled={loading}>
                Export evidence bundle
              </button>
            </div>
          </details>
        }
      />

      {error ? <InlineErrorState message={error} onRetry={() => void fetchEvents()} /> : null}

      <div className="filters-panel">
        <div className="filters-panel-note">Audit filters use your local browser time. Export actions follow the selected tenant and current time window; blank time fields fall back to the server default window.</div>
        <div className="table-subtext">
          Export window (UTC):
          {' '}since <code className="mono">{resolvedSince || 'server default: last 7 days'}</code>
          {' '}· until <code className="mono">{resolvedUntil || 'server default: request time'}</code>
        </div>
        <div className="filters-bar filters-bar-dense">
          <div className="form-group">
            <label htmlFor="events-filter-tenant">Tenant</label>
            <input id="events-filter-tenant" value={filters.tenant_id} onChange={e => updateFilter('tenant_id', e.target.value)} placeholder="tenant_id" />
          </div>
          <div className="form-group">
            <label htmlFor="events-filter-user-id">User ID</label>
            <input id="events-filter-user-id" value={filters.user_id} onChange={e => updateFilter('user_id', e.target.value)} placeholder="user_id" />
          </div>
          <div className="form-group">
            <label htmlFor="events-filter-agent-id">Agent ID</label>
            <input id="events-filter-agent-id" value={filters.agent_id} onChange={e => updateFilter('agent_id', e.target.value)} placeholder="agent_id" />
          </div>
          <div className="form-group">
            <label htmlFor="events-filter-trace-id">Trace ID</label>
            <input id="events-filter-trace-id" value={filters.trace_id} onChange={e => updateFilter('trace_id', e.target.value)} placeholder="trace_id" />
          </div>
          <div className="form-group">
            <label htmlFor="events-filter-tool">Tool</label>
            <input id="events-filter-tool" value={filters.tool} onChange={e => updateFilter('tool', e.target.value)} placeholder="slack" />
          </div>
          <div className="form-group">
            <label htmlFor="events-filter-action">Action</label>
            <input id="events-filter-action" value={filters.action} onChange={e => updateFilter('action', e.target.value)} placeholder="msg.post" />
          </div>
          <div className="form-group">
            <label htmlFor="events-filter-decision">Decision</label>
            <select id="events-filter-decision" value={filters.decision} onChange={e => updateFilter('decision', e.target.value)}>
              <option value="">Any</option>
              <option value="allow">Allow</option>
              <option value="deny">Deny</option>
              <option value="approve">Approve</option>
            </select>
          </div>
          <div className="form-group">
            <label htmlFor="events-filter-session-id">Session ID</label>
            <input id="events-filter-session-id" value={filters.session_id} onChange={e => updateFilter('session_id', e.target.value)} placeholder="session_id" />
          </div>
          <div className="form-group form-group-small">
            <label htmlFor="events-filter-risk-min">Risk min</label>
            <input id="events-filter-risk-min" type="number" min={0} max={10} inputMode="numeric" value={filters.risk_min} onChange={e => updateFilter('risk_min', e.target.value)} placeholder="0" />
          </div>
          <div className="form-group form-group-small">
            <label htmlFor="events-filter-risk-max">Risk max</label>
            <input id="events-filter-risk-max" type="number" min={0} max={10} inputMode="numeric" value={filters.risk_max} onChange={e => updateFilter('risk_max', e.target.value)} placeholder="10" />
          </div>
          <div className="form-group">
            <label htmlFor="events-filter-since">Since (local time)</label>
            <input id="events-filter-since" type="datetime-local" value={filters.since} onChange={e => updateFilter('since', e.target.value)} />
          </div>
          <div className="form-group">
            <label htmlFor="events-filter-until">Until (local time)</label>
            <input id="events-filter-until" type="datetime-local" value={filters.until} onChange={e => updateFilter('until', e.target.value)} />
          </div>
          <div className="form-actions-row form-actions-row-end">
            <button className="btn btn-outline btn-sm" type="button" onClick={clearFilters} disabled={!hasActiveFilters || loading}>
              Clear filters
            </button>
          </div>
        </div>
      </div>

      <ActiveFiltersBar
        resultCount={visibleEvents.length}
        resultLabel={visibleEvents.length === 1 ? 'event' : 'events'}
        chips={activeFilterChips}
        onClearAll={hasActiveFilters ? clearFilters : undefined}
        note={sortState.key ? 'Sorted within the current page.' : 'Using backend order until you sort this page.'}
      />

      <TableFrame stickyHeader>
        <table>
          <thead>
            <tr>
              <th>Tool call</th>
              <th>Requested by</th>
              <th>Tenant</th>
              <th>
                <SortHeader label="Decision" sortKey="decision" sortState={sortState} onSortChange={updateSort} />
              </th>
              <th className="col-num">
                <SortHeader label="Risk" sortKey="risk_score" sortState={sortState} onSortChange={updateSort} className="col-num" />
              </th>
              <th className="col-time">
                <SortHeader label="Time" sortKey="received_at" sortState={sortState} onSortChange={updateSort} defaultDir="desc" className="col-time" />
              </th>
              <th className="table-action-col"></th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <TableSkeleton columns={7} rows={6} />
            ) : visibleEvents.length === 0 ? (
              <TableEmptyStateRow
                colSpan={7}
                icon="▤"
                title={hasActiveFilters ? 'No audit events match these filters' : 'No audit events yet'}
                description={hasActiveFilters ? 'Adjust the tenant, user, agent, session, or time filters to widen the search.' : 'Tool calls will appear here once agents start sending governed runs through the gateway.'}
                action={hasActiveFilters ? <button className="btn btn-outline btn-sm" type="button" onClick={clearFilters}>Clear filters</button> : undefined}
              />
            ) : (
              visibleEvents.map(event => {
                const received = formatTimeWithTitle(event.received_at)
                return (
                <tr key={event.event_id}>
                  <td>
                    <div className="table-primary-cell">
                      <Link to={`/events/${event.event_id}`} className="table-primary table-primary-link" title={`${event.tool}.${event.action}`}>
                        {event.tool}.{event.action}
                      </Link>
                      <div className="inline-value-copy">
                        <code className="mono" title={event.event_id}>{shortID(event.event_id)}</code>
                        <CopyIconButton text={event.event_id} label="Event ID" />
                      </div>
                      <div className="table-tertiary">
                        <button className="filter-chip filter-chip-inline" type="button" onClick={() => updateFilter('tool', event.tool)}>
                          {event.tool}
                        </button>
                        <button className="filter-chip filter-chip-inline" type="button" onClick={() => updateFilter('action', event.action)}>
                          {event.action}
                        </button>
                        {event.trace_id ? (
                          <button className="filter-chip filter-chip-inline" type="button" onClick={() => updateFilter('trace_id', event.trace_id || '')} title={event.trace_id}>
                            Trace {shortID(event.trace_id, 10)}
                          </button>
                        ) : null}
                      </div>
                    </div>
                  </td>
                  <td>
                    <div className="table-primary-cell">
                      <div className="table-primary">{formatRequester(event.user_id, event.user_name, event.user_email, event.agent_id)}</div>
                      <div className="table-subtext">Agent <span className="mono" title={event.agent_id || '(none)'}>{event.agent_id || '(none)'}</span></div>
                      <div className="table-tertiary">
                        {event.user_id ? (
                          <button className="filter-chip filter-chip-inline" type="button" onClick={() => updateFilter('user_id', event.user_id || '')} title={event.user_id}>
                            User {shortID(event.user_id, 10)}
                          </button>
                        ) : null}
                        <button className="filter-chip filter-chip-inline" type="button" onClick={() => updateFilter('agent_id', event.agent_id || '')} title={event.agent_id || '(none)'}>
                          Agent {shortID(event.agent_id, 10)}
                        </button>
                      </div>
                    </div>
                  </td>
                  <td>
                    <div className="table-primary-cell">
                      <div className="inline-value-copy">
                        <code className="mono" title={event.tenant_id}>{shortID(event.tenant_id, 12)}</code>
                        <CopyIconButton text={event.tenant_id} label="Tenant ID" />
                      </div>
                      <div className="table-tertiary">
                        <button className="filter-chip filter-chip-inline" type="button" onClick={() => updateFilter('tenant_id', event.tenant_id)} title={event.tenant_id}>
                          Tenant {shortID(event.tenant_id, 10)}
                        </button>
                        {event.session_id ? (
                          <div className="inline-value-copy">
                            <button className="filter-chip filter-chip-inline" type="button" onClick={() => updateFilter('session_id', event.session_id || '')} title={event.session_id}>
                              Session {shortID(event.session_id, 10)}
                            </button>
                            <CopyIconButton text={event.session_id} label="Session ID" />
                          </div>
                        ) : null}
                      </div>
                    </div>
                  </td>
                  <td>
                    <button className={`badge badge-${decisionTone(event.decision)} badge-button`} type="button" onClick={() => updateFilter('decision', event.decision)}>
                      {event.decision}
                    </button>
                  </td>
                  <td className="col-num">{event.risk_score}</td>
                  <td className="col-time" title={received.title}>{received.label}</td>
                  <td className="table-action-cell">
                    <div className="table-primary-cell table-action-stack">
                      <Link to={`/events/${event.event_id}`} className="btn btn-outline btn-sm">
                        Open event
                      </Link>
                      {event.session_id ? (
                        <Link
                          to={`/sessions/${encodeURIComponent(event.session_id)}${buildQuery({ tenant_id: event.tenant_id })}`}
                          className="link-button"
                          title={event.session_id}
                        >
                          Open session
                        </Link>
                      ) : null}
                    </div>
                  </td>
                </tr>
              )})
            )}
          </tbody>
        </table>
      </TableFrame>

      <div className="pagination">
        <button className="btn btn-outline btn-sm" disabled={page === 0 || loading} onClick={() => setPage(current => current - 1)}>
          Previous
        </button>
        <span>Page {page + 1}</span>
        <button className="btn btn-outline btn-sm" disabled={loading || events.length < limit} onClick={() => setPage(current => current + 1)}>
          Next
        </button>
      </div>
    </div>
  )
}
