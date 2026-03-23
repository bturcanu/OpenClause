import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import { getFieldByLabelText } from '../test/form'
import Login from './Login'
import SetupWizard from './SetupWizard'
import InviteAccept from './InviteAccept'
import PasswordReset from './PasswordReset'

function makeToken(payload: Record<string, unknown>) {
  const encoded = btoa(JSON.stringify(payload)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '')
  return `header.${encoded}.signature`
}

describe('auth pages', () => {
  it('signs in and stores auth state', async () => {
    const user = userEvent.setup()
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      if (String(input) === '/api/auth/login') {
        return new Response(JSON.stringify({
          token: makeToken({ sid: 'session-42', email: 'admin@example.com' }),
          session_id: 'session-42',
        }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      throw new Error(`Unhandled fetch call for ${String(input)}`)
    }))

    render(
      <MemoryRouter initialEntries={['/login']}>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/" element={<div>Signed in</div>} />
        </Routes>
      </MemoryRouter>,
    )

    await user.type(screen.getByPlaceholderText('admin@example.com'), 'admin@example.com')
    await user.type(screen.getByPlaceholderText('••••••••'), 'Admin123!')
    await user.click(screen.getByRole('button', { name: /sign in/i }))

    expect(await screen.findByText('Signed in')).toBeInTheDocument()
    expect(localStorage.getItem('oc_token')).toBeTruthy()
    expect(localStorage.getItem('oc_session_id')).toBe('session-42')
  })

  it('submits setup and navigates to login after success', async () => {
    const user = userEvent.setup()
    const onInitialized = vi.fn()
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      if (String(input) === '/api/setup/initialize') {
        return new Response(JSON.stringify({ initialized: true }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      throw new Error(`Unhandled fetch call for ${String(input)}`)
    }))

    render(
      <MemoryRouter initialEntries={['/setup']}>
        <Routes>
          <Route path="/setup" element={<SetupWizard onInitialized={onInitialized} />} />
          <Route path="/login" element={<div>Back at login</div>} />
        </Routes>
      </MemoryRouter>,
    )

    await user.type(getFieldByLabelText(/platform admin password/i), 'Admin123!')
    await user.click(screen.getByRole('button', { name: /initialize/i }))

    expect(await screen.findByText(/setup complete/i)).toBeInTheDocument()
    expect(onInitialized).toHaveBeenCalled()
  })

  it('accepts invites from the tokenized route and shows tenant-admin guidance', async () => {
    const user = userEvent.setup()
    vi.spyOn(api, 'unauthPost').mockResolvedValue({
      tenant_id: 'tenant-1',
      role: 'tenant_admin',
    })

    render(
      <MemoryRouter initialEntries={['/invite/accept?token=invite-token']}>
        <Routes>
          <Route path="/invite/accept" element={<InviteAccept />} />
        </Routes>
      </MemoryRouter>,
    )

    expect(screen.getByDisplayValue('invite-token')).toBeInTheDocument()
    await user.type(getFieldByLabelText(/^password$/i), 'Admin123!')
    await user.click(screen.getByRole('button', { name: /accept invite/i }))

    expect(await screen.findByText(/you are now tenant_admin/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /tenant api keys/i })).toHaveAttribute('href', '/tenants/tenant-1?tab=api_keys')
  })

  it('requests and confirms a password reset', async () => {
    const user = userEvent.setup()
    const unauthPost = vi.spyOn(api, 'unauthPost').mockImplementation(async (path: string) => {
      if (path === '/auth/reset/request') return {}
      if (path === '/auth/reset/confirm') return {}
      throw new Error(`Unhandled unauthPost call for ${path}`)
    })

    render(
      <MemoryRouter initialEntries={['/reset?token=reset-token']}>
        <Routes>
          <Route path="/reset" element={<PasswordReset />} />
        </Routes>
      </MemoryRouter>,
    )

    await user.type(getFieldByLabelText(/^email$/i), 'user@example.com')
    await user.click(screen.getByRole('button', { name: /^request$/i }))
    expect(await screen.findByText(/reset request created/i)).toBeInTheDocument()

    await user.type(getFieldByLabelText(/new password/i), 'NewPassword123!')
    await user.click(screen.getByRole('button', { name: /update password/i }))
    expect(await screen.findByText(/password updated/i)).toBeInTheDocument()
    await waitFor(() => expect(unauthPost).toHaveBeenCalledTimes(2))
  })
})
