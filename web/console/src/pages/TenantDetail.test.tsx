import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import TenantDetail from './TenantDetail'
import { renderRoute } from '../test/render'
import { getFieldByLabelText } from '../test/form'
import { mockApiGet, stubMutableApi } from '../test/mockApi'

function mockTenantDetailApi() {
  return mockApiGet([
    ['/admin/tenants/tenant-1', {
      id: 'tenant-1',
      name: 'Tenant One',
      status: 'active',
      config: {},
      created_at: '2026-03-20T12:00:00Z',
    }],
    [(path) => path === '/admin/tenants/tenant-1/agents?include_disabled=true', {
      agents: [
        { id: 'agent-active', name: 'Agent Active', tenant_id: 'tenant-1', status: 'active', created_at: '2026-03-22T12:00:00Z' },
        { id: 'agent-disabled', name: 'Agent Disabled', tenant_id: 'tenant-1', status: 'disabled', created_at: '2026-03-21T12:00:00Z' },
      ],
    }],
    [(path) => path === '/admin/tenants/tenant-1/agents?include_disabled=false', {
      agents: [
        { id: 'agent-active', name: 'Agent Active', tenant_id: 'tenant-1', status: 'active', created_at: '2026-03-22T12:00:00Z' },
      ],
    }],
    ['/admin/tenants/tenant-1/apikeys', { api_keys: [] }],
    ['/admin/tenants/tenant-1/approvers', []],
    ['/admin/tenants/tenant-1/notification-config', { approver_group: '', notify: [] }],
    [(path) => path.startsWith('/admin/tenants/tenant-1/analytics/summary'), {
      range_start: '2026-03-22T12:00:00Z',
      range_end: '2026-03-23T12:00:00Z',
      totals: { total_events: 8, allow_count: 5, deny_count: 2, approve_count: 1 },
      trend: [],
      risk_heatmap: [],
      per_agent: [],
      onboarding_checklist: {
        has_api_key: true,
        has_approver: true,
        has_toolcall: true,
        has_approval: false,
        has_execution: false,
      },
    }],
  ])
}

describe('Tenant detail page', () => {
  it('hides disabled agents on demand and refetches the scoped list', async () => {
    const user = userEvent.setup()
    stubMutableApi()
    const getSpy = mockTenantDetailApi()

    renderRoute(<TenantDetail />, { path: '/tenants/:id', route: '/tenants/tenant-1' })

    expect(await screen.findByText('Agent Active')).toBeInTheDocument()
    expect(screen.getByText('Agent Disabled')).toBeInTheDocument()

    await user.click(screen.getByRole('checkbox', { name: /hide disabled/i }))

    await waitFor(() => expect(getSpy).toHaveBeenCalledWith('/admin/tenants/tenant-1/agents?include_disabled=false'))
    await waitFor(() => expect(screen.queryByText('Agent Disabled')).not.toBeInTheDocument())
  })

  it('loads analytics for the selected tab and updates the range preset', async () => {
    const user = userEvent.setup()
    stubMutableApi()
    const getSpy = mockTenantDetailApi()

    renderRoute(<TenantDetail />, { path: '/tenants/:id', route: '/tenants/tenant-1?tab=analytics' })

    expect(await screen.findByRole('heading', { name: /tenant analytics/i })).toBeInTheDocument()
    expect(await screen.findByText(/resolved utc range/i)).toBeInTheDocument()

    await user.selectOptions(getFieldByLabelText(/^range$/i), '48')

    await waitFor(() =>
      expect(getSpy).toHaveBeenCalledWith(
        '/admin/tenants/tenant-1/analytics/summary?range=48h&bucket_minutes=60&top_agents=5',
      ),
    )
  })
})
