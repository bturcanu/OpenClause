import { useEffect, useState, type FormEvent } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, formatDate } from '../api'
import { EmptyState, InlineErrorState, PageHeaderBlock, TableSkeleton } from '../ui'

interface Tenant {
  id: string
  name: string
  status: string
}

interface TenantPolicyConfig {
  max_risk_auto_approve: number
  read_actions: string[]
  write_actions: string[]
  destructive_actions: string[]
  require_destructive_approval: boolean
}

interface PolicyVersion {
  id: number
  version: string
  deployed_by: string
  deployed_at: string
  notes: string
  policy_data?: TenantPolicyConfig
}

interface SimResultDecision {
  decision?: string
  reason?: string
}

interface SimulationResponse {
  simulation: boolean
  tenant_id: string
  policy_result?: {
    result?: SimResultDecision
  }
}

function parseActions(raw: string): string[] {
  return raw
    .split(',')
    .map(v => v.trim().toLowerCase())
    .filter(Boolean)
}

export default function Policies() {
  const [searchParams] = useSearchParams()
  const [tenants, setTenants] = useState<Tenant[]>([])
  const [selectedTenantID, setSelectedTenantID] = useState('')
  const [versions, setVersions] = useState<PolicyVersion[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [savingConfig, setSavingConfig] = useState(false)
  const [creatingVersion, setCreatingVersion] = useState(false)
  const [rollingBack, setRollingBack] = useState(false)
  const [simLoading, setSimLoading] = useState(false)
  const [simResult, setSimResult] = useState<SimulationResponse | null>(null)

  const [builder, setBuilder] = useState<TenantPolicyConfig>({
    max_risk_auto_approve: 7,
    read_actions: [],
    write_actions: [],
    destructive_actions: [],
    require_destructive_approval: true,
  })
  const [readActionsText, setReadActionsText] = useState('')
  const [writeActionsText, setWriteActionsText] = useState('')
  const [destructiveActionsText, setDestructiveActionsText] = useState('')
  const [versionForm, setVersionForm] = useState({ version: '', notes: '' })
  const [selectedVersionID, setSelectedVersionID] = useState<number | null>(null)

  const [simForm, setSimForm] = useState({ agent_id: 'agent-1', tool: 'jira', action: 'issue.create', resource: 'project/OPS', risk_score: 8 })

  const selectedVersion = versions.find(v => v.id === selectedVersionID) ?? null

  async function fetchTenants() {
    try {
      const data = await api.get('/admin/tenants')
      const items = Array.isArray(data) ? (data as Tenant[]) : []
      setTenants(items)
      if (!selectedTenantID && items.length > 0) {
        const requestedTenantID = searchParams.get('tenant_id') || ''
        const matchedTenant = requestedTenantID ? items.find(item => item.id === requestedTenantID) : null
        setSelectedTenantID(matchedTenant?.id || items[0].id)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load tenants')
      setLoading(false)
    }
  }

  async function fetchPolicyState(tenantID: string) {
    setLoading(true)
    setError('')
    try {
      const [cfgResp, versionsResp] = await Promise.all([
        api.get(`/admin/tenants/${tenantID}/policy/config`),
        api.get(`/admin/tenants/${tenantID}/policy/versions`),
      ])
      const cfg = cfgResp as TenantPolicyConfig
      setBuilder(cfg)
      setReadActionsText((cfg.read_actions || []).join(', '))
      setWriteActionsText((cfg.write_actions || []).join(', '))
      setDestructiveActionsText((cfg.destructive_actions || []).join(', '))

      const vs = Array.isArray(versionsResp) ? (versionsResp as PolicyVersion[]) : []
      setVersions(vs)
      setSelectedVersionID(vs.length > 0 ? vs[0].id : null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load policy state')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void fetchTenants()
  }, [searchParams])

  useEffect(() => {
    if (!selectedTenantID) return
    void fetchPolicyState(selectedTenantID)
  }, [selectedTenantID])

  function buildConfigPayload(): TenantPolicyConfig {
    return {
      max_risk_auto_approve: Number(builder.max_risk_auto_approve),
      read_actions: parseActions(readActionsText),
      write_actions: parseActions(writeActionsText),
      destructive_actions: parseActions(destructiveActionsText),
      require_destructive_approval: builder.require_destructive_approval,
    }
  }

  async function handleSaveConfig(e: FormEvent) {
    e.preventDefault()
    if (!selectedTenantID) return
    setSavingConfig(true)
    setError('')
    try {
      const payload = buildConfigPayload()
      await api.put(`/admin/tenants/${selectedTenantID}/policy/config`, payload)
      setBuilder(payload)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save policy config')
    } finally {
      setSavingConfig(false)
    }
  }

  async function handleCreateVersion(e: FormEvent) {
    e.preventDefault()
    if (!selectedTenantID) return
    setCreatingVersion(true)
    setError('')
    try {
      const payload = {
        version: versionForm.version,
        notes: versionForm.notes,
        policy_data: buildConfigPayload(),
      }
      await api.post(`/admin/tenants/${selectedTenantID}/policy/versions`, payload)
      setVersionForm({ version: '', notes: '' })
      await fetchPolicyState(selectedTenantID)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create policy version')
    } finally {
      setCreatingVersion(false)
    }
  }

  async function handleRollback() {
    if (!selectedTenantID || !selectedVersionID) return
    setRollingBack(true)
    setError('')
    try {
      await api.post(`/admin/tenants/${selectedTenantID}/policy/versions/${selectedVersionID}/rollback`, {})
      await fetchPolicyState(selectedTenantID)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to rollback policy version')
    } finally {
      setRollingBack(false)
    }
  }

  async function handleSimulate(e: FormEvent) {
    e.preventDefault()
    if (!selectedTenantID) return
    setSimLoading(true)
    setError('')
    setSimResult(null)
    try {
      const payload = {
        ...simForm,
        risk_score: Number(simForm.risk_score),
        policy_config: buildConfigPayload(),
      }
      const resp = await api.post(`/admin/tenants/${selectedTenantID}/policy/simulate`, payload)
      setSimResult(resp as SimulationResponse)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to simulate policy')
    } finally {
      setSimLoading(false)
    }
  }

  const diffPreview = selectedVersion?.policy_data
    ? {
        current: buildConfigPayload(),
        selected_version: selectedVersion.policy_data,
      }
    : null

  if (!loading && tenants.length === 0) {
    return (
      <div>
        <PageHeaderBlock
          title="Policies"
          description="Tenant rule builder, simulation, versioning, and rollback."
        />
        {error ? <InlineErrorState message={error} onRetry={() => void fetchTenants()} /> : null}
        <EmptyState
          icon="☰"
          title="No tenants available"
          description="Create a tenant first so you can save policy rules, run simulations, and manage version history."
        />
      </div>
    )
  }

  return (
    <div>
      <PageHeaderBlock
        title="Policies"
        description="Tenant rule builder, simulation, versioning, and rollback."
      />

      {error ? <InlineErrorState message={error} onRetry={() => selectedTenantID ? void fetchPolicyState(selectedTenantID) : void fetchTenants()} /> : null}

      <div className="form-card">
        <h3>Tenant</h3>
        <div className="form-group" style={{ maxWidth: 420 }}>
          <label>Selected tenant</label>
          <select value={selectedTenantID} onChange={e => setSelectedTenantID(e.target.value)}>
            {tenants.map(t => (
              <option key={t.id} value={t.id}>{t.name} ({t.id})</option>
            ))}
          </select>
        </div>
      </div>

      <div className="form-card mt-16">
        <h3>Rule Builder</h3>
        <form onSubmit={handleSaveConfig}>
          <div className="form-inline" style={{ gap: 16, flexWrap: 'wrap' }}>
            <div className="form-group" style={{ minWidth: 220 }}>
              <label>Max risk auto-approve</label>
              <input
                type="number"
                min={0}
                max={10}
                value={builder.max_risk_auto_approve}
                onChange={e => setBuilder(prev => ({ ...prev, max_risk_auto_approve: Number(e.target.value) }))}
              />
            </div>
            <div className="form-group" style={{ minWidth: 280 }}>
              <label>Destructive actions require approval</label>
              <label style={{ display: 'inline-flex', gap: 8, alignItems: 'center', marginTop: 8 }}>
                <input
                  type="checkbox"
                  checked={builder.require_destructive_approval}
                  onChange={e => setBuilder(prev => ({ ...prev, require_destructive_approval: e.target.checked }))}
                />
                <span>{builder.require_destructive_approval ? 'Enabled' : 'Disabled'}</span>
              </label>
            </div>
          </div>

          <div className="form-group">
            <label>Read allowlist actions (comma separated)</label>
            <textarea rows={3} value={readActionsText} onChange={e => setReadActionsText(e.target.value)} />
          </div>
          <div className="form-group">
            <label>Write allowlist actions (comma separated)</label>
            <textarea rows={3} value={writeActionsText} onChange={e => setWriteActionsText(e.target.value)} />
          </div>
          <div className="form-group">
            <label>Destructive actions (comma separated)</label>
            <textarea rows={3} value={destructiveActionsText} onChange={e => setDestructiveActionsText(e.target.value)} />
          </div>

          <button className="btn btn-primary" disabled={savingConfig || loading}>
            {savingConfig ? 'Saving…' : 'Save Tenant Policy Config'}
          </button>
        </form>
      </div>

      <div className="form-card mt-16">
        <h3>Create Version</h3>
        <form onSubmit={handleCreateVersion}>
          <div className="form-inline" style={{ gap: 16, flexWrap: 'wrap' }}>
            <div className="form-group" style={{ minWidth: 220 }}>
              <label>Version</label>
              <input value={versionForm.version} onChange={e => setVersionForm(prev => ({ ...prev, version: e.target.value }))} required />
            </div>
            <div className="form-group" style={{ minWidth: 420 }}>
              <label>Notes</label>
              <input value={versionForm.notes} onChange={e => setVersionForm(prev => ({ ...prev, notes: e.target.value }))} />
            </div>
            <button className="btn btn-primary" disabled={creatingVersion || loading}>
              {creatingVersion ? 'Creating…' : 'Create Version Snapshot'}
            </button>
          </div>
        </form>
      </div>

      <div className="table-container mt-16">
        <table>
          <thead>
            <tr>
              <th>Version</th>
              <th>ID</th>
              <th>Deployed By</th>
              <th>Deployed At</th>
              <th>Notes</th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <TableSkeleton columns={5} rows={5} />
            ) : versions.length === 0 ? (
              <tr><td colSpan={5} style={{ textAlign: 'center', padding: 24, color: '#94a3b8' }}>No policy versions</td></tr>
            ) : (
              versions.map(v => (
                <tr
                  key={v.id}
                  onClick={() => setSelectedVersionID(v.id)}
                  style={{ cursor: 'pointer', background: selectedVersionID === v.id ? '#eff6ff' : undefined }}
                >
                  <td><span className="badge badge-blue">{v.version}</span></td>
                  <td style={{ fontFamily: 'monospace', fontSize: 12 }}>{v.id}</td>
                  <td>{v.deployed_by || '—'}</td>
                  <td>{formatDate(v.deployed_at)}</td>
                  <td>{v.notes || '—'}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      <div className="form-card mt-16">
        <h3>Version Diff + Rollback</h3>
        {!selectedVersion ? (
          <div style={{ color: '#64748b' }}>Select a policy version to compare and rollback.</div>
        ) : (
          <>
            <div style={{ marginBottom: 12 }}>
              Selected version: <span className="badge badge-blue">{selectedVersion.version}</span> ({selectedVersion.id})
            </div>
            {diffPreview && (
              <pre className="code-block" style={{ maxHeight: 260 }}>
                {JSON.stringify(diffPreview, null, 2)}
              </pre>
            )}
            <button className="btn btn-danger" onClick={handleRollback} disabled={rollingBack || loading}>
              {rollingBack ? 'Rolling back…' : 'Rollback to Selected Version'}
            </button>
          </>
        )}
      </div>

      <div className="form-card mt-16">
        <h3>Policy Simulator (Preview)</h3>
        <p style={{ fontSize: 13, color: '#64748b', marginBottom: 12 }}>
          Preview decisions using the current rule-builder values before saving.
        </p>
        <form onSubmit={handleSimulate}>
          <div className="form-inline" style={{ gap: 16, flexWrap: 'wrap' }}>
            <div className="form-group">
              <label>Agent ID</label>
              <input value={simForm.agent_id} onChange={e => setSimForm(prev => ({ ...prev, agent_id: e.target.value }))} required />
            </div>
            <div className="form-group">
              <label>Tool</label>
              <input value={simForm.tool} onChange={e => setSimForm(prev => ({ ...prev, tool: e.target.value }))} required />
            </div>
            <div className="form-group">
              <label>Action</label>
              <input value={simForm.action} onChange={e => setSimForm(prev => ({ ...prev, action: e.target.value }))} required />
            </div>
            <div className="form-group">
              <label>Resource</label>
              <input value={simForm.resource} onChange={e => setSimForm(prev => ({ ...prev, resource: e.target.value }))} />
            </div>
            <div className="form-group">
              <label>Risk score</label>
              <input
                type="number"
                min={0}
                max={10}
                value={simForm.risk_score}
                onChange={e => setSimForm(prev => ({ ...prev, risk_score: Number(e.target.value) }))}
              />
            </div>
            <button className="btn btn-primary" disabled={simLoading || loading}>
              {simLoading ? 'Simulating…' : 'Preview Decision'}
            </button>
          </div>
        </form>

        {simResult && (
          <div style={{ marginTop: 12 }}>
            <div style={{ marginBottom: 8 }}>
              <span className={`badge badge-${simResult.policy_result?.result?.decision || 'gray'}`}>
                {simResult.policy_result?.result?.decision || 'unknown'}
              </span>
              {simResult.policy_result?.result?.reason && (
                <span style={{ marginLeft: 8, fontSize: 13, color: '#475569' }}>{simResult.policy_result.result.reason}</span>
              )}
            </div>
            <pre className="code-block" style={{ maxHeight: 280 }}>
              {JSON.stringify(simResult, null, 2)}
            </pre>
          </div>
        )}
      </div>
    </div>
  )
}
