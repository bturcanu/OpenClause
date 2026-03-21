import { useEffect, useState, FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, clearStoredAuth, formatDate, getStoredAuthClaims, getStoredSessionID } from '../api'
import { EmptyState, InlineErrorState, PageHeaderBlock } from '../ui'

type UserRole = {
  id: string
  user_id: string
  tenant_id?: string | null
  role: string
}

type User = {
  id: string
  email: string
  name: string
  slack_user_id?: string | null
  status: string
  created_at: string
  roles: UserRole[]
  active_session_count?: number
}

type AuthSession = {
  id: string
  user_id: string
  tenant_id?: string
  user_agent?: string
  client_ip?: string
  created_at: string
  last_seen_at: string
  expires_at: string
}

export default function Users() {
  const navigate = useNavigate()
  const [users, setUsers] = useState<User[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [canManageUsers, setCanManageUsers] = useState(false)
  const [canManageSessions, setCanManageSessions] = useState(false)
  const [currentSessionID, setCurrentSessionID] = useState('')
  const [expandedUserID, setExpandedUserID] = useState('')
  const [sessionsLoadingUserID, setSessionsLoadingUserID] = useState('')
  const [revokingSessionID, setRevokingSessionID] = useState('')
  const [authSessionsByUser, setAuthSessionsByUser] = useState<Record<string, AuthSession[]>>({})
  const [copyStatus, setCopyStatus] = useState('')
  const [inviteCreated, setInviteCreated] = useState<{
    token: string
    expires_at?: string
    accept_url?: string
    email_status?: string
    email_error?: string
  } | null>(null)

  const [createUserForm, setCreateUserForm] = useState({
    email: '',
    name: '',
    password: '',
    slack_user_id: '',
  })

  const [inviteForm, setInviteForm] = useState({
    email: '',
    tenant_id: '',
    role: 'tenant_admin' as 'tenant_admin' | 'approver' | 'viewer',
    name: '',
  })

  const [assignRoleForm, setAssignRoleForm] = useState({
    user_id: '',
    role: 'viewer' as 'tenant_admin' | 'approver' | 'viewer',
    tenant_id: '',
  })

  useEffect(() => {
    const claims = getStoredAuthClaims()
    const roles = Array.isArray(claims?.roles) ? claims.roles : []
    const isAdmin = roles.includes('platform_admin') || roles.includes('tenant_admin')
    setCanManageUsers(isAdmin)
    setCanManageSessions(isAdmin)
    setCurrentSessionID(getStoredSessionID())
  }, [])

  async function fetchUsers() {
    setLoading(true)
    setError('')
    try {
      const data = await api.get('/admin/users')
      setUsers(Array.isArray(data?.users) ? data.users : [])
    } catch (err: any) {
      setError(err.message || 'Failed to load users')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void fetchUsers()
  }, [])

  const [submitting, setSubmitting] = useState(false)

  async function handleCreateUser(e: FormEvent) {
    e.preventDefault()
    setError('')
    setSubmitting(true)
    try {
      const payload: any = {
        email: createUserForm.email,
        name: createUserForm.name,
      }
      if (createUserForm.password.trim()) payload.password = createUserForm.password
      if (createUserForm.slack_user_id.trim()) payload.slack_user_id = createUserForm.slack_user_id.trim()
      await api.post('/admin/users', payload)
      setCreateUserForm({ email: '', name: '', password: '', slack_user_id: '' })
      await fetchUsers()
    } catch (err: any) {
      setError(err.message || 'Failed to create user')
    } finally {
      setSubmitting(false)
    }
  }

  async function handleCreateInvite(e: FormEvent) {
    e.preventDefault()
    setError('')
    setCopyStatus('')
    try {
      const resp = await api.post('/admin/invites', {
        email: inviteForm.email,
        tenant_id: inviteForm.tenant_id,
        role: inviteForm.role,
        name: inviteForm.name || undefined,
      })
      setInviteCreated({
        token: resp?.token,
        expires_at: resp?.expires_at,
        accept_url: resp?.accept_url,
        email_status: resp?.email_status,
        email_error: resp?.email_error,
      })
      setInviteForm({ email: '', tenant_id: '', role: 'tenant_admin', name: '' })
      await fetchUsers()
    } catch (err: any) {
      setError(err.message || 'Failed to create invite')
    }
  }

  async function handleAssignRole(e: FormEvent) {
    e.preventDefault()
    setError('')
    try {
      if (!assignRoleForm.user_id) throw new Error('Select a user')
      if (!assignRoleForm.tenant_id) throw new Error('tenant_id is required')
      await api.post(`/admin/users/${assignRoleForm.user_id}/roles`, {
        role: assignRoleForm.role,
        tenant_id: assignRoleForm.tenant_id,
      })
      await fetchUsers()
    } catch (err: any) {
      setError(err.message || 'Failed to assign role')
    }
  }

  async function handleRemoveRole(userID: string, roleID: string) {
    setError('')
    try {
      await api.delete(`/admin/users/${userID}/roles/${roleID}`)
      await fetchUsers()
    } catch (err: any) {
      setError(err.message || 'Failed to remove role')
    }
  }

  async function loadUserSessions(userID: string) {
    setSessionsLoadingUserID(userID)
    setError('')
    try {
      const data = await api.get(`/admin/auth-sessions?user_id=${encodeURIComponent(userID)}`)
      setAuthSessionsByUser(prev => ({
        ...prev,
        [userID]: Array.isArray(data?.sessions) ? data.sessions : [],
      }))
    } catch (err: any) {
      setError(err.message || 'Failed to load active sessions')
    } finally {
      setSessionsLoadingUserID('')
    }
  }

  async function toggleUserSessions(userID: string) {
    if (expandedUserID === userID) {
      setExpandedUserID('')
      return
    }
    setExpandedUserID(userID)
    if (!authSessionsByUser[userID]) {
      await loadUserSessions(userID)
    }
  }

  async function handleRevokeSession(userID: string, sessionID: string) {
    setRevokingSessionID(sessionID)
    setError('')
    try {
      await api.post(`/admin/auth-sessions/${sessionID}/revoke`)
      setAuthSessionsByUser(prev => ({
        ...prev,
        [userID]: (prev[userID] || []).filter(s => s.id !== sessionID),
      }))
      setUsers(prev => prev.map(u => (
        u.id === userID
          ? { ...u, active_session_count: Math.max(0, (u.active_session_count || 0) - 1) }
          : u
      )))
      if (sessionID === currentSessionID) {
        clearStoredAuth()
        navigate('/login')
      }
    } catch (err: any) {
      setError(err.message || 'Failed to revoke session')
    } finally {
      setRevokingSessionID('')
    }
  }

  function formatClient(session: AuthSession) {
    const parts = [session.client_ip, session.user_agent].filter(Boolean)
    if (parts.length === 0) return 'Unknown client'
    return parts.join(' · ')
  }

  return (
    <div>
      <PageHeaderBlock
        title="Users"
        description="Manage console users, invite access, role assignments, and active login sessions without leaving the console."
      />

      {error ? <InlineErrorState message={error} onRetry={() => void fetchUsers()} /> : null}
      {loading ? <div className="loading">Loading…</div> : null}

      {canManageUsers ? (
        <>
          <div className="grid-2 mt-16">
            <div className="form-card">
              <h3>Create User</h3>
              <form onSubmit={handleCreateUser}>
                <div className="form-group">
                  <label>Email</label>
                  <input
                    value={createUserForm.email}
                    onChange={e => setCreateUserForm({ ...createUserForm, email: e.target.value })}
                    required
                  />
                </div>
                <div className="form-group">
                  <label>Name</label>
                  <input
                    value={createUserForm.name}
                    onChange={e => setCreateUserForm({ ...createUserForm, name: e.target.value })}
                  />
                </div>
                <div className="form-group">
                  <label>Password (optional)</label>
                  <input
                    type="password"
                    value={createUserForm.password}
                    onChange={e => setCreateUserForm({ ...createUserForm, password: e.target.value })}
                    placeholder="Set password now or invite/reset later"
                  />
                </div>
                <div className="form-group">
                  <label>Slack user id (optional)</label>
                  <input
                    value={createUserForm.slack_user_id}
                    onChange={e => setCreateUserForm({ ...createUserForm, slack_user_id: e.target.value })}
                  />
                </div>
                <button className="btn btn-primary mt-8" type="submit" disabled={submitting}>
                  {submitting ? 'Creating…' : 'Create'}
                </button>
              </form>
            </div>

            <div className="form-card">
              <h3>Invite User</h3>
              <form onSubmit={handleCreateInvite}>
                <div className="form-grid form-grid-2">
                  <div className="form-group">
                    <label>Email</label>
                    <input value={inviteForm.email} onChange={e => setInviteForm({ ...inviteForm, email: e.target.value })} required />
                  </div>
                  <div className="form-group">
                    <label>Tenant ID</label>
                    <input
                      value={inviteForm.tenant_id}
                      onChange={e => setInviteForm({ ...inviteForm, tenant_id: e.target.value })}
                      required
                    />
                  </div>
                  <div className="form-group">
                    <label>Role</label>
                    <select value={inviteForm.role} onChange={e => setInviteForm({ ...inviteForm, role: e.target.value as any })}>
                      <option value="tenant_admin">tenant_admin</option>
                      <option value="approver">approver</option>
                      <option value="viewer">viewer</option>
                    </select>
                  </div>
                  <div className="form-group">
                    <label>Name (optional)</label>
                    <input value={inviteForm.name} onChange={e => setInviteForm({ ...inviteForm, name: e.target.value })} />
                  </div>
                </div>
                <div className="form-actions-row">
                  <p className="form-helper-text">
                    Invites create a time-limited accept link. If email delivery fails, admins can still copy the link below.
                  </p>
                  <button className="btn btn-primary" type="submit">
                    Create invite
                  </button>
                </div>
              </form>
              {inviteCreated && (
                <div className="detail-panel invite-result-panel">
                  <div className="invite-result-header">
                    <div>
                      <div className="meta-label">Invite status</div>
                      <div className="invite-result-title">Invite created</div>
                    </div>
                    <div className={`invite-status-pill invite-status-${inviteCreated.email_status || 'ready'}`}>
                      {inviteCreated.email_status === 'sent' ? 'Email sent' : null}
                      {inviteCreated.email_status === 'failed' ? 'Email failed' : null}
                      {inviteCreated.email_status === 'logged' ? 'Logged for dev' : null}
                      {!inviteCreated.email_status ? 'Link ready' : null}
                    </div>
                  </div>
                  <div className="table-subtext">
                    {inviteCreated.email_status === 'sent' ? 'Email sent' : null}
                    {inviteCreated.email_status === 'failed' ? `Email failed (copy link instead)${inviteCreated.email_error ? `: ${inviteCreated.email_error}` : ''}` : null}
                    {inviteCreated.email_status === 'logged' ? 'Invite link logged for dev; copy it below or check console-api logs.' : null}
                    {!inviteCreated.email_status ? 'Invite link ready to copy.' : null}
                  </div>
                  <div className="detail-row detail-row-block">
                    <div className="meta-label">Accept link</div>
                    {(() => {
                      const acceptUrl = inviteCreated.accept_url || new URL(`/invite/accept?token=${encodeURIComponent(inviteCreated.token)}`, window.location.origin).toString()
                      return (
                        <div className="invite-link-actions">
                          <a href={acceptUrl} target="_blank" rel="noreferrer" className="link-button">
                            Open accept page
                          </a>
                          <button
                            className="btn btn-outline btn-sm"
                            type="button"
                            onClick={async () => {
                              try {
                                await navigator.clipboard.writeText(acceptUrl)
                                setCopyStatus('Link copied')
                                setTimeout(() => setCopyStatus(''), 1500)
                              } catch {
                                setCopyStatus('Copy failed')
                                setTimeout(() => setCopyStatus(''), 1500)
                              }
                            }}
                          >
                            Copy link
                          </button>
                        </div>
                      )
                    })()}
                  </div>
                  {copyStatus && (
                    <div className="success-inline">
                      {copyStatus}
                    </div>
                  )}
                  <div className="meta-label">Token</div>
                  <div className="invite-token-block">
                    {inviteCreated.token}
                  </div>
                </div>
              )}
            </div>
          </div>

          <div className="form-card mt-16">
            <h3>Assign Role</h3>
            <form onSubmit={handleAssignRole}>
              <div className="form-inline" style={{ gap: 16, flexWrap: 'wrap' }}>
                <div className="form-group" style={{ minWidth: 280 }}>
                  <label>User</label>
                  <select value={assignRoleForm.user_id} onChange={e => setAssignRoleForm({ ...assignRoleForm, user_id: e.target.value })}>
                    <option value="">Select user…</option>
                    {users.map(u => (
                      <option key={u.id} value={u.id}>
                        {u.email}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="form-group" style={{ minWidth: 220 }}>
                  <label>Role</label>
                  <select value={assignRoleForm.role} onChange={e => setAssignRoleForm({ ...assignRoleForm, role: e.target.value as any })}>
                    <option value="tenant_admin">tenant_admin</option>
                    <option value="approver">approver</option>
                    <option value="viewer">viewer</option>
                  </select>
                </div>
                <div className="form-group" style={{ minWidth: 280 }}>
                  <label>Tenant ID</label>
                  <input
                    value={assignRoleForm.tenant_id}
                    onChange={e => setAssignRoleForm({ ...assignRoleForm, tenant_id: e.target.value })}
                    placeholder="tenant1"
                  />
                </div>
                <button className="btn btn-primary" type="submit">
                  Assign
                </button>
              </div>
            </form>
          </div>
        </>
      ) : (
        <div className="detail-panel mt-16">
          <h3>Read-only Access</h3>
          <p style={{ margin: 0, color: '#64748b' }}>Viewer access can inspect users, but user changes and login-session revocation are reserved for tenant_admin and platform_admin roles.</p>
        </div>
      )}

      <div className="table-container mt-16">
        <table>
          <thead>
            <tr>
              <th>Email</th>
              <th>Name</th>
              <th>Slack</th>
              <th>Created</th>
              <th>Roles</th>
              {canManageSessions ? <th>Sessions</th> : null}
            </tr>
          </thead>
          <tbody>
            {users.length === 0 ? (
              <tr>
                <td colSpan={canManageSessions ? 6 : 5} style={{ textAlign: 'center', padding: 24, color: '#94a3b8' }}>
                  No users
                </td>
              </tr>
            ) : (
              users.flatMap(u => {
                const sessions = authSessionsByUser[u.id] || []
                const isExpanded = expandedUserID === u.id
                return [
                  <tr key={u.id}>
                    <td>{u.email}</td>
                    <td>{u.name || '—'}</td>
                    <td style={{ fontFamily: 'monospace', fontSize: 12 }}>{u.slack_user_id ? u.slack_user_id : '—'}</td>
                    <td>{formatDate(u.created_at, 'date')}</td>
                    <td>
                      {u.roles.length === 0 ? (
                        '—'
                      ) : (
                        <div className="role-list">
                          {u.roles.map(rr => (
                            <div key={rr.id} className="role-item">
                              <span className="badge badge-green badge-lower">
                                {rr.role}
                              </span>
                              <span className="role-scope mono">
                                {rr.tenant_id ?? 'platform'}
                              </span>
                              {canManageUsers ? (
                                <button className="btn btn-danger btn-sm role-action-button" onClick={() => handleRemoveRole(u.id, rr.id)}>
                                  Remove
                                </button>
                              ) : null}
                            </div>
                          ))}
                        </div>
                      )}
                    </td>
                    {canManageSessions ? (
                      <td style={{ minWidth: 180 }}>
                        <div className="session-summary-cell">
                          <span className={`badge ${(u.active_session_count || 0) > 0 ? 'badge-green' : 'badge-gray'}`}>
                            {u.active_session_count || 0} active
                          </span>
                          <button
                            className="btn btn-outline btn-sm"
                            onClick={() => void toggleUserSessions(u.id)}
                            disabled={sessionsLoadingUserID === u.id}
                          >
                            {isExpanded ? 'Hide' : 'Review'}
                          </button>
                        </div>
                      </td>
                    ) : null}
                  </tr>,
                  ...(canManageSessions && isExpanded ? [
                    <tr key={`${u.id}-sessions`}>
                      <td colSpan={6} style={{ background: '#f8fafc' }}>
                        <div className="detail-panel" style={{ margin: 12 }}>
                          <h3>Active Login Sessions</h3>
                          {sessionsLoadingUserID === u.id ? (
                            <div className="loading">Loading sessions…</div>
                          ) : sessions.length === 0 ? (
                            <EmptyState
                              icon="↻"
                              title="No active login sessions"
                              description="This user does not have any active console sessions to revoke right now."
                            />
                          ) : (
                            <div className="table-container" style={{ marginBottom: 0 }}>
                              <table>
                                <thead>
                                  <tr>
                                    <th>Session</th>
                                    <th>Created</th>
                                    <th>Last Seen</th>
                                    <th>Expires</th>
                                    <th>Client</th>
                                    <th></th>
                                  </tr>
                                </thead>
                                <tbody>
                                  {sessions.map(session => (
                                    <tr key={session.id}>
                                      <td style={{ fontFamily: 'monospace', fontSize: 12 }}>
                                        {session.id.slice(0, 12)}…
                                        {session.id === currentSessionID ? (
                                          <span className="badge badge-green" style={{ marginLeft: 8 }}>Current</span>
                                        ) : null}
                                      </td>
                                      <td>{formatDate(session.created_at)}</td>
                                      <td>{formatDate(session.last_seen_at)}</td>
                                      <td>{formatDate(session.expires_at)}</td>
                                      <td style={{ maxWidth: 360, fontSize: 12, color: '#64748b' }}>{formatClient(session)}</td>
                                      <td>
                                        <button
                                          className="btn btn-danger btn-sm"
                                          onClick={() => void handleRevokeSession(u.id, session.id)}
                                          disabled={revokingSessionID === session.id}
                                        >
                                          {revokingSessionID === session.id ? 'Revoking…' : 'Revoke'}
                                        </button>
                                      </td>
                                    </tr>
                                  ))}
                                </tbody>
                              </table>
                            </div>
                          )}
                        </div>
                      </td>
                    </tr>,
                  ] : []),
                ]
              })
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}
