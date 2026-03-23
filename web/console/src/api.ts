const API_BASE = '/api';

export class APIClientError extends Error {
  status: number
  code?: string
  details?: unknown
  candidates?: string[]
  requestId?: string

  constructor(message: string, options: { status: number; code?: string; details?: unknown; candidates?: string[]; requestId?: string }) {
    super(message)
    this.name = 'APIClientError'
    this.status = options.status
    this.code = options.code
    this.details = options.details
    this.candidates = options.candidates
    this.requestId = options.requestId
  }
}

export type StoredAuthClaims = {
  sub?: string
  sid?: string
  email?: string
  name?: string
  roles?: string[]
  tenant?: string
}

function decodeTokenClaims(token: string | null): StoredAuthClaims | null {
  if (!token) return null
  try {
    const payload = token.split('.')[1]
    if (!payload) return null
    const base64 = payload.replace(/-/g, '+').replace(/_/g, '/')
    const padded = base64.padEnd(base64.length + (4 - (base64.length % 4)) % 4, '=')
    return JSON.parse(atob(padded))
  } catch {
    return null
  }
}

export function clearStoredAuth() {
  localStorage.removeItem('oc_token')
  localStorage.removeItem('oc_session_id')
}

export function getStoredAuthClaims(): StoredAuthClaims | null {
  return decodeTokenClaims(localStorage.getItem('oc_token'))
}

export function getStoredSessionID(): string {
  return localStorage.getItem('oc_session_id') || getStoredAuthClaims()?.sid || ''
}

export function storeAuthSession(token: string, sessionID?: string) {
  localStorage.setItem('oc_token', token)
  const sid = sessionID || decodeTokenClaims(token)?.sid || ''
  if (sid) localStorage.setItem('oc_session_id', sid)
  else localStorage.removeItem('oc_session_id')
}

function extractCandidates(payload: any): string[] | undefined {
  const direct = Array.isArray(payload?.candidates) ? payload.candidates : null
  if (direct && direct.length > 0) return direct.map((value: unknown) => String(value))
  const nested = Array.isArray(payload?.details?.candidates) ? payload.details.candidates : null
  if (nested && nested.length > 0) return nested.map((value: unknown) => String(value))
  return undefined
}

function extractRequestId(headers?: Headers): string | undefined {
  const requestId = headers?.get('x-request-id') || headers?.get('x-correlation-id') || ''
  const trimmed = requestId.trim()
  return trimmed || undefined
}

function toAPIClientError(status: number, fallback: string, payload: any, headers?: Headers) {
  const requestId = extractRequestId(headers)
  const baseMessage = payload?.message || payload?.error || fallback
  const message = requestId ? `${baseMessage} (request id: ${requestId})` : baseMessage
  return new APIClientError(message, {
    status,
    code: payload?.code,
    details: payload?.details,
    candidates: extractCandidates(payload),
    requestId,
  })
}

export async function readJSONResponse(res: Response) {
  const text = await res.text()
  if (!text.trim()) return {}
  try {
    return JSON.parse(text)
  } catch {
    if (res.ok) return {}
    return { error: `${res.status} ${res.statusText}` }
  }
}

async function apiFetch(path: string, options?: RequestInit) {
  const token = localStorage.getItem('oc_token');
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
  };
  const res = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: { ...headers, ...(options?.headers as Record<string, string>) },
  });
  if (res.status === 401) {
    clearStoredAuth();
    window.location.href = '/login';
    throw new Error('Unauthorized');
  }
  if (!res.ok) {
    const err = await readJSONResponse(res);
    throw toAPIClientError(res.status, res.statusText, err, res.headers);
  }
  return res;
}

async function unauthFetch(path: string, body: unknown) {
  const res = await fetch(`${API_BASE}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const err = await readJSONResponse(res);
    throw toAPIClientError(res.status, res.statusText, err, res.headers);
  }
  return readJSONResponse(res);
}

export const api = {
  get: (path: string) => apiFetch(path).then(readJSONResponse),
  post: (path: string, body?: unknown) =>
    apiFetch(path, { method: 'POST', body: body ? JSON.stringify(body) : undefined }).then(readJSONResponse),
  put: (path: string, body?: unknown) =>
    apiFetch(path, { method: 'PUT', body: body ? JSON.stringify(body) : undefined }).then(readJSONResponse),
  delete: (path: string) =>
    apiFetch(path, { method: 'DELETE' }).then(r => r.status === 204 ? {} : readJSONResponse(r)),
  getBlob: (path: string) => apiFetch(path).then(r => r.blob()),
  unauthPost: unauthFetch,
};

export function toQueryTimestamp(value: string | undefined | null): string {
  const text = (value || '').trim()
  if (!text) return ''

  if (/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(:\d{2})?$/.test(text)) {
    const normalized = text.length === 16 ? `${text}:00` : text
    const localDate = new Date(normalized)
    if (!Number.isNaN(localDate.getTime())) {
      return localDate.toISOString()
    }
  }

  return text
}

export function toLocalDateTimeInput(value: string | undefined | null): string {
  const text = (value || '').trim()
  if (!text) return ''

  if (/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(text)) {
    return text
  }

  const parsed = new Date(text)
  if (Number.isNaN(parsed.getTime())) return text

  const pad = (input: number) => String(input).padStart(2, '0')
  return `${parsed.getFullYear()}-${pad(parsed.getMonth() + 1)}-${pad(parsed.getDate())}T${pad(parsed.getHours())}:${pad(parsed.getMinutes())}`
}

export function formatDate(value: string | undefined | null, style: 'full' | 'date' = 'full'): string {
  if (!value) return '—';
  const d = new Date(value);
  if (isNaN(d.getTime())) return value;
  return style === 'date' ? d.toLocaleDateString() : d.toLocaleString();
}
