import { useState, useEffect } from 'react'
import { api } from '../api'

interface Connector {
  tool: string
  actions: string[]
  event_count: number
}

export default function Connectors() {
  const [connectors, setConnectors] = useState<Connector[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    api.get('/admin/connectors')
      .then(data => setConnectors(Array.isArray(data) ? data : data?.connectors || []))
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
            <div key={c.tool} className="connector-card">
              <div className="flex-between">
                <h4>{c.tool}</h4>
                <span className="badge badge-gray">{c.event_count} events</span>
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
