import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import EventDetail from './EventDetail'
import { renderRoute } from '../test/render'

describe('Event detail page', () => {
  it('renders detail sections and supports copy actions', async () => {
    const user = userEvent.setup()
    const writeTextSpy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined)

    vi.spyOn(api, 'get').mockResolvedValue({
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
      resource: 'channel/ops',
      payload_json: { channel: '#ops' },
      risk_score: 8,
      decision: 'approve',
      policy_result: {
        decision: 'approve',
        reason: 'Allowed in preview',
        approver_group: 'ops',
        requirements: { change_type: 'standard' },
        risk_overrides: { resource_sensitivity: 2 },
        notify: [{ kind: 'slack', channel: '#ops' }],
      },
      prev_hash: 'prev-hash',
      hash: 'event-hash',
      result: { status: 'success', duration_ms: 120, output_json: { ok: true } },
      received_at: '2026-03-23T12:00:00Z',
    })

    renderRoute(<EventDetail />, { path: '/events/:eventId', route: '/events/event-1' })

    expect(await screen.findByRole('heading', { name: /event detail/i })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /run context/i })).toBeInTheDocument()
    expect(screen.getByText(/approver group/i)).toBeInTheDocument()
    expect(screen.getByText(/change type/i)).toBeInTheDocument()
    expect(screen.getByText('standard')).toBeInTheDocument()
    expect(screen.getByText(/resource sensitivity/i)).toBeInTheDocument()
    expect(screen.getByText('2')).toBeInTheDocument()
    expect(screen.getByText(/slack #ops/i)).toBeInTheDocument()

    const hero = screen.getByRole('heading', { name: /event detail/i }).closest('.page-hero') as HTMLElement | null
    expect(hero).not.toBeNull()
    const sessionCard = screen.getByText(/^session$/i, { selector: '.meta-label' }).closest('.identity-card') as HTMLElement | null
    expect(sessionCard).not.toBeNull()
    expect(within(sessionCard!).getByRole('link', { name: 'session-1' })).toHaveAttribute('href', '/sessions/session-1?tenant_id=tenant-1')

    await user.click(within(hero!).getByRole('button', { name: /^copy event id$/i }))
    expect(writeTextSpy).toHaveBeenCalledWith('event-1')
    expect(await screen.findByText(/event id copied/i)).toBeInTheDocument()
  })

  it('shows retryable errors and recovers on retry', async () => {
    const user = userEvent.setup()

    const getSpy = vi.spyOn(api, 'get')
      .mockRejectedValueOnce(new Error('Event detail unavailable'))
      .mockResolvedValueOnce({
        event_id: 'event-2',
        tenant_id: 'tenant-1',
        agent_id: 'agent-1',
        session_id: 'session-2',
        tool: 'jira',
        action: 'issue.create',
        payload_json: {},
        risk_score: 4,
        decision: 'allow',
        policy_result: { reason: 'Allowed' },
        prev_hash: '',
        hash: 'hash-2',
        result: null,
        received_at: '2026-03-23T12:00:00Z',
      })

    renderRoute(<EventDetail />, { path: '/events/:eventId', route: '/events/event-2' })

    expect(await screen.findByText(/event detail unavailable/i)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /^retry$/i }))

    const hero = (await screen.findByRole('heading', { name: /event detail/i })).closest('.page-hero') as HTMLElement | null
    expect(hero).not.toBeNull()
    expect(within(hero!).getByRole('button', { name: /^copy event id$/i })).toBeInTheDocument()
    expect(getSpy).toHaveBeenCalledTimes(2)
  })

  it('handles missing optional fields without crashing', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      event_id: 'event-3',
      tenant_id: 'tenant-1',
      agent_id: '',
      session_id: '',
      trace_id: '',
      tool: 'slack',
      action: 'msg.post',
      payload_json: {},
      risk_score: 5,
      decision: 'deny',
      policy_result: null,
      prev_hash: '',
      hash: '',
      result: null,
      received_at: '2026-03-23T12:00:00Z',
    })

    renderRoute(<EventDetail />, { path: '/events/:eventId', route: '/events/event-3' })

    expect(await screen.findByText(/a detailed policy reason was not recorded/i)).toBeInTheDocument()
    expect(screen.getByText(/this event does not have an execution result/i)).toBeInTheDocument()
    expect(screen.getAllByText('(none)', { selector: 'code' }).length).toBeGreaterThan(0)
    expect(screen.getAllByText('Not recorded').length).toBeGreaterThan(0)
    expect(screen.getByText('(genesis)')).toBeInTheDocument()
  })
})
