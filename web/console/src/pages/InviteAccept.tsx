import { useEffect, useState, FormEvent } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api } from '../api'

export default function InviteAccept() {
  const [searchParams] = useSearchParams()
  const [token, setToken] = useState('')

  const [password, setPassword] = useState('')
  const [name, setName] = useState('')

  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [status, setStatus] = useState('')
  const [acceptedTenantId, setAcceptedTenantId] = useState('')
  const [acceptedRole, setAcceptedRole] = useState('')

  useEffect(() => {
    const t = searchParams.get('token')
    if (t) setToken(t)
  }, [searchParams])

  async function submit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setStatus('')
    setAcceptedTenantId('')
    setAcceptedRole('')
    setLoading(true)
    try {
      if (!token) throw new Error('Missing token')
      const data = await api.unauthPost('/auth/invite/accept', { token, password, name })
      const tenantId = data?.tenant_id || ''
      const role = data?.role || ''
      setAcceptedTenantId(tenantId)
      setAcceptedRole(role)
      if (role === 'tenant_admin' && tenantId) {
        setStatus(`Invite accepted. You are now tenant_admin for ${tenantId}. Next: log in and create/manage your API keys.`)
      } else {
        setStatus('Invite accepted. You can now log in.')
      }
    } catch (err: any) {
      setError(err.message || 'Failed to accept invite')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="auth-page">
      <div className="page-header">
        <h2>Accept Invite</h2>
        <p>Set your password to activate your account.</p>
      </div>

      {error && <div className="error-msg">{error}</div>}
      {status && <div className="success-msg">{status}</div>}
      {acceptedRole === 'tenant_admin' && acceptedTenantId && (
        <div style={{ marginTop: 12, fontSize: 13, color: '#475569' }}>
          After logging in, jump to{' '}
          <Link to={`/tenants/${acceptedTenantId}?tab=api_keys`}>Tenant API Keys</Link>.
        </div>
      )}

      <form onSubmit={submit} className="form-card" style={{ maxWidth: 520 }}>
        <div className="form-group">
          <label>Token</label>
          <input value={token} onChange={e => setToken(e.target.value)} required />
        </div>
        <div className="form-group">
          <label>Password</label>
          <input type="password" value={password} onChange={e => setPassword(e.target.value)} required />
        </div>
        <div className="form-group">
          <label>Name (optional)</label>
          <input value={name} onChange={e => setName(e.target.value)} />
        </div>

        <button className="btn btn-primary" disabled={loading} type="submit">
          {loading ? 'Accepting…' : 'Accept Invite'}
        </button>
      </form>
    </div>
  )
}

