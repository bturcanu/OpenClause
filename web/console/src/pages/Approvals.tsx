import { useState, useEffect, useCallback } from 'react'
import { api, formatDate } from '../api'

interface Approval {
  id: string
  tool: string
  action: string
  risk_score: number
  agent_id: string
  tenant_id: string
  status: string
  payload: any
  created_at: string
}

export default function Approvals() {
  const [approvals, setApprovals] = useState<Approval[]>([])
  const [selected, setSelected] = useState<Approval | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [actionLoading, setActionLoading] = useState<string | null>(null)

  const fetchApprovals = useCallback(async () => {
    try {
      setLoading(true)
      const data = await api.get('/admin/approvals/pending')
      setApprovals(Array.isArray(data) ? data : data?.approvals || [])
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { fetchApprovals() }, [fetchApprovals])

  async function handleAction(id: string, action: 'approve' | 'deny') {
    setActionLoading(id)
    try {
      await api.post(`/admin/approvals/${id}/${action}`)
      await fetchApprovals()
      if (selected?.id === id) setSelected(null)
    } catch (err: any) {
      setError(err.message)
    } finally {
      setActionLoading(null)
    }
  }

  return (
    <div>
      <div className="page-header">
        <h2>Approvals Queue</h2>
        <p>Review and action pending human-in-the-loop requests</p>
      </div>

      {error && <div className="error-msg">{error}</div>}

      <div className="table-container">
        <table>
          <thead>
            <tr>
              <th>ID</th>
              <th>Tool</th>
              <th>Action</th>
              <th>Risk Score</th>
              <th>Agent</th>
              <th>Created</th>
              <th>Status</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <tr><td colSpan={8} className="loading">Loading…</td></tr>
            ) : approvals.length === 0 ? (
              <tr><td colSpan={8} style={{ textAlign: 'center', padding: 32, color: '#94a3b8' }}>No pending approvals</td></tr>
            ) : (
              approvals.map(a => (
                <tr key={a.id}>
                  <td>
                    <button
                      onClick={() => setSelected(a)}
                      style={{ background: 'none', border: 'none', color: '#3b82f6', cursor: 'pointer', fontWeight: 600, fontSize: 13 }}
                    >
                      {a.id.slice(0, 8)}…
                    </button>
                  </td>
                  <td>{a.tool}</td>
                  <td>{a.action}</td>
                  <td>{a.risk_score}</td>
                  <td>{a.agent_id?.slice(0, 8) || '—'}</td>
                  <td>{formatDate(a.created_at)}</td>
                  <td><span className="badge badge-pending">{a.status || 'pending'}</span></td>
                  <td>
                    <div className="btn-group">
                      <button
                        className="btn btn-success btn-sm"
                        disabled={actionLoading === a.id}
                        onClick={() => handleAction(a.id, 'approve')}
                      >
                        Approve
                      </button>
                      <button
                        className="btn btn-danger btn-sm"
                        disabled={actionLoading === a.id}
                        onClick={() => handleAction(a.id, 'deny')}
                      >
                        Deny
                      </button>
                    </div>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {selected && (
        <div className="modal-backdrop" onClick={() => setSelected(null)}>
          <div className="modal" onClick={e => e.stopPropagation()}>
            <div className="flex-between mb-16">
              <h3>Approval Detail</h3>
              <button className="btn btn-outline btn-sm" onClick={() => setSelected(null)}>Close</button>
            </div>
            <div className="detail-row">
              <div className="detail-label">ID</div>
              <div className="detail-value">{selected.id}</div>
            </div>
            <div className="detail-row">
              <div className="detail-label">Tool</div>
              <div className="detail-value">{selected.tool}</div>
            </div>
            <div className="detail-row">
              <div className="detail-label">Action</div>
              <div className="detail-value">{selected.action}</div>
            </div>
            <div className="detail-row">
              <div className="detail-label">Risk Score</div>
              <div className="detail-value">{selected.risk_score}</div>
            </div>
            <div className="detail-row">
              <div className="detail-label">Agent</div>
              <div className="detail-value">{selected.agent_id || '—'}</div>
            </div>
            <div className="detail-row">
              <div className="detail-label">Tenant</div>
              <div className="detail-value">{selected.tenant_id}</div>
            </div>
            <div className="detail-row">
              <div className="detail-label">Created</div>
              <div className="detail-value">{formatDate(selected.created_at)}</div>
            </div>
            {selected.payload && (
              <div className="detail-row">
                <div className="detail-label">Payload</div>
                <div className="detail-value">
                  <pre>{JSON.stringify(selected.payload, null, 2)}</pre>
                </div>
              </div>
            )}
            <div className="btn-group mt-16">
              <button
                className="btn btn-success"
                disabled={actionLoading === selected.id}
                onClick={() => handleAction(selected.id, 'approve')}
              >
                Approve
              </button>
              <button
                className="btn btn-danger"
                disabled={actionLoading === selected.id}
                onClick={() => handleAction(selected.id, 'deny')}
              >
                Deny
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
