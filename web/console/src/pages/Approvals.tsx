import { useState, useEffect, useCallback, useRef } from 'react'
import { api, formatDate } from '../api'

interface Approval {
  id: string
  event_id: string
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
  const fetchSeq = useRef(0)
  const [executeApiKey, setExecuteApiKey] = useState('')
  const [copyStatus, setCopyStatus] = useState('')

  const executeCommand = (() => {
    if (!selected?.event_id) return ''
    const apiKey = executeApiKey.trim()
    const gatewayBase = 'http://localhost:8080'
    const url = `${gatewayBase}/v1/toolcalls/${encodeURIComponent(selected.event_id)}/execute`
    const header = apiKey ? `X-API-Key: ${apiKey}` : 'X-API-Key: <API_KEY>'
    return `curl -s -X POST "${url}" -H "Content-Type: application/json" -H "${header}" -d '{}'`
  })()

  const fetchApprovals = useCallback(async (silent = false) => {
    const seq = ++fetchSeq.current
    try {
      if (!silent) {
        setLoading(true)
        setError('')
      }
      const data = await api.get('/admin/approvals/pending')
      if (seq !== fetchSeq.current) return
      setApprovals(Array.isArray(data) ? data : data?.approvals || [])
    } catch (err: any) {
      if (!silent) setError(err.message)
    } finally {
      if (!silent) {
        if (seq === fetchSeq.current) setLoading(false)
      }
    }
  }, [])

  useEffect(() => {
    void fetchApprovals(false)
    const timer = window.setInterval(() => {
      void fetchApprovals(true)
    }, 5000)
    return () => window.clearInterval(timer)
  }, [fetchApprovals])

  // Prevent stale execution helper state when switching between approvals.
  useEffect(() => {
    setExecuteApiKey('')
    setCopyStatus('')
  }, [selected?.id])

  // If the selected pending approval disappears from the polled list (resolved/expired),
  // close the modal to avoid a stale UI.
  useEffect(() => {
    if (!selected) return
    if (selected.status === 'approved') return
    const stillPending = approvals.some(a => a.id === selected.id)
    if (!stillPending) setSelected(null)
  }, [approvals, selected?.id, selected?.status])

  async function handleAction(id: string, action: 'approve' | 'deny') {
    setActionLoading(id)
    try {
      await api.post(`/admin/approvals/${id}/${action}`)
      await fetchApprovals(true)
      setSelected(prev => {
        if (!prev || prev.id !== id) return prev
        if (action === 'approve') return { ...prev, status: 'approved' }
        return null
      })
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

      <div className="btn-group mb-16">
        <button className="btn btn-outline" disabled={loading} onClick={() => void fetchApprovals(false)}>
          Refresh
        </button>
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
              <div className="detail-label">Event</div>
              <div className="detail-value" style={{ fontFamily: 'monospace', fontSize: 12 }}>
                {selected.event_id}
              </div>
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

            {selected.status !== 'approved' ? (
              <div style={{ marginTop: 16, color: '#64748b', fontSize: 12 }}>
                Approve this request first to unlock the execution helper.
              </div>
            ) : (
              <div style={{ marginTop: 16 }}>
                <div className="detail-row" style={{ display: 'block' }}>
                  <div style={{ fontSize: 13, fontWeight: 700, marginBottom: 6 }}>Execute approved request</div>
                  <div style={{ color: '#64748b', fontSize: 12, marginBottom: 10 }}>
                    This sends{' '}
                    <code>{`POST /v1/toolcalls/${selected.event_id}/execute`}</code> to the gateway. You need a tenant-scoped API
                    key.
                  </div>
                  <div className="form-group" style={{ marginTop: 8, marginBottom: 8 }}>
                    <label>Gateway API Key</label>
                    <input
                      value={executeApiKey}
                      onChange={e => setExecuteApiKey(e.target.value)}
                      placeholder="sk-oc-..."
                      style={{ fontFamily: 'monospace' }}
                    />
                  </div>
                  <textarea
                    readOnly
                    value={executeCommand}
                    style={{
                      width: '100%',
                      minHeight: 64,
                      fontFamily: 'monospace',
                      fontSize: 12,
                      padding: 10,
                      borderRadius: 6,
                      border: '1px solid #e2e8f0',
                    }}
                  />
                  <div className="btn-group" style={{ marginTop: 10 }}>
                    <button
                      className="btn btn-outline btn-sm"
                      type="button"
                      disabled={!executeApiKey.trim()}
                      onClick={async () => {
                        try {
                          await navigator.clipboard.writeText(executeCommand)
                          setCopyStatus('Copied')
                          setTimeout(() => setCopyStatus(''), 1500)
                        } catch {
                          setCopyStatus('Copy failed')
                          setTimeout(() => setCopyStatus(''), 1500)
                        }
                      }}
                    >
                      Copy execute command
                    </button>
                    {copyStatus ? (
                      <div style={{ alignSelf: 'center', color: '#16a34a', fontSize: 12, fontWeight: 700 }}>
                        {copyStatus}
                      </div>
                    ) : null}
                  </div>
                </div>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
