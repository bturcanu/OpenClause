import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import SessionTimeline from './SessionTimeline'
import { APIClientError, api } from '../api'
import { renderRoute } from '../test/render'
import { mockApiGet, stubMutableApi } from '../test/mockApi'

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

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
    let timelineCalls = 0
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
        timelineCalls += 1
        if (timelineCalls === 1) {
          throw new Error('Timeline unavailable')
        }
        return {
          events: [
            {
              event_id: 'event-retry',
              tenant_id: 'tenant-a',
              agent_id: 'agent-1',
              tool: 'slack',
              action: 'msg.post',
              risk_score: 3,
              decision: 'allow',
              session_id: 'demo',
              received_at: '2026-03-23T10:15:00Z',
              explain: 'Recovered on retry',
            },
          ],
        }
      }],
    ])

    renderRoute(<SessionTimeline />, { path: '/sessions/:id', route: '/sessions/demo?tenant_id=tenant-a' })

    expect(await screen.findByText(/timeline unavailable/i)).toBeInTheDocument()
    expect(screen.getByText(/requested by ada/i)).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /^retry$/i }))
    await waitFor(() => expect(getSpy).toHaveBeenCalledWith('/admin/sessions/demo/timeline?tenant_id=tenant-a'))
    expect(await screen.findByText('slack.msg.post')).toBeInTheDocument()
    expect(screen.queryByText(/timeline unavailable/i)).not.toBeInTheDocument()
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
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
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

    expect(await screen.findByText(/^Export blocked \(request id: req-session-9\)$/i)).toBeInTheDocument()
    expect(screen.getByText('req-session-9', { selector: 'code' })).toBeInTheDocument()
    expect(warnSpy).toHaveBeenCalledWith(
      '[openclause-console] session detail issue',
      expect.objectContaining({
        stage: 'export:csv',
        sessionId: 'demo',
        tenantId: 'tenant-a',
        requestId: 'req-session-9',
      }),
    )
  })

  it('fails closed when the session summary payload is malformed even if the request succeeds', async () => {
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    stubMutableApi()
    mockApiGet([
      [(path) => path === '/admin/sessions/demo?tenant_id=tenant-a', { session: null }],
      [(path) => path === '/admin/sessions/demo/timeline?tenant_id=tenant-a', {
        events: [
          {
            event_id: 'event-ghost',
            tenant_id: 'tenant-a',
            agent_id: 'agent-1',
            tool: 'slack',
            action: 'msg.post',
            risk_score: 2,
            decision: 'allow',
            session_id: 'demo',
            received_at: '2026-03-23T10:15:00Z',
            explain: 'Should not render without a valid summary',
          },
        ],
      }],
    ])

    renderRoute(<SessionTimeline />, { path: '/sessions/:id', route: '/sessions/demo?tenant_id=tenant-a' })

    expect(await screen.findByText(/session not found/i)).toBeInTheDocument()
    expect(screen.queryByText(/run context/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/should not render without a valid summary/i)).not.toBeInTheDocument()
    expect(warnSpy).toHaveBeenCalledWith(
      '[openclause-console] session detail issue',
      expect.objectContaining({
        stage: 'summary-contract',
        sessionId: 'demo',
        tenantId: 'tenant-a',
        message: 'Malformed session summary payload',
      }),
    )
  })

  it('accepts wrapped summary payloads and still renders approval and execution sections', async () => {
    stubMutableApi()
    mockApiGet([
      [(path) => path === '/admin/sessions/demo?tenant_id=tenant-a', {
        session: {
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
          allow_count: 0,
          deny_count: 0,
          approve_count: 1,
        },
      }],
      [(path) => path === '/admin/sessions/demo/timeline?tenant_id=tenant-a', {
        events: [
          {
            event_id: 'event-approve',
            tenant_id: 'tenant-a',
            agent_id: 'agent-1',
            tool: 'slack',
            action: 'msg.post',
            risk_score: 8,
            decision: 'approve',
            session_id: 'demo',
            received_at: '2026-03-23T10:15:00Z',
            explain: 'Needs approval',
            approval: {
              id: 'approval-1',
              status: 'approved',
              reason: 'Looks safe',
              created_at: '2026-03-23T10:14:00Z',
              expires_at: '2026-03-23T10:20:00Z',
            },
            execution: {
              event_id: 'event-execution-1',
              received_at: '2026-03-23T10:16:00Z',
              status: 'success',
              duration_ms: 120,
            },
          },
        ],
      }],
    ])

    renderRoute(<SessionTimeline />, { path: '/sessions/:id', route: '/sessions/demo?tenant_id=tenant-a' })

    expect(await screen.findByRole('heading', { name: /session detail/i })).toBeInTheDocument()
    expect(screen.getByText(/approval/i, { selector: 'strong' })).toBeInTheDocument()
    expect(screen.getByText(/execution/i, { selector: 'strong' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /open approval/i })).toHaveAttribute('href', expect.stringContaining('approval_id=approval-1'))
    expect(screen.getByRole('link', { name: /open execution event/i })).toHaveAttribute('href', '/events/event-execution-1')
  })

  it('keeps the summary visible when a fulfilled timeline payload is malformed and logs the contract issue', async () => {
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
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
      [(path) => path === '/admin/sessions/demo/timeline?tenant_id=tenant-a', { events: null }],
    ])

    renderRoute(<SessionTimeline />, { path: '/sessions/:id', route: '/sessions/demo?tenant_id=tenant-a' })

    expect(await screen.findByText(/timeline payload was malformed/i)).toBeInTheDocument()
    expect(screen.getByText(/requested by ada/i)).toBeInTheDocument()
    expect(screen.queryByText(/no timeline items match these filters/i)).not.toBeInTheDocument()
    expect(warnSpy).toHaveBeenCalledWith(
      '[openclause-console] session detail issue',
      expect.objectContaining({
        stage: 'timeline-contract',
        sessionId: 'demo',
        tenantId: 'tenant-a',
        message: 'The session summary loaded, but the timeline payload was malformed.',
      }),
    )
  })

  it('keeps valid timeline rows, warns about dropped malformed rows, and shows latest diagnostics', async () => {
    const user = userEvent.setup()
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const writeTextSpy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined)
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
        deny_count: 0,
        approve_count: 1,
      }],
      [(path) => path === '/admin/sessions/demo/timeline?tenant_id=tenant-a', {
        events: [
          {
            event_id: 'event-valid',
            tenant_id: 'tenant-a',
            agent_id: 'agent-1',
            tool: 'slack',
            action: 'msg.post',
            risk_score: 2,
            decision: 'allow',
            session_id: 'demo',
            received_at: '2026-03-23T10:15:00Z',
            explain: 'Valid row',
          },
          {
            event_id: 'event-invalid',
            tenant_id: 'tenant-a',
            agent_id: 'agent-1',
            tool: 'jira',
            action: 'issue.delete',
            risk_score: 'high',
            decision: 'deny',
            session_id: 'demo',
            received_at: '2026-03-23T10:16:00Z',
            explain: 'Malformed row',
          },
        ],
      }],
    ])

    renderRoute(<SessionTimeline />, { path: '/sessions/:id', route: '/sessions/demo?tenant_id=tenant-a' })

    expect(await screen.findByText(/some timeline rows were malformed and were ignored/i)).toBeInTheDocument()
    expect(screen.getByText('slack.msg.post')).toBeInTheDocument()
    expect(screen.queryByText('jira.issue.delete')).not.toBeInTheDocument()
    expect(screen.getByText(/latest diagnostics/i)).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /copy session diagnostics/i }))
    expect(writeTextSpy).toHaveBeenCalledWith(expect.stringContaining('stage=timeline-contract'))
    expect(writeTextSpy).toHaveBeenCalledWith(expect.stringContaining('message=Some timeline rows were malformed and were ignored.'))
    expect(warnSpy).toHaveBeenCalledWith(
      '[openclause-console] session detail issue',
      expect.objectContaining({
        stage: 'timeline-contract',
        droppedRows: 1,
      }),
    )
  })

  it('fails closed when every fulfilled timeline row is malformed instead of showing a filter-empty state', async () => {
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
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
        allow_count: 0,
        deny_count: 0,
        approve_count: 1,
      }],
      [(path) => path === '/admin/sessions/demo/timeline?tenant_id=tenant-a', {
        events: [
          {
            event_id: '',
            tenant_id: 'tenant-a',
            agent_id: 'agent-1',
            tool: 'slack',
            action: 'msg.post',
            risk_score: 2,
            decision: 'approve',
            session_id: 'demo',
            received_at: '',
            explain: 'Malformed row',
          },
        ],
      }],
    ])

    renderRoute(<SessionTimeline />, { path: '/sessions/:id', route: '/sessions/demo?tenant_id=tenant-a' })

    expect(await screen.findByText(/every timeline row was malformed/i)).toBeInTheDocument()
    expect(screen.getByText(/requested by ada/i)).toBeInTheDocument()
    expect(screen.queryByText(/no timeline items match these filters/i)).not.toBeInTheDocument()
    expect(warnSpy).toHaveBeenCalledWith(
      '[openclause-console] session detail issue',
      expect.objectContaining({
        stage: 'timeline-contract',
        droppedRows: 1,
        message: 'The session summary loaded, but every timeline row was malformed.',
      }),
    )
  })

  it('surfaces summary request ids and logs summary fetch failures', async () => {
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    stubMutableApi()
    mockApiGet([
      [(path) => path === '/admin/sessions/demo?tenant_id=tenant-a', () => {
        throw new APIClientError('Session blocked (request id: req-summary-5)', {
          status: 403,
          code: 'forbidden',
          requestId: 'req-summary-5',
          details: { reason: 'scope_denied' },
        })
      }],
      [(path) => path === '/admin/sessions/demo/timeline?tenant_id=tenant-a', { events: [] }],
    ])

    renderRoute(<SessionTimeline />, { path: '/sessions/:id', route: '/sessions/demo?tenant_id=tenant-a' })

    expect(await screen.findByText(/^Session blocked \(request id: req-summary-5\)$/i)).toBeInTheDocument()
    expect(screen.getByText('req-summary-5', { selector: 'code' })).toBeInTheDocument()
    expect(screen.queryByText(/run context/i)).not.toBeInTheDocument()
    expect(warnSpy).toHaveBeenCalledWith(
      '[openclause-console] session detail issue',
      expect.objectContaining({
        stage: 'summary',
        sessionId: 'demo',
        tenantId: 'tenant-a',
        requestId: 'req-summary-5',
      }),
    )
  })

  it('surfaces timeline request ids and shows a repeated-failure triage banner after a second retry miss', async () => {
    const user = userEvent.setup()
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const writeTextSpy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined)
    stubMutableApi()
    let timelineCalls = 0
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
      [(path) => path === '/admin/sessions/demo/timeline?tenant_id=tenant-a', () => {
        timelineCalls += 1
        throw new APIClientError(`Timeline blocked (request id: req-timeline-${timelineCalls})`, {
          status: 502,
          code: 'timeline_unavailable',
          requestId: `req-timeline-${timelineCalls}`,
        })
      }],
    ])

    renderRoute(<SessionTimeline />, { path: '/sessions/:id', route: '/sessions/demo?tenant_id=tenant-a' })

    expect(await screen.findByText(/^Timeline blocked \(request id: req-timeline-1\)$/i)).toBeInTheDocument()
    expect(screen.getByText('req-timeline-1', { selector: 'code' })).toBeInTheDocument()
    expect(screen.getByText(/requested by ada/i)).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /^retry$/i }))

    expect(await screen.findByText(/^Timeline blocked \(request id: req-timeline-2\)$/i)).toBeInTheDocument()
    expect(screen.getByText(/repeated failures detected/i)).toBeInTheDocument()
    expect(screen.getByText(/repeated timeline failures detected for this run/i)).toBeInTheDocument()
    expect(screen.getByText(/latest stage:/i)).toBeInTheDocument()
    expect(screen.getByText('req-timeline-2')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /copy session diagnostics/i }))
    expect(writeTextSpy).toHaveBeenCalledWith(expect.stringContaining('session_id=demo'))
    expect(writeTextSpy).toHaveBeenCalledWith(expect.stringContaining('request_id=req-timeline-2'))
    expect(warnSpy).toHaveBeenCalledWith(
      '[openclause-console] session detail issue',
      expect.objectContaining({
        stage: 'timeline',
        sessionId: 'demo',
        tenantId: 'tenant-a',
        requestId: 'req-timeline-2',
      }),
    )
  })
})
