import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { api, formatDate } from '../api'
import {
  CopyIconButton,
  InlineErrorState,
  PageHeaderBlock,
  StatCard,
  buildQuery,
  copyText,
  decisionTone,
  formatRequester,
  noneText,
} from '../ui'

type EventResult = {
  status: string
  error_msg?: string
  duration_ms?: number
  output_json?: unknown
}

type EventData = {
  event_id: string
  tenant_id: string
  agent_id: string
  user_id?: string
  user_name?: string
  user_email?: string
  session_id: string
  trace_id?: string
  tool: string
  action: string
  resource?: string
  payload_json: unknown
  risk_score: number
  decision: string
  policy_result: unknown
  prev_hash: string
  hash: string
  result: EventResult | null
  received_at: string
}

type ParsedPolicyResult = {
  decision?: string
  reason?: string
  approver_group?: string
  requirements?: Record<string, string>
  risk_overrides?: Record<string, number>
  notify?: Array<{ kind?: string; channel?: string; url?: string }>
}

function renderTechnicalBlock(value: unknown) {
  if (value === null || value === undefined || value === '') return 'Not recorded'
  return typeof value === 'string' ? value : JSON.stringify(value, null, 2)
}

function parsePolicyResult(value: unknown): ParsedPolicyResult | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  const candidate = value as ParsedPolicyResult
  return candidate
}

function statToneFor(value?: string | null): 'default' | 'green' | 'red' | 'yellow' | 'blue' {
  switch (decisionTone(value)) {
    case 'green':
      return 'green'
    case 'red':
      return 'red'
    case 'yellow':
      return 'yellow'
    default:
      return 'default'
  }
}

export default function EventDetail() {
  const { eventId } = useParams<{ eventId: string }>()
  const [event, setEvent] = useState<EventData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [copyStatus, setCopyStatus] = useState('')

  const fetchDetail = useCallback(async () => {
    if (!eventId) return
    setLoading(true)
    setError('')
    try {
      const data = await api.get(`/admin/events/${eventId}`)
      setEvent(data as EventData)
    } catch (err: any) {
      setError(err?.message || 'Failed to load event detail')
      setEvent(null)
    } finally {
      setLoading(false)
    }
  }, [eventId])

  useEffect(() => {
    void fetchDetail()
  }, [fetchDetail])

  async function handleCopy(label: string, value?: string | null) {
    const text = (value || '').trim()
    if (!text) return
    try {
      await copyText(text)
      setCopyStatus(`${label} copied`)
      window.setTimeout(() => setCopyStatus(''), 1800)
    } catch {
      setCopyStatus('Copy failed')
      window.setTimeout(() => setCopyStatus(''), 1800)
    }
  }

  if (loading) {
    return (
      <div className="detail-panel">
        <div className="skeleton-line skeleton-line-lg" />
        <div className="skeleton-line" />
        <div className="skeleton-line" />
      </div>
    )
  }

  if (error) {
    return (
      <div>
        <InlineErrorState message={error} onRetry={() => void fetchDetail()} />
        <Link to="/events" className="btn btn-outline">
          Back to audit trail
        </Link>
      </div>
    )
  }

  if (!event) {
    return (
      <div>
        <InlineErrorState message="Event not found." />
        <Link to="/events" className="btn btn-outline">
          Back to audit trail
        </Link>
      </div>
    )
  }

  const policy = parsePolicyResult(event.policy_result)
  const payloadText = renderTechnicalBlock(event.payload_json)
  const policyText = renderTechnicalBlock(event.policy_result)
  const outputText = renderTechnicalBlock(event.result?.output_json)
  const policyRequirements = Object.entries(policy?.requirements || {})
  const policyOverrides = Object.entries(policy?.risk_overrides || {})
  const notifyTargets = policy?.notify || []

  return (
    <div>
      <PageHeaderBlock
        title="Event detail"
        description={`${event.tool}.${event.action} at ${formatDate(event.received_at)}.`}
        actions={
          <div className="btn-group">
            <Link to="/events" className="btn btn-outline">
              Back to audit trail
            </Link>
            <button className="btn btn-primary" type="button" onClick={() => void handleCopy('Event ID', event.event_id)}>
              Copy event ID
            </button>
          </div>
        }
      />

      {copyStatus ? <div className="success-msg"><span>{copyStatus}</span></div> : null}

      <div className="stats-grid">
        <StatCard label="Decision" value={event.decision} tone={statToneFor(event.decision)} hint={`${event.tool}.${event.action}`} />
        <StatCard label="Risk score" value={event.risk_score} hint="Recorded on request receipt" />
        <StatCard label="Received" value={formatDate(event.received_at)} />
        <StatCard label="Execution" value={event.result?.status || 'Not executed'} tone={statToneFor(event.result?.status)} hint={typeof event.result?.duration_ms === 'number' ? `${event.result.duration_ms}ms` : 'No execution duration recorded'} />
      </div>

      <div className="detail-panel">
        <h3>Run context</h3>
        <div className="identity-grid">
          <div className="identity-card">
            <span className="meta-label">Requested by</span>
            <div className="identity-primary">{formatRequester(event.user_id, event.user_name, event.user_email, event.agent_id)}</div>
            <div className="identity-secondary">Use these IDs to correlate the event with approvals, sessions, and downstream traces.</div>
          </div>
                <div className="identity-card">
                  <span className="meta-label">Event ID</span>
                  <div className="identity-copy-row">
                    <code className="mono" title={event.event_id}>{event.event_id}</code>
                    <CopyIconButton text={event.event_id} label="Event ID" />
                  </div>
                </div>
                <div className="identity-card">
                  <span className="meta-label">Tenant</span>
                  <div className="identity-copy-row">
                    <Link to={`/tenants/${event.tenant_id}`} className="mono" title={event.tenant_id}>{event.tenant_id}</Link>
                    <CopyIconButton text={event.tenant_id} label="Tenant ID" />
                  </div>
                </div>
                <div className="identity-card">
                  <span className="meta-label">Agent</span>
                  <div className="identity-copy-row">
                    <code className="mono" title={noneText(event.agent_id)}>{noneText(event.agent_id)}</code>
                    <CopyIconButton text={event.agent_id} label="Agent ID" disabled={!event.agent_id} />
                  </div>
                </div>
                <div className="identity-card">
                  <span className="meta-label">Session</span>
                  <div className="identity-copy-row">
                    {event.session_id ? (
                      <Link to={`/sessions/${encodeURIComponent(event.session_id)}${buildQuery({ tenant_id: event.tenant_id })}`} className="mono" title={event.session_id}>{event.session_id}</Link>
                    ) : (
                      <code className="mono">(none)</code>
                    )}
                    <CopyIconButton text={event.session_id} label="Session ID" disabled={!event.session_id} />
                  </div>
                </div>
                <div className="identity-card">
                  <span className="meta-label">Trace</span>
                  <div className="identity-copy-row">
                    <code className="mono" title={noneText(event.trace_id)}>{noneText(event.trace_id)}</code>
                    <CopyIconButton text={event.trace_id} label="Trace ID" disabled={!event.trace_id} />
                  </div>
                </div>
                <div className="identity-card">
                  <span className="meta-label">User ID</span>
                  <div className="identity-copy-row">
                    <code className="mono" title={noneText(event.user_id)}>{noneText(event.user_id)}</code>
                    <CopyIconButton text={event.user_id} label="User ID" disabled={!event.user_id} />
                  </div>
                </div>
          <div className="identity-card">
            <span className="meta-label">Resource</span>
            <div className="identity-primary">{noneText(event.resource)}</div>
          </div>
        </div>
      </div>

      <div className="session-callout session-callout-strong">
        <strong>Why OpenClause decided this way</strong>
        <p>{policy?.reason || 'A detailed policy reason was not recorded for this event.'}</p>
        {policy?.approver_group ? (
          <p className="table-subtext">Approver group: <span className="mono">{policy.approver_group}</span></p>
        ) : null}
        {notifyTargets.length > 0 ? (
          <div className="stacked-badges mt-16">
            {notifyTargets.map((target, index) => (
              <span key={`${target.kind || 'notify'}-${index}`} className="badge badge-blue">
                {target.kind === 'slack' ? `Slack ${target.channel || ''}`.trim() : target.kind === 'webhook' ? 'Webhook notify' : target.kind || 'notify'}
              </span>
            ))}
          </div>
        ) : null}
      </div>

      {policyRequirements.length > 0 || policyOverrides.length > 0 ? (
        <div className="detail-panel">
          <h3>Policy details</h3>
          {policyRequirements.length > 0 ? (
            <div className="mini-detail-list">
              {policyRequirements.map(([key, value]) => (
                <div key={key} className="mini-detail-row">
                  <span className="meta-label">{key.replace(/_/g, ' ')}</span>
                  <div>{value}</div>
                </div>
              ))}
            </div>
          ) : null}
          {policyOverrides.length > 0 ? (
            <div className="mini-detail-list mt-16">
              {policyOverrides.map(([key, value]) => (
                <div key={key} className="mini-detail-row">
                  <span className="meta-label">{key.replace(/_/g, ' ')}</span>
                  <div>{value}</div>
                </div>
              ))}
            </div>
          ) : null}
        </div>
      ) : null}

      <div className="detail-panel">
        <div className="session-subpanel-header">
          <div>
            <h3>Execution result</h3>
            <p className="table-subtext">Execution metadata is recorded only for allow or post-approval execute paths.</p>
          </div>
          {event.result?.output_json ? (
            <button className="btn btn-outline btn-sm" type="button" onClick={() => void handleCopy('Execution output', outputText)}>
              Copy output
            </button>
          ) : null}
        </div>
        {!event.result ? (
          <div className="table-subtext">This event does not have an execution result.</div>
        ) : (
          <>
            <div className="detail-row">
              <div className="detail-label">Status</div>
              <div className="detail-value">
                <span className={`badge badge-${decisionTone(event.result.status)}`}>{event.result.status}</span>
              </div>
            </div>
            <div className="detail-row">
              <div className="detail-label">Duration</div>
              <div className="detail-value">{typeof event.result.duration_ms === 'number' ? `${event.result.duration_ms}ms` : 'Not recorded'}</div>
            </div>
            <div className="detail-row">
              <div className="detail-label">Error</div>
              <div className="detail-value">{event.result.error_msg || 'None'}</div>
            </div>
            {event.result.output_json ? <pre className="code-block mt-16">{outputText}</pre> : null}
          </>
        )}
      </div>

      <div className="detail-panel">
        <div className="session-subpanel-header">
          <div>
            <h3>Request payload</h3>
            <p className="table-subtext">Canonical request payload captured when the tool call entered the gateway.</p>
          </div>
          <button className="btn btn-outline btn-sm" type="button" onClick={() => void handleCopy('Payload JSON', payloadText)}>
            Copy payload
          </button>
        </div>
        <pre className="code-block">{payloadText}</pre>
      </div>

      <div className="detail-panel">
        <div className="session-subpanel-header">
          <div>
            <h3>Policy evaluation</h3>
            <p className="table-subtext">Raw policy output is preserved for audit and troubleshooting.</p>
          </div>
          <button className="btn btn-outline btn-sm" type="button" onClick={() => void handleCopy('Policy result', policyText)}>
            Copy policy output
          </button>
        </div>
        <pre className="code-block">{policyText}</pre>
      </div>

      <div className="detail-panel">
        <h3>Hash chain</h3>
        <div className="identity-grid">
          <div className="identity-card">
            <span className="meta-label">Previous hash</span>
            <div className="identity-copy-row">
              <code className="mono">{event.prev_hash || '(genesis)'}</code>
              <button className="btn btn-outline btn-sm" type="button" onClick={() => void handleCopy('Previous hash', event.prev_hash)} disabled={!event.prev_hash}>
                Copy
              </button>
            </div>
          </div>
          <div className="identity-card">
            <span className="meta-label">Event hash</span>
            <div className="identity-copy-row">
              <code className="mono">{event.hash || 'Not recorded'}</code>
              <button className="btn btn-outline btn-sm" type="button" onClick={() => void handleCopy('Event hash', event.hash)} disabled={!event.hash}>
                Copy
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
