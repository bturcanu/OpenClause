import { useState, FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { readJSONResponse } from '../api'
import { InlineErrorState } from '../ui'

type InitResp = {
  initialized: boolean
  tenant_id?: string
}

export default function SetupWizard(props: { onInitialized?: () => void }) {
  const navigate = useNavigate()

  const [orgName, setOrgName] = useState('')
  const [email, setEmail] = useState('admin@openclause.dev')
  const [password, setPassword] = useState('')
  const [firstTenantName, setFirstTenantName] = useState('First tenant')

  const [error, setError] = useState('')
  const [status, setStatus] = useState('')
  const [submitting, setSubmitting] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setStatus('')
    setSubmitting(true)
    try {
      const resp = await fetch('/api/setup/initialize', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          org_name: orgName,
          email,
          password,
          first_tenant_name: firstTenantName,
        }),
      })
      const data: InitResp = await readJSONResponse(resp)
      if (!resp.ok) {
        throw new Error((data as any)?.message || (data as any)?.error || 'Failed to initialize')
      }

      props.onInitialized?.()
      setStatus('Setup complete. Redirecting to login…')
      // Small delay for UX.
      setTimeout(() => navigate('/login'), 700)
    } catch (err: any) {
      setError(err.message || 'Setup failed')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="auth-page">
      <div className="page-header">
        <h2>First-run Setup</h2>
        <p>Create the initial platform admin + first tenant.</p>
      </div>

      {error ? <InlineErrorState message={error} /> : null}
      {status && <div className="success-msg">{status}</div>}

      <form onSubmit={submit} className="form-card" style={{ maxWidth: 720 }}>
        <div className="form-group">
          <label>Organization name (optional)</label>
          <input value={orgName} onChange={e => setOrgName(e.target.value)} placeholder="Acme Co" />
        </div>

        <div className="grid-2">
          <div className="form-group">
            <label>Platform admin email</label>
            <input value={email} onChange={e => setEmail(e.target.value)} required />
          </div>
          <div className="form-group">
            <label>Platform admin password</label>
            <input type="password" value={password} onChange={e => setPassword(e.target.value)} required />
          </div>
        </div>

        <div className="form-group mt-8">
          <label>First tenant name</label>
          <input value={firstTenantName} onChange={e => setFirstTenantName(e.target.value)} required />
        </div>

        <div className="form-helper-text" style={{ marginTop: 16 }}>
          Tip: use the provided secret generator scripts (see README) for `CONSOLE_JWT_SECRET` and JWT bootstrap settings.
        </div>

        <button className="btn btn-primary mt-16" disabled={submitting} type="submit">
          {submitting ? 'Initializing…' : 'Initialize'}
        </button>
      </form>
    </div>
  )
}
