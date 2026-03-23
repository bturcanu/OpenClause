import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import Overview from './Overview'
import { renderRoute } from '../test/render'
import { mockApiGet, stubMutableApi } from '../test/mockApi'

describe('Overview', () => {
  it('pins and unpins chart buckets and links to the audit trail window', async () => {
    const user = userEvent.setup()
    stubMutableApi()
    mockApiGet([
      ['/admin/analytics/overview', {
        total_events: 12,
        allow_count: 8,
        deny_count: 2,
        approve_count: 2,
        pending_approvals: 1,
        active_tenants: 3,
      }],
      ['/admin/analytics/timeseries', {
        buckets: [
          { bucket: '2026-03-23T10:00:00Z', total: 2 },
          { bucket: '2026-03-23T11:00:00Z', total: 5 },
          { bucket: '2026-03-23T12:00:00Z', total: 3 },
        ],
      }],
      ['/admin/events?limit=10', {
        events: [
          {
            event_id: 'event-1',
            tool: 'slack',
            action: 'msg.post',
            decision: 'allow',
            risk_score: 2,
            received_at: '2026-03-23T11:15:00Z',
            tenant_id: 'tenant-1',
          },
        ],
      }],
    ])

    renderRoute(<Overview />, { path: '/', route: '/' })

    expect(await screen.findByRole('heading', { name: /event volume/i })).toBeInTheDocument()

    const bars = Array.from(document.querySelectorAll<HTMLButtonElement>('.event-volume-bar'))
    expect(bars).toHaveLength(3)

    await user.click(bars[1])
    expect(screen.getByText(/selected/i)).toBeInTheDocument()
    const auditTrailLink = screen.getByRole('link', { name: /view events in audit trail/i })
    expect(auditTrailLink.getAttribute('href')).toContain('/events?since=2026-03-23T11%3A00%3A00.000Z&until=2026-03-23T12%3A00%3A00.000Z')

    await user.click(bars[1])
    await waitFor(() => expect(screen.queryByText(/selected/i)).not.toBeInTheDocument())

    bars[0].focus()
    await user.keyboard('[Space]')
    expect(await screen.findByText(/selected/i)).toBeInTheDocument()
  })
})
