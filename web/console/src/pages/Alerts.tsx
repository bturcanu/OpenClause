import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { api, formatDate } from '../api'
import {
  ActiveFiltersBar,
  CopyIconButton,
  InlineErrorState,
  PageHeaderBlock,
  SortHeader,
  TableEmptyStateRow,
  TableFrame,
  TableSkeleton,
  applySort,
  compareDate,
  compareNumber,
  compareText,
  formatTimeWithTitle,
  shortID,
  type SortState,
} from '../ui'

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
  const [ruleSort, setRuleSort] = useState<SortState<'tenant_id' | 'name' | 'status' | 'created_at'>>({ key: null, dir: 'asc' })
  const [eventSort, setEventSort] = useState<SortState<'status' | 'severity' | 'attempt_count' | 'created_at' | 'next_attempt_at'>>({
    key: null,
    dir: 'desc',
  })

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

  const visibleRules = useMemo(() => {
    if (!ruleSort.key) return rules
    return [...rules].sort((left, right) => {
      switch (ruleSort.key) {
        case 'tenant_id':
          return applySort(compareText(left.tenant_id, right.tenant_id), ruleSort.dir)
        case 'name':
          return applySort(compareText(left.name, right.name), ruleSort.dir)
        case 'status':
          return applySort(compareText(left.enabled ? 'active' : 'disabled', right.enabled ? 'active' : 'disabled'), ruleSort.dir)
        case 'created_at':
          return applySort(compareDate(left.created_at, right.created_at), ruleSort.dir)
        default:
          return 0
      }
    })
  }, [ruleSort, rules])

  const visibleEvents = useMemo(() => {
    if (!eventSort.key) return events
    return [...events].sort((left, right) => {
      switch (eventSort.key) {
        case 'status':
          return applySort(compareText(left.status, right.status), eventSort.dir)
        case 'severity':
          return applySort(compareText(left.severity, right.severity), eventSort.dir)
        case 'attempt_count':
          return applySort(compareNumber(left.attempt_count, right.attempt_count), eventSort.dir)
        case 'created_at':
          return applySort(compareDate(left.created_at, right.created_at), eventSort.dir)
        case 'next_attempt_at':
          return applySort(compareDate(left.next_attempt_at || '', right.next_attempt_at || ''), eventSort.dir)
        default:
          return 0
      }
    })
  }, [eventSort, events])

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
                <label htmlFor="alert-rule-tenant-id">Tenant ID</label>
                <input
                  id="alert-rule-tenant-id"
                  value={form.tenantId}
                  onChange={event => setForm(current => ({ ...current, tenantId: event.target.value }))}
                  placeholder="tenant-id"
                  required
                />
              </div>
            )}
            <div className="form-group">
              <label htmlFor="alert-rule-name">Rule Name</label>
              <input
                id="alert-rule-name"
                value={form.name}
                onChange={event => setForm(current => ({ ...current, name: event.target.value }))}
                placeholder="e.g., High-risk deny spike"
                required
              />
            </div>
            <div className="form-grid alert-rule-form-grid">
              <div className="form-group">
                <label htmlFor="alert-rule-threshold">N (deny count threshold)</label>
                <input
                  id="alert-rule-threshold"
                  type="number"
                  min={1}
                  value={form.n}
                  onChange={event => setForm(current => ({ ...current, n: parseInt(event.target.value || '0', 10) || 1 }))}
                  required
                />
              </div>
              <div className="form-group">
                <label htmlFor="alert-rule-window">M (window minutes)</label>
                <input
                  id="alert-rule-window"
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
                    id="alert-rule-enabled"
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
      <ActiveFiltersBar
        resultCount={visibleRules.length}
        resultLabel={visibleRules.length === 1 ? 'rule' : 'rules'}
        chips={[]}
        note={ruleSort.key ? 'Sorted within the current page.' : 'Using backend order until you sort this page.'}
      />
      <TableFrame stickyHeader>
        <table>
          <thead>
            <tr>
              <th>
                <SortHeader label="Tenant" sortKey="tenant_id" sortState={ruleSort} onSortChange={(key, dir) => setRuleSort({ key, dir })} />
              </th>
              <th>
                <SortHeader label="Rule" sortKey="name" sortState={ruleSort} onSortChange={(key, dir) => setRuleSort({ key, dir })} />
              </th>
              <th>Type</th>
              <th>
                <SortHeader label="Status" sortKey="status" sortState={ruleSort} onSortChange={(key, dir) => setRuleSort({ key, dir })} />
              </th>
              <th className="col-time">
                <SortHeader label="Created" sortKey="created_at" sortState={ruleSort} onSortChange={(key, dir) => setRuleSort({ key, dir })} defaultDir="desc" className="col-time" />
              </th>
              <th className="table-action-col"></th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <TableSkeleton columns={6} rows={5} />
            ) : visibleRules.length === 0 ? (
              <TableEmptyStateRow
                colSpan={6}
                icon="⚠"
                title="No global alert rules yet"
                description="Create a deny_spike rule to watch for unusual policy-deny volume across one or more tenants."
              />
            ) : (
              visibleRules.map(rule => {
                const created = formatTimeWithTitle(rule.created_at)
                return (
                <tr key={rule.id}>
                  <td>
                    <div className="table-primary-cell">
                      <div className="inline-value-copy">
                        <code className="table-primary mono" title={rule.tenant_id || '—'}>{shortID(rule.tenant_id || '—', 12)}</code>
                        {rule.tenant_id ? <CopyIconButton text={rule.tenant_id} label="Tenant ID" /> : null}
                      </div>
                      <div className="table-subtext">Tenant scope</div>
                    </div>
                  </td>
                  <td>
                    <div className="table-primary-cell">
                      <div className="table-primary">{rule.name}</div>
                      <div className="table-subtext mono">{rule.id}</div>
                    </div>
                  </td>
                  <td className="mono">{rule.kind}</td>
                  <td>
                    {rule.enabled !== false
                      ? <span className="badge badge-green">Active</span>
                      : <span className="badge badge-gray">Disabled</span>}
                  </td>
                  <td className="col-time" title={created.title}>{created.label}</td>
                  <td className="table-action-cell">
                    <Link to={`/tenants/${encodeURIComponent(rule.tenant_id)}?tab=alerts`} className="btn btn-outline btn-sm">
                      Open tenant
                    </Link>
                  </td>
                </tr>
              )})
            )}
          </tbody>
        </table>
      </TableFrame>

      <div className="section-title mt-16">Alert Events</div>
      <ActiveFiltersBar
        resultCount={visibleEvents.length}
        resultLabel={visibleEvents.length === 1 ? 'event' : 'events'}
        chips={[]}
        note={eventSort.key ? 'Sorted within the current page.' : 'Using backend order until you sort this page.'}
      />
      <TableFrame stickyHeader>
        <table>
          <thead>
            <tr>
              <th>Tenant</th>
              <th>Rule</th>
              <th>
                <SortHeader label="Status" sortKey="status" sortState={eventSort} onSortChange={(key, dir) => setEventSort({ key, dir })} />
              </th>
              <th>
                <SortHeader label="Severity" sortKey="severity" sortState={eventSort} onSortChange={(key, dir) => setEventSort({ key, dir })} />
              </th>
              <th className="col-num">
                <SortHeader label="Attempts" sortKey="attempt_count" sortState={eventSort} onSortChange={(key, dir) => setEventSort({ key, dir })} defaultDir="desc" className="col-num" />
              </th>
              <th className="col-time">
                <SortHeader label="Next attempt" sortKey="next_attempt_at" sortState={eventSort} onSortChange={(key, dir) => setEventSort({ key, dir })} defaultDir="asc" className="col-time" />
              </th>
              <th>Message</th>
              <th className="col-time">
                <SortHeader label="Fired" sortKey="created_at" sortState={eventSort} onSortChange={(key, dir) => setEventSort({ key, dir })} defaultDir="desc" className="col-time" />
              </th>
              <th className="table-action-col"></th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <TableSkeleton columns={9} rows={6} />
            ) : visibleEvents.length === 0 ? (
              <TableEmptyStateRow
                colSpan={9}
                icon="⌁"
                title="No alert events yet"
                description="Triggered deliveries and retry attempts will appear here as soon as a rule fires."
              />
            ) : (
              visibleEvents.map(event => {
                const created = formatTimeWithTitle(event.created_at)
                const nextAttempt = formatTimeWithTitle(event.next_attempt_at || null)
                return (
                <tr key={event.id}>
                  <td>
                    <div className="table-primary-cell">
                      <div className="inline-value-copy">
                        <code className="table-primary mono" title={event.tenant_id || '—'}>{shortID(event.tenant_id || '—', 12)}</code>
                        {event.tenant_id ? <CopyIconButton text={event.tenant_id} label="Tenant ID" /> : null}
                      </div>
                      <div className="table-subtext">Tenant</div>
                    </div>
                  </td>
                  <td>
                    <div className="table-primary-cell">
                      <div className="inline-value-copy">
                        <code className="table-primary mono" title={event.rule_id || '—'}>{shortID(event.rule_id || '—', 12)}</code>
                        {event.rule_id ? <CopyIconButton text={event.rule_id} label="Rule ID" /> : null}
                      </div>
                      <div className="table-subtext">Rule ID</div>
                    </div>
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
                  <td className="col-num tabular">{event.attempt_count ?? 0}</td>
                  <td className="col-time" title={nextAttempt.title}>
                    {event.next_attempt_at ? nextAttempt.label : <span className="table-subtext">—</span>}
                  </td>
                  <td>
                    <div className="table-primary-cell">
                      <div>{event.message}</div>
                    {event.last_error ? <div className="table-subtext">Last delivery error: {event.last_error}</div> : null}
                      {event.delivered_at ? <div className="table-subtext">Delivered {formatDate(event.delivered_at)}</div> : null}
                    </div>
                  </td>
                  <td className="col-time" title={created.title}>{created.label}</td>
                  <td className="table-action-cell">
                    <Link to={`/tenants/${encodeURIComponent(event.tenant_id)}?tab=alerts`} className="btn btn-outline btn-sm">
                      Open tenant
                    </Link>
                  </td>
                </tr>
              )})
            )}
          </tbody>
        </table>
      </TableFrame>
    </div>
  )
}
