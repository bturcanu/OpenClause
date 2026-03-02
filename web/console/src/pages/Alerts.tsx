import { useState, useEffect, FormEvent } from 'react'
import { api } from '../api'

interface AlertRule {
  id: string
  name: string
  condition: string
  channel: string
  enabled: boolean
  created_at: string
}

interface AlertEvent {
  id: string
  rule_id: string
  rule_name: string
  message: string
  severity: string
  fired_at: string
}

export default function Alerts() {
  const [rules, setRules] = useState<AlertRule[]>([])
  const [events, setEvents] = useState<AlertEvent[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [showCreate, setShowCreate] = useState(false)
  const [form, setForm] = useState({ name: '', condition: '', channel: 'webhook' })
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
      await api.post('/admin/alerts/rules', form)
      setForm({ name: '', condition: '', channel: 'webhook' })
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
            <div className="form-group">
              <label>Condition (CEL expression)</label>
              <textarea
                value={form.condition}
                onChange={e => setForm(f => ({ ...f, condition: e.target.value }))}
                placeholder='e.g., event.decision == "deny" && event.risk_score > 80'
                required
              />
            </div>
            <div className="form-group">
              <label>Channel</label>
              <select value={form.channel} onChange={e => setForm(f => ({ ...f, channel: e.target.value }))}>
                <option value="webhook">Webhook</option>
                <option value="email">Email</option>
                <option value="slack">Slack</option>
              </select>
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
              <th>Condition</th>
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
                    {r.condition}
                  </td>
                  <td><span className="badge badge-gray">{r.channel}</span></td>
                  <td>
                    {r.enabled !== false
                      ? <span className="badge badge-green">Active</span>
                      : <span className="badge badge-gray">Disabled</span>}
                  </td>
                  <td>{new Date(r.created_at).toLocaleDateString()}</td>
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
                  <td style={{ fontWeight: 600 }}>{ev.rule_name || ev.rule_id?.slice(0, 8)}</td>
                  <td>
                    <span className={`badge ${ev.severity === 'critical' ? 'badge-red' : ev.severity === 'warning' ? 'badge-yellow' : 'badge-gray'}`}>
                      {ev.severity}
                    </span>
                  </td>
                  <td>{ev.message}</td>
                  <td>{new Date(ev.fired_at).toLocaleString()}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}
