import { useState, FormEvent } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { readJSONResponse, storeAuthSession } from '../api'
import { InlineErrorState } from '../ui'

export default function Login() {
  const emailFieldID = 'login-email'
  const passwordFieldID = 'login-password'
  const navigate = useNavigate()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const res = await fetch('/api/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, password }),
      })
      const data = await readJSONResponse(res)
      if (!res.ok) {
        throw new Error(data.message || data.error || 'Login failed')
      }
      storeAuthSession(data.token, data.session_id)
      navigate('/')
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="login-page">
      <div className="login-card">
        <h1>OpenClause</h1>
        <p className="login-subtitle">Sign in to the admin console</p>
        {error ? <InlineErrorState message={error} /> : null}
        <form onSubmit={handleSubmit}>
          <div className="form-group">
            <label htmlFor={emailFieldID}>Email</label>
            <input
              id={emailFieldID}
              type="email"
              value={email}
              onChange={e => setEmail(e.target.value)}
              placeholder="admin@example.com"
              required
              autoFocus
            />
          </div>
          <div className="form-group">
            <label htmlFor={passwordFieldID}>Password</label>
            <input
              id={passwordFieldID}
              type="password"
              value={password}
              onChange={e => setPassword(e.target.value)}
              placeholder="••••••••"
              required
            />
          </div>
          <button className="btn btn-primary" style={{ width: '100%', marginTop: 8 }} disabled={loading}>
            {loading ? 'Signing in…' : 'Sign in'}
          </button>
        </form>
        <div className="auth-page-links">
          <Link to="/reset" className="auth-page-link">Forgot password?</Link>
        </div>
      </div>
    </div>
  )
}
