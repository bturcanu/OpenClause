import { describe, expect, it, vi } from 'vitest'
import {
  APIClientError,
  api,
  clearStoredAuth,
  formatDate,
  getStoredAuthClaims,
  getStoredSessionID,
  readJSONResponse,
  resetAPIFailureTrackingForTests,
  storeAuthSession,
  toLocalDateTimeInput,
  toQueryTimestamp,
} from './api'

function makeToken(payload: Record<string, unknown>) {
  const encoded = btoa(JSON.stringify(payload)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '')
  return `header.${encoded}.signature`
}

describe('api helpers', () => {
  it('logs repeated API failures with request ids and resets after success', async () => {
    resetAPIFailureTrackingForTests()
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ message: 'Boom' }), { status: 500, headers: { 'Content-Type': 'application/json', 'X-Request-Id': 'req-repeat' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ message: 'Boom' }), { status: 500, headers: { 'Content-Type': 'application/json', 'X-Request-Id': 'req-repeat' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ message: 'Boom' }), { status: 500, headers: { 'Content-Type': 'application/json', 'X-Request-Id': 'req-repeat' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ ok: true }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ message: 'Boom again' }), { status: 500, headers: { 'Content-Type': 'application/json', 'X-Request-Id': 'req-repeat-2' } }))
    vi.stubGlobal('fetch', fetchMock)

    for (let index = 0; index < 3; index += 1) {
      await expect(api.get('/admin/tenants')).rejects.toMatchObject({ status: 500 })
    }

    expect(warnSpy).toHaveBeenCalledWith(
      '[openclause-console] repeated api failures',
      expect.objectContaining({
        method: 'GET',
        path: '/admin/tenants',
        count: 3,
        status: 500,
        requestId: 'req-repeat',
      }),
    )

    await expect(api.get('/admin/tenants')).resolves.toEqual({ ok: true })
    await expect(api.get('/admin/tenants')).rejects.toMatchObject({ status: 500 })
    expect(warnSpy).toHaveBeenCalledTimes(1)
  })

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
      }), {
        status: 409,
        headers: {
          'Content-Type': 'application/json',
          'X-Request-Id': 'req-123',
        },
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    await expect(api.get('/admin/sessions/demo')).rejects.toMatchObject({
      message: 'Need tenant (request id: req-123)',
      status: 409,
      code: 'tenant_required',
      candidates: ['tenant-a', 'tenant-b'],
      requestId: 'req-123',
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

  it('handles blank, seconds-based, and passthrough timestamp edge cases', () => {
    expect(toQueryTimestamp('')).toBe('')
    expect(toQueryTimestamp('2026-03-23T12:34:56')).toMatch(/^2026-03-23T/)
    expect(toQueryTimestamp('2026-03-23T12:34:56Z')).toBe('2026-03-23T12:34:56Z')
    expect(toLocalDateTimeInput('')).toBe('')
    expect(toLocalDateTimeInput('not-a-date')).toBe('not-a-date')
  })

  it('round-trips a matrix of local datetime inputs through query timestamps', () => {
    const samples = [
      '2026-01-01T00:00',
      '2026-03-08T01:59',
      '2026-03-08T03:01',
      '2026-11-01T01:30',
      '2026-12-31T23:59',
    ]

    for (const sample of samples) {
      const iso = toQueryTimestamp(sample)
      expect(iso).toMatch(/Z$/)
      expect(toLocalDateTimeInput(iso)).toBe(sample)
    }
  })

  it('formats dates defensively', () => {
    expect(formatDate('')).toBe('—')
    expect(formatDate('not-a-date')).toBe('not-a-date')
  })
})
