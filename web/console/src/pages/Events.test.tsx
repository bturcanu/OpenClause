import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { APIClientError, api } from '../api'
import Events from './Events'
import { renderRoute } from '../test/render'
import { mockApiGet } from '../test/mockApi'

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

const secondEventFixture = {
  ...eventFixture,
  event_id: 'event-2222222222',
  tool: 'jira',
  action: 'issue.create',
  decision: 'allow',
  risk_score: 3,
  tenant_id: 'tenant-2',
  session_id: 'session-2',
  agent_id: 'agent-2',
  user_id: 'user-2',
  user_name: 'Grace Hopper',
  user_email: 'grace@example.com',
  trace_id: 'trace-2',
  received_at: '2026-03-22T10:00:00Z',
}

function makeToken(payload: Record<string, unknown>) {
  const encoded = btoa(JSON.stringify(payload)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '')
  return `header.${encoded}.signature`
}

describe('Audit Trail page', () => {
  it('hydrates filters from the URL and removes a single active filter chip', async () => {
    const user = userEvent.setup()
    mockApiGet([
      [/^\/admin\/events/, { events: [eventFixture] }],
    ])

    renderRoute(<Events />, { path: '/events', route: '/events?tenant_id=tenant-1&decision=approve' })

    expect(await screen.findByRole('heading', { name: /audit trail/i })).toBeInTheDocument()
    await waitFor(() => expect(screen.getByLabelText(/^tenant$/i)).toHaveValue('tenant-1'))
    expect(screen.getByLabelText(/^decision$/i)).toHaveValue('approve')

    await user.click(screen.getByRole('button', { name: /decision: approve/i }))

    await waitFor(() => expect(screen.getByLabelText(/^decision$/i)).toHaveValue(''))
    expect(screen.getByLabelText(/^tenant$/i)).toHaveValue('tenant-1')
  })

  it('applies inline row filters and keeps open actions available', async () => {
    const user = userEvent.setup()
    const getSpy = mockApiGet([
      [/^\/admin\/events/, { events: [eventFixture] }],
    ])

    renderRoute(<Events />, { path: '/events', route: '/events' })

    expect(await screen.findByRole('link', { name: /open event/i })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'approve' }))

    await waitFor(() => expect(screen.getByLabelText(/^decision$/i)).toHaveValue('approve'))
    expect(getSpy).toHaveBeenCalledWith(expect.stringContaining('decision=approve'))
    expect(screen.getByRole('link', { name: /open session/i })).toHaveAttribute('href', expect.stringContaining('/sessions/session-1'))
  })

  it('clears filters back to defaults and enforces tenant selection before platform-admin exports', async () => {
    const user = userEvent.setup()
    localStorage.setItem('oc_token', makeToken({ roles: ['platform_admin'] }))

    mockApiGet([
      [/^\/admin\/events/, { events: [eventFixture] }],
    ])
    const getBlobSpy = vi.spyOn(api, 'getBlob').mockResolvedValue(new Blob(['ok'], { type: 'text/csv' }))

    renderRoute(<Events />, {
      path: '/events',
      route: '/events?tenant_id=tenant-1&decision=approve&tool=slack&since=2026-03-23T09:00:00Z',
    })

    expect(await screen.findByRole('heading', { name: /audit trail/i })).toBeInTheDocument()
    await waitFor(() => expect(screen.getByLabelText(/^tenant$/i)).toHaveValue('tenant-1'))
    expect(screen.getByLabelText(/^decision$/i)).toHaveValue('approve')

    await user.click(screen.getByRole('button', { name: /clear filters/i }))

    await waitFor(() => {
      expect(screen.getByLabelText(/^tenant$/i)).toHaveValue('')
      expect(screen.getByLabelText(/^decision$/i)).toHaveValue('')
      expect(screen.getByLabelText(/^tool$/i)).toHaveValue('')
      expect(screen.getByLabelText(/^since/i)).toHaveValue('')
    })

    await user.click(screen.getByText(/export ▾/i))
    await user.click(screen.getByRole('button', { name: /export csv/i }))

    expect(await screen.findByText(/select a tenant before exporting csv/i)).toBeInTheDocument()
    expect(getBlobSpy).not.toHaveBeenCalled()

    await user.type(screen.getByLabelText(/^tenant$/i), 'tenant-1')
    await user.click(screen.getByText(/export ▾/i))
    await user.click(screen.getByRole('button', { name: /export csv/i }))

    await waitFor(() =>
      expect(getBlobSpy).toHaveBeenCalledWith(expect.stringContaining('/admin/events/export/csv?tenant_id=tenant-1')),
    )
  })

  it('sorts the current page by risk score when the header is activated', async () => {
    const user = userEvent.setup()
    mockApiGet([
      [/^\/admin\/events/, { events: [secondEventFixture, eventFixture] }],
    ])

    renderRoute(<Events />, { path: '/events', route: '/events' })

    expect(await screen.findByRole('heading', { name: /audit trail/i })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /^risk$/i }))
    await user.click(screen.getByRole('button', { name: /^risk$/i }))

    await waitFor(() => {
      const rows = screen.getAllByRole('row')
      expect(within(rows[1]).getByRole('link', { name: /slack\.msg\.post/i })).toBeInTheDocument()
    })
  })

  it('surfaces the structured range-too-large export contract for evidence bundles', async () => {
    const user = userEvent.setup()
    localStorage.setItem('oc_token', makeToken({ roles: ['platform_admin'] }))

    mockApiGet([
      [/^\/admin\/events/, { events: [eventFixture] }],
    ])
    vi.spyOn(api, 'getBlob').mockRejectedValue(new APIClientError('Range too large', {
      status: 400,
      details: { reason: 'range_too_large', max_events: 10000 },
    }))

    renderRoute(<Events />, {
      path: '/events',
      route: '/events?tenant_id=tenant-1',
    })

    expect(await screen.findByRole('heading', { name: /audit trail/i })).toBeInTheDocument()

    await user.click(screen.getByText(/export ▾/i))
    await user.click(screen.getByRole('button', { name: /export evidence bundle/i }))

    expect(await screen.findByText(/evidence bundle exports are limited to 10,000 events/i)).toBeInTheDocument()
  })
})
