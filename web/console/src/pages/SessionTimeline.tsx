import { useState, useEffect } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api } from '../api'

interface TimelineEvent {
  id: string
  tool: string
  action: string
  decision: string
  risk_score: number
  created_at: string
}

export default function SessionTimeline() {
  const { id } = useParams<{ id: string }>()
  const [events, setEvents] = useState<TimelineEvent[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    api.get(`/admin/sessions/${id}/timeline`)
      .then(data => setEvents(Array.isArray(data) ? data : data?.events || []))
      .catch(err => setError(err.message))
      .finally(() => setLoading(false))
  }, [id])

  if (loading) return <div className="loading">Loading timeline…</div>
  if (error) return <div className="error-msg">{error}</div>

  return (
    <div>
      <div className="flex-between">
        <div className="page-header">
          <h2>Session Timeline</h2>
          <p>{id}</p>
        </div>
        <Link to="/sessions" className="btn btn-outline">← Back to Sessions</Link>
      </div>

      {events.length === 0 ? (
        <div className="empty-state">
          <div className="empty-icon">↻</div>
          <p>No events in this session</p>
        </div>
      ) : (
        <div className="timeline">
          {events.map(ev => (
            <div key={ev.id} className={`timeline-item ${ev.decision}`}>
              <div className="tl-time">{new Date(ev.created_at).toLocaleString()}</div>
              <div className="tl-content">
                <div className="tl-title">
                  <Link to={`/events/${ev.id}`}>{ev.tool} → {ev.action}</Link>
                </div>
                <div className="tl-meta">
                  <span className={`badge badge-${ev.decision}`}>{ev.decision}</span>
                  {' · '}Risk: {ev.risk_score}
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
