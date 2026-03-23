import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, toQueryTimestamp } from '../api'
import {
  ActiveFiltersBar,
  TableSkeleton,
  InlineErrorState,
  PageHeaderBlock,
  StatCard,
  CopyIconButton,
  SortHeader,
  TableEmptyStateRow,
  TableFrame,
  applySort,
  buildQuery,
  compareDate,
  compareNumber,
  decisionTone,
  formatRequester,
  formatTimeWithTitle,
  noneText,
  shortID,
  type SortState,
} from '../ui'

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
  const [sortState, setSortState] = useState<SortState<'started_at' | 'last_event_at' | 'event_count' | 'approve_count'>>({
    key: null,
    dir: 'desc',
  })
  const limit = 25

  const fetchSessions = useCallback(async () => {
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

  function resetFilters() {
    setFilters({ ...defaultFilters })
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

  const activeFilterChips = useMemo(
    () =>
      (Object.entries(filters) as Array<[keyof SessionFilters, string]>)
        .filter(([, value]) => value.trim() !== '')
        .map(([key, value]) => ({
          key,
          label: `${key.replace(/_/g, ' ')}: ${value.trim()}`,
          onRemove: () => updateFilter(key, ''),
        })),
    [filters],
  )

  const visibleSessions = useMemo(() => {
    if (!sortState.key) return sessions
    return [...sessions].sort((left, right) => {
      switch (sortState.key) {
        case 'started_at':
          return applySort(compareDate(left.started_at, right.started_at), sortState.dir)
        case 'last_event_at':
          return applySort(compareDate(left.last_event_at, right.last_event_at), sortState.dir)
        case 'event_count':
          return applySort(compareNumber(left.event_count, right.event_count), sortState.dir)
        case 'approve_count':
          return applySort(compareNumber(left.approve_count, right.approve_count), sortState.dir)
        default:
          return 0
      }
    })
  }, [sessions, sortState])

  return (
    <div>
      <PageHeaderBlock
        title="Sessions"
        description="Trace a single agent run from request to decision, approval, and execution without leaving the console."
        actions={
          <div className="btn-group">
            <button className="btn btn-outline" type="button" onClick={resetFilters}>
              Clear filters
            </button>
            <button className="btn btn-primary" type="button" onClick={() => void fetchSessions()}>
              Refresh
            </button>
          </div>
        }
      />

      <div className="banner-note banner-note-compact mb-16">
        <div>
          <strong>Runs are grouped by tool-call `session_id`.</strong> Events without one still appear in Audit Trail as <strong>(none)</strong>. Platform admins will be asked to pick a tenant only when the same session id exists in multiple tenants.
        </div>
        <Link to="/events" className="link-button">
          Open Audit Trail
        </Link>
      </div>

      <div className="stats-grid">
        <StatCard label="Tracked sessions" value={sessions.length} hint="Current page after filters" />
        <StatCard label="Events in view" value={totals.eventCount} tone="blue" />
        <StatCard label="Denied in view" value={totals.denyCount} tone="red" />
        <StatCard label="Awaiting review in view" value={totals.approveCount} tone="yellow" />
      </div>

      {error ? <InlineErrorState message={error} onRetry={() => void fetchSessions()} /> : null}

      <div className="filters-panel">
        <div className="filters-panel-note">Date filters use your local browser time.</div>
        <div className="form-grid sessions-filter-grid">
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
          <div className="form-group form-group-small session-filter-field-short">
            <label>Risk min</label>
            <input
              type="number"
              min={0}
              max={10}
              inputMode="numeric"
              value={filters.risk_min}
              onChange={e => updateFilter('risk_min', e.target.value)}
              placeholder="0"
            />
          </div>
          <div className="form-group form-group-small session-filter-field-short">
            <label>Risk max</label>
            <input
              type="number"
              min={0}
              max={10}
              inputMode="numeric"
              value={filters.risk_max}
              onChange={e => updateFilter('risk_max', e.target.value)}
              placeholder="10"
            />
          </div>
          <div className="form-group session-filter-field-wide">
            <label>Since (local time)</label>
            <input value={filters.since} onChange={e => updateFilter('since', e.target.value)} type="datetime-local" />
          </div>
          <div className="form-group session-filter-field-wide">
            <label>Until (local time)</label>
            <input value={filters.until} onChange={e => updateFilter('until', e.target.value)} type="datetime-local" />
          </div>
        </div>
      </div>

      <ActiveFiltersBar
        resultCount={visibleSessions.length}
        resultLabel={visibleSessions.length === 1 ? 'session' : 'sessions'}
        chips={activeFilterChips}
        onClearAll={activeFilterChips.length > 0 ? resetFilters : undefined}
        note={sortState.key ? 'Sorted within the current page.' : 'Using backend order until you sort this page.'}
      />

      <TableFrame className="sessions-table-container" stickyHeader>
        <table className="sessions-table">
          <thead>
            <tr>
              <th>Session</th>
              <th>Requested by</th>
              <th>Tenant</th>
              <th className="col-time">
                <SortHeader label="Started" sortKey="started_at" sortState={sortState} onSortChange={(key, dir) => setSortState({ key, dir })} defaultDir="desc" className="col-time" />
              </th>
              <th className="col-time">
                <SortHeader label="Last event" sortKey="last_event_at" sortState={sortState} onSortChange={(key, dir) => setSortState({ key, dir })} defaultDir="desc" className="col-time" />
              </th>
              <th className="col-num">
                <SortHeader label="Events" sortKey="event_count" sortState={sortState} onSortChange={(key, dir) => setSortState({ key, dir })} defaultDir="desc" className="col-num" />
              </th>
              <th className="col-num">
                <SortHeader label="Approvals" sortKey="approve_count" sortState={sortState} onSortChange={(key, dir) => setSortState({ key, dir })} defaultDir="desc" className="col-num" />
              </th>
              <th>Last action</th>
              <th className="table-action-col"></th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <TableSkeleton columns={9} rows={6} />
            ) : visibleSessions.length === 0 ? (
              <TableEmptyStateRow
                colSpan={9}
                icon="↻"
                title="No matching sessions"
                description="Try widening the time range or filtering by agent, user, or trace instead of an exact session id."
                action={
                  <button className="btn btn-outline btn-sm" type="button" onClick={resetFilters}>
                    Reset filters
                  </button>
                }
              />
            ) : (
              visibleSessions.map(session => {
                const started = formatTimeWithTitle(session.started_at)
                const lastEvent = formatTimeWithTitle(session.last_event_at)
                return (
                <tr key={`${session.tenant_id}:${session.id}`}>
                  <td>
                    <div className="table-primary-cell">
                      <div className="inline-value-copy">
                        <Link
                          to={`/sessions/${encodeURIComponent(session.id)}${buildQuery({ tenant_id: session.tenant_id })}`}
                          className="table-primary table-primary-link"
                          title={session.id}
                        >
                          {shortID(session.id, 14)}
                        </Link>
                        <CopyIconButton text={session.id} label="Session ID" />
                      </div>
                      <div className="table-subtext">
                        Agent <span className="mono" title={session.agent_id || '(none)'}>{session.agent_id || '(none)'}</span>
                      </div>
                      <div className="inline-value-copy">
                        <span className="table-subtext" title={noneText(session.trace_id)}>
                          Trace {noneText(shortID(session.trace_id, 12))}
                        </span>
                        {session.trace_id ? <CopyIconButton text={session.trace_id} label="Trace ID" /> : null}
                      </div>
                    </div>
                  </td>
                  <td className="session-requester-cell">
                    <div className="table-primary">{formatRequester(session.user_id, session.user_name, session.user_email, session.agent_id)}</div>
                    <div className="table-subtext">{session.allow_count || 0} allow · {session.deny_count || 0} deny</div>
                  </td>
                  <td>
                    <div className="inline-value-copy">
                      <code title={session.tenant_id}>{shortID(session.tenant_id, 12)}</code>
                      <CopyIconButton text={session.tenant_id} label="Tenant ID" />
                    </div>
                  </td>
                  <td className="col-time" title={started.title}>{started.label}</td>
                  <td className="col-time" title={lastEvent.title}>{lastEvent.label}</td>
                  <td className="col-num tabular">{session.event_count || 0}</td>
                  <td className="col-num tabular">{session.approve_count || 0}</td>
                  <td className="session-last-action-cell">
                    <div className="table-primary-cell">
                      <div className="table-primary">
                      {session.last_tool || '(unknown)'}.{session.last_action || '(unknown)'}
                      </div>
                      <div className="stacked-badges">
                        <span className="badge badge-green">Allow {session.allow_count || 0}</span>
                        <span className="badge badge-red">Deny {session.deny_count || 0}</span>
                        <span className="badge badge-yellow">Approve {session.approve_count || 0}</span>
                      </div>
                      <div className="table-subtext">
                        <span className={`badge badge-${decisionTone(session.last_decision)}`}>{noneText(session.last_decision)}</span>
                        {' · '}Risk {session.last_risk_score ?? 0}
                      </div>
                    </div>
                  </td>
                  <td className="table-action-cell session-open-cell">
                    <Link
                      to={`/sessions/${encodeURIComponent(session.id)}${buildQuery({ tenant_id: session.tenant_id })}`}
                      className="btn btn-outline btn-sm"
                    >
                      Open run
                    </Link>
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
        <button className="btn btn-outline btn-sm" disabled={loading || sessions.length < limit} onClick={() => setPage(current => current + 1)}>
          Next
        </button>
      </div>
    </div>
  )
}
