import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import App from './App'
import { mockApiGet, stubMutableApi } from './test/mockApi'

function makeToken(payload: Record<string, unknown>) {
  const encoded = btoa(JSON.stringify(payload)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '')
  return `header.${encoded}.signature`
}

function mockSetupStatus(initialized: boolean) {
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
    if (String(input) === '/api/setup/status') {
      return new Response(JSON.stringify({ initialized }), { status: 200, headers: { 'Content-Type': 'application/json' } })
    }
    throw new Error(`Unhandled fetch call for ${String(input)}`)
  }))
}

describe('App shell', () => {
  it('shows the setup wizard when the instance is not initialized', async () => {
    mockSetupStatus(false)

    render(
      <MemoryRouter initialEntries={['/']}>
        <App />
      </MemoryRouter>,
    )

    expect(await screen.findByRole('heading', { name: /first-run setup/i })).toBeInTheDocument()
  })

  it('redirects protected routes to login when there is no stored token', async () => {
    mockSetupStatus(true)

    render(
      <MemoryRouter initialEntries={['/events']}>
        <App />
      </MemoryRouter>,
    )

    expect(await screen.findByText(/sign in to the admin console/i)).toBeInTheDocument()
  })

  it('shows setup-status errors and retries successfully', async () => {
    const user = userEvent.setup()
    localStorage.setItem('oc_token', makeToken({ roles: ['platform_admin'], sid: 'session-1' }))
    let attempts = 0

    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      if (String(input) === '/api/setup/status') {
        attempts += 1
        if (attempts === 1) {
          return new Response(JSON.stringify({ message: 'console-api unavailable' }), {
            status: 503,
            headers: { 'Content-Type': 'application/json' },
          })
        }
        return new Response(JSON.stringify({ initialized: true }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      throw new Error(`Unhandled fetch call for ${String(input)}`)
    }))

    stubMutableApi()
    mockApiGet([
      ['/admin/connectors', { connectors: [{ name: 'slack', type: 'remote', actions: ['msg.post'] }] }],
    ])

    render(
      <MemoryRouter initialEntries={['/connectors']}>
        <App />
      </MemoryRouter>,
    )

    expect(await screen.findByText(/console-api unavailable/i)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /^retry$/i }))
    expect(await screen.findByRole('heading', { name: /connectors/i })).toBeInTheDocument()
  })

  it('renders authenticated routes after setup has completed', async () => {
    localStorage.setItem('oc_token', makeToken({ roles: ['platform_admin'], sid: 'session-1' }))
    mockSetupStatus(true)
    stubMutableApi()
    mockApiGet([
      ['/admin/connectors', { connectors: [{ name: 'slack', type: 'remote', actions: ['msg.post'] }] }],
    ])

    render(
      <MemoryRouter initialEntries={['/connectors']}>
        <App />
      </MemoryRouter>,
    )

    expect(await screen.findByRole('heading', { name: /connectors/i })).toBeInTheDocument()
    await waitFor(() => expect(screen.getByText('slack')).toBeInTheDocument())
  })
})
