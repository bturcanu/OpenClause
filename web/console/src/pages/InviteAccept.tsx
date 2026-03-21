import { useEffect, useState, FormEvent } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api } from '../api'
import { InlineErrorState } from '../ui'

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

      {error ? <InlineErrorState message={error} /> : null}
      {status && <div className="success-msg">{status}</div>}
      {acceptedRole === 'tenant_admin' && acceptedTenantId && (
        <div className="banner-note" style={{ marginTop: 12 }}>
          <span>
          After logging in, jump to{' '}
          <Link to={`/tenants/${acceptedTenantId}?tab=api_keys`}>Tenant API Keys</Link>.
          </span>
        </div>
      )}

      <form onSubmit={submit} className="form-card" style={{ maxWidth: 520 }}>
        <div className="form-group">
          <label>Token</label>
          <input className="mono" value={token} onChange={e => setToken(e.target.value)} required />
          <div className="form-helper-text">If you opened the invite from email, this field should already be filled in.</div>
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
      <div className="auth-page-links">
        <Link to="/login" className="auth-page-link">Back to sign in</Link>
      </div>
    </div>
  )
}
