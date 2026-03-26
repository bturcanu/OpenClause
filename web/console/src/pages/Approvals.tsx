import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api, formatDate } from '../api'
import {
  ActiveFiltersBar,
  CopyIconButton,
  InlineErrorState,
  PageHeaderBlock,
  SortHeader,
  TableEmptyStateRow,
  TableFrame,
  TableSkeleton,
  applySort,
  buildQuery,
  compareDate,
  compareNumber,
  compareText,
  decisionTone,
  formatRequester,
  formatTimeWithTitle,
  noneText,
  shortID,
  type SortState,
} from '../ui'

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
  const [searchParams, setSearchParams] = useSearchParams()
  const approvalID = searchParams.get('approval_id') || ''
  const tenantFilter = searchParams.get('tenant_id') || ''
  const [approvals, setApprovals] = useState<Approval[]>([])
  const [selected, setSelected] = useState<Approval | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [actionLoading, setActionLoading] = useState<string | null>(null)
  const [copyStatus, setCopyStatus] = useState('')
  const [executeApiKey, setExecuteApiKey] = useState('')
  const fetchSeq = useRef(0)
  const [sortState, setSortState] = useState<SortState<'risk_score' | 'created_at' | 'expires_at' | 'status'>>({
    key: null,
    dir: 'desc',
  })

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
        setSelected(match || null)
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
    return `OPENCLAUSE_BASE_URL="\${OPENCLAUSE_BASE_URL:-http://localhost:8080}" curl -fsS -X POST "$OPENCLAUSE_BASE_URL/v1/toolcalls/${encodeURIComponent(selected.event_id)}/execute" -H "Content-Type: application/json" -H "${header}" -d '{}'`
  })()

  const visibleApprovals = useMemo(() => {
    const filtered = tenantFilter
      ? approvals.filter(approval => approval.tenant_id === tenantFilter)
      : approvals
    if (!sortState.key) return filtered
    return [...filtered].sort((left, right) => {
      switch (sortState.key) {
        case 'risk_score':
          return applySort(compareNumber(left.risk_score, right.risk_score), sortState.dir)
        case 'created_at':
          return applySort(compareDate(left.created_at, right.created_at), sortState.dir)
        case 'expires_at':
          return applySort(compareDate(left.expires_at, right.expires_at), sortState.dir)
        case 'status':
          return applySort(compareText(left.status, right.status), sortState.dir)
        default:
          return 0
      }
    })
  }, [approvals, sortState, tenantFilter])

  useEffect(() => {
    if (!selected) return
    if (selected.status === 'approved') {
      if (tenantFilter && selected.tenant_id !== tenantFilter) setSelected(null)
      return
    }
    const stillVisible = visibleApprovals.some(approval => approval.id === selected.id)
    if (!stillVisible) setSelected(null)
  }, [selected, tenantFilter, visibleApprovals])

  function clearTenantFilter() {
    const next = new URLSearchParams(searchParams)
    next.delete('tenant_id')
    setSearchParams(next)
  }

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

      <ActiveFiltersBar
        resultCount={visibleApprovals.length}
        resultLabel={visibleApprovals.length === 1 ? 'request' : 'requests'}
        chips={tenantFilter ? [{
          key: 'tenant_id',
          label: `tenant id: ${tenantFilter}`,
          onRemove: clearTenantFilter,
        }] : []}
        note={sortState.key ? 'Sorted within the current page.' : 'Using backend order until you sort this page.'}
      />

      <TableFrame stickyHeader>
        <table>
          <thead>
            <tr>
              <th>Approval</th>
              <th>Requested by</th>
              <th className="col-num">
                <SortHeader label="Risk" sortKey="risk_score" sortState={sortState} onSortChange={(key, dir) => setSortState({ key, dir })} defaultDir="desc" className="col-num" />
              </th>
              <th className="col-time">
                <SortHeader label="Age" sortKey="created_at" sortState={sortState} onSortChange={(key, dir) => setSortState({ key, dir })} defaultDir="desc" className="col-time" />
              </th>
              <th className="col-time">
                <SortHeader label="Expires" sortKey="expires_at" sortState={sortState} onSortChange={(key, dir) => setSortState({ key, dir })} defaultDir="asc" className="col-time" />
              </th>
              <th>
                <SortHeader label="Status" sortKey="status" sortState={sortState} onSortChange={(key, dir) => setSortState({ key, dir })} />
              </th>
              <th className="table-action-col"></th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <TableSkeleton columns={7} rows={5} />
            ) : visibleApprovals.length === 0 ? (
              <TableEmptyStateRow
                colSpan={7}
                icon="✓"
                title="No approvals are waiting right now"
                description="New high-risk or destructive requests will appear here as soon as policy routes them for review."
              />
            ) : (
              visibleApprovals.map(approval => {
                const created = formatTimeWithTitle(approval.created_at)
                const expires = formatTimeWithTitle(approval.expires_at)
                const expiresAt = new Date(approval.expires_at).getTime()
                const now = Date.now()
                const isExpired = !Number.isNaN(expiresAt) && expiresAt <= now
                const expiresSoon = !isExpired && !Number.isNaN(expiresAt) && expiresAt - now <= 60 * 60 * 1000
                return (
                <tr key={approval.id}>
                  <td>
                    <div className="table-primary-cell">
                      <div className="inline-value-copy">
                        <button className="link-button table-primary" onClick={() => setSelected(approval)} type="button" title={approval.id}>
                          {approval.tool}.{approval.action}
                        </button>
                        <CopyIconButton text={approval.id} label="Approval ID" />
                      </div>
                      <div className="table-subtext">
                        Approval <span className="mono" title={approval.id}>{shortID(approval.id)}</span>
                        {' · '}
                        <span className="inline-value-copy">
                          <span className="mono" title={approval.event_id}>Event {shortID(approval.event_id)}</span>
                          <CopyIconButton text={approval.event_id} label="Event ID" />
                        </span>
                      </div>
                      <div className="inline-value-copy">
                        <code className="mono" title={approval.tenant_id}>{shortID(approval.tenant_id, 12)}</code>
                        <CopyIconButton text={approval.tenant_id} label="Tenant ID" />
                      </div>
                    </div>
                  </td>
                  <td>
                    <div className="table-primary-cell">
                      <div className="table-primary">{formatRequester(approval.user_id, approval.user_name, approval.user_email, approval.agent_id)}</div>
                      <div className="table-subtext">
                      <span title={approval.session_id || '(none)'}>Session {approval.session_id ? shortID(approval.session_id) : '(none)'}</span>
                      {' · '}
                      <span title={approval.trace_id || '(none)'}>Trace {approval.trace_id ? shortID(approval.trace_id) : '(none)'}</span>
                      </div>
                    </div>
                  </td>
                  <td className="col-num tabular">{approval.risk_score}</td>
                  <td className="col-time" title={created.title}>
                    <div className="table-primary-cell">
                      <span>{created.label}</span>
                      <span className="table-subtext">{approval.reason || 'Waiting for human review'}</span>
                    </div>
                  </td>
                  <td className="col-time" title={expires.title}>
                    <div className="table-primary-cell">
                      <span className={isExpired ? 'table-danger-text' : expiresSoon ? 'table-warn-text' : ''}>{expires.label}</span>
                      <span className="table-subtext">
                        {isExpired ? 'Expired' : expiresSoon ? 'Expires soon' : formatDate(approval.expires_at)}
                      </span>
                    </div>
                  </td>
                  <td>
                    <span className={`badge badge-${decisionTone(approval.status)}`}>{approval.status || 'pending'}</span>
                  </td>
                  <td className="table-action-cell">
                    <div className="table-primary-cell table-action-stack">
                      <button className="btn btn-outline btn-sm" type="button" onClick={() => setSelected(approval)}>
                        Review
                      </button>
                      <Link to={`/events/${approval.event_id}`} className="btn btn-outline btn-sm">
                        Open event
                      </Link>
                    </div>
                  </td>
                </tr>
              )})
            )}
          </tbody>
        </table>
      </TableFrame>

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
                    <code className="mono" title={selected.id}>{selected.id}</code>
                    <CopyIconButton text={selected.id} label="Approval ID" />
                  </div>
                </div>
                <div className="identity-card">
                  <span className="meta-label">Event ID</span>
                  <div className="identity-copy-row">
                    <Link to={`/events/${selected.event_id}`} className="mono" title={selected.event_id}>{selected.event_id}</Link>
                    <CopyIconButton text={selected.event_id} label="Event ID" />
                  </div>
                </div>
                <div className="identity-card">
                  <span className="meta-label">Session</span>
                  <div className="identity-copy-row">
                    {selected.session_id ? (
                      <Link to={`/sessions/${encodeURIComponent(selected.session_id)}${buildQuery({ tenant_id: selected.tenant_id })}`} className="mono" title={selected.session_id}>
                        {selected.session_id}
                      </Link>
                    ) : (
                      <code className="mono">(none)</code>
                    )}
                    <CopyIconButton text={selected.session_id} label="Session ID" disabled={!selected.session_id} />
                  </div>
                </div>
                <div className="identity-card">
                  <span className="meta-label">Trace</span>
                  <div className="identity-copy-row">
                    <code className="mono" title={noneText(selected.trace_id)}>{noneText(selected.trace_id)}</code>
                    <CopyIconButton text={selected.trace_id} label="Trace ID" disabled={!selected.trace_id} />
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
                  <label htmlFor="approvals-execute-api-key">Gateway API key</label>
                  <input id="approvals-execute-api-key" value={executeApiKey} onChange={e => setExecuteApiKey(e.target.value)} placeholder="sk-oc-..." className="mono" />
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
