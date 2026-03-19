import { useState, FormEvent } from 'react'
import { api } from '../api'
import { useSearchParams } from 'react-router-dom'

export default function PasswordReset() {
  const [searchParams] = useSearchParams()
  const presetToken = searchParams.get('token') || ''

  const [requestEmail, setRequestEmail] = useState('')
  const [requestStatus, setRequestStatus] = useState('')
  const [confirmToken, setConfirmToken] = useState(presetToken)
  const [confirmPassword, setConfirmPassword] = useState('')
  const [confirmStatus, setConfirmStatus] = useState('')
  const [error, setError] = useState('')

  async function handleResetRequest(e: FormEvent) {
    e.preventDefault()
    setError('')
    setRequestStatus('')
    try {
      await api.post('/auth/reset/request', { email: requestEmail })
      setRequestStatus('Reset request created. Check console logs for the token (dev mode).')
    } catch (err: any) {
      setError(err.message || 'Failed to request reset')
    }
  }

  async function handleResetConfirm(e: FormEvent) {
    e.preventDefault()
    setError('')
    setConfirmStatus('')
    try {
      if (!confirmToken) throw new Error('token is required')
      await api.post('/auth/reset/confirm', { token: confirmToken, password: confirmPassword })
      setConfirmStatus('Password updated. You can now log in.')
      setConfirmPassword('')
    } catch (err: any) {
      setError(err.message || 'Failed to confirm reset')
    }
  }

  return (
    <div className="auth-page">
      <div className="page-header">
        <h2>Password Reset</h2>
        <p>Request a reset token, then confirm with the token.</p>
      </div>

      {error && <div className="error-msg">{error}</div>}

      <div className="grid-2 mt-16">
        <div className="form-card">
          <h3>Request Reset</h3>
          <form onSubmit={handleResetRequest}>
            <div className="form-group">
              <label>Email</label>
              <input value={requestEmail} onChange={e => setRequestEmail(e.target.value)} required />
            </div>
            <button className="btn btn-primary mt-8" type="submit">
              Request
            </button>
          </form>
          {requestStatus && <div className="success-msg mt-8">{requestStatus}</div>}
        </div>

        <div className="form-card">
          <h3>Confirm Reset</h3>
          <form onSubmit={handleResetConfirm}>
            <div className="form-group">
              <label>Token</label>
              <input value={confirmToken} onChange={e => setConfirmToken(e.target.value)} required />
            </div>
            <div className="form-group">
              <label>New Password</label>
              <input type="password" value={confirmPassword} onChange={e => setConfirmPassword(e.target.value)} required />
            </div>
            <button className="btn btn-primary mt-8" type="submit">
              Update password
            </button>
          </form>
          {confirmStatus && <div className="success-msg mt-8">{confirmStatus}</div>}
        </div>
      </div>
    </div>
  )
}

