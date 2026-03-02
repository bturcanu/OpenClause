import { useState, useEffect } from 'react'
import { api } from '../api'

interface Connector {
  id: string
  name: string
  type: string
  actions: string[]
  description?: string
  enabled?: boolean
}

export default function Connectors() {
  const [connectors, setConnectors] = useState<Connector[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    api.get('/v1/connectors')
      .then(data => setConnectors(Array.isArray(data) ? data : data?.connectors || []))
      .catch(err => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  if (loading) return <div className="loading">Loading connectors…</div>
  if (error) return <div className="error-msg">{error}</div>

  return (
    <div>
      <div className="page-header">
        <h2>Connectors</h2>
        <p>Available tool connectors registered with OpenClause</p>
      </div>

      {connectors.length === 0 ? (
        <div className="empty-state">
          <div className="empty-icon">⧉</div>
          <p>No connectors registered</p>
        </div>
      ) : (
        <div className="connector-grid">
          {connectors.map(c => (
            <div key={c.id} className="connector-card">
              <div className="flex-between">
                <h4>{c.name}</h4>
                {c.enabled !== false
                  ? <span className="badge badge-green">Active</span>
                  : <span className="badge badge-gray">Disabled</span>}
              </div>
              <div className="cc-type">{c.type}</div>
              {c.description && (
                <p style={{ fontSize: 13, color: '#475569', marginBottom: 10 }}>{c.description}</p>
              )}
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
