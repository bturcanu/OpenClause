import { useState, useEffect, FormEvent, useRef } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, formatDate } from '../api'

interface Tenant {
  id: string
  name: string
  status: string
  config: any
  created_at: string
}

interface Agent {
  id: string
  name: string
  tenant_id: string
  created_at: string
}

interface ApiKey {
  id: string
  key_prefix: string
  name: string
  status: string
  created_at: string
}

export default function TenantDetail() {
  const { id } = useParams<{ id: string }>()
  const [tenant, setTenant] = useState<Tenant | null>(null)
  const [agents, setAgents] = useState<Agent[]>([])
  const [apiKeys, setApiKeys] = useState<ApiKey[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const fetchSeq = useRef(0)

  const [agentForm, setAgentForm] = useState({ name: '' })
  const [keyForm, setKeyForm] = useState({ name: '' })
  const [newKeyRaw, setNewKeyRaw] = useState('')
  const [creating, setCreating] = useState(false)

  async function fetchAll() {
    const seq = ++fetchSeq.current
    setLoading(true)
    setError('')
    setTenant(null)
    setAgents([])
    setApiKeys([])

    try {
      const [t, ag, keys] = await Promise.all([
        api.get(`/admin/tenants/${id}`).catch(() => null),
        api.get(`/admin/tenants/${id}/agents`).catch(() => []),
        api.get(`/admin/tenants/${id}/apikeys`).catch(() => []),
      ])
      if (seq !== fetchSeq.current) return
      setTenant(t ?? null)
      setAgents(Array.isArray(ag) ? ag : ag?.agents || [])
      setApiKeys(Array.isArray(keys) ? keys : keys?.api_keys || [])
    } catch (err: any) {
      if (seq === fetchSeq.current) setError(err.message)
    } finally {
      if (seq === fetchSeq.current) setLoading(false)
    }
  }

  useEffect(() => { fetchAll() }, [id])

  async function createAgent(e: FormEvent) {
    e.preventDefault()
    setCreating(true)
    try {
      await api.post(`/admin/tenants/${id}/agents`, agentForm)
      setAgentForm({ name: '' })
      await fetchAll()
    } catch (err: any) {
      setError(err.message)
    } finally {
      setCreating(false)
    }
  }

  async function createKey(e: FormEvent) {
    e.preventDefault()
    setCreating(true)
    setNewKeyRaw('')
    try {
      const data = await api.post(`/admin/tenants/${id}/apikeys`, { name: keyForm.name })
      setNewKeyRaw(data.raw_key || data.key || '')
      setKeyForm({ name: '' })
      await fetchAll()
    } catch (err: any) {
      setError(err.message)
    } finally {
      setCreating(false)
    }
  }

  async function revokeKey(keyId: string) {
    try {
      await api.post(`/admin/tenants/${id}/apikeys/${keyId}/revoke`)
      await fetchAll()
    } catch (err: any) {
      setError(err.message)
    }
  }

  if (loading) return <div className="loading">Loading tenant…</div>
  if (!tenant) return <div className="error-msg">Tenant not found</div>

  return (
    <div>
      <div className="flex-between">
        <div className="page-header">
          <h2>{tenant.name}</h2>
          <p>Tenant management — {tenant.id}</p>
        </div>
        <Link to="/tenants" className="btn btn-outline">← Back</Link>
      </div>

      {error && <div className="error-msg">{error}</div>}

      <div className="detail-panel">
        <h3>Tenant Info</h3>
        <div className="detail-row">
          <div className="detail-label">ID</div>
          <div className="detail-value">{tenant.id}</div>
        </div>
        <div className="detail-row">
          <div className="detail-label">Status</div>
          <div className="detail-value">
            <span className={`badge ${tenant.status === 'active' ? 'badge-green' : 'badge-red'}`}>{tenant.status}</span>
          </div>
        </div>
        <div className="detail-row">
          <div className="detail-label">Created</div>
          <div className="detail-value">{formatDate(tenant.created_at)}</div>
        </div>
      </div>

      {/* Agents */}
      <div className="section-title">Agents</div>
      <div className="form-card">
        <h3>Register Agent</h3>
        <form onSubmit={createAgent}>
          <div className="form-inline">
            <div className="form-group">
              <label>Agent Name</label>
              <input value={agentForm.name} onChange={e => setAgentForm({ name: e.target.value })} required />
            </div>
            <button className="btn btn-primary" disabled={creating}>Create</button>
          </div>
        </form>
      </div>

      <div className="table-container">
        <table>
          <thead>
            <tr>
              <th>ID</th>
              <th>Name</th>
              <th>Created</th>
            </tr>
          </thead>
          <tbody>
            {agents.length === 0 ? (
              <tr><td colSpan={3} style={{ textAlign: 'center', padding: 24, color: '#94a3b8' }}>No agents</td></tr>
            ) : (
              agents.map(a => (
                <tr key={a.id}>
                  <td style={{ fontFamily: 'monospace', fontSize: 12 }}>{a.id.slice(0, 12)}…</td>
                  <td>{a.name}</td>
                  <td>{formatDate(a.created_at, 'date')}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* API Keys */}
      <div className="section-title mt-16">API Keys</div>
      <div className="form-card">
        <h3>Create API Key</h3>
        <form onSubmit={createKey}>
          <div className="form-inline">
            <div className="form-group">
              <label>Name</label>
              <input value={keyForm.name} onChange={e => setKeyForm({ name: e.target.value })} required />
            </div>
            <button className="btn btn-primary" disabled={creating}>Create</button>
          </div>
        </form>
        {newKeyRaw && (
          <div style={{ marginTop: 16 }}>
            <p style={{ fontSize: 13, fontWeight: 600, color: '#ef4444', marginBottom: 4 }}>
              Copy this key now — it will not be shown again:
            </p>
            <div className="key-display">{newKeyRaw}</div>
          </div>
        )}
      </div>

      <div className="table-container">
        <table>
          <thead>
            <tr>
              <th>Prefix</th>
              <th>Name</th>
              <th>Status</th>
              <th>Created</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {apiKeys.length === 0 ? (
              <tr><td colSpan={5} style={{ textAlign: 'center', padding: 24, color: '#94a3b8' }}>No API keys</td></tr>
            ) : (
              apiKeys.map(k => (
                <tr key={k.id}>
                  <td style={{ fontFamily: 'monospace' }}>{k.key_prefix}…</td>
                  <td>{k.name}</td>
                  <td>
                    {k.status === 'revoked'
                      ? <span className="badge badge-red">Revoked</span>
                      : <span className="badge badge-green">Active</span>}
                  </td>
                  <td>{formatDate(k.created_at, 'date')}</td>
                  <td>
                    {k.status !== 'revoked' && (
                      <button className="btn btn-danger btn-sm" onClick={() => revokeKey(k.id)}>Revoke</button>
                    )}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}
