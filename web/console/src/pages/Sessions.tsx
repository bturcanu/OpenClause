import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, formatDate } from '../api'
import { TableSkeleton, EmptyState, InlineErrorState, PageHeaderBlock, StatCard, buildQuery, decisionTone, formatRequester, noneText, shortID } from '../ui'

type Session = {
  id: string
  tenant_id: string
  agent_id: string
  user_id?: string
  user_name?: string
  user_email?: string
  trace_id?: string
  started_at: string
  last_event_at: string
  event_count: number
  allow_count: number
  deny_count: number
  approve_count: number
  last_event_id?: string
  last_tool?: string
  last_action?: string
  last_decision?: string
  last_resource?: string
  last_risk_score?: number
}

type SessionFilters = {
  tenant_id: string
  session_id: string
  user_id: string
  agent_id: string
  trace_id: string
  tool: string
  action: string
  decision: string
  risk_min: string
  risk_max: string
  since: string
  until: string
}

const defaultFilters: SessionFilters = {
  tenant_id: '',
  session_id: '',
  user_id: '',
  agent_id: '',
  trace_id: '',
  tool: '',
  action: '',
  decision: '',
  risk_min: '',
  risk_max: '',
  since: '',
  until: '',
}

export default function Sessions() {
  const [sessions, setSessions] = useState<Session[]>([])
  const [filters, setFilters] = useState<SessionFilters>(defaultFilters)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [page, setPage] = useState(0)
  const fetchSeq = useRef(0)
  const limit = 25

  const fetchSessions = useCallback(async () => {
    const seq = ++fetchSeq.current
    setLoading(true)
    setError('')
    try {
      const query = buildQuery({
        ...filters,
        limit,
        offset: page * limit,
      })
      const data = await api.get(`/admin/sessions${query}`)
      if (seq !== fetchSeq.current) return
      setSessions(Array.isArray(data) ? data : data?.sessions || [])
    } catch (err: any) {
      if (seq === fetchSeq.current) setError(err.message)
    } finally {
      if (seq === fetchSeq.current) setLoading(false)
    }
  }, [filters, page])

  useEffect(() => {
    void fetchSessions()
  }, [fetchSessions])

  function updateFilter(key: keyof SessionFilters, value: string) {
    setFilters(current => ({ ...current, [key]: value }))
    setPage(0)
  }

  const totals = useMemo(() => {
    return sessions.reduce(
      (acc, session) => {
        acc.eventCount += session.event_count || 0
        acc.allowCount += session.allow_count || 0
        acc.denyCount += session.deny_count || 0
        acc.approveCount += session.approve_count || 0
        return acc
      },
      { eventCount: 0, allowCount: 0, denyCount: 0, approveCount: 0 }
    )
  }, [sessions])

  return (
    <div>
      <PageHeaderBlock
        title="Sessions"
        description="Trace a single agent run from request to decision, approval, and execution without leaving the console."
        actions={
          <div className="btn-group">
            <button className="btn btn-outline" type="button" onClick={() => setFilters(defaultFilters)}>
              Clear filters
            </button>
            <button className="btn btn-primary" type="button" onClick={() => void fetchSessions()}>
              Refresh
            </button>
          </div>
        }
      />

      <div className="banner-note mb-16">
        Sessions are derived from observed `session_id` values on tool calls. Events without a session id still appear in Audit Trail as <strong>(none)</strong>.
      </div>

      <div className="stats-grid">
        <StatCard label="Tracked sessions" value={sessions.length} hint="Current page after filters" />
        <StatCard label="Events in view" value={totals.eventCount} tone="blue" />
        <StatCard label="Denied in view" value={totals.denyCount} tone="red" />
        <StatCard label="Awaiting review in view" value={totals.approveCount} tone="yellow" />
      </div>

      {error ? <InlineErrorState message={error} onRetry={() => void fetchSessions()} /> : null}

      <div className="filters-panel">
        <div className="filters-bar filters-bar-dense">
          <div className="form-group">
            <label>Tenant</label>
            <input value={filters.tenant_id} onChange={e => updateFilter('tenant_id', e.target.value)} placeholder="tenant_id" />
          </div>
          <div className="form-group">
            <label>Session ID</label>
            <input value={filters.session_id} onChange={e => updateFilter('session_id', e.target.value)} placeholder="session_id" />
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
          <div className="form-group form-group-small">
            <label>Risk min</label>
            <input value={filters.risk_min} onChange={e => updateFilter('risk_min', e.target.value)} placeholder="0" />
          </div>
          <div className="form-group form-group-small">
            <label>Risk max</label>
            <input value={filters.risk_max} onChange={e => updateFilter('risk_max', e.target.value)} placeholder="10" />
          </div>
          <div className="form-group">
            <label>Since</label>
            <input value={filters.since} onChange={e => updateFilter('since', e.target.value)} type="datetime-local" />
          </div>
          <div className="form-group">
            <label>Until</label>
            <input value={filters.until} onChange={e => updateFilter('until', e.target.value)} type="datetime-local" />
          </div>
        </div>
      </div>

      <div className="table-container table-sticky">
        <table>
          <thead>
            <tr>
              <th>Session</th>
              <th>Requested by</th>
              <th>Tenant</th>
              <th>Trace</th>
              <th>Started</th>
              <th>Last event</th>
              <th>Decision mix</th>
              <th>Last action</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <TableSkeleton columns={9} rows={6} />
            ) : sessions.length === 0 ? (
              <tr>
                <td colSpan={9}>
                  <EmptyState
                    icon="↻"
                    title="No matching sessions"
                    description="Try widening the time range or filtering by agent, user, or trace instead of an exact session id."
                    action={
                      <button className="btn btn-outline btn-sm" type="button" onClick={() => setFilters(defaultFilters)}>
                        Reset filters
                      </button>
                    }
                  />
                </td>
              </tr>
            ) : (
              sessions.map(session => (
                <tr key={`${session.tenant_id}:${session.id}`}>
                  <td>
                    <div className="table-primary">
                      <Link to={`/sessions/${encodeURIComponent(session.id)}${buildQuery({ tenant_id: session.tenant_id })}`}>{shortID(session.id, 14)}</Link>
                    </div>
                    <div className="table-subtext">{session.event_count} events</div>
                  </td>
                  <td>
                    <div className="table-primary">{formatRequester(session.user_id, session.user_name, session.user_email, session.agent_id)}</div>
                    <div className="table-subtext">Agent {noneText(session.agent_id)}</div>
                  </td>
                  <td>
                    <code>{shortID(session.tenant_id, 12)}</code>
                  </td>
                  <td>
                    <div className="table-primary">{noneText(shortID(session.trace_id, 12))}</div>
                  </td>
                  <td>{formatDate(session.started_at)}</td>
                  <td>{formatDate(session.last_event_at)}</td>
                  <td>
                    <div className="stacked-badges">
                      <span className="badge badge-green">Allow {session.allow_count || 0}</span>
                      <span className="badge badge-red">Deny {session.deny_count || 0}</span>
                      <span className="badge badge-yellow">Approve {session.approve_count || 0}</span>
                    </div>
                  </td>
                  <td>
                    <div className="table-primary">
                      {session.last_tool || '(unknown)'}.{session.last_action || '(unknown)'}
                    </div>
                    <div className="table-subtext">
                      <span className={`badge badge-${decisionTone(session.last_decision)}`}>{noneText(session.last_decision)}</span>
                      {' · '}Risk {session.last_risk_score ?? 0}
                    </div>
                  </td>
                  <td>
                    <Link
                      to={`/sessions/${encodeURIComponent(session.id)}${buildQuery({ tenant_id: session.tenant_id })}`}
                      className="btn btn-outline btn-sm"
                    >
                      Open run
                    </Link>
                  </td>
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
        <button className="btn btn-outline btn-sm" disabled={loading || sessions.length < limit} onClick={() => setPage(current => current + 1)}>
          Next
        </button>
      </div>
    </div>
  )
}
