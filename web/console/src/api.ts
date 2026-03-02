const API_BASE = '/api';

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
    localStorage.removeItem('oc_token');
    window.location.href = '/login';
    throw new Error('Unauthorized');
  }
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error || res.statusText);
  }
  return res;
}

export const api = {
  get: (path: string) => apiFetch(path).then(r => r.json()),
  post: (path: string, body?: unknown) =>
    apiFetch(path, { method: 'POST', body: body ? JSON.stringify(body) : undefined }).then(r => r.json()),
  delete: (path: string) =>
    apiFetch(path, { method: 'DELETE' }).then(r => r.json()),
  getBlob: (path: string) => apiFetch(path).then(r => r.blob()),
};
