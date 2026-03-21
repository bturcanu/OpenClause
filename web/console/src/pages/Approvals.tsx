import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api, formatDate } from '../api'
import { EmptyState, InlineErrorState, PageHeaderBlock, TableSkeleton, buildQuery, copyText, decisionTone, formatRequester, noneText, shortID } from '../ui'

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
      setError('')
      if (approvalID) {
        const match = nextApprovals.find((approval: Approval) => approval.id === approvalID)
        if (match) setSelected(match)
      }
    } catch (err: any) {
      if (seq === fetchSeq.current) setError(err?.message || (silent ? 'Approval queue refresh failed.' : 'Failed to load approvals'))
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

  async function handleCopy(label: string, value?: string | null) {
    const text = (value || '').trim()
    if (!text) return
    try {
      await copyText(text)
      setCopyStatus(`${label} copied`)
      window.setTimeout(() => setCopyStatus(''), 1500)
    } catch {
      setCopyStatus('Copy failed')
      window.setTimeout(() => setCopyStatus(''), 1500)
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
                    <button className="link-button table-primary" onClick={() => setSelected(approval)} type="button">
                      {shortID(approval.id)}
                    </button>
                    <div className="table-subtext">Risk {approval.risk_score} · Event {shortID(approval.event_id)}</div>
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
                      <Link to={`/events/${approval.event_id}`} className="btn btn-outline btn-sm">
                        Event
                      </Link>
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
              <div className="btn-group">
                <Link to={`/events/${selected.event_id}`} className="btn btn-outline btn-sm">
                  Open event
                </Link>
                <button className="btn btn-outline btn-sm" type="button" onClick={() => setSelected(null)}>
                  Close
                </button>
              </div>
            </div>

            <div className="detail-panel">
              <h3>Approval context</h3>
              <div className="identity-grid">
                <div className="identity-card">
                  <span className="meta-label">Requested by</span>
                  <div className="identity-primary">{formatRequester(selected.user_id, selected.user_name, selected.user_email, selected.agent_id)}</div>
                  <div className="identity-secondary">High-risk actions land here when policy requires a human decision before execution.</div>
                </div>
                <div className="identity-card">
                  <span className="meta-label">Approval ID</span>
                  <div className="identity-copy-row">
                    <code className="mono">{selected.id}</code>
                    <button className="btn btn-outline btn-sm" type="button" onClick={() => void handleCopy('Approval ID', selected.id)}>
                      Copy
                    </button>
                  </div>
                </div>
                <div className="identity-card">
                  <span className="meta-label">Event ID</span>
                  <div className="identity-copy-row">
                    <Link to={`/events/${selected.event_id}`} className="mono">{selected.event_id}</Link>
                    <button className="btn btn-outline btn-sm" type="button" onClick={() => void handleCopy('Event ID', selected.event_id)}>
                      Copy
                    </button>
                  </div>
                </div>
                <div className="identity-card">
                  <span className="meta-label">Session</span>
                  <div className="identity-copy-row">
                    {selected.session_id ? (
                      <Link to={`/sessions/${encodeURIComponent(selected.session_id)}${buildQuery({ tenant_id: selected.tenant_id })}`} className="mono">
                        {selected.session_id}
                      </Link>
                    ) : (
                      <code className="mono">(none)</code>
                    )}
                    <button className="btn btn-outline btn-sm" type="button" onClick={() => void handleCopy('Session ID', selected.session_id)} disabled={!selected.session_id}>
                      Copy
                    </button>
                  </div>
                </div>
                <div className="identity-card">
                  <span className="meta-label">Trace</span>
                  <div className="identity-copy-row">
                    <code className="mono">{noneText(selected.trace_id)}</code>
                    <button className="btn btn-outline btn-sm" type="button" onClick={() => void handleCopy('Trace ID', selected.trace_id)} disabled={!selected.trace_id}>
                      Copy
                    </button>
                  </div>
                </div>
                <div className="identity-card">
                  <span className="meta-label">Status</span>
                  <div className="identity-primary">
                    <span className={`badge badge-${decisionTone(selected.status)}`}>{selected.status}</span>
                  </div>
                  <div className="identity-secondary">Created {formatDate(selected.created_at)} · Expires {formatDate(selected.expires_at)}</div>
                </div>
              </div>
            </div>

            <div className="session-callout">
              <strong>Why this request needs review</strong>
              <p>{selected.reason || 'A specific approval reason was not recorded, but policy flagged this action for human review.'}</p>
              {selected.deny_reason ? <p className="table-subtext">Last deny reason: {selected.deny_reason}</p> : null}
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
                <pre className="code-block">{executeCommand}</pre>
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
