import { useState, useEffect, FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { api, formatDate } from '../api'
import { EmptyState, InlineErrorState, PageHeaderBlock, TableSkeleton, shortID } from '../ui'

interface Tenant {
  id: string
  name: string
  status: string
  created_at: string
}

export default function Tenants() {
  const [tenants, setTenants] = useState<Tenant[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState({ name: '' })
  const [creating, setCreating] = useState(false)

  async function fetchTenants() {
    setLoading(true)
    setError('')
    try {
      const data = await api.get('/admin/tenants')
      setTenants(Array.isArray(data) ? data : data?.tenants || [])
    } catch (err: any) {
      setError(err?.message || 'Failed to load tenants')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { fetchTenants() }, [])

  async function handleCreate(e: FormEvent) {
    e.preventDefault()
    setCreating(true)
    setError('')
    try {
      await api.post('/admin/tenants', { name: form.name })
      setForm({ name: '' })
      setShowForm(false)
      await fetchTenants()
    } catch (err: any) {
      setError(err.message)
    } finally {
      setCreating(false)
    }
  }

  return (
    <div>
      <PageHeaderBlock
        title="Tenants"
        description="Manage organizations using OpenClause, review their current state, and jump straight into tenant-level operations."
        actions={
          <div className="btn-group">
            <button className="btn btn-outline" type="button" onClick={() => void fetchTenants()} disabled={loading}>
              Refresh
            </button>
            <button className="btn btn-primary" type="button" onClick={() => setShowForm(f => !f)}>
              {showForm ? 'Cancel' : '+ New Tenant'}
            </button>
          </div>
        }
      />

      {error ? <InlineErrorState message={error} onRetry={() => void fetchTenants()} /> : null}

      {showForm && (
        <div className="form-card">
          <h3>Create Tenant</h3>
          <form onSubmit={handleCreate}>
            <div className="form-inline">
              <div className="form-group">
                <label>Name</label>
                <input value={form.name} onChange={e => setForm({ name: e.target.value })} required />
              </div>
              <button className="btn btn-primary" disabled={creating}>
                {creating ? 'Creating…' : 'Create'}
              </button>
            </div>
          </form>
        </div>
      )}

      <div className="table-container">
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>ID</th>
              <th>Status</th>
              <th>Created</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <TableSkeleton columns={5} rows={6} />
            ) : tenants.length === 0 ? (
              <tr>
                <td colSpan={5} style={{ padding: 0 }}>
                  <EmptyState
                    icon="⊞"
                    title="No tenants yet"
                    description="Create the first tenant to start registering agents, API keys, approvers, and policy settings."
                  />
                </td>
              </tr>
            ) : (
              tenants.map(t => (
                <tr key={t.id}>
                  <td>
                    <Link to={`/tenants/${t.id}`} className="table-primary">{t.name}</Link>
                    <div className="table-subtext mono">{shortID(t.id, 12)}</div>
                  </td>
                  <td className="mono">{shortID(t.id, 12)}</td>
                  <td>
                    <span className={`badge ${t.status === 'active' ? 'badge-green' : 'badge-red'}`}>
                      {t.status}
                    </span>
                  </td>
                  <td>{formatDate(t.created_at, 'date')}</td>
                  <td><Link to={`/tenants/${t.id}`} className="btn btn-outline btn-sm">View</Link></td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}
