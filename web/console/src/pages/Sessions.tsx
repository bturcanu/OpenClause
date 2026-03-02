import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { api, formatDate } from '../api'

interface Session {
  id: string
  tenant_id: string
  agent_id: string
  started_at: string
  ended_at: string | null
  event_count: number
}

export default function Sessions() {
  const [sessions, setSessions] = useState<Session[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    api.get('/admin/sessions')
      .then(data => setSessions(Array.isArray(data) ? data : data?.sessions || []))
      .catch(err => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  return (
    <div>
      <div className="page-header">
        <h2>Sessions</h2>
        <p>Agent interaction sessions</p>
      </div>

      {error && <div className="error-msg">{error}</div>}

      <div className="table-container">
        <table>
          <thead>
            <tr>
              <th>Session ID</th>
              <th>Tenant</th>
              <th>Agent</th>
              <th>Events</th>
              <th>Started</th>
              <th>Ended</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <tr><td colSpan={7} className="loading">Loading…</td></tr>
            ) : sessions.length === 0 ? (
              <tr><td colSpan={7} style={{ textAlign: 'center', padding: 32, color: '#94a3b8' }}>No sessions found</td></tr>
            ) : (
              sessions.map(s => (
                <tr key={s.id}>
                  <td><Link to={`/sessions/${s.id}`}>{s.id.slice(0, 12)}…</Link></td>
                  <td>{s.tenant_id?.slice(0, 8) || '—'}</td>
                  <td>{s.agent_id?.slice(0, 8) || '—'}</td>
                  <td>{s.event_count ?? '—'}</td>
                  <td>{formatDate(s.started_at)}</td>
                  <td>{s.ended_at ? formatDate(s.ended_at) : <span className="badge badge-green">Active</span>}</td>
                  <td><Link to={`/sessions/${s.id}`} className="btn btn-outline btn-sm">Timeline</Link></td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}
