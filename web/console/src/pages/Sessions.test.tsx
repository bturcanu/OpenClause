import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import Sessions from './Sessions'
import { shortID } from '../ui'
import { renderRoute } from '../test/render'
import { mockApiGet, stubMutableApi } from '../test/mockApi'

const sessionsFixture = [
  {
    id: 'session-low-1234567890',
    tenant_id: 'tenant-1',
    agent_id: 'agent-1',
    user_id: 'user-1',
    user_name: 'Ada Lovelace',
    user_email: 'ada@example.com',
    trace_id: 'trace-low-123',
    started_at: '2026-03-23T10:00:00Z',
    last_event_at: '2026-03-23T10:15:00Z',
    event_count: 2,
    allow_count: 2,
    deny_count: 0,
    approve_count: 0,
    last_tool: 'slack',
    last_action: 'msg.post',
    last_decision: 'allow',
    last_risk_score: 2,
  },
  {
    id: 'session-high-1234567890',
    tenant_id: 'tenant-2',
    agent_id: 'agent-2',
    user_id: 'user-2',
    user_name: 'Grace Hopper',
    user_email: 'grace@example.com',
    trace_id: 'trace-high-123',
    started_at: '2026-03-23T11:00:00Z',
    last_event_at: '2026-03-23T11:30:00Z',
    event_count: 9,
    allow_count: 4,
    deny_count: 1,
    approve_count: 3,
    last_tool: 'jira',
    last_action: 'issue.create',
    last_decision: 'approve',
    last_risk_score: 8,
  },
]

describe('Sessions page', () => {
  it('sorts sessions within the current page by event count', async () => {
    const user = userEvent.setup()
    stubMutableApi()
    mockApiGet([
      [/^\/admin\/sessions/, { sessions: sessionsFixture }],
    ])

    renderRoute(<Sessions />, { path: '/sessions', route: '/sessions' })

    expect(await screen.findByRole('heading', { name: /sessions/i })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /events/i }))

    await waitFor(() => {
      const rows = screen.getAllByRole('row')
      expect(within(rows[1]).getByRole('link', { name: shortID('session-high-1234567890', 14) })).toBeInTheDocument()
    })
    expect(screen.getAllByRole('link', { name: /open run/i }).length).toBeGreaterThan(0)
  })
})
