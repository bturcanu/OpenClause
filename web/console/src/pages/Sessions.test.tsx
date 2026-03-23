import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import Sessions from './Sessions'
import { shortID } from '../ui'
import { renderRoute } from '../test/render'
import { mockApiGet } from '../test/mockApi'

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

  it('updates filters, resets them, and keeps copy actions usable', async () => {
    const user = userEvent.setup()
    const writeTextSpy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
    const getSpy = mockApiGet([
      [/^\/admin\/sessions/, { sessions: sessionsFixture }],
    ])

    renderRoute(<Sessions />, { path: '/sessions', route: '/sessions' })

    expect(await screen.findByRole('heading', { name: /sessions/i })).toBeInTheDocument()

    await user.type(screen.getByLabelText(/^tenant$/i), 'tenant-1')

    await waitFor(() => expect(getSpy).toHaveBeenCalledWith(expect.stringContaining('tenant_id=tenant-1')))
    expect(await screen.findByRole('button', { name: /tenant id: tenant-1/i })).toBeInTheDocument()

    const firstSessionRow = screen.getByRole('link', { name: shortID('session-low-1234567890', 14) }).closest('tr')
    expect(firstSessionRow).not.toBeNull()

    await user.click(within(firstSessionRow as HTMLElement).getByRole('button', { name: /copy session id/i }))
    await user.click(within(firstSessionRow as HTMLElement).getByRole('button', { name: /copy tenant id/i }))
    await user.click(within(firstSessionRow as HTMLElement).getByRole('button', { name: /copy trace id/i }))

    expect(writeTextSpy).toHaveBeenCalledWith('session-low-1234567890')
    expect(writeTextSpy).toHaveBeenCalledWith('tenant-1')
    expect(writeTextSpy).toHaveBeenCalledWith('trace-low-123')

    await user.click(screen.getByRole('button', { name: /clear filters/i }))

    await waitFor(() => {
      expect(screen.getByLabelText(/^tenant$/i)).toHaveValue('')
      expect(screen.queryByRole('button', { name: /tenant id: tenant-1/i })).not.toBeInTheDocument()
    })
  })

  it('accepts wrapped payloads and keeps fallback text stable when optional fields are missing', async () => {
    mockApiGet([
      [/^\/admin\/sessions/, {
        sessions: [
          {
            id: 'session-minimal',
            tenant_id: 'tenant-3',
            agent_id: '',
            started_at: '2026-03-23T08:00:00Z',
            last_event_at: '2026-03-23T08:05:00Z',
            event_count: 1,
            allow_count: 0,
            deny_count: 1,
            approve_count: 0,
          },
        ],
      }],
    ])

    renderRoute(<Sessions />, { path: '/sessions', route: '/sessions' })

    expect(await screen.findByRole('link', { name: shortID('session-minimal', 14) })).toBeInTheDocument()
    expect(screen.getByText(/requested without user or agent attribution/i)).toBeInTheDocument()
    expect(screen.getByText(/trace \(none\)/i)).toBeInTheDocument()
  })
})
