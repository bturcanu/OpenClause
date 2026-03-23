import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import Events from './Events'
import { renderRoute } from '../test/render'
import { getFieldByLabelText } from '../test/form'
import { mockApiGet, stubMutableApi } from '../test/mockApi'

const eventFixture = {
  event_id: 'event-1234567890',
  tool: 'slack',
  action: 'msg.post',
  decision: 'approve',
  risk_score: 9,
  tenant_id: 'tenant-1',
  session_id: 'session-1',
  agent_id: 'agent-1',
  user_id: 'user-1',
  user_name: 'Ada Lovelace',
  user_email: 'ada@example.com',
  trace_id: 'trace-1',
  received_at: '2026-03-23T12:00:00Z',
}

describe('Audit Trail page', () => {
  it('hydrates filters from the URL and removes a single active filter chip', async () => {
    const user = userEvent.setup()
    stubMutableApi()
    mockApiGet([
      [/^\/admin\/events/, { events: [eventFixture] }],
    ])

    renderRoute(<Events />, { path: '/events', route: '/events?tenant_id=tenant-1&decision=approve' })

    expect(await screen.findByRole('heading', { name: /audit trail/i })).toBeInTheDocument()
    await waitFor(() => expect(getFieldByLabelText(/^tenant$/i)).toHaveValue('tenant-1'))
    expect(getFieldByLabelText(/^decision$/i)).toHaveValue('approve')

    await user.click(screen.getByRole('button', { name: /decision: approve/i }))

    await waitFor(() => expect(getFieldByLabelText(/^decision$/i)).toHaveValue(''))
    expect(getFieldByLabelText(/^tenant$/i)).toHaveValue('tenant-1')
  })

  it('applies inline row filters and keeps open actions available', async () => {
    const user = userEvent.setup()
    stubMutableApi()
    const getSpy = mockApiGet([
      [/^\/admin\/events/, { events: [eventFixture] }],
    ])

    renderRoute(<Events />, { path: '/events', route: '/events' })

    expect(await screen.findByRole('link', { name: /open event/i })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'approve' }))

    await waitFor(() => expect(getFieldByLabelText(/^decision$/i)).toHaveValue('approve'))
    expect(getSpy).toHaveBeenCalledWith(expect.stringContaining('decision=approve'))
    expect(screen.getByRole('link', { name: /open session/i })).toHaveAttribute('href', expect.stringContaining('/sessions/session-1'))
  })
})
