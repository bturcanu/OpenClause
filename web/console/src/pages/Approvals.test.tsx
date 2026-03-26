import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useSearchParams } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import { mockApiGet } from '../test/mockApi'
import { renderRoute } from '../test/render'
import Approvals from './Approvals'

const approvalFixture = {
  id: 'approval-1',
  event_id: 'event-1',
  tool: 'slack',
  action: 'msg.post',
  risk_score: 9,
  agent_id: 'agent-1',
  tenant_id: 'tenant-1',
  status: 'pending',
  reason: 'Needs operator approval',
  user_id: 'user-1',
  user_name: 'Ada',
  user_email: 'ada@example.com',
  session_id: 'session-1',
  trace_id: 'trace-1',
  created_at: '2026-03-23T10:00:00Z',
  expires_at: '2026-03-23T11:00:00Z',
}

describe('Approvals page', () => {
  it('accepts array-form approvals and handles missing optional detail fields', async () => {
    const user = userEvent.setup()
    mockApiGet([
      ['/admin/approvals/pending', [
        {
          id: 'approval-2',
          event_id: 'event-2',
          tool: 'jira',
          action: 'issue.create',
          risk_score: 5,
          agent_id: 'agent-2',
          tenant_id: 'tenant-2',
          status: 'pending',
          created_at: '2026-03-23T09:00:00Z',
          expires_at: '2026-03-23T12:00:00Z',
        },
      ]],
    ])

    renderRoute(<Approvals />, { path: '/approvals', route: '/approvals' })

    await user.click(await screen.findByRole('button', { name: /review/i }))
    expect(await screen.findByRole('heading', { name: /approval detail/i })).toBeInTheDocument()
    expect(screen.getByText(/session \(none\)/i)).toBeInTheDocument()
    expect(screen.getByText(/trace \(none\)/i)).toBeInTheDocument()
    expect(screen.getByText(/waiting for human review/i)).toBeInTheDocument()
  })

  it('opens the URL-selected approval and refreshes the queue on the polling interval', async () => {
    const intervalSpy = vi.spyOn(window, 'setInterval')
    const getSpy = mockApiGet([
      ['/admin/approvals/pending', { approvals: [approvalFixture] }],
    ])

    renderRoute(<Approvals />, { path: '/approvals', route: '/approvals?approval_id=approval-1' })

    expect(await screen.findByRole('heading', { name: /approval detail/i })).toBeInTheDocument()
    expect(getSpy).toHaveBeenCalledWith('/admin/approvals/pending')
    expect(intervalSpy).toHaveBeenCalledWith(expect.any(Function), 5000)
  })

  it('approves a request, unlocks the execute helper, and copies the generated command', async () => {
    const user = userEvent.setup()
    mockApiGet([
      ['/admin/approvals/pending', { approvals: [approvalFixture] }],
    ])

    const postSpy = vi.spyOn(api, 'post').mockImplementation(async (path) => {
      if (path === '/admin/approvals/approval-1/approve') return {}
      throw new Error(`Unhandled api.post call for ${path}`)
    })

    renderRoute(<Approvals />, { path: '/approvals', route: '/approvals' })

    await user.click(await screen.findByRole('button', { name: /review/i }))
    await user.click(screen.getByRole('button', { name: /approve/i }))

    await waitFor(() => expect(postSpy).toHaveBeenCalledWith('/admin/approvals/approval-1/approve'))
    expect(await screen.findByRole('heading', { name: /execute approved request/i })).toBeInTheDocument()

    const writeTextSpy = vi.spyOn(navigator.clipboard, 'writeText')
    await user.type(screen.getByLabelText(/^gateway api key$/i), 'sk-oc-live-123')
    await user.click(screen.getByRole('button', { name: /copy execute command/i }))

    expect(writeTextSpy).toHaveBeenCalledWith(
      expect.stringContaining('/v1/toolcalls/event-1/execute'),
    )
    expect(writeTextSpy).toHaveBeenCalledWith(
      expect.stringContaining('OPENCLAUSE_BASE_URL="${OPENCLAUSE_BASE_URL:-http://localhost:8080}"'),
    )
    expect(writeTextSpy).toHaveBeenCalledWith(
      expect.stringContaining('X-API-Key: sk-oc-live-123'),
    )
  })

  it('honors the tenant_id query filter for verification deep links', async () => {
    mockApiGet([
      ['/admin/approvals/pending', {
        approvals: [
          approvalFixture,
          { ...approvalFixture, id: 'approval-2', event_id: 'event-2', tenant_id: 'tenant-2', session_id: 'session-2' },
        ],
      }],
    ])

    renderRoute(<Approvals />, { path: '/approvals', route: '/approvals?tenant_id=tenant-1' })

    expect(await screen.findByText(/tenant id: tenant-1/i)).toBeInTheDocument()
    expect(screen.getByText(/slack\.msg\.post/i)).toBeInTheDocument()
    expect(screen.queryByText('approval-2')).not.toBeInTheDocument()
  })

  it('clears a selected approval when a tenant filter hides it', async () => {
    mockApiGet([
      ['/admin/approvals/pending', {
        approvals: [
          approvalFixture,
          { ...approvalFixture, id: 'approval-2', event_id: 'event-2', tenant_id: 'tenant-2', session_id: 'session-2' },
        ],
      }],
    ])

    renderRoute(<Approvals />, { path: '/approvals', route: '/approvals?tenant_id=tenant-1&approval_id=approval-2' })

    expect(await screen.findByText(/tenant id: tenant-1/i)).toBeInTheDocument()
    await waitFor(() => expect(screen.queryByRole('heading', { name: /approval detail/i })).not.toBeInTheDocument())
    expect(screen.queryByText('approval-2')).not.toBeInTheDocument()
    expect(screen.getByText(/slack\.msg\.post/i)).toBeInTheDocument()
  })

  it('clears a URL-selected approval when the route changes to a missing approval id', async () => {
    const user = userEvent.setup()
    mockApiGet([
      ['/admin/approvals/pending', { approvals: [approvalFixture] }],
    ])

    function Harness() {
      const [searchParams, setSearchParams] = useSearchParams()
      return (
        <>
          <button type="button" onClick={() => setSearchParams(new URLSearchParams({ approval_id: 'missing' }))}>
            Switch to missing approval
          </button>
          <Approvals />
        </>
      )
    }

    renderRoute(<Harness />, { path: '/approvals', route: '/approvals?approval_id=approval-1' })

    expect(await screen.findByRole('heading', { name: /approval detail/i })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /switch to missing approval/i }))
    await waitFor(() => expect(screen.queryByRole('heading', { name: /approval detail/i })).not.toBeInTheDocument())
  })

  it('removes only the tenant_id filter chip and preserves other query params', async () => {
    const user = userEvent.setup()
    mockApiGet([
      ['/admin/approvals/pending', {
        approvals: [
          approvalFixture,
          { ...approvalFixture, id: 'approval-2', event_id: 'event-2', tenant_id: 'tenant-2', session_id: 'session-2' },
        ],
      }],
    ])

    renderRoute(<Approvals />, { path: '/approvals', route: '/approvals?tenant_id=tenant-1&approval_id=approval-1' })

    expect(await screen.findByRole('heading', { name: /approval detail/i })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /^tenant id: tenant-1$/i }))

    expect(await screen.findByRole('heading', { name: /approval detail/i })).toBeInTheDocument()
    expect(screen.queryByText(/tenant id: tenant-1/i)).not.toBeInTheDocument()
    expect(screen.getAllByText(/slack\.msg\.post/i).length).toBeGreaterThan(0)
  })
})
