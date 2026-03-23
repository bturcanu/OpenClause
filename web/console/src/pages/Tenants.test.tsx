import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { api } from '../api'
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
})
