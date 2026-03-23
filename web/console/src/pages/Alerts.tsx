import { useEffect, useState, type FormEvent } from 'react'
import { api, formatDate } from '../api'
import { EmptyState, InlineErrorState, PageHeaderBlock, TableSkeleton, shortID } from '../ui'

interface AlertRule {
  id: string
  tenant_id: string
  name: string
  kind: string
  enabled: boolean
  created_at: string
}

interface AlertEvent {
  id: string
  rule_id: string
  tenant_id: string
  severity: string
  message: string
  status: string
  created_at: string
  delivered_at?: string | null
  attempt_count?: number
  last_error?: string
  next_attempt_at?: string | null
}

export default function Alerts() {
  const [rules, setRules] = useState<AlertRule[]>([])
  const [events, setEvents] = useState<AlertEvent[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [isPlatformAdmin, setIsPlatformAdmin] = useState(false)
  const [scopedTenantID, setScopedTenantID] = useState('')

  const [showCreate, setShowCreate] = useState(false)
  const [form, setForm] = useState({ tenantId: '', name: '', n: 3, mMinutes: 5, enabled: true })
  const [creating, setCreating] = useState(false)

  useEffect(() => {
    const token = localStorage.getItem('oc_token')
    if (!token) return
    try {
      const payload = token.split('.')[1]
      const base64 = payload.replace(/-/g, '+').replace(/_/g, '/')
      const padded = base64.padEnd(base64.length + (4 - (base64.length % 4)) % 4, '=')
      const decoded = JSON.parse(atob(padded))
      const roles: string[] = Array.isArray(decoded?.roles) ? decoded.roles : []
      const tenantID = typeof decoded?.tenant === 'string' ? decoded.tenant : ''
      setIsPlatformAdmin(roles.includes('platform_admin'))
      setScopedTenantID(tenantID)
      if (tenantID) {
        setForm(current => ({ ...current, tenantId: tenantID }))
      }
    } catch {
      setIsPlatformAdmin(false)
      setScopedTenantID('')
    }
  }, [])

  async function fetchAll() {
    setLoading(true)
    setError('')

    const [rulesResult, eventsResult] = await Promise.allSettled([
      api.get('/admin/alerts/rules'),
      api.get('/admin/alerts/events'),
    ])

    const failures: string[] = []

    if (rulesResult.status === 'fulfilled') {
      setRules(Array.isArray(rulesResult.value) ? rulesResult.value as AlertRule[] : ((rulesResult.value as { rules?: AlertRule[] })?.rules || []))
    } else {
      setRules([])
      failures.push('alert rules')
    }

    if (eventsResult.status === 'fulfilled') {
      setEvents(Array.isArray(eventsResult.value) ? eventsResult.value as AlertEvent[] : ((eventsResult.value as { events?: AlertEvent[] })?.events || []))
    } else {
      setEvents([])
      failures.push('alert events')
    }

    if (failures.length > 0) {
      setError(`Some alert data could not be loaded: ${failures.join(', ')}.`)
    }

    setLoading(false)
  }

  useEffect(() => {
    void fetchAll()
  }, [])

  async function handleCreate(event: FormEvent) {
    event.preventDefault()
    setCreating(true)
    setError('')
    try {
      const tenantID = form.tenantId.trim() || scopedTenantID.trim()
      if (!tenantID) {
        throw new Error('Tenant ID is required to create an alert rule.')
      }
      await api.post('/admin/alerts/rules', {
        tenant_id: tenantID,
        name: form.name,
        kind: 'deny_spike',
        enabled: form.enabled,
        config_json: { n: form.n, m_minutes: form.mMinutes },
      })
      setForm({ tenantId: tenantID, name: '', n: 3, mMinutes: 5, enabled: true })
      setShowCreate(false)
      await fetchAll()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create alert rule')
    } finally {
      setCreating(false)
    }
  }

  return (
    <div>
      <PageHeaderBlock
        title="Alerts"
        description="Monitor tenant alert rules, delivery status, and the retry state of triggered alert events."
        actions={(
          <div className="btn-group">
            <button className="btn btn-outline" type="button" onClick={() => void fetchAll()} disabled={loading}>
              Refresh
            </button>
            <button className="btn btn-primary" type="button" onClick={() => setShowCreate(current => !current)}>
              {showCreate ? 'Cancel' : '+ New Rule'}
            </button>
          </div>
        )}
      />

      {error ? <InlineErrorState message={error} onRetry={() => void fetchAll()} /> : null}

      {showCreate ? (
        <div className="form-card">
          <h3>Create Alert Rule</h3>
          <p className="form-helper-text">Use global alerts to watch retry state and tenant-level deny spikes across the whole console.</p>
          <form onSubmit={handleCreate}>
            {(isPlatformAdmin || !scopedTenantID) && (
              <div className="form-group">
                <label>Tenant ID</label>
                <input
                  value={form.tenantId}
                  onChange={event => setForm(current => ({ ...current, tenantId: event.target.value }))}
                  placeholder="tenant-id"
                  required
                />
              </div>
            )}
            <div className="form-group">
              <label>Rule Name</label>
              <input
                value={form.name}
                onChange={event => setForm(current => ({ ...current, name: event.target.value }))}
                placeholder="e.g., High-risk deny spike"
                required
              />
            </div>
            <div className="form-grid alert-rule-form-grid">
              <div className="form-group">
                <label>N (deny count threshold)</label>
                <input
                  type="number"
                  min={1}
                  value={form.n}
                  onChange={event => setForm(current => ({ ...current, n: parseInt(event.target.value || '0', 10) || 1 }))}
                  required
                />
              </div>
              <div className="form-group">
                <label>M (window minutes)</label>
                <input
                  type="number"
                  min={1}
                  value={form.mMinutes}
                  onChange={event => setForm(current => ({ ...current, mMinutes: parseInt(event.target.value || '0', 10) || 1 }))}
                  required
                />
              </div>
              <div className="form-group alert-activation-field">
                <label>Activation</label>
                <label className="toggle-field toggle-field-boxed">
                  <input
                    type="checkbox"
                    checked={form.enabled}
                    onChange={event => setForm(current => ({ ...current, enabled: event.target.checked }))}
                  />
                  <span>{form.enabled ? 'Enabled immediately' : 'Save as disabled'}</span>
                </label>
              </div>
              <div className="form-actions-row form-actions-row-end">
                <button className="btn btn-primary" disabled={creating}>
                  {creating ? 'Creating…' : 'Create Rule'}
                </button>
              </div>
            </div>
          </form>
        </div>
      ) : null}

      <div className="section-title">Alert Rules</div>
      <div className="table-container table-sticky">
        <table>
          <thead>
            <tr>
              <th>Tenant</th>
              <th>Name</th>
              <th>Type</th>
              <th>Status</th>
              <th>Created</th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <TableSkeleton columns={5} rows={5} />
            ) : rules.length === 0 ? (
              <tr>
                <td colSpan={5} style={{ padding: 0 }}>
                  <EmptyState
                    icon="⚠"
                    title="No global alert rules yet"
                    description="Create a deny_spike rule to watch for unusual policy-deny volume across one or more tenants."
                  />
                </td>
              </tr>
            ) : (
              rules.map(rule => (
                <tr key={rule.id}>
                  <td>
                    <div className="table-primary mono">{shortID(rule.tenant_id || '—', 12)}</div>
                    <div className="table-subtext">Tenant scope</div>
                  </td>
                  <td>
                    <div className="table-primary">{rule.name}</div>
                    <div className="table-subtext mono">{rule.kind}</div>
                  </td>
                  <td className="mono">{rule.kind}</td>
                  <td>
                    {rule.enabled !== false
                      ? <span className="badge badge-green">Active</span>
                      : <span className="badge badge-gray">Disabled</span>}
                  </td>
                  <td>{formatDate(rule.created_at, 'date')}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      <div className="section-title mt-16">Alert Events</div>
      <div className="table-container table-sticky">
        <table>
          <thead>
            <tr>
              <th>Tenant</th>
              <th>Rule</th>
              <th>Status</th>
              <th>Severity</th>
              <th>Attempts</th>
              <th>Message</th>
              <th>Fired At</th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <TableSkeleton columns={7} rows={6} />
            ) : events.length === 0 ? (
              <tr>
                <td colSpan={7} style={{ padding: 0 }}>
                  <EmptyState
                    icon="⌁"
                    title="No alert events yet"
                    description="Triggered deliveries and retry attempts will appear here as soon as a rule fires."
                  />
                </td>
              </tr>
            ) : (
              events.map(event => (
                <tr key={event.id}>
                  <td>
                    <div className="table-primary mono">{shortID(event.tenant_id || '—', 12)}</div>
                    <div className="table-subtext">Tenant</div>
                  </td>
                  <td>
                    <div className="table-primary mono">{shortID(event.rule_id || '—', 12)}</div>
                    <div className="table-subtext">Rule ID</div>
                  </td>
                  <td>
                    <span className={`badge ${
                      event.status === 'sent'
                        ? 'badge-green'
                        : event.status === 'pending'
                          ? 'badge-yellow'
                          : 'badge-gray'
                    }`}>
                      {event.status || 'unknown'}
                    </span>
                    {event.delivered_at ? <div className="table-subtext">Delivered {formatDate(event.delivered_at)}</div> : null}
                    {!event.delivered_at && event.next_attempt_at ? (
                      <div className="table-subtext">Retry scheduled {formatDate(event.next_attempt_at)}</div>
                    ) : null}
                  </td>
                  <td>
                    <span className={`badge ${
                      event.severity === 'critical'
                        ? 'badge-red'
                        : event.severity === 'warning'
                          ? 'badge-yellow'
                          : 'badge-gray'
                    }`}>
                      {event.severity || 'info'}
                    </span>
                  </td>
                  <td>{event.attempt_count ?? 0}</td>
                  <td>
                    <div>{event.message}</div>
                    {event.last_error ? <div className="table-subtext">Last delivery error: {event.last_error}</div> : null}
                  </td>
                  <td>{formatDate(event.created_at)}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}
