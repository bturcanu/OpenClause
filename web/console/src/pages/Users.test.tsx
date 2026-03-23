import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import { mockApiGet } from '../test/mockApi'
import Users from './Users'

function makeToken(payload: Record<string, unknown>) {
  const encoded = btoa(JSON.stringify(payload)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '')
  return `header.${encoded}.signature`
}

function getFieldIn(container: HTMLElement, labelText: RegExp | string) {
  const label = within(container).getByText(labelText, { selector: 'label' })
  const control = label.parentElement?.querySelector('input, select, textarea')
  if (!control) throw new Error(`No form control found for ${String(labelText)}`)
  return control as HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement
}

describe('Users page', () => {
  it('accepts array-form user and auth-session payloads at the API boundary', async () => {
    const user = userEvent.setup()
    localStorage.setItem('oc_token', makeToken({ roles: ['platform_admin'], sid: 'current-session' }))

    mockApiGet([
      ['/admin/users', [
        {
          id: 'user-1',
          email: 'ada@example.com',
          name: 'Ada Lovelace',
          status: 'active',
          created_at: '2026-03-20T12:00:00Z',
          active_session_count: 1,
          roles: [{ id: 'role-1', user_id: 'user-1', role: 'platform_admin' }],
        },
      ]],
      ['/admin/auth-sessions?user_id=user-1', [
        {
          id: 'current-session',
          user_id: 'user-1',
          tenant_id: 'tenant-demo',
          user_agent: 'Firefox',
          client_ip: '127.0.0.1',
          created_at: '2026-03-23T10:00:00Z',
          last_seen_at: '2026-03-23T12:00:00Z',
          expires_at: '2026-03-24T10:00:00Z',
        },
      ]],
    ])

    render(
      <MemoryRouter initialEntries={['/users']}>
        <Routes>
          <Route path="/users" element={<Users />} />
        </Routes>
      </MemoryRouter>,
    )

    expect(await screen.findByText('Ada Lovelace')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /review/i }))
    expect(await screen.findByRole('heading', { name: /active login sessions/i })).toBeInTheDocument()
    expect(screen.getByText(/firefox/i)).toBeInTheDocument()
  })

  it('creates invites, refreshes the list, and shows one-time token/link messaging', async () => {
    const user = userEvent.setup()
    localStorage.setItem('oc_token', makeToken({ roles: ['platform_admin'], sid: 'session-1' }))

    type TestUser = {
      id: string
      email: string
      name: string
      status: string
      created_at: string
      active_session_count: number
      roles: Array<{ id: string; user_id: string; role: string; tenant_id?: string }>
    }

    let usersResponse: { users: TestUser[] } = {
      users: [
        {
          id: 'user-1',
          email: 'ada@example.com',
          name: 'Ada Lovelace',
          status: 'active',
          created_at: '2026-03-20T12:00:00Z',
          active_session_count: 0,
          roles: [{ id: 'role-1', user_id: 'user-1', role: 'platform_admin' }],
        },
      ],
    }

    mockApiGet([
      ['/admin/users', () => usersResponse],
    ])

    const postSpy = vi.spyOn(api, 'post').mockImplementation(async (path, payload) => {
      if (path === '/admin/invites') {
        expect(payload).toEqual({
          email: 'new.user@example.com',
          tenant_id: 'tenant-demo',
          role: 'approver',
          name: 'New User',
        })
        usersResponse = {
          users: [
            ...usersResponse.users,
            {
              id: 'user-2',
              email: 'new.user@example.com',
              name: 'New User',
              status: 'invited',
              created_at: '2026-03-23T12:00:00Z',
              active_session_count: 0,
              roles: [{ id: 'role-2', user_id: 'user-2', role: 'approver', tenant_id: 'tenant-demo' }],
            },
          ],
        }
        return {
          token: 'invite-token-123',
          accept_url: 'https://console.example.test/invite/accept?token=invite-token-123',
          email_status: 'failed',
          email_error: 'SMTP unavailable',
        }
      }
      throw new Error(`Unhandled api.post call for ${path}`)
    })

    render(
      <MemoryRouter initialEntries={['/users']}>
        <Routes>
          <Route path="/users" element={<Users />} />
        </Routes>
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getAllByText('ada@example.com').length).toBeGreaterThan(0))

    const inviteCard = screen.getByRole('heading', { name: /invite user/i }).closest('.form-card') as HTMLElement | null
    expect(inviteCard).not.toBeNull()
    await user.type(getFieldIn(inviteCard!, /^email$/i), 'new.user@example.com')
    await user.type(getFieldIn(inviteCard!, /^tenant id$/i), 'tenant-demo')
    await user.selectOptions(getFieldIn(inviteCard!, /^role$/i), 'approver')
    await user.type(getFieldIn(inviteCard!, /^name \(optional\)$/i), 'New User')
    await user.click(within(inviteCard!).getByRole('button', { name: /create invite/i }))

    await waitFor(() => expect(postSpy).toHaveBeenCalledWith('/admin/invites', expect.any(Object)))
    expect(await screen.findByText(/invite created/i)).toBeInTheDocument()
    expect(screen.getByText(/raw invite token is only shown for this create response/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /open accept page/i })).toHaveAttribute(
      'href',
      'https://console.example.test/invite/accept?token=invite-token-123',
    )
    expect(screen.getByText(/email failed \(copy link instead\): SMTP unavailable/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /copy token/i })).toBeInTheDocument()
    await waitFor(() => expect(screen.getAllByText('new.user@example.com').length).toBeGreaterThan(0))
  })

  it('loads auth sessions and revokes the current session by clearing auth and navigating to login', async () => {
    const user = userEvent.setup()
    localStorage.setItem('oc_token', makeToken({ roles: ['platform_admin'], sid: 'current-session' }))

    mockApiGet([
      ['/admin/users', {
        users: [
          {
            id: 'user-1',
            email: 'ada@example.com',
            name: 'Ada Lovelace',
            status: 'active',
            created_at: '2026-03-20T12:00:00Z',
            active_session_count: 1,
            roles: [{ id: 'role-1', user_id: 'user-1', role: 'platform_admin' }],
          },
        ],
      }],
      ['/admin/auth-sessions?user_id=user-1', {
        sessions: [
          {
            id: 'current-session',
            user_id: 'user-1',
            tenant_id: 'tenant-demo',
            user_agent: 'Firefox',
            client_ip: '127.0.0.1',
            created_at: '2026-03-23T10:00:00Z',
            last_seen_at: '2026-03-23T12:00:00Z',
            expires_at: '2026-03-24T10:00:00Z',
          },
        ],
      }],
    ])

    const postSpy = vi.spyOn(api, 'post').mockImplementation(async (path) => {
      if (path === '/admin/auth-sessions/current-session/revoke') return {}
      throw new Error(`Unhandled api.post call for ${path}`)
    })

    render(
      <MemoryRouter initialEntries={['/users']}>
        <Routes>
          <Route path="/users" element={<Users />} />
          <Route path="/login" element={<div>Login page</div>} />
        </Routes>
      </MemoryRouter>,
    )

    expect(await screen.findByRole('button', { name: /review/i })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /review/i }))
    expect(await screen.findByRole('heading', { name: /active login sessions/i })).toBeInTheDocument()
    expect(screen.getAllByText(/current/i).length).toBeGreaterThan(0)

    await user.click(screen.getByRole('button', { name: /revoke/i }))

    await waitFor(() => expect(postSpy).toHaveBeenCalledWith('/admin/auth-sessions/current-session/revoke'))
    await waitFor(() => expect(localStorage.getItem('oc_token')).toBeNull())
    expect(await screen.findByText('Login page')).toBeInTheDocument()
  })
})
