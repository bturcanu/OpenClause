import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
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
    await user.type(screen.getByPlaceholderText(/sk-oc/i), 'sk-oc-live-123')
    await user.click(screen.getByRole('button', { name: /copy execute command/i }))

    expect(writeTextSpy).toHaveBeenCalledWith(
      expect.stringContaining('/v1/toolcalls/event-1/execute'),
    )
    expect(writeTextSpy).toHaveBeenCalledWith(
      expect.stringContaining('X-API-Key: sk-oc-live-123'),
    )
  })
})
