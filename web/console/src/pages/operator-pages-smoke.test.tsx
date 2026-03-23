import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import Approvals from './Approvals'
import Alerts from './Alerts'
import Connectors from './Connectors'
import EventDetail from './EventDetail'
import Policies from './Policies'
import Tenants from './Tenants'
import Users from './Users'
import { renderRoute } from '../test/render'
import { getFieldByLabelText } from '../test/form'
import { mockApiGet, stubMutableApi } from '../test/mockApi'

function makeToken(payload: Record<string, unknown>) {
  const encoded = btoa(JSON.stringify(payload)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '')
  return `header.${encoded}.signature`
}

describe('operator page smoke coverage', () => {
  it('filters tenants by search text', async () => {
    const user = userEvent.setup()
    stubMutableApi()
    mockApiGet([
      ['/admin/tenants', {
        tenants: [
          { id: 'tenant-a', name: 'Alpha Corp', status: 'active', created_at: '2026-03-20T12:00:00Z' },
          { id: 'tenant-b', name: 'Beta Labs', status: 'disabled', created_at: '2026-03-21T12:00:00Z' },
        ],
      }],
    ])

    renderRoute(<Tenants />, { path: '/tenants', route: '/tenants' })

    expect(await screen.findByText('Alpha Corp')).toBeInTheDocument()
    await user.type(getFieldByLabelText(/^search$/i), 'beta')
    expect(screen.queryByText('Alpha Corp')).not.toBeInTheDocument()
    expect(screen.getByText('Beta Labs')).toBeInTheDocument()
  })

  it('opens an approval for review', async () => {
    const user = userEvent.setup()
    stubMutableApi()
    mockApiGet([
      ['/admin/approvals/pending', {
        approvals: [{
          id: 'approval-1',
          event_id: 'event-1',
          tool: 'slack',
          action: 'msg.post',
          risk_score: 9,
          agent_id: 'agent-1',
          tenant_id: 'tenant-1',
          status: 'pending',
          user_id: 'user-1',
          user_name: 'Ada',
          user_email: 'ada@example.com',
          session_id: 'session-1',
          trace_id: 'trace-1',
          created_at: '2026-03-23T10:00:00Z',
          expires_at: '2026-03-23T11:00:00Z',
        }],
      }],
    ])

    renderRoute(<Approvals />, { path: '/approvals', route: '/approvals' })

    await user.click(await screen.findByRole('button', { name: /review/i }))
    expect(screen.getByRole('heading', { name: /approval context/i })).toBeInTheDocument()
  })

  it('renders users and their roles for admins', async () => {
    localStorage.setItem('oc_token', makeToken({ roles: ['platform_admin'], sid: 'session-1' }))
    stubMutableApi()
    mockApiGet([
      ['/admin/users', {
        users: [{
          id: 'user-1',
          email: 'ada@example.com',
          name: 'Ada Lovelace',
          status: 'active',
          created_at: '2026-03-20T12:00:00Z',
          active_session_count: 1,
          roles: [{ id: 'role-1', user_id: 'user-1', role: 'platform_admin' }],
        }],
      }],
    ])

    renderRoute(<Users />, { path: '/users', route: '/users' })

    expect((await screen.findAllByText('ada@example.com')).length).toBeGreaterThan(0)
    expect(screen.getByText(/ada lovelace/i)).toBeInTheDocument()
  })

  it('renders alerts rules and retry metadata', async () => {
    localStorage.setItem('oc_token', makeToken({ roles: ['platform_admin'] }))
    stubMutableApi()
    mockApiGet([
      ['/admin/alerts/rules', {
        rules: [{
          id: 'rule-1',
          tenant_id: 'tenant-1',
          name: 'Deny spike',
          kind: 'deny_spike',
          enabled: true,
          created_at: '2026-03-20T12:00:00Z',
        }],
      }],
      ['/admin/alerts/events', {
        events: [{
          id: 'alert-1',
          rule_id: 'rule-1',
          tenant_id: 'tenant-1',
          severity: 'warning',
          message: 'Deny spike detected',
          status: 'pending',
          created_at: '2026-03-23T12:00:00Z',
          attempt_count: 2,
          next_attempt_at: '2026-03-23T12:05:00Z',
        }],
      }],
    ])

    renderRoute(<Alerts />, { path: '/alerts', route: '/alerts' })

    expect(await screen.findByText('Deny spike')).toBeInTheDocument()
    expect(screen.getByText('Deny spike detected')).toBeInTheDocument()
  })

  it('loads policy versions and keeps the current marker visible', async () => {
    stubMutableApi()
    mockApiGet([
      ['/admin/tenants', {
        tenants: [{ id: 'tenant-1', name: 'Tenant One', status: 'active' }],
      }],
      ['/admin/tenants/tenant-1/policy/config', {
        max_risk_auto_approve: 7,
        read_actions: ['jira.issue.read'],
        write_actions: ['jira.issue.create'],
        destructive_actions: ['aws.instance.delete'],
        require_destructive_approval: true,
      }],
      ['/admin/tenants/tenant-1/policy/versions', [
        {
          id: 2,
          version: 'v2',
          deployed_by: 'admin@example.com',
          deployed_at: '2026-03-23T12:00:00Z',
          notes: 'Latest',
        },
        {
          id: 1,
          version: 'v1',
          deployed_by: 'admin@example.com',
          deployed_at: '2026-03-20T12:00:00Z',
          notes: 'Initial',
        },
      ]],
    ])

    renderRoute(<Policies />, { path: '/policies', route: '/policies' })

    expect((await screen.findAllByText('v2')).length).toBeGreaterThan(0)
    expect(screen.getByText(/^current$/i, { selector: 'span' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^selected$/i })).toBeInTheDocument()
  })

  it('renders connector actions and supports local search', async () => {
    const user = userEvent.setup()
    stubMutableApi()
    mockApiGet([
      ['/admin/connectors', {
        connectors: [{
          name: 'slack',
          type: 'remote',
          actions: ['msg.post', 'channel.list', 'user.lookup', 'message.pin', 'message.update', 'message.delete', 'conversation.info'],
        }],
      }],
    ])

    renderRoute(<Connectors />, { path: '/connectors', route: '/connectors' })

    expect(await screen.findByText('slack')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /\+1 more/i })).toBeInTheDocument()
    await user.type(getFieldByLabelText(/^search$/i), 'channel.list')
    expect(screen.getByText('channel.list')).toBeInTheDocument()
  })

  it('renders event detail run context and evidence fields', async () => {
    stubMutableApi()
    mockApiGet([
      ['/admin/events/event-1', {
        event_id: 'event-1',
        tenant_id: 'tenant-1',
        agent_id: 'agent-1',
        user_id: 'user-1',
        user_name: 'Ada',
        user_email: 'ada@example.com',
        session_id: 'session-1',
        trace_id: 'trace-1',
        tool: 'slack',
        action: 'msg.post',
        payload_json: { channel: '#ops' },
        risk_score: 8,
        decision: 'approve',
        policy_result: { decision: 'approve', reason: 'High risk' },
        prev_hash: 'prev',
        hash: 'hash',
        result: { status: 'pending' },
        received_at: '2026-03-23T12:00:00Z',
      }],
    ])

    renderRoute(<EventDetail />, { path: '/events/:eventId', route: '/events/event-1' })

    expect(await screen.findByRole('heading', { name: /run context/i })).toBeInTheDocument()
    expect(screen.getByText(/policy evaluation/i)).toBeInTheDocument()
    expect(screen.getByText(/hash chain/i)).toBeInTheDocument()
  })
})
