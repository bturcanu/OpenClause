import { useState, useEffect, FormEvent } from 'react'
import { api, formatDate } from '../api'

interface AlertRule {
  id: string
  name: string
  rule_type: string
  notify_kind: string
  enabled: boolean
  created_at: string
}

interface AlertEvent {
  id: string
  rule_id: string
  message: string
  severity: string
  created_at: string
}

export default function Alerts() {
  const [rules, setRules] = useState<AlertRule[]>([])
  const [events, setEvents] = useState<AlertEvent[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [showCreate, setShowCreate] = useState(false)
  const [form, setForm] = useState({ name: '', n: 3, mMinutes: 5, enabled: true })
  const [creating, setCreating] = useState(false)

  async function fetchAll() {
    try {
      const [r, e] = await Promise.all([
        api.get('/admin/alerts/rules').catch(() => []),
        api.get('/admin/alerts/events').catch(() => []),
      ])
      setRules(Array.isArray(r) ? r : r?.rules || [])
      setEvents(Array.isArray(e) ? e : e?.events || [])
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { fetchAll() }, [])

  async function handleCreate(e: FormEvent) {
    e.preventDefault()
    setCreating(true)
    setError('')
    try {
      await api.post('/admin/alerts/rules', {
        name: form.name,
        kind: 'deny_spike',
        enabled: form.enabled,
        config_json: { n: form.n, m_minutes: form.mMinutes },
      })
      setForm({ name: '', n: 3, mMinutes: 5, enabled: true })
      setShowCreate(false)
      await fetchAll()
    } catch (err: any) {
      setError(err.message)
    } finally {
      setCreating(false)
    }
  }

  return (
    <div>
      <div className="flex-between">
        <div className="page-header">
          <h2>Alerts</h2>
          <p>Configure alert rules and view triggered events</p>
        </div>
        <button className="btn btn-primary" onClick={() => setShowCreate(f => !f)}>
          {showCreate ? 'Cancel' : '+ New Rule'}
        </button>
      </div>

      {error && <div className="error-msg">{error}</div>}

      {showCreate && (
        <div className="form-card">
          <h3>Create Alert Rule</h3>
          <form onSubmit={handleCreate}>
            <div className="form-group">
              <label>Rule Name</label>
              <input
                value={form.name}
                onChange={e => setForm(f => ({ ...f, name: e.target.value }))}
                placeholder="e.g., High-risk deny spike"
                required
              />
            </div>
            <div className="form-inline" style={{ gap: 16, flexWrap: 'wrap' }}>
              <div className="form-group" style={{ minWidth: 180 }}>
                <label>N (deny count threshold)</label>
                <input
                  type="number"
                  min={1}
                  value={form.n}
                  onChange={e => setForm(f => ({ ...f, n: parseInt(e.target.value || '0', 10) || 1 }))}
                  required
                />
              </div>
              <div className="form-group" style={{ minWidth: 220 }}>
                <label>M (window minutes)</label>
                <input
                  type="number"
                  min={1}
                  value={form.mMinutes}
                  onChange={e => setForm(f => ({ ...f, mMinutes: parseInt(e.target.value || '0', 10) || 1 }))}
                  required
                />
              </div>
              <div className="form-group">
                <label>Enabled</label>
                <div style={{ marginTop: 6 }}>
                  <input
                    type="checkbox"
                    checked={form.enabled}
                    onChange={e => setForm(f => ({ ...f, enabled: e.target.checked }))}
                  />
                </div>
              </div>
            </div>
            <button className="btn btn-primary" disabled={creating}>
              {creating ? 'Creating…' : 'Create Rule'}
            </button>
          </form>
        </div>
      )}

      <div className="section-title">Alert Rules</div>
      <div className="table-container">
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Type</th>
              <th>Channel</th>
              <th>Status</th>
              <th>Created</th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <tr><td colSpan={5} className="loading">Loading…</td></tr>
            ) : rules.length === 0 ? (
              <tr><td colSpan={5} style={{ textAlign: 'center', padding: 24, color: '#94a3b8' }}>No alert rules configured</td></tr>
            ) : (
              rules.map(r => (
                <tr key={r.id}>
                  <td style={{ fontWeight: 600 }}>{r.name}</td>
                  <td style={{ fontFamily: 'monospace', fontSize: 12, maxWidth: 300, overflow: 'hidden', textOverflow: 'ellipsis' }}>
                    {r.rule_type}
                  </td>
                  <td><span className="badge badge-gray">{r.notify_kind}</span></td>
                  <td>
                    {r.enabled !== false
                      ? <span className="badge badge-green">Active</span>
                      : <span className="badge badge-gray">Disabled</span>}
                  </td>
                  <td>{formatDate(r.created_at, 'date')}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      <div className="section-title mt-16">Alert Events</div>
      <div className="table-container">
        <table>
          <thead>
            <tr>
              <th>Rule</th>
              <th>Severity</th>
              <th>Message</th>
              <th>Fired At</th>
            </tr>
          </thead>
          <tbody>
            {events.length === 0 ? (
              <tr><td colSpan={4} style={{ textAlign: 'center', padding: 24, color: '#94a3b8' }}>No alert events</td></tr>
            ) : (
              events.map(ev => (
                <tr key={ev.id}>
                  <td style={{ fontWeight: 600 }}>{ev.rule_id?.slice(0, 8)}</td>
                  <td>
                    <span className={`badge ${ev.severity === 'critical' ? 'badge-red' : ev.severity === 'warning' ? 'badge-yellow' : 'badge-gray'}`}>
                      {ev.severity}
                    </span>
                  </td>
                  <td>{ev.message}</td>
                  <td>{formatDate(ev.created_at)}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}
