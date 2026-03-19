import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { api, formatDate } from '../api'

interface OverviewData {
  total_events: number
  allow_count: number
  deny_count: number
  approve_count: number
  pending_approvals: number
  active_tenants: number
}

interface TimeseriesBucket {
  bucket: string
  count: number
}

interface Event {
  event_id: string
  tool: string
  action: string
  decision: string
  risk_score: number
  received_at: string
  tenant_id: string
}

export default function Overview() {
  const [overview, setOverview] = useState<OverviewData | null>(null)
  const [timeseries, setTimeseries] = useState<TimeseriesBucket[]>([])
  const [events, setEvents] = useState<Event[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    Promise.all([
      api.get('/admin/analytics/overview').catch(() => null),
      api.get('/admin/analytics/timeseries').catch(() => []),
      api.get('/admin/events?limit=10').catch(() => []),
    ]).then(([ov, ts, ev]) => {
      if (ov) setOverview(ov)
      else setOverview({ total_events: 0, allow_count: 0, deny_count: 0, approve_count: 0, pending_approvals: 0, active_tenants: 0 })
      setTimeseries(Array.isArray(ts) ? ts : ts?.buckets || [])
      setEvents(Array.isArray(ev) ? ev : ev?.events || [])
    }).catch(err => setError(err.message))
    .finally(() => setLoading(false))
  }, [])

  if (error) return <div className="error-msg">{error}</div>
  if (loading || !overview) return <div className="loading">Loading dashboard…</div>

  const maxCount = Math.max(...timeseries.map(b => b.count), 1)

  return (
    <div>
      <div className="page-header">
        <h2>Overview</h2>
        <p>System-wide analytics and recent activity</p>
      </div>

      <div className="card-grid">
        <div className="card">
          <div className="card-label">Total Events</div>
          <div className="card-value">{overview.total_events.toLocaleString()}</div>
        </div>
        <div className="card">
          <div className="card-label">Allowed</div>
          <div className="card-value green">{overview.allow_count.toLocaleString()}</div>
        </div>
        <div className="card">
          <div className="card-label">Denied</div>
          <div className="card-value red">{overview.deny_count.toLocaleString()}</div>
        </div>
        <div className="card">
          <div className="card-label">Approval Required</div>
          <div className="card-value yellow">{overview.approve_count.toLocaleString()}</div>
        </div>
        <div className="card">
          <div className="card-label">Pending Approvals</div>
          <div className="card-value yellow">{overview.pending_approvals.toLocaleString()}</div>
        </div>
        <div className="card">
          <div className="card-label">Active Tenants</div>
          <div className="card-value blue">{overview.active_tenants.toLocaleString()}</div>
        </div>
      </div>

      {timeseries.length > 0 && (
        <div className="detail-panel">
          <h3>Event Volume (Time Series)</h3>
          <div style={{ display: 'flex', alignItems: 'flex-end', gap: 2, height: 120, padding: '12px 0' }}>
            {timeseries.map((b, i) => (
              <div
                key={i}
                title={`${b.bucket}: ${b.count}`}
                style={{
                  flex: 1,
                  background: '#3b82f6',
                  borderRadius: '3px 3px 0 0',
                  height: `${(b.count / maxCount) * 100}%`,
                  minHeight: 2,
                  transition: 'height 0.3s',
                }}
              />
            ))}
          </div>
          <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, color: '#94a3b8' }}>
            {timeseries.length > 0 && <span>{formatDate(timeseries[0].bucket, 'date')}</span>}
            {timeseries.length > 1 && <span>{formatDate(timeseries[timeseries.length - 1].bucket, 'date')}</span>}
          </div>
        </div>
      )}

      <div className="section-title">Recent Events</div>
      <div className="table-container">
        <table>
          <thead>
            <tr>
              <th>Tool</th>
              <th>Action</th>
              <th>Decision</th>
              <th>Risk</th>
              <th>Time</th>
            </tr>
          </thead>
          <tbody>
            {events.length === 0 ? (
              <tr><td colSpan={5} style={{ textAlign: 'center', color: '#94a3b8' }}>No events yet</td></tr>
            ) : (
              events.map(ev => (
                <tr key={ev.event_id}>
                  <td><Link to={`/events/${ev.event_id}`}>{ev.tool}</Link></td>
                  <td>{ev.action}</td>
                  <td><span className={`badge badge-${ev.decision}`}>{ev.decision}</span></td>
                  <td>{ev.risk_score}</td>
                  <td>{formatDate(ev.received_at)}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}
