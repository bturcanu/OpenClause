import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api, formatDate } from '../api'
import { EmptyState, InlineErrorState, PageHeaderBlock, TableSkeleton, decisionTone, formatRequester, buildQuery, noneText, shortID } from '../ui'

type Approval = {
  id: string
  event_id: string
  tool: string
  action: string
  risk_score: number
  agent_id: string
  tenant_id: string
  status: string
  reason?: string
  deny_reason?: string
  user_id?: string
  user_name?: string
  user_email?: string
  session_id?: string
  trace_id?: string
  created_at: string
  expires_at: string
}

export default function Approvals() {
  const [searchParams] = useSearchParams()
  const approvalID = searchParams.get('approval_id') || ''
  const [approvals, setApprovals] = useState<Approval[]>([])
  const [selected, setSelected] = useState<Approval | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [actionLoading, setActionLoading] = useState<string | null>(null)
  const [copyStatus, setCopyStatus] = useState('')
  const [executeApiKey, setExecuteApiKey] = useState('')
  const fetchSeq = useRef(0)

  const fetchApprovals = useCallback(async (silent = false) => {
    const seq = ++fetchSeq.current
    try {
      if (!silent) {
        setLoading(true)
        setError('')
      }
      const data = await api.get('/admin/approvals/pending')
      if (seq !== fetchSeq.current) return
      const nextApprovals = Array.isArray(data) ? data : data?.approvals || []
      setApprovals(nextApprovals)
      if (approvalID) {
        const match = nextApprovals.find((approval: Approval) => approval.id === approvalID)
        if (match) setSelected(match)
      }
    } catch (err: any) {
      if (!silent) setError(err.message)
    } finally {
      if (!silent && seq === fetchSeq.current) setLoading(false)
    }
  }, [approvalID])

  useEffect(() => {
    void fetchApprovals(false)
    const timer = window.setInterval(() => {
      void fetchApprovals(true)
    }, 5000)
    return () => window.clearInterval(timer)
  }, [fetchApprovals])

  useEffect(() => {
    setExecuteApiKey('')
    setCopyStatus('')
  }, [selected?.id])

  useEffect(() => {
    if (!selected) return
    if (selected.status === 'approved') return
    const stillPending = approvals.some(approval => approval.id === selected.id)
    if (!stillPending) setSelected(null)
  }, [approvals, selected])

  async function handleAction(id: string, action: 'approve' | 'deny') {
    setActionLoading(id)
    try {
      await api.post(`/admin/approvals/${id}/${action}`)
      await fetchApprovals(true)
      setSelected(current => {
        if (!current || current.id !== id) return current
        if (action === 'approve') return { ...current, status: 'approved' }
        return null
      })
    } catch (err: any) {
      setError(err.message)
    } finally {
      setActionLoading(null)
    }
  }

  const executeCommand = (() => {
    if (!selected?.event_id) return ''
    const apiKey = executeApiKey.trim()
    const header = apiKey ? `X-API-Key: ${apiKey}` : 'X-API-Key: <API_KEY>'
    return `curl -s -X POST "http://localhost:8080/v1/toolcalls/${encodeURIComponent(selected.event_id)}/execute" -H "Content-Type: application/json" -H "${header}" -d '{}'`
  })()

  return (
    <div>
      <PageHeaderBlock
        title="Approvals"
        description="Review high-risk requests, understand who triggered them, and send operators straight to the underlying evidence."
        actions={
          <button className="btn btn-primary" type="button" disabled={loading} onClick={() => void fetchApprovals(false)}>
            Refresh queue
          </button>
        }
      />

      {error ? <InlineErrorState message={error} onRetry={() => void fetchApprovals(false)} /> : null}

      <div className="table-container table-sticky">
        <table>
          <thead>
            <tr>
              <th>Approval</th>
              <th>Requested by</th>
              <th>Tool</th>
              <th>Tenant</th>
              <th>Created</th>
              <th>Status</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <TableSkeleton columns={7} rows={5} />
            ) : approvals.length === 0 ? (
              <tr>
                <td colSpan={7}>
                  <EmptyState
                    icon="✓"
                    title="No approvals are waiting right now"
                    description="New high-risk or destructive requests will appear here as soon as policy routes them for review."
                  />
                </td>
              </tr>
            ) : (
              approvals.map(approval => (
                <tr key={approval.id}>
                  <td>
                    <button className="link-button" onClick={() => setSelected(approval)} type="button">
                      {shortID(approval.id)}
                    </button>
                    <div className="table-subtext">Risk {approval.risk_score}</div>
                  </td>
                  <td>
                    <div className="table-primary">{formatRequester(approval.user_id, approval.user_name, approval.user_email, approval.agent_id)}</div>
                    <div className="table-subtext">
                      Session {approval.session_id ? shortID(approval.session_id) : '(none)'}
                      {' · '}Trace {approval.trace_id ? shortID(approval.trace_id) : '(none)'}
                    </div>
                  </td>
                  <td>
                    <div className="table-primary">{approval.tool}.{approval.action}</div>
                    <div className="table-subtext">{approval.reason || 'Waiting for human review'}</div>
                  </td>
                  <td>
                    <code>{shortID(approval.tenant_id, 12)}</code>
                  </td>
                  <td>{formatDate(approval.created_at)}</td>
                  <td>
                    <span className={`badge badge-${decisionTone(approval.status)}`}>{approval.status || 'pending'}</span>
                  </td>
                  <td>
                    <div className="btn-group">
                      <button className="btn btn-success btn-sm" disabled={actionLoading === approval.id} onClick={() => handleAction(approval.id, 'approve')}>
                        Approve
                      </button>
                      <button className="btn btn-danger btn-sm" disabled={actionLoading === approval.id} onClick={() => handleAction(approval.id, 'deny')}>
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

      {selected ? (
        <div className="modal-backdrop" onClick={() => setSelected(null)}>
          <div className="modal" onClick={e => e.stopPropagation()}>
            <div className="flex-between mb-16">
              <div>
                <h3>Approval detail</h3>
                <p className="table-subtext">{selected.tool}.{selected.action}</p>
              </div>
              <button className="btn btn-outline btn-sm" type="button" onClick={() => setSelected(null)}>
                Close
              </button>
            </div>

            <div className="detail-row">
              <div className="detail-label">Requested by</div>
              <div className="detail-value">{formatRequester(selected.user_id, selected.user_name, selected.user_email, selected.agent_id)}</div>
            </div>
            <div className="detail-row">
              <div className="detail-label">Approval reason</div>
              <div className="detail-value">{selected.reason || 'Not recorded'}</div>
            </div>
            <div className="detail-row">
              <div className="detail-label">Status</div>
              <div className="detail-value">
                <span className={`badge badge-${decisionTone(selected.status)}`}>{selected.status}</span>
              </div>
            </div>
            <div className="detail-row">
              <div className="detail-label">Event</div>
              <div className="detail-value mono">
                <Link to={`/events/${selected.event_id}`}>{selected.event_id}</Link>
              </div>
            </div>
            <div className="detail-row">
              <div className="detail-label">Session</div>
              <div className="detail-value">
                {selected.session_id ? (
                  <Link to={`/sessions/${encodeURIComponent(selected.session_id)}${buildQuery({ tenant_id: selected.tenant_id })}`}>
                    {selected.session_id}
                  </Link>
                ) : (
                  '(none)'
                )}
              </div>
            </div>
            <div className="detail-row">
              <div className="detail-label">Trace</div>
              <div className="detail-value">{noneText(selected.trace_id)}</div>
            </div>
            <div className="detail-row">
              <div className="detail-label">Created</div>
              <div className="detail-value">{formatDate(selected.created_at)}</div>
            </div>
            <div className="detail-row">
              <div className="detail-label">Expires</div>
              <div className="detail-value">{formatDate(selected.expires_at)}</div>
            </div>

            <div className="btn-group mt-16">
              <button className="btn btn-success" disabled={actionLoading === selected.id} onClick={() => handleAction(selected.id, 'approve')}>
                Approve
              </button>
              <button className="btn btn-danger" disabled={actionLoading === selected.id} onClick={() => handleAction(selected.id, 'deny')}>
                Deny
              </button>
            </div>

            {selected.status !== 'approved' ? (
              <div className="banner-note mt-16">
                Approve this request first to unlock the execution helper for the agent.
              </div>
            ) : (
              <div className="detail-panel mt-16">
                <h3>Execute approved request</h3>
                <p className="table-subtext mb-16">
                  The agent can resume the run with <code>POST /v1/toolcalls/{selected.event_id}/execute</code>.
                </p>
                <div className="form-group">
                  <label>Gateway API key</label>
                  <input value={executeApiKey} onChange={e => setExecuteApiKey(e.target.value)} placeholder="sk-oc-..." style={{ fontFamily: 'monospace' }} />
                </div>
                <textarea readOnly value={executeCommand} className="code-textarea" />
                <div className="btn-group mt-16">
                  <button
                    className="btn btn-outline btn-sm"
                    type="button"
                    disabled={!executeApiKey.trim()}
                    onClick={async () => {
                      try {
                        await navigator.clipboard.writeText(executeCommand)
                        setCopyStatus('Copied execute command')
                        setTimeout(() => setCopyStatus(''), 1500)
                      } catch {
                        setCopyStatus('Copy failed')
                        setTimeout(() => setCopyStatus(''), 1500)
                      }
                    }}
                  >
                    Copy execute command
                  </button>
                  {copyStatus ? <div className="success-inline">{copyStatus}</div> : null}
                </div>
              </div>
            )}
          </div>
        </div>
      ) : null}
    </div>
  )
}
