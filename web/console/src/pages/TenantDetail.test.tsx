import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import TenantDetail from './TenantDetail'
import { renderRoute } from '../test/render'
import { mockApiGet } from '../test/mockApi'

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

type TenantDetailState = {
  tenant: {
    id: string
    name: string
    status: string
    config: Record<string, unknown>
    created_at: string
  }
  agents: Array<{
    id: string
    name: string
    tenant_id: string
    status: 'active' | 'disabled'
    created_at: string
  }>
  apiKeys: Array<{
    id: string
    key_prefix: string
    name: string
    status: string
    created_at: string
    expires_at?: string | null
    last_used_at?: string | null
    is_primary: boolean
  }>
  approvers: Array<{
    id: string
    email: string
    name: string
    slack_user_id?: string | null
  }>
  notificationConfig: {
    approver_group?: string
    notify?: Array<{ kind: string; url?: string; secret_ref?: string; channel?: string }>
  }
  alertRules: Array<{
    id: string
    tenant_id: string
    name: string
    kind: string
    enabled: boolean
    config_json: { n: number; m_minutes: number }
    created_at: string
    updated_at: string
  }>
  alertEvents: Array<{
    id: string
    rule_id: string
    tenant_id: string
    severity: string
    message: string
    status: string
    delivered_at?: string
    attempt_count?: number
    next_attempt_at?: string
    last_error?: string
    created_at: string
  }>
  analytics: {
    range_start: string
    range_end: string
    totals: { total_events: number; allow_count: number; deny_count: number; approve_count: number }
    trend: Array<{ bucket: string; total: number; allow_count: number; deny_count: number; approve_count: number }>
    risk_heatmap: Array<{ risk_score: number; allow_count: number; deny_count: number; approve_count: number; total: number }>
    per_agent: Array<{ agent_id: string; allow_count: number; deny_count: number; approve_count: number; total: number }>
    onboarding_checklist: {
      has_api_key: boolean
      has_approver: boolean
      has_toolcall: boolean
      has_approval: boolean
      has_execution: boolean
    }
  }
}

function getCardByHeading(headingText: RegExp | string) {
  const heading = screen.getByRole('heading', { name: headingText })
  const card = heading.closest('.form-card')
  if (!card) throw new Error(`No form card found for ${String(headingText)}`)
  return card as HTMLElement
}

function createTenantDetailState(overrides: Partial<TenantDetailState> = {}): TenantDetailState {
  return {
    tenant: {
      id: 'tenant-1',
      name: 'Tenant One',
      status: 'active',
      config: {},
      created_at: '2026-03-20T12:00:00Z',
    },
    agents: [
      { id: 'agent-active', name: 'Agent Active', tenant_id: 'tenant-1', status: 'active', created_at: '2026-03-22T12:00:00Z' },
      { id: 'agent-disabled', name: 'Agent Disabled', tenant_id: 'tenant-1', status: 'disabled', created_at: '2026-03-21T12:00:00Z' },
    ],
    apiKeys: [
      {
        id: 'key-primary',
        key_prefix: 'sk-oc-primary',
        name: 'Primary key',
        status: 'active',
        created_at: '2026-03-20T12:00:00Z',
        expires_at: null,
        last_used_at: '2026-03-22T12:00:00Z',
        is_primary: true,
      },
    ],
    approvers: [
      { id: 'approver-existing', email: 'approver@example.com', name: 'Existing Approver', slack_user_id: null },
    ],
    notificationConfig: { approver_group: '', notify: [] },
    alertRules: [
      {
        id: 'rule-1',
        tenant_id: 'tenant-1',
        name: 'Existing deny spike',
        kind: 'deny_spike',
        enabled: true,
        config_json: { n: 3, m_minutes: 5 },
        created_at: '2026-03-22T12:00:00Z',
        updated_at: '2026-03-22T12:00:00Z',
      },
    ],
    alertEvents: [
      {
        id: 'alert-event-1',
        rule_id: 'rule-1',
        tenant_id: 'tenant-1',
        severity: 'warning',
        message: 'Three denies in five minutes',
        status: 'pending',
        attempt_count: 1,
        next_attempt_at: '2026-03-23T13:00:00Z',
        created_at: '2026-03-23T12:00:00Z',
      },
    ],
    analytics: {
      range_start: '2026-03-22T12:00:00Z',
      range_end: '2026-03-23T12:00:00Z',
      totals: { total_events: 8, allow_count: 5, deny_count: 2, approve_count: 1 },
      trend: [],
      risk_heatmap: [],
      per_agent: [],
      onboarding_checklist: {
        has_api_key: true,
        has_approver: true,
        has_toolcall: true,
        has_approval: false,
        has_execution: false,
      },
    },
    ...overrides,
  }
}

function installTenantDetailApi(overrides: Partial<TenantDetailState> = {}) {
  const state = createTenantDetailState(overrides)

  const getSpy = mockApiGet([
    ['/admin/tenants/tenant-1', () => state.tenant],
    [(path) => path === '/admin/tenants/tenant-1/agents?include_disabled=true', () => ({ agents: state.agents })],
    [(path) => path === '/admin/tenants/tenant-1/agents?include_disabled=false', () => ({ agents: state.agents.filter(agent => agent.status !== 'disabled') })],
    ['/admin/tenants/tenant-1/apikeys', () => ({ api_keys: state.apiKeys })],
    ['/admin/tenants/tenant-1/approvers', () => ({ approvers: state.approvers, allowlist_source: 'db' })],
    ['/admin/tenants/tenant-1/notification-config', () => state.notificationConfig],
    [(path) => path === '/admin/tenants/tenant-1/alerts/rules', () => state.alertRules],
    [(path) => path.startsWith('/admin/tenants/tenant-1/alerts/events?'), () => state.alertEvents],
    [(path) => path.startsWith('/admin/tenants/tenant-1/analytics/summary'), () => state.analytics],
  ])

  const postSpy = vi.spyOn(api, 'post').mockImplementation(async (path, payload) => {
    if (path === '/admin/tenants/tenant-1/agents') {
      const body = payload as { name: string }
      state.agents = [
        ...state.agents,
        {
          id: `agent-${state.agents.length + 1}`,
          name: body.name,
          tenant_id: 'tenant-1',
          status: 'active',
          created_at: '2026-03-23T13:00:00Z',
        },
      ]
      return {}
    }

    const agentStatusMatch = path.match(/^\/admin\/tenants\/tenant-1\/agents\/([^/]+)\/status$/)
    if (agentStatusMatch) {
      const [, agentId] = agentStatusMatch
      const body = payload as { status: 'active' | 'disabled' }
      state.agents = state.agents.map(agent => agent.id === agentId ? { ...agent, status: body.status } : agent)
      return {}
    }

    if (path === '/admin/tenants/tenant-1/status') {
      const body = payload as { status: string }
      state.tenant = { ...state.tenant, status: body.status }
      return {}
    }

    if (path === '/admin/tenants/tenant-1/apikeys') {
      const body = payload as { name: string }
      state.apiKeys = [
        ...state.apiKeys,
        {
          id: `key-${state.apiKeys.length + 1}`,
          key_prefix: 'sk-oc-created',
          name: body.name,
          status: 'active',
          created_at: '2026-03-23T13:05:00Z',
          expires_at: null,
          last_used_at: null,
          is_primary: false,
        },
      ]
      return { raw_key: 'sk-oc-created-raw' }
    }

    if (path === '/admin/tenants/tenant-1/apikeys/rotate') {
      const body = payload as { name: string; make_primary: boolean; revoke_old_primary: boolean; expires_at?: string }
      const currentPrimary = state.apiKeys.find(key => key.is_primary && key.status !== 'revoked')
      if (currentPrimary && body.make_primary) {
        state.apiKeys = state.apiKeys.map(key => key.id === currentPrimary.id ? { ...key, is_primary: false } : key)
      }
      if (currentPrimary && body.revoke_old_primary) {
        state.apiKeys = state.apiKeys.map(key => key.id === currentPrimary.id ? { ...key, status: 'revoked', is_primary: false } : key)
      }
      state.apiKeys = [
        ...state.apiKeys,
        {
          id: `key-${state.apiKeys.length + 1}`,
          key_prefix: 'sk-oc-rotated',
          name: body.name,
          status: 'active',
          created_at: '2026-03-23T13:15:00Z',
          expires_at: body.expires_at ?? null,
          last_used_at: null,
          is_primary: body.make_primary,
        },
      ]
      return { raw_key: 'sk-oc-rotated-raw' }
    }

    const revokeKeyMatch = path.match(/^\/admin\/tenants\/tenant-1\/apikeys\/([^/]+)\/revoke$/)
    if (revokeKeyMatch) {
      const [, keyId] = revokeKeyMatch
      state.apiKeys = state.apiKeys.map(key => key.id === keyId ? { ...key, status: 'revoked', is_primary: false } : key)
      return {}
    }

    if (path === '/admin/tenants/tenant-1/approvers') {
      const body = payload as { email?: string; slack_user_id?: string; name?: string }
      state.approvers = [
        ...state.approvers,
        {
          id: `approver-${state.approvers.length + 1}`,
          email: body.email || '',
          name: body.name || '',
          slack_user_id: body.slack_user_id || null,
        },
      ]
      return {}
    }

    if (path === '/admin/tenants/tenant-1/alerts/rules') {
      const body = payload as { name: string; kind: string; enabled: boolean; config_json: { n: number; m_minutes: number } }
      state.alertRules = [
        ...state.alertRules,
        {
          id: `rule-${state.alertRules.length + 1}`,
          tenant_id: 'tenant-1',
          name: body.name,
          kind: body.kind,
          enabled: body.enabled,
          config_json: body.config_json,
          created_at: '2026-03-23T13:20:00Z',
          updated_at: '2026-03-23T13:20:00Z',
        },
      ]
      return {}
    }

    throw new Error(`Unhandled api.post call for ${path}`)
  })

  const putSpy = vi.spyOn(api, 'put').mockImplementation(async (path, payload) => {
    if (path === '/admin/tenants/tenant-1/notification-config') {
      state.notificationConfig = payload as TenantDetailState['notificationConfig']
      return {}
    }

    const updateRuleMatch = path.match(/^\/admin\/tenants\/tenant-1\/alerts\/rules\/([^/]+)$/)
    if (updateRuleMatch) {
      const [, ruleId] = updateRuleMatch
      const body = payload as { name: string; enabled: boolean; config_json: { n: number; m_minutes: number } }
      state.alertRules = state.alertRules.map(rule => (
        rule.id === ruleId
          ? {
              ...rule,
              name: body.name,
              enabled: body.enabled,
              config_json: body.config_json,
              updated_at: '2026-03-23T13:30:00Z',
            }
          : rule
      ))
      return {}
    }

    throw new Error(`Unhandled api.put call for ${path}`)
  })

  const deleteSpy = vi.spyOn(api, 'delete').mockImplementation(async (path) => {
    const approverMatch = path.match(/^\/admin\/tenants\/tenant-1\/approvers\/([^/]+)$/)
    if (approverMatch) {
      const [, approverId] = approverMatch
      state.approvers = state.approvers.filter(approver => approver.id !== approverId)
      return {}
    }

    const ruleMatch = path.match(/^\/admin\/tenants\/tenant-1\/alerts\/rules\/([^/]+)$/)
    if (ruleMatch) {
      const [, ruleId] = ruleMatch
      state.alertRules = state.alertRules.filter(rule => rule.id !== ruleId)
      return {}
    }

    throw new Error(`Unhandled api.delete call for ${path}`)
  })

  return { state, getSpy, postSpy, putSpy, deleteSpy }
}

describe('Tenant detail page', () => {
  it('hides disabled agents on demand and refetches the scoped list', async () => {
    const user = userEvent.setup()
    const { getSpy } = installTenantDetailApi()

    renderRoute(<TenantDetail />, { path: '/tenants/:id', route: '/tenants/tenant-1' })

    expect(await screen.findByText('Agent Active')).toBeInTheDocument()
    expect(screen.getByText('Agent Disabled')).toBeInTheDocument()

    await user.click(screen.getByRole('checkbox', { name: /hide disabled/i }))

    await waitFor(() => expect(getSpy).toHaveBeenCalledWith('/admin/tenants/tenant-1/agents?include_disabled=false'))
    await waitFor(() => expect(screen.queryByText('Agent Disabled')).not.toBeInTheDocument())
  })

  it('creates agents and toggles both agent and tenant status', async () => {
    const user = userEvent.setup()
    const { postSpy } = installTenantDetailApi()

    renderRoute(<TenantDetail />, { path: '/tenants/:id', route: '/tenants/tenant-1' })

    expect(await screen.findByText('Agent Active')).toBeInTheDocument()

    await user.type(screen.getByLabelText(/^agent name$/i), 'Agent Fresh')
    await user.click(screen.getByRole('button', { name: /^create$/i }))

    await waitFor(() => expect(postSpy).toHaveBeenCalledWith('/admin/tenants/tenant-1/agents', { name: 'Agent Fresh' }))
    expect(await screen.findByText('Agent Fresh')).toBeInTheDocument()

    const activeRow = screen.getByText('Agent Active').closest('tr')
    expect(activeRow).not.toBeNull()
    await user.click(within(activeRow as HTMLElement).getByRole('button', { name: /disable/i }))

    await waitFor(() => expect(postSpy).toHaveBeenCalledWith('/admin/tenants/tenant-1/agents/agent-active/status', { status: 'disabled' }))
    await waitFor(() => expect(within(screen.getByText('Agent Active').closest('tr') as HTMLElement).getByText('disabled')).toBeInTheDocument())

    await user.click(screen.getByRole('button', { name: /disable tenant/i }))

    await waitFor(() => expect(postSpy).toHaveBeenCalledWith('/admin/tenants/tenant-1/status', { status: 'disabled' }))
    expect(await screen.findByRole('button', { name: /enable tenant/i })).toBeInTheDocument()
  })

  it('creates, rotates, and revokes API keys while showing one-time raw values', async () => {
    const user = userEvent.setup()
    const { postSpy } = installTenantDetailApi()

    renderRoute(<TenantDetail />, { path: '/tenants/:id', route: '/tenants/tenant-1?tab=api_keys' })

    expect(await screen.findByRole('heading', { name: /create api key/i })).toBeInTheDocument()

    const createCard = getCardByHeading(/create api key/i)
    await user.type(within(createCard).getByLabelText(/^name$/i), 'Read only key')
    await user.click(within(createCard).getByRole('button', { name: /^create$/i }))

    await waitFor(() => expect(postSpy).toHaveBeenCalledWith('/admin/tenants/tenant-1/apikeys', { name: 'Read only key' }))
    expect(await screen.findByText(/copy this key now/i)).toBeInTheDocument()
    expect(screen.getByText('sk-oc-created-raw')).toBeInTheDocument()
    expect(screen.getByText('Read only key')).toBeInTheDocument()

    const rotateCard = getCardByHeading(/rotate primary key/i)
    await user.type(within(rotateCard).getByLabelText(/^new key name$/i), 'Rotated primary key')
    await user.type(within(rotateCard).getByLabelText(/^expires on/i), '2030-01-01')
    await user.click(within(rotateCard).getByRole('checkbox', { name: /revoke the old primary after rotation/i }))
    await user.click(within(rotateCard).getByRole('button', { name: /^rotate$/i }))

    await waitFor(() =>
      expect(postSpy).toHaveBeenCalledWith('/admin/tenants/tenant-1/apikeys/rotate', {
        name: 'Rotated primary key',
        make_primary: true,
        revoke_old_primary: false,
        expires_at: '2030-01-01',
      }),
    )

    expect(await screen.findByText(/copy this rotated key now/i)).toBeInTheDocument()
    expect(screen.getByText('sk-oc-rotated-raw')).toBeInTheDocument()

    const rotatedRow = screen.getByText('Rotated primary key').closest('tr')
    expect(rotatedRow).not.toBeNull()
    expect(within(rotatedRow as HTMLElement).getByText('Primary')).toBeInTheDocument()

    const createdKeyRow = screen.getByText('Read only key').closest('tr')
    expect(createdKeyRow).not.toBeNull()
    await user.click(within(createdKeyRow as HTMLElement).getByRole('button', { name: /revoke/i }))

    await waitFor(() => expect(postSpy).toHaveBeenCalledWith('/admin/tenants/tenant-1/apikeys/key-2/revoke'))
    expect(await within(screen.getByText('Read only key').closest('tr') as HTMLElement).findByText('Revoked')).toBeInTheDocument()
  })

  it('validates notification config and then adds and removes approvers', async () => {
    const user = userEvent.setup()
    const { postSpy, putSpy, deleteSpy } = installTenantDetailApi()

    renderRoute(<TenantDetail />, { path: '/tenants/:id', route: '/tenants/tenant-1?tab=api_keys' })

    expect(await screen.findByRole('heading', { name: /approval notifications/i })).toBeInTheDocument()

    await user.type(screen.getByLabelText(/^webhook url$/i), 'https://hooks.example.test/alerts')
    await user.click(screen.getByRole('button', { name: /save notification config/i }))

    expect(await screen.findByText(/webhook secret reference is required/i)).toBeInTheDocument()
    expect(putSpy).not.toHaveBeenCalled()

    await user.type(screen.getByLabelText(/^approver group$/i), 'tenant_admin')
    await user.type(screen.getByLabelText(/^slack channel$/i), '#tenant-alerts')
    await user.type(screen.getByLabelText(/^webhook secret reference$/i), 'tenant-alert-secret')
    await user.click(screen.getByRole('button', { name: /save notification config/i }))

    await waitFor(() =>
      expect(putSpy).toHaveBeenCalledWith('/admin/tenants/tenant-1/notification-config', {
        approver_group: 'tenant_admin',
        notify: [
          { kind: 'slack', channel: '#tenant-alerts' },
          { kind: 'webhook', url: 'https://hooks.example.test/alerts', secret_ref: 'tenant-alert-secret' },
        ],
      }),
    )

    await user.click(screen.getByRole('button', { name: /approvers/i }))

    expect(await screen.findByRole('heading', { name: /add approver/i })).toBeInTheDocument()
    await user.type(screen.getByLabelText(/^email$/i), 'grace@example.com')
    await user.type(screen.getByLabelText(/^slack user id/i), 'U123456789')
    await user.type(screen.getByLabelText(/^name \(optional\)$/i), 'Grace Hopper')
    await user.click(screen.getByRole('button', { name: /add approver/i }))

    await waitFor(() =>
      expect(postSpy).toHaveBeenCalledWith('/admin/tenants/tenant-1/approvers', {
        role: 'approver',
        email: 'grace@example.com',
        slack_user_id: 'U123456789',
        name: 'Grace Hopper',
      }),
    )

    const graceRow = await screen.findByText('grace@example.com')
    expect(graceRow).toBeInTheDocument()
    expect(screen.getByText('Both')).toBeInTheDocument()

    await user.click(within(screen.getByText('grace@example.com').closest('tr') as HTMLElement).getByRole('button', { name: /remove/i }))

    await waitFor(() => expect(deleteSpy).toHaveBeenCalledWith('/admin/tenants/tenant-1/approvers/approver-2'))
    await waitFor(() => expect(screen.queryByText('grace@example.com')).not.toBeInTheDocument())
  })

  it('loads alerts lazily and supports create, edit, and delete flows', async () => {
    const user = userEvent.setup()
    const { getSpy, postSpy, putSpy, deleteSpy } = installTenantDetailApi()

    renderRoute(<TenantDetail />, { path: '/tenants/:id', route: '/tenants/tenant-1?tab=alerts' })

    expect(await screen.findByRole('heading', { name: /create deny_spike rule/i })).toBeInTheDocument()
    await waitFor(() => expect(getSpy).toHaveBeenCalledWith('/admin/tenants/tenant-1/alerts/rules'))

    const createCard = getCardByHeading(/create deny_spike rule/i)
    await user.type(within(createCard).getByLabelText(/^rule name$/i), 'Night shift deny spike')
    await user.click(within(createCard).getByRole('checkbox', { name: /enabled immediately/i }))
    await user.click(within(createCard).getByRole('button', { name: /^create$/i }))

    await waitFor(() =>
      expect(postSpy).toHaveBeenCalledWith('/admin/tenants/tenant-1/alerts/rules', {
        name: 'Night shift deny spike',
        kind: 'deny_spike',
        enabled: false,
        config_json: { n: 3, m_minutes: 5 },
      }),
    )

    expect(await screen.findByText('Night shift deny spike')).toBeInTheDocument()

    await user.click(within(screen.getByText('Existing deny spike').closest('tr') as HTMLElement).getByRole('button', { name: /edit/i }))
    const modal = await screen.findByRole('heading', { name: /edit alert rule/i })
    expect(modal).toBeInTheDocument()

    const modalContainer = modal.closest('.modal')
    expect(modalContainer).not.toBeNull()
    await user.clear(within(modalContainer as HTMLElement).getByLabelText(/^rule name$/i))
    await user.type(within(modalContainer as HTMLElement).getByLabelText(/^rule name$/i), 'Existing deny spike updated')
    await user.click(screen.getByRole('button', { name: /^save rule$/i }))

    await waitFor(() =>
      expect(putSpy).toHaveBeenCalledWith('/admin/tenants/tenant-1/alerts/rules/rule-1', {
        name: 'Existing deny spike updated',
        kind: 'deny_spike',
        enabled: true,
        config_json: { n: 3, m_minutes: 5 },
      }),
    )

    expect(await screen.findByText('Existing deny spike updated')).toBeInTheDocument()

    await user.click(within(screen.getByText('Night shift deny spike').closest('tr') as HTMLElement).getByRole('button', { name: /delete/i }))

    await waitFor(() => expect(deleteSpy).toHaveBeenCalledWith('/admin/tenants/tenant-1/alerts/rules/rule-2'))
    await waitFor(() => expect(screen.queryByText('Night shift deny spike')).not.toBeInTheDocument())
  })

  it('keeps alert rules visible through partial alert-event failures and recovers on retry', async () => {
    const user = userEvent.setup()
    let eventsCalls = 0
    const getSpy = mockApiGet([
      ['/admin/tenants/tenant-1', {
        id: 'tenant-1',
        name: 'Tenant One',
        status: 'active',
        config: {},
        created_at: '2026-03-20T12:00:00Z',
      }],
      [(path) => path === '/admin/tenants/tenant-1/agents?include_disabled=true', { agents: [] }],
      ['/admin/tenants/tenant-1/apikeys', { api_keys: [] }],
      ['/admin/tenants/tenant-1/approvers', { approvers: [] }],
      ['/admin/tenants/tenant-1/notification-config', { approver_group: '', notify: [] }],
      ['/admin/tenants/tenant-1/alerts/rules', {
        rules: [
          {
            id: 'rule-1',
            tenant_id: 'tenant-1',
            name: 'Retry burst',
            kind: 'deny_spike',
            enabled: true,
            config_json: { n: 4, m_minutes: 10 },
            created_at: '2026-03-23T10:00:00Z',
            updated_at: '2026-03-23T10:00:00Z',
          },
        ],
      }],
      [(path) => path.startsWith('/admin/tenants/tenant-1/alerts/events?'), () => {
        eventsCalls += 1
        if (eventsCalls === 1) {
          throw new Error('Alert events unavailable')
        }
        return {
          events: [
            {
              id: 'alert-event-1',
              rule_id: 'rule-1',
              tenant_id: 'tenant-1',
              severity: 'warning',
              message: 'Recovered event',
              status: 'pending',
              created_at: '2026-03-23T11:00:00Z',
            },
          ],
        }
      }],
      [(path) => path.startsWith('/admin/tenants/tenant-1/analytics/summary'), {
        range_start: '2026-03-22T12:00:00Z',
        range_end: '2026-03-23T12:00:00Z',
        totals: { total_events: 0, allow_count: 0, deny_count: 0, approve_count: 0 },
        trend: [],
        risk_heatmap: [],
        per_agent: [],
        onboarding_checklist: {
          has_api_key: false,
          has_approver: false,
          has_toolcall: false,
          has_approval: false,
          has_execution: false,
        },
      }],
    ])

    renderRoute(<TenantDetail />, { path: '/tenants/:id', route: '/tenants/tenant-1?tab=alerts' })

    expect(await screen.findByText('Retry burst')).toBeInTheDocument()
    expect(await screen.findByText(/some alert data could not be loaded: events/i)).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /^retry$/i }))

    await waitFor(() => expect(getSpy).toHaveBeenCalledWith('/admin/tenants/tenant-1/alerts/rules'))
    expect(await screen.findByText('Recovered event')).toBeInTheDocument()
    expect(screen.queryByText(/some alert data could not be loaded: events/i)).not.toBeInTheDocument()
  })

  it('ignores stale alert responses when the alerts tab refetches quickly', async () => {
    const user = userEvent.setup()
    const firstRules = deferred<any>()
    const firstEvents = deferred<any>()
    let rulesCalls = 0
    let eventsCalls = 0

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/admin/tenants/tenant-1') {
        return {
          id: 'tenant-1',
          name: 'Tenant One',
          status: 'active',
          config: {},
          created_at: '2026-03-20T12:00:00Z',
        }
      }
      if (path === '/admin/tenants/tenant-1/agents?include_disabled=true') return { agents: [] }
      if (path === '/admin/tenants/tenant-1/apikeys') return { api_keys: [] }
      if (path === '/admin/tenants/tenant-1/approvers') return { approvers: [] }
      if (path === '/admin/tenants/tenant-1/notification-config') return { approver_group: '', notify: [] }
      if (path === '/admin/tenants/tenant-1/alerts/rules') {
        rulesCalls += 1
        if (rulesCalls === 1) return firstRules.promise
        return [
          {
            id: 'rule-fresh',
            tenant_id: 'tenant-1',
            name: 'Fresh rule',
            kind: 'deny_spike',
            enabled: true,
            config_json: { n: 4, m_minutes: 10 },
            created_at: '2026-03-23T12:00:00Z',
            updated_at: '2026-03-23T12:00:00Z',
          },
        ]
      }
      if (path.startsWith('/admin/tenants/tenant-1/alerts/events?')) {
        eventsCalls += 1
        if (eventsCalls === 1) return firstEvents.promise
        return [
          {
            id: 'alert-fresh',
            rule_id: 'rule-fresh',
            tenant_id: 'tenant-1',
            severity: 'warning',
            message: 'Fresh alert event',
            status: 'pending',
            created_at: '2026-03-23T12:15:00Z',
          },
        ]
      }
      if (path.startsWith('/admin/tenants/tenant-1/analytics/summary')) {
        return {
          range_start: '2026-03-22T12:00:00Z',
          range_end: '2026-03-23T12:00:00Z',
          totals: { total_events: 0, allow_count: 0, deny_count: 0, approve_count: 0 },
          trend: [],
          risk_heatmap: [],
          per_agent: [],
          onboarding_checklist: {
            has_api_key: false,
            has_approver: false,
            has_toolcall: false,
            has_approval: false,
            has_execution: false,
          },
        }
      }
      throw new Error(`Unhandled api.get call for ${path}`)
    })

    renderRoute(<TenantDetail />, { path: '/tenants/:id', route: '/tenants/tenant-1?tab=alerts' })

    expect(await screen.findByText(/^Alert Rules$/i)).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /agents/i }))
    await user.click(screen.getByRole('button', { name: /alerts/i }))

    expect(await screen.findByText('Fresh rule')).toBeInTheDocument()
    expect(screen.getByText('Fresh alert event')).toBeInTheDocument()

    firstRules.resolve([
      {
        id: 'rule-stale',
        tenant_id: 'tenant-1',
        name: 'Stale rule',
        kind: 'deny_spike',
        enabled: true,
        config_json: { n: 2, m_minutes: 5 },
        created_at: '2026-03-23T11:00:00Z',
        updated_at: '2026-03-23T11:00:00Z',
      },
    ])
    firstEvents.resolve([
      {
        id: 'alert-stale',
        rule_id: 'rule-stale',
        tenant_id: 'tenant-1',
        severity: 'warning',
        message: 'Stale alert event',
        status: 'pending',
        created_at: '2026-03-23T11:05:00Z',
      },
    ])

    await waitFor(() => expect(screen.queryByText('Stale rule')).not.toBeInTheDocument())
    expect(screen.getByText('Fresh rule')).toBeInTheDocument()
    expect(screen.queryByText('Stale alert event')).not.toBeInTheDocument()
  })

  it('loads analytics for the selected tab and updates the range preset', async () => {
    const user = userEvent.setup()
    const { getSpy } = installTenantDetailApi()

    renderRoute(<TenantDetail />, { path: '/tenants/:id', route: '/tenants/tenant-1?tab=analytics' })

    expect(await screen.findByRole('heading', { name: /tenant analytics/i })).toBeInTheDocument()
    expect(await screen.findByText(/resolved utc range/i)).toBeInTheDocument()

    await user.selectOptions(screen.getByLabelText(/^range$/i), '48')

    await waitFor(() =>
      expect(getSpy).toHaveBeenCalledWith(
        '/admin/tenants/tenant-1/analytics/summary?range=48h&bucket_minutes=60&top_agents=5',
      ),
    )
  })

  it('ignores stale analytics responses when the range changes quickly', async () => {
    const user = userEvent.setup()
    const firstAnalytics = deferred<any>()
    const secondAnalytics = deferred<any>()
    const getSpy = vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/admin/tenants/tenant-1') {
        return {
          id: 'tenant-1',
          name: 'Tenant One',
          status: 'active',
          config: {},
          created_at: '2026-03-20T12:00:00Z',
        }
      }
      if (path === '/admin/tenants/tenant-1/agents?include_disabled=true') return { agents: [] }
      if (path === '/admin/tenants/tenant-1/apikeys') return { api_keys: [] }
      if (path === '/admin/tenants/tenant-1/approvers') return { approvers: [] }
      if (path === '/admin/tenants/tenant-1/notification-config') return { approver_group: '', notify: [] }
      if (path === '/admin/tenants/tenant-1/analytics/summary?range=24h&bucket_minutes=60&top_agents=5') return firstAnalytics.promise
      if (path === '/admin/tenants/tenant-1/analytics/summary?range=48h&bucket_minutes=60&top_agents=5') return secondAnalytics.promise
      if (path === '/admin/tenants/tenant-1/alerts/rules') return []
      if (path.startsWith('/admin/tenants/tenant-1/alerts/events?')) return []
      throw new Error(`Unhandled api.get call for ${path}`)
    })

    renderRoute(<TenantDetail />, { path: '/tenants/:id', route: '/tenants/tenant-1?tab=analytics' })

    expect(await screen.findByRole('heading', { name: /tenant analytics/i })).toBeInTheDocument()
    await waitFor(() =>
      expect(getSpy).toHaveBeenCalledWith('/admin/tenants/tenant-1/analytics/summary?range=24h&bucket_minutes=60&top_agents=5'),
    )

    await user.selectOptions(screen.getByLabelText(/^range$/i), '48')
    await waitFor(() =>
      expect(getSpy).toHaveBeenCalledWith('/admin/tenants/tenant-1/analytics/summary?range=48h&bucket_minutes=60&top_agents=5'),
    )

    secondAnalytics.resolve({
      range_start: '2026-03-21T12:00:00Z',
      range_end: '2026-03-23T12:00:00Z',
      totals: { total_events: 43, allow_count: 20, deny_count: 10, approve_count: 13 },
      trend: [],
      risk_heatmap: [],
      per_agent: [],
      onboarding_checklist: {
        has_api_key: true,
        has_approver: true,
        has_toolcall: true,
        has_approval: true,
        has_execution: true,
      },
    })
    expect(await screen.findByText('43')).toBeInTheDocument()

    firstAnalytics.resolve({
      range_start: '2026-03-22T12:00:00Z',
      range_end: '2026-03-23T12:00:00Z',
      totals: { total_events: 17, allow_count: 7, deny_count: 5, approve_count: 5 },
      trend: [],
      risk_heatmap: [],
      per_agent: [],
      onboarding_checklist: {
        has_api_key: false,
        has_approver: false,
        has_toolcall: false,
        has_approval: false,
        has_execution: false,
      },
    })

    await waitFor(() => expect(screen.queryByText('17')).not.toBeInTheDocument())
    expect(screen.getByText('43')).toBeInTheDocument()
  })

  it('accepts array and wrapped payload variants for approvers and tenant alerts', async () => {
    const user = userEvent.setup()
    const getSpy = mockApiGet([
      ['/admin/tenants/tenant-1', {
        id: 'tenant-1',
        name: 'Tenant One',
        status: 'active',
        config: {},
        created_at: '2026-03-20T12:00:00Z',
      }],
      [(path) => path === '/admin/tenants/tenant-1/agents?include_disabled=true', { agents: [] }],
      ['/admin/tenants/tenant-1/apikeys', { api_keys: [] }],
      ['/admin/tenants/tenant-1/approvers', [
        { id: 'approver-array', email: 'array@example.com', name: 'Array Approver', slack_user_id: 'U111' },
      ]],
      ['/admin/tenants/tenant-1/notification-config', { approver_group: '', notify: [] }],
      ['/admin/tenants/tenant-1/alerts/rules', {
        rules: [
          {
            id: 'rule-array',
            tenant_id: 'tenant-1',
            name: 'Wrapped rule',
            kind: 'deny_spike',
            enabled: true,
            config_json: { n: 3, m_minutes: 5 },
            created_at: '2026-03-22T12:00:00Z',
            updated_at: '2026-03-22T12:00:00Z',
          },
        ],
      }],
      [(path) => path.startsWith('/admin/tenants/tenant-1/alerts/events?'), {
        events: [
          {
            id: 'alert-array',
            rule_id: 'rule-array',
            tenant_id: 'tenant-1',
            severity: 'warning',
            message: 'Wrapped alert event',
            status: 'pending',
            created_at: '2026-03-23T12:00:00Z',
          },
        ],
      }],
      [(path) => path.startsWith('/admin/tenants/tenant-1/analytics/summary'), {
        range_start: '2026-03-22T12:00:00Z',
        range_end: '2026-03-23T12:00:00Z',
        totals: { total_events: 0, allow_count: 0, deny_count: 0, approve_count: 0 },
        trend: [],
        risk_heatmap: [],
        per_agent: [],
        onboarding_checklist: {
          has_api_key: false,
          has_approver: false,
          has_toolcall: false,
          has_approval: false,
          has_execution: false,
        },
      }],
    ])

    renderRoute(<TenantDetail />, { path: '/tenants/:id', route: '/tenants/tenant-1?tab=approvers' })

    expect(await screen.findByText('array@example.com')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /alerts/i }))

    expect(await screen.findByText('Wrapped rule')).toBeInTheDocument()
    expect(screen.getByText('Wrapped alert event')).toBeInTheDocument()
    expect(getSpy).toHaveBeenCalledWith('/admin/tenants/tenant-1/approvers')
    expect(getSpy).toHaveBeenCalledWith('/admin/tenants/tenant-1/alerts/rules')
  })

  it('drops stale notification config state when a later refetch can no longer load it', async () => {
    const user = userEvent.setup()
    let notifCalls = 0
    const getSpy = mockApiGet([
      ['/admin/tenants/tenant-1', {
        id: 'tenant-1',
        name: 'Tenant One',
        status: 'active',
        config: {},
        created_at: '2026-03-20T12:00:00Z',
      }],
      [(path) => path === '/admin/tenants/tenant-1/agents?include_disabled=true', { agents: [{ id: 'agent-a', name: 'Agent A', tenant_id: 'tenant-1', status: 'active', created_at: '2026-03-22T12:00:00Z' }] }],
      [(path) => path === '/admin/tenants/tenant-1/agents?include_disabled=false', { agents: [{ id: 'agent-a', name: 'Agent A', tenant_id: 'tenant-1', status: 'active', created_at: '2026-03-22T12:00:00Z' }] }],
      ['/admin/tenants/tenant-1/apikeys', { api_keys: [] }],
      ['/admin/tenants/tenant-1/approvers', { approvers: [] }],
      ['/admin/tenants/tenant-1/notification-config', () => {
        notifCalls += 1
        if (notifCalls === 1) {
          return {
            approver_group: 'tenant_admin',
            notify: [{ kind: 'slack', channel: '#tenant-alerts' }],
          }
        }
        throw new Error('Notification configuration unavailable')
      }],
      ['/admin/tenants/tenant-1/alerts/rules', []],
      [(path) => path.startsWith('/admin/tenants/tenant-1/alerts/events?'), []],
      [(path) => path.startsWith('/admin/tenants/tenant-1/analytics/summary'), {
        range_start: '2026-03-22T12:00:00Z',
        range_end: '2026-03-23T12:00:00Z',
        totals: { total_events: 0, allow_count: 0, deny_count: 0, approve_count: 0 },
        trend: [],
        risk_heatmap: [],
        per_agent: [],
        onboarding_checklist: {
          has_api_key: false,
          has_approver: false,
          has_toolcall: false,
          has_approval: false,
          has_execution: false,
        },
      }],
    ])

    renderRoute(<TenantDetail />, { path: '/tenants/:id', route: '/tenants/tenant-1?tab=api_keys' })

    expect(await screen.findByRole('heading', { name: /approval notifications/i })).toBeInTheDocument()
    expect(screen.getByLabelText(/^approver group$/i)).toHaveValue('tenant_admin')

    await user.click(screen.getByRole('button', { name: /agents/i }))
    await user.click(await screen.findByRole('checkbox', { name: /hide disabled/i }))

    await waitFor(() => expect(getSpy).toHaveBeenCalledWith('/admin/tenants/tenant-1/agents?include_disabled=false'))

    await user.click(screen.getByRole('button', { name: /api keys/i }))
    expect(await screen.findByText(/notification configuration unavailable/i)).toBeInTheDocument()
    expect(screen.queryByLabelText(/^approver group$/i)).not.toBeInTheDocument()
  })
})
