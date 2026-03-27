import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import Overview from './Overview'
import { renderRoute } from '../test/render'
import { mockApiGet, stubMutableApi } from '../test/mockApi'

describe('Overview', () => {
  it('exposes a tiny onboarding CTA that routes into the existing tenants flow', async () => {
    stubMutableApi()
    mockApiGet([
      ['/admin/analytics/overview', {
        total_events: 0,
        allow_count: 0,
        deny_count: 0,
        approve_count: 0,
        pending_approvals: 0,
        active_tenants: 0,
      }],
      ['/admin/analytics/timeseries', { buckets: [] }],
      ['/admin/events?limit=10', { events: [] }],
    ])

    renderRoute(<Overview />, { path: '/', route: '/' })

    const [onboardingLink, checklistLink] = await screen.findAllByRole('link', { name: /connect agent/i })
    expect(onboardingLink).toHaveAttribute('href', '/tenants?onboarding=1')
    expect(screen.getByRole('link', { name: /view tenants/i })).toHaveAttribute('href', '/tenants')
    expect(screen.getByText(/connect one governed agent first/i)).toBeInTheDocument()
    expect(checklistLink).toHaveAttribute('href', '/tenants?onboarding=1')
  })

  it('adapts the getting-started guidance once traffic and approvals exist', async () => {
    mockApiGet([
      ['/admin/analytics/overview', {
        total_events: 12,
        allow_count: 8,
        deny_count: 1,
        approve_count: 3,
        pending_approvals: 2,
        active_tenants: 1,
      }],
      ['/admin/analytics/timeseries', { buckets: [] }],
      ['/admin/events?limit=10', { events: [] }],
    ])

    renderRoute(<Overview />, { path: '/', route: '/' })

    expect(await screen.findByText(/traffic is flowing\. clear pending approvals/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /open approvals/i })).toHaveAttribute('href', '/approvals')
  })

  it('does not claim the pilot is configured just because tenants exist without traffic', async () => {
    mockApiGet([
      ['/admin/analytics/overview', {
        total_events: 0,
        allow_count: 0,
        deny_count: 0,
        approve_count: 0,
        pending_approvals: 0,
        active_tenants: 1,
      }],
      ['/admin/analytics/timeseries', { buckets: [] }],
      ['/admin/events?limit=10', { events: [] }],
    ])

    renderRoute(<Overview />, { path: '/', route: '/' })

    expect(await screen.findByText(/connect one governed agent first/i)).toBeInTheDocument()
    expect(screen.getAllByRole('link', { name: /connect agent/i })[0]).toHaveAttribute('href', '/tenants?onboarding=1')
  })

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

    await user.tab()
    await user.keyboard('[Space]')
    expect(await screen.findByText(/selected/i)).toBeInTheDocument()
  })

  it('keeps partial-success data visible without showing the no-activity empty state when chart loading fails', async () => {
    mockApiGet([
      ['/admin/analytics/overview', {
        total_events: 12,
        allow_count: 8,
        deny_count: 2,
        approve_count: 2,
        pending_approvals: 1,
        active_tenants: 3,
      }],
      ['/admin/analytics/timeseries', () => {
        throw new Error('Timeseries unavailable')
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

    expect(await screen.findByText(/some dashboard data could not be loaded: event volume chart/i)).toBeInTheDocument()
    expect(screen.getByText('slack.msg.post')).toBeInTheDocument()
    expect(screen.queryByText(/not enough activity yet/i)).not.toBeInTheDocument()
  })

  it('accepts direct-array analytics payloads for timeseries and recent events', async () => {
    mockApiGet([
      ['/admin/analytics/overview', {
        total_events: 3,
        allow_count: 2,
        deny_count: 1,
        approve_count: 0,
        pending_approvals: 0,
        active_tenants: 1,
      }],
      ['/admin/analytics/timeseries', [
        { bucket: '2026-03-23T10:00:00Z', total: 1 },
        { bucket: '2026-03-23T11:00:00Z', total: 2 },
      ]],
      ['/admin/events?limit=10', [
        {
          event_id: 'event-array-1',
          tool: 'jira',
          action: 'issue.create',
          decision: 'deny',
          risk_score: 7,
          received_at: '2026-03-23T11:15:00Z',
          tenant_id: 'tenant-array',
        },
      ]],
    ])

    renderRoute(<Overview />, { path: '/', route: '/' })

    expect(await screen.findByRole('heading', { name: /event volume/i })).toBeInTheDocument()
    expect(screen.getByText('jira.issue.create')).toBeInTheDocument()
    expect(screen.getByText('3')).toBeInTheDocument()
  })

  it('falls back to a one-hour bucket window when only one chart bucket is available', async () => {
    const user = userEvent.setup()
    mockApiGet([
      ['/admin/analytics/overview', {
        total_events: 1,
        allow_count: 1,
        deny_count: 0,
        approve_count: 0,
        pending_approvals: 0,
        active_tenants: 1,
      }],
      ['/admin/analytics/timeseries', {
        buckets: [{ bucket: '2026-03-23T10:00:00Z', total: 1 }],
      }],
      ['/admin/events?limit=10', { events: [] }],
    ])

    renderRoute(<Overview />, { path: '/', route: '/' })

    expect(await screen.findByRole('heading', { name: /event volume/i })).toBeInTheDocument()
    const bars = Array.from(document.querySelectorAll<HTMLButtonElement>('.event-volume-bar'))
    expect(bars).toHaveLength(1)

    await user.click(bars[0])
    expect(screen.getByRole('link', { name: /view events in audit trail/i }).getAttribute('href')).toContain(
      '/events?since=2026-03-23T10%3A00%3A00.000Z&until=2026-03-23T11%3A00%3A00.000Z',
    )
  })
})
