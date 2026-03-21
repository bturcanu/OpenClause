import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, formatDate, toQueryTimestamp } from '../api'
import { EmptyState, InlineErrorState, PageHeaderBlock, TableSkeleton, buildQuery, decisionTone, downloadBlob, formatRequester, shortID } from '../ui'

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

export default function Events() {
  const [events, setEvents] = useState<Event[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [filters, setFilters] = useState<EventFilters>(defaultFilters)
  const fetchSeq = useRef(0)
  const [page, setPage] = useState(0)
  const limit = 25

  const selectedTenant = filters.tenant_id.trim()
  const hasActiveFilters = useMemo(
    () => Object.values(filters).some(value => value.trim() !== ''),
    [filters],
  )
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

  function updateFilter(key: keyof EventFilters, value: string) {
    setFilters(current => ({ ...current, [key]: value }))
    setPage(0)
  }

  function clearFilters() {
    setFilters(defaultFilters)
    setPage(0)
  }

  async function exportCSV() {
    try {
      if (isPlatformAdmin && !selectedTenant) {
        setError('Select a tenant before exporting CSV')
        return
      }
      const blob = await api.getBlob(selectedTenant ? `/admin/events/export/csv${buildQuery({ tenant_id: selectedTenant })}` : '/admin/events/export/csv')
      downloadBlob(blob, 'events.csv')
    } catch (err: any) {
      setError(err.message)
    }
  }

  async function exportBundle() {
    try {
      if (isPlatformAdmin && !selectedTenant) {
        setError('Select a tenant before exporting the evidence bundle')
        return
      }
      const blob = await api.getBlob(selectedTenant ? `/admin/reports/export/bundle${buildQuery({ tenant_id: selectedTenant })}` : '/admin/reports/export/bundle')
      downloadBlob(blob, 'audit-bundle.json')
    } catch (err: any) {
      setError(err.message)
    }
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
        <div className="filters-panel-note">Audit filters use your local browser time. Export actions use the selected tenant scope.</div>
        <div className="filters-bar filters-bar-dense">
          <div className="form-group">
            <label>Tenant</label>
            <input value={filters.tenant_id} onChange={e => updateFilter('tenant_id', e.target.value)} placeholder="tenant_id" />
          </div>
          <div className="form-group">
            <label>User ID</label>
            <input value={filters.user_id} onChange={e => updateFilter('user_id', e.target.value)} placeholder="user_id" />
          </div>
          <div className="form-group">
            <label>Agent ID</label>
            <input value={filters.agent_id} onChange={e => updateFilter('agent_id', e.target.value)} placeholder="agent_id" />
          </div>
          <div className="form-group">
            <label>Trace ID</label>
            <input value={filters.trace_id} onChange={e => updateFilter('trace_id', e.target.value)} placeholder="trace_id" />
          </div>
          <div className="form-group">
            <label>Tool</label>
            <input value={filters.tool} onChange={e => updateFilter('tool', e.target.value)} placeholder="slack" />
          </div>
          <div className="form-group">
            <label>Action</label>
            <input value={filters.action} onChange={e => updateFilter('action', e.target.value)} placeholder="msg.post" />
          </div>
          <div className="form-group">
            <label>Decision</label>
            <select value={filters.decision} onChange={e => updateFilter('decision', e.target.value)}>
              <option value="">Any</option>
              <option value="allow">Allow</option>
              <option value="deny">Deny</option>
              <option value="approve">Approve</option>
            </select>
          </div>
          <div className="form-group">
            <label>Session ID</label>
            <input value={filters.session_id} onChange={e => updateFilter('session_id', e.target.value)} placeholder="session_id" />
          </div>
          <div className="form-group form-group-small">
            <label>Risk min</label>
            <input type="number" min={0} max={10} inputMode="numeric" value={filters.risk_min} onChange={e => updateFilter('risk_min', e.target.value)} placeholder="0" />
          </div>
          <div className="form-group form-group-small">
            <label>Risk max</label>
            <input type="number" min={0} max={10} inputMode="numeric" value={filters.risk_max} onChange={e => updateFilter('risk_max', e.target.value)} placeholder="10" />
          </div>
          <div className="form-group">
            <label>Since (local time)</label>
            <input type="datetime-local" value={filters.since} onChange={e => updateFilter('since', e.target.value)} />
          </div>
          <div className="form-group">
            <label>Until (local time)</label>
            <input type="datetime-local" value={filters.until} onChange={e => updateFilter('until', e.target.value)} />
          </div>
          <div className="form-actions-row form-actions-row-end">
            <button className="btn btn-outline btn-sm" type="button" onClick={clearFilters} disabled={!hasActiveFilters || loading}>
              Clear filters
            </button>
          </div>
        </div>
      </div>

      <div className="table-container table-sticky">
        <table>
          <thead>
            <tr>
              <th>Event</th>
              <th>Requested by</th>
              <th>Tool</th>
              <th>Decision</th>
              <th>Tenant</th>
              <th>Session</th>
              <th>Time</th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <TableSkeleton columns={7} rows={6} />
            ) : events.length === 0 ? (
              <tr>
                <td colSpan={7}>
                  <EmptyState
                    icon="▤"
                    title={hasActiveFilters ? 'No audit events match these filters' : 'No audit events yet'}
                    description={hasActiveFilters ? 'Adjust the tenant, user, agent, session, or time filters to widen the search.' : 'Tool calls will appear here once agents start sending governed runs through the gateway.'}
                    action={hasActiveFilters ? <button className="btn btn-outline btn-sm" type="button" onClick={clearFilters}>Clear filters</button> : undefined}
                  />
                </td>
              </tr>
            ) : (
              events.map(event => (
                <tr key={event.event_id}>
                  <td>
                    <Link to={`/events/${event.event_id}`} className="table-primary">
                      {shortID(event.event_id)}
                    </Link>
                    <div className="table-subtext">Trace {event.trace_id ? shortID(event.trace_id) : '(none)'}</div>
                  </td>
                  <td>
                    <div className="table-primary">{formatRequester(event.user_id, event.user_name, event.user_email, event.agent_id)}</div>
                    <div className="table-subtext">Agent <span className="mono">{event.agent_id || '(none)'}</span></div>
                  </td>
                  <td>
                    <div className="table-primary">{event.tool}.{event.action}</div>
                    <div className="table-subtext">Risk {event.risk_score}</div>
                  </td>
                  <td>
                    <span className={`badge badge-${decisionTone(event.decision)}`}>{event.decision}</span>
                  </td>
                  <td>
                    <span className="mono">{shortID(event.tenant_id, 12)}</span>
                  </td>
                  <td>
                    {event.session_id ? (
                      <Link to={`/sessions/${encodeURIComponent(event.session_id)}${buildQuery({ tenant_id: event.tenant_id })}`} className="mono">{shortID(event.session_id)}</Link>
                    ) : (
                      '(none)'
                    )}
                  </td>
                  <td>{formatDate(event.received_at)}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

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
