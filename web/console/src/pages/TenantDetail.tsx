import { useState, useEffect, FormEvent, useRef } from 'react'
import { useParams, Link, useSearchParams } from 'react-router-dom'
import { api, formatDate } from '../api'
import { InlineErrorState } from '../ui'

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
  expires_at?: string | null
  last_used_at?: string | null
  is_primary: boolean
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
  next_attempt_at?: string
  last_error?: string
  created_at: string
}

interface TenantAnalyticsTotals {
  total_events: number
  allow_count: number
  deny_count: number
  approve_count: number
}

interface TenantAnalyticsTrendBucket {
  bucket: string
  total: number
  allow_count: number
  deny_count: number
  approve_count: number
}

interface TenantRiskHeatmapRow {
  risk_score: number
  allow_count: number
  deny_count: number
  approve_count: number
  total: number
}

interface TenantAgentBreakdownRow {
  agent_id: string
  allow_count: number
  deny_count: number
  approve_count: number
  total: number
}

interface TenantOnboardingChecklist {
  has_api_key: boolean
  has_approver: boolean
  has_toolcall: boolean
  has_approval: boolean
  has_execution: boolean
}

interface TenantAnalyticsSummary {
  range_start: string
  range_end: string
  totals: TenantAnalyticsTotals
  trend: TenantAnalyticsTrendBucket[]
  risk_heatmap: TenantRiskHeatmapRow[]
  per_agent: TenantAgentBreakdownRow[]
  onboarding_checklist: TenantOnboardingChecklist
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
  const [rotationNewKeyRaw, setRotationNewKeyRaw] = useState('')
  const [rotationName, setRotationName] = useState('')
  const [rotationExpiresAt, setRotationExpiresAt] = useState('')
  const [rotationMakePrimary, setRotationMakePrimary] = useState(true)
  const [rotationRevokeOldPrimary, setRotationRevokeOldPrimary] = useState(true)
  const [rotating, setRotating] = useState(false)
  const [rotationError, setRotationError] = useState('')
  const [creating, setCreating] = useState(false)
  const [updatingTenantStatus, setUpdatingTenantStatus] = useState(false)

  const [approverEmail, setApproverEmail] = useState('')
  const [approverSlackUserID, setApproverSlackUserID] = useState('')
  const [approverName, setApproverName] = useState('')

  const [allowlistSource, setAllowlistSource] = useState<string>('db')
  const [activeTab, setActiveTab] = useState<'agents' | 'api_keys' | 'approvers' | 'alerts' | 'analytics'>('agents')

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

  const [tenantAnalytics, setTenantAnalytics] = useState<TenantAnalyticsSummary | null>(null)
  const [analyticsLoading, setAnalyticsLoading] = useState(false)
  const [analyticsError, setAnalyticsError] = useState('')
  const [analyticsRangeHours, setAnalyticsRangeHours] = useState(24)
  const [analyticsBucketMinutes] = useState(60)
  const [analyticsTopAgents] = useState(5)

  useEffect(() => {
    const tab = searchParams.get('tab')
    if (tab === 'agents' || tab === 'api_keys' || tab === 'approvers' || tab === 'alerts' || tab === 'analytics') {
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
    setTenantAnalytics(null)
    setAnalyticsError('')
    setRotationNewKeyRaw('')
    setRotationName('')
    setRotationExpiresAt('')
    setRotationMakePrimary(true)
    setRotationRevokeOldPrimary(true)
    setRotating(false)
    setRotationError('')
    setNotifForm({ approver_group: '', slack_channel: '', webhook_url: '', webhook_secret_ref: '' })
    setAllowlistSource('db')

    try {
      const [tenantResp, agentsResp, keysResp, approversResp, notifCfgResp] = await Promise.allSettled([
        api.get(`/admin/tenants/${id}`),
        api.get(`/admin/tenants/${id}/agents`),
        api.get(`/admin/tenants/${id}/apikeys`),
        api.get(`/admin/tenants/${id}/approvers`),
        api.get(`/admin/tenants/${id}/notification-config`),
      ])
      if (seq !== fetchSeq.current) return

      if (tenantResp.status !== 'fulfilled') {
        throw tenantResp.reason
      }

      const partialFailures: string[] = []
      const tenantData = tenantResp.value as Tenant
      setTenant(tenantData ?? null)

      if (agentsResp.status === 'fulfilled') {
        const agentsData = agentsResp.value as Agent[] | { agents?: Agent[] }
        setAgents(Array.isArray(agentsData) ? agentsData : agentsData?.agents || [])
      } else {
        partialFailures.push('agents')
      }

      if (keysResp.status === 'fulfilled') {
        const apiKeyData = keysResp.value as ApiKey[] | { api_keys?: ApiKey[] }
        setApiKeys(Array.isArray(apiKeyData) ? apiKeyData : apiKeyData?.api_keys || [])
      } else {
        partialFailures.push('API keys')
      }

      if (approversResp.status === 'fulfilled') {
        const approverData = approversResp.value as { approvers?: Approver[]; allowlist_source?: string }
        setApprovers(Array.isArray(approverData?.approvers) ? approverData.approvers : [])
        if (approverData?.allowlist_source) setAllowlistSource(approverData.allowlist_source)
      } else {
        partialFailures.push('approvers')
      }

      if (notifCfgResp.status === 'fulfilled') {
        const notifCfg = notifCfgResp.value as TenantNotificationConfig
        setNotificationConfig(notifCfg)
        const slack = notifCfg.notify?.find((n: any) => n.kind === 'slack')
        const webhook = notifCfg.notify?.find((n: any) => n.kind === 'webhook')
        setNotifForm({
          approver_group: notifCfg.approver_group || '',
          slack_channel: slack?.channel || '',
          webhook_url: webhook?.url || '',
          webhook_secret_ref: webhook?.secret_ref || '',
        })
      } else {
        setNotifError(notifCfgResp.reason?.message || 'Failed to load notification config')
      }

      if (partialFailures.length > 0) {
        setError(`Some tenant sections could not be loaded: ${partialFailures.join(', ')}.`)
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
      const [rulesResp, eventsResp] = await Promise.allSettled([
        api.get(`/admin/tenants/${id}/alerts/rules`),
        api.get(`/admin/tenants/${id}/alerts/events?limit=50&since=${encodeURIComponent(since)}`),
      ])
      const failures: string[] = []
      if (rulesResp.status === 'fulfilled') {
        setAlertRules(Array.isArray(rulesResp.value) ? rulesResp.value as AlertRule[] : [])
      } else {
        setAlertRules([])
        failures.push('rules')
      }
      if (eventsResp.status === 'fulfilled') {
        setAlertEvents(Array.isArray(eventsResp.value) ? eventsResp.value as AlertEvent[] : [])
      } else {
        setAlertEvents([])
        failures.push('events')
      }
      if (failures.length > 0) {
        setAlertsError(`Some alert data could not be loaded: ${failures.join(', ')}.`)
      }
    } catch (err: any) {
      setAlertsError(err?.message || 'Failed to load alerts')
    } finally {
      setAlertsLoading(false)
    }
  }

  useEffect(() => {
    if (activeTab === 'alerts') void fetchAlerts()
  }, [activeTab, id])

  async function fetchTenantAnalytics() {
    setAnalyticsLoading(true)
    setAnalyticsError('')
    try {
      if (!id) throw new Error('tenant id missing')
      const rangeHours = analyticsRangeHours
      const summary = await api.get(
        `/admin/tenants/${id}/analytics/summary?range=${rangeHours}h&bucket_minutes=${analyticsBucketMinutes}&top_agents=${analyticsTopAgents}`,
      )
      setTenantAnalytics(summary as TenantAnalyticsSummary)
    } catch (err) {
      if (err instanceof Error) setAnalyticsError(err.message)
      else setAnalyticsError('Failed to load analytics')
      setTenantAnalytics(null)
    } finally {
      setAnalyticsLoading(false)
    }
  }

  useEffect(() => {
    if (activeTab === 'analytics') void fetchTenantAnalytics()
  }, [activeTab, id, analyticsRangeHours])

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

  async function rotatePrimaryKey(e: FormEvent) {
    e.preventDefault()
    setRotating(true)
    setRotationError('')
    setRotationNewKeyRaw('')
    try {
      if (!rotationName.trim()) throw new Error('rotation name required')

      const payload: Record<string, unknown> = {
        name: rotationName.trim(),
        make_primary: rotationMakePrimary,
        revoke_old_primary: rotationRevokeOldPrimary,
      }
      if (rotationExpiresAt.trim()) {
        payload.expires_at = rotationExpiresAt.trim()
      }

      const rotated = (await api.post(`/admin/tenants/${id}/apikeys/rotate`, payload)) as { raw_key?: string }
      const rawKey = rotated.raw_key || ''

      await fetchAll()
      setRotationNewKeyRaw(rawKey)
      setRotationName('')
      setRotationExpiresAt('')
      setRotationMakePrimary(true)
      setRotationRevokeOldPrimary(true)
    } catch (err: unknown) {
      setRotationError(err instanceof Error ? err.message : 'Failed to rotate API key')
    } finally {
      setRotating(false)
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

  async function updateTenantStatus(status: 'active' | 'disabled') {
    setUpdatingTenantStatus(true)
    setError('')
    try {
      await api.post(`/admin/tenants/${id}/status`, { status })
      await fetchAll()
    } catch (err: any) {
      setError(err.message)
    } finally {
      setUpdatingTenantStatus(false)
    }
  }

  if (loading) return <div className="loading">Loading tenant…</div>
  if (error && !tenant) return (
    <div>
      <InlineErrorState message={error} onRetry={() => void fetchAll()} />
      <Link to="/tenants" className="btn btn-outline" style={{ marginTop: 16 }}>← Back to Tenants</Link>
    </div>
  )
  if (!tenant) return (
    <div>
      <div className="error-msg">Tenant not found</div>
      <Link to="/tenants" className="btn btn-outline" style={{ marginTop: 16 }}>← Back to Tenants</Link>
    </div>
  )

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
          <div className="detail-label">Tenant Controls</div>
          <div className="detail-value">
            <button
              className={`btn btn-sm ${tenant.status === 'active' ? 'btn-danger' : 'btn-primary'}`}
              onClick={() => updateTenantStatus(tenant.status === 'active' ? 'disabled' : 'active')}
              disabled={updatingTenantStatus}
            >
              {updatingTenantStatus
                ? (tenant.status === 'active' ? 'Disabling…' : 'Enabling…')
                : (tenant.status === 'active' ? 'Disable Tenant' : 'Enable Tenant')}
            </button>
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
        <button
          className={`btn btn-outline btn-sm ${activeTab === 'analytics' ? 'active' : ''}`}
          onClick={() => setActiveTab('analytics')}
        >
          Analytics
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

          <div className="form-card mt-16">
            <h3>Rotate Primary Key</h3>
            {rotationError && <div className="error-msg">{rotationError}</div>}
            <div style={{ fontSize: 13, color: '#64748b', marginBottom: 12 }}>
              Workflow: create new key -&gt; optionally mark primary -&gt; optionally revoke old primary.
            </div>

            <form onSubmit={rotatePrimaryKey}>
              <div className="form-inline" style={{ gap: 16, flexWrap: 'wrap', alignItems: 'flex-end' }}>
                <div className="form-group" style={{ minWidth: 260 }}>
                  <label>New key name</label>
                  <input value={rotationName} onChange={e => setRotationName(e.target.value)} required placeholder="e.g., rotated-2026-03" />
                </div>

                <div className="form-group" style={{ minWidth: 220 }}>
                  <label>Expires (optional)</label>
                  <input
                    type="date"
                    value={rotationExpiresAt}
                    onChange={e => setRotationExpiresAt(e.target.value)}
                  />
                </div>

                <div className="form-group" style={{ minWidth: 240 }}>
                  <label style={{ display: 'block' }}>Options</label>
                  <div style={{ display: 'grid', gap: 8, marginTop: 6 }}>
                    <label style={{ display: 'inline-flex', gap: 10, alignItems: 'center' }}>
                      <input type="checkbox" checked={rotationMakePrimary} onChange={e => setRotationMakePrimary(e.target.checked)} />
                      <span style={{ fontSize: 13, color: '#334155' }}>Make new key primary</span>
                    </label>
                    <label style={{ display: 'inline-flex', gap: 10, alignItems: 'center' }}>
                      <input type="checkbox" checked={rotationRevokeOldPrimary} onChange={e => setRotationRevokeOldPrimary(e.target.checked)} />
                      <span style={{ fontSize: 13, color: '#334155' }}>Revoke old primary</span>
                    </label>
                  </div>
                </div>

                <div style={{ display: 'flex', alignItems: 'flex-end' }}>
                  <button className="btn btn-primary" disabled={rotating || creating}>
                    {rotating ? 'Rotating…' : 'Rotate'}
                  </button>
                </div>
              </div>
            </form>

            {rotationNewKeyRaw && (
              <div style={{ marginTop: 16 }}>
                <p style={{ fontSize: 13, fontWeight: 600, color: '#ef4444', marginBottom: 4 }}>
                  Copy this rotated key now — it will not be shown again:
                </p>
                <div className="key-display">{rotationNewKeyRaw}</div>
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
                  <th>Primary</th>
                  <th>Status</th>
                  <th>Created</th>
                  <th>Expires</th>
                  <th>Last used</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {apiKeys.length === 0 ? (
                  <tr><td colSpan={8} style={{ textAlign: 'center', padding: 24, color: '#94a3b8' }}>No API keys</td></tr>
                ) : (
                  apiKeys.map(k => (
                    <tr key={k.id}>
                      <td style={{ fontFamily: 'monospace' }}>{k.key_prefix}…</td>
                      <td>{k.name}</td>
                      <td>
                        {k.is_primary && k.status === 'active'
                          ? <span className="badge badge-green">Primary</span>
                          : <span className="badge badge-gray">—</span>}
                      </td>
                      <td>
                        {k.status === 'revoked'
                          ? <span className="badge badge-red">Revoked</span>
                          : <span className="badge badge-green">Active</span>}
                      </td>
                      <td>{formatDate(k.created_at, 'date')}</td>
                      <td>
                        {k.expires_at ? (
                          (() => {
                            const expiresMs = new Date(k.expires_at || '').getTime()
                            const nowMs = Date.now()
                            const expired = !Number.isNaN(expiresMs) && expiresMs <= nowMs
                            const expiringSoon = !Number.isNaN(expiresMs) && !expired && expiresMs <= (nowMs + 30 * 24 * 60 * 60 * 1000)
                            return (
                              <span style={{ display: 'inline-flex', gap: 8, alignItems: 'center' }}>
                                <span>{formatDate(k.expires_at || null, 'date')}</span>
                                {expired && <span className="badge badge-red">Expired</span>}
                                {!expired && expiringSoon && <span className="badge badge-yellow">Expiring</span>}
                              </span>
                            )
                          })()
                        ) : (
                          <span style={{ color: '#64748b' }}>Never</span>
                        )}
                      </td>
                      <td>{k.last_used_at ? formatDate(k.last_used_at, 'date') : <span style={{ color: '#64748b' }}>—</span>}</td>
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

          {alertsError && <InlineErrorState message={alertsError} onRetry={() => void fetchAlerts()} />}

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
                  <th>Attempts</th>
                  <th>Message</th>
                  <th>Fired</th>
                  <th>Delivered</th>
                </tr>
              </thead>
              <tbody>
                {alertsLoading ? (
                  <tr>
                    <td colSpan={7} className="loading">
                      Loading…
                    </td>
                  </tr>
                ) : alertEvents.length === 0 ? (
                  <tr>
                    <td colSpan={7} style={{ textAlign: 'center', padding: 24, color: '#94a3b8' }}>
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
                      <td>{ev.attempt_count ?? 0}</td>
                      <td>
                        <div>{ev.message}</div>
                        {ev.last_error ? <div className="table-subtext">Last error: {ev.last_error}</div> : null}
                      </td>
                      <td>{formatDate(ev.created_at, 'date')}</td>
                      <td>
                        <div>{formatDate(ev.delivered_at || null, 'date')}</div>
                        {!ev.delivered_at && ev.next_attempt_at ? (
                          <div className="table-subtext">Retry {formatDate(ev.next_attempt_at, 'date')}</div>
                        ) : null}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </>
      )}

      {activeTab === 'analytics' && (
        <>
          <div className="section-title mt-16">Analytics</div>

          <div className="form-card mt-16">
            <h3>Tenant analytics</h3>
            <div className="form-inline" style={{ gap: 16, flexWrap: 'wrap', alignItems: 'flex-end' }}>
              <div className="form-group" style={{ minWidth: 220 }}>
                <label>Range</label>
                <select value={analyticsRangeHours} onChange={(e) => setAnalyticsRangeHours(Number(e.target.value))}>
                  <option value={6}>Last 6 hours</option>
                  <option value={24}>Last 24 hours</option>
                  <option value={168}>Last 7 days</option>
                </select>
              </div>
              <div style={{ fontSize: 13, color: '#64748b', paddingBottom: 6 }}>
                Bucket: {analyticsBucketMinutes} min · Top agents: {analyticsTopAgents}
              </div>
            </div>
          </div>

          {analyticsError && <div className="error-msg">{analyticsError}</div>}
          {analyticsLoading && <div className="loading">Loading analytics…</div>}

          {tenantAnalytics && (() => {
            const totals = tenantAnalytics.totals
            const trend = tenantAnalytics.trend
            const riskHeatmap = tenantAnalytics.risk_heatmap
            const perAgent = tenantAnalytics.per_agent
            const onboarding = tenantAnalytics.onboarding_checklist

            const maxDecision = Math.max(...trend.flatMap(b => [b.allow_count, b.deny_count, b.approve_count]), 1)
            const riskMaxTotal = Math.max(...riskHeatmap.map(r => r.total), 1)

            const badgeFor = (ok: boolean) => (ok ? <span className="badge badge-green">Done</span> : <span className="badge badge-gray">Pending</span>)
            const alphaFor = (count: number) => 0.08 + 0.92 * (riskMaxTotal > 0 ? count / riskMaxTotal : 0)

            return (
              <>
                <div className="card-grid mt-16">
                  <div className="card">
                    <div className="card-label">Total Events</div>
                    <div className="card-value">{totals.total_events.toLocaleString()}</div>
                  </div>
                  <div className="card">
                    <div className="card-label">Allow</div>
                    <div className="card-value green">{totals.allow_count.toLocaleString()}</div>
                  </div>
                  <div className="card">
                    <div className="card-label">Deny</div>
                    <div className="card-value red">{totals.deny_count.toLocaleString()}</div>
                  </div>
                  <div className="card">
                    <div className="card-label">Approve</div>
                    <div className="card-value yellow">{totals.approve_count.toLocaleString()}</div>
                  </div>
                </div>

                {trend.length > 0 && (
                  <div className="detail-panel">
                    <h3>Allow/Deny/Approve Trend</h3>
                    <div style={{ display: 'flex', alignItems: 'flex-end', gap: 2, height: 120, padding: '12px 0' }}>
                      {trend.map((b, i) => (
                        <div key={i} style={{ flex: 1, display: 'flex', alignItems: 'flex-end', gap: 2 }}>
                          <div
                            title={`allow: ${b.allow_count}`}
                            style={{
                              flex: 1,
                              background: '#22c55e',
                              borderRadius: '3px 3px 0 0',
                              height: `${(b.allow_count / maxDecision) * 100}%`,
                            }}
                          />
                          <div
                            title={`deny: ${b.deny_count}`}
                            style={{
                              flex: 1,
                              background: '#ef4444',
                              borderRadius: '3px 3px 0 0',
                              height: `${(b.deny_count / maxDecision) * 100}%`,
                            }}
                          />
                          <div
                            title={`approve: ${b.approve_count}`}
                            style={{
                              flex: 1,
                              background: '#eab308',
                              borderRadius: '3px 3px 0 0',
                              height: `${(b.approve_count / maxDecision) * 100}%`,
                            }}
                          />
                        </div>
                      ))}
                    </div>
                    <div style={{ display: 'flex', gap: 12, fontSize: 12, color: '#64748b', marginTop: 8 }}>
                      <span><span style={{ display: 'inline-block', width: 10, height: 10, background: '#22c55e', borderRadius: 2, marginRight: 6 }} />Allow</span>
                      <span><span style={{ display: 'inline-block', width: 10, height: 10, background: '#ef4444', borderRadius: 2, marginRight: 6 }} />Deny</span>
                      <span><span style={{ display: 'inline-block', width: 10, height: 10, background: '#eab308', borderRadius: 2, marginRight: 6 }} />Approve</span>
                    </div>
                    <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, color: '#94a3b8', marginTop: 12 }}>
                      <span>{formatDate(trend[0].bucket, 'date')}</span>
                      <span>{formatDate(trend[trend.length - 1].bucket, 'date')}</span>
                    </div>
                  </div>
                )}

                <div className="detail-panel mt-16">
                  <h3>Risk Heatmap</h3>
                  <div className="table-container" style={{ marginBottom: 0 }}>
                    <table>
                      <thead>
                        <tr>
                          <th>Risk</th>
                          <th>Allow</th>
                          <th>Deny</th>
                          <th>Approve</th>
                          <th>Total</th>
                        </tr>
                      </thead>
                      <tbody>
                        {riskHeatmap.map(r => (
                          <tr key={r.risk_score}>
                            <td style={{ fontFamily: 'monospace' }}>{r.risk_score}</td>
                            <td style={{ background: `rgba(34,197,94,${alphaFor(r.allow_count)})` }}>{r.allow_count}</td>
                            <td style={{ background: `rgba(239,68,68,${alphaFor(r.deny_count)})` }}>{r.deny_count}</td>
                            <td style={{ background: `rgba(234,179,8,${alphaFor(r.approve_count)})` }}>{r.approve_count}</td>
                            <td style={{ color: '#334155' }}>{r.total}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </div>

                <div className="section-title mt-16">Per-Agent Breakdown</div>
                <div className="table-container">
                  <table>
                    <thead>
                      <tr>
                        <th>Agent</th>
                        <th>Allow</th>
                        <th>Deny</th>
                        <th>Approve</th>
                        <th>Total</th>
                      </tr>
                    </thead>
                    <tbody>
                      {perAgent.length === 0 ? (
                        <tr>
                          <td colSpan={5} style={{ textAlign: 'center', padding: 24, color: '#94a3b8' }}>
                            No tool events in this range
                          </td>
                        </tr>
                      ) : (
                        perAgent.map(a => (
                          <tr key={a.agent_id}>
                            <td style={{ fontFamily: 'monospace', fontSize: 12 }}>{a.agent_id.slice(0, 12)}…</td>
                            <td>{a.allow_count}</td>
                            <td>{a.deny_count}</td>
                            <td>{a.approve_count}</td>
                            <td>{a.total}</td>
                          </tr>
                        ))
                      )}
                    </tbody>
                  </table>
                </div>

                <div className="form-card mt-16">
                  <h3>Onboarding Checklist</h3>
                  <div style={{ display: 'grid', gap: 12 }}>
                    <div className="detail-row" style={{ borderBottom: 'none', padding: 0 }}>
                      <div className="detail-label" style={{ minWidth: 240, color: '#64748b', fontWeight: 600 }}>Create API key</div>
                      <div>{badgeFor(onboarding.has_api_key)}</div>
                    </div>
                    <div className="detail-row" style={{ borderBottom: 'none', padding: 0 }}>
                      <div className="detail-label" style={{ minWidth: 240, color: '#64748b', fontWeight: 600 }}>Add approver</div>
                      <div>{badgeFor(onboarding.has_approver)}</div>
                    </div>
                    <div className="detail-row" style={{ borderBottom: 'none', padding: 0 }}>
                      <div className="detail-label" style={{ minWidth: 240, color: '#64748b', fontWeight: 600 }}>First tool-call</div>
                      <div>{badgeFor(onboarding.has_toolcall)}</div>
                    </div>
                    <div className="detail-row" style={{ borderBottom: 'none', padding: 0 }}>
                      <div className="detail-label" style={{ minWidth: 240, color: '#64748b', fontWeight: 600 }}>First approval</div>
                      <div>{badgeFor(onboarding.has_approval)}</div>
                    </div>
                    <div className="detail-row" style={{ borderBottom: 'none', padding: 0 }}>
                      <div className="detail-label" style={{ minWidth: 240, color: '#64748b', fontWeight: 600 }}>First execution</div>
                      <div>{badgeFor(onboarding.has_execution)}</div>
                    </div>
                  </div>
                </div>
              </>
            )
          })()}

          {!analyticsLoading && !tenantAnalytics && !analyticsError && (
            <div style={{ color: '#64748b', fontSize: 13, marginTop: 16 }}>No analytics data yet</div>
          )}
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
