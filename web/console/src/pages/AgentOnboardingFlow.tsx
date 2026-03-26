import { useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import { CopyIconButton, InlineErrorState, downloadBlob } from '../ui'

type TenantOption = {
  id: string
  name: string
  status?: string
}

type AgentOption = {
  id: string
  name: string
  status?: string
  onboarding?: {
    runtime?: RuntimeOption['value'] | string
    environment_label?: string
    owner_name?: string
    description?: string
    approval_posture?: ApprovalPostureOption['value'] | string
    updated_at?: string
    tools?: SelectedTool[]
  } | null
}

type ConnectorInfo = {
  name: string
  actions: string[]
  type: string
}

type SelectedTool = {
  tool: string
  action: string
}

type BundleArtifact = {
  id: string
  label: string
  file_name: string
  path_hint?: string
  kind: string
  purpose?: string
  writable?: boolean
  language?: string
  content: string
}

type VerificationLink = {
  label: string
  path: string
  description?: string
}

type OnboardingResponse = {
  mode: 'preview' | 'created' | 'regenerated' | 'regenerated_defaults'
  tenant: { id: string; name: string; created: boolean }
  agent: { id: string; name: string; status: string; created_at?: string; preview: boolean }
  api_key?: { id: string; name: string; key_prefix: string; raw_key: string }
  bundle: {
    title?: string
    summary?: string
    runtime: string
    runtime_label: string
    starter_file_name: string
    environment: Record<string, string>
    environment_script: string
    environment_file: string
    starter_snippet: string
    readme_snippet: string
    sample_call: string
    artifacts: BundleArtifact[]
    verification_checklist: string[]
    verification_links: VerificationLink[]
    applied_defaults?: Array<{ field: string; value: string; reason?: string }>
    notes: string[]
  }
}

type Props = {
  open: boolean
  onClose: () => void
  presetTenant?: TenantOption | null
  presetAgent?: AgentOption | null
  tenantOptions?: TenantOption[]
  onCreated?: (result: OnboardingResponse) => void
}

type RuntimeOption = {
  value: 'python' | 'typescript' | 'langchain' | 'openai_local'
  label: string
  description: string
}

type ApprovalPostureOption = {
  value: 'pilot_safe' | 'read_only_first' | 'tenant_default'
  label: string
  description: string
}

type ResultTab = 'environment' | 'files' | 'test' | 'verify'

type OnboardingDraft = {
  runtime: RuntimeOption['value']
  tenantMode: 'existing' | 'create'
  tenantID: string
  newTenantName: string
  agentName: string
  environmentLabel: string
  ownerName: string
  description: string
  approvalPosture: ApprovalPostureOption['value']
  selectedTools: SelectedTool[]
}

const runtimeOptions: RuntimeOption[] = [
  { value: 'python', label: 'Python service', description: 'Best first path when you already own the tool-execution loop.' },
  { value: 'typescript', label: 'TypeScript / Node service', description: 'Best for workers and services already running on Node.js or edge-friendly TypeScript runtimes.' },
  { value: 'langchain', label: 'LangChain agent', description: 'Fastest framework-native path in Python ecosystems.' },
  { value: 'openai_local', label: 'Local OpenAI-compatible model', description: 'Best for LM Studio, Ollama, vLLM, and similar local model setups.' },
]

const approvalPostureOptions: ApprovalPostureOption[] = [
  { value: 'pilot_safe', label: 'Pilot-safe', description: 'Allow safer reads and expect high-risk writes to go through approval.' },
  { value: 'read_only_first', label: 'Reads first', description: 'Start with read-heavy evaluation traffic and add write approvals later.' },
  { value: 'tenant_default', label: 'Tenant default', description: 'Generate the bundle without adding extra posture assumptions in the starter copy.' },
]

const curatedToolOrder = [
  { tool: 'slack', action: 'slack.channel.list', title: 'Slack channel list', hint: 'Safe pilot read' },
  { tool: 'slack', action: 'slack.msg.post', title: 'Slack message post', hint: 'Good approval/write path' },
  { tool: 'jira', action: 'jira.issue.list', title: 'Jira issue list', hint: 'Good read path for ticketing pilots' },
  { tool: 'jira', action: 'jira.issue.create', title: 'Jira issue create', hint: 'Clear write-path example' },
  { tool: 'postgres', action: 'query.readonly', title: 'Postgres readonly query', hint: 'Great for safe data-access pilots' },
  { tool: 'github', action: 'issue.create', title: 'GitHub issue create', hint: 'Useful dev/demo write action' },
  { tool: 'email', action: 'send', title: 'Email send', hint: 'Simple high-signal write example' },
  { tool: 'webhook', action: 'post', title: 'Webhook post', hint: 'Flexible adapter-friendly action' },
]

const resultTabs: Array<{ id: ResultTab; label: string }> = [
  { id: 'environment', label: 'Environment' },
  { id: 'files', label: 'Files' },
  { id: 'test', label: 'Test call' },
  { id: 'verify', label: 'Verify in console' },
]

function formatToolSelection(selection: SelectedTool) {
  return `${selection.tool}:${selection.action}`
}

function approvalPostureLabel(value: ApprovalPostureOption['value'] | string) {
  return approvalPostureOptions.find(option => option.value === value)?.label || value
}

function supportedRuntimeOrDefault(value?: string) {
  return runtimeOptions.find(option => option.value === value)?.value || 'python'
}

function supportedApprovalPostureOrDefault(value?: string) {
  return approvalPostureOptions.find(option => option.value === value)?.value || 'pilot_safe'
}

function onboardingErrorHint(message: string) {
  const normalized = message.toLowerCase()
  if (normalized.includes('preview requires tenant_id')) {
    return 'Preview only works against an existing tenant. Switch to an existing tenant or create the tenant first, then preview future updates against it.'
  }
  if (normalized.includes('at least one tool selection is required')) {
    return 'Pick at least one governed tool before previewing, creating, or regenerating a bundle.'
  }
  if (normalized.includes('no curated onboarding defaults are available')) {
    return 'Regenerate with defaults needs at least one curated connector action in the catalog. Use Regenerate bundle to choose tools manually or enable a supported connector first.'
  }
  if (normalized.includes('insufficient permissions')) {
    return 'Open onboarding inside a tenant you administer, or ask a platform admin to create the tenant or agent context for you.'
  }
  if (normalized.includes('failed to load connector registry')) {
    return 'OpenClause could not load connector metadata. Confirm the gateway and connector catalog are healthy, then retry the flow.'
  }
  if (normalized.includes('tenant_id or new_tenant_name required')) {
    return 'Choose an existing tenant for preview, or enter a new tenant name before creating the real integration.'
  }
  return ''
}

function resultStatusCopy(result: OnboardingResponse) {
  switch (result.mode) {
    case 'preview':
      return {
        title: 'Preview only: nothing was created yet.',
        body: 'You have a synthetic preview agent id and placeholder API key values so you can inspect the bundle safely before minting real credentials.',
      }
    case 'regenerated':
      return {
        title: 'Bundle refreshed for an existing agent.',
        body: result.api_key?.key_prefix
          ? 'Tenant and agent records were reused. The bundle now points back at an existing key reference, not a newly issued raw key, and the saved onboarding setup was refreshed for future regenerations.'
          : 'Tenant and agent records were reused, but no active API key is available to reference yet. Create or rotate a key before handing this bundle off.',
      }
    case 'regenerated_defaults':
      return {
        title: 'Bundle refreshed from explicit defaults.',
        body: result.api_key?.key_prefix
          ? 'OpenClause applied the current default runtime, pilot posture, and curated tool set so you can hand off a safe starting point quickly.'
          : 'OpenClause applied the current default runtime, pilot posture, and curated tool set, but you still need to create or rotate an API key before anyone can use this bundle.',
      }
    default:
      return {
        title: 'Integration created successfully.',
        body: 'The tenant context, agent, and API key now exist. The full raw key is visible only in this result and the generated env artifacts from this create step. OpenClause also saved the runtime, tools, and approval posture on the agent for future regenerations.',
      }
  }
}

export default function AgentOnboardingFlow({ open, onClose, presetTenant = null, presetAgent = null, tenantOptions = [], onCreated }: Props) {
  const draftStoreRef = useRef<Record<string, OnboardingDraft>>({})
  const [runtime, setRuntime] = useState<RuntimeOption['value']>('python')
  const [tenantMode, setTenantMode] = useState<'existing' | 'create'>(presetTenant ? 'existing' : (tenantOptions.length > 0 ? 'existing' : 'create'))
  const [tenantID, setTenantID] = useState(presetTenant?.id || tenantOptions[0]?.id || '')
  const [newTenantName, setNewTenantName] = useState('')
  const [agentName, setAgentName] = useState('')
  const [environmentLabel, setEnvironmentLabel] = useState('dev')
  const [ownerName, setOwnerName] = useState('')
  const [description, setDescription] = useState('')
  const [approvalPosture, setApprovalPosture] = useState<ApprovalPostureOption['value']>('pilot_safe')
  const [connectors, setConnectors] = useState<ConnectorInfo[]>([])
  const [selectedTools, setSelectedTools] = useState<SelectedTool[]>([])
  const [loadingCatalog, setLoadingCatalog] = useState(false)
  const [previewing, setPreviewing] = useState(false)
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')
  const [result, setResult] = useState<OnboardingResponse | null>(null)
  const [activeTab, setActiveTab] = useState<ResultTab>('environment')
  const [selectedArtifactID, setSelectedArtifactID] = useState('')
  const [downloading, setDownloading] = useState(false)
  const isRegenerateFlow = !!presetAgent
  const draftKey = `${presetTenant?.id || 'shared'}:${presetAgent?.id || 'new'}`

  useEffect(() => {
    if (!open) return
    const savedDraft = draftStoreRef.current[draftKey]
    setError('')
    setResult(null)
    setActiveTab('environment')
    setSelectedArtifactID('')
    setDownloading(false)
    setRuntime(savedDraft?.runtime || supportedRuntimeOrDefault(presetAgent?.onboarding?.runtime))
    setTenantMode(savedDraft?.tenantMode || (presetTenant ? 'existing' : (tenantOptions.length > 0 ? 'existing' : 'create')))
    setTenantID(savedDraft?.tenantID || presetTenant?.id || tenantOptions[0]?.id || '')
    setNewTenantName(savedDraft?.newTenantName || '')
    setAgentName(presetAgent?.name || savedDraft?.agentName || '')
    setEnvironmentLabel(savedDraft?.environmentLabel || presetAgent?.onboarding?.environment_label || 'dev')
    setOwnerName(savedDraft?.ownerName || presetAgent?.onboarding?.owner_name || '')
    setDescription(savedDraft?.description || presetAgent?.onboarding?.description || '')
    setApprovalPosture(savedDraft?.approvalPosture || supportedApprovalPostureOrDefault(presetAgent?.onboarding?.approval_posture))
    setSelectedTools(savedDraft?.selectedTools || presetAgent?.onboarding?.tools || [])
  }, [draftKey, open, presetTenant?.id, presetAgent?.id])

  useEffect(() => {
    if (!open) return
    let cancelled = false
    async function fetchCatalog() {
      setLoadingCatalog(true)
      setError('')
      try {
        const data = await api.get('/admin/connectors')
        if (!cancelled) {
          const nextConnectors = Array.isArray(data) ? data as ConnectorInfo[] : []
          setConnectors(nextConnectors)
          const byTool = new Map(nextConnectors.map(connector => [connector.name, new Set(connector.actions || [])]))
          const defaults = curatedToolOrder
            .filter(item => byTool.get(item.tool)?.has(item.action))
            .slice(0, Math.min(2, curatedToolOrder.length))
            .map(item => ({ tool: item.tool, action: item.action }))
          if (defaults.length > 0) {
            setSelectedTools(current => {
              const validCurrent = current.filter(item => byTool.get(item.tool)?.has(item.action))
              if (validCurrent.length > 0) return validCurrent
              return defaults
            })
          } else {
            setSelectedTools(current => current.filter(item => byTool.get(item.tool)?.has(item.action)))
          }
        }
      } catch (err: any) {
        if (!cancelled) setError(err?.message || 'Failed to load connector catalog')
      } finally {
        if (!cancelled) setLoadingCatalog(false)
      }
    }
    void fetchCatalog()
    return () => { cancelled = true }
  }, [open])

  useEffect(() => {
    if (!result || result.bundle.artifacts.length === 0) return
    setSelectedArtifactID(current => current || result.bundle.artifacts[0].id)
  }, [result])

  useEffect(() => {
    if (!open) return
    draftStoreRef.current[draftKey] = {
      runtime,
      tenantMode,
      tenantID,
      newTenantName,
      agentName,
      environmentLabel,
      ownerName,
      description,
      approvalPosture,
      selectedTools,
    }
  }, [draftKey, open, runtime, tenantMode, tenantID, newTenantName, agentName, environmentLabel, ownerName, description, approvalPosture, selectedTools])

  const curatedOptions = useMemo(() => {
    const byTool = new Map(connectors.map(connector => [connector.name, new Set(connector.actions || [])]))
    return curatedToolOrder.filter(item => byTool.get(item.tool)?.has(item.action))
  }, [connectors])

  const effectiveTenantID = presetTenant?.id || tenantID || tenantOptions[0]?.id || ''
  const trimmedAgentName = agentName.trim()
  const trimmedNewTenantName = newTenantName.trim()
  const hasToolSelection = selectedTools.length > 0
  const canPreview = !!(presetTenant?.id || (tenantMode === 'existing' && effectiveTenantID))
  const previewBlockedReason = !canPreview
    ? 'Preview uses an existing tenant only. Create the tenant on submit, then regenerate or preview future updates against that tenant.'
    : ''
  const canSubmitPreview = canPreview && trimmedAgentName.length > 0 && hasToolSelection && !loadingCatalog && !creating && !previewing
  const canSubmitCreate = !isRegenerateFlow && trimmedAgentName.length > 0 && hasToolSelection && (presetTenant ? true : (tenantMode === 'existing' ? !!effectiveTenantID : trimmedNewTenantName.length > 0)) && !loadingCatalog && !creating && !previewing
  const canSubmitRegenerate = isRegenerateFlow && hasToolSelection && !loadingCatalog && !creating && !previewing
  const canSubmitRegenerateDefaults = isRegenerateFlow && curatedOptions.length > 0 && !loadingCatalog && !creating && !previewing
  const previewActionHint = !canPreview
    ? previewBlockedReason
    : trimmedAgentName.length === 0
      ? 'Add an agent name before generating a preview bundle.'
      : !hasToolSelection
        ? 'Pick at least one governed tool before previewing the bundle.'
        : 'Preview is safe: it never creates a tenant, agent, or API key.'
  const createActionHint = presetTenant || tenantMode === 'existing'
    ? (!effectiveTenantID
      ? 'Select a tenant before creating a real integration.'
      : trimmedAgentName.length === 0
        ? 'Add an agent name before creating the real integration.'
        : !hasToolSelection
          ? 'Pick at least one governed tool before creating the real integration.'
          : 'Create is the real persistence step: it mints the agent and one-time raw API key.')
    : (trimmedNewTenantName.length === 0
      ? 'Enter a tenant name before creating a new tenant inline.'
      : trimmedAgentName.length === 0
        ? 'Add an agent name before creating the real integration.'
        : !hasToolSelection
          ? 'Pick at least one governed tool before creating the real integration.'
          : 'Create will make the tenant, agent, and API key in one step.')
  const regenerateActionHint = !hasToolSelection
    ? 'Pick at least one governed tool before regenerating a manual bundle.'
    : 'Manual regenerate reuses the tenant and agent, but respects the runtime, tool, and posture choices you made above.'
  const regenerateDefaultsHint = curatedOptions.length === 0
    ? 'Defaults regeneration is unavailable until the connector catalog exposes at least one curated golden-path action.'
    : 'Defaults regeneration chooses the current safe runtime, curated tools, and pilot posture without reissuing credentials.'
  const errorHint = error ? onboardingErrorHint(error) : ''

  const selectedArtifact = useMemo(
    () => result?.bundle.artifacts.find(artifact => artifact.id === selectedArtifactID) || result?.bundle.artifacts[0] || null,
    [result, selectedArtifactID],
  )
  const resultToolSummary = useMemo(() => {
    if (!result) return [] as string[]
    const defaultedTools = result.bundle.applied_defaults?.filter(item => item.field === 'tool').map(item => item.value) || []
    if (defaultedTools.length > 0) return defaultedTools
    return selectedTools.map(formatToolSelection)
  }, [result, selectedTools])
  const resultApprovalSummary = useMemo(() => {
    if (!result) return ''
    const defaultedPosture = result.bundle.applied_defaults?.find(item => item.field === 'approval_posture')?.value
    return approvalPostureLabel(defaultedPosture || approvalPosture)
  }, [approvalPosture, result])

  useEffect(() => {
    if (!open) return
    if (presetTenant) return
    if (tenantMode !== 'existing') return
    if (tenantID || tenantOptions.length === 0) return
    setTenantID(tenantOptions[0].id)
  }, [open, presetTenant, tenantID, tenantMode, tenantOptions])

  if (!open) return null

  function isToolSelected(tool: string, action: string) {
    return selectedTools.some(item => item.tool === tool && item.action === action)
  }

  function toggleTool(tool: string, action: string) {
    setSelectedTools(current =>
      isToolSelected(tool, action)
        ? current.filter(item => !(item.tool === tool && item.action === action))
        : [...current, { tool, action }],
    )
  }

  function buildPayload() {
    return {
      runtime,
      tenant_id: presetTenant ? presetTenant.id : (tenantMode === 'existing' ? effectiveTenantID : undefined),
      new_tenant_name: presetTenant ? undefined : (tenantMode === 'create' ? newTenantName : undefined),
      agent_name: agentName,
      environment_label: environmentLabel,
      owner_name: ownerName,
      description,
      approval_posture: approvalPosture,
      tools: selectedTools,
    }
  }

  function buildArchiveFilename(nextResult: OnboardingResponse) {
    const slug = (value: string | undefined) => (value || '').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '') || 'bundle'
    return `openclause-${slug(nextResult.tenant.name || nextResult.tenant.id)}-${slug(nextResult.agent.name || nextResult.agent.id)}-${slug(nextResult.mode)}.zip`
  }

  async function handlePreview() {
    if (!canPreview) return
    setPreviewing(true)
    setError('')
    try {
      const payload = {
        runtime,
        tenant_id: presetTenant ? presetTenant.id : effectiveTenantID,
        agent_name: agentName,
        environment_label: environmentLabel,
        owner_name: ownerName,
        description,
        approval_posture: approvalPosture,
        tools: selectedTools,
      }
      const next = await api.post('/admin/onboarding/bundles/preview', payload) as OnboardingResponse
      setResult(next)
      setActiveTab('environment')
    } catch (err: any) {
      setError(err?.message || 'Failed to preview onboarding bundle')
    } finally {
      setPreviewing(false)
    }
  }

  async function handleCreate() {
    setCreating(true)
    setError('')
    try {
      const next = await api.post('/admin/onboarding/integrations', buildPayload()) as OnboardingResponse
      setResult(next)
      setActiveTab('environment')
      onCreated?.(next)
    } catch (err: any) {
      setError(err?.message || 'Failed to create onboarding bundle')
    } finally {
      setCreating(false)
    }
  }

  async function handleRegenerate() {
    if (!presetTenant || !presetAgent) return
    setCreating(true)
    setError('')
    try {
      const next = await api.post('/admin/onboarding/bundles/regenerate', {
        runtime,
        tenant_id: presetTenant.id,
        agent_id: presetAgent.id,
        environment_label: environmentLabel,
        owner_name: ownerName,
        description,
        approval_posture: approvalPosture,
        tools: selectedTools,
      }) as OnboardingResponse
      setResult(next)
      setActiveTab('environment')
    } catch (err: any) {
      setError(err?.message || 'Failed to regenerate onboarding bundle')
    } finally {
      setCreating(false)
    }
  }

  async function handleRegenerateDefaults() {
    if (!presetTenant || !presetAgent) return
    setCreating(true)
    setError('')
    try {
      const next = await api.post('/admin/onboarding/bundles/regenerate-defaults', {
        tenant_id: presetTenant.id,
        agent_id: presetAgent.id,
        environment_label: environmentLabel,
        owner_name: ownerName,
        description,
      }) as OnboardingResponse
      setResult(next)
      setActiveTab('environment')
    } catch (err: any) {
      setError(err?.message || 'Failed to regenerate onboarding bundle with defaults')
    } finally {
      setCreating(false)
    }
  }

  function resetResult() {
    setResult(null)
    setActiveTab('environment')
    setSelectedArtifactID('')
  }

  async function handleDownloadBundle() {
    if (!result) return
    setDownloading(true)
    setError('')
    try {
      const blob = await api.postBlob('/admin/onboarding/bundles/archive', result)
      downloadBlob(blob, buildArchiveFilename(result))
    } catch (err: any) {
      setError(err?.message || 'Failed to download onboarding bundle')
    } finally {
      setDownloading(false)
    }
  }

  function resultModeLabel(nextResult: OnboardingResponse) {
    switch (nextResult.mode) {
      case 'preview':
        return 'Preview bundle'
      case 'regenerated':
        return 'Regenerated bundle'
      case 'regenerated_defaults':
        return 'Regenerated from defaults'
      default:
        return 'Created bundle'
    }
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal onboarding-modal" onClick={event => event.stopPropagation()}>
        <div className="flex-between mb-16">
          <div>
            <h3>{isRegenerateFlow ? 'Regenerate Agent Bundle' : 'Create Agent Integration'}</h3>
            <p className="table-subtext">
              {isRegenerateFlow
                ? 'Refresh onboarding artifacts for an existing agent without minting a new API key.'
                : 'Start the v0.5 golden path by previewing a bundle, then creating a real agent and API key when the generated shape looks right.'}
            </p>
          </div>
          <button className="btn btn-outline btn-sm" type="button" onClick={onClose}>
            Close
          </button>
        </div>

        {error ? (
          <>
            <InlineErrorState message={error} />
            {errorHint ? <div className="form-helper-text mt-8">{errorHint}</div> : null}
          </>
        ) : null}

        <div className="banner-note banner-note-compact mb-16">
          <div>
            {isRegenerateFlow ? (
              <>
                <strong>Regeneration is non-destructive.</strong> It reuses the persisted tenant and agent, and it does not reissue a raw API key.
              </>
            ) : (
              <>
                <strong>Preview is non-destructive.</strong> It reuses an existing tenant and generates a synthetic preview agent id.
                Creating the integration is what mints the real agent record and API key.
              </>
            )}
          </div>
        </div>

        {isRegenerateFlow && presetAgent?.onboarding ? (
          <div className="form-helper-text mb-16">
            Starting from the last saved onboarding setup for this agent{presetAgent.onboarding.updated_at ? ` (updated ${presetAgent.onboarding.updated_at})` : ''}. You can adjust any field before regenerating.
          </div>
        ) : null}

        {result ? (
          <div className="onboarding-results">
            <div className={`${result.mode === 'created' ? 'success-msg' : 'banner-note'} banner-note-compact`}>
              <div>
                <strong>{resultStatusCopy(result).title}</strong> {resultStatusCopy(result).body}
              </div>
            </div>

            <div className="detail-panel mt-16">
              <h3>{result.bundle.title || resultModeLabel(result)}</h3>
              <div className="detail-row">
                <div className="detail-label">Bundle summary</div>
                <div className="detail-value">
                  <div>{result.bundle.summary || `${result.bundle.runtime_label} starter bundle`}</div>
                  <div className="form-helper-text mt-8">Goal: send one call, see one event, see one session before widening the integration.</div>
                </div>
              </div>
              <div className="detail-row">
                <div className="detail-label">Governed tools</div>
                <div className="detail-value">
                  {resultToolSummary.length > 0 ? resultToolSummary.join(' · ') : 'No tool summary available'}
                </div>
              </div>
              <div className="detail-row">
                <div className="detail-label">Approval posture</div>
                <div className="detail-value">{resultApprovalSummary || 'No posture hint recorded'}</div>
              </div>
              <div className="detail-row">
                <div className="detail-label">What to do next</div>
                <div className="detail-value">
                  {result.mode === 'preview'
                    ? 'Review the generated files, then use Create agent and API key when you are ready to mint the real credentials.'
                    : result.mode === 'created'
                      ? 'Load the generated environment, run the smoke test, and verify the first event and session before sharing the bundle.'
                      : result.api_key?.key_prefix
                        ? `Load the generated environment, point OPENCLAUSE_API_KEY at the active key matching ${result.api_key.key_prefix}, and rerun the smoke test.`
                        : 'Create or rotate an API key from Tenant Detail -> API Keys, then rerun regenerate before handing this bundle off.'}
                </div>
              </div>
            </div>

            <div className="card-grid">
              <div className="card">
                <div className="card-label">Tenant</div>
                <div className="card-value">{result.tenant.name}</div>
                <div className="stat-hint mono">{result.tenant.id}</div>
                <div className="stat-hint">{result.tenant.created ? 'Generated during create' : 'User-provided existing tenant'}</div>
              </div>
              <div className="card">
                <div className="card-label">Agent</div>
                <div className="card-value">{result.agent.name}</div>
                <div className="stat-hint mono">{result.agent.id}</div>
                <div className="stat-hint">
                  {result.mode === 'preview'
                    ? 'Generated preview id'
                    : result.mode === 'regenerated' || result.mode === 'regenerated_defaults'
                      ? 'Persisted existing agent'
                      : 'Generated during create'}
                </div>
              </div>
              <div className="card">
                <div className="card-label">API key</div>
                <div className="card-value">{result.mode === 'created' ? 'Issued' : result.mode === 'preview' ? 'Not minted yet' : result.api_key?.key_prefix ? 'Existing key reference' : 'No active key found'}</div>
                <div className="stat-hint mono">{result.mode === 'created' ? result.api_key?.key_prefix : result.api_key?.key_prefix || (result.mode === 'preview' ? 'generated-on-create' : 'create-or-rotate-key')}</div>
                <div className="stat-hint">
                  {result.mode === 'created'
                    ? 'One-time raw key shown now'
                    : result.mode === 'regenerated' || result.mode === 'regenerated_defaults'
                      ? 'Raw key is not reissued during regeneration'
                      : 'Generated value shown as placeholder in preview'}
                </div>
              </div>
              <div className="card">
                <div className="card-label">Runtime</div>
                <div className="card-value">{result.bundle.runtime_label}</div>
                <div className="stat-hint">{result.bundle.starter_file_name}</div>
                <div className="stat-hint">{resultModeLabel(result)}</div>
              </div>
            </div>

            {result.mode === 'created' && result.api_key?.raw_key ? (
              <div className="form-card mt-16">
                <div className="flex-between">
                  <h3>One-time API key</h3>
                  <CopyIconButton text={result.api_key.raw_key} label="Onboarding API key" />
                </div>
                <div className="form-helper-text">
                  This full key is only returned during create. Copy it now or download the bundle before leaving this result because it will not be shown again.
                </div>
                <pre className="code-block">{result.api_key.raw_key}</pre>
              </div>
            ) : null}

            {(result.mode === 'regenerated' || result.mode === 'regenerated_defaults') && !result.api_key ? (
              <div className="banner-note banner-note-compact mt-16">
                <div>
                  <strong>Action required before the smoke test.</strong> This tenant has no active API key to reuse yet. Create or rotate one from the tenant API Keys tab, then regenerate the bundle again.
                </div>
              </div>
            ) : null}

            {result.bundle.applied_defaults && result.bundle.applied_defaults.length > 0 ? (
              <div className="banner-note banner-note-compact mt-16">
                <div>
                  <strong>Defaults applied.</strong>{' '}
                  {result.bundle.applied_defaults.map(item => `${item.field}: ${item.value}${item.reason ? ` (${item.reason})` : ''}`).join(' · ')}
                </div>
              </div>
            ) : null}

            <div className="onboarding-tabbar mt-16">
              {resultTabs.map(tab => (
                <button
                  key={tab.id}
                  className={`btn ${activeTab === tab.id ? 'btn-primary' : 'btn-outline'} btn-sm`}
                  type="button"
                  onClick={() => setActiveTab(tab.id)}
                >
                  {tab.label}
                </button>
              ))}
            </div>

	            {activeTab === 'environment' ? (
	              <div className="form-card mt-16">
                <h3>Environment</h3>
                <div className="form-helper-text">
                  {result.mode === 'regenerated'
                    ? 'Generated export script references an existing API key variable because raw keys are only shown at creation time.'
                    : result.mode === 'regenerated_defaults'
                      ? 'This bundle was regenerated from explicit safe defaults. Review the assumed runtime, tools, and approval posture before handing it off.'
                    : result.mode === 'preview'
                      ? 'Preview uses placeholder API key values and does not create credentials.'
                      : 'Generated export script for quick local testing. This create result includes the one-time raw key shown above.'}
                </div>
                <pre className="code-block">{result.bundle.environment_script}</pre>
                <div className="form-helper-text mt-16">Generated <code className="mono">.env.example</code> content.</div>
                <pre className="code-block">{result.bundle.environment_file}</pre>
                {result.bundle.applied_defaults && result.bundle.applied_defaults.length > 0 ? (
                  <div className="form-helper-text mt-16">
                    {result.bundle.applied_defaults.map(item => `${item.field}: ${item.value}${item.reason ? ` (${item.reason})` : ''}`).join(' · ')}
                  </div>
                ) : null}
              </div>
            ) : null}

            {activeTab === 'files' ? (
              <div className="form-card mt-16">
                <h3>Files</h3>
                <div className="onboarding-artifact-grid">
                  <div className="onboarding-artifact-list">
                    {result.bundle.artifacts.map(artifact => (
                      <button
                        key={artifact.id}
                        className={`choice-card onboarding-artifact-button ${selectedArtifact?.id === artifact.id ? 'is-selected' : ''}`}
                        type="button"
                        onClick={() => setSelectedArtifactID(artifact.id)}
                      >
                        <span className="choice-card-title">{artifact.label}</span>
                        <span className="choice-card-body">
                          <code className="mono">{artifact.file_name}</code>
                        </span>
                      </button>
                    ))}
                  </div>
	                  <div>
	                    <div className="form-helper-text">
	                      {selectedArtifact ? <>File: <code className="mono">{selectedArtifact.file_name}</code></> : 'Select a generated file to inspect.'}
	                    </div>
	                    {selectedArtifact?.purpose ? <div className="form-helper-text mt-8">{selectedArtifact.purpose}</div> : null}
                      {selectedArtifact?.path_hint ? <div className="form-helper-text mt-8">Suggested path: <code className="mono">{selectedArtifact.path_hint}</code></div> : null}
	                    {selectedArtifact ? <pre className="code-block">{selectedArtifact.content}</pre> : null}
	                  </div>
	                </div>
              </div>
            ) : null}

            {activeTab === 'test' ? (
              <div className="form-card mt-16">
                <h3>Test call</h3>
                <div className="form-helper-text">Use this smoke test once you have the generated environment loaded. Success looks like one event, one session, and an approval record only when the selected action is gated.</div>
                <pre className="code-block">{result.bundle.sample_call}</pre>
              </div>
            ) : null}

            {activeTab === 'verify' ? (
              <div className="form-card mt-16">
                <h3>Verify in console</h3>
                <div className="form-helper-text">
                  Look for one new event under this tenant and agent, then follow the same session into approvals or execution if the action is gated.
                </div>
                <ol className="onboarding-checklist">
                  {result.bundle.verification_checklist.map(item => <li key={item}>{item}</li>)}
                </ol>
                <div className="btn-group mt-16">
                  {result.bundle.verification_links.map(link => (
                    <Link key={link.path} to={link.path} className="btn btn-outline btn-sm">
                      {link.label}
                    </Link>
                  ))}
                </div>
                {result.bundle.notes.length > 0 ? (
                  <ul className="onboarding-checklist mt-16">
                    {result.bundle.notes.map(item => <li key={item}>{item}</li>)}
                  </ul>
                ) : null}
              </div>
            ) : null}

	            <div className="row-actions row-actions-end mt-16">
	              <button className="btn btn-outline" type="button" onClick={() => void handleDownloadBundle()} disabled={downloading}>
	                {downloading ? 'Downloading…' : 'Download result bundle'}
	              </button>
	              <button className="btn btn-outline" type="button" onClick={resetResult}>
	                Back to form
	              </button>
              {result.mode === 'preview' ? (
                <button className="btn btn-primary" type="button" onClick={() => void handleCreate()} disabled={creating}>
                  {creating ? 'Creating…' : 'Create agent and API key'}
                </button>
              ) : null}
            </div>
          </div>
        ) : (
          <>
            <div className="form-card">
              <h3>1. Choose integration type</h3>
              <div className="form-helper-text mb-16">
                Start with the runtime that already owns tool execution. Python and TypeScript are the strongest golden paths when you control the tool loop directly.
              </div>
              <div className="onboarding-choice-grid">
                {runtimeOptions.map(option => (
                  <label key={option.value} className={`choice-card ${runtime === option.value ? 'is-selected' : ''}`}>
                    <input
                      type="radio"
                      name="runtime"
                      value={option.value}
                      checked={runtime === option.value}
                      onChange={() => setRuntime(option.value)}
                    />
                    <span className="choice-card-title">{option.label}</span>
                    <span className="choice-card-body">{option.description}</span>
                  </label>
                ))}
              </div>
            </div>

            <div className="form-card mt-16">
              <h3>2. Choose tenant</h3>
              {presetTenant ? (
                <div className="detail-panel">
                  <div className="detail-row" style={{ borderBottom: 'none', paddingBottom: 0 }}>
                    <div className="detail-label">Using current tenant</div>
                    <div className="detail-value">
                      <strong>{presetTenant.name}</strong>
                      <div className="table-subtext mono">{presetTenant.id}</div>
                      <div className="form-helper-text">Tenant is fixed because you launched onboarding from an existing tenant context. Runtime, tool, and posture drafts are still preserved while this page stays open.</div>
                    </div>
                  </div>
                </div>
              ) : (
                <>
                  <div className="toggle-stack">
                    <label className="toggle-field toggle-field-boxed">
                      <input type="radio" name="tenant-mode" checked={tenantMode === 'existing'} onChange={() => setTenantMode('existing')} />
                      <span>Select existing tenant</span>
                    </label>
                    <label className="toggle-field toggle-field-boxed">
                      <input type="radio" name="tenant-mode" checked={tenantMode === 'create'} onChange={() => setTenantMode('create')} />
                      <span>Create tenant inline</span>
                    </label>
                  </div>

                  {tenantMode === 'existing' ? (
                    <div className="form-group mt-16">
                      <label htmlFor="onboarding-tenant-id">Tenant</label>
                      <select id="onboarding-tenant-id" value={effectiveTenantID} onChange={e => setTenantID(e.target.value)}>
                        <option value="">Select a tenant</option>
                        {tenantOptions.map(option => (
                          <option key={option.id} value={option.id}>{option.name}</option>
                        ))}
                      </select>
                      <div className="form-helper-text">Preview and create both work here because the tenant already exists.</div>
                    </div>
                  ) : (
                    <div className="form-group mt-16">
                      <label htmlFor="onboarding-new-tenant-name">New tenant name</label>
                      <input id="onboarding-new-tenant-name" value={newTenantName} onChange={e => setNewTenantName(e.target.value)} placeholder="e.g., Demo Corp" />
                      <div className="form-helper-text">Creating a new tenant is only applied when you submit the real integration.</div>
                    </div>
                  )}
                  <div className="form-helper-text mt-16">Runtime, selected tools, and approval posture stay prefilled while this onboarding modal remains open on the page.</div>
                </>
              )}
            </div>

            <div className="form-card mt-16">
              <h3>{isRegenerateFlow ? '3. Use existing agent identity' : '3. Create agent identity'}</h3>
              <div className="form-helper-text mb-16">
                {isRegenerateFlow
                  ? 'This bundle targets a persisted agent. You can adjust runtime/tool metadata without recreating the agent.'
                  : 'Agent name is user-provided. Agent id is generated for preview and minted for real on create.'}
              </div>
              <div className="form-grid form-grid-2">
                <div className="form-group">
                  <label htmlFor="onboarding-agent-name">Agent name</label>
                  <input id="onboarding-agent-name" value={agentName} onChange={e => setAgentName(e.target.value)} placeholder="e.g., Support Bot" disabled={isRegenerateFlow} />
                  {isRegenerateFlow ? <div className="form-helper-text">Persisted agent id: <code className="mono">{presetAgent?.id}</code></div> : null}
                </div>
                <div className="form-group">
                  <label htmlFor="onboarding-env-label">Environment label</label>
                  <input id="onboarding-env-label" value={environmentLabel} onChange={e => setEnvironmentLabel(e.target.value)} placeholder="dev" />
                </div>
                <div className="form-group">
                  <label htmlFor="onboarding-owner-name">Owner or team</label>
                  <input id="onboarding-owner-name" value={ownerName} onChange={e => setOwnerName(e.target.value)} placeholder="AI Platform" />
                </div>
                <div className="form-group">
                  <label htmlFor="onboarding-description">Description</label>
                  <input id="onboarding-description" value={description} onChange={e => setDescription(e.target.value)} placeholder="Optional integration note" />
                </div>
              </div>
            </div>

            <div className="form-card mt-16">
              <h3>4. Choose first governed tools</h3>
              <div className="form-helper-text mb-16">
                Pick one safe read path and one write or approval path when possible. The bundle stays disabled until at least one governed tool is selected.
              </div>
              {loadingCatalog ? (
                <div className="loading">Loading connector catalog…</div>
              ) : curatedOptions.length === 0 ? (
                <div className="form-helper-text">
                  No curated golden-path tools are currently available in the connector catalog.
                  Connect one of the supported pilot actions first, or expect defaults regeneration to stay unavailable until the catalog exposes a compatible action.
                </div>
              ) : (
                <div className="onboarding-choice-grid">
                  {curatedOptions.map(option => (
                    <label key={`${option.tool}:${option.action}`} className={`choice-card ${isToolSelected(option.tool, option.action) ? 'is-selected' : ''}`}>
                      <input
                        type="checkbox"
                        checked={isToolSelected(option.tool, option.action)}
                        onChange={() => toggleTool(option.tool, option.action)}
                      />
                      <span className="choice-card-title">{option.title}</span>
                      <span className="choice-card-body">
                        <code className="mono">{option.tool}.{option.action}</code>
                        <br />
                        {option.hint}
                      </span>
                    </label>
                  ))}
                </div>
              )}
              {!loadingCatalog && curatedOptions.length > 0 && !hasToolSelection ? (
                <div className="form-helper-text mt-16">Select at least one tool before previewing, creating, or regenerating a bundle.</div>
              ) : null}
            </div>

            <div className="form-card mt-16">
              <h3>5. Choose approval posture</h3>
              <div className="form-helper-text mb-16">
                This changes the starter guidance only. Your real tenant policy still decides whether the final call is allowed, denied, or sent to approval.
              </div>
              <div className="onboarding-choice-grid">
                {approvalPostureOptions.map(option => (
                  <label key={option.value} className={`choice-card ${approvalPosture === option.value ? 'is-selected' : ''}`}>
                    <input
                      type="radio"
                      name="approval-posture"
                      checked={approvalPosture === option.value}
                      onChange={() => setApprovalPosture(option.value)}
                    />
                    <span className="choice-card-title">{option.label}</span>
                    <span className="choice-card-body">{option.description}</span>
                  </label>
                ))}
              </div>
            </div>

            {!canPreview ? <div className="form-helper-text mt-16">{previewBlockedReason}</div> : null}
            {!isRegenerateFlow ? <div className="form-helper-text mt-16">{previewActionHint}</div> : null}
            {isRegenerateFlow ? (
              <>
                <div className="form-helper-text mt-8">{regenerateActionHint}</div>
                <div className="form-helper-text mt-8">{regenerateDefaultsHint}</div>
              </>
            ) : (
              <div className="form-helper-text mt-8">{createActionHint}</div>
            )}

            <div className="row-actions row-actions-end mt-16">
              <button className="btn btn-outline" type="button" onClick={onClose}>
                Cancel
              </button>
              {isRegenerateFlow ? (
                <>
                  <button className="btn btn-outline" type="button" onClick={() => void handleRegenerateDefaults()} disabled={!canSubmitRegenerateDefaults}>
                    {creating ? 'Regenerating…' : 'Regenerate with defaults'}
                  </button>
                  <button className="btn btn-primary" type="button" onClick={() => void handleRegenerate()} disabled={!canSubmitRegenerate}>
                    {creating ? 'Regenerating…' : 'Regenerate bundle'}
                  </button>
                </>
              ) : (
                <>
                  <button className="btn btn-outline" type="button" onClick={() => void handlePreview()} disabled={!canSubmitPreview}>
                    {previewing ? 'Previewing…' : 'Preview bundle'}
                  </button>
                  <button className="btn btn-primary" type="button" onClick={() => void handleCreate()} disabled={!canSubmitCreate}>
                    {creating ? 'Creating…' : 'Create agent and API key'}
                  </button>
                </>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  )
}

export type { AgentOption, TenantOption, OnboardingResponse }
