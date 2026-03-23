import { useState, useEffect, FormEvent, useMemo } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import {
  CopyIconButton,
  InlineErrorState,
  PageHeaderBlock,
  SortHeader,
  TableEmptyStateRow,
  TableFrame,
  TableSkeleton,
  applySort,
  compareDate,
  compareText,
  formatTimeWithTitle,
  shortID,
  type SortState,
} from '../ui'

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
  const [search, setSearch] = useState('')
  const [sortState, setSortState] = useState<SortState<'name' | 'status' | 'created_at'>>({ key: null, dir: 'asc' })

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

  const filteredTenants = useMemo(() => {
    const query = search.trim().toLowerCase()
    const next = query
      ? tenants.filter(tenant =>
        tenant.name.toLowerCase().includes(query) ||
        tenant.id.toLowerCase().includes(query) ||
        tenant.status.toLowerCase().includes(query))
      : [...tenants]

    if (!sortState.key) return next

    return [...next].sort((left, right) => {
      switch (sortState.key) {
        case 'name':
          return applySort(compareText(left.name, right.name), sortState.dir)
        case 'status':
          return applySort(compareText(left.status, right.status), sortState.dir)
        case 'created_at':
          return applySort(compareDate(left.created_at, right.created_at), sortState.dir)
        default:
          return 0
      }
    })
  }, [search, sortState, tenants])

  return (
    <div>
      <PageHeaderBlock
        title="Tenants"
        description="Manage organizations using OpenClause, review their current state, and jump straight into tenant-level operations."
        actions={
          <div className="btn-group">
            <div className="form-group connectors-search">
              <label>Search</label>
              <input value={search} onChange={e => setSearch(e.target.value)} placeholder="Find by tenant name or ID" />
            </div>
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

      <div className="active-filters-bar">
        <div className="active-filters-summary">
          <strong>Showing {filteredTenants.length.toLocaleString()} tenants</strong>
          <span className="active-filters-note">
            {sortState.key ? 'Sorted within the current page.' : 'Using backend order until you sort this page.'}
          </span>
        </div>
      </div>

      <TableFrame className="table-sticky" stickyHeader>
        <table>
          <thead>
            <tr>
              <th>
                <SortHeader label="Name" sortKey="name" sortState={sortState} onSortChange={(key, dir) => setSortState({ key, dir })} />
              </th>
              <th>ID</th>
              <th>
                <SortHeader label="Status" sortKey="status" sortState={sortState} onSortChange={(key, dir) => setSortState({ key, dir })} />
              </th>
              <th className="col-time">
                <SortHeader label="Created" sortKey="created_at" sortState={sortState} onSortChange={(key, dir) => setSortState({ key, dir })} className="col-time" />
              </th>
              <th className="table-action-col"></th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <TableSkeleton columns={5} rows={6} />
            ) : filteredTenants.length === 0 ? (
              <TableEmptyStateRow
                colSpan={5}
                icon="⊞"
                title={search ? 'No tenants match this search' : 'No tenants yet'}
                description={search ? 'Try a different tenant name, ID, or clear the search.' : 'Create the first tenant to start registering agents, API keys, approvers, and policy settings.'}
                action={search ? <button className="btn btn-outline btn-sm" type="button" onClick={() => setSearch('')}>Clear search</button> : undefined}
              />
            ) : (
              filteredTenants.map(t => {
                const created = formatTimeWithTitle(t.created_at)
                return (
                <tr key={t.id}>
                  <td>
                    <div className="table-primary-cell">
                      <Link to={`/tenants/${t.id}`} className="table-primary table-primary-link">{t.name}</Link>
                      <div className="table-subtext mono" title={t.id}>{t.id}</div>
                    </div>
                  </td>
                  <td>
                    <div className="inline-value-copy">
                      <code className="mono" title={t.id}>{shortID(t.id, 12)}</code>
                      <CopyIconButton text={t.id} label="Tenant ID" />
                    </div>
                  </td>
                  <td>
                    <span className={`badge ${t.status === 'active' ? 'badge-green' : 'badge-red'}`}>
                      {t.status}
                    </span>
                  </td>
                  <td className="col-time" title={created.title}>{created.label}</td>
                  <td className="table-action-cell">
                    <Link to={`/tenants/${t.id}`} className="btn btn-outline btn-sm">Open tenant</Link>
                  </td>
                </tr>
              )})
            )}
          </tbody>
        </table>
      </TableFrame>
    </div>
  )
}
