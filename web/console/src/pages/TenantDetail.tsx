import { useState, useEffect, FormEvent, useMemo, useRef } from 'react'
import { useParams, Link, useSearchParams } from 'react-router-dom'
import { APIClientError, api, formatDate } from '../api'
import { CopyIconButton, EmptyState, InlineErrorState, compareDate, downloadBlob } from '../ui'
import AgentOnboardingFlow from './AgentOnboardingFlow'

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

interface AgentIntegrationRecord {
  id: string
  tenant_id: string
  agent_id: string
  runtime: string
  environment_label?: string
  owner_name?: string
  description?: string
  approval_posture?: string
  created_at: string
  updated_at: string
  tools?: Array<{ tool: string; action: string }>
}

interface AgentIntegrationRevision {
  id: string
  integration_id: string
  tenant_id: string
  agent_id: string
  mode?: string
  runtime: string
  environment_label?: string
  owner_name?: string
  description?: string
  approval_posture?: string
  created_at: string
  tools?: Array<{ tool: string; action: string }>
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
  pilot_health: {
    status: string
    status_reason: string
    last_event?: {
      event_id: string
      agent_id: string
      tool: string
      action: string
      decision: string
      session_id: string
      trace_id: string
      received_at: string
    }
    last_session?: {
      session_id: string
      agent_id: string
      last_event_id: string
      last_event_at: string
    }
    last_approval?: {
      request_id: string
      event_id: string
      tool: string
      action: string
      status: string
      created_at: string
      resolved_at?: string
      latency_ms?: number
    }
    pending_approvals: number
    oldest_pending_approval_at?: string
    execution_success_count: number
    execution_total: number
    execution_success_rate: number
    missing_session_count: number
    missing_trace_count: number
    missing_session_rate: number
    missing_trace_rate: number
    top_connector_failures: Array<{
      tool: string
      action: string
      status: string
      error_message: string
      count: number
      last_seen_at: string
    }>
    top_deny_reasons: Array<{
      reason: string
      count: number
      last_seen_at: string
    }>
    next_actions: Array<{
      id: string
      title: string
      description: string
      path?: string
      severity?: string
    }>
  }
}

type TenantIssueSummary = {
  stage: string
  message: string
  requestId?: string
  code?: string
  status?: number
}

type TenantIssueHistoryEntry = TenantIssueSummary & {
  key: string
  count: number
  lastSeenAt: string
}

function repeatedTenantTriageNotice(stage: string, requestId?: string) {
  const stageLabel = stage.replace(/-/g, ' ')
  return `Repeated ${stageLabel} failures detected for this tenant. Check the latest request id${requestId ? ` (${requestId})` : ''} and browser console details before retrying.`
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

function runtimeLabel(runtime?: string) {
  switch (runtime) {
    case 'python':
      return 'Python'
    case 'typescript':
      return 'TypeScript'
    case 'langchain':
      return 'LangChain'
    case 'openai_local':
      return 'Local OpenAI model'
    default:
      return runtime || 'Unspecified runtime'
  }
}

function summarizeAgentIntegration(integration: AgentIntegrationRecord | null | undefined) {
  if (integration === undefined) return 'Loading saved setup summary…'
  if (!integration) return 'Bare agent record only. Connect this agent to save a runtime, starter tools, and approval posture.'
  const parts = [runtimeLabel(integration.runtime)]
  if (integration.environment_label) parts.push(integration.environment_label)
  if (integration.tools && integration.tools.length > 0) {
    parts.push(integration.tools.map(tool => `${tool.tool}:${tool.action}`).join(' · '))
  }
  return parts.join(' · ')
}

function summarizeIntegrationTools(tools?: Array<{ tool: string; action: string }>) {
  if (!tools || tools.length === 0) return 'No governed tools saved'
  return tools.map(tool => `${tool.tool}:${tool.action}`).join(' · ')
}

function integrationRevisionLabel(mode?: string) {
  switch (mode) {
    case 'created':
      return 'Connected'
    case 'regenerated':
      return 'Rebuilt last setup'
    case 'regenerated_defaults':
      return 'Rebuilt from safe defaults'
    default:
      return 'Saved'
  }
}

function bundleArchiveFilename(agentName: string, useDefaults: boolean) {
  const slug = agentName.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '') || 'agent'
  return useDefaults ? `openclause-${slug}-defaults.zip` : `openclause-${slug}.zip`
}

function pilotStatusBadge(status: string) {
  switch (status) {
    case 'healthy':
      return { label: 'Healthy pilot', className: 'badge badge-green' }
    case 'needs_attention':
      return { label: 'Needs attention', className: 'badge badge-yellow' }
    case 'setup_required':
      return { label: 'Setup required', className: 'badge badge-red' }
    default:
      return { label: 'Collecting baseline', className: 'badge badge-gray' }
  }
}

function formatPercent(value: number) {
  return `${Math.round(value * 100)}%`
}

function normalizeTenantNotificationConfig(payload: unknown): { config: TenantNotificationConfig; dropped: number } | null {
  if (!payload || typeof payload !== 'object' || Array.isArray(payload)) return null
  const candidate = payload as Partial<TenantNotificationConfig>
  if (candidate.approver_group !== undefined && typeof candidate.approver_group !== 'string') return null
  if (candidate.notify !== undefined && !Array.isArray(candidate.notify)) return null

  const rawNotify = Array.isArray(candidate.notify) ? candidate.notify : []
  const notify: NonNullable<TenantNotificationConfig['notify']> = []
  rawNotify.forEach((row) => {
    if (!row || typeof row !== 'object' || Array.isArray(row)) return
    const item = row as Record<string, unknown>
    if (typeof item.kind !== 'string' || !item.kind.trim()) return
    const kind = item.kind.trim().toLowerCase()
    if (kind === 'slack') {
      if (typeof item.channel !== 'string' || !item.channel.trim()) return
      notify.push({ kind: 'slack', channel: item.channel.trim() })
      return
    }
    if (kind === 'webhook') {
      if (typeof item.url !== 'string' || !item.url.trim()) return
      if (typeof item.secret_ref !== 'string' || !item.secret_ref.trim()) return
      notify.push({ kind: 'webhook', url: item.url.trim(), secret_ref: item.secret_ref.trim() })
      return
    }
  })

  return {
    config: {
      approver_group: candidate.approver_group || '',
      notify,
    },
    dropped: rawNotify.length - notify.length,
  }
}

function isAlertRule(value: unknown): value is AlertRule {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const candidate = value as Partial<AlertRule>
  return (
    typeof candidate.id === 'string' &&
    candidate.id.trim() !== '' &&
    typeof candidate.tenant_id === 'string' &&
    candidate.tenant_id.trim() !== '' &&
    typeof candidate.name === 'string' &&
    candidate.name.trim() !== '' &&
    typeof candidate.kind === 'string' &&
    candidate.kind.trim() !== '' &&
    typeof candidate.enabled === 'boolean' &&
    !!candidate.config_json &&
    typeof candidate.config_json.n === 'number' &&
    typeof candidate.config_json.m_minutes === 'number' &&
    typeof candidate.created_at === 'string' &&
    candidate.created_at.trim() !== '' &&
    typeof candidate.updated_at === 'string' &&
    candidate.updated_at.trim() !== ''
  )
}

function normalizeAlertRulesPayload(payload: unknown): { rules: AlertRule[]; dropped: number } | null {
  if (Array.isArray(payload)) {
    const rules = payload.filter(isAlertRule)
    return { rules, dropped: payload.length - rules.length }
  }
  if (!payload || typeof payload !== 'object') return null
  const wrapped = (payload as { rules?: unknown }).rules
  if (!Array.isArray(wrapped)) return null
  const rules = wrapped.filter(isAlertRule)
  return { rules, dropped: wrapped.length - rules.length }
}

function isAlertEvent(value: unknown): value is AlertEvent {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const candidate = value as Partial<AlertEvent>
  return (
    typeof candidate.id === 'string' &&
    candidate.id.trim() !== '' &&
    typeof candidate.rule_id === 'string' &&
    candidate.rule_id.trim() !== '' &&
    typeof candidate.tenant_id === 'string' &&
    candidate.tenant_id.trim() !== '' &&
    typeof candidate.severity === 'string' &&
    candidate.severity.trim() !== '' &&
    typeof candidate.message === 'string' &&
    candidate.message.trim() !== '' &&
    typeof candidate.status === 'string' &&
    candidate.status.trim() !== '' &&
    typeof candidate.created_at === 'string' &&
    candidate.created_at.trim() !== '' &&
    (candidate.attempt_count === undefined || typeof candidate.attempt_count === 'number') &&
    (candidate.next_attempt_at === undefined || typeof candidate.next_attempt_at === 'string') &&
    (candidate.last_error === undefined || typeof candidate.last_error === 'string')
  )
}

function normalizeAlertEventsPayload(payload: unknown): { events: AlertEvent[]; dropped: number } | null {
  if (Array.isArray(payload)) {
    const events = payload.filter(isAlertEvent)
    return { events, dropped: payload.length - events.length }
  }
  if (!payload || typeof payload !== 'object') return null
  const wrapped = (payload as { events?: unknown }).events
  if (!Array.isArray(wrapped)) return null
  const events = wrapped.filter(isAlertEvent)
  return { events, dropped: wrapped.length - events.length }
}

function integrationToOnboardingPreset(integration: AgentIntegrationRecord | null | undefined) {
  if (!integration) return null
  return {
    runtime: integration.runtime,
    environment_label: integration.environment_label,
    owner_name: integration.owner_name,
    description: integration.description,
    approval_posture: integration.approval_posture,
    updated_at: integration.updated_at,
    tools: integration.tools || [],
  }
}

function normalizeTenantAnalyticsSummary(payload: unknown): TenantAnalyticsSummary | null {
  if (!payload || typeof payload !== 'object' || Array.isArray(payload)) return null
  const candidate = payload as Partial<TenantAnalyticsSummary>
  const totals = candidate.totals as Partial<TenantAnalyticsTotals> | undefined
  if (
    !totals ||
    typeof totals.total_events !== 'number' ||
    typeof totals.allow_count !== 'number' ||
    typeof totals.deny_count !== 'number' ||
    typeof totals.approve_count !== 'number'
  ) {
    return null
  }

  const normalizeTrend = Array.isArray(candidate.trend)
    ? candidate.trend.filter((row): row is TenantAnalyticsTrendBucket => (
      !!row &&
      typeof row.bucket === 'string' &&
      typeof row.total === 'number' &&
      typeof row.allow_count === 'number' &&
      typeof row.deny_count === 'number' &&
      typeof row.approve_count === 'number'
    ))
    : []

  const normalizeRiskHeatmap = Array.isArray(candidate.risk_heatmap)
    ? candidate.risk_heatmap.filter((row): row is TenantRiskHeatmapRow => (
      !!row &&
      typeof row.risk_score === 'number' &&
      typeof row.allow_count === 'number' &&
      typeof row.deny_count === 'number' &&
      typeof row.approve_count === 'number' &&
      typeof row.total === 'number'
    ))
    : []

  const normalizePerAgent = Array.isArray(candidate.per_agent)
    ? candidate.per_agent.filter((row): row is TenantAgentBreakdownRow => (
      !!row &&
      typeof row.agent_id === 'string' &&
      typeof row.allow_count === 'number' &&
      typeof row.deny_count === 'number' &&
      typeof row.approve_count === 'number' &&
      typeof row.total === 'number'
    ))
    : []

  const onboarding = candidate.onboarding_checklist && typeof candidate.onboarding_checklist === 'object'
    ? candidate.onboarding_checklist
    : {}
  const pilotHealth = candidate.pilot_health && typeof candidate.pilot_health === 'object'
    ? candidate.pilot_health as Partial<TenantAnalyticsSummary['pilot_health']>
    : {}

  const normalizeTopConnectorFailures = Array.isArray(pilotHealth.top_connector_failures)
    ? pilotHealth.top_connector_failures.filter((row): row is TenantAnalyticsSummary['pilot_health']['top_connector_failures'][number] => (
      !!row &&
      typeof row.tool === 'string' &&
      typeof row.action === 'string' &&
      typeof row.status === 'string' &&
      typeof row.error_message === 'string' &&
      typeof row.count === 'number' &&
      typeof row.last_seen_at === 'string'
    ))
    : []

  const normalizeTopDenyReasons = Array.isArray(pilotHealth.top_deny_reasons)
    ? pilotHealth.top_deny_reasons.filter((row): row is TenantAnalyticsSummary['pilot_health']['top_deny_reasons'][number] => (
      !!row &&
      typeof row.reason === 'string' &&
      typeof row.count === 'number' &&
      typeof row.last_seen_at === 'string'
    ))
    : []

  const normalizeNextActions = Array.isArray(pilotHealth.next_actions)
    ? pilotHealth.next_actions.filter((row): row is TenantAnalyticsSummary['pilot_health']['next_actions'][number] => (
      !!row &&
      typeof row.id === 'string' &&
      typeof row.title === 'string' &&
      typeof row.description === 'string' &&
      (row.path === undefined || typeof row.path === 'string') &&
      (row.severity === undefined || typeof row.severity === 'string')
    ))
    : []

  return {
    range_start: typeof candidate.range_start === 'string' ? candidate.range_start : '',
    range_end: typeof candidate.range_end === 'string' ? candidate.range_end : '',
    totals: {
      total_events: totals.total_events,
      allow_count: totals.allow_count,
      deny_count: totals.deny_count,
      approve_count: totals.approve_count,
    },
    trend: normalizeTrend,
    risk_heatmap: normalizeRiskHeatmap,
    per_agent: normalizePerAgent,
    onboarding_checklist: {
      has_api_key: !!(onboarding as Partial<TenantOnboardingChecklist>).has_api_key,
      has_approver: !!(onboarding as Partial<TenantOnboardingChecklist>).has_approver,
      has_toolcall: !!(onboarding as Partial<TenantOnboardingChecklist>).has_toolcall,
      has_approval: !!(onboarding as Partial<TenantOnboardingChecklist>).has_approval,
      has_execution: !!(onboarding as Partial<TenantOnboardingChecklist>).has_execution,
    },
    pilot_health: {
      status: typeof pilotHealth.status === 'string' ? pilotHealth.status : '',
      status_reason: typeof pilotHealth.status_reason === 'string' ? pilotHealth.status_reason : '',
      last_event: pilotHealth.last_event && typeof pilotHealth.last_event === 'object' && !Array.isArray(pilotHealth.last_event)
        && typeof pilotHealth.last_event.event_id === 'string'
        && typeof pilotHealth.last_event.agent_id === 'string'
        && typeof pilotHealth.last_event.tool === 'string'
        && typeof pilotHealth.last_event.action === 'string'
        && typeof pilotHealth.last_event.decision === 'string'
        && typeof pilotHealth.last_event.session_id === 'string'
        && typeof pilotHealth.last_event.trace_id === 'string'
        && typeof pilotHealth.last_event.received_at === 'string'
        ? pilotHealth.last_event
        : undefined,
      last_session: pilotHealth.last_session && typeof pilotHealth.last_session === 'object' && !Array.isArray(pilotHealth.last_session)
        && typeof pilotHealth.last_session.session_id === 'string'
        && typeof pilotHealth.last_session.agent_id === 'string'
        && typeof pilotHealth.last_session.last_event_id === 'string'
        && typeof pilotHealth.last_session.last_event_at === 'string'
        ? pilotHealth.last_session
        : undefined,
      last_approval: pilotHealth.last_approval && typeof pilotHealth.last_approval === 'object' && !Array.isArray(pilotHealth.last_approval)
        && typeof pilotHealth.last_approval.request_id === 'string'
        && typeof pilotHealth.last_approval.event_id === 'string'
        && typeof pilotHealth.last_approval.tool === 'string'
        && typeof pilotHealth.last_approval.action === 'string'
        && typeof pilotHealth.last_approval.status === 'string'
        && typeof pilotHealth.last_approval.created_at === 'string'
        ? pilotHealth.last_approval
        : undefined,
      pending_approvals: typeof pilotHealth.pending_approvals === 'number' ? pilotHealth.pending_approvals : 0,
      oldest_pending_approval_at: typeof pilotHealth.oldest_pending_approval_at === 'string' ? pilotHealth.oldest_pending_approval_at : undefined,
      execution_success_count: typeof pilotHealth.execution_success_count === 'number' ? pilotHealth.execution_success_count : 0,
      execution_total: typeof pilotHealth.execution_total === 'number' ? pilotHealth.execution_total : 0,
      execution_success_rate: typeof pilotHealth.execution_success_rate === 'number' ? pilotHealth.execution_success_rate : 0,
      missing_session_count: typeof pilotHealth.missing_session_count === 'number' ? pilotHealth.missing_session_count : 0,
      missing_trace_count: typeof pilotHealth.missing_trace_count === 'number' ? pilotHealth.missing_trace_count : 0,
      missing_session_rate: typeof pilotHealth.missing_session_rate === 'number' ? pilotHealth.missing_session_rate : 0,
      missing_trace_rate: typeof pilotHealth.missing_trace_rate === 'number' ? pilotHealth.missing_trace_rate : 0,
      top_connector_failures: normalizeTopConnectorFailures,
      top_deny_reasons: normalizeTopDenyReasons,
      next_actions: normalizeNextActions,
    },
  }
}

export default function TenantDetail() {
  const { id } = useParams<{ id: string }>()
  const [searchParams, setSearchParams] = useSearchParams()
  const [tenant, setTenant] = useState<Tenant | null>(null)
  const [agents, setAgents] = useState<Agent[]>([])
  const [apiKeys, setApiKeys] = useState<ApiKey[]>([])
  const [approvers, setApprovers] = useState<Approver[]>([])
  const [agentIntegrations, setAgentIntegrations] = useState<Record<string, AgentIntegrationRecord | null>>({})
  const [agentIntegrationRevisions, setAgentIntegrationRevisions] = useState<Record<string, AgentIntegrationRevision[]>>({})
  const [integrationHistoryErrorByAgent, setIntegrationHistoryErrorByAgent] = useState<Record<string, string>>({})
  const [activeIntegrationAgentID, setActiveIntegrationAgentID] = useState<string | null>(null)
  const [integrationLoadingByAgent, setIntegrationLoadingByAgent] = useState<Record<string, boolean>>({})
  const [integrationDownloadByAgent, setIntegrationDownloadByAgent] = useState<Record<string, boolean>>({})
  const [onboardingOpen, setOnboardingOpen] = useState(false)
  const [onboardingAgent, setOnboardingAgent] = useState<Agent | null>(null)
  const [notificationConfig, setNotificationConfig] = useState<TenantNotificationConfig | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const fetchSeq = useRef(0)
  const alertsFetchSeq = useRef(0)
  const analyticsFetchSeq = useRef(0)
  const onboardingTenantNameRef = useRef('')
  const onboardingTenantStatusRef = useRef('')
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
  const [triageNotice, setTriageNotice] = useState('')
  const [latestIssue, setLatestIssue] = useState<TenantIssueSummary | null>(null)
  const [issueHistory, setIssueHistory] = useState<TenantIssueHistoryEntry[]>([])
  const issueCounts = useRef<Record<string, number>>({})

  function clearTenantIssues(...stages: string[]) {
    let changed = false
    stages.forEach(stage => {
      if (issueCounts.current[stage]) {
        delete issueCounts.current[stage]
        changed = true
      }
    })
    if (!changed) return

    const activeStages = new Set(Object.keys(issueCounts.current))
    let nextHistory: TenantIssueHistoryEntry[] = []
    setIssueHistory(current => {
      nextHistory = current.filter(entry => activeStages.has(entry.stage))
      return nextHistory
    })

    const remainingStages = Object.keys(issueCounts.current)
    if (remainingStages.length === 0) {
      setTriageNotice('')
      setLatestIssue(null)
      return
    }

    const freshest = [...nextHistory].sort((left, right) => right.lastSeenAt.localeCompare(left.lastSeenAt))[0]
    setLatestIssue(freshest ? { stage: freshest.stage, message: freshest.message, requestId: freshest.requestId } : null)

    const freshestRepeated = [...nextHistory]
      .filter(entry => (issueCounts.current[entry.stage] || 0) >= 2)
      .sort((left, right) => right.lastSeenAt.localeCompare(left.lastSeenAt))[0]
    setTriageNotice(freshestRepeated ? repeatedTenantTriageNotice(freshestRepeated.stage, freshestRepeated.requestId) : '')
  }

  function markTenantIssue(stage: string, requestId?: string) {
    const nextCount = (issueCounts.current[stage] || 0) + 1
    issueCounts.current[stage] = nextCount
    if (nextCount < 2) return

    setTriageNotice(repeatedTenantTriageNotice(stage, requestId))
  }

  function logTenantDetailIssue(stage: string, err: unknown, extra: Record<string, unknown> = {}) {
    const message = err instanceof Error ? err.message : String(err || 'Unknown tenant detail failure')
    const requestId = err instanceof Error && 'requestId' in err ? (err as { requestId?: string }).requestId : undefined
    const code = err instanceof Error && 'code' in err ? (err as { code?: string }).code : undefined
    const status = err instanceof Error && 'status' in err ? (err as { status?: number }).status : undefined
    const lastSeenAt = new Date().toISOString()
    markTenantIssue(stage, requestId)
    setLatestIssue({ stage, message, requestId, code, status })
    setIssueHistory(current => {
      const key = stage
      const existing = current.find(entry => entry.key === key)
      const next = existing
        ? current.map(entry => entry.key === key ? { ...entry, message, requestId, code, status, count: entry.count + 1, lastSeenAt } : entry)
        : [{ key, stage, message, requestId, code, status, count: 1, lastSeenAt }, ...current]
      return next
        .sort((left, right) => right.lastSeenAt.localeCompare(left.lastSeenAt))
        .slice(0, 5)
    })
    console.warn('[openclause-console] tenant detail issue', {
      tenantId: id,
      stage,
      requestId,
      code,
      status,
      message,
      ...extra,
    })
  }

  const tenantDiagnostics = useMemo(() => {
    if (!latestIssue) return ''
    const latestCount = issueHistory.find(entry => entry.stage === latestIssue.stage)?.count || 1
    const lines = [
      'OpenClause tenant detail diagnostics',
      `tenant_id=${id || ''}`,
      `active_tab=${activeTab}`,
      `stage=${latestIssue.stage}`,
      `request_id=${latestIssue.requestId || ''}`,
      `error_code=${latestIssue.code || ''}`,
      `status=${latestIssue.status ?? ''}`,
      `occurrences=${latestCount}`,
      `message=${latestIssue.message}`,
    ]
    if (issueHistory.length > 0) {
      lines.push('', 'active_issues:')
      issueHistory.forEach((entry, index) => {
        lines.push(
          `${index + 1}. stage=${entry.stage}; count=${entry.count}; request_id=${entry.requestId || ''}; error_code=${entry.code || ''}; status=${entry.status ?? ''}; last_seen=${entry.lastSeenAt}; message=${entry.message}`,
        )
      })
    }
    return lines.join('\n')
  }, [activeTab, id, issueHistory, latestIssue])

  const latestIssueCount = useMemo(
    () => (latestIssue ? issueHistory.find(entry => entry.stage === latestIssue.stage)?.count || 1 : 0),
    [issueHistory, latestIssue],
  )

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
  const onboardingPresetTenant = useMemo(() => {
    if (!id) return null
    return {
      id,
      name: tenant?.name || onboardingTenantNameRef.current || 'Current tenant',
      status: tenant?.status || onboardingTenantStatusRef.current || 'active',
    }
  }, [id, tenant?.name, tenant?.status])

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

  async function prefetchAgentIntegrations(agentList: Agent[]) {
    if (!id || agentList.length === 0) return
    const results = await Promise.all(agentList.map(async agent => {
      try {
        const integration = await api.get(`/admin/tenants/${id}/agents/${agent.id}/integration`) as AgentIntegrationRecord
        return { agentID: agent.id, integration }
      } catch (err) {
        if (err instanceof APIClientError && err.status === 404) {
          return { agentID: agent.id, integration: null }
        }
        return { agentID: agent.id, integration: undefined }
      }
    }))
    setAgentIntegrations(current => {
      const next = { ...current }
      results.forEach(result => {
        if (result.integration !== undefined) next[result.agentID] = result.integration
      })
      return next
    })
  }

  async function fetchAll() {
    const seq = ++fetchSeq.current
    setLoading(true)
    setError('')
    setTenant(null)
    setAgents([])
    setApiKeys([])
    setApprovers([])
    setAgentIntegrations({})
    setAgentIntegrationRevisions({})
    setIntegrationHistoryErrorByAgent({})
    setActiveIntegrationAgentID(null)
    setIntegrationLoadingByAgent({})
    setIntegrationDownloadByAgent({})
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
        const nextAgents = Array.isArray(agentsData) ? agentsData : agentsData?.agents || []
        setAgents(nextAgents)
        void prefetchAgentIntegrations(nextAgents)
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
        const normalizedConfig = normalizeTenantNotificationConfig(notifCfgResp.value)
        if (!normalizedConfig) {
          const malformedNotificationError = new Error('Notification configuration payload was malformed.')
          setNotificationConfig(null)
          setNotifError(malformedNotificationError.message)
          logTenantDetailIssue('notification-config-contract', malformedNotificationError)
        } else {
          const notifCfg = normalizedConfig.config
          setNotificationConfig(notifCfg)
          const slack = notifCfg.notify?.find((n: any) => n.kind === 'slack')
          const webhook = notifCfg.notify?.find((n: any) => n.kind === 'webhook')
          setNotifForm({
            approver_group: notifCfg.approver_group || '',
            slack_channel: slack?.channel || '',
            webhook_url: webhook?.url || '',
            webhook_secret_ref: webhook?.secret_ref || '',
          })
          if (normalizedConfig.dropped > 0) {
            const partialNotificationError = new Error('Some notification delivery entries were malformed and were ignored.')
            setNotifError(partialNotificationError.message)
            logTenantDetailIssue('notification-config-contract', partialNotificationError, { droppedRows: normalizedConfig.dropped })
          } else {
            setNotifError('')
            clearTenantIssues('notification-config', 'notification-config-contract')
          }
        }
      } else {
        setNotificationConfig(null)
        setNotifError(notifCfgResp.reason?.message || 'Failed to load notification config')
        logTenantDetailIssue('notification-config', notifCfgResp.reason)
      }

      if (partialFailures.length > 0) {
        logTenantDetailIssue('overview-partial', new Error(`Some tenant sections could not be loaded: ${partialFailures.join(', ')}.`), { sections: partialFailures })
        setError(`Some tenant sections could not be loaded: ${partialFailures.join(', ')}.`)
      } else {
        clearTenantIssues('overview', 'overview-partial')
      }
    } catch (err: any) {
      if (seq === fetchSeq.current) logTenantDetailIssue('overview', err)
      if (seq === fetchSeq.current) setError(err.message)
    } finally {
      if (seq === fetchSeq.current) setLoading(false)
    }
  }

  useEffect(() => { fetchAll() }, [id, hideDisabledAgents])

  useEffect(() => {
    if (!tenant) return
    onboardingTenantNameRef.current = tenant.name
    onboardingTenantStatusRef.current = tenant.status
  }, [tenant])

  async function openAgentIntegrationHistory(agent: Agent) {
    if (!id) return
    const isOpen = activeIntegrationAgentID === agent.id
    setActiveIntegrationAgentID(isOpen ? null : agent.id)
    if (isOpen) return

    setIntegrationLoadingByAgent(current => ({ ...current, [agent.id]: true }))
    setIntegrationHistoryErrorByAgent(current => {
      if (!(agent.id in current)) return current
      const next = { ...current }
      delete next[agent.id]
      return next
    })
    try {
      const [integrationResp, revisionsResp] = await Promise.all([
        api.get(`/admin/tenants/${id}/agents/${agent.id}/integration`),
        api.get(`/admin/tenants/${id}/agents/${agent.id}/integration/revisions?limit=5`),
      ])
      setAgentIntegrations(current => ({
        ...current,
        [agent.id]: integrationResp as AgentIntegrationRecord,
      }))
      const revisionsPayload = revisionsResp as { revisions?: AgentIntegrationRevision[] } | AgentIntegrationRevision[]
      setAgentIntegrationRevisions(current => ({
        ...current,
        [agent.id]: Array.isArray(revisionsPayload) ? revisionsPayload : revisionsPayload.revisions || [],
      }))
      clearTenantIssues('integration-history')
    } catch (err) {
      if (err instanceof APIClientError && err.status === 404) {
        setAgentIntegrations(current => ({
          ...current,
          [agent.id]: null,
        }))
        setAgentIntegrationRevisions(current => ({
          ...current,
          [agent.id]: [],
        }))
        setIntegrationHistoryErrorByAgent(current => {
          if (!(agent.id in current)) return current
          const next = { ...current }
          delete next[agent.id]
          return next
        })
        clearTenantIssues('integration-history')
        return
      }
      logTenantDetailIssue('integration-history', err, { agentId: agent.id })
      setIntegrationHistoryErrorByAgent(current => ({
        ...current,
        [agent.id]: err instanceof Error ? err.message : 'Failed to load saved setup history',
      }))
    } finally {
      setIntegrationLoadingByAgent(current => ({ ...current, [agent.id]: false }))
    }
  }

  async function openOnboardingForAgent(agent: Agent) {
    if (id && !(agent.id in agentIntegrations)) {
      setIntegrationLoadingByAgent(current => ({ ...current, [agent.id]: true }))
      try {
        await prefetchAgentIntegrations([agent])
      } finally {
        setIntegrationLoadingByAgent(current => ({ ...current, [agent.id]: false }))
      }
    }
    setOnboardingAgent(agent)
    setOnboardingOpen(true)
  }

  async function downloadAgentIntegrationBundle(agent: Agent, useDefaults: boolean) {
    if (!id) return
    setIntegrationDownloadByAgent(current => ({ ...current, [agent.id]: true }))
    try {
      const query = useDefaults ? '?defaults=true&archive=true' : '?archive=true'
      const blob = await api.getBlob(`/admin/tenants/${id}/agents/${agent.id}/integration/bundle${query}`)
      downloadBlob(blob, bundleArchiveFilename(agent.name, useDefaults))
      clearTenantIssues('integration-bundle')
    } catch (err) {
      if (err instanceof APIClientError && err.status === 404) {
        setAgentIntegrations(current => ({
          ...current,
          [agent.id]: null,
        }))
        setAgentIntegrationRevisions(current => ({
          ...current,
          [agent.id]: [],
        }))
        setIntegrationHistoryErrorByAgent(current => {
          if (!(agent.id in current)) return current
          const next = { ...current }
          delete next[agent.id]
          return next
        })
        const message = 'No saved setup files exist for this agent yet. Connect or rebuild this agent first.'
        setError(message)
        clearTenantIssues('integration-bundle')
        return
      }
      logTenantDetailIssue('integration-bundle', err, { agentId: agent.id, useDefaults })
      setError(err instanceof Error ? err.message : 'Failed to download saved setup files')
    } finally {
      setIntegrationDownloadByAgent(current => ({ ...current, [agent.id]: false }))
    }
  }

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
      const contractIssues: string[] = []
      if (rulesResp.status === 'fulfilled') {
        const normalizedRules = normalizeAlertRulesPayload(rulesResp.value)
        if (!normalizedRules) {
          setAlertRules([])
          contractIssues.push('rules payload')
        } else {
          setAlertRules(normalizedRules.rules)
          if (normalizedRules.dropped > 0) contractIssues.push(`${normalizedRules.dropped} malformed rule row${normalizedRules.dropped === 1 ? '' : 's'}`)
        }
      } else {
        setAlertRules([])
        failures.push('rules')
      }
      if (eventsResp.status === 'fulfilled') {
        const normalizedEvents = normalizeAlertEventsPayload(eventsResp.value)
        if (!normalizedEvents) {
          setAlertEvents([])
          contractIssues.push('events payload')
        } else {
          setAlertEvents(normalizedEvents.events)
          if (normalizedEvents.dropped > 0) contractIssues.push(`${normalizedEvents.dropped} malformed event row${normalizedEvents.dropped === 1 ? '' : 's'}`)
        }
      } else {
        setAlertEvents([])
        failures.push('events')
      }
      if (failures.length > 0) {
        logTenantDetailIssue('alerts-partial', new Error(`Some alert data could not be loaded: ${failures.join(', ')}.`), { sections: failures })
        setAlertsError(`Some alert data could not be loaded: ${failures.join(', ')}.`)
      } else if (contractIssues.length > 0) {
        const contractError = new Error(`Some alert data was malformed and was ignored: ${contractIssues.join(', ')}.`)
        logTenantDetailIssue('alerts-contract', contractError, { issues: contractIssues })
        setAlertsError(contractError.message)
      } else {
        clearTenantIssues('alerts', 'alerts-partial', 'alerts-contract')
      }
    } catch (err: any) {
      if (seq !== alertsFetchSeq.current) return
      logTenantDetailIssue('alerts', err)
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
      const normalizedSummary = normalizeTenantAnalyticsSummary(summary)
      if (!normalizedSummary) {
        const malformedSummaryError = new Error('Analytics summary payload was malformed.')
        logTenantDetailIssue('analytics-contract', malformedSummaryError, { rangeHours })
        setTenantAnalytics(null)
        setAnalyticsError(malformedSummaryError.message)
        return
      }
      setTenantAnalytics(normalizedSummary)
      clearTenantIssues('analytics', 'analytics-contract')
    } catch (err) {
      if (seq !== analyticsFetchSeq.current) return
      logTenantDetailIssue('analytics', err, { rangeHours: analyticsRangeHours })
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
      {triageNotice ? (
        <div className="warn-banner mb-16">
          <div className="warn-banner-header">
            <div className="warn-banner-title">Repeated failures detected</div>
            <CopyIconButton text={tenantDiagnostics} label="Tenant diagnostics" disabled={!tenantDiagnostics} />
          </div>
          <div className="form-helper-text helper-text-warn">{triageNotice}</div>
          {latestIssue ? (
            <>
              <div className="form-helper-text helper-text-warn">{latestIssue.message}</div>
              <div className="warn-banner-meta">
                <span>Latest stage: <code className="mono">{latestIssue.stage}</code></span>
                {latestIssue.requestId ? <span>Request ID: <code className="mono">{latestIssue.requestId}</code></span> : null}
                {latestIssue.code ? <span>Error code: <code className="mono">{latestIssue.code}</code></span> : null}
                {latestIssueCount > 0 ? <span>Occurrences: <code className="mono">{latestIssueCount}</code></span> : null}
              </div>
            </>
          ) : null}
          {issueHistory.length > 0 ? (
            <details className="warn-banner-history">
              <summary>Issue history ({issueHistory.length})</summary>
              <ul className="warn-banner-history-list">
                {issueHistory.map(entry => (
                  <li key={entry.key}>
                    <code className="mono">{entry.stage}</code>
                    <span>{entry.message}</span>
                    {entry.requestId ? <code className="mono">{entry.requestId}</code> : null}
                    {entry.code ? <code className="mono">{entry.code}</code> : null}
                    {entry.count > 1 ? <span className="badge badge-yellow">{entry.count}x</span> : null}
                  </li>
                ))}
              </ul>
            </details>
          ) : null}
        </div>
      ) : null}
      {!triageNotice && latestIssue && (error || notifError || alertsError || analyticsError) ? (
        <div className="warn-banner warn-banner-subtle mb-16">
          <div className="warn-banner-header">
            <div className="warn-banner-title">Latest diagnostics</div>
            <CopyIconButton text={tenantDiagnostics} label="Tenant diagnostics" disabled={!tenantDiagnostics} />
          </div>
          <div className="form-helper-text helper-text-warn">{latestIssue.message}</div>
          <div className="warn-banner-meta">
            <span>Latest stage: <code className="mono">{latestIssue.stage}</code></span>
            {latestIssue.requestId ? <span>Request ID: <code className="mono">{latestIssue.requestId}</code></span> : null}
            {latestIssue.code ? <span>Error code: <code className="mono">{latestIssue.code}</code></span> : null}
            {latestIssueCount > 0 ? <span>Occurrences: <code className="mono">{latestIssueCount}</code></span> : null}
          </div>
          {issueHistory.length > 0 ? (
            <details className="warn-banner-history">
              <summary>Issue history ({issueHistory.length})</summary>
              <ul className="warn-banner-history-list">
                {issueHistory.map(entry => (
                  <li key={entry.key}>
                    <code className="mono">{entry.stage}</code>
                    <span>{entry.message}</span>
                    {entry.requestId ? <code className="mono">{entry.requestId}</code> : null}
                    {entry.code ? <code className="mono">{entry.code}</code> : null}
                    {entry.count > 1 ? <span className="badge badge-yellow">{entry.count}x</span> : null}
                  </li>
                ))}
              </ul>
            </details>
          ) : null}
        </div>
      ) : null}

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
          <div className="section-title section-title-spacious section-title-with-action">
            <span>Agents</span>
            <button className="btn btn-outline btn-sm" type="button" onClick={() => setOnboardingOpen(true)}>
              Connect Agent
            </button>
          </div>
          <div className="form-card">
            <h3>Register Agent</h3>
            <div className="form-helper-text" style={{ marginBottom: 12 }}>
              Need a full starter handoff instead of a bare agent record? Use <strong>Connect Agent</strong> to create the agent, issue an API key, and generate the starter files in one flow.
            </div>
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
                  visibleAgents.map(a => {
                    const missingSavedIntegration = agentIntegrations[a.id] === null
                    const integrationSummary = agentIntegrations[a.id]
                    return (
                    <tr key={a.id}>
                      <td>
                        <div className="inline-value-copy">
                          <code className="mono" title={a.id}>{a.id.slice(0, 12)}…</code>
                          <CopyIconButton text={a.id} label="Agent ID" />
                        </div>
                      </td>
                      <td>
                        <div className="table-primary-cell">
                          <div className="table-primary">{a.name}</div>
                          <div className="table-subtext">{summarizeAgentIntegration(integrationSummary)}</div>
                        </div>
                      </td>
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
                        <button
                          className="btn btn-sm btn-outline"
                          type="button"
                          disabled={!!integrationDownloadByAgent[a.id] || missingSavedIntegration}
                          onClick={() => { void downloadAgentIntegrationBundle(a, false) }}
                        >
                          {integrationDownloadByAgent[a.id] ? 'Downloading…' : (missingSavedIntegration ? 'No saved files yet' : 'Download latest files')}
                        </button>
                        <button
                          className="btn btn-sm btn-outline"
                          type="button"
                          onClick={() => {
                            void openOnboardingForAgent(a)
                          }}
                        >
                          Rebuild setup
                        </button>
                        <button
                          className="btn btn-sm btn-outline"
                          type="button"
                          onClick={() => { void openAgentIntegrationHistory(a) }}
                        >
                          {activeIntegrationAgentID === a.id ? 'Hide saved setup' : 'Saved setup'}
                        </button>
                      </td>
                    </tr>
                  )})
                )}
              </tbody>
            </table>
          </div>

          {activeIntegrationAgentID ? (
            <div className="detail-panel mt-16">
              <div className="section-title section-title-with-action">
                <h3>Saved setup</h3>
                {(() => {
                  const agent = agents.find(candidate => candidate.id === activeIntegrationAgentID)
                  const missingSavedIntegration = agent ? agentIntegrations[agent.id] === null : false
                  return agent ? (
                    <div className="btn-group">
                      <button
                        className="btn btn-outline btn-sm"
                        type="button"
                        disabled={!!integrationDownloadByAgent[agent.id] || missingSavedIntegration}
                        onClick={() => { void downloadAgentIntegrationBundle(agent, false) }}
                      >
                        {integrationDownloadByAgent[agent.id] ? 'Downloading…' : (missingSavedIntegration ? 'No saved files yet' : 'Download current files')}
                      </button>
                      <button
                        className="btn btn-outline btn-sm"
                        type="button"
                        disabled={!!integrationDownloadByAgent[agent.id] || missingSavedIntegration}
                        onClick={() => { void downloadAgentIntegrationBundle(agent, true) }}
                      >
                        {missingSavedIntegration ? 'No safe-default files yet' : 'Download safe-default files'}
                      </button>
                    </div>
                  ) : null
                })()}
              </div>
              {integrationLoadingByAgent[activeIntegrationAgentID] ? (
                <div className="table-subtext">Loading the latest saved setup details…</div>
              ) : (() => {
                const integration = agentIntegrations[activeIntegrationAgentID]
                const revisions = agentIntegrationRevisions[activeIntegrationAgentID] || []
                const integrationError = integrationHistoryErrorByAgent[activeIntegrationAgentID]
                if (integrationError) {
                  return <InlineErrorState message={integrationError} />
                }
                if (!integration) {
                  return <div className="table-subtext">No saved setup exists for this agent yet. Connect or rebuild this agent first.</div>
                }
                return (
                  <>
                    <div className="table-subtext">
                      {runtimeLabel(integration.runtime)} · {integration.environment_label || 'unlabeled environment'} · {integration.approval_posture || 'tenant default posture'}
                    </div>
                    <div className="table-subtext mt-8">{summarizeIntegrationTools(integration.tools)}</div>
                    {integration.description ? <div className="table-subtext mt-8">{integration.description}</div> : null}
                    <div className="stats-grid mt-16">
                      <div className="stat-card">
                        <div className="stat-label">Owner</div>
                        <div className="stat-value">{integration.owner_name || '—'}</div>
                      </div>
                      <div className="stat-card">
                        <div className="stat-label">Saved</div>
                        <div className="stat-value">{formatDate(integration.updated_at)}</div>
                      </div>
                    </div>
                    <div className="mt-16">
                      <h3>Recent rebuilds</h3>
                      {revisions.length === 0 ? (
                        <div className="table-subtext">No saved rebuilds yet.</div>
                      ) : (
                        <table>
                          <thead>
                            <tr>
                              <th>When</th>
                              <th>Mode</th>
                              <th>Runtime</th>
                              <th>Tools</th>
                            </tr>
                          </thead>
                          <tbody>
                            {revisions.map(revision => (
                              <tr key={revision.id}>
                                <td>{formatDate(revision.created_at)}</td>
                                <td>{integrationRevisionLabel(revision.mode)}</td>
                                <td>{runtimeLabel(revision.runtime)}</td>
                                <td>{summarizeIntegrationTools(revision.tools)}</td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      )}
                    </div>
                  </>
                )
              })()}
            </div>
          ) : null}
        </>
      )}

      <AgentOnboardingFlow
        open={onboardingOpen}
        onClose={() => {
          setOnboardingOpen(false)
          setOnboardingAgent(null)
        }}
        presetTenant={onboardingPresetTenant}
        presetAgent={onboardingAgent ? {
          id: onboardingAgent.id,
          name: onboardingAgent.name,
          status: onboardingAgent.status,
          onboarding: integrationToOnboardingPreset(agentIntegrations[onboardingAgent.id]),
        } : null}
        onCreated={() => { void fetchAll() }}
      />

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
            const pilotHealth = tenantAnalytics.pilot_health

            const maxDecision = Math.max(...trend.flatMap(b => [b.allow_count, b.deny_count, b.approve_count]), 1)
            const riskMaxTotal = Math.max(...riskHeatmap.map(r => r.total), 1)
            const hasMeaningfulTrendData = trend.length >= 2 && trend.some(bucket => bucket.total > 0)
            const pilotStatus = pilotStatusBadge(pilotHealth.status)

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

                <div className="detail-panel mt-16">
                  <div className="flex-between" style={{ alignItems: 'center', gap: 12 }}>
                    <div>
                      <h3>Pilot cockpit</h3>
                      <div className="table-subtext">A plain-English health check for whether this tenant can run a real pilot and how operators should respond next.</div>
                    </div>
                    <span className={pilotStatus.className}>{pilotStatus.label}</span>
                  </div>
                  <div className="form-helper-text mt-16">{pilotHealth.status_reason || 'No pilot summary available yet.'}</div>

                  <div className="card-grid mt-16">
                    <div className="card">
                      <div className="card-label">Execution success</div>
                      <div className="card-value">{pilotHealth.execution_total > 0 ? formatPercent(pilotHealth.execution_success_rate) : '—'}</div>
                      <div className="stat-hint">{pilotHealth.execution_success_count} successful runs out of {pilotHealth.execution_total} results in this range</div>
                    </div>
                    <div className="card">
                      <div className="card-label">Missing session_id</div>
                      <div className="card-value">{pilotHealth.missing_session_count > 0 ? formatPercent(pilotHealth.missing_session_rate) : '0%'}</div>
                      <div className="stat-hint">{pilotHealth.missing_session_count} events missing session context</div>
                    </div>
                    <div className="card">
                      <div className="card-label">Missing trace_id</div>
                      <div className="card-value">{pilotHealth.missing_trace_count > 0 ? formatPercent(pilotHealth.missing_trace_rate) : '0%'}</div>
                      <div className="stat-hint">{pilotHealth.missing_trace_count} events missing trace context</div>
                    </div>
                    <div className="card">
                      <div className="card-label">Pending approvals</div>
                      <div className="card-value">{pilotHealth.pending_approvals.toLocaleString()}</div>
                      <div className="stat-hint">
                        {pilotHealth.oldest_pending_approval_at
                          ? `Oldest pending approval: ${formatUTCDateTime(pilotHealth.oldest_pending_approval_at)}`
                          : 'No pending approvals right now'}
                      </div>
                    </div>
                  </div>

                  <div className="form-card mt-16">
                    <h3>What happened most recently</h3>
                    <div style={{ display: 'grid', gap: 12 }}>
                      <div className="detail-row" style={{ borderBottom: 'none', padding: 0 }}>
                        <div className="detail-label" style={{ minWidth: 240, color: '#64748b', fontWeight: 600 }}>Last governed event</div>
                        <div className="detail-value">
                          {pilotHealth.last_event ? (
                            <>
                              <div><strong>{pilotHealth.last_event.tool}:{pilotHealth.last_event.action}</strong> · <span className={`badge ${pilotHealth.last_event.decision === 'allow' ? 'badge-green' : pilotHealth.last_event.decision === 'deny' ? 'badge-red' : 'badge-yellow'}`}>{pilotHealth.last_event.decision}</span></div>
                              <div className="table-subtext">Agent {pilotHealth.last_event.agent_id} · {formatUTCDateTime(pilotHealth.last_event.received_at)}</div>
                              <div className="table-subtext">session_id: <code className="mono">{pilotHealth.last_event.session_id || '(missing)'}</code> · trace_id: <code className="mono">{pilotHealth.last_event.trace_id || '(missing)'}</code></div>
                            </>
                          ) : 'No governed events in the selected range yet.'}
                        </div>
                      </div>
                      <div className="detail-row" style={{ borderBottom: 'none', padding: 0 }}>
                        <div className="detail-label" style={{ minWidth: 240, color: '#64748b', fontWeight: 600 }}>Last session seen</div>
                        <div className="detail-value">
                          {pilotHealth.last_session ? (
                            <>
                              <div><code className="mono">{pilotHealth.last_session.session_id}</code></div>
                              <div className="table-subtext">Agent {pilotHealth.last_session.agent_id} · Last event at {formatUTCDateTime(pilotHealth.last_session.last_event_at)}</div>
                            </>
                          ) : 'No session_id has been observed yet. Run the smoke test and make sure your agent sends session_id on every governed call.'}
                        </div>
                      </div>
                      <div className="detail-row" style={{ borderBottom: 'none', padding: 0 }}>
                        <div className="detail-label" style={{ minWidth: 240, color: '#64748b', fontWeight: 600 }}>Last approval</div>
                        <div className="detail-value">
                          {pilotHealth.last_approval ? (
                            <>
                              <div><strong>{pilotHealth.last_approval.tool}:{pilotHealth.last_approval.action}</strong> · <span className={`badge ${pilotHealth.last_approval.status === 'approved' ? 'badge-green' : pilotHealth.last_approval.status === 'denied' ? 'badge-red' : 'badge-yellow'}`}>{pilotHealth.last_approval.status}</span></div>
                              <div className="table-subtext">Created {formatUTCDateTime(pilotHealth.last_approval.created_at)}{pilotHealth.last_approval.resolved_at ? ` · Resolved ${formatUTCDateTime(pilotHealth.last_approval.resolved_at)}` : ''}</div>
                              <div className="table-subtext">{pilotHealth.last_approval.latency_ms != null ? `Approval latency: ${Math.round(pilotHealth.last_approval.latency_ms / 1000)}s` : 'Approval is still waiting on action or did not record a latency yet.'}</div>
                            </>
                          ) : 'No approval requests in the selected range yet.'}
                        </div>
                      </div>
                    </div>
                  </div>

                  <div className="form-card mt-16">
                    <h3>Next best actions</h3>
                    <div style={{ display: 'grid', gap: 12 }}>
                      {pilotHealth.next_actions.length === 0 ? (
                        <div className="table-subtext">OpenClause does not see an immediate blocker right now.</div>
                      ) : (
                        pilotHealth.next_actions.map(action => (
                          <div key={action.id} className="detail-row" style={{ borderBottom: 'none', padding: 0, alignItems: 'flex-start' }}>
                            <div className="detail-label" style={{ minWidth: 240, color: '#64748b', fontWeight: 600 }}>
                              <span className={`badge ${action.severity === 'high' ? 'badge-red' : action.severity === 'medium' ? 'badge-yellow' : 'badge-blue'}`}>{action.severity || 'info'}</span>
                            </div>
                            <div className="detail-value">
                              <div><strong>{action.title}</strong></div>
                              <div className="table-subtext">{action.description}</div>
                              {action.path ? (
                                <div className="mt-8">
                                  <Link to={action.path} className="btn btn-outline btn-sm">Open</Link>
                                </div>
                              ) : null}
                            </div>
                          </div>
                        ))
                      )}
                    </div>
                  </div>

                  {(pilotHealth.top_connector_failures.length > 0 || pilotHealth.top_deny_reasons.length > 0) ? (
                    <div className="form-card mt-16">
                      <h3>Operator diagnostics</h3>
                      {pilotHealth.top_connector_failures.length > 0 ? (
                        <div style={{ marginBottom: pilotHealth.top_deny_reasons.length > 0 ? 16 : 0 }}>
                          <div className="form-helper-text">Top connector failures in the selected range.</div>
                          <ul className="onboarding-checklist mt-16">
                            {pilotHealth.top_connector_failures.map(failure => (
                              <li key={`${failure.tool}-${failure.action}-${failure.error_message}`}>
                                <strong>{failure.tool}:{failure.action}</strong> failed {failure.count} times ({failure.status}). Latest: {failure.error_message}
                              </li>
                            ))}
                          </ul>
                        </div>
                      ) : null}
                      {pilotHealth.top_deny_reasons.length > 0 ? (
                        <div>
                          <div className="form-helper-text">Top deny reasons in the selected range.</div>
                          <ul className="onboarding-checklist mt-16">
                            {pilotHealth.top_deny_reasons.map(reason => (
                              <li key={reason.reason}>
                                <strong>{reason.reason}</strong> · {reason.count} denies
                              </li>
                            ))}
                          </ul>
                        </div>
                      ) : null}
                    </div>
                  ) : null}
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
