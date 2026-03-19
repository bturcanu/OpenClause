import { useEffect, useState, FormEvent } from 'react'
import { api, formatDate } from '../api'

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
}

export default function Users() {
  const [users, setUsers] = useState<User[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [copyStatus, setCopyStatus] = useState('')
  const [inviteCreated, setInviteCreated] = useState<{
    token: string
    expires_at?: string
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
      setInviteCreated({ token: resp?.token, expires_at: resp?.expires_at })
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

  return (
    <div>
      <div className="flex-between">
        <div className="page-header">
          <h2>Users</h2>
          <p>Manage console users, roles, and invites</p>
        </div>
      </div>

      {error && <div className="error-msg">{error}</div>}
      {loading ? <div className="loading">Loading…</div> : null}

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
          <h3>Invite User (token)</h3>
          <form onSubmit={handleCreateInvite}>
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
            <button className="btn btn-primary mt-8" type="submit">
              Create invite
            </button>
          </form>
          {inviteCreated && (
            <div style={{ marginTop: 14 }}>
              <div style={{ fontSize: 13, fontWeight: 700, marginBottom: 6 }}>
                Invite created
              </div>
              <div className="detail-row" style={{ display: 'block', marginBottom: 8 }}>
                <div style={{ fontSize: 12, color: '#64748b' }}>Accept link</div>
                {(() => {
                  const acceptUrl = `/invite/accept?token=${encodeURIComponent(inviteCreated.token)}`
                  return (
                    <>
                      <a href={acceptUrl} target="_blank" rel="noreferrer">
                        Open accept page
                      </a>
                      <div style={{ marginTop: 6 }}>
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
                    </>
                  )
                })()}
              </div>
              {copyStatus && (
                <div style={{ marginTop: 8, color: '#16a34a', fontSize: 12, fontWeight: 700 }}>
                  {copyStatus}
                </div>
              )}
              <div style={{ fontSize: 12, color: '#64748b' }}>Token</div>
              <div
                style={{
                  fontFamily: 'monospace',
                  fontSize: 12,
                  background: '#f1f5f9',
                  padding: 10,
                  borderRadius: 6,
                  wordBreak: 'break-all',
                  marginTop: 6,
                }}
              >
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

      <div className="table-container mt-16">
        <table>
          <thead>
            <tr>
              <th>Email</th>
              <th>Name</th>
              <th>Slack</th>
              <th>Created</th>
              <th>Roles</th>
            </tr>
          </thead>
          <tbody>
            {users.length === 0 ? (
              <tr>
                <td colSpan={5} style={{ textAlign: 'center', padding: 24, color: '#94a3b8' }}>
                  No users
                </td>
              </tr>
            ) : (
              users.map(u => (
                <tr key={u.id}>
                  <td>{u.email}</td>
                  <td>{u.name || '—'}</td>
                  <td style={{ fontFamily: 'monospace', fontSize: 12 }}>{u.slack_user_id ? u.slack_user_id : '—'}</td>
                  <td>{formatDate(u.created_at, 'date')}</td>
                  <td>
                    {u.roles.length === 0 ? (
                      '—'
                    ) : (
                      <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                        {u.roles.map(rr => (
                          <div key={rr.id} style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                            <span className="badge badge-green" style={{ textTransform: 'none' }}>
                              {rr.role}
                            </span>
                            <span style={{ fontFamily: 'monospace', fontSize: 12, color: '#64748b' }}>
                              {rr.tenant_id ?? 'platform'}
                            </span>
                            <button className="btn btn-danger btn-sm" onClick={() => handleRemoveRole(u.id, rr.id)}>
                              Remove
                            </button>
                          </div>
                        ))}
                      </div>
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

