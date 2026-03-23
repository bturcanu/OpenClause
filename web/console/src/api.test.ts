import { describe, expect, it, vi } from 'vitest'
import {
  APIClientError,
  api,
  clearStoredAuth,
  formatDate,
  getStoredAuthClaims,
  getStoredSessionID,
  readJSONResponse,
  storeAuthSession,
  toLocalDateTimeInput,
  toQueryTimestamp,
} from './api'

function makeToken(payload: Record<string, unknown>) {
  const encoded = btoa(JSON.stringify(payload)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '')
  return `header.${encoded}.signature`
}

describe('api helpers', () => {
  it('stores auth claims and falls back to sid from the token', () => {
    const token = makeToken({ sub: 'user-1', sid: 'session-123', email: 'admin@example.com', roles: ['platform_admin'] })

    storeAuthSession(token)

    expect(getStoredAuthClaims()).toMatchObject({
      sub: 'user-1',
      sid: 'session-123',
      email: 'admin@example.com',
      roles: ['platform_admin'],
    })
    expect(getStoredSessionID()).toBe('session-123')
  })

  it('prefers the explicit session id when storing auth state', () => {
    const token = makeToken({ sid: 'session-from-token' })

    storeAuthSession(token, 'session-from-response')

    expect(getStoredSessionID()).toBe('session-from-response')
  })

  it('clears stored auth keys', () => {
    localStorage.setItem('oc_token', 'token')
    localStorage.setItem('oc_session_id', 'sid')

    clearStoredAuth()

    expect(localStorage.getItem('oc_token')).toBeNull()
    expect(localStorage.getItem('oc_session_id')).toBeNull()
  })

  it('parses JSON responses and falls back gracefully', async () => {
    const ok = new Response(JSON.stringify({ status: 'ok' }), { status: 200 })
    const blank = new Response(null, { status: 204 })
    const broken = new Response('not json', { status: 500, statusText: 'Boom' })

    await expect(readJSONResponse(ok)).resolves.toEqual({ status: 'ok' })
    await expect(readJSONResponse(blank)).resolves.toEqual({})
    await expect(readJSONResponse(broken)).resolves.toEqual({ error: '500 Boom' })
  })

  it('maps API errors with details and tenant candidates', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({
        message: 'Need tenant',
        code: 'tenant_required',
        details: { reason: 'ambiguous', candidates: ['tenant-a', 'tenant-b'] },
      }), { status: 409, headers: { 'Content-Type': 'application/json' } }),
    )
    vi.stubGlobal('fetch', fetchMock)

    await expect(api.get('/admin/sessions/demo')).rejects.toMatchObject({
      message: 'Need tenant',
      status: 409,
      code: 'tenant_required',
      candidates: ['tenant-a', 'tenant-b'],
    })
  })

  it('clears auth when the API returns 401', async () => {
    localStorage.setItem('oc_token', 'token')
    localStorage.setItem('oc_session_id', 'sid')
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('', { status: 401 })))

    await expect(api.get('/admin/tenants')).rejects.toThrow('Unauthorized')
    expect(localStorage.getItem('oc_token')).toBeNull()
    expect(localStorage.getItem('oc_session_id')).toBeNull()
  })

  it('converts local datetime inputs to API timestamps and back', () => {
    const iso = toQueryTimestamp('2026-03-23T12:34')
    expect(iso).toMatch(/^2026-03-23T/)
    expect(toLocalDateTimeInput(iso)).toMatch(/^2026-03-23T\d{2}:\d{2}$/)
    expect(toLocalDateTimeInput('2026-03-23T12:34')).toBe('2026-03-23T12:34')
  })

  it('formats dates defensively', () => {
    expect(formatDate('')).toBe('—')
    expect(formatDate('not-a-date')).toBe('not-a-date')
  })
})
