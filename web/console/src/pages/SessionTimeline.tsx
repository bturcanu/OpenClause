import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { api, formatDate } from '../api'
import { EmptyState, InlineErrorState, PageHeaderBlock, StatCard, buildQuery, copyText, decisionTone, downloadBlob, formatRequester, noneText } from '../ui'

type SessionSummary = {
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
}

type SessionTimelineEvent = {
  event_id: string
  tenant_id: string
  agent_id: string
  user_id?: string
  user_name?: string
  user_email?: string
  tool: string
  action: string
  resource?: string
  risk_score: number
  decision: string
  session_id: string
  trace_id?: string
  received_at: string
  policy_reason?: string
  risk_factors?: string[]
  explain: string
  approval?: {
    id: string
    status: string
    reason?: string
    deny_reason?: string
    created_at: string
    expires_at: string
  } | null
  execution?: {
    event_id: string
    received_at: string
    status: string
    error_msg?: string
    duration_ms?: number
    policy_reason?: string
  } | null
}

type TimelineFilters = {
  decision: string
  tool: string
  action: string
  risk_min: string
  risk_max: string
  since: string
  until: string
}

const defaultFilters: TimelineFilters = {
  decision: '',
  tool: '',
  action: '',
  risk_min: '',
  risk_max: '',
  since: '',
  until: '',
}

export default function SessionTimeline() {
  const { id = '' } = useParams<{ id: string }>()
  const [searchParams] = useSearchParams()
  const tenantID = searchParams.get('tenant_id') || ''
  const [session, setSession] = useState<SessionSummary | null>(null)
  const [events, setEvents] = useState<SessionTimelineEvent[]>([])
  const [filters, setFilters] = useState<TimelineFilters>(defaultFilters)
  const [selectedExplain, setSelectedExplain] = useState<SessionTimelineEvent | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [copyStatus, setCopyStatus] = useState('')
  const fetchSeq = useRef(0)

  const fetchSession = useCallback(async () => {
    const seq = ++fetchSeq.current
    setLoading(true)
    setError('')
    try {
      const query = buildQuery({ tenant_id: tenantID })
      const [summary, timeline] = await Promise.all([
        api.get(`/admin/sessions/${encodeURIComponent(id)}${query}`),
        api.get(`/admin/sessions/${encodeURIComponent(id)}/timeline${query}`),
      ])
      if (seq !== fetchSeq.current) return
      setSession(summary)
      setEvents(Array.isArray(timeline) ? timeline : timeline?.events || [])
    } catch (err: any) {
      if (seq === fetchSeq.current) setError(err.message)
    } finally {
      if (seq === fetchSeq.current) setLoading(false)
    }
  }, [id, tenantID])

  useEffect(() => {
    if (!id) return
    void fetchSession()
  }, [fetchSession, id])

  function updateFilter(key: keyof TimelineFilters, value: string) {
    setFilters(current => ({ ...current, [key]: value }))
  }

  const filteredEvents = useMemo(() => {
    const riskMin = filters.risk_min ? Number(filters.risk_min) : null
    const riskMax = filters.risk_max ? Number(filters.risk_max) : null
    const since = filters.since ? new Date(filters.since) : null
    const until = filters.until ? new Date(filters.until) : null
    return events.filter(event => {
      if (filters.decision && event.decision !== filters.decision) return false
      if (filters.tool && event.tool !== filters.tool.trim()) return false
      if (filters.action && event.action !== filters.action.trim()) return false
      if (riskMin !== null && event.risk_score < riskMin) return false
      if (riskMax !== null && event.risk_score > riskMax) return false
      if (since && !Number.isNaN(since.getTime()) && new Date(event.received_at) < since) return false
      if (until && !Number.isNaN(until.getTime()) && new Date(event.received_at) > until) return false
      return true
    })
  }, [events, filters])

  const shareableSummary = useMemo(() => {
    if (!session) return ''
    const lines = [
      `Session ${session.id}`,
      `Tenant: ${session.tenant_id}`,
      formatRequester(session.user_id, session.user_name, session.user_email, session.agent_id),
      `Started: ${formatDate(session.started_at)}`,
      `Last event: ${formatDate(session.last_event_at)}`,
      `Decision mix: allow ${session.allow_count || 0}, deny ${session.deny_count || 0}, approve ${session.approve_count || 0}`,
      '',
      'Timeline:',
    ]
    filteredEvents.forEach(event => {
      lines.push(`- ${formatDate(event.received_at)} | ${event.tool}.${event.action} | ${event.decision.toUpperCase()} | ${event.explain}`)
    })
    return lines.join('\n')
  }, [filteredEvents, session])

  async function handleCopySummary() {
    try {
      await copyText(shareableSummary)
      setCopyStatus('Summary copied')
      window.setTimeout(() => setCopyStatus(''), 1800)
    } catch {
      setCopyStatus('Copy failed')
      window.setTimeout(() => setCopyStatus(''), 1800)
    }
  }

  async function exportSession(kind: 'csv' | 'json') {
    try {
      const query = buildQuery({ tenant_id: tenantID })
      const blob = await api.getBlob(`/admin/sessions/${encodeURIComponent(id)}/export/${kind}${query}`)
      downloadBlob(blob, `session-${id}.${kind}`)
    } catch (err: any) {
      setError(err.message)
    }
  }

  return (
    <div>
      <PageHeaderBlock
        title="Session detail"
        description={session ? `Follow the run from request to policy decision, approval, and execution.` : 'Follow a single run end to end.'}
        actions={
          <div className="btn-group">
            <Link to="/sessions" className="btn btn-outline">
              Back to sessions
            </Link>
            <button className="btn btn-outline" type="button" onClick={() => void exportSession('csv')} disabled={loading}>
              Export CSV
            </button>
            <button className="btn btn-outline" type="button" onClick={() => void exportSession('json')} disabled={loading}>
              Export JSON
            </button>
            <button className="btn btn-primary" type="button" onClick={() => void handleCopySummary()} disabled={loading || !session}>
              Copy shareable summary
            </button>
          </div>
        }
      />

      {copyStatus ? <div className="success-msg mb-16">{copyStatus}</div> : null}
      {error ? <InlineErrorState message={error} onRetry={() => void fetchSession()} /> : null}

      {session ? (
        <div className="stats-grid">
          <StatCard label="Requested by" value={formatRequester(session.user_id, session.user_name, session.user_email, session.agent_id)} hint={`Trace ${noneText(session.trace_id)}`} />
          <StatCard label="Started" value={formatDate(session.started_at)} />
          <StatCard label="Last event" value={formatDate(session.last_event_at)} />
          <StatCard label="Decision mix" value={`${session.allow_count || 0}/${session.deny_count || 0}/${session.approve_count || 0}`} hint="allow / deny / approve" />
        </div>
      ) : null}

      <div className="filters-panel">
        <div className="filters-bar filters-bar-dense">
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
            <label>Tool</label>
            <input value={filters.tool} onChange={e => updateFilter('tool', e.target.value)} placeholder="slack" />
          </div>
          <div className="form-group">
            <label>Action</label>
            <input value={filters.action} onChange={e => updateFilter('action', e.target.value)} placeholder="msg.post" />
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
          <div className="form-group">
            <label>&nbsp;</label>
            <button className="btn btn-outline" type="button" onClick={() => setFilters(defaultFilters)}>
              Reset filters
            </button>
          </div>
        </div>
      </div>

      {loading ? (
        <div className="detail-panel">
          <div className="skeleton-line skeleton-line-lg" />
          <div className="skeleton-line" />
          <div className="skeleton-line" />
        </div>
      ) : filteredEvents.length === 0 ? (
        <EmptyState
          icon="↻"
          title="No timeline items match these filters"
          description="This run exists, but the current decision, action, or risk filters hid every event."
          action={
            <button className="btn btn-outline btn-sm" type="button" onClick={() => setFilters(defaultFilters)}>
              Reset filters
            </button>
          }
        />
      ) : (
        <div className="timeline timeline-rich">
          {filteredEvents.map(event => (
            <div key={event.event_id} className={`timeline-item ${event.decision}`}>
              <div className="tl-time">{formatDate(event.received_at)}</div>
              <div className="tl-content tl-content-rich">
                <div className="session-card-header">
                  <div>
                    <div className="tl-title">
                      {event.tool}.{event.action}
                    </div>
                    <div className="table-subtext">{formatRequester(event.user_id, event.user_name, event.user_email, event.agent_id)}</div>
                  </div>
                  <div className="stacked-badges">
                    <span className={`badge badge-${decisionTone(event.decision)}`}>{event.decision}</span>
                    <span className="badge badge-gray">Risk {event.risk_score}</span>
                  </div>
                </div>

                <div className="session-meta-grid">
                  <div>
                    <span className="meta-label">Trace</span>
                    <div className="meta-value">{noneText(event.trace_id)}</div>
                  </div>
                  <div>
                    <span className="meta-label">Resource</span>
                    <div className="meta-value">{noneText(event.resource)}</div>
                  </div>
                  <div>
                    <span className="meta-label">Event</span>
                    <div className="meta-value mono">
                      <Link to={`/events/${event.event_id}`}>{event.event_id}</Link>
                    </div>
                  </div>
                </div>

                <div className="session-callout">
                  <strong>Why this happened</strong>
                  <p>{event.policy_reason || 'Policy reason was not recorded for this event.'}</p>
                </div>

                {event.risk_factors && event.risk_factors.length > 0 ? (
                  <div className="stacked-badges">
                    {event.risk_factors.map(factor => (
                      <span key={`${event.event_id}:${factor}`} className="badge badge-blue">
                        {factor}
                      </span>
                    ))}
                  </div>
                ) : null}

                {event.approval ? (
                  <div className="session-subpanel">
                    <div className="session-subpanel-header">
                      <strong>Approval</strong>
                      <Link
                        to={`/approvals${buildQuery({ approval_id: event.approval.id, tenant_id: event.tenant_id })}`}
                        className="btn btn-outline btn-sm"
                      >
                        Open approval
                      </Link>
                    </div>
                    <p>
                      Approval <code>{event.approval.id}</code> is <strong>{event.approval.status}</strong>.
                    </p>
                    {event.approval.reason ? <p className="table-subtext">Reason: {event.approval.reason}</p> : null}
                    {event.approval.deny_reason ? <p className="table-subtext">Deny reason: {event.approval.deny_reason}</p> : null}
                  </div>
                ) : null}

                {event.execution ? (
                  <div className="session-subpanel">
                    <div className="session-subpanel-header">
                      <strong>Execution</strong>
                      <Link to={`/events/${event.execution.event_id}`} className="btn btn-outline btn-sm">
                        Open execution event
                      </Link>
                    </div>
                    <p>
                      Execution finished with <strong>{event.execution.status}</strong>
                      {typeof event.execution.duration_ms === 'number' ? ` in ${event.execution.duration_ms}ms.` : '.'}
                    </p>
                    {event.execution.error_msg ? <p className="table-subtext">Error: {event.execution.error_msg}</p> : null}
                  </div>
                ) : null}

                <div className="btn-group mt-16">
                  <button className="btn btn-outline btn-sm" type="button" onClick={() => setSelectedExplain(event)}>
                    Explain
                  </button>
                  <Link to={`/events/${event.event_id}`} className="btn btn-outline btn-sm">
                    Event detail
                  </Link>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {selectedExplain ? (
        <div className="modal-backdrop" onClick={() => setSelectedExplain(null)}>
          <div className="modal modal-side-panel" onClick={e => e.stopPropagation()}>
            <div className="flex-between mb-16">
              <div>
                <h3>Explain decision</h3>
                <p className="table-subtext">{selectedExplain.tool}.{selectedExplain.action}</p>
              </div>
              <button className="btn btn-outline btn-sm" type="button" onClick={() => setSelectedExplain(null)}>
                Close
              </button>
            </div>
            <div className="session-callout session-callout-strong">
              <p>{selectedExplain.explain}</p>
            </div>
            <div className="detail-row">
              <div className="detail-label">Policy reason</div>
              <div className="detail-value">{selectedExplain.policy_reason || 'Not recorded'}</div>
            </div>
            <div className="detail-row">
              <div className="detail-label">Risk factors</div>
              <div className="detail-value">{selectedExplain.risk_factors?.length ? selectedExplain.risk_factors.join(', ') : 'None recorded'}</div>
            </div>
            <div className="detail-row">
              <div className="detail-label">Requested by</div>
              <div className="detail-value">{formatRequester(selectedExplain.user_id, selectedExplain.user_name, selectedExplain.user_email, selectedExplain.agent_id)}</div>
            </div>
            <div className="detail-row">
              <div className="detail-label">Decision</div>
              <div className="detail-value">
                <span className={`badge badge-${decisionTone(selectedExplain.decision)}`}>{selectedExplain.decision}</span>
              </div>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  )
}
