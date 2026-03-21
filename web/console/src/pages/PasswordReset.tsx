import { useState, FormEvent } from 'react'
import { api } from '../api'
import { Link, useSearchParams } from 'react-router-dom'
import { InlineErrorState } from '../ui'

export default function PasswordReset() {
  const [searchParams] = useSearchParams()
  const presetToken = searchParams.get('token') || ''

  const [requestEmail, setRequestEmail] = useState('')
  const [requestStatus, setRequestStatus] = useState('')
  const [requestLoading, setRequestLoading] = useState(false)
  const [confirmToken, setConfirmToken] = useState(presetToken)
  const [confirmPassword, setConfirmPassword] = useState('')
  const [confirmStatus, setConfirmStatus] = useState('')
  const [confirmLoading, setConfirmLoading] = useState(false)
  const [error, setError] = useState('')

  async function handleResetRequest(e: FormEvent) {
    e.preventDefault()
    setError('')
    setRequestStatus('')
    setRequestLoading(true)
    try {
      await api.unauthPost('/auth/reset/request', { email: requestEmail })
      setRequestStatus('Reset request created. Check console logs for the token (dev mode).')
    } catch (err: any) {
      setError(err.message || 'Failed to request reset')
    } finally {
      setRequestLoading(false)
    }
  }

  async function handleResetConfirm(e: FormEvent) {
    e.preventDefault()
    setError('')
    setConfirmStatus('')
    setConfirmLoading(true)
    try {
      if (!confirmToken) throw new Error('token is required')
      await api.unauthPost('/auth/reset/confirm', { token: confirmToken, password: confirmPassword })
      setConfirmStatus('Password updated. You can now log in.')
      setConfirmPassword('')
    } catch (err: any) {
      setError(err.message || 'Failed to confirm reset')
    } finally {
      setConfirmLoading(false)
    }
  }

  return (
    <div className="auth-page">
      <div className="page-header">
        <h2>Password Reset</h2>
        <p>Request a reset token, then confirm with the token.</p>
      </div>

      {error ? <InlineErrorState message={error} /> : null}

      <div className="grid-2 mt-16">
        <div className="form-card">
          <h3>Request Reset</h3>
          <form onSubmit={handleResetRequest}>
            <div className="form-group">
              <label>Email</label>
              <input value={requestEmail} onChange={e => setRequestEmail(e.target.value)} required />
              <div className="form-helper-text">In local development, reset links may also be logged by console-api if SMTP is not configured.</div>
            </div>
            <button className="btn btn-primary mt-8" type="submit" disabled={requestLoading}>
              {requestLoading ? 'Requesting…' : 'Request'}
            </button>
          </form>
          {requestStatus && <div className="success-msg mt-8">{requestStatus}</div>}
        </div>

        <div className="form-card">
          <h3>Confirm Reset</h3>
          <form onSubmit={handleResetConfirm}>
            <div className="form-group">
              <label>Token</label>
              <input className="mono" value={confirmToken} onChange={e => setConfirmToken(e.target.value)} required />
              <div className="form-helper-text">If you came from a reset email, the token should already be populated from the link.</div>
            </div>
            <div className="form-group">
              <label>New Password</label>
              <input type="password" value={confirmPassword} onChange={e => setConfirmPassword(e.target.value)} required />
            </div>
            <button className="btn btn-primary mt-8" type="submit" disabled={confirmLoading}>
              {confirmLoading ? 'Updating…' : 'Update password'}
            </button>
          </form>
          {confirmStatus && <div className="success-msg mt-8">{confirmStatus}</div>}
        </div>
      </div>
      <div className="auth-page-links">
        <Link to="/login" className="auth-page-link">Back to sign in</Link>
      </div>
    </div>
  )
}
