import { useState, useEffect, FormEvent, useMemo, useRef } from 'react'
import { useParams, Link, useSearchParams } from 'react-router-dom'
import { api, formatDate } from '../api'
import { CopyIconButton, EmptyState, InlineErrorState, compareDate } from '../ui'

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
  status: 'active' | 'disabled'
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

function formatUTCDateTime(value?: string | null) {
  if (!value) return '—'
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return value
  return `${new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
    timeZone: 'UTC',
  }).format(parsed)} UTC`
}

function analyticsRangeLabel(rangeHours: number) {
  switch (rangeHours) {
    case 6:
      return 'Last 6 hours'
    case 24:
      return 'Last 24 hours'
    case 48:
      return 'Last 48 hours'
    case 168:
      return 'Last 7 days'
    default:
      return `Last ${rangeHours} hours`
  }
}

function approverLinkStatus(approver: Approver) {
  const hasEmail = !!approver.email?.trim()
  const hasSlack = !!approver.slack_user_id?.trim()
  if (hasEmail && hasSlack) return { label: 'Both', tone: 'green' }
  if (hasSlack) return { label: 'Slack linked', tone: 'blue' }
  return { label: 'Email only', tone: 'gray' }
}

export default function TenantDetail() {
  const { id } = useParams<{ id: string }>()
  const [searchParams, setSearchParams] = useSearchParams()
  const [tenant, setTenant] = useState<Tenant | null>(null)
  const [agents, setAgents] = useState<Agent[]>([])
  const [apiKeys, setApiKeys] = useState<ApiKey[]>([])
  const [approvers, setApprovers] = useState<Approver[]>([])
  const [notificationConfig, setNotificationConfig] = useState<TenantNotificationConfig | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const fetchSeq = useRef(0)
  const alertsFetchSeq = useRef(0)
  const analyticsFetchSeq = useRef(0)
  const [hideDisabledAgents, setHideDisabledAgents] = useState(false)

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

  const visibleAgents = useMemo(
    () =>
      [...agents].sort((left, right) => {
        if (left.status !== right.status) return left.status === 'active' ? -1 : 1
        return compareDate(right.created_at, left.created_at)
      }),
    [agents],
  )

  const visibleApiKeys = useMemo(
    () =>
      [...apiKeys].sort((left, right) => {
        if (left.is_primary !== right.is_primary) return left.is_primary ? -1 : 1
        if (left.status !== right.status) return left.status === 'active' ? -1 : 1
        return compareDate(right.created_at, left.created_at)
      }),
    [apiKeys],
  )

  const visibleApprovers = useMemo(
    () =>
      [...approvers].sort((left, right) => {
        const leftStatus = approverLinkStatus(left).label
        const rightStatus = approverLinkStatus(right).label
        if (leftStatus !== rightStatus) {
          const order = ['Both', 'Slack linked', 'Email only']
          return order.indexOf(leftStatus) - order.indexOf(rightStatus)
        }
        return (left.name || left.email || '').localeCompare(right.name || right.email || '')
      }),
    [approvers],
  )

  useEffect(() => {
    const tab = searchParams.get('tab')
    if (tab === 'agents' || tab === 'api_keys' || tab === 'approvers' || tab === 'alerts' || tab === 'analytics') {
      setActiveTab(tab)
    } else {
      setActiveTab('agents')
    }
  }, [searchParams, id])

  function selectTab(tab: 'agents' | 'api_keys' | 'approvers' | 'alerts' | 'analytics') {
    setActiveTab(tab)
    const next = new URLSearchParams(searchParams)
    next.set('tab', tab)
    setSearchParams(next)
  }

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
    setNotificationConfig(null)

    try {
      const [tenantResp, agentsResp, keysResp, approversResp, notifCfgResp] = await Promise.allSettled([
        api.get(`/admin/tenants/${id}`),
        api.get(`/admin/tenants/${id}/agents?include_disabled=${hideDisabledAgents ? 'false' : 'true'}`),
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
        const approverData = approversResp.value as Approver[] | { approvers?: Approver[]; allowlist_source?: string }
        const approverPayload = Array.isArray(approverData) ? null : approverData
        setApprovers(Array.isArray(approverData) ? approverData : Array.isArray(approverPayload?.approvers) ? approverPayload.approvers : [])
        if (approverPayload?.allowlist_source) setAllowlistSource(approverPayload.allowlist_source)
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
        setNotificationConfig(null)
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

  useEffect(() => { fetchAll() }, [id, hideDisabledAgents])

  async function fetchAlerts() {
    const seq = ++alertsFetchSeq.current
    setAlertsLoading(true)
    setAlertsError('')
    try {
      const since = new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString()
      const [rulesResp, eventsResp] = await Promise.allSettled([
        api.get(`/admin/tenants/${id}/alerts/rules`),
        api.get(`/admin/tenants/${id}/alerts/events?limit=50&since=${encodeURIComponent(since)}`),
      ])
      if (seq !== alertsFetchSeq.current) return
      const failures: string[] = []
      if (rulesResp.status === 'fulfilled') {
        const rulesData = rulesResp.value as AlertRule[] | { rules?: AlertRule[] }
        setAlertRules(Array.isArray(rulesData) ? rulesData : rulesData?.rules || [])
      } else {
        setAlertRules([])
        failures.push('rules')
      }
      if (eventsResp.status === 'fulfilled') {
        const eventsData = eventsResp.value as AlertEvent[] | { events?: AlertEvent[] }
        setAlertEvents(Array.isArray(eventsData) ? eventsData : eventsData?.events || [])
      } else {
        setAlertEvents([])
        failures.push('events')
      }
      if (failures.length > 0) {
        setAlertsError(`Some alert data could not be loaded: ${failures.join(', ')}.`)
      }
    } catch (err: any) {
      if (seq !== alertsFetchSeq.current) return
      setAlertsError(err?.message || 'Failed to load alerts')
    } finally {
      if (seq === alertsFetchSeq.current) setAlertsLoading(false)
    }
  }

  useEffect(() => {
    if (activeTab === 'alerts') void fetchAlerts()
  }, [activeTab, id])

  async function fetchTenantAnalytics() {
    const seq = ++analyticsFetchSeq.current
    setAnalyticsLoading(true)
    setAnalyticsError('')
    try {
      if (!id) throw new Error('tenant id missing')
      const rangeHours = analyticsRangeHours
      const summary = await api.get(
        `/admin/tenants/${id}/analytics/summary?range=${rangeHours}h&bucket_minutes=${analyticsBucketMinutes}&top_agents=${analyticsTopAgents}`,
      )
      if (seq !== analyticsFetchSeq.current) return
      setTenantAnalytics(summary as TenantAnalyticsSummary)
    } catch (err) {
      if (seq !== analyticsFetchSeq.current) return
      if (err instanceof Error) setAnalyticsError(err.message)
      else setAnalyticsError('Failed to load analytics')
      setTenantAnalytics(null)
    } finally {
      if (seq === analyticsFetchSeq.current) setAnalyticsLoading(false)
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

  async function updateAgentStatus(agentId: string, status: 'active' | 'disabled') {
    setCreating(true)
    setError('')
    try {
      await api.post(`/admin/tenants/${id}/agents/${agentId}/status`, { status })
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
      <Link to="/tenants" className="btn btn-outline back-link-spaced">← Back to Tenants</Link>
    </div>
  )
  if (!tenant) return (
    <div>
      <div className="error-msg">Tenant not found</div>
      <Link to="/tenants" className="btn btn-outline back-link-spaced">← Back to Tenants</Link>
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
          <div className="detail-value">
            <div className="inline-value-copy">
              <code className="mono" title={tenant.id}>{tenant.id}</code>
              <CopyIconButton text={tenant.id} label="Tenant ID" />
            </div>
          </div>
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

      <div className="tabs tenant-tabs mt-16">
        <button
          className={`btn btn-outline btn-sm tenant-tab ${activeTab === 'agents' ? 'is-active' : ''}`}
          onClick={() => selectTab('agents')}
        >
          Agents
        </button>
        <button
          className={`btn btn-outline btn-sm tenant-tab ${activeTab === 'api_keys' ? 'is-active' : ''}`}
          onClick={() => selectTab('api_keys')}
        >
          API Keys
        </button>
        <button
          className={`btn btn-outline btn-sm tenant-tab ${activeTab === 'approvers' ? 'is-active' : ''}`}
          onClick={() => selectTab('approvers')}
        >
          Approvers
        </button>
        <button
          className={`btn btn-outline btn-sm tenant-tab ${activeTab === 'alerts' ? 'is-active' : ''}`}
          onClick={() => selectTab('alerts')}
        >
          Alerts
        </button>
        <button
          className={`btn btn-outline btn-sm tenant-tab ${activeTab === 'analytics' ? 'is-active' : ''}`}
          onClick={() => selectTab('analytics')}
        >
          Analytics
        </button>
      </div>

      {activeTab === 'agents' && (
        <>
          <div className="section-title section-title-spacious">Agents</div>
          <div className="form-card">
            <h3>Register Agent</h3>
            <form onSubmit={createAgent}>
              <div className="form-inline">
                <div className="form-group">
                  <label htmlFor="tenant-agent-name">Agent Name</label>
                  <input id="tenant-agent-name" value={agentForm.name} onChange={e => setAgentForm({ name: e.target.value })} required />
                </div>
                <button className="btn btn-primary" disabled={creating}>Create</button>
              </div>
            </form>
            <div className="toggle-stack mt-16">
              <label className="toggle-field toggle-field-boxed">
                <input
                  type="checkbox"
                  checked={hideDisabledAgents}
                  onChange={e => setHideDisabledAgents(e.target.checked)}
                />
                <span>Hide disabled</span>
              </label>
            </div>
          </div>

          <div className="table-container">
            <table>
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Name</th>
                  <th>Status</th>
                  <th>Created</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {visibleAgents.length === 0 ? (
                  <tr><td colSpan={5} className="table-empty-copy-cell">No agents</td></tr>
                ) : (
                  visibleAgents.map(a => (
                    <tr key={a.id}>
                      <td>
                        <div className="inline-value-copy">
                          <code className="mono" title={a.id}>{a.id.slice(0, 12)}…</code>
                          <CopyIconButton text={a.id} label="Agent ID" />
                        </div>
                      </td>
                      <td>{a.name}</td>
                      <td>
                        <span className={`badge ${a.status === 'active' ? 'badge-green' : 'badge-red'}`}>{a.status}</span>
                      </td>
                      <td>{formatDate(a.created_at, 'date')}</td>
                      <td>
                        <button
                          className={`btn btn-sm ${a.status === 'active' ? 'btn-danger' : 'btn-outline'}`}
                          onClick={() => updateAgentStatus(a.id, a.status === 'active' ? 'disabled' : 'active')}
                          disabled={creating}
                        >
                          {a.status === 'active' ? 'Disable' : 'Enable'}
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

      {activeTab === 'api_keys' && (
        <>
          <div className="section-title section-title-spacious">API Keys</div>
          <div className="form-card">
            <h3>Create API Key</h3>
            <form onSubmit={createKey}>
              <div className="form-inline">
                <div className="form-group">
                  <label htmlFor="tenant-api-key-name">Name</label>
                  <input id="tenant-api-key-name" value={keyForm.name} onChange={e => setKeyForm({ name: e.target.value })} required />
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
            <div className="form-helper-text" style={{ marginBottom: 12 }}>
              Workflow: create new key -&gt; optionally mark primary -&gt; optionally revoke old primary.
            </div>

            <form onSubmit={rotatePrimaryKey}>
              <div className="form-grid api-key-rotation-grid">
                <div className="form-group api-key-rotation-field">
                  <label htmlFor="tenant-rotation-name">New key name</label>
                  <input id="tenant-rotation-name" value={rotationName} onChange={e => setRotationName(e.target.value)} required placeholder="e.g., rotated-2026-03" />
                </div>

                <div className="form-group api-key-rotation-field">
                  <label htmlFor="tenant-rotation-expires">Expires on (UTC date, optional)</label>
                  <input
                    id="tenant-rotation-expires"
                    type="date"
                    value={rotationExpiresAt}
                    onChange={e => setRotationExpiresAt(e.target.value)}
                  />
                </div>

                <div className="form-group api-key-rotation-field">
                  <label>Rotation options</label>
                  <div className="toggle-stack rotation-options-stack">
                    <label className="toggle-field toggle-field-boxed">
                      <input type="checkbox" checked={rotationMakePrimary} onChange={e => setRotationMakePrimary(e.target.checked)} />
                      <span>Make the new key primary immediately</span>
                    </label>
                    <label className="toggle-field toggle-field-boxed">
                      <input type="checkbox" checked={rotationRevokeOldPrimary} onChange={e => setRotationRevokeOldPrimary(e.target.checked)} />
                      <span>Revoke the old primary after rotation</span>
                    </label>
                  </div>
                </div>

                <div className="form-actions-row form-actions-row-end api-key-rotation-actions">
                  <button className="btn btn-primary" disabled={rotating || creating}>
                    {rotating ? 'Rotating…' : 'Rotate'}
                  </button>
                </div>
              </div>
              <div className="form-helper-text api-key-rotation-note">
                Use a calendar date like <code className="mono">2030-01-01</code>. The key stays active until that date passes.
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

          <div className="section-title section-title-spacious">Notification Routing</div>
          <div className="form-card">
            <h3>Approval notifications</h3>
            {notifError && <div className="error-msg">{notifError}</div>}
            {notificationConfig === null ? (
              <div className="form-helper-text">
                Notification configuration not available for this user (or not yet loaded).
              </div>
            ) : (
              <form onSubmit={saveNotificationConfig}>
                <div className="notification-config-grid">
                  <div className="detail-panel notification-config-card">
                    <h3>Routing defaults</h3>
                    <div className="form-group">
                      <label htmlFor="tenant-approver-group">Approver group</label>
                      <input
                        id="tenant-approver-group"
                        value={notifForm.approver_group}
                        onChange={e => setNotifForm({ ...notifForm, approver_group: e.target.value })}
                        placeholder="platform_admin or tenant_admin"
                      />
                      <div className="form-helper-text">
                        Controls which operator group is notified for new approval requests.
                      </div>
                    </div>
                    <div className="table-subtext">
                      Add one or both delivery channels below. Slack is easiest for demos, while webhooks are useful for external incident tooling.
                    </div>
                  </div>

                  <div className="notification-config-stack">
                    <div className="detail-panel notification-config-card">
                      <h3>Slack delivery</h3>
                      <div className="form-group">
                        <label htmlFor="tenant-slack-channel">Slack channel</label>
                        <input
                          id="tenant-slack-channel"
                          value={notifForm.slack_channel}
                          onChange={e => setNotifForm({ ...notifForm, slack_channel: e.target.value })}
                          placeholder="#team-alerts"
                        />
                        <div className="form-helper-text">Leave blank if this tenant should not send approval notifications to Slack.</div>
                      </div>
                    </div>

                    <div className="detail-panel notification-config-card">
                      <h3>Webhook delivery</h3>
                      <div className="form-grid form-grid-2">
                        <div className="form-group">
                          <label htmlFor="tenant-webhook-url">Webhook URL</label>
                          <input
                            id="tenant-webhook-url"
                            value={notifForm.webhook_url}
                            onChange={e => setNotifForm({ ...notifForm, webhook_url: e.target.value })}
                            placeholder="https://hooks.example.com/..."
                          />
                        </div>

                        <div className="form-group">
                          <label htmlFor="tenant-webhook-secret-ref">Webhook secret reference</label>
                          <input
                            id="tenant-webhook-secret-ref"
                            value={notifForm.webhook_secret_ref}
                            onChange={e => setNotifForm({ ...notifForm, webhook_secret_ref: e.target.value })}
                            placeholder="secret_ref name"
                          />
                        </div>
                      </div>
                      <div className="form-helper-text">Both fields are required together so OpenClause can sign outbound webhook payloads.</div>
                    </div>
                  </div>
                </div>
                <div className="form-actions-row">
                  <p className="form-helper-text">Changes affect newly created approvals and alert notifications for this tenant.</p>
                  <button className="btn btn-primary" disabled={savingNotif}>
                    {savingNotif ? 'Saving…' : 'Save notification config'}
                  </button>
                </div>
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
                {visibleApiKeys.length === 0 ? (
                  <tr><td colSpan={8} style={{ textAlign: 'center', padding: 24, color: '#94a3b8' }}>No API keys</td></tr>
                ) : (
                  visibleApiKeys.map(k => (
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
                      <td>
                        {k.last_used_at ? (
                          formatDate(k.last_used_at, 'date')
                        ) : (
                          <span style={{ display: 'inline-flex', gap: 8, alignItems: 'center' }}>
                            <span style={{ color: '#64748b' }}>—</span>
                            <span className="badge badge-gray">Never used</span>
                          </span>
                        )}
                      </td>
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
          <div className="section-title section-title-spacious">Alerts</div>

          {alertsError && <InlineErrorState message={alertsError} onRetry={() => void fetchAlerts()} />}

          <div className="form-card mt-16">
            <h3>Create deny_spike rule</h3>
            <p className="form-helper-text">
              Create a rule that fires when denies exceed a threshold inside a rolling time window.
            </p>
            <form onSubmit={createAlertRule}>
              <div className="form-grid alert-rule-form-grid">
                <div className="form-group">
                  <label htmlFor="tenant-alert-rule-name">Rule name</label>
                  <input
                    id="tenant-alert-rule-name"
                    value={alertRuleForm.name}
                    onChange={e => setAlertRuleForm(f => ({ ...f, name: e.target.value }))}
                    placeholder="e.g., Deny spike detector"
                    required
                  />
                </div>
                <div className="form-group">
                  <label htmlFor="tenant-alert-rule-threshold">N (denies)</label>
                  <input
                    id="tenant-alert-rule-threshold"
                    type="number"
                    value={alertRuleForm.n}
                    min={1}
                    onChange={e => setAlertRuleForm(f => ({ ...f, n: parseInt(e.target.value || '0', 10) || 1 }))}
                    required
                  />
                </div>
                <div className="form-group">
                  <label htmlFor="tenant-alert-rule-window">M (window minutes)</label>
                  <input
                    id="tenant-alert-rule-window"
                    type="number"
                    value={alertRuleForm.mMinutes}
                    min={1}
                    onChange={e => setAlertRuleForm(f => ({ ...f, mMinutes: parseInt(e.target.value || '0', 10) || 1 }))}
                    required
                  />
                </div>
                <div className="form-group alert-activation-field">
                  <label>Activation</label>
                  <label className="toggle-field toggle-field-boxed">
                    <input
                      type="checkbox"
                      checked={alertRuleForm.enabled}
                      onChange={e => setAlertRuleForm(f => ({ ...f, enabled: e.target.checked }))}
                    />
                    <span>{alertRuleForm.enabled ? 'Enabled immediately' : 'Save as disabled'}</span>
                  </label>
                </div>
                <div className="form-actions-row form-actions-row-end">
                  <button className="btn btn-primary" disabled={alertRuleSaving || alertsLoading}>
                    {alertRuleSaving ? 'Saving…' : 'Create'}
                  </button>
                </div>
              </div>
            </form>
          </div>

          <div className="section-title section-title-spacious">Alert Rules</div>
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
                  <tr><td colSpan={6} className="table-empty-state-cell"><EmptyState icon="⚠" title="No alert rules yet" description="Create a deny_spike rule to notify operators when a tenant starts hitting repeated policy denials." /></td></tr>
                ) : (
                  alertRules.map(r => (
                    <tr key={r.id}>
                      <td style={{ fontWeight: 600 }}>{r.name}</td>
                      <td>{r.config_json.n}</td>
                      <td>{r.config_json.m_minutes}</td>
                      <td>
                        {r.enabled ? (
                          <span className="badge badge-green">Active</span>
                        ) : (
                          <span className="badge badge-gray">Disabled</span>
                        )}
                      </td>
                      <td>{formatDate(r.updated_at, 'date')}</td>
                      <td className="tenant-alert-actions-cell">
                        <div className="row-actions row-actions-end">
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
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>

          {editingRuleId ? (
            <div className="modal-backdrop" onClick={() => setEditingRuleId(null)}>
              <div className="modal" onClick={event => event.stopPropagation()}>
                <div className="flex-between mb-16">
                  <div>
                    <h3>Edit alert rule</h3>
                    <p className="table-subtext">Update the rule name, threshold window, and activation state without changing the list layout.</p>
                  </div>
                  <button className="btn btn-outline btn-sm" type="button" onClick={() => setEditingRuleId(null)}>
                    Close
                  </button>
                </div>
                <div className="form-grid alert-rule-form-grid">
                  <div className="form-group">
                    <label htmlFor="tenant-edit-alert-rule-name">Rule name</label>
                    <input
                      id="tenant-edit-alert-rule-name"
                      value={editRuleForm.name}
                      onChange={e => setEditRuleForm(f => ({ ...f, name: e.target.value }))}
                      required
                    />
                  </div>
                  <div className="form-group">
                    <label htmlFor="tenant-edit-alert-rule-threshold">N (denies)</label>
                    <input
                      id="tenant-edit-alert-rule-threshold"
                      type="number"
                      min={1}
                      value={editRuleForm.n}
                      onChange={e => setEditRuleForm(f => ({ ...f, n: parseInt(e.target.value || '0', 10) || 1 }))}
                    />
                  </div>
                  <div className="form-group">
                    <label htmlFor="tenant-edit-alert-rule-window">M (window minutes)</label>
                    <input
                      id="tenant-edit-alert-rule-window"
                      type="number"
                      min={1}
                      value={editRuleForm.mMinutes}
                      onChange={e => setEditRuleForm(f => ({ ...f, mMinutes: parseInt(e.target.value || '0', 10) || 1 }))}
                    />
                  </div>
                  <div className="form-group alert-activation-field">
                    <label>Activation</label>
                    <label className="toggle-field toggle-field-boxed toggle-field-compact">
                      <input
                        type="checkbox"
                        checked={editRuleForm.enabled}
                        onChange={e => setEditRuleForm(f => ({ ...f, enabled: e.target.checked }))}
                      />
                      <span>{editRuleForm.enabled ? 'Enabled' : 'Disabled'}</span>
                    </label>
                  </div>
                </div>
                <div className="row-actions row-actions-end mt-16">
                  <button className="btn btn-outline" type="button" onClick={() => setEditingRuleId(null)}>
                    Cancel
                  </button>
                  <button className="btn btn-primary" type="button" onClick={saveEditRule} disabled={alertRuleSaving}>
                    {alertRuleSaving ? 'Saving…' : 'Save rule'}
                  </button>
                </div>
              </div>
            </div>
          ) : null}

          <div className="section-title section-title-spacious">Alert Events</div>
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
                  <tr><td colSpan={7} className="table-empty-state-cell"><EmptyState icon="⌁" title="No alert events yet" description="Triggered alert deliveries will appear here with retry state and delivery outcomes." /></td></tr>
                ) : (
                  alertEvents.map(ev => (
                    <tr key={ev.id}>
                      <td>
                        <div className="inline-value-copy">
                          <code className="mono" title={ev.rule_id}>{ev.rule_id.slice(0, 8)}…</code>
                          <CopyIconButton text={ev.rule_id} label="Alert rule ID" />
                        </div>
                      </td>
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
          <div className="section-title section-title-spacious">Analytics</div>

          <div className="form-card mt-16">
            <h3>Tenant analytics</h3>
            <div className="analytics-toolbar">
              <div className="form-group analytics-range-select">
                <label htmlFor="tenant-analytics-range">Range</label>
                <select id="tenant-analytics-range" value={analyticsRangeHours} onChange={(e) => setAnalyticsRangeHours(Number(e.target.value))}>
                  <option value={6}>Last 6 hours</option>
                  <option value={24}>Last 24 hours</option>
                  <option value={48}>Last 48 hours</option>
                  <option value={168}>Last 7 days</option>
                </select>
              </div>
              <div className="analytics-range-meta">
                <div style={{ fontSize: 13, color: '#64748b' }}>
                  Bucket: {analyticsBucketMinutes} min · Top agents: {analyticsTopAgents}
                </div>
                <div className="form-helper-text analytics-range-note">
                  Resolved UTC range:{' '}
                  {tenantAnalytics
                    ? `${formatUTCDateTime(tenantAnalytics.range_start)} → ${formatUTCDateTime(tenantAnalytics.range_end)}`
                    : `${analyticsRangeLabel(analyticsRangeHours)} (exact UTC bounds appear after refresh)`}
                </div>
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
            const hasMeaningfulTrendData = trend.length >= 2 && trend.some(bucket => bucket.total > 0)

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
                    {!hasMeaningfulTrendData ? (
                      <EmptyState
                        icon="◔"
                        title="Not enough data yet"
                        description="OpenClause needs a few tool calls in this range before the trend chart becomes useful."
                      />
                    ) : (
                      <>
                        <div className="trend-chart">
                          {trend.map((b, i) => (
                            <div key={i} className="trend-chart-bucket">
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
                        <div className="trend-legend">
                          <span><span className="trend-legend-chip trend-legend-allow" />Allow</span>
                          <span><span className="trend-legend-chip trend-legend-deny" />Deny</span>
                          <span><span className="trend-legend-chip trend-legend-approve" />Approve</span>
                        </div>
                        <div className="trend-range-labels">
                          <span>{formatDate(trend[0].bucket, 'date')}</span>
                          <span>{formatDate(trend[trend.length - 1].bucket, 'date')}</span>
                        </div>
                      </>
                    )}
                  </div>
                )}

                        <div className="detail-panel mt-16">
                          <h3>Risk Heatmap</h3>
                          <div className="table-subtext" style={{ marginBottom: 12 }}>
                            Darker cells mean more events landed at that decision/risk combination in the selected range.
                          </div>
                          <div className="table-container risk-heatmap-table" style={{ marginBottom: 0 }}>
                            <table>
                              <thead>
                                <tr>
                                  <th>Risk</th>
                                  <th className="col-num">Allow</th>
                                  <th className="col-num">Deny</th>
                                  <th className="col-num">Approve</th>
                                  <th className="col-num">Total</th>
                                </tr>
                              </thead>
                              <tbody>
                        {riskHeatmap.map(r => (
                          <tr key={r.risk_score}>
                            <td style={{ fontFamily: 'monospace' }}>{r.risk_score}</td>
                            <td className={r.allow_count === 0 ? 'heatmap-zero' : ''} style={{ background: r.allow_count === 0 ? undefined : `rgba(34,197,94,${alphaFor(r.allow_count)})` }}>{r.allow_count === 0 ? '—' : r.allow_count}</td>
                            <td className={r.deny_count === 0 ? 'heatmap-zero' : ''} style={{ background: r.deny_count === 0 ? undefined : `rgba(239,68,68,${alphaFor(r.deny_count)})` }}>{r.deny_count === 0 ? '—' : r.deny_count}</td>
                            <td className={r.approve_count === 0 ? 'heatmap-zero' : ''} style={{ background: r.approve_count === 0 ? undefined : `rgba(234,179,8,${alphaFor(r.approve_count)})` }}>{r.approve_count === 0 ? '—' : r.approve_count}</td>
                            <td style={{ color: '#334155' }}>{r.total === 0 ? '—' : r.total}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </div>

                <div className="section-title section-title-spacious">Per-Agent Breakdown</div>
                <div className="table-container">
                  <table>
                    <thead>
                      <tr>
                        <th>Agent</th>
                        <th className="col-num">Allow</th>
                        <th className="col-num">Deny</th>
                        <th className="col-num">Approve</th>
                        <th className="col-num">% of total</th>
                        <th className="col-num">Total</th>
                      </tr>
                    </thead>
                    <tbody>
                      {perAgent.length === 0 ? (
                        <tr>
                          <td colSpan={6} style={{ textAlign: 'center', padding: 24, color: '#94a3b8' }}>
                            No tool events in this range
                          </td>
                        </tr>
                      ) : (
                        perAgent.map(a => {
                          const share = totals.total_events > 0 ? (a.total / totals.total_events) * 100 : 0
                          return (
                          <tr key={a.agent_id}>
                            <td>
                              <div className="inline-value-copy">
                                <code className="mono" title={a.agent_id}>{a.agent_id.slice(0, 12)}…</code>
                                <CopyIconButton text={a.agent_id} label="Analytics agent ID" />
                              </div>
                            </td>
                            <td className="col-num">{a.allow_count}</td>
                            <td className="col-num">{a.deny_count}</td>
                            <td className="col-num">{a.approve_count}</td>
                            <td className="col-num">
                              <div className="analytics-share-cell">
                                <div className="analytics-share-bar">
                                  <span style={{ width: `${Math.max(share, 4)}%` }} />
                                </div>
                                <span>{share.toFixed(0)}%</span>
                              </div>
                            </td>
                            <td className="col-num">{a.total}</td>
                          </tr>
                        )})
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
            <div className="warn-banner mt-16">
              <div className="warn-banner-title">Dev bootstrap allowlists enabled</div>
              <div className="form-helper-text helper-text-warn">Approver authorization may allow env allowlists in addition to DB roles.</div>
            </div>
          )}

          <div className="section-title section-title-spacious">Approvers</div>

          <div className="form-card">
            <h3>Add Approver</h3>
            <form onSubmit={addApprover}>
              <div className="form-grid approver-form-grid">
                <div className="form-group">
                  <label htmlFor="tenant-approver-email">Email</label>
                  <input id="tenant-approver-email" value={approverEmail} onChange={e => setApproverEmail(e.target.value)} placeholder="name@company.com" />
                </div>
                <div className="form-group">
                  <label htmlFor="tenant-approver-slack-user-id">Slack user id (optional)</label>
                  <input id="tenant-approver-slack-user-id" value={approverSlackUserID} onChange={e => setApproverSlackUserID(e.target.value)} placeholder="U1234567890" />
                </div>
                <div className="form-group">
                  <label htmlFor="tenant-approver-name">Name (optional)</label>
                  <input id="tenant-approver-name" value={approverName} onChange={e => setApproverName(e.target.value)} placeholder="Full name" />
                </div>
                <div className="form-actions-row form-actions-row-end approver-form-actions">
                  <button className="btn btn-primary" disabled={creating}>Add approver</button>
                </div>
              </div>
              <div className="form-helper-text approver-form-note">
                Add an email to create or match a console user. A Slack user id on its own only links to an existing user.
              </div>
            </form>
          </div>

          <div className="table-container mt-16">
            <table>
              <thead>
                <tr>
                  <th>Email</th>
                  <th>Name</th>
                  <th>Slack user id</th>
                  <th>Link status</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {visibleApprovers.length === 0 ? (
                  <tr><td colSpan={5} className="table-empty-state-cell"><EmptyState icon="✓" title="No approvers yet" description="Add at least one approver so high-risk actions can be reviewed in the console or via notifications." /></td></tr>
                ) : (
                  visibleApprovers.map(a => {
                    const linkStatus = approverLinkStatus(a)
                    return (
                    <tr key={a.id}>
                      <td>{a.email}</td>
                      <td>{a.name || '—'}</td>
                      <td className="mono" title={a.slack_user_id || '—'}>{a.slack_user_id ? a.slack_user_id : '—'}</td>
                      <td><span className={`badge badge-${linkStatus.tone}`}>{linkStatus.label}</span></td>
                      <td>
                        <button className="btn btn-danger btn-sm" onClick={() => removeApprover(a.id)} disabled={creating}>
                          Remove
                        </button>
                      </td>
                    </tr>
                  )})
                )}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  )
}
