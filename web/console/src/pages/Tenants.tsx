import { useState, useEffect, FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'

interface Tenant {
  id: string
  name: string
  slug: string
  plan: string
  created_at: string
}

export default function Tenants() {
  const [tenants, setTenants] = useState<Tenant[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState({ name: '', slug: '', plan: 'free' })
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
      await api.post('/admin/tenants', form)
      setForm({ name: '', slug: '', plan: 'free' })
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
                <input value={form.name} onChange={e => setForm(f => ({ ...f, name: e.target.value }))} required />
              </div>
              <div className="form-group">
                <label>Slug</label>
                <input value={form.slug} onChange={e => setForm(f => ({ ...f, slug: e.target.value }))} required />
              </div>
              <div className="form-group">
                <label>Plan</label>
                <select value={form.plan} onChange={e => setForm(f => ({ ...f, plan: e.target.value }))}>
                  <option value="free">Free</option>
                  <option value="pro">Pro</option>
                  <option value="enterprise">Enterprise</option>
                </select>
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
              <th>Slug</th>
              <th>Plan</th>
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
                  <td><span className="badge badge-gray">{t.slug}</span></td>
                  <td><span className="badge badge-blue">{t.plan}</span></td>
                  <td>{new Date(t.created_at).toLocaleDateString()}</td>
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
