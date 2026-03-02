import { useState, useEffect, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { api, formatDate } from '../api'

interface Event {
  event_id: string
  tool: string
  action: string
  decision: string
  risk_score: number
  tenant_id: string
  session_id: string
  agent_id: string
  received_at: string
}

export default function Events() {
  const [events, setEvents] = useState<Event[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [filters, setFilters] = useState({
    tenant_id: '',
    tool: '',
    action: '',
    decision: '',
    session_id: '',
  })
  const [page, setPage] = useState(0)
  const limit = 25

  const fetchEvents = useCallback(async () => {
    setLoading(true)
    try {
      const params = new URLSearchParams()
      params.set('limit', String(limit))
      params.set('offset', String(page * limit))
      if (filters.tenant_id) params.set('tenant_id', filters.tenant_id)
      if (filters.tool) params.set('tool', filters.tool)
      if (filters.action) params.set('action', filters.action)
      if (filters.decision) params.set('decision', filters.decision)
      if (filters.session_id) params.set('session_id', filters.session_id)
      const data = await api.get(`/admin/events?${params}`)
      setEvents(Array.isArray(data) ? data : data?.events || [])
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }, [filters, page])

  useEffect(() => { fetchEvents() }, [fetchEvents])

  function updateFilter(key: string, value: string) {
    setFilters(f => ({ ...f, [key]: value }))
    setPage(0)
  }

  async function exportCSV() {
    try {
      const blob = await api.getBlob('/admin/events/export/csv')
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = 'events.csv'
      a.click()
      URL.revokeObjectURL(url)
    } catch (err: any) {
      setError(err.message)
    }
  }

  async function exportBundle() {
    try {
      const blob = await api.getBlob('/admin/reports/export/bundle')
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = 'audit-bundle.zip'
      a.click()
      URL.revokeObjectURL(url)
    } catch (err: any) {
      setError(err.message)
    }
  }

  return (
    <div>
      <div className="flex-between">
        <div className="page-header">
          <h2>Audit Trail</h2>
          <p>All governance events across tenants</p>
        </div>
        <div className="btn-group">
          <button className="btn btn-outline" onClick={exportCSV}>Export CSV</button>
          <button className="btn btn-outline" onClick={exportBundle}>Export Bundle</button>
        </div>
      </div>

      {error && <div className="error-msg">{error}</div>}

      <div className="filters-bar">
        <div className="form-group">
          <label>Tenant ID</label>
          <input placeholder="Filter…" value={filters.tenant_id} onChange={e => updateFilter('tenant_id', e.target.value)} />
        </div>
        <div className="form-group">
          <label>Tool</label>
          <input placeholder="Filter…" value={filters.tool} onChange={e => updateFilter('tool', e.target.value)} />
        </div>
        <div className="form-group">
          <label>Action</label>
          <input placeholder="Filter…" value={filters.action} onChange={e => updateFilter('action', e.target.value)} />
        </div>
        <div className="form-group">
          <label>Decision</label>
          <select value={filters.decision} onChange={e => updateFilter('decision', e.target.value)}>
            <option value="">All</option>
            <option value="allow">Allow</option>
            <option value="deny">Deny</option>
            <option value="approve">Approve</option>
          </select>
        </div>
        <div className="form-group">
          <label>Session ID</label>
          <input placeholder="Filter…" value={filters.session_id} onChange={e => updateFilter('session_id', e.target.value)} />
        </div>
      </div>

      <div className="table-container">
        <table>
          <thead>
            <tr>
              <th>ID</th>
              <th>Tool</th>
              <th>Action</th>
              <th>Decision</th>
              <th>Risk</th>
              <th>Tenant</th>
              <th>Session</th>
              <th>Time</th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <tr><td colSpan={8} className="loading">Loading…</td></tr>
            ) : events.length === 0 ? (
              <tr><td colSpan={8} style={{ textAlign: 'center', padding: 32, color: '#94a3b8' }}>No events found</td></tr>
            ) : (
              events.map(ev => (
                <tr key={ev.event_id}>
                  <td><Link to={`/events/${ev.event_id}`}>{ev.event_id.slice(0, 8)}…</Link></td>
                  <td>{ev.tool}</td>
                  <td>{ev.action}</td>
                  <td><span className={`badge badge-${ev.decision}`}>{ev.decision}</span></td>
                  <td>{ev.risk_score}</td>
                  <td>{ev.tenant_id?.slice(0, 8) || '—'}</td>
                  <td>
                    {ev.session_id ? (
                      <Link to={`/sessions/${ev.session_id}`}>{ev.session_id.slice(0, 8)}…</Link>
                    ) : '—'}
                  </td>
                  <td>{formatDate(ev.received_at)}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      <div className="pagination">
        <button className="btn btn-outline btn-sm" disabled={page === 0} onClick={() => setPage(p => p - 1)}>
          ← Previous
        </button>
        <span>Page {page + 1}</span>
        <button className="btn btn-outline btn-sm" disabled={events.length < limit} onClick={() => setPage(p => p + 1)}>
          Next →
        </button>
      </div>
    </div>
  )
}
