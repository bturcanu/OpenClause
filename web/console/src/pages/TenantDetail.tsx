import { useState, useEffect, FormEvent, useRef } from 'react'
import { useParams, Link, useSearchParams } from 'react-router-dom'
import { api, formatDate } from '../api'

interface Tenant {
  id: string
  name: string
  status: string
  config: any
  created_at: string
}

interface Agent {
  id: string
  name: string
  tenant_id: string
  created_at: string
}

interface ApiKey {
  id: string
  key_prefix: string
  name: string
  status: string
  created_at: string
}

interface Approver {
  id: string
  email: string
  name: string
  slack_user_id?: string | null
}

interface TenantNotificationConfig {
  approver_group?: string
  notify?: Array<{
    kind: string
    url?: string
    secret_ref?: string
    channel?: string
  }>
}

interface AlertRuleConfigJSON {
  n: number
  m_minutes: number
}

interface AlertRule {
  id: string
  tenant_id: string
  name: string
  kind: string
  enabled: boolean
  config_json: AlertRuleConfigJSON
  created_at: string
  updated_at: string
}

interface AlertEvent {
  id: string
  rule_id: string
  tenant_id: string
  severity: string
  message: string
  context_json?: Record<string, unknown>
  status: string
  delivered_at?: string
  attempt_count?: number
  created_at: string
}

export default function TenantDetail() {
  const { id } = useParams<{ id: string }>()
  const [searchParams] = useSearchParams()
  const [tenant, setTenant] = useState<Tenant | null>(null)
  const [agents, setAgents] = useState<Agent[]>([])
  const [apiKeys, setApiKeys] = useState<ApiKey[]>([])
  const [approvers, setApprovers] = useState<Approver[]>([])
  const [notificationConfig, setNotificationConfig] = useState<TenantNotificationConfig | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const fetchSeq = useRef(0)

  const [agentForm, setAgentForm] = useState({ name: '' })
  const [keyForm, setKeyForm] = useState({ name: '' })
  const [newKeyRaw, setNewKeyRaw] = useState('')
  const [creating, setCreating] = useState(false)

  const [approverEmail, setApproverEmail] = useState('')
  const [approverSlackUserID, setApproverSlackUserID] = useState('')
  const [approverName, setApproverName] = useState('')

  const [allowlistSource, setAllowlistSource] = useState<string>('db')
  const [activeTab, setActiveTab] = useState<'agents' | 'api_keys' | 'approvers' | 'alerts'>('agents')

  const [notifForm, setNotifForm] = useState({ approver_group: '', slack_channel: '', webhook_url: '', webhook_secret_ref: '' })
  const [savingNotif, setSavingNotif] = useState(false)
  const [notifError, setNotifError] = useState('')

  const [alertRules, setAlertRules] = useState<AlertRule[]>([])
  const [alertEvents, setAlertEvents] = useState<AlertEvent[]>([])
  const [alertsLoading, setAlertsLoading] = useState(false)
  const [alertsError, setAlertsError] = useState('')

  const [alertRuleForm, setAlertRuleForm] = useState({ name: '', n: 3, mMinutes: 5, enabled: true })
  const [editingRuleId, setEditingRuleId] = useState<string | null>(null)
  const [editRuleForm, setEditRuleForm] = useState({ name: '', n: 3, mMinutes: 5, enabled: true })
  const [alertRuleSaving, setAlertRuleSaving] = useState(false)

  useEffect(() => {
    const tab = searchParams.get('tab')
    if (tab === 'agents' || tab === 'api_keys' || tab === 'approvers' || tab === 'alerts') {
      setActiveTab(tab)
    } else {
      setActiveTab('agents')
    }
  }, [searchParams, id])

  async function fetchAll() {
    const seq = ++fetchSeq.current
    setLoading(true)
    setError('')
    setTenant(null)
    setAgents([])
    setApiKeys([])
    setApprovers([])
    setNotificationConfig(null)
    setNotifError('')
    setAlertRules([])
    setAlertEvents([])
    setAlertsError('')
    setNotifForm({ approver_group: '', slack_channel: '', webhook_url: '', webhook_secret_ref: '' })
    setAllowlistSource('db')

    try {
      let notifCfgFetchError: string | null = null
      const [t, ag, keys, approverResp, notifCfg] = await Promise.all([
        api.get(`/admin/tenants/${id}`).catch(() => null),
        api.get(`/admin/tenants/${id}/agents`).catch(() => []),
        api.get(`/admin/tenants/${id}/apikeys`).catch(() => []),
        api.get(`/admin/tenants/${id}/approvers`).catch(() => ({ approvers: [], allowlist_source: 'db' })),
        api.get(`/admin/tenants/${id}/notification-config`).catch((err) => {
          notifCfgFetchError = err?.message || 'Failed to load notification config'
          return null
        }),
      ])
      if (seq !== fetchSeq.current) return
      setTenant(t ?? null)
      setAgents(Array.isArray(ag) ? ag : ag?.agents || [])
      setApiKeys(Array.isArray(keys) ? keys : keys?.api_keys || [])
      setApprovers(Array.isArray(approverResp?.approvers) ? approverResp.approvers : [])
      if (approverResp?.allowlist_source) setAllowlistSource(approverResp.allowlist_source)
      if (notifCfg) {
        setNotificationConfig(notifCfg)
        const slack = notifCfg.notify?.find((n: any) => n.kind === 'slack')
        const webhook = notifCfg.notify?.find((n: any) => n.kind === 'webhook')
        setNotifForm({
          approver_group: notifCfg.approver_group || '',
          slack_channel: slack?.channel || '',
          webhook_url: webhook?.url || '',
          webhook_secret_ref: webhook?.secret_ref || '',
        })
      } else if (notifCfgFetchError) {
        setNotifError(notifCfgFetchError)
      }
    } catch (err: any) {
      if (seq === fetchSeq.current) setError(err.message)
    } finally {
      if (seq === fetchSeq.current) setLoading(false)
    }
  }

  useEffect(() => { fetchAll() }, [id])

  async function fetchAlerts() {
    setAlertsLoading(true)
    setAlertsError('')
    try {
      const since = new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString()
      const [rulesResp, eventsResp] = await Promise.all([
        api.get(`/admin/tenants/${id}/alerts/rules`).catch(() => []),
        api.get(`/admin/tenants/${id}/alerts/events?limit=50&since=${encodeURIComponent(since)}`).catch(() => []),
      ])
      setAlertRules(Array.isArray(rulesResp) ? rulesResp as AlertRule[] : [])
      setAlertEvents(Array.isArray(eventsResp) ? eventsResp as AlertEvent[] : [])
    } catch (err: any) {
      setAlertsError(err?.message || 'Failed to load alerts')
    } finally {
      setAlertsLoading(false)
    }
  }

  useEffect(() => {
    if (activeTab === 'alerts') void fetchAlerts()
  }, [activeTab, id])

  async function createAlertRule(e: FormEvent) {
    e.preventDefault()
    setAlertRuleSaving(true)
    setAlertsError('')
    try {
      if (!id) throw new Error('tenant id missing')
      await api.post(`/admin/tenants/${id}/alerts/rules`, {
        name: alertRuleForm.name,
        kind: 'deny_spike',
        enabled: alertRuleForm.enabled,
        config_json: { n: alertRuleForm.n, m_minutes: alertRuleForm.mMinutes },
      })
      setAlertRuleForm({ name: '', n: 3, mMinutes: 5, enabled: true })
      await fetchAlerts()
    } catch (err: any) {
      setAlertsError(err?.message || 'Failed to create alert rule')
    } finally {
      setAlertRuleSaving(false)
    }
  }

  function startEditRule(rule: AlertRule) {
    setEditingRuleId(rule.id)
    setEditRuleForm({
      name: rule.name,
      n: rule.config_json.n,
      mMinutes: rule.config_json.m_minutes,
      enabled: rule.enabled,
    })
  }

  async function saveEditRule() {
    if (!editingRuleId) return
    setAlertRuleSaving(true)
    setAlertsError('')
    try {
      await api.put(`/admin/tenants/${id}/alerts/rules/${editingRuleId}`, {
        name: editRuleForm.name,
        kind: 'deny_spike',
        enabled: editRuleForm.enabled,
        config_json: { n: editRuleForm.n, m_minutes: editRuleForm.mMinutes },
      })
      setEditingRuleId(null)
      await fetchAlerts()
    } catch (err: any) {
      setAlertsError(err?.message || 'Failed to update alert rule')
    } finally {
      setAlertRuleSaving(false)
    }
  }

  async function setRuleEnabled(rule: AlertRule, enabled: boolean) {
    setAlertRuleSaving(true)
    setAlertsError('')
    try {
      await api.put(`/admin/tenants/${id}/alerts/rules/${rule.id}`, {
        name: rule.name,
        kind: 'deny_spike',
        enabled,
        config_json: { n: rule.config_json.n, m_minutes: rule.config_json.m_minutes },
      })
      await fetchAlerts()
    } catch (err: any) {
      setAlertsError(err?.message || 'Failed to update rule')
    } finally {
      setAlertRuleSaving(false)
    }
  }

  async function deleteAlertRule(ruleID: string) {
    setAlertRuleSaving(true)
    setAlertsError('')
    try {
      await api.delete(`/admin/tenants/${id}/alerts/rules/${ruleID}`)
      if (editingRuleId === ruleID) setEditingRuleId(null)
      await fetchAlerts()
    } catch (err: any) {
      setAlertsError(err?.message || 'Failed to delete rule')
    } finally {
      setAlertRuleSaving(false)
    }
  }

  async function createAgent(e: FormEvent) {
    e.preventDefault()
    setCreating(true)
    try {
      await api.post(`/admin/tenants/${id}/agents`, agentForm)
      setAgentForm({ name: '' })
      await fetchAll()
    } catch (err: any) {
      setError(err.message)
    } finally {
      setCreating(false)
    }
  }

  async function createKey(e: FormEvent) {
    e.preventDefault()
    setCreating(true)
    setNewKeyRaw('')
    try {
      const data = await api.post(`/admin/tenants/${id}/apikeys`, { name: keyForm.name })
      setNewKeyRaw(data.raw_key || data.key || '')
      setKeyForm({ name: '' })
      await fetchAll()
    } catch (err: any) {
      setError(err.message)
    } finally {
      setCreating(false)
    }
  }

  async function revokeKey(keyId: string) {
    try {
      await api.post(`/admin/tenants/${id}/apikeys/${keyId}/revoke`)
      await fetchAll()
    } catch (err: any) {
      setError(err.message)
    }
  }

  async function saveNotificationConfig(e: FormEvent) {
    e.preventDefault()
    setSavingNotif(true)
    setNotifError('')

    try {
      if (!notificationConfig) throw new Error('Notification configuration not available for this user.')

      const approverGroup = notifForm.approver_group.trim()
      const notify: Array<any> = []

      const slackChannel = notifForm.slack_channel.trim()
      if (slackChannel) {
        notify.push({ kind: 'slack', channel: slackChannel })
      }

      const webhookUrl = notifForm.webhook_url.trim()
      const webhookSecretRef = notifForm.webhook_secret_ref.trim()
      if (webhookUrl || webhookSecretRef) {
        if (!webhookUrl) throw new Error('Webhook URL is required when configuring a webhook.')
        if (!webhookSecretRef) throw new Error('Webhook secret reference is required when configuring a webhook.')
        notify.push({ kind: 'webhook', url: webhookUrl, secret_ref: webhookSecretRef })
      }

      await api.put(`/admin/tenants/${id}/notification-config`, {
        approver_group: approverGroup,
        notify,
      })
      await fetchAll()
    } catch (err: any) {
      setNotifError(err.message || 'Failed to save notification configuration.')
    } finally {
      setSavingNotif(false)
    }
  }

  async function addApprover(e: FormEvent) {
    e.preventDefault()
    setCreating(true)
    setError('')
    try {
      const payload: any = { role: 'approver' }
      const email = approverEmail.trim()
      const slackUserID = approverSlackUserID.trim()
      const name = approverName.trim()
      if (email) payload.email = email
      if (slackUserID) payload.slack_user_id = slackUserID
      if (name) payload.name = name
      if (!payload.email && !payload.slack_user_id) {
        throw new Error('Provide email and/or slack_user_id')
      }
      await api.post(`/admin/tenants/${id}/approvers`, payload)
      setApproverEmail('')
      setApproverSlackUserID('')
      setApproverName('')
      await fetchAll()
    } catch (err: any) {
      setError(err.message)
    } finally {
      setCreating(false)
    }
  }

  async function removeApprover(userID: string) {
    setCreating(true)
    setError('')
    try {
      await api.delete(`/admin/tenants/${id}/approvers/${userID}`)
      await fetchAll()
    } catch (err: any) {
      setError(err.message)
    } finally {
      setCreating(false)
    }
  }

  if (loading) return <div className="loading">Loading tenant…</div>
  if (error) return <div className="error-msg">{error}</div>
  if (!tenant) return <div className="error-msg">Tenant not found</div>

  return (
    <div>
      <div className="flex-between">
        <div className="page-header">
          <h2>{tenant.name}</h2>
          <p>Tenant management — {tenant.id}</p>
        </div>
        <Link to="/tenants" className="btn btn-outline">← Back</Link>
      </div>

      {error && <div className="error-msg">{error}</div>}

      <div className="detail-panel">
        <h3>Tenant Info</h3>
        <div className="detail-row">
          <div className="detail-label">ID</div>
          <div className="detail-value">{tenant.id}</div>
        </div>
        {tenant.config?.org_name && (
          <div className="detail-row">
            <div className="detail-label">Organization</div>
            <div className="detail-value">{tenant.config.org_name}</div>
          </div>
        )}
        <div className="detail-row">
          <div className="detail-label">Status</div>
          <div className="detail-value">
            <span className={`badge ${tenant.status === 'active' ? 'badge-green' : 'badge-red'}`}>{tenant.status}</span>
          </div>
        </div>
        <div className="detail-row">
          <div className="detail-label">Created</div>
          <div className="detail-value">{formatDate(tenant.created_at)}</div>
        </div>
      </div>

      <div className="tabs mt-16" style={{ display: 'flex', gap: 8 }}>
        <button
          className={`btn btn-outline btn-sm ${activeTab === 'agents' ? 'active' : ''}`}
          onClick={() => setActiveTab('agents')}
        >
          Agents
        </button>
        <button
          className={`btn btn-outline btn-sm ${activeTab === 'api_keys' ? 'active' : ''}`}
          onClick={() => setActiveTab('api_keys')}
        >
          API Keys
        </button>
        <button
          className={`btn btn-outline btn-sm ${activeTab === 'approvers' ? 'active' : ''}`}
          onClick={() => setActiveTab('approvers')}
        >
          Approvers
        </button>
        <button
          className={`btn btn-outline btn-sm ${activeTab === 'alerts' ? 'active' : ''}`}
          onClick={() => setActiveTab('alerts')}
        >
          Alerts
        </button>
      </div>

      {activeTab === 'agents' && (
        <>
          <div className="section-title">Agents</div>
          <div className="form-card">
            <h3>Register Agent</h3>
            <form onSubmit={createAgent}>
              <div className="form-inline">
                <div className="form-group">
                  <label>Agent Name</label>
                  <input value={agentForm.name} onChange={e => setAgentForm({ name: e.target.value })} required />
                </div>
                <button className="btn btn-primary" disabled={creating}>Create</button>
              </div>
            </form>
          </div>

          <div className="table-container">
            <table>
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Name</th>
                  <th>Created</th>
                </tr>
              </thead>
              <tbody>
                {agents.length === 0 ? (
                  <tr><td colSpan={3} style={{ textAlign: 'center', padding: 24, color: '#94a3b8' }}>No agents</td></tr>
                ) : (
                  agents.map(a => (
                    <tr key={a.id}>
                      <td style={{ fontFamily: 'monospace', fontSize: 12 }}>{a.id.slice(0, 12)}…</td>
                      <td>{a.name}</td>
                      <td>{formatDate(a.created_at, 'date')}</td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </>
      )}

      {activeTab === 'api_keys' && (
        <>
          <div className="section-title mt-16">API Keys</div>
          <div className="form-card">
            <h3>Create API Key</h3>
            <form onSubmit={createKey}>
              <div className="form-inline">
                <div className="form-group">
                  <label>Name</label>
                  <input value={keyForm.name} onChange={e => setKeyForm({ name: e.target.value })} required />
                </div>
                <button className="btn btn-primary" disabled={creating}>Create</button>
              </div>
            </form>
            {newKeyRaw && (
              <div style={{ marginTop: 16 }}>
                <p style={{ fontSize: 13, fontWeight: 600, color: '#ef4444', marginBottom: 4 }}>
                  Copy this key now — it will not be shown again:
                </p>
                <div className="key-display">{newKeyRaw}</div>
              </div>
            )}
          </div>

          <div className="section-title mt-16">Notification Routing</div>
          <div className="form-card">
            <h3>Approval notifications</h3>
            {notifError && <div className="error-msg">{notifError}</div>}
            {notificationConfig === null ? (
              <div style={{ color: '#64748b', fontSize: 13 }}>
                Notification configuration not available for this user (or not yet loaded).
              </div>
            ) : (
              <form onSubmit={saveNotificationConfig}>
                <div className="form-group">
                  <label>Approver group</label>
                  <input
                    value={notifForm.approver_group}
                    onChange={e => setNotifForm({ ...notifForm, approver_group: e.target.value })}
                    placeholder="approver_group (e.g., platform_admin/tenant_admin)"
                  />
                </div>

                <div className="form-group">
                  <label>Slack channel (optional)</label>
                  <input
                    value={notifForm.slack_channel}
                    onChange={e => setNotifForm({ ...notifForm, slack_channel: e.target.value })}
                    placeholder="#team-alerts"
                  />
                </div>

                <div className="form-group">
                  <label>Webhook URL (optional)</label>
                  <input
                    value={notifForm.webhook_url}
                    onChange={e => setNotifForm({ ...notifForm, webhook_url: e.target.value })}
                    placeholder="https://hooks.example.com/..."
                  />
                </div>

                <div className="form-group">
                  <label>Webhook secret reference (optional)</label>
                  <input
                    value={notifForm.webhook_secret_ref}
                    onChange={e => setNotifForm({ ...notifForm, webhook_secret_ref: e.target.value })}
                    placeholder="secret_ref name"
                  />
                </div>

                <button className="btn btn-primary" disabled={savingNotif}>
                  {savingNotif ? 'Saving…' : 'Save notification config'}
                </button>
              </form>
            )}
          </div>

          <div className="table-container">
            <table>
              <thead>
                <tr>
                  <th>Prefix</th>
                  <th>Name</th>
                  <th>Status</th>
                  <th>Created</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {apiKeys.length === 0 ? (
                  <tr><td colSpan={5} style={{ textAlign: 'center', padding: 24, color: '#94a3b8' }}>No API keys</td></tr>
                ) : (
                  apiKeys.map(k => (
                    <tr key={k.id}>
                      <td style={{ fontFamily: 'monospace' }}>{k.key_prefix}…</td>
                      <td>{k.name}</td>
                      <td>
                        {k.status === 'revoked'
                          ? <span className="badge badge-red">Revoked</span>
                          : <span className="badge badge-green">Active</span>}
                      </td>
                      <td>{formatDate(k.created_at, 'date')}</td>
                      <td>
                        {k.status !== 'revoked' && (
                          <button className="btn btn-danger btn-sm" onClick={() => revokeKey(k.id)}>Revoke</button>
                        )}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </>
      )}

      {activeTab === 'alerts' && (
        <>
          <div className="section-title mt-16">Alerts</div>

          {alertsError && <div className="error-msg">{alertsError}</div>}

          <div className="form-card mt-16">
            <h3>+ New deny_spike rule</h3>
            <form onSubmit={createAlertRule}>
              <div className="form-inline" style={{ gap: 16, flexWrap: 'wrap' }}>
                <div className="form-group" style={{ minWidth: 240 }}>
                  <label>Rule name</label>
                  <input
                    value={alertRuleForm.name}
                    onChange={e => setAlertRuleForm(f => ({ ...f, name: e.target.value }))}
                    placeholder="e.g., Deny spike detector"
                    required
                  />
                </div>
                <div className="form-group" style={{ minWidth: 180 }}>
                  <label>N (denies)</label>
                  <input
                    type="number"
                    value={alertRuleForm.n}
                    min={1}
                    onChange={e => setAlertRuleForm(f => ({ ...f, n: parseInt(e.target.value || '0', 10) || 1 }))}
                    required
                  />
                </div>
                <div className="form-group" style={{ minWidth: 220 }}>
                  <label>M (window minutes)</label>
                  <input
                    type="number"
                    value={alertRuleForm.mMinutes}
                    min={1}
                    onChange={e => setAlertRuleForm(f => ({ ...f, mMinutes: parseInt(e.target.value || '0', 10) || 1 }))}
                    required
                  />
                </div>
                <div className="form-group" style={{ minWidth: 200 }}>
                  <label>Enabled</label>
                  <div style={{ marginTop: 6 }}>
                    <input
                      type="checkbox"
                      checked={alertRuleForm.enabled}
                      onChange={e => setAlertRuleForm(f => ({ ...f, enabled: e.target.checked }))}
                    />{' '}
                    <span style={{ fontSize: 13, color: '#334155' }}>{alertRuleForm.enabled ? 'On' : 'Off'}</span>
                  </div>
                </div>
                <div style={{ display: 'flex', alignItems: 'flex-end' }}>
                  <button className="btn btn-primary" disabled={alertRuleSaving || alertsLoading}>
                    {alertRuleSaving ? 'Saving…' : 'Create'}
                  </button>
                </div>
              </div>
            </form>
          </div>

          <div className="section-title mt-16">Alert Rules</div>
          <div className="table-container">
            <table>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>N</th>
                  <th>M (minutes)</th>
                  <th>Status</th>
                  <th>Updated</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {alertsLoading ? (
                  <tr>
                    <td colSpan={6} className="loading">
                      Loading…
                    </td>
                  </tr>
                ) : alertRules.length === 0 ? (
                  <tr>
                    <td colSpan={6} style={{ textAlign: 'center', padding: 24, color: '#94a3b8' }}>
                      No alert rules configured
                    </td>
                  </tr>
                ) : (
                  alertRules.map(r => (
                    <tr key={r.id}>
                      <td style={{ fontWeight: 600 }}>
                        {editingRuleId === r.id ? (
                          <input
                            value={editRuleForm.name}
                            onChange={e => setEditRuleForm(f => ({ ...f, name: e.target.value }))}
                            required
                          />
                        ) : (
                          r.name
                        )}
                      </td>
                      <td>
                        {editingRuleId === r.id ? (
                          <input
                            type="number"
                            min={1}
                            value={editRuleForm.n}
                            onChange={e => setEditRuleForm(f => ({ ...f, n: parseInt(e.target.value || '0', 10) || 1 }))}
                          />
                        ) : (
                          r.config_json.n
                        )}
                      </td>
                      <td>
                        {editingRuleId === r.id ? (
                          <input
                            type="number"
                            min={1}
                            value={editRuleForm.mMinutes}
                            onChange={e => setEditRuleForm(f => ({ ...f, mMinutes: parseInt(e.target.value || '0', 10) || 1 }))}
                          />
                        ) : (
                          r.config_json.m_minutes
                        )}
                      </td>
                      <td>
                        {editingRuleId === r.id ? (
                          <label style={{ display: 'inline-flex', gap: 8, alignItems: 'center' }}>
                            <input
                              type="checkbox"
                              checked={editRuleForm.enabled}
                              onChange={e => setEditRuleForm(f => ({ ...f, enabled: e.target.checked }))}
                            />
                            <span style={{ fontSize: 13, color: '#334155' }}>{editRuleForm.enabled ? 'On' : 'Off'}</span>
                          </label>
                        ) : r.enabled ? (
                          <span className="badge badge-green">Active</span>
                        ) : (
                          <span className="badge badge-gray">Disabled</span>
                        )}
                      </td>
                      <td>{formatDate(r.updated_at, 'date')}</td>
                      <td style={{ width: 280 }}>
                        {editingRuleId === r.id ? (
                          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', justifyContent: 'flex-end' }}>
                            <button className="btn btn-primary btn-sm" onClick={saveEditRule}>
                              Save
                            </button>
                            <button className="btn btn-outline btn-sm" onClick={() => setEditingRuleId(null)}>
                              Cancel
                            </button>
                          </div>
                        ) : (
                          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', justifyContent: 'flex-end' }}>
                            <button className="btn btn-outline btn-sm" onClick={() => startEditRule(r)}>
                              Edit
                            </button>
                            <button
                              className={`btn btn-sm ${r.enabled ? 'btn-outline-danger' : 'btn-outline-success'}`}
                              onClick={() => setRuleEnabled(r, !r.enabled)}
                              disabled={alertRuleSaving}
                            >
                              {r.enabled ? 'Disable' : 'Enable'}
                            </button>
                            <button className="btn btn-danger btn-sm" onClick={() => deleteAlertRule(r.id)} disabled={alertRuleSaving}>
                              Delete
                            </button>
                          </div>
                        )}
                      </td>
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
                  <th>Status</th>
                  <th>Severity</th>
                  <th>Message</th>
                  <th>Fired</th>
                  <th>Delivered</th>
                </tr>
              </thead>
              <tbody>
                {alertsLoading ? (
                  <tr>
                    <td colSpan={6} className="loading">
                      Loading…
                    </td>
                  </tr>
                ) : alertEvents.length === 0 ? (
                  <tr>
                    <td colSpan={6} style={{ textAlign: 'center', padding: 24, color: '#94a3b8' }}>
                      No alert events yet
                    </td>
                  </tr>
                ) : (
                  alertEvents.map(ev => (
                    <tr key={ev.id}>
                      <td style={{ fontFamily: 'monospace', fontSize: 12 }}>{ev.rule_id.slice(0, 8)}…</td>
                      <td>
                        {ev.status === 'sent' ? (
                          <span className="badge badge-green">Sent</span>
                        ) : ev.status === 'pending' ? (
                          <span className="badge badge-yellow">Pending</span>
                        ) : (
                          <span className="badge badge-gray">{ev.status}</span>
                        )}
                      </td>
                      <td>
                        <span className={`badge ${ev.severity === 'critical' ? 'badge-red' : ev.severity === 'warning' ? 'badge-yellow' : 'badge-gray'}`}>
                          {ev.severity}
                        </span>
                      </td>
                      <td>{ev.message}</td>
                      <td>{formatDate(ev.created_at, 'date')}</td>
                      <td>{formatDate(ev.delivered_at || null, 'date')}</td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </>
      )}

      {activeTab === 'approvers' && (
        <>
          { (allowlistSource === 'env' || allowlistSource === 'both') && (
            <div className="warn-banner" style={{ marginTop: 16, border: '1px solid #f59e0b', padding: 12, borderRadius: 8, background: '#fffbeb' }}>
              <div style={{ fontWeight: 700, marginBottom: 4 }}>Dev bootstrap allowlists enabled</div>
              <div style={{ color: '#92400e', fontSize: 13 }}>Approver authorization may allow env allowlists in addition to DB roles.</div>
            </div>
          )}

          <div className="section-title mt-16">Approvers</div>

          <div className="form-card">
            <h3>Add Approver</h3>
            <form onSubmit={addApprover}>
              <div className="form-inline" style={{ gap: 16, flexWrap: 'wrap' }}>
                <div className="form-group" style={{ minWidth: 280 }}>
                  <label>Email</label>
                  <input value={approverEmail} onChange={e => setApproverEmail(e.target.value)} placeholder="name@company.com" />
                </div>
                <div className="form-group" style={{ minWidth: 280 }}>
                  <label>Slack user id (optional)</label>
                  <input value={approverSlackUserID} onChange={e => setApproverSlackUserID(e.target.value)} placeholder="U1234567890" />
                  <div style={{ marginTop: 4, color: '#64748b', fontSize: 12 }}>
                    If provided without an email, this will only link to an existing user.
                    Provide an email to create the user + link Slack id.
                  </div>
                </div>
                <div className="form-group" style={{ minWidth: 220 }}>
                  <label>Name (optional)</label>
                  <input value={approverName} onChange={e => setApproverName(e.target.value)} placeholder="Full name" />
                </div>
                <button className="btn btn-primary" disabled={creating}>Add</button>
              </div>
            </form>
          </div>

          <div className="table-container" style={{ marginTop: 16 }}>
            <table>
              <thead>
                <tr>
                  <th>Email</th>
                  <th>Name</th>
                  <th>Slack user id</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {approvers.length === 0 ? (
                  <tr><td colSpan={4} style={{ textAlign: 'center', padding: 24, color: '#94a3b8' }}>No approvers</td></tr>
                ) : (
                  approvers.map(a => (
                    <tr key={a.id}>
                      <td>{a.email}</td>
                      <td>{a.name || '—'}</td>
                      <td style={{ fontFamily: 'monospace', fontSize: 12 }}>{a.slack_user_id ? a.slack_user_id : '—'}</td>
                      <td>
                        <button className="btn btn-danger btn-sm" onClick={() => removeApprover(a.id)} disabled={creating}>
                          Remove
                        </button>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  )
}
