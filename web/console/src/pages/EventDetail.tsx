import { useState, useEffect } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, formatDate } from '../api'

interface EventData {
  event_id: string
  tenant_id: string
  agent_id: string
  session_id: string
  tool: string
  action: string
  payload_json: any
  risk_score: number
  decision: string
  policy_result: any
  prev_hash: string
  hash: string
  result: any
  received_at: string
}

export default function EventDetail() {
  const { eventId } = useParams<{ eventId: string }>()
  const [event, setEvent] = useState<EventData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    api.get(`/admin/events/${eventId}`)
      .then(data => setEvent(data))
      .catch(err => setError(err.message))
      .finally(() => setLoading(false))
  }, [eventId])

  if (loading) return <div className="loading">Loading event…</div>
  if (error) return <div className="error-msg">{error}</div>
  if (!event) return <div className="error-msg">Event not found</div>

  return (
    <div>
      <div className="page-header">
        <div className="flex-between">
          <div>
            <h2>Event Detail</h2>
            <p>{event.event_id}</p>
          </div>
          <Link to="/events" className="btn btn-outline">← Back to Events</Link>
        </div>
      </div>

      <div className="detail-panel">
        <h3>Core Information</h3>
        <div className="detail-row">
          <div className="detail-label">Tool</div>
          <div className="detail-value">{event.tool}</div>
        </div>
        <div className="detail-row">
          <div className="detail-label">Action</div>
          <div className="detail-value">{event.action}</div>
        </div>
        <div className="detail-row">
          <div className="detail-label">Decision</div>
          <div className="detail-value">
            <span className={`badge badge-${event.decision}`}>{event.decision}</span>
          </div>
        </div>
        <div className="detail-row">
          <div className="detail-label">Risk Score</div>
          <div className="detail-value">{event.risk_score}</div>
        </div>
        <div className="detail-row">
          <div className="detail-label">Tenant ID</div>
          <div className="detail-value">
            <Link to={`/tenants/${event.tenant_id}`}>{event.tenant_id}</Link>
          </div>
        </div>
        <div className="detail-row">
          <div className="detail-label">Agent ID</div>
          <div className="detail-value">{event.agent_id || '—'}</div>
        </div>
        <div className="detail-row">
          <div className="detail-label">Session ID</div>
          <div className="detail-value">
            {event.session_id ? (
              <Link to={`/sessions/${event.session_id}`}>{event.session_id}</Link>
            ) : '—'}
          </div>
        </div>
        <div className="detail-row">
          <div className="detail-label">Received At</div>
          <div className="detail-value">{formatDate(event.received_at)}</div>
        </div>
      </div>

      {event.payload_json && (
        <div className="detail-panel">
          <h3>Request Payload</h3>
          <pre style={{ background: '#f1f5f9', padding: 16, borderRadius: 6, fontSize: 12, overflow: 'auto' }}>
            {typeof event.payload_json === 'string' ? event.payload_json : JSON.stringify(event.payload_json, null, 2)}
          </pre>
        </div>
      )}

      {event.policy_result && (
        <div className="detail-panel">
          <h3>Policy Result</h3>
          <pre style={{ background: '#f1f5f9', padding: 16, borderRadius: 6, fontSize: 12, overflow: 'auto' }}>
            {typeof event.policy_result === 'string' ? event.policy_result : JSON.stringify(event.policy_result, null, 2)}
          </pre>
        </div>
      )}

      {event.result && (
        <div className="detail-panel">
          <h3>Execution Result</h3>
          <pre style={{ background: '#f1f5f9', padding: 16, borderRadius: 6, fontSize: 12, overflow: 'auto' }}>
            {JSON.stringify(event.result, null, 2)}
          </pre>
        </div>
      )}

      <div className="detail-panel">
        <h3>Hash Chain</h3>
        <div className="detail-row">
          <div className="detail-label">Previous Hash</div>
          <div className="detail-value" style={{ fontFamily: 'monospace', fontSize: 12 }}>
            {event.prev_hash || '(genesis)'}
          </div>
        </div>
        <div className="detail-row">
          <div className="detail-label">Event Hash</div>
          <div className="detail-value" style={{ fontFamily: 'monospace', fontSize: 12 }}>
            {event.hash || '—'}
          </div>
        </div>
      </div>
    </div>
  )
}
