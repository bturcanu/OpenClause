const API_BASE = '/api';

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
    const err = await res.json().catch(() => ({} as any));
    const msg = err?.message || err?.error || res.statusText;
    throw new Error(msg);
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
    const err = await res.json().catch(() => ({} as any));
    throw new Error(err?.message || err?.error || res.statusText);
  }
  return res.json();
}

export const api = {
  get: (path: string) => apiFetch(path).then(r => r.json()),
  post: (path: string, body?: unknown) =>
    apiFetch(path, { method: 'POST', body: body ? JSON.stringify(body) : undefined }).then(r => r.json()),
  put: (path: string, body?: unknown) =>
    apiFetch(path, { method: 'PUT', body: body ? JSON.stringify(body) : undefined }).then(r => r.json()),
  delete: (path: string) =>
    apiFetch(path, { method: 'DELETE' }).then(r => r.status === 204 ? {} : r.json()),
  getBlob: (path: string) => apiFetch(path).then(r => r.blob()),
  unauthPost: unauthFetch,
};

export function formatDate(value: string | undefined | null, style: 'full' | 'date' = 'full'): string {
  if (!value) return '—';
  const d = new Date(value);
  if (isNaN(d.getTime())) return value;
  return style === 'date' ? d.toLocaleDateString() : d.toLocaleString();
}
