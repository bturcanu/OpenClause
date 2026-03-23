import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import Policies from './Policies'
import { renderRoute } from '../test/render'

describe('Policies page', () => {
  it('accepts array tenant responses and honors the tenant_id query param', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/admin/tenants') {
        return [
          { id: 'tenant-1', name: 'Tenant One', status: 'active' },
          { id: 'tenant-2', name: 'Tenant Two', status: 'active' },
        ]
      }
      if (path === '/admin/tenants/tenant-2/policy/config') {
        return {
          max_risk_auto_approve: 5,
          read_actions: ['jira.issue.read'],
          write_actions: ['jira.issue.create'],
          destructive_actions: ['aws.instance.delete'],
          require_destructive_approval: true,
        }
      }
      if (path === '/admin/tenants/tenant-2/policy/versions') {
        return [
          {
            id: 2,
            version: 'v2',
            deployed_by: 'admin@example.com',
            deployed_at: '2026-03-23T12:00:00Z',
            notes: 'Latest',
          },
        ]
      }
      throw new Error(`Unhandled api.get call for ${path}`)
    })

    renderRoute(<Policies />, { path: '/policies', route: '/policies?tenant_id=tenant-2' })

    const tenantCard = screen.getByRole('heading', { name: /^tenant$/i }).closest('.form-card') as HTMLElement | null
    expect(tenantCard).not.toBeNull()
    await waitFor(() => expect(within(tenantCard!).getByLabelText(/^selected tenant$/i)).toHaveValue('tenant-2'))
    expect(await screen.findByRole('button', { name: /^selected$/i })).toBeInTheDocument()
  })

  it('serializes builder textareas for version creation, previews decisions, and rolls back the selected version', async () => {
    const user = userEvent.setup()

    const config = {
      max_risk_auto_approve: 7,
      read_actions: ['jira.issue.read'],
      write_actions: ['jira.issue.create'],
      destructive_actions: ['aws.instance.delete'],
      require_destructive_approval: true,
    }
    let versions = [
      {
        id: 2,
        version: 'v2',
        deployed_by: 'admin@example.com',
        deployed_at: '2026-03-23T12:00:00Z',
        notes: 'Latest',
        policy_data: config,
      },
      {
        id: 1,
        version: 'v1',
        deployed_by: 'admin@example.com',
        deployed_at: '2026-03-20T12:00:00Z',
        notes: 'Initial',
        policy_data: config,
      },
    ]

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/admin/tenants') return { tenants: [{ id: 'tenant-1', name: 'Tenant One', status: 'active' }] }
      if (path === '/admin/tenants/tenant-1/policy/config') return config
      if (path === '/admin/tenants/tenant-1/policy/versions') return versions
      throw new Error(`Unhandled api.get call for ${path}`)
    })

    const postSpy = vi.spyOn(api, 'post').mockImplementation(async (path, payload) => {
      if (path === '/admin/tenants/tenant-1/policy/versions') {
        const versionPayload = payload as {
          version: string
          notes: string
          policy_data: typeof config
        }
        expect(payload).toEqual({
          version: 'v3',
          notes: 'Previewable snapshot',
          policy_data: {
            max_risk_auto_approve: 7,
            read_actions: ['jira.issue.read', 'slack.msg.post'],
            write_actions: ['jira.issue.create'],
            destructive_actions: ['aws.instance.delete'],
            require_destructive_approval: true,
          },
        })
        versions = [
          {
            id: 3,
            version: 'v3',
            deployed_by: 'admin@example.com',
            deployed_at: '2026-03-24T12:00:00Z',
            notes: 'Previewable snapshot',
            policy_data: versionPayload.policy_data,
          },
          ...versions,
        ]
        return {}
      }
      if (path === '/admin/tenants/tenant-1/policy/simulate') {
        expect(payload).toMatchObject({
          tool: 'jira',
          action: 'issue.create',
          risk_score: 8,
          policy_config: {
            read_actions: ['jira.issue.read', 'slack.msg.post'],
          },
        })
        return {
          simulation: true,
          tenant_id: 'tenant-1',
          policy_result: { result: { decision: 'approve', reason: 'Allowed in preview' } },
        }
      }
      if (path === '/admin/tenants/tenant-1/policy/versions/1/rollback') {
        return {}
      }
      throw new Error(`Unhandled api.post call for ${path}`)
    })

    renderRoute(<Policies />, { path: '/policies', route: '/policies' })

    expect(await screen.findByRole('button', { name: /^selected$/i })).toBeInTheDocument()

    const builderCard = screen.getByRole('heading', { name: /rule builder/i }).closest('.form-card') as HTMLElement | null
    expect(builderCard).not.toBeNull()
    expect(within(builderCard!).getByLabelText(/^destructive actions require approval$/i)).toBeChecked()
    await user.clear(within(builderCard!).getByLabelText(/^read allowlist actions \(comma separated\)$/i))
    await user.type(
      within(builderCard!).getByLabelText(/^read allowlist actions \(comma separated\)$/i),
      'JIRA.ISSUE.READ, slack.msg.post',
    )

    const simulatorCard = screen.getByRole('heading', { name: /policy simulator/i }).closest('.form-card') as HTMLElement | null
    expect(simulatorCard).not.toBeNull()
    await user.click(within(simulatorCard!).getByRole('button', { name: /preview decision/i }))
    expect(await within(simulatorCard!).findByText(/allowed in preview/i, { selector: '.policy-sim-reason' })).toBeInTheDocument()

    const createVersionCard = screen.getByRole('heading', { name: /create version/i }).closest('.form-card') as HTMLElement | null
    expect(createVersionCard).not.toBeNull()
    await user.type(within(createVersionCard!).getByLabelText(/^version$/i), 'v3')
    await user.type(within(createVersionCard!).getByLabelText(/^notes$/i), 'Previewable snapshot')
    await user.click(within(createVersionCard!).getByRole('button', { name: /create version snapshot/i }))

    await waitFor(() => expect(postSpy).toHaveBeenCalledWith('/admin/tenants/tenant-1/policy/versions', expect.any(Object)))
    await waitFor(() => expect(screen.getAllByText('v3').length).toBeGreaterThan(0))

    const versionRows = screen.getAllByRole('row')
    const v1Row = versionRows.find((row) => within(row).queryByText('v1'))
    expect(v1Row).toBeTruthy()
    await user.click(within(v1Row!).getByRole('button', { name: /^select$/i }))
    await user.click(screen.getByRole('button', { name: /rollback to selected version/i }))

    await waitFor(() => expect(postSpy).toHaveBeenCalledWith('/admin/tenants/tenant-1/policy/versions/1/rollback', {}))
  })

  it('accepts wrapped policy version payloads without losing the current selection state', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/admin/tenants') {
        return { tenants: [{ id: 'tenant-1', name: 'Tenant One', status: 'active' }] }
      }
      if (path === '/admin/tenants/tenant-1/policy/config') {
        return {
          max_risk_auto_approve: 5,
          read_actions: ['jira.issue.read'],
          write_actions: ['jira.issue.create'],
          destructive_actions: [],
          require_destructive_approval: true,
        }
      }
      if (path === '/admin/tenants/tenant-1/policy/versions') {
        return {
          versions: [
            {
              id: 9,
              version: 'v9',
              deployed_by: 'admin@example.com',
              deployed_at: '2026-03-25T12:00:00Z',
              notes: 'Wrapped payload',
            },
          ],
        }
      }
      throw new Error(`Unhandled api.get call for ${path}`)
    })

    renderRoute(<Policies />, { path: '/policies', route: '/policies' })

    expect(await screen.findByText('Wrapped payload')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^selected$/i })).toBeInTheDocument()
    expect(screen.getByText(/^current$/i, { selector: 'span' })).toBeInTheDocument()
  })
})
