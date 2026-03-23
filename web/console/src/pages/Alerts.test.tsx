import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import Alerts from './Alerts'
import { renderRoute } from '../test/render'

function makeToken(payload: Record<string, unknown>) {
  const encoded = btoa(JSON.stringify(payload)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '')
  return `header.${encoded}.signature`
}

describe('Alerts page', () => {
  it('keeps successful event data visible when rules fail to load', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/admin/alerts/rules') throw new Error('Rules unavailable')
      if (path === '/admin/alerts/events') {
        return {
          events: [
            {
              id: 'alert-1',
              rule_id: 'rule-1',
              tenant_id: 'tenant-1',
              severity: 'warning',
              message: 'Deny spike detected',
              status: 'pending',
              created_at: '2026-03-23T12:00:00Z',
              attempt_count: 2,
              next_attempt_at: '2026-03-23T12:05:00Z',
            },
          ],
        }
      }
      throw new Error(`Unhandled api.get call for ${path}`)
    })

    renderRoute(<Alerts />, { path: '/alerts', route: '/alerts' })

    expect(await screen.findByText(/some alert data could not be loaded: alert rules/i)).toBeInTheDocument()
    expect(screen.getByText('Deny spike detected')).toBeInTheDocument()
    expect(screen.getByText('pending')).toBeInTheDocument()
  })

  it('creates alert rules with the expected payload and refreshes the rules table', async () => {
    const user = userEvent.setup()
    localStorage.setItem('oc_token', makeToken({ roles: ['platform_admin'] }))

    let rules = [
      {
        id: 'rule-1',
        tenant_id: 'tenant-1',
        name: 'Existing rule',
        kind: 'deny_spike',
        enabled: true,
        created_at: '2026-03-20T12:00:00Z',
      },
    ]

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/admin/alerts/rules') return rules
      if (path === '/admin/alerts/events') return []
      throw new Error(`Unhandled api.get call for ${path}`)
    })

    const postSpy = vi.spyOn(api, 'post').mockImplementation(async (path, payload) => {
      if (path === '/admin/alerts/rules') {
        expect(payload).toEqual({
          tenant_id: 'tenant-42',
          name: 'Retry burst',
          kind: 'deny_spike',
          enabled: false,
          config_json: { n: 7, m_minutes: 15 },
        })
        rules = [
          ...rules,
          {
            id: 'rule-2',
            tenant_id: 'tenant-42',
            name: 'Retry burst',
            kind: 'deny_spike',
            enabled: false,
            created_at: '2026-03-23T12:00:00Z',
          },
        ]
        return {}
      }
      throw new Error(`Unhandled api.post call for ${path}`)
    })

    renderRoute(<Alerts />, { path: '/alerts', route: '/alerts' })

    expect(await screen.findByText('Existing rule')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /\+ new rule/i }))

    const createCard = screen.getByRole('heading', { name: /create alert rule/i }).closest('.form-card') as HTMLElement | null
    expect(createCard).not.toBeNull()
    await user.type(within(createCard!).getByLabelText(/^tenant id$/i), 'tenant-42')
    await user.type(within(createCard!).getByLabelText(/^rule name$/i), 'Retry burst')
    fireEvent.change(within(createCard!).getByLabelText(/^n \(deny count threshold\)$/i), { target: { value: '7' } })
    fireEvent.change(within(createCard!).getByLabelText(/^m \(window minutes\)$/i), { target: { value: '15' } })
    await user.click(screen.getByRole('checkbox', { name: /enabled immediately/i }))
    await user.click(within(createCard!).getByRole('button', { name: /create rule/i }))

    await waitFor(() => expect(postSpy).toHaveBeenCalledWith('/admin/alerts/rules', expect.any(Object)))
    expect(await screen.findByText('Retry burst')).toBeInTheDocument()
    expect(screen.getByText('Disabled')).toBeInTheDocument()
  })
})
