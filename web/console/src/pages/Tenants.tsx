import { useState, useEffect, FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { api, formatDate } from '../api'

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
    try {
      const data = await api.get('/admin/tenants')
      setTenants(Array.isArray(data) ? data : data?.tenants || [])
    } catch (err: any) {
      setError(err.message)
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
      <div className="flex-between">
        <div className="page-header">
          <h2>Tenants</h2>
          <p>Manage organizations using OpenClause</p>
        </div>
        <button className="btn btn-primary" onClick={() => setShowForm(f => !f)}>
          {showForm ? 'Cancel' : '+ New Tenant'}
        </button>
      </div>

      {error && <div className="error-msg">{error}</div>}

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
              <tr><td colSpan={5} className="loading">Loading…</td></tr>
            ) : tenants.length === 0 ? (
              <tr><td colSpan={5} style={{ textAlign: 'center', padding: 32, color: '#94a3b8' }}>No tenants yet</td></tr>
            ) : (
              tenants.map(t => (
                <tr key={t.id}>
                  <td><Link to={`/tenants/${t.id}`}>{t.name}</Link></td>
                  <td style={{ fontFamily: 'monospace', fontSize: 12 }}>{t.id.slice(0, 12)}</td>
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
