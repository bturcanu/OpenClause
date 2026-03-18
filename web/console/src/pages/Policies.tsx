import { useState, useEffect, FormEvent } from 'react'
import { api, formatDate } from '../api'

interface PolicyVersion {
  id: number
  version: string
  deployed_by: string
  deployed_at: string
  notes: string
}

interface SimulationResult {
  decision: string
  reason: string
  [key: string]: any
}

export default function Policies() {
  const [versions, setVersions] = useState<PolicyVersion[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [showCreate, setShowCreate] = useState(false)
  const [createForm, setCreateForm] = useState({ rego_source: '' })
  const [creating, setCreating] = useState(false)

  const [simForm, setSimForm] = useState({ tenant_id: '', tool: '', action: '', risk_score: 50 })
  const [simResult, setSimResult] = useState<SimulationResult | null>(null)
  const [simLoading, setSimLoading] = useState(false)

  async function fetchVersions() {
    try {
      const data = await api.get('/admin/policy/versions')
      setVersions(Array.isArray(data) ? data : data?.versions || [])
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { fetchVersions() }, [])

  async function handleCreate(e: FormEvent) {
    e.preventDefault()
    setCreating(true)
    setError('')
    try {
      await api.post('/admin/policy/versions', createForm)
      setCreateForm({ rego_source: '' })
      setShowCreate(false)
      await fetchVersions()
    } catch (err: any) {
      setError(err.message)
    } finally {
      setCreating(false)
    }
  }

  async function handleSimulate(e: FormEvent) {
    e.preventDefault()
    setSimLoading(true)
    setSimResult(null)
    try {
      const result = await api.post('/admin/policy/simulate', {
        ...simForm,
        risk_score: Number(simForm.risk_score),
      })
      setSimResult(result)
    } catch (err: any) {
      setError(err.message)
    } finally {
      setSimLoading(false)
    }
  }

  return (
    <div>
      <div className="flex-between">
        <div className="page-header">
          <h2>Policies</h2>
          <p>OPA/Rego policy version management and simulation</p>
        </div>
        <button className="btn btn-primary" onClick={() => setShowCreate(f => !f)}>
          {showCreate ? 'Cancel' : '+ New Version'}
        </button>
      </div>

      {error && <div className="error-msg">{error}</div>}

      {showCreate && (
        <div className="form-card">
          <h3>Create Policy Version</h3>
          <form onSubmit={handleCreate}>
            <div className="form-group">
              <label>Rego Source</label>
              <textarea
                rows={10}
                value={createForm.rego_source}
                onChange={e => setCreateForm({ rego_source: e.target.value })}
                placeholder={`package openclause\n\ndefault decision = "allow"`}
                required
              />
            </div>
            <button className="btn btn-primary" disabled={creating}>
              {creating ? 'Creating…' : 'Publish Version'}
            </button>
          </form>
        </div>
      )}

      <div className="table-container">
        <table>
          <thead>
            <tr>
              <th>Version</th>
              <th>ID</th>
              <th>Deployed By</th>
              <th>Deployed At</th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <tr><td colSpan={4} className="loading">Loading…</td></tr>
            ) : versions.length === 0 ? (
              <tr><td colSpan={4} style={{ textAlign: 'center', padding: 32, color: '#94a3b8' }}>No policy versions</td></tr>
            ) : (
              versions.map(v => (
                <tr key={v.id}>
                  <td><span className="badge badge-blue">v{v.version}</span></td>
                  <td style={{ fontFamily: 'monospace', fontSize: 12 }}>{String(v.id)}</td>
                  <td>{v.deployed_by || '—'}</td>
                  <td>{formatDate(v.deployed_at)}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      <div className="form-card mt-16">
        <h3>Policy Simulator</h3>
        <p style={{ fontSize: 13, color: '#64748b', marginBottom: 16 }}>
          Test how the current policy evaluates a hypothetical request.
        </p>
        <form onSubmit={handleSimulate}>
          <div className="form-inline" style={{ marginBottom: 16 }}>
            <div className="form-group">
              <label>Tenant ID</label>
              <input
                value={simForm.tenant_id}
                onChange={e => setSimForm(f => ({ ...f, tenant_id: e.target.value }))}
                required
              />
            </div>
            <div className="form-group">
              <label>Tool</label>
              <input
                value={simForm.tool}
                onChange={e => setSimForm(f => ({ ...f, tool: e.target.value }))}
                required
              />
            </div>
            <div className="form-group">
              <label>Action</label>
              <input
                value={simForm.action}
                onChange={e => setSimForm(f => ({ ...f, action: e.target.value }))}
                required
              />
            </div>
            <div className="form-group">
              <label>Risk Score</label>
              <input
                type="number"
                min={0}
                max={100}
                value={simForm.risk_score}
                onChange={e => setSimForm(f => ({ ...f, risk_score: Number(e.target.value) }))}
              />
            </div>
            <button className="btn btn-primary" disabled={simLoading}>
              {simLoading ? 'Simulating…' : 'Simulate'}
            </button>
          </div>
        </form>

        {simResult && (
          <div style={{ marginTop: 12 }}>
            <div style={{ marginBottom: 8 }}>
              <span className={`badge badge-${simResult.decision}`}>{simResult.decision}</span>
              {simResult.reason && <span style={{ marginLeft: 8, fontSize: 13, color: '#475569' }}>{simResult.reason}</span>}
            </div>
            <pre style={{ background: '#f1f5f9', padding: 12, borderRadius: 6, fontSize: 12, overflow: 'auto' }}>
              {JSON.stringify(simResult, null, 2)}
            </pre>
          </div>
        )}
      </div>
    </div>
  )
}
