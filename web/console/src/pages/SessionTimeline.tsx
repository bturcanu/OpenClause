import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { APIClientError, api, formatDate } from '../api'
import { CopyIconButton, EmptyState, InlineErrorState, PageHeaderBlock, StatCard, buildQuery, copyText, decisionTone, downloadBlob, formatRequester, noneText } from '../ui'

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

type SessionIssueSummary = {
  stage: string
  message: string
  requestId?: string
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

function isSessionSummary(value: unknown): value is SessionSummary {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const candidate = value as Partial<SessionSummary>
  return typeof candidate.id === 'string' && candidate.id.trim() !== '' && typeof candidate.tenant_id === 'string' && candidate.tenant_id.trim() !== ''
}

function normalizeSessionSummary(payload: unknown): SessionSummary | null {
  if (Array.isArray(payload)) return payload.find(isSessionSummary) || null
  if (!payload || typeof payload !== 'object') return null
  const wrapped = (payload as { session?: unknown }).session
  if (isSessionSummary(wrapped)) return wrapped
  return isSessionSummary(payload) ? payload : null
}

function isSessionTimelineEvent(value: unknown): value is SessionTimelineEvent {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const candidate = value as Partial<SessionTimelineEvent>
  return (
    typeof candidate.event_id === 'string' &&
    candidate.event_id.trim() !== '' &&
    typeof candidate.tenant_id === 'string' &&
    candidate.tenant_id.trim() !== '' &&
    typeof candidate.agent_id === 'string' &&
    candidate.agent_id.trim() !== '' &&
    typeof candidate.tool === 'string' &&
    candidate.tool.trim() !== '' &&
    typeof candidate.action === 'string' &&
    candidate.action.trim() !== '' &&
    typeof candidate.risk_score === 'number' &&
    typeof candidate.decision === 'string' &&
    candidate.decision.trim() !== '' &&
    typeof candidate.session_id === 'string' &&
    candidate.session_id.trim() !== '' &&
    typeof candidate.received_at === 'string' &&
    candidate.received_at.trim() !== '' &&
    typeof candidate.explain === 'string'
  )
}

function normalizeTimelineEvents(payload: unknown): { events: SessionTimelineEvent[]; dropped: number } | null {
  if (Array.isArray(payload)) {
    const events = payload.filter(isSessionTimelineEvent)
    return { events, dropped: payload.length - events.length }
  }
  if (!payload || typeof payload !== 'object') return null
  const wrapped = (payload as { events?: unknown }).events
  if (!Array.isArray(wrapped)) return null
  const events = wrapped.filter(isSessionTimelineEvent)
  return { events, dropped: wrapped.length - events.length }
}

export default function SessionTimeline() {
  const { id = '' } = useParams<{ id: string }>()
  const [searchParams, setSearchParams] = useSearchParams()
  const tenantID = searchParams.get('tenant_id') || ''
  const [session, setSession] = useState<SessionSummary | null>(null)
  const [events, setEvents] = useState<SessionTimelineEvent[]>([])
  const [filters, setFilters] = useState<TimelineFilters>(defaultFilters)
  const [selectedExplain, setSelectedExplain] = useState<SessionTimelineEvent | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [timelineLoadFailed, setTimelineLoadFailed] = useState(false)
  const [ambiguityMessage, setAmbiguityMessage] = useState('')
  const [tenantCandidates, setTenantCandidates] = useState<string[]>([])
  const [selectedTenantCandidate, setSelectedTenantCandidate] = useState('')
  const [copyStatus, setCopyStatus] = useState('')
  const [triageNotice, setTriageNotice] = useState('')
  const [latestIssue, setLatestIssue] = useState<SessionIssueSummary | null>(null)
  const fetchSeq = useRef(0)
  const issueCounts = useRef<Record<string, number>>({})

  const clearSessionIssues = useCallback((...stages: string[]) => {
    let changed = false
    stages.forEach(stage => {
      if (issueCounts.current[stage]) {
        delete issueCounts.current[stage]
        changed = true
      }
    })
    if (changed && Object.keys(issueCounts.current).length === 0) {
      setTriageNotice('')
      setLatestIssue(null)
    }
  }, [])

  const markSessionIssue = useCallback((stage: string, requestId?: string) => {
    const nextCount = (issueCounts.current[stage] || 0) + 1
    issueCounts.current[stage] = nextCount
    if (nextCount < 2) return

    const stageLabel = stage.startsWith('export:')
      ? `session ${stage.replace(':', ' ')}`
      : stage.replace(/-/g, ' ')
    setTriageNotice(
      `Repeated ${stageLabel} failures detected for this run. Check the latest request id${requestId ? ` (${requestId})` : ''} and browser console details before retrying.`,
    )
  }, [])

  const logSessionDetailIssue = useCallback((stage: string, err: unknown, extra: Record<string, unknown> = {}) => {
    const message = err instanceof Error ? err.message : String(err || 'Unknown session detail failure')
    const requestId = err instanceof APIClientError ? err.requestId : undefined
    markSessionIssue(stage, requestId)
    setLatestIssue({ stage, message, requestId })
    console.warn('[openclause-console] session detail issue', {
      stage,
      sessionId: id,
      tenantId: tenantID || undefined,
      requestId,
      message,
      ...extra,
    })
  }, [id, markSessionIssue, tenantID])

  const sessionDiagnostics = useMemo(() => {
    if (!latestIssue) return ''
    return [
      'OpenClause session detail diagnostics',
      `session_id=${id || ''}`,
      `tenant_id=${tenantID || ''}`,
      `stage=${latestIssue.stage}`,
      `request_id=${latestIssue.requestId || ''}`,
      `message=${latestIssue.message}`,
    ].join('\n')
  }, [id, latestIssue, tenantID])

  const handleSessionError = useCallback((err: unknown) => {
    const message = err instanceof Error ? err.message : 'Failed to load session'
    if (err instanceof APIClientError && (err.candidates?.length || 0) > 0) {
      setTenantCandidates(err.candidates || [])
      setSelectedTenantCandidate('')
      setAmbiguityMessage('This session id exists in multiple tenants. Pick the tenant you want to inspect and the page will reload automatically.')
      setError('')
      return
    }
    setTenantCandidates([])
    setSelectedTenantCandidate('')
    setAmbiguityMessage('')
    setError(message)
  }, [])

  const fetchSession = useCallback(async () => {
    const seq = ++fetchSeq.current
    setLoading(true)
    setError('')
    setTimelineLoadFailed(false)
    if (tenantID) {
      setTenantCandidates([])
      setSelectedTenantCandidate('')
      setAmbiguityMessage('')
    }
    try {
      const query = buildQuery({ tenant_id: tenantID })
      const [summaryResult, timelineResult] = await Promise.allSettled([
        api.get(`/admin/sessions/${encodeURIComponent(id)}${query}`),
        api.get(`/admin/sessions/${encodeURIComponent(id)}/timeline${query}`),
      ])
      if (seq !== fetchSeq.current) return

      if (summaryResult.status === 'rejected') {
        setSession(null)
        setEvents([])
        logSessionDetailIssue('summary', summaryResult.reason)
        handleSessionError(summaryResult.reason)
        return
      }

      const normalizedSummary = normalizeSessionSummary(summaryResult.value)
      if (!normalizedSummary) {
        setSession(null)
        setEvents([])
        logSessionDetailIssue('summary-contract', new Error('Malformed session summary payload'))
        return
      }
      setSession(normalizedSummary)
      clearSessionIssues('summary', 'summary-contract', 'fetch')

      if (timelineResult.status === 'fulfilled') {
        const normalizedTimeline = normalizeTimelineEvents(timelineResult.value)
        if (normalizedTimeline === null) {
          const malformedTimelineError = new Error('The session summary loaded, but the timeline payload was malformed.')
          setEvents([])
          setTimelineLoadFailed(true)
          logSessionDetailIssue('timeline-contract', malformedTimelineError)
          setError(malformedTimelineError.message)
        } else {
          if (normalizedTimeline.events.length === 0 && normalizedTimeline.dropped > 0) {
            const malformedRowsError = new Error('The session summary loaded, but every timeline row was malformed.')
            setEvents([])
            setTimelineLoadFailed(true)
            logSessionDetailIssue('timeline-contract', malformedRowsError, { droppedRows: normalizedTimeline.dropped })
            setError(malformedRowsError.message)
          } else {
            setEvents(normalizedTimeline.events)
            if (normalizedTimeline.dropped > 0) {
              const partialTimelineError = new Error('Some timeline rows were malformed and were ignored.')
              logSessionDetailIssue('timeline-contract', partialTimelineError, { droppedRows: normalizedTimeline.dropped })
              setError(partialTimelineError.message)
            } else {
              setError('')
              clearSessionIssues('summary', 'summary-contract', 'timeline', 'timeline-contract', 'fetch')
            }
          }
        }
      } else {
        setEvents([])
        setTimelineLoadFailed(true)
        logSessionDetailIssue('timeline', timelineResult.reason)
        setError(timelineResult.reason instanceof Error ? timelineResult.reason.message : 'The session summary loaded, but the timeline could not be loaded.')
      }

      setTenantCandidates([])
      setSelectedTenantCandidate('')
      setAmbiguityMessage('')
    } catch (err: any) {
      if (seq === fetchSeq.current) {
        setSession(null)
        setEvents([])
        setTimelineLoadFailed(false)
        logSessionDetailIssue('fetch', err)
        handleSessionError(err)
      }
    } finally {
      if (seq === fetchSeq.current) setLoading(false)
    }
  }, [handleSessionError, id, logSessionDetailIssue, tenantID])

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
      clearSessionIssues(`export:${kind}`)
    } catch (err: any) {
      logSessionDetailIssue(`export:${kind}`, err)
      handleSessionError(err)
    }
  }

  function handleTenantCandidateChange(value: string) {
    setSelectedTenantCandidate(value)
    if (!value) return
    const next = new URLSearchParams(searchParams)
    next.set('tenant_id', value)
    setTenantCandidates([])
    setAmbiguityMessage('')
    setError('')
    setSearchParams(next)
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
            <details className="action-menu">
              <summary className="btn btn-outline" aria-disabled={loading || !session || tenantCandidates.length > 0}>
                Export ▾
              </summary>
              <div className="action-menu-list">
                <button className="action-menu-item" type="button" onClick={() => void exportSession('csv')} disabled={loading || !session || tenantCandidates.length > 0}>
                  Export CSV
                </button>
                <button className="action-menu-item" type="button" onClick={() => void exportSession('json')} disabled={loading || !session || tenantCandidates.length > 0}>
                  Export JSON
                </button>
              </div>
            </details>
            <button className="btn btn-primary" type="button" onClick={() => void handleCopySummary()} disabled={loading || !session || tenantCandidates.length > 0}>
              Copy shareable summary
            </button>
          </div>
        }
      />

      {copyStatus ? <div className="success-msg mb-16">{copyStatus}</div> : null}
      {!triageNotice && latestIssue && (error || timelineLoadFailed) ? (
        <div className="warn-banner warn-banner-subtle mb-16">
          <div className="warn-banner-header">
            <div className="warn-banner-title">Latest diagnostics</div>
            <CopyIconButton text={sessionDiagnostics} label="Session diagnostics" disabled={!sessionDiagnostics} />
          </div>
          <div className="form-helper-text helper-text-warn">{latestIssue.message}</div>
          <div className="warn-banner-meta">
            <span>Latest stage: <code className="mono">{latestIssue.stage}</code></span>
            {latestIssue.requestId ? <span>Request ID: <code className="mono">{latestIssue.requestId}</code></span> : null}
          </div>
        </div>
      ) : null}
      {triageNotice ? (
        <div className="warn-banner mb-16">
          <div className="warn-banner-header">
            <div className="warn-banner-title">Repeated failures detected</div>
            <CopyIconButton text={sessionDiagnostics} label="Session diagnostics" disabled={!sessionDiagnostics} />
          </div>
          <div className="form-helper-text helper-text-warn">{triageNotice}</div>
          {latestIssue ? (
            <>
              <div className="form-helper-text helper-text-warn">{latestIssue.message}</div>
              <div className="warn-banner-meta">
                <span>Latest stage: <code className="mono">{latestIssue.stage}</code></span>
                {latestIssue.requestId ? <span>Request ID: <code className="mono">{latestIssue.requestId}</code></span> : null}
              </div>
            </>
          ) : null}
        </div>
      ) : null}
      {error && tenantCandidates.length === 0 && !timelineLoadFailed ? <InlineErrorState message={error} onRetry={() => void fetchSession()} /> : null}

      {tenantCandidates.length > 0 ? (
        <div className="detail-panel">
          <h3>Pick a tenant to continue</h3>
          <p className="table-subtext">
            {ambiguityMessage || 'This session id exists in multiple tenants for your platform-admin view.'}
          </p>
            <div className="form-inline mt-16">
            <div className="form-group session-tenant-picker">
              <label htmlFor="session-tenant-candidate">Tenant</label>
              <select id="session-tenant-candidate" value={selectedTenantCandidate} onChange={e => handleTenantCandidateChange(e.target.value)}>
                <option value="">Choose a tenant</option>
                {tenantCandidates.map(candidate => (
                  <option key={candidate} value={candidate}>{candidate}</option>
                ))}
              </select>
            </div>
          </div>
        </div>
      ) : null}

      {session ? (
        <div className="stats-grid">
          <StatCard label="Events" value={session.event_count} hint="Events attached to this run" />
          <StatCard label="Started" value={formatDate(session.started_at)} />
          <StatCard label="Last event" value={formatDate(session.last_event_at)} />
          <StatCard label="Decision mix" value={`${session.allow_count || 0}/${session.deny_count || 0}/${session.approve_count || 0}`} hint="allow / deny / approve" />
        </div>
      ) : null}

      {session && tenantCandidates.length === 0 ? (
        <div className="detail-panel">
          <h3>Run context</h3>
          <div className="identity-grid">
            <div className="identity-card">
              <span className="meta-label">Requested by</span>
              <div className="identity-primary">{formatRequester(session.user_id, session.user_name, session.user_email, session.agent_id)}</div>
              <div className="identity-secondary">Use these IDs to line up approvals, analytics, and downstream traces.</div>
            </div>
            <div className="identity-card">
              <span className="meta-label">Session</span>
              <div className="identity-copy-row">
                <code className="mono" title={noneText(session.id)}>{noneText(session.id)}</code>
                <CopyIconButton text={session.id} label="Session ID" />
              </div>
            </div>
            <div className="identity-card">
              <span className="meta-label">Tenant</span>
              <div className="identity-copy-row">
                <code className="mono" title={noneText(session.tenant_id)}>{noneText(session.tenant_id)}</code>
                <CopyIconButton text={session.tenant_id} label="Tenant ID" />
              </div>
            </div>
            <div className="identity-card">
              <span className="meta-label">Agent</span>
              <div className="identity-copy-row">
                <code className="mono" title={noneText(session.agent_id)}>{noneText(session.agent_id)}</code>
                <CopyIconButton text={session.agent_id} label="Agent ID" />
              </div>
            </div>
            <div className="identity-card">
              <span className="meta-label">User ID</span>
              <div className="identity-copy-row">
                <code className="mono" title={noneText(session.user_id)}>{noneText(session.user_id)}</code>
                <CopyIconButton text={session.user_id} label="User ID" disabled={!session.user_id} />
              </div>
            </div>
            <div className="identity-card">
              <span className="meta-label">Trace</span>
              <div className="identity-copy-row">
                <code className="mono" title={noneText(session.trace_id)}>{noneText(session.trace_id)}</code>
                <CopyIconButton text={session.trace_id} label="Trace ID" disabled={!session.trace_id} />
              </div>
            </div>
          </div>
        </div>
      ) : null}

      {session && tenantCandidates.length === 0 ? (
        <div className="filters-panel">
          <div className="filters-panel-note">Timeline filters use your local browser time.</div>
          <div className="filters-bar filters-bar-dense">
            <div className="form-group">
              <label htmlFor="session-filter-decision">Decision</label>
              <select id="session-filter-decision" value={filters.decision} onChange={e => updateFilter('decision', e.target.value)}>
                <option value="">Any</option>
                <option value="allow">Allow</option>
                <option value="deny">Deny</option>
                <option value="approve">Approve</option>
              </select>
            </div>
            <div className="form-group">
              <label htmlFor="session-filter-tool">Tool</label>
              <input id="session-filter-tool" value={filters.tool} onChange={e => updateFilter('tool', e.target.value)} placeholder="slack" />
            </div>
            <div className="form-group">
              <label htmlFor="session-filter-action">Action</label>
              <input id="session-filter-action" value={filters.action} onChange={e => updateFilter('action', e.target.value)} placeholder="msg.post" />
            </div>
            <div className="form-group form-group-small">
              <label htmlFor="session-filter-risk-min">Risk min</label>
              <input id="session-filter-risk-min" type="number" min={0} max={10} inputMode="numeric" value={filters.risk_min} onChange={e => updateFilter('risk_min', e.target.value)} placeholder="0" />
            </div>
            <div className="form-group form-group-small">
              <label htmlFor="session-filter-risk-max">Risk max</label>
              <input id="session-filter-risk-max" type="number" min={0} max={10} inputMode="numeric" value={filters.risk_max} onChange={e => updateFilter('risk_max', e.target.value)} placeholder="10" />
            </div>
            <div className="form-group">
              <label htmlFor="session-filter-since">Since (local time)</label>
              <input id="session-filter-since" value={filters.since} onChange={e => updateFilter('since', e.target.value)} type="datetime-local" />
            </div>
            <div className="form-group">
              <label htmlFor="session-filter-until">Until (local time)</label>
              <input id="session-filter-until" value={filters.until} onChange={e => updateFilter('until', e.target.value)} type="datetime-local" />
            </div>
            <div className="form-group">
              <label>&nbsp;</label>
              <button className="btn btn-outline" type="button" onClick={() => setFilters(defaultFilters)}>
                Reset filters
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {loading ? (
        <div className="detail-panel">
          <div className="skeleton-line skeleton-line-lg" />
          <div className="skeleton-line" />
          <div className="skeleton-line" />
        </div>
      ) : tenantCandidates.length > 0 ? null : !session ? (
        <EmptyState
          icon="↻"
          title="Session not found"
          description="We could not find a run for this session id in the selected scope."
          action={
            <Link to="/sessions" className="btn btn-outline btn-sm">
              Back to sessions
            </Link>
          }
        />
      ) : timelineLoadFailed ? (
        <div className="detail-panel">
          <InlineErrorState
            message={error || 'The session summary loaded, but the timeline could not be loaded.'}
            onRetry={() => void fetchSession()}
          />
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
                  <strong>Why OpenClause decided this way</strong>
                  <p>{event.explain}</p>
                  <p className="table-subtext">{event.policy_reason || 'A detailed policy reason was not recorded for this event.'}</p>
                  <div className="session-callout-actions">
                    <Link to={`/policies?tenant_id=${encodeURIComponent(event.tenant_id)}`} className="btn btn-outline btn-sm">
                      Review tenant policy
                    </Link>
                  </div>
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
                    <div className="mini-detail-list">
                      <div className="mini-detail-row">
                        <span className="meta-label">Approval ID</span>
                        <div className="identity-copy-row">
                          <code className="mono" title={event.approval.id}>{event.approval.id}</code>
                          <CopyIconButton text={event.approval.id} label="Approval ID" />
                        </div>
                      </div>
                      <div className="mini-detail-row">
                        <span className="meta-label">Status</span>
                        <span className={`badge badge-${decisionTone(event.approval.status)}`}>{event.approval.status}</span>
                      </div>
                      {event.approval.reason ? (
                        <div className="mini-detail-row">
                          <span className="meta-label">Approver note</span>
                          <div>{event.approval.reason}</div>
                        </div>
                      ) : null}
                      {event.approval.deny_reason ? (
                        <div className="mini-detail-row">
                          <span className="meta-label">Deny reason</span>
                          <div>{event.approval.deny_reason}</div>
                        </div>
                      ) : null}
                    </div>
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
                    <div className="mini-detail-list">
                      <div className="mini-detail-row">
                        <span className="meta-label">Execution event</span>
                        <div className="identity-copy-row">
                          <code className="mono" title={event.execution.event_id}>{event.execution.event_id}</code>
                          <CopyIconButton text={event.execution.event_id} label="Execution event ID" />
                        </div>
                      </div>
                      <div className="mini-detail-row">
                        <span className="meta-label">Status</span>
                        <span className={`badge badge-${decisionTone(event.execution.status)}`}>{event.execution.status}</span>
                      </div>
                      {typeof event.execution.duration_ms === 'number' ? (
                        <div className="mini-detail-row">
                          <span className="meta-label">Duration</span>
                          <div>{event.execution.duration_ms}ms</div>
                        </div>
                      ) : null}
                      {event.execution.error_msg ? (
                        <div className="mini-detail-row">
                          <span className="meta-label">Error</span>
                          <div>{event.execution.error_msg}</div>
                        </div>
                      ) : null}
                    </div>
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
