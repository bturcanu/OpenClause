import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { useParams } from 'react-router-dom'
import { api, formatDate } from '../api'
import { InlineErrorState, PageHeaderBlock, decisionTone, formatRequester, buildQuery, noneText } from '../ui'

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
  payload_json: any
  risk_score: number
  decision: string
  policy_result: any
  prev_hash: string
  hash: string
  result: {
    status: string
    error_msg?: string
    duration_ms?: number
    output_json?: any
  } | null
  received_at: string
}

function renderTechnicalBlock(value: unknown) {
  if (!value) return 'Not recorded'
  return typeof value === 'string' ? value : JSON.stringify(value, null, 2)
}

export default function EventDetail() {
  const { eventId } = useParams<{ eventId: string }>()
  const [event, setEvent] = useState<EventData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!eventId) return
    api.get(`/admin/events/${eventId}`)
      .then(data => setEvent(data))
      .catch(err => setError(err.message))
      .finally(() => setLoading(false))
  }, [eventId])

  if (loading) {
    return <div className="detail-panel"><div className="skeleton-line skeleton-line-lg" /><div className="skeleton-line" /><div className="skeleton-line" /></div>
  }

  if (error) {
    return <InlineErrorState message={error} />
  }

  if (!event) {
    return <InlineErrorState message="Event not found" />
  }

  return (
    <div>
      <PageHeaderBlock
        title="Event detail"
        description={`${event.tool}.${event.action} at ${formatDate(event.received_at)}`}
        actions={
          <Link to="/events" className="btn btn-outline">
            Back to audit trail
          </Link>
        }
      />

      <div className="detail-panel">
        <h3>Operator summary</h3>
        <div className="detail-row">
          <div className="detail-label">Requested by</div>
          <div className="detail-value">{formatRequester(event.user_id, event.user_name, event.user_email, event.agent_id)}</div>
        </div>
        <div className="detail-row">
          <div className="detail-label">Decision</div>
          <div className="detail-value">
            <span className={`badge badge-${decisionTone(event.decision)}`}>{event.decision}</span>
          </div>
        </div>
        <div className="detail-row">
          <div className="detail-label">Risk score</div>
          <div className="detail-value">{event.risk_score}</div>
        </div>
        <div className="detail-row">
          <div className="detail-label">Tenant</div>
          <div className="detail-value">
            <Link to={`/tenants/${event.tenant_id}`}>{event.tenant_id}</Link>
          </div>
        </div>
        <div className="detail-row">
          <div className="detail-label">Session</div>
          <div className="detail-value">
            {event.session_id ? (
              <Link to={`/sessions/${encodeURIComponent(event.session_id)}${buildQuery({ tenant_id: event.tenant_id })}`}>{event.session_id}</Link>
            ) : (
              '(none)'
            )}
          </div>
        </div>
        <div className="detail-row">
          <div className="detail-label">Trace</div>
          <div className="detail-value">{noneText(event.trace_id)}</div>
        </div>
        <div className="detail-row">
          <div className="detail-label">Resource</div>
          <div className="detail-value">{noneText(event.resource)}</div>
        </div>
      </div>

      {event.result ? (
        <div className="detail-panel">
          <h3>Execution result</h3>
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
        </div>
      ) : null}

      <div className="detail-panel">
        <h3>Technical payload</h3>
        <pre className="code-block">{renderTechnicalBlock(event.payload_json)}</pre>
      </div>

      <div className="detail-panel">
        <h3>Policy evaluation</h3>
        <pre className="code-block">{renderTechnicalBlock(event.policy_result)}</pre>
      </div>

      <div className="detail-panel">
        <h3>Hash chain</h3>
        <div className="detail-row">
          <div className="detail-label">Previous hash</div>
          <div className="detail-value mono">{event.prev_hash || '(genesis)'}</div>
        </div>
        <div className="detail-row">
          <div className="detail-label">Event hash</div>
          <div className="detail-value mono">{event.hash || 'Not recorded'}</div>
        </div>
      </div>
    </div>
  )
}
