import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import * as ui from '../ui'
import Tenants from './Tenants'
import { renderRoute } from '../test/render'

function rowNames() {
  return screen.getAllByRole('row').slice(1).map(row => within(row).getAllByRole('link')[0].textContent)
}

describe('Tenants page', () => {
  it('supports search, client-side sorting, and open actions', async () => {
    const user = userEvent.setup()

    vi.spyOn(api, 'get').mockResolvedValue({
      tenants: [
        { id: 'tenant-z', name: 'Zulu Labs', status: 'disabled', created_at: '2026-03-21T12:00:00Z' },
        { id: 'tenant-a', name: 'Alpha Corp', status: 'active', created_at: '2026-03-20T12:00:00Z' },
        { id: 'tenant-b', name: 'Beta Works', status: 'active', created_at: '2026-03-22T12:00:00Z' },
      ],
    })

    renderRoute(<Tenants />, { path: '/tenants', route: '/tenants' })

    expect(await screen.findByText('Zulu Labs')).toBeInTheDocument()
    expect(rowNames()).toEqual(['Zulu Labs', 'Alpha Corp', 'Beta Works'])

    await user.click(screen.getByRole('button', { name: /^name$/i }))
    expect(rowNames()).toEqual(['Alpha Corp', 'Beta Works', 'Zulu Labs'])

    await user.type(screen.getByLabelText(/^search$/i), 'disabled')
    expect(screen.getByText('Zulu Labs')).toBeInTheDocument()
    expect(screen.queryByText('Alpha Corp')).not.toBeInTheDocument()

    const openLink = screen.getByRole('link', { name: /open tenant/i })
    expect(openLink).toHaveAttribute('href', '/tenants/tenant-z')
  })

  it('creates a tenant and refreshes the list', async () => {
    const user = userEvent.setup()

    let tenants = [{ id: 'tenant-a', name: 'Alpha Corp', status: 'active', created_at: '2026-03-20T12:00:00Z' }]

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/admin/tenants') {
        return { tenants }
      }
      throw new Error(`Unhandled api.get call for ${path}`)
    })

    const postSpy = vi.spyOn(api, 'post').mockImplementation(async (path, payload) => {
      if (path === '/admin/tenants') {
        expect(payload).toEqual({ name: 'Gamma Ops' })
        tenants = [
          ...tenants,
          { id: 'tenant-g', name: 'Gamma Ops', status: 'active', created_at: '2026-03-23T12:00:00Z' },
        ]
        return {}
      }
      throw new Error(`Unhandled api.post call for ${path}`)
    })

    renderRoute(<Tenants />, { path: '/tenants', route: '/tenants' })

    expect(await screen.findByText('Alpha Corp')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /\+ new tenant/i }))
    await user.type(screen.getByLabelText(/^name$/i), 'Gamma Ops')
    await user.click(screen.getByRole('button', { name: /^create$/i }))

    await waitFor(() => expect(postSpy).toHaveBeenCalledWith('/admin/tenants', { name: 'Gamma Ops' }))
    expect(await screen.findByText('Gamma Ops')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /^create$/i })).not.toBeInTheDocument()
  })

  it('shows empty states for no tenants and no search matches', async () => {
    const user = userEvent.setup()
    const getSpy = vi.spyOn(api, 'get')

    getSpy.mockResolvedValue({ tenants: [] })

    renderRoute(<Tenants />, { path: '/tenants', route: '/tenants' })

    expect(await screen.findByText(/no tenants yet/i)).toBeInTheDocument()

    getSpy.mockResolvedValueOnce({
      tenants: [{ id: 'tenant-a', name: 'Alpha Corp', status: 'active', created_at: '2026-03-20T12:00:00Z' }],
    })

    await user.click(screen.getByRole('button', { name: /refresh/i }))
    expect(await screen.findByText('Alpha Corp')).toBeInTheDocument()

    await user.type(screen.getByLabelText(/^search$/i), 'missing')
    expect(screen.getByText(/no tenants match this search/i)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /clear search/i }))
    expect(screen.getByText('Alpha Corp')).toBeInTheDocument()
  })

  it('defaults onboarding to inline tenant creation when no tenant options exist', async () => {
    const user = userEvent.setup()

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/admin/tenants') return { tenants: [] }
      if (path === '/admin/connectors') return [{ name: 'slack', actions: ['slack.channel.list'], type: 'remote' }]
      throw new Error(`Unhandled api.get call for ${path}`)
    })

    renderRoute(<Tenants />, { path: '/tenants', route: '/tenants' })

    expect(await screen.findByText(/no tenants yet/i)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Connect Agent' }))

    const modalHeading = await screen.findByRole('heading', { name: /connect agent/i })
    const modal = modalHeading.closest('.modal') as HTMLElement

    expect(within(modal).getByLabelText(/new tenant name/i)).toBeInTheDocument()
    expect(within(modal).queryByLabelText(/^tenant$/i)).not.toBeInTheDocument()
    expect(within(modal).queryByRole('button', { name: /review starter files/i })).not.toBeInTheDocument()
    expect(within(modal).getByRole('button', { name: /^connect agent$/i })).toBeDisabled()
  })

  it('opens the onboarding flow, previews artifacts, and renders verification links', async () => {
    const user = userEvent.setup()
    const downloadSpy = vi.spyOn(ui, 'downloadBlob').mockImplementation(() => {})
    const writeTextSpy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined)

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/admin/tenants') {
        return {
          tenants: [{ id: 'tenant-a', name: 'Alpha Corp', status: 'active', created_at: '2026-03-20T12:00:00Z' }],
        }
      }
      if (path === '/admin/connectors') {
        return [
          { name: 'slack', actions: ['slack.channel.list', 'slack.msg.post'], type: 'remote' },
          { name: 'github', actions: ['issue.create'], type: 'builtin' },
        ]
      }
      throw new Error(`Unhandled api.get call for ${path}`)
    })

    vi.spyOn(api, 'post').mockImplementation(async (path) => {
      if (path === '/admin/onboarding/bundles/preview') {
        return {
          mode: 'preview',
          tenant: { id: 'tenant-a', name: 'Alpha Corp', created: false },
          agent: { id: 'preview-support-bot', name: 'Support Bot', status: 'preview', preview: true },
          bundle: {
            runtime: 'python',
            runtime_label: 'Python SDK wrapper',
            starter_file_name: 'agent.py',
            environment: {
              OPENCLAUSE_BASE_URL: 'http://localhost:8080',
              OPENCLAUSE_TENANT_ID: 'tenant-a',
              OPENCLAUSE_AGENT_ID: 'preview-support-bot',
              OPENCLAUSE_API_KEY: '${OPENCLAUSE_API_KEY:-generated-on-create}',
            },
            environment_script: 'export OPENCLAUSE_BASE_URL="http://localhost:8080"',
            environment_file: 'OPENCLAUSE_BASE_URL="http://localhost:8080"',
            starter_snippet: 'def governed_call():\n    pass',
            readme_snippet: '# Quick start',
            sample_call: 'curl -sS "$OPENCLAUSE_BASE_URL/v1/toolcalls"',
            artifacts: [
              { id: 'env-script', label: 'Environment shell exports', file_name: 'setup-env.sh', kind: 'environment_script', content: 'export OPENCLAUSE_BASE_URL="http://localhost:8080"' },
              { id: 'starter', label: 'Starter runtime file', file_name: 'agent.py', kind: 'starter_file', content: 'def governed_call():\n    pass' },
            ],
            verification_checklist: ['Open Audit Trail', 'Open Sessions'],
            verification_links: [
              { label: 'Open Audit Trail', path: '/events?agent_id=preview-support-bot&tenant_id=tenant-a' },
              { label: 'Open Sessions', path: '/sessions?agent_id=preview-support-bot&tenant_id=tenant-a' },
              { label: 'Open Approvals', path: '/approvals?tenant_id=tenant-a' },
            ],
            notes: ['Starter bundle note'],
          },
        }
      }
      if (path === '/admin/onboarding/integrations') {
        return {
          mode: 'created',
          tenant: { id: 'tenant-a', name: 'Alpha Corp', created: false },
          agent: { id: 'agent-onboarded', name: 'Support Bot', status: 'active', created_at: '2026-03-23T12:00:00Z', preview: false },
          api_key: { id: 'key-1', name: 'Support Bot onboarding key', key_prefix: 'sk-oc-demo', raw_key: 'sk-oc-demo-raw' },
          bundle: {
            runtime: 'python',
            runtime_label: 'Python SDK wrapper',
            starter_file_name: 'agent.py',
            environment: {
              OPENCLAUSE_BASE_URL: 'http://localhost:8080',
              OPENCLAUSE_TENANT_ID: 'tenant-a',
              OPENCLAUSE_AGENT_ID: 'agent-onboarded',
              OPENCLAUSE_API_KEY: 'sk-oc-demo-raw',
            },
            environment_script: 'export OPENCLAUSE_BASE_URL="http://localhost:8080"',
            environment_file: 'OPENCLAUSE_BASE_URL="http://localhost:8080"',
            starter_snippet: 'def governed_call():\n    pass',
            readme_snippet: '# Quick start',
            sample_call: 'curl -sS "$OPENCLAUSE_BASE_URL/v1/toolcalls"',
            artifacts: [
              { id: 'env-script', label: 'Environment shell exports', file_name: 'setup-env.sh', kind: 'environment_script', content: 'export OPENCLAUSE_BASE_URL="http://localhost:8080"' },
              { id: 'starter', label: 'Starter runtime file', file_name: 'agent.py', kind: 'starter_file', content: 'def governed_call():\n    pass' },
            ],
            verification_checklist: ['Open Audit Trail', 'Open Sessions'],
            verification_links: [
              { label: 'Open Audit Trail', path: '/events?agent_id=agent-onboarded&tenant_id=tenant-a' },
              { label: 'Open Sessions', path: '/sessions?agent_id=agent-onboarded&tenant_id=tenant-a' },
              { label: 'Open Approvals', path: '/approvals?tenant_id=tenant-a' },
            ],
            notes: ['Starter bundle note'],
          },
        }
      }
      throw new Error(`Unhandled api.post call for ${path}`)
    })

    vi.spyOn(api, 'postBlob').mockResolvedValue(new Blob(['bundle'], { type: 'application/zip' }))

    renderRoute(<Tenants />, { path: '/tenants', route: '/tenants' })

    expect(await screen.findByText('Alpha Corp')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Connect Agent' }))
    const modalHeading = await screen.findByRole('heading', { name: /connect agent/i })
    const modal = modalHeading.closest('.modal') as HTMLElement
    expect(within(modal).queryByRole('button', { name: /review starter files/i })).not.toBeInTheDocument()
    await user.selectOptions(within(modal).getByLabelText(/^tenant$/i), 'tenant-a')
    await user.type(within(modal).getByLabelText(/agent name/i), 'Support Bot')
    await user.click(within(modal).getByRole('button', { name: /open advanced setup/i }))
    await user.click(within(modal).getByRole('button', { name: /review starter files/i }))

    expect(await screen.findByRole('heading', { name: /copy env/i })).toBeInTheDocument()
    expect(screen.getByText(/review only: nothing has been created yet/i)).toBeInTheDocument()
    expect(screen.getByText(/python sdk wrapper starter bundle/i)).toBeInTheDocument()
    expect(screen.getByText(/goal: send one call, see one event, see one session/i)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /^files$/i }))
    expect(screen.getAllByText(/setup-env\.sh/i).length).toBeGreaterThan(0)
    await user.click(screen.getByRole('button', { name: /starter runtime file/i }))
    expect(screen.getByText(/def governed_call/i)).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /3\. verify/i }))
    expect(screen.getByRole('link', { name: /open audit trail/i })).toHaveAttribute('href', '/events?agent_id=preview-support-bot&tenant_id=tenant-a')
    expect(screen.getByRole('link', { name: /open sessions/i })).toHaveAttribute('href', '/sessions?agent_id=preview-support-bot&tenant_id=tenant-a')
    expect(screen.getByRole('link', { name: /open approvals/i })).toHaveAttribute('href', '/approvals?tenant_id=tenant-a')

    await user.click(screen.getByRole('button', { name: /download starter files/i }))
    expect(downloadSpy).toHaveBeenCalled()

    await user.click(within(modal).getByRole('button', { name: /^connect agent$/i }))
    expect(await screen.findByRole('heading', { name: /one-time api key/i })).toBeInTheDocument()
    expect(screen.getByText(/this full key is only returned during connect/i)).toBeInTheDocument()
    expect(screen.getByText(/sk-oc-demo-raw/i)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /copy onboarding api key/i }))
    expect(writeTextSpy).toHaveBeenCalledWith('sk-oc-demo-raw')

    await user.click(screen.getByRole('button', { name: /adjust setup/i }))
    expect(within(modal).getByLabelText(/agent name/i)).toHaveValue('Support Bot')
    expect(within(modal).getAllByText(/slack channel list/i).length).toBeGreaterThan(0)
  }, 20000)

  it('explains when curated tools are unavailable and keeps bundle actions disabled', async () => {
    const user = userEvent.setup()

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/admin/tenants') {
        return {
          tenants: [{ id: 'tenant-a', name: 'Alpha Corp', status: 'active', created_at: '2026-03-20T12:00:00Z' }],
        }
      }
      if (path === '/admin/connectors') return []
      throw new Error(`Unhandled api.get call for ${path}`)
    })

    renderRoute(<Tenants />, { path: '/tenants', route: '/tenants' })

    expect(await screen.findByText('Alpha Corp')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Connect Agent' }))
    const modalHeading = await screen.findByRole('heading', { name: /connect agent/i })
    const modal = modalHeading.closest('.modal') as HTMLElement
    await user.selectOptions(within(modal).getByLabelText(/^tenant$/i), 'tenant-a')
    await user.type(within(modal).getByLabelText(/agent name/i), 'Support Bot')

    expect(screen.getByText(/no recommended starter pack is available yet/i)).toBeInTheDocument()
    expect(within(modal).queryByRole('button', { name: /review starter files/i })).not.toBeInTheDocument()
    expect(within(modal).getByRole('button', { name: /^connect agent$/i })).toBeDisabled()
  })

  it('opens onboarding directly from the overview query shortcut and does not reopen after close', async () => {
    const user = userEvent.setup()

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/admin/tenants') {
        return {
          tenants: [{ id: 'tenant-a', name: 'Alpha Corp', status: 'active', created_at: '2026-03-20T12:00:00Z' }],
        }
      }
      if (path === '/admin/connectors') {
        return [{ name: 'slack', actions: ['slack.channel.list', 'slack.msg.post'], type: 'remote' }]
      }
      throw new Error(`Unhandled api.get call for ${path}`)
    })

    renderRoute(<Tenants />, { path: '/tenants', route: '/tenants?onboarding=1' })

    expect(await screen.findByRole('heading', { name: /connect agent/i })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /^close$/i }))
    await waitFor(() => expect(screen.queryByRole('heading', { name: /connect agent/i })).not.toBeInTheDocument())
  })

  it('supports the TypeScript golden path in the onboarding preview flow', async () => {
    const user = userEvent.setup()

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/admin/tenants') {
        return {
          tenants: [{ id: 'tenant-a', name: 'Alpha Corp', status: 'active', created_at: '2026-03-20T12:00:00Z' }],
        }
      }
      if (path === '/admin/connectors') {
        return [{ name: 'slack', actions: ['slack.channel.list', 'slack.msg.post'], type: 'remote' }]
      }
      throw new Error(`Unhandled api.get call for ${path}`)
    })

    vi.spyOn(api, 'post').mockImplementation(async (path, payload) => {
      if (path !== '/admin/onboarding/bundles/preview') {
        throw new Error(`Unhandled api.post call for ${path}`)
      }
      expect(payload).toMatchObject({ runtime: 'typescript' })
      return {
        mode: 'preview',
        tenant: { id: 'tenant-a', name: 'Alpha Corp', created: false },
        agent: { id: 'preview-node-bot', name: 'Node Bot', status: 'preview', preview: true },
        bundle: {
          runtime: 'typescript',
          runtime_label: 'TypeScript SDK wrapper',
          starter_file_name: 'agent.ts',
          environment: {
            OPENCLAUSE_BASE_URL: 'http://localhost:8080',
            OPENCLAUSE_TENANT_ID: 'tenant-a',
            OPENCLAUSE_AGENT_ID: 'preview-node-bot',
            OPENCLAUSE_API_KEY: '${OPENCLAUSE_API_KEY:-generated-on-create}',
          },
          environment_script: 'export OPENCLAUSE_BASE_URL="http://localhost:8080"',
          environment_file: 'OPENCLAUSE_BASE_URL="http://localhost:8080"',
          starter_snippet: 'import { OpenClauseClient } from "openclause"',
          readme_snippet: '# Quick start',
          sample_call: 'curl -sS "$OPENCLAUSE_BASE_URL/v1/toolcalls"',
          artifacts: [
            { id: 'starter', label: 'Starter runtime file', file_name: 'agent.ts', kind: 'starter_file', content: 'import { OpenClauseClient } from "openclause"' },
            { id: 'package-snippet', label: 'Package snippet', file_name: 'package.onboarding.json', kind: 'package_snippet', content: '{ "dependencies": { "openclause": "latest" } }' },
          ],
          verification_checklist: ['Open Audit Trail'],
          verification_links: [
            { label: 'Open Audit Trail', path: '/events?agent_id=preview-node-bot&tenant_id=tenant-a' },
            { label: 'Open Sessions', path: '/sessions?agent_id=preview-node-bot&tenant_id=tenant-a' },
            { label: 'Open Approvals', path: '/approvals?tenant_id=tenant-a' },
          ],
          notes: ['TypeScript preview note'],
        },
      }
    })

    renderRoute(<Tenants />, { path: '/tenants', route: '/tenants' })

    expect(await screen.findByText('Alpha Corp')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Connect Agent' }))
    const modalHeading = await screen.findByRole('heading', { name: /connect agent/i })
    const modal = modalHeading.closest('.modal') as HTMLElement
    await user.click(within(modal).getByRole('radio', { name: /typescript \/ node service/i }))
    await user.selectOptions(within(modal).getByLabelText(/^tenant$/i), 'tenant-a')
    await user.type(within(modal).getByLabelText(/agent name/i), 'Node Bot')
    await user.click(within(modal).getByRole('button', { name: /open advanced setup/i }))
    await user.click(within(modal).getByRole('button', { name: /review starter files/i }))

    expect(await screen.findByText(/typescript sdk wrapper starter bundle/i)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /^files$/i }))
    expect(screen.getByRole('button', { name: /package snippet/i })).toBeInTheDocument()
    expect(screen.getAllByText(/agent\.ts/i).length).toBeGreaterThan(0)
  })

  it('opens tenant-scoped onboarding directly from a tenant row without reselecting the tenant', async () => {
    const user = userEvent.setup()

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/admin/tenants') {
        return {
          tenants: [
            { id: 'tenant-a', name: 'Alpha Corp', status: 'active', created_at: '2026-03-20T12:00:00Z' },
            { id: 'tenant-b', name: 'Beta Works', status: 'active', created_at: '2026-03-21T12:00:00Z' },
          ],
        }
      }
      if (path === '/admin/connectors') {
        return [{ name: 'slack', actions: ['slack.channel.list'], type: 'remote' }]
      }
      throw new Error(`Unhandled api.get call for ${path}`)
    })

    vi.spyOn(api, 'post').mockImplementation(async (path, payload) => {
      if (path !== '/admin/onboarding/bundles/preview') {
        throw new Error(`Unhandled api.post call for ${path}`)
      }
      expect(payload).toMatchObject({
        tenant_id: 'tenant-b',
        agent_name: 'Beta Bot',
        runtime: 'python',
      })
      return {
        mode: 'preview',
        tenant: { id: 'tenant-b', name: 'Beta Works', created: false },
        agent: { id: 'preview-beta-bot', name: 'Beta Bot', status: 'preview', preview: true },
        bundle: {
          runtime: 'python',
          runtime_label: 'Python SDK wrapper',
          starter_file_name: 'agent.py',
          environment: {
            OPENCLAUSE_BASE_URL: 'http://localhost:8080',
            OPENCLAUSE_TENANT_ID: 'tenant-b',
            OPENCLAUSE_AGENT_ID: 'preview-beta-bot',
            OPENCLAUSE_API_KEY: '${OPENCLAUSE_API_KEY:-generated-on-create}',
          },
          environment_script: 'export OPENCLAUSE_BASE_URL="http://localhost:8080"',
          environment_file: 'OPENCLAUSE_BASE_URL="http://localhost:8080"',
          starter_snippet: 'def governed_call():\n    pass',
          readme_snippet: '# Quick start',
          sample_call: 'curl -sS "$OPENCLAUSE_BASE_URL/v1/toolcalls"',
          artifacts: [
            { id: 'starter', label: 'Starter runtime file', file_name: 'agent.py', kind: 'starter_file', content: 'def governed_call():\n    pass' },
          ],
          verification_checklist: ['Open Audit Trail'],
          verification_links: [
            { label: 'Open Audit Trail', path: '/events?agent_id=preview-beta-bot&tenant_id=tenant-b' },
            { label: 'Open Sessions', path: '/sessions?agent_id=preview-beta-bot&tenant_id=tenant-b' },
            { label: 'Open Approvals', path: '/approvals?tenant_id=tenant-b' },
          ],
          notes: ['Tenant row preview note'],
        },
      }
    })

    renderRoute(<Tenants />, { path: '/tenants', route: '/tenants' })

    expect(await screen.findByText('Alpha Corp')).toBeInTheDocument()
    const betaRow = screen.getByRole('link', { name: 'Beta Works' }).closest('tr')
    expect(betaRow).not.toBeNull()
    await user.click(within(betaRow as HTMLElement).getByRole('button', { name: /connect agent/i }))

    const modal = screen.getByRole('heading', { name: /connect agent/i }).closest('.modal')
    expect(modal).not.toBeNull()
    expect(screen.getByText(/using current tenant/i)).toBeInTheDocument()
    expect(within(modal as HTMLElement).getByText('Beta Works')).toBeInTheDocument()
    expect(screen.queryByLabelText(/^tenant$/i)).not.toBeInTheDocument()

    await user.type(screen.getByLabelText(/agent name/i), 'Beta Bot')
    await user.click(screen.getByRole('button', { name: /open advanced setup/i }))
    await user.click(screen.getByRole('button', { name: /review starter files/i }))

    expect(await screen.findByText(/python sdk wrapper starter bundle/i)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /3\. verify/i }))
    expect(screen.getByRole('link', { name: /open sessions/i })).toHaveAttribute('href', '/sessions?agent_id=preview-beta-bot&tenant_id=tenant-b')
  })
})
