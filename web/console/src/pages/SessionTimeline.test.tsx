import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import SessionTimeline from './SessionTimeline'
import { APIClientError, api } from '../api'
import { renderRoute } from '../test/render'
import { mockApiGet, stubMutableApi } from '../test/mockApi'

describe('Session detail page', () => {
  it('asks platform admins to disambiguate duplicate session ids by tenant', async () => {
    const user = userEvent.setup()
    stubMutableApi()
    const getSpy = mockApiGet([
      [(path) => path === '/admin/sessions/demo', () => {
        throw new APIClientError('Need tenant', {
          status: 409,
          code: 'tenant_required',
          candidates: ['tenant-a', 'tenant-b'],
        })
      }],
      [(path) => path === '/admin/sessions/demo/timeline', []],
      [(path) => path === '/admin/sessions/demo?tenant_id=tenant-b', {
        id: 'demo',
        tenant_id: 'tenant-b',
        agent_id: 'agent-1',
        user_id: 'user-1',
        user_name: 'Ada Lovelace',
        user_email: 'ada@example.com',
        trace_id: 'trace-1',
        started_at: '2026-03-23T10:00:00Z',
        last_event_at: '2026-03-23T10:15:00Z',
        event_count: 1,
        allow_count: 1,
        deny_count: 0,
        approve_count: 0,
      }],
      [(path) => path === '/admin/sessions/demo/timeline?tenant_id=tenant-b', {
        events: [
          {
            event_id: 'event-1',
            tenant_id: 'tenant-b',
            agent_id: 'agent-1',
            user_id: 'user-1',
            user_name: 'Ada Lovelace',
            user_email: 'ada@example.com',
            tool: 'slack',
            action: 'msg.post',
            risk_score: 2,
            decision: 'allow',
            session_id: 'demo',
            trace_id: 'trace-1',
            received_at: '2026-03-23T10:15:00Z',
            explain: 'Allowed',
          },
        ],
      }],
    ])

    renderRoute(<SessionTimeline />, { path: '/sessions/:id', route: '/sessions/demo' })

    expect(await screen.findByRole('heading', { name: /pick a tenant to continue/i })).toBeInTheDocument()

    await user.selectOptions(screen.getByLabelText(/^tenant$/i), 'tenant-b')

    await waitFor(() => expect(getSpy).toHaveBeenCalledWith('/admin/sessions/demo?tenant_id=tenant-b'))
    expect(await screen.findByText('slack.msg.post')).toBeInTheDocument()
  })

  it('keeps the session summary visible when the timeline load fails and retries the session fetch', async () => {
    const user = userEvent.setup()
    stubMutableApi()
    const getSpy = mockApiGet([
      [(path) => path === '/admin/sessions/demo?tenant_id=tenant-a', {
        id: 'demo',
        tenant_id: 'tenant-a',
        agent_id: 'agent-1',
        user_id: 'user-1',
        user_name: 'Ada Lovelace',
        user_email: 'ada@example.com',
        trace_id: 'trace-1',
        started_at: '2026-03-23T10:00:00Z',
        last_event_at: '2026-03-23T10:15:00Z',
        event_count: 1,
        allow_count: 1,
        deny_count: 0,
        approve_count: 0,
      }],
      [(path) => path === '/admin/sessions/demo/timeline?tenant_id=tenant-a', () => {
        throw new Error('Timeline unavailable')
      }],
    ])

    renderRoute(<SessionTimeline />, { path: '/sessions/:id', route: '/sessions/demo?tenant_id=tenant-a' })

    expect(await screen.findByText(/timeline unavailable/i)).toBeInTheDocument()
    expect(screen.getByText(/requested by ada/i)).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /^retry$/i }))
    await waitFor(() => expect(getSpy).toHaveBeenCalledWith('/admin/sessions/demo/timeline?tenant_id=tenant-a'))
  })

  it('copies the shareable summary and exports the tenant-scoped session bundle', async () => {
    const user = userEvent.setup()
    const writeTextSpy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined)
    const getBlobSpy = vi.spyOn(api, 'getBlob').mockResolvedValue(new Blob(['csv'], { type: 'text/csv' }))
    const createObjectURLSpy = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:session-export')
    const revokeObjectURLSpy = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})

    stubMutableApi()
    mockApiGet([
      [(path) => path === '/admin/sessions/demo?tenant_id=tenant-a', {
        id: 'demo',
        tenant_id: 'tenant-a',
        agent_id: 'agent-1',
        user_id: 'user-1',
        user_name: 'Ada Lovelace',
        user_email: 'ada@example.com',
        trace_id: 'trace-1',
        started_at: '2026-03-23T10:00:00Z',
        last_event_at: '2026-03-23T10:15:00Z',
        event_count: 1,
        allow_count: 1,
        deny_count: 0,
        approve_count: 0,
      }],
      [(path) => path === '/admin/sessions/demo/timeline?tenant_id=tenant-a', {
        events: [
          {
            event_id: 'event-1',
            tenant_id: 'tenant-a',
            agent_id: 'agent-1',
            user_id: 'user-1',
            user_name: 'Ada Lovelace',
            user_email: 'ada@example.com',
            tool: 'slack',
            action: 'msg.post',
            risk_score: 2,
            decision: 'allow',
            session_id: 'demo',
            trace_id: 'trace-1',
            received_at: '2026-03-23T10:15:00Z',
            explain: 'Allowed',
          },
        ],
      }],
    ])

    renderRoute(<SessionTimeline />, { path: '/sessions/:id', route: '/sessions/demo?tenant_id=tenant-a' })

    expect(await screen.findByText('slack.msg.post')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /copy shareable summary/i }))
    expect(writeTextSpy).toHaveBeenCalledWith(expect.stringContaining('Session demo'))
    expect(await screen.findByText(/summary copied/i)).toBeInTheDocument()

    await user.click(screen.getByText(/export/i, { selector: 'summary' }))
    await user.click(screen.getByRole('button', { name: /export csv/i }))

    await waitFor(() => expect(getBlobSpy).toHaveBeenCalledWith('/admin/sessions/demo/export/csv?tenant_id=tenant-a'))
    expect(createObjectURLSpy).toHaveBeenCalled()
    expect(revokeObjectURLSpy).toHaveBeenCalledWith('blob:session-export')
  })

  it('filters timeline events through label-bound controls and resets back to all events', async () => {
    const user = userEvent.setup()
    stubMutableApi()
    mockApiGet([
      [(path) => path === '/admin/sessions/demo?tenant_id=tenant-a', {
        id: 'demo',
        tenant_id: 'tenant-a',
        agent_id: 'agent-1',
        user_id: 'user-1',
        user_name: 'Ada Lovelace',
        user_email: 'ada@example.com',
        trace_id: 'trace-1',
        started_at: '2026-03-23T10:00:00Z',
        last_event_at: '2026-03-23T10:15:00Z',
        event_count: 2,
        allow_count: 1,
        deny_count: 1,
        approve_count: 0,
      }],
      [(path) => path === '/admin/sessions/demo/timeline?tenant_id=tenant-a', {
        events: [
          {
            event_id: 'event-allow',
            tenant_id: 'tenant-a',
            agent_id: 'agent-1',
            tool: 'slack',
            action: 'msg.post',
            risk_score: 2,
            decision: 'allow',
            session_id: 'demo',
            received_at: '2026-03-23T10:10:00Z',
            explain: 'Allowed',
          },
          {
            event_id: 'event-deny',
            tenant_id: 'tenant-a',
            agent_id: 'agent-1',
            tool: 'jira',
            action: 'issue.delete',
            risk_score: 9,
            decision: 'deny',
            session_id: 'demo',
            received_at: '2026-03-23T10:15:00Z',
            explain: 'Denied',
          },
        ],
      }],
    ])

    renderRoute(<SessionTimeline />, { path: '/sessions/:id', route: '/sessions/demo?tenant_id=tenant-a' })

    expect(await screen.findByText('slack.msg.post')).toBeInTheDocument()
    expect(screen.getByText('jira.issue.delete')).toBeInTheDocument()

    await user.selectOptions(screen.getByLabelText(/^decision$/i), 'deny')
    expect(screen.queryByText('slack.msg.post')).not.toBeInTheDocument()
    expect(screen.getByText('jira.issue.delete')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /reset filters/i }))
    expect(await screen.findByText('slack.msg.post')).toBeInTheDocument()
  })

  it('surfaces request ids when a session export fails', async () => {
    const user = userEvent.setup()
    stubMutableApi()
    vi.spyOn(api, 'getBlob').mockRejectedValue(new APIClientError('Export blocked (request id: req-session-9)', {
      status: 409,
      code: 'tenant_required',
      requestId: 'req-session-9',
    }))

    mockApiGet([
      [(path) => path === '/admin/sessions/demo?tenant_id=tenant-a', {
        id: 'demo',
        tenant_id: 'tenant-a',
        agent_id: 'agent-1',
        user_id: 'user-1',
        user_name: 'Ada Lovelace',
        user_email: 'ada@example.com',
        trace_id: 'trace-1',
        started_at: '2026-03-23T10:00:00Z',
        last_event_at: '2026-03-23T10:15:00Z',
        event_count: 1,
        allow_count: 1,
        deny_count: 0,
        approve_count: 0,
      }],
      [(path) => path === '/admin/sessions/demo/timeline?tenant_id=tenant-a', { events: [] }],
    ])

    renderRoute(<SessionTimeline />, { path: '/sessions/:id', route: '/sessions/demo?tenant_id=tenant-a' })

    expect(await screen.findByRole('heading', { name: /session detail/i })).toBeInTheDocument()

    await user.click(screen.getByText(/export/i, { selector: 'summary' }))
    await user.click(screen.getByRole('button', { name: /export csv/i }))

    expect(await screen.findByText(/req-session-9/i)).toBeInTheDocument()
  })
})
