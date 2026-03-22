import { useEffect, useMemo, useState } from 'react'
import { api } from '../api'
import { EmptyState, InlineErrorState, PageHeaderBlock } from '../ui'

interface Connector {
  name: string
  type?: string
  actions: string[]
  event_count?: number
}

function normalizeConnectors(data: any): Connector[] {
  const arr: any[] = Array.isArray(data) ? data : data?.connectors || []
  return arr.map(c => ({
    name: c.name || c.tool || 'unknown',
    type: c.type,
    actions: c.actions || [],
    event_count: c.event_count,
  }))
}

export default function Connectors() {
  const [connectors, setConnectors] = useState<Connector[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [search, setSearch] = useState('')
  const [expanded, setExpanded] = useState<Record<string, boolean>>({})

  async function fetchConnectors() {
    setLoading(true)
    setError('')
    try {
      const data = await api.get('/admin/connectors')
      setConnectors(normalizeConnectors(data))
    } catch (err: any) {
      setConnectors([])
      setError(err?.message || 'Failed to load connector registry')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void fetchConnectors()
  }, [])

  const filteredConnectors = useMemo(() => {
    const query = search.trim().toLowerCase()
    if (!query) return connectors
    return connectors.filter(connector =>
      connector.name.toLowerCase().includes(query) ||
      (connector.type || '').toLowerCase().includes(query) ||
      (connector.actions || []).some(action => action.toLowerCase().includes(query))
    )
  }, [connectors, search])

  return (
    <div>
      <PageHeaderBlock
        title="Connectors"
        description="Browse the live connector catalog the gateway can execute today, including supported actions for demo planning and operator troubleshooting."
        actions={
          <div className="form-group connectors-search">
            <label>Search</label>
            <input value={search} onChange={e => setSearch(e.target.value)} placeholder="Find a connector or action" />
          </div>
        }
      />

      {error ? <InlineErrorState message={error} onRetry={() => void fetchConnectors()} /> : null}

      <div className="connector-overview-grid">
        <div className="detail-panel connector-help-panel">
          <h3>How to use this page</h3>
          <p className="table-subtext">
            Remote connectors call external services. Built-in connectors run in-process inside the gateway. Use this catalog to confirm action names before running a demo or troubleshooting a policy decision.
          </p>
          <div className="stacked-badges mt-16">
            <span className="badge connector-kind-remote">{connectors.filter(c => c.type === 'remote').length} remote</span>
            <span className="badge connector-kind-builtin">{connectors.filter(c => c.type !== 'remote').length} built-in</span>
            <span className="badge badge-green">{connectors.reduce((total, connector) => total + (connector.actions?.length || 0), 0)} actions</span>
          </div>
        </div>
      </div>

      {loading ? (
        <div className="loading">Loading connectors…</div>
      ) : error && connectors.length === 0 ? null : filteredConnectors.length === 0 ? (
        <EmptyState
          icon="⧉"
          title={connectors.length === 0 ? 'No connectors registered' : 'No connectors match this search'}
          description={connectors.length === 0 ? 'The connector registry is empty for this environment.' : 'Try a connector name like slack or an action like msg.post.'}
          action={search ? <button className="btn btn-outline btn-sm" type="button" onClick={() => setSearch('')}>Clear search</button> : undefined}
        />
      ) : (
        <div className="connector-grid">
          {filteredConnectors.map(c => {
            const allActions = c.actions || []
            const isExpanded = !!expanded[c.name]
            const visibleActions = isExpanded ? allActions : allActions.slice(0, 6)
            const hiddenCount = Math.max(0, allActions.length - visibleActions.length)
            return (
            <div key={c.name} className="connector-card">
              <div className="flex-between">
                <h4>{c.name}</h4>
                <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
                  {c.type && <span className={`badge ${c.type === 'remote' ? 'connector-type-remote' : 'connector-type-builtin'}`}>{c.type}</span>}
                  {c.event_count != null && <span className="badge badge-gray">{c.event_count} events</span>}
                </div>
              </div>
              <div className="cc-actions">
                {visibleActions.map(action => (
                  <span key={action} className="badge connector-action-badge">{action}</span>
                ))}
                {hiddenCount > 0 ? (
                  <button
                    className="link-button connector-expand-button"
                    type="button"
                    onClick={() => setExpanded(current => ({ ...current, [c.name]: true }))}
                  >
                    +{hiddenCount} more
                  </button>
                ) : null}
                {isExpanded && allActions.length > 6 ? (
                  <button
                    className="link-button connector-expand-button"
                    type="button"
                    onClick={() => setExpanded(current => ({ ...current, [c.name]: false }))}
                  >
                    Show less
                  </button>
                ) : null}
              </div>
            </div>
          )})}
        </div>
      )}
    </div>
  )
}
