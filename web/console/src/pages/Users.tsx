import { useEffect, useMemo, useState, FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, clearStoredAuth, formatDate, getStoredAuthClaims, getStoredSessionID } from '../api'
import {
  ActiveFiltersBar,
  CopyIconButton,
  EmptyState,
  InlineErrorState,
  PageHeaderBlock,
  SortHeader,
  TableEmptyStateRow,
  TableFrame,
  applySort,
  compareDate,
  compareNumber,
  compareText,
  copyText,
  formatTimeWithTitle,
  shortID,
  type SortState,
} from '../ui'

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

function roleRank(role: string) {
  switch (role) {
    case 'platform_admin':
      return 0
    case 'tenant_admin':
      return 1
    case 'approver':
      return 2
    case 'viewer':
      return 3
    default:
      return 4
  }
}

function sortRoles(roles: UserRole[]) {
  return [...roles].sort((left, right) => {
    const rankDiff = roleRank(left.role) - roleRank(right.role)
    if (rankDiff !== 0) return rankDiff
    return compareText(left.tenant_id || 'platform', right.tenant_id || 'platform')
  })
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
  const [sortState, setSortState] = useState<SortState<'email' | 'created_at' | 'primary_role' | 'active_session_count'>>({
    key: null,
    dir: 'asc',
  })
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
      setUsers(Array.isArray(data) ? data : Array.isArray(data?.users) ? data.users : [])
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
        [userID]: Array.isArray(data) ? data : Array.isArray(data?.sessions) ? data.sessions : [],
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

  const visibleUsers = useMemo(() => {
    if (!sortState.key) return users
    return [...users].sort((left, right) => {
      const leftRoles = sortRoles(left.roles)
      const rightRoles = sortRoles(right.roles)
      switch (sortState.key) {
        case 'email':
          return applySort(compareText(left.email, right.email), sortState.dir)
        case 'created_at':
          return applySort(compareDate(left.created_at, right.created_at), sortState.dir)
        case 'primary_role':
          return applySort(compareNumber(roleRank(leftRoles[0]?.role || ''), roleRank(rightRoles[0]?.role || '')), sortState.dir)
        case 'active_session_count':
          return applySort(compareNumber(left.active_session_count, right.active_session_count), sortState.dir)
        default:
          return 0
      }
    })
  }, [sortState, users])

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
                  <label htmlFor="users-create-email">Email</label>
                  <input
                    id="users-create-email"
                    value={createUserForm.email}
                    onChange={e => setCreateUserForm({ ...createUserForm, email: e.target.value })}
                    required
                  />
                </div>
                <div className="form-group">
                  <label htmlFor="users-create-name">Name</label>
                  <input
                    id="users-create-name"
                    value={createUserForm.name}
                    onChange={e => setCreateUserForm({ ...createUserForm, name: e.target.value })}
                  />
                </div>
                <div className="form-group">
                  <label htmlFor="users-create-password">Password (optional)</label>
                  <input
                    id="users-create-password"
                    type="password"
                    value={createUserForm.password}
                    onChange={e => setCreateUserForm({ ...createUserForm, password: e.target.value })}
                    placeholder="Set password now or invite/reset later"
                  />
                </div>
                <div className="form-group">
                  <label htmlFor="users-create-slack-user-id">Slack user id (optional)</label>
                  <input
                    id="users-create-slack-user-id"
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
                    <label htmlFor="users-invite-email">Email</label>
                    <input id="users-invite-email" value={inviteForm.email} onChange={e => setInviteForm({ ...inviteForm, email: e.target.value })} required />
                  </div>
                  <div className="form-group">
                    <label htmlFor="users-invite-tenant-id">Tenant ID</label>
                    <input
                      id="users-invite-tenant-id"
                      value={inviteForm.tenant_id}
                      onChange={e => setInviteForm({ ...inviteForm, tenant_id: e.target.value })}
                      required
                    />
                  </div>
                  <div className="form-group">
                    <label htmlFor="users-invite-role">Role</label>
                    <select id="users-invite-role" value={inviteForm.role} onChange={e => setInviteForm({ ...inviteForm, role: e.target.value as any })}>
                      <option value="tenant_admin">tenant_admin</option>
                      <option value="approver">approver</option>
                      <option value="viewer">viewer</option>
                    </select>
                  </div>
                  <div className="form-group">
                    <label htmlFor="users-invite-name">Name (optional)</label>
                    <input id="users-invite-name" value={inviteForm.name} onChange={e => setInviteForm({ ...inviteForm, name: e.target.value })} />
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
                  <div className="table-subtext">
                    The raw invite token is only shown for this create response. Later invite lists keep delivery status but never return the token again.
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
                                await copyText(acceptUrl)
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
                  <div className="detail-row detail-row-block">
                    <div className="meta-label">Raw token (shown only once)</div>
                    <div className="invite-token-block">
                      {inviteCreated.token}
                    </div>
                    <div className="invite-link-actions">
                      <button
                        className="btn btn-outline btn-sm"
                        type="button"
                        onClick={async () => {
                          try {
                            await copyText(inviteCreated.token)
                            setCopyStatus('Token copied')
                            setTimeout(() => setCopyStatus(''), 1500)
                          } catch {
                            setCopyStatus('Copy failed')
                            setTimeout(() => setCopyStatus(''), 1500)
                          }
                        }}
                      >
                        Copy token
                      </button>
                    </div>
                  </div>
                </div>
              )}
            </div>
          </div>

          <div className="form-card mt-16">
            <h3>Assign Role</h3>
            <form onSubmit={handleAssignRole}>
              <div className="form-grid assign-role-grid">
                <div className="form-group">
                  <label htmlFor="users-assign-user">User</label>
                  <select id="users-assign-user" value={assignRoleForm.user_id} onChange={e => setAssignRoleForm({ ...assignRoleForm, user_id: e.target.value })}>
                    <option value="">Select user…</option>
                    {users.map(u => (
                      <option key={u.id} value={u.id}>
                        {u.email}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="form-group">
                  <label htmlFor="users-assign-role">Role</label>
                  <select id="users-assign-role" value={assignRoleForm.role} onChange={e => setAssignRoleForm({ ...assignRoleForm, role: e.target.value as any })}>
                    <option value="tenant_admin">tenant_admin</option>
                    <option value="approver">approver</option>
                    <option value="viewer">viewer</option>
                  </select>
                </div>
                <div className="form-group">
                  <label htmlFor="users-assign-tenant-id">Tenant ID</label>
                  <input
                    id="users-assign-tenant-id"
                    value={assignRoleForm.tenant_id}
                    onChange={e => setAssignRoleForm({ ...assignRoleForm, tenant_id: e.target.value })}
                    placeholder="tenant1"
                  />
                </div>
                <div className="form-actions-row form-actions-row-end assign-role-actions">
                  <button className="btn btn-primary" type="submit">
                    Assign
                  </button>
                </div>
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

      <ActiveFiltersBar
        resultCount={visibleUsers.length}
        resultLabel={visibleUsers.length === 1 ? 'user' : 'users'}
        chips={[]}
        note={sortState.key ? 'Sorted within the current page.' : 'Using backend order until you sort this page.'}
      />

      <TableFrame className="mt-16" stickyHeader>
        <table className="users-table">
          <thead>
            <tr>
              <th>
                <SortHeader label="User" sortKey="email" sortState={sortState} onSortChange={(key, dir) => setSortState({ key, dir })} />
              </th>
              <th>Slack</th>
              <th className="col-time">
                <SortHeader label="Created" sortKey="created_at" sortState={sortState} onSortChange={(key, dir) => setSortState({ key, dir })} defaultDir="desc" className="col-time" />
              </th>
              <th>
                <SortHeader label="Roles" sortKey="primary_role" sortState={sortState} onSortChange={(key, dir) => setSortState({ key, dir })} />
              </th>
              {canManageSessions ? (
                <th className="col-num">
                  <SortHeader label="Sessions" sortKey="active_session_count" sortState={sortState} onSortChange={(key, dir) => setSortState({ key, dir })} defaultDir="desc" className="col-num" />
                </th>
              ) : null}
              {canManageSessions ? <th className="table-action-col"></th> : null}
            </tr>
          </thead>
          <tbody>
            {visibleUsers.length === 0 ? (
              <TableEmptyStateRow
                colSpan={canManageSessions ? 6 : 4}
                icon="◎"
                title="No users yet"
                description="Create a user or send an invite to start assigning roles and reviewing login sessions."
              />
            ) : (
              visibleUsers.flatMap(u => {
                const sessions = authSessionsByUser[u.id] || []
                const isExpanded = expandedUserID === u.id
                const sortedRoles = sortRoles(u.roles)
                const primaryRole = sortedRoles[0]
                const remainingRoles = sortedRoles.slice(1)
                const created = formatTimeWithTitle(u.created_at)
                return [
                  <tr key={u.id}>
                    <td>
                      <div className="table-primary-cell">
                        <div className="table-primary">{u.email}</div>
                        <div className="table-subtext">{u.name || '—'}</div>
                      </div>
                    </td>
                    <td className="users-slack-cell" style={{ fontFamily: 'monospace', fontSize: 12 }}>{u.slack_user_id ? u.slack_user_id : '—'}</td>
                    <td className="col-time" title={created.title}>{created.label}</td>
                    <td>
                      {sortedRoles.length === 0 ? (
                        '—'
                      ) : (
                        <div className="table-primary-cell">
                          <div className="stacked-badges">
                            <span className="badge badge-green badge-lower">{primaryRole.role}</span>
                            <span className="role-scope mono">{primaryRole.tenant_id ?? 'platform'}</span>
                          </div>
                          {remainingRoles.length > 0 ? (
                            <details className="roles-disclosure">
                              <summary>+{remainingRoles.length} more</summary>
                              <div className="role-list">
                                {sortedRoles.map(rr => (
                                  <div key={rr.id} className="role-item">
                                    <span className="badge badge-green badge-lower">{rr.role}</span>
                                    <span className="role-scope mono">{rr.tenant_id ?? 'platform'}</span>
                                    {canManageUsers ? (
                                      <button className="btn btn-danger btn-sm role-action-button" onClick={() => handleRemoveRole(u.id, rr.id)}>
                                        Remove
                                      </button>
                                    ) : null}
                                  </div>
                                ))}
                              </div>
                            </details>
                          ) : null}
                        </div>
                      )}
                    </td>
                    {canManageSessions ? (
                      <td className="users-sessions-cell col-num tabular">
                        <div className="session-summary-cell">
                          <span className={`badge ${(u.active_session_count || 0) > 0 ? 'badge-green' : 'badge-gray'}`}>
                            {u.active_session_count || 0} active
                          </span>
                        </div>
                      </td>
                    ) : null}
                    {canManageSessions ? (
                      <td className="table-action-cell">
                        <button
                          className="btn btn-outline btn-sm"
                          onClick={() => void toggleUserSessions(u.id)}
                          disabled={sessionsLoadingUserID === u.id}
                        >
                          {isExpanded ? 'Hide' : 'Review'}
                        </button>
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
                            <TableFrame style={{ marginBottom: 0 }}>
                              <table>
                                <thead>
                                  <tr>
                                    <th>Session</th>
                                    <th className="col-time">Created</th>
                                    <th className="col-time">Last Seen</th>
                                    <th className="col-time">Expires</th>
                                    <th>Client</th>
                                    <th className="table-action-col"></th>
                                  </tr>
                                </thead>
                                <tbody>
                                  {sessions.map(session => {
                                    const createdAt = formatTimeWithTitle(session.created_at)
                                    const lastSeen = formatTimeWithTitle(session.last_seen_at)
                                    const expiresAt = formatTimeWithTitle(session.expires_at)
                                    const isStale = new Date(session.last_seen_at).getTime() < Date.now() - 24 * 60 * 60 * 1000
                                    return (
                                    <tr key={session.id}>
                                      <td>
                                        <div className="table-primary-cell">
                                          <div className="inline-value-copy">
                                            <code className="mono" title={session.id}>{shortID(session.id, 12)}</code>
                                            <CopyIconButton text={session.id} label="Auth session ID" />
                                            {session.id === currentSessionID ? <span className="badge badge-green">Current</span> : null}
                                            {isStale ? <span className="badge badge-gray">Stale</span> : null}
                                          </div>
                                          <div className="table-subtext">{session.tenant_id || 'Platform-wide session'}</div>
                                        </div>
                                      </td>
                                      <td className="col-time" title={createdAt.title}>{createdAt.label}</td>
                                      <td className="col-time" title={lastSeen.title}>{lastSeen.label}</td>
                                      <td className="col-time" title={expiresAt.title}>{expiresAt.label}</td>
                                      <td style={{ maxWidth: 360, fontSize: 12, color: '#64748b' }}>{formatClient(session)}</td>
                                      <td className="table-action-cell">
                                        <button
                                          className="btn btn-danger btn-sm"
                                          onClick={() => void handleRevokeSession(u.id, session.id)}
                                          disabled={revokingSessionID === session.id}
                                        >
                                          {revokingSessionID === session.id ? 'Revoking…' : 'Revoke'}
                                        </button>
                                      </td>
                                    </tr>
                                  )})}
                                </tbody>
                              </table>
                            </TableFrame>
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
      </TableFrame>
    </div>
  )
}
