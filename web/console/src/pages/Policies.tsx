import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api } from '../api'
import {
  ActiveFiltersBar,
  EmptyState,
  InlineErrorState,
  PageHeaderBlock,
  SortHeader,
  TableEmptyStateRow,
  TableFrame,
  TableSkeleton,
  applySort,
  compareDate,
  compareText,
  formatTimeWithTitle,
  type SortState,
} from '../ui'

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

const DEFAULT_POLICY_CONFIG: TenantPolicyConfig = {
  max_risk_auto_approve: 7,
  read_actions: [],
  write_actions: [],
  destructive_actions: [],
  require_destructive_approval: true,
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

  const [builder, setBuilder] = useState<TenantPolicyConfig>(DEFAULT_POLICY_CONFIG)
  const [readActionsText, setReadActionsText] = useState('')
  const [writeActionsText, setWriteActionsText] = useState('')
  const [destructiveActionsText, setDestructiveActionsText] = useState('')
  const [versionForm, setVersionForm] = useState({ version: '', notes: '' })
  const [selectedVersionID, setSelectedVersionID] = useState<number | null>(null)
  const [versionSort, setVersionSort] = useState<SortState<'version' | 'deployed_at'>>({ key: null, dir: 'desc' })

  const [simForm, setSimForm] = useState({ agent_id: 'agent-1', tool: 'jira', action: 'issue.create', resource: 'project/OPS', risk_score: 8 })

  const selectedVersion = versions.find(v => v.id === selectedVersionID) ?? null

  async function fetchTenants() {
    try {
      const data = await api.get('/admin/tenants')
      const items = Array.isArray(data) ? (data as Tenant[]) : ((data as { tenants?: Tenant[] })?.tenants || [])
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
    const [cfgResp, versionsResp] = await Promise.allSettled([
      api.get(`/admin/tenants/${tenantID}/policy/config`),
      api.get(`/admin/tenants/${tenantID}/policy/versions`),
    ])

    const failures: string[] = []

    if (cfgResp.status === 'fulfilled') {
      const cfg = cfgResp.value as TenantPolicyConfig
      setBuilder(cfg)
      setReadActionsText((cfg.read_actions || []).join(', '))
      setWriteActionsText((cfg.write_actions || []).join(', '))
      setDestructiveActionsText((cfg.destructive_actions || []).join(', '))
    } else {
      setBuilder(DEFAULT_POLICY_CONFIG)
      setReadActionsText('')
      setWriteActionsText('')
      setDestructiveActionsText('')
      failures.push('policy config')
    }

    if (versionsResp.status === 'fulfilled') {
      const versionsData = versionsResp.value as PolicyVersion[] | { versions?: PolicyVersion[] }
      const vs = Array.isArray(versionsData) ? versionsData : Array.isArray(versionsData?.versions) ? versionsData.versions : []
      setVersions(vs)
      setSelectedVersionID(vs.length > 0 ? vs[0].id : null)
    } else {
      setVersions([])
      setSelectedVersionID(null)
      failures.push('version history')
    }

    if (failures.length > 0) {
      setError(`Some policy data could not be loaded: ${failures.join(', ')}.`)
    }

    setLoading(false)
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

  const visibleVersions = useMemo(() => {
    if (!versionSort.key) return versions
    return [...versions].sort((left, right) => {
      switch (versionSort.key) {
        case 'version':
          return applySort(compareText(left.version, right.version), versionSort.dir)
        case 'deployed_at':
          return applySort(compareDate(left.deployed_at, right.deployed_at), versionSort.dir)
        default:
          return 0
      }
    })
  }, [versionSort, versions])

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
        <div className="form-group policy-tenant-field">
          <label htmlFor="policy-selected-tenant">Selected tenant</label>
          <select id="policy-selected-tenant" value={selectedTenantID} onChange={e => setSelectedTenantID(e.target.value)}>
            {tenants.map(t => (
              <option key={t.id} value={t.id}>{t.name} ({t.id})</option>
            ))}
          </select>
        </div>
      </div>

      <div className="form-card mt-16">
        <h3>Rule Builder</h3>
        <form onSubmit={handleSaveConfig}>
          <div className="form-grid policy-builder-grid">
            <div className="form-group">
              <label htmlFor="policy-max-risk-auto-approve">Max risk auto-approve</label>
              <input
                id="policy-max-risk-auto-approve"
                type="number"
                min={0}
                max={10}
                value={builder.max_risk_auto_approve}
                onChange={e => setBuilder(prev => ({ ...prev, max_risk_auto_approve: Number(e.target.value) }))}
              />
            </div>
            <div className="form-group policy-toggle-group">
              <label htmlFor="policy-require-destructive-approval">Destructive actions require approval</label>
              <div className="toggle-field toggle-field-boxed">
                <input
                  id="policy-require-destructive-approval"
                  type="checkbox"
                  checked={builder.require_destructive_approval}
                  onChange={e => setBuilder(prev => ({ ...prev, require_destructive_approval: e.target.checked }))}
                />
                <span>{builder.require_destructive_approval ? 'Enabled' : 'Disabled'}</span>
              </div>
            </div>
          </div>

          <div className="form-group">
            <label htmlFor="policy-read-actions">Read allowlist actions (comma separated)</label>
            <textarea id="policy-read-actions" rows={3} value={readActionsText} onChange={e => setReadActionsText(e.target.value)} />
          </div>
          <div className="form-group">
            <label htmlFor="policy-write-actions">Write allowlist actions (comma separated)</label>
            <textarea id="policy-write-actions" rows={3} value={writeActionsText} onChange={e => setWriteActionsText(e.target.value)} />
          </div>
          <div className="form-group">
            <label htmlFor="policy-destructive-actions">Destructive actions (comma separated)</label>
            <textarea id="policy-destructive-actions" rows={3} value={destructiveActionsText} onChange={e => setDestructiveActionsText(e.target.value)} />
          </div>

          <button className="btn btn-primary" disabled={savingConfig || loading}>
            {savingConfig ? 'Saving…' : 'Save Tenant Policy Config'}
          </button>
        </form>
      </div>

      <div className="form-card mt-16">
        <h3>Create Version</h3>
        <form onSubmit={handleCreateVersion}>
          <div className="form-grid policy-version-grid">
            <div className="form-group">
              <label htmlFor="policy-version-name">Version</label>
              <input id="policy-version-name" value={versionForm.version} onChange={e => setVersionForm(prev => ({ ...prev, version: e.target.value }))} required />
            </div>
            <div className="form-group">
              <label htmlFor="policy-version-notes">Notes</label>
              <input id="policy-version-notes" value={versionForm.notes} onChange={e => setVersionForm(prev => ({ ...prev, notes: e.target.value }))} />
            </div>
            <div className="form-actions-row form-actions-row-end policy-version-actions">
              <button className="btn btn-primary" disabled={creatingVersion || loading}>
                {creatingVersion ? 'Creating…' : 'Create Version Snapshot'}
              </button>
            </div>
          </div>
        </form>
      </div>

      <ActiveFiltersBar
        resultCount={visibleVersions.length}
        resultLabel={visibleVersions.length === 1 ? 'version' : 'versions'}
        chips={[]}
        note={versionSort.key ? 'Sorted within the current page.' : 'Using backend order until you sort this page.'}
      />

      <TableFrame className="mt-16" stickyHeader>
        <table>
          <thead>
            <tr>
              <th>
                <SortHeader label="Version" sortKey="version" sortState={versionSort} onSortChange={(key, dir) => setVersionSort({ key, dir })} defaultDir="desc" />
              </th>
              <th>ID</th>
              <th>Deployed By</th>
              <th className="col-time">
                <SortHeader label="Deployed" sortKey="deployed_at" sortState={versionSort} onSortChange={(key, dir) => setVersionSort({ key, dir })} defaultDir="desc" className="col-time" />
              </th>
              <th>Notes</th>
              <th className="table-action-col"></th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <TableSkeleton columns={6} rows={5} />
            ) : visibleVersions.length === 0 ? (
              <TableEmptyStateRow
                colSpan={6}
                icon="↺"
                title="No saved policy versions yet"
                description="Save a version snapshot when you want a rollback point or a reviewable change history for this tenant."
              />
            ) : (
              visibleVersions.map((v, index) => {
                const deployed = formatTimeWithTitle(v.deployed_at)
                const isCurrent = index === 0 && !versionSort.key
                return (
                <tr key={v.id} className={selectedVersionID === v.id ? 'policy-version-row is-selected' : 'policy-version-row'}>
                  <td>
                    <div className="table-primary-cell">
                      <div className="stacked-badges">
                        <span className="badge badge-blue">{v.version}</span>
                        {isCurrent ? <span className="badge badge-green">Current</span> : null}
                      </div>
                    </div>
                  </td>
                  <td className="mono">{v.id}</td>
                  <td>{v.deployed_by || '—'}</td>
                  <td className="col-time" title={deployed.title}>{deployed.label}</td>
                  <td>{v.notes || '—'}</td>
                  <td className="table-action-cell">
                    <button className="btn btn-outline btn-sm" type="button" onClick={() => setSelectedVersionID(v.id)}>
                      {selectedVersionID === v.id ? 'Selected' : 'Select'}
                    </button>
                  </td>
                </tr>
              )})
            )}
          </tbody>
        </table>
      </TableFrame>

      <div className="form-card mt-16">
        <h3>Version Diff + Rollback</h3>
        {!selectedVersion ? (
          <div className="policy-empty-copy">Select a policy version to compare and rollback.</div>
        ) : (
          <>
            <div className="policy-selected-version">
              Selected version: <span className="badge badge-blue">{selectedVersion.version}</span> ({selectedVersion.id})
            </div>
            {diffPreview && (
              <pre className="code-block policy-code-block-sm">
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
        <p className="policy-helper-copy">
          Preview decisions using the current rule-builder values before saving.
        </p>
        <form onSubmit={handleSimulate}>
          <div className="form-grid policy-simulator-grid">
            <div className="form-group">
              <label htmlFor="policy-sim-agent-id">Agent ID</label>
              <input id="policy-sim-agent-id" value={simForm.agent_id} onChange={e => setSimForm(prev => ({ ...prev, agent_id: e.target.value }))} required />
            </div>
            <div className="form-group">
              <label htmlFor="policy-sim-tool">Tool</label>
              <input id="policy-sim-tool" value={simForm.tool} onChange={e => setSimForm(prev => ({ ...prev, tool: e.target.value }))} required />
            </div>
            <div className="form-group">
              <label htmlFor="policy-sim-action">Action</label>
              <input id="policy-sim-action" value={simForm.action} onChange={e => setSimForm(prev => ({ ...prev, action: e.target.value }))} required />
            </div>
            <div className="form-group">
              <label htmlFor="policy-sim-resource">Resource</label>
              <input id="policy-sim-resource" value={simForm.resource} onChange={e => setSimForm(prev => ({ ...prev, resource: e.target.value }))} />
            </div>
            <div className="form-group">
              <label htmlFor="policy-sim-risk-score">Risk score</label>
              <input
                id="policy-sim-risk-score"
                type="number"
                min={0}
                max={10}
                value={simForm.risk_score}
                onChange={e => setSimForm(prev => ({ ...prev, risk_score: Number(e.target.value) }))}
              />
            </div>
            <div className="form-actions-row form-actions-row-end policy-simulator-actions">
              <button className="btn btn-primary" disabled={simLoading || loading}>
                {simLoading ? 'Simulating…' : 'Preview Decision'}
              </button>
            </div>
          </div>
        </form>

        {simResult && (
          <div className="policy-sim-result">
            <div className="policy-sim-summary">
              <span className={`badge badge-${simResult.policy_result?.result?.decision || 'gray'}`}>
                {simResult.policy_result?.result?.decision || 'unknown'}
              </span>
              {simResult.policy_result?.result?.reason && (
                <span className="policy-sim-reason">{simResult.policy_result.result.reason}</span>
              )}
            </div>
            <pre className="code-block policy-code-block">
              {JSON.stringify(simResult, null, 2)}
            </pre>
          </div>
        )}
      </div>
    </div>
  )
}
