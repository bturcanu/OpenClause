import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import SessionTimeline from './SessionTimeline'
import { APIClientError } from '../api'
import { renderRoute } from '../test/render'
import { getFieldByLabelText } from '../test/form'
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

    await user.selectOptions(getFieldByLabelText(/^tenant$/i), 'tenant-b')

    await waitFor(() => expect(getSpy).toHaveBeenCalledWith('/admin/sessions/demo?tenant_id=tenant-b'))
    expect(await screen.findByText('slack.msg.post')).toBeInTheDocument()
  })
})
