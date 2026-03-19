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

interface Approver {
  id: string
  email: string
  name: string
  slack_user_id?: string | null
}

export default function TenantDetail() {
  const { id } = useParams<{ id: string }>()
  const [tenant, setTenant] = useState<Tenant | null>(null)
  const [agents, setAgents] = useState<Agent[]>([])
  const [apiKeys, setApiKeys] = useState<ApiKey[]>([])
  const [approvers, setApprovers] = useState<Approver[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const fetchSeq = useRef(0)

  const [agentForm, setAgentForm] = useState({ name: '' })
  const [keyForm, setKeyForm] = useState({ name: '' })
  const [newKeyRaw, setNewKeyRaw] = useState('')
  const [creating, setCreating] = useState(false)

  const [approverEmail, setApproverEmail] = useState('')
  const [approverSlackUserID, setApproverSlackUserID] = useState('')
  const [approverName, setApproverName] = useState('')

  const [allowlistSource, setAllowlistSource] = useState<string>('db')
  const [activeTab, setActiveTab] = useState<'agents' | 'api_keys' | 'approvers'>('agents')

  async function fetchAll() {
    const seq = ++fetchSeq.current
    setLoading(true)
    setError('')
    setTenant(null)
    setAgents([])
    setApiKeys([])
    setApprovers([])
    setAllowlistSource('db')

    try {
      const [t, ag, keys, approverResp] = await Promise.all([
        api.get(`/admin/tenants/${id}`).catch(() => null),
        api.get(`/admin/tenants/${id}/agents`).catch(() => []),
        api.get(`/admin/tenants/${id}/apikeys`).catch(() => []),
        api.get(`/admin/tenants/${id}/approvers`).catch(() => ({ approvers: [], allowlist_source: 'db' })),
      ])
      if (seq !== fetchSeq.current) return
      setTenant(t ?? null)
      setAgents(Array.isArray(ag) ? ag : ag?.agents || [])
      setApiKeys(Array.isArray(keys) ? keys : keys?.api_keys || [])
      setApprovers(Array.isArray(approverResp?.approvers) ? approverResp.approvers : [])
      if (approverResp?.allowlist_source) setAllowlistSource(approverResp.allowlist_source)
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

  async function addApprover(e: FormEvent) {
    e.preventDefault()
    setCreating(true)
    setError('')
    try {
      const payload: any = { role: 'approver' }
      const email = approverEmail.trim()
      const slackUserID = approverSlackUserID.trim()
      const name = approverName.trim()
      if (email) payload.email = email
      if (slackUserID) payload.slack_user_id = slackUserID
      if (name) payload.name = name
      if (!payload.email && !payload.slack_user_id) {
        throw new Error('Provide email and/or slack_user_id')
      }
      await api.post(`/admin/tenants/${id}/approvers`, payload)
      setApproverEmail('')
      setApproverSlackUserID('')
      setApproverName('')
      await fetchAll()
    } catch (err: any) {
      setError(err.message)
    } finally {
      setCreating(false)
    }
  }

  async function removeApprover(userID: string) {
    setCreating(true)
    setError('')
    try {
      await api.delete(`/admin/tenants/${id}/approvers/${userID}`)
      await fetchAll()
    } catch (err: any) {
      setError(err.message)
    } finally {
      setCreating(false)
    }
  }

  if (loading) return <div className="loading">Loading tenant…</div>
  if (error) return <div className="error-msg">{error}</div>
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
        {tenant.config?.org_name && (
          <div className="detail-row">
            <div className="detail-label">Organization</div>
            <div className="detail-value">{tenant.config.org_name}</div>
          </div>
        )}
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

      <div className="tabs mt-16" style={{ display: 'flex', gap: 8 }}>
        <button
          className={`btn btn-outline btn-sm ${activeTab === 'agents' ? 'active' : ''}`}
          onClick={() => setActiveTab('agents')}
        >
          Agents
        </button>
        <button
          className={`btn btn-outline btn-sm ${activeTab === 'api_keys' ? 'active' : ''}`}
          onClick={() => setActiveTab('api_keys')}
        >
          API Keys
        </button>
        <button
          className={`btn btn-outline btn-sm ${activeTab === 'approvers' ? 'active' : ''}`}
          onClick={() => setActiveTab('approvers')}
        >
          Approvers
        </button>
      </div>

      {activeTab === 'agents' && (
        <>
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
        </>
      )}

      {activeTab === 'api_keys' && (
        <>
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
        </>
      )}

      {activeTab === 'approvers' && (
        <>
          { (allowlistSource === 'env' || allowlistSource === 'both') && (
            <div className="warn-banner" style={{ marginTop: 16, border: '1px solid #f59e0b', padding: 12, borderRadius: 8, background: '#fffbeb' }}>
              <div style={{ fontWeight: 700, marginBottom: 4 }}>Dev bootstrap allowlists enabled</div>
              <div style={{ color: '#92400e', fontSize: 13 }}>Approver authorization may allow env allowlists in addition to DB roles.</div>
            </div>
          )}

          <div className="section-title mt-16">Approvers</div>

          <div className="form-card">
            <h3>Add Approver</h3>
            <form onSubmit={addApprover}>
              <div className="form-inline" style={{ gap: 16, flexWrap: 'wrap' }}>
                <div className="form-group" style={{ minWidth: 280 }}>
                  <label>Email</label>
                  <input value={approverEmail} onChange={e => setApproverEmail(e.target.value)} placeholder="name@company.com" />
                </div>
                <div className="form-group" style={{ minWidth: 280 }}>
                  <label>Slack user id (optional)</label>
                  <input value={approverSlackUserID} onChange={e => setApproverSlackUserID(e.target.value)} placeholder="U1234567890" />
                  <div style={{ marginTop: 4, color: '#64748b', fontSize: 12 }}>
                    If provided without an email, this will only link to an existing user.
                    Provide an email to create the user + link Slack id.
                  </div>
                </div>
                <div className="form-group" style={{ minWidth: 220 }}>
                  <label>Name (optional)</label>
                  <input value={approverName} onChange={e => setApproverName(e.target.value)} placeholder="Full name" />
                </div>
                <button className="btn btn-primary" disabled={creating}>Add</button>
              </div>
            </form>
          </div>

          <div className="table-container" style={{ marginTop: 16 }}>
            <table>
              <thead>
                <tr>
                  <th>Email</th>
                  <th>Name</th>
                  <th>Slack user id</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {approvers.length === 0 ? (
                  <tr><td colSpan={4} style={{ textAlign: 'center', padding: 24, color: '#94a3b8' }}>No approvers</td></tr>
                ) : (
                  approvers.map(a => (
                    <tr key={a.id}>
                      <td>{a.email}</td>
                      <td>{a.name || '—'}</td>
                      <td style={{ fontFamily: 'monospace', fontSize: 12 }}>{a.slack_user_id ? a.slack_user_id : '—'}</td>
                      <td>
                        <button className="btn btn-danger btn-sm" onClick={() => removeApprover(a.id)} disabled={creating}>
                          Remove
                        </button>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  )
}
