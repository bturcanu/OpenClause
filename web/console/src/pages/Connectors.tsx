import { useState, useEffect } from 'react'
import { api } from '../api'

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

  useEffect(() => {
    api.get('/v1/connectors')
      .then(data => setConnectors(normalizeConnectors(data)))
      .catch(err => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  if (loading) return <div className="loading">Loading connectors…</div>

  return (
    <div>
      <div className="page-header">
        <h2>Connectors</h2>
        <p>Available tool connectors registered with OpenClause</p>
      </div>

      {error && <div className="error-msg">{error}</div>}

      {connectors.length === 0 ? (
        <div className="empty-state">
          <div className="empty-icon">⧉</div>
          <p>No connectors registered</p>
        </div>
      ) : (
        <div className="connector-grid">
          {connectors.map(c => (
            <div key={c.name} className="connector-card">
              <div className="flex-between">
                <h4>{c.name}</h4>
                <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
                  {c.type && <span className={`badge badge-${c.type === 'remote' ? 'blue' : 'gray'}`}>{c.type}</span>}
                  {c.event_count != null && <span className="badge badge-gray">{c.event_count} events</span>}
                </div>
              </div>
              <div className="cc-actions">
                {(c.actions || []).map(action => (
                  <span key={action} className="badge badge-blue">{action}</span>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
