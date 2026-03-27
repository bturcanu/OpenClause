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

type ResultTab = 'copy_env' | 'first_call' | 'verify' | 'files' | 'history'

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
  advancedOpen: boolean
  useRecommendedTools: boolean
  useRecommendedPosture: boolean
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
  { id: 'copy_env', label: '1. Copy env' },
  { id: 'first_call', label: '2. Run first call' },
  { id: 'verify', label: '3. Verify' },
  { id: 'files', label: 'Files' },
  { id: 'history', label: 'History' },
]

const starterPackCandidates: Record<RuntimeOption['value'], SelectedTool[][]> = {
  python: [
    [{ tool: 'slack', action: 'slack.channel.list' }, { tool: 'slack', action: 'slack.msg.post' }],
    [{ tool: 'postgres', action: 'query.readonly' }, { tool: 'github', action: 'issue.create' }],
    [{ tool: 'jira', action: 'jira.issue.list' }, { tool: 'jira', action: 'jira.issue.create' }],
  ],
  typescript: [
    [{ tool: 'slack', action: 'slack.channel.list' }, { tool: 'slack', action: 'slack.msg.post' }],
    [{ tool: 'webhook', action: 'post' }, { tool: 'email', action: 'send' }],
    [{ tool: 'postgres', action: 'query.readonly' }, { tool: 'github', action: 'issue.create' }],
  ],
  langchain: [
    [{ tool: 'jira', action: 'jira.issue.list' }, { tool: 'jira', action: 'jira.issue.create' }],
    [{ tool: 'slack', action: 'slack.channel.list' }, { tool: 'slack', action: 'slack.msg.post' }],
    [{ tool: 'postgres', action: 'query.readonly' }, { tool: 'github', action: 'issue.create' }],
  ],
  openai_local: [
    [{ tool: 'postgres', action: 'query.readonly' }, { tool: 'github', action: 'issue.create' }],
    [{ tool: 'postgres', action: 'query.readonly' }, { tool: 'slack', action: 'slack.msg.post' }],
    [{ tool: 'github', action: 'issue.create' }],
  ],
}

const toolDisplayNames = new Map(curatedToolOrder.map(option => [`${option.tool}:${option.action}`, option.title]))

function formatToolSelection(selection: SelectedTool) {
  return `${selection.tool}:${selection.action}`
}

function approvalPostureLabel(value: ApprovalPostureOption['value'] | string) {
  return approvalPostureOptions.find(option => option.value === value)?.label || value
}

function recommendedApprovalPosture(runtime: RuntimeOption['value']) {
  return runtime === 'openai_local' ? 'read_only_first' : 'pilot_safe'
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
        title: 'Review only: nothing has been created yet.',
        body: 'This shows the starter files and placeholder credentials before you connect the real agent.',
      }
    case 'regenerated':
      return {
        title: 'Last setup rebuilt for this agent.',
        body: result.api_key?.key_prefix
          ? 'OpenClause reused the tenant, agent, and key reference, then rebuilt the starter files from the saved setup.'
          : 'OpenClause reused the tenant and agent, but there is still no active API key to reference. Create or rotate one before handing this setup off.',
      }
    case 'regenerated_defaults':
      return {
        title: 'Safe starter rebuilt for this agent.',
        body: result.api_key?.key_prefix
          ? 'OpenClause applied the current safe runtime, starter tools, and approval posture so you can hand off a cleaner first-run setup.'
          : 'OpenClause applied the current safe runtime, starter tools, and approval posture, but you still need to create or rotate an API key first.',
      }
    default:
      return {
        title: 'Agent connected successfully.',
        body: 'The tenant context, agent, and API key now exist. OpenClause also saved the runtime, starter tools, and approval posture so you can rebuild this setup later.',
      }
  }
}

function toolKey(selection: SelectedTool) {
  return `${selection.tool}:${selection.action}`
}

function sameToolSelections(left: SelectedTool[], right: SelectedTool[]) {
  if (left.length !== right.length) return false
  const leftKeys = left.map(toolKey).sort()
  const rightKeys = right.map(toolKey).sort()
  return leftKeys.every((value, index) => value === rightKeys[index])
}

function recommendedToolsForRuntime(runtime: RuntimeOption['value'], connectors: ConnectorInfo[]) {
  const byTool = new Map(connectors.map(connector => [connector.name, new Set(connector.actions || [])]))
  const candidatePacks = starterPackCandidates[runtime] || []
  for (const candidate of candidatePacks) {
    const resolved = candidate.filter(item => byTool.get(item.tool)?.has(item.action))
    if (resolved.length > 0 && resolved.length === candidate.length) {
      return resolved
    }
  }
  return curatedToolOrder
    .filter(item => byTool.get(item.tool)?.has(item.action))
    .slice(0, Math.min(2, curatedToolOrder.length))
    .map(item => ({ tool: item.tool, action: item.action }))
}

function summarizeStarterPack(tools: SelectedTool[]) {
  if (tools.length === 0) return 'No governed starter tools available yet'
  return tools.map(tool => toolDisplayNames.get(toolKey(tool)) || formatToolSelection(tool)).join(' · ')
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
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [useRecommendedTools, setUseRecommendedTools] = useState(true)
  const [useRecommendedPosture, setUseRecommendedPosture] = useState(true)
  const [loadingCatalog, setLoadingCatalog] = useState(false)
  const [previewing, setPreviewing] = useState(false)
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')
  const [result, setResult] = useState<OnboardingResponse | null>(null)
  const [activeTab, setActiveTab] = useState<ResultTab>('copy_env')
  const [selectedArtifactID, setSelectedArtifactID] = useState('')
  const [downloading, setDownloading] = useState(false)
  const isRegenerateFlow = !!presetAgent
  const draftKey = `${presetTenant?.id || 'shared'}:${presetAgent?.id || 'new'}`

  useEffect(() => {
    if (!open) return
    const savedDraft = draftStoreRef.current[draftKey]
    const nextRuntime = savedDraft?.runtime || supportedRuntimeOrDefault(presetAgent?.onboarding?.runtime)
    const nextApprovalPosture = savedDraft?.approvalPosture
      || supportedApprovalPostureOrDefault(presetAgent?.onboarding?.approval_posture)
      || recommendedApprovalPosture(nextRuntime)
    setError('')
    setResult(null)
    setActiveTab('copy_env')
    setSelectedArtifactID('')
    setDownloading(false)
    setRuntime(nextRuntime)
    setTenantMode(savedDraft?.tenantMode || (presetTenant ? 'existing' : (tenantOptions.length > 0 ? 'existing' : 'create')))
    setTenantID(savedDraft?.tenantID || presetTenant?.id || tenantOptions[0]?.id || '')
    setNewTenantName(savedDraft?.newTenantName || '')
    setAgentName(presetAgent?.name || savedDraft?.agentName || '')
    setEnvironmentLabel(savedDraft?.environmentLabel || presetAgent?.onboarding?.environment_label || 'dev')
    setOwnerName(savedDraft?.ownerName || presetAgent?.onboarding?.owner_name || '')
    setDescription(savedDraft?.description || presetAgent?.onboarding?.description || '')
    setApprovalPosture(nextApprovalPosture)
    setSelectedTools(savedDraft?.selectedTools || presetAgent?.onboarding?.tools || [])
    setAdvancedOpen(savedDraft?.advancedOpen || false)
    setUseRecommendedTools(savedDraft?.useRecommendedTools ?? !presetAgent?.onboarding?.tools?.length)
    setUseRecommendedPosture(savedDraft?.useRecommendedPosture ?? !presetAgent?.onboarding?.approval_posture)
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
          const defaults = recommendedToolsForRuntime(runtime, nextConnectors)
          if (defaults.length > 0) {
            setSelectedTools(current => {
              const byTool = new Map(nextConnectors.map(connector => [connector.name, new Set(connector.actions || [])]))
              const validCurrent = current.filter(item => byTool.get(item.tool)?.has(item.action))
              if (validCurrent.length > 0 && !useRecommendedTools) return validCurrent
              return defaults
            })
          } else {
            const byTool = new Map(nextConnectors.map(connector => [connector.name, new Set(connector.actions || [])]))
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
  }, [open, runtime, useRecommendedTools])

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
      advancedOpen,
      useRecommendedTools,
      useRecommendedPosture,
    }
  }, [draftKey, open, runtime, tenantMode, tenantID, newTenantName, agentName, environmentLabel, ownerName, description, approvalPosture, selectedTools, advancedOpen, useRecommendedTools, useRecommendedPosture])

  const curatedOptions = useMemo(() => {
    const byTool = new Map(connectors.map(connector => [connector.name, new Set(connector.actions || [])]))
    return curatedToolOrder.filter(item => byTool.get(item.tool)?.has(item.action))
  }, [connectors])

  useEffect(() => {
    if (!open) return
    if (useRecommendedPosture) {
      setApprovalPosture(recommendedApprovalPosture(runtime))
    }
  }, [open, runtime, useRecommendedPosture])

  useEffect(() => {
    if (!open || !useRecommendedTools || connectors.length === 0) return
    const nextTools = recommendedToolsForRuntime(runtime, connectors)
    setSelectedTools(current => sameToolSelections(current, nextTools) ? current : nextTools)
  }, [connectors, open, runtime, useRecommendedTools])

  const effectiveTenantID = presetTenant?.id || tenantID || tenantOptions[0]?.id || ''
  const trimmedAgentName = agentName.trim()
  const trimmedNewTenantName = newTenantName.trim()
  const hasToolSelection = selectedTools.length > 0
  const starterPackSummary = summarizeStarterPack(selectedTools)
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
    setUseRecommendedTools(false)
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
      setActiveTab('copy_env')
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
      setActiveTab('copy_env')
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
      setActiveTab('copy_env')
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
      setActiveTab('copy_env')
    } catch (err: any) {
      setError(err?.message || 'Failed to regenerate onboarding bundle with defaults')
    } finally {
      setCreating(false)
    }
  }

  function resetResult() {
    setResult(null)
    setActiveTab('copy_env')
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
        return 'Review starter files'
      case 'regenerated':
        return 'Rebuilt last setup'
      case 'regenerated_defaults':
        return 'Rebuilt from safe defaults'
      default:
        return 'Connected agent'
    }
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal onboarding-modal" onClick={event => event.stopPropagation()}>
        <div className="flex-between mb-16">
          <div>
            <h3>{isRegenerateFlow ? 'Rebuild Agent Setup' : 'Connect Agent'}</h3>
            <p className="table-subtext">
              {isRegenerateFlow
                ? 'Reuse the saved setup for this agent and rebuild starter files without minting a new raw API key.'
                : 'Get one working governed call first. OpenClause will create the agent, issue the API key, and hand you starter files in one flow.'}
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
                <strong>Rebuild is non-destructive.</strong> It reuses the saved tenant, agent, and key reference. Rebuilding never reissues a raw API key.
              </>
            ) : (
              <>
                <strong>Fast path:</strong> choose a runtime, name the agent, keep the recommended starter pack, then connect it. Review files first only if you want to inspect the output before creating anything.
              </>
            )}
          </div>
        </div>

        {isRegenerateFlow && presetAgent?.onboarding ? (
          <div className="form-helper-text mb-16">
            Starting from the last saved setup for this agent{presetAgent.onboarding.updated_at ? ` (updated ${presetAgent.onboarding.updated_at})` : ''}. Open Advanced Setup only if you want to change the runtime, starter tools, or posture before rebuilding.
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
                    ? 'Review the starter files, then connect the real agent when you are ready to mint credentials.'
                    : result.mode === 'created'
                      ? 'Copy the environment, run the first call, and verify the first event and session before sharing the setup.'
                      : result.api_key?.key_prefix
                        ? `Copy the environment, point OPENCLAUSE_API_KEY at the active key matching ${result.api_key.key_prefix}, and rerun the first call.`
                        : 'Create or rotate an API key from Tenant Detail -> API Keys, then rebuild this setup again before handing it off.'}
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
                  This full key is only returned during connect. Copy it now or download the starter files before leaving this result because it will not be shown again.
              </div>
              <pre className="code-block">{result.api_key.raw_key}</pre>
            </div>
          ) : null}

            {(result.mode === 'regenerated' || result.mode === 'regenerated_defaults') && !result.api_key ? (
              <div className="banner-note banner-note-compact mt-16">
              <div>
                  <strong>Action required before the first call.</strong> This tenant has no active API key to reuse yet. Create or rotate one from the tenant API Keys tab, then rebuild this setup again.
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

	            {activeTab === 'copy_env' ? (
	              <div className="form-card mt-16">
                <h3>Copy env</h3>
                <div className="form-helper-text">
                  {result.mode === 'regenerated'
                    ? 'These exports reference an existing API key variable because raw keys are only shown at connect time.'
                    : result.mode === 'regenerated_defaults'
                      ? 'This setup was rebuilt from explicit safe defaults. Review the assumed runtime, starter tools, and posture before handing it off.'
                    : result.mode === 'preview'
                      ? 'Review mode uses placeholder API key values and does not create credentials.'
                      : 'Use these exports for quick local testing. This connect result includes the one-time raw key shown above.'}
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
                <h3>Extra files</h3>
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

            {activeTab === 'first_call' ? (
              <div className="form-card mt-16">
                <h3>Run first call</h3>
                <div className="form-helper-text">Use this first-run call once you have the generated environment loaded. Success looks like one event, one session, and an approval record only when the selected action is gated.</div>
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

            {activeTab === 'history' ? (
              <div className="form-card mt-16">
                <h3>Rebuild later</h3>
                <div className="form-helper-text">
                  {result.mode === 'preview'
                    ? 'Preview does not save anything yet. Connect the agent first if you want OpenClause to remember this setup for later rebuilds.'
                    : 'Open Tenant Detail to rebuild the last working setup, rebuild from safe defaults, or download the latest saved files again later.'}
                </div>
                <div className="btn-group mt-16">
                  <Link to={`/tenants/${result.tenant.id}?tab=agents`} className="btn btn-outline btn-sm">
                    Open tenant agents
                  </Link>
                  {result.mode !== 'preview' ? (
                    <Link to={`/tenants/${result.tenant.id}?tab=agents`} className="btn btn-outline btn-sm">
                      Open saved setup
                    </Link>
                  ) : null}
                </div>
              </div>
            ) : null}

	            <div className="row-actions row-actions-end mt-16">
	              <button className="btn btn-outline" type="button" onClick={() => void handleDownloadBundle()} disabled={downloading}>
	                {downloading ? 'Downloading…' : 'Download starter files'}
	              </button>
	              <button className="btn btn-outline" type="button" onClick={resetResult}>
	                Adjust setup
	              </button>
              {result.mode === 'preview' ? (
                <button className="btn btn-primary" type="button" onClick={() => void handleCreate()} disabled={creating}>
                  {creating ? 'Connecting…' : 'Connect agent'}
                </button>
              ) : null}
            </div>
          </div>
        ) : (
          <>
            <div className="form-card">
              <h3>{isRegenerateFlow ? 'Last working setup' : '1. Choose runtime'}</h3>
              <div className="form-helper-text mb-16">
                {isRegenerateFlow
                  ? 'Keep the saved runtime and starter pack unless you know you need to change them. Advanced Setup lets you change the runtime, starter tools, posture, and metadata.'
                  : 'Start with the runtime that already owns tool execution. Python and TypeScript are still the simplest first-run paths when you control the tool loop directly.'}
              </div>
              {isRegenerateFlow ? (
                <div className="detail-panel">
                  <div className="detail-row" style={{ borderBottom: 'none', paddingBottom: 0 }}>
                    <div className="detail-label">Current runtime</div>
                    <div className="detail-value">
                      <strong>{runtimeOptions.find(option => option.value === runtime)?.label || runtime}</strong>
                      <div className="form-helper-text">Open Advanced Setup if you want to switch runtimes before rebuilding.</div>
                    </div>
                  </div>
                </div>
              ) : (
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
              )}
            </div>

            <div className="form-card mt-16">
              <h3>{isRegenerateFlow ? 'Tenant' : '2. Choose tenant'}</h3>
              {presetTenant ? (
                <div className="detail-panel">
                  <div className="detail-row" style={{ borderBottom: 'none', paddingBottom: 0 }}>
                    <div className="detail-label">Using current tenant</div>
                    <div className="detail-value">
                      <strong>{presetTenant.name}</strong>
                      <div className="table-subtext mono">{presetTenant.id}</div>
                      <div className="form-helper-text">Tenant is fixed because you launched setup from an existing tenant context.</div>
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
                      <div className="form-helper-text">Use an existing tenant when you want the fastest path to one working governed call.</div>
                    </div>
                  ) : (
                    <div className="form-group mt-16">
                      <label htmlFor="onboarding-new-tenant-name">New tenant name</label>
                      <input id="onboarding-new-tenant-name" value={newTenantName} onChange={e => setNewTenantName(e.target.value)} placeholder="e.g., Demo Corp" />
                      <div className="form-helper-text">The tenant is only created when you connect the real agent.</div>
                    </div>
                  )}
                  <div className="form-helper-text mt-16">Your runtime, starter tools, and posture stay prefilled while this setup window remains open.</div>
                </>
              )}
            </div>

            <div className="form-card mt-16">
              <h3>{isRegenerateFlow ? 'Agent' : '3. Name agent'}</h3>
              <div className="form-helper-text mb-16">
                {isRegenerateFlow
                  ? 'This setup targets a persisted agent. Rebuilding does not recreate the agent.'
                  : 'The agent name is the main thing a teammate will recognize later when they rebuild or download this setup again.'}
              </div>
              <div className="form-grid form-grid-2">
                <div className="form-group">
                  <label htmlFor="onboarding-agent-name">Agent name</label>
                  <input id="onboarding-agent-name" value={agentName} onChange={e => setAgentName(e.target.value)} placeholder="e.g., Support Bot" disabled={isRegenerateFlow} />
                  {isRegenerateFlow ? <div className="form-helper-text">Persisted agent id: <code className="mono">{presetAgent?.id}</code></div> : null}
                </div>
              </div>
            </div>

            <div className="form-card mt-16">
              <h3>{isRegenerateFlow ? 'Starter plan' : '4. Pick starter plan'}</h3>
              <div className="form-helper-text mb-16">
                {isRegenerateFlow
                  ? 'Rebuild uses the saved starter pack by default. Open Advanced Setup only if you want to customize the tools or approval posture first.'
                  : 'Start with the recommended pack: one safe read path and one write or approval path when possible. You can customize it later.'}
              </div>
              {loadingCatalog ? (
                <div className="loading">Loading connector catalog…</div>
              ) : curatedOptions.length === 0 ? (
                <div className="form-helper-text">
                  No recommended starter pack is available yet because the live connector catalog does not expose a compatible pilot action.
                </div>
              ) : (
                <div className="detail-panel">
                  <div className="detail-row">
                    <div className="detail-label">Starter tools</div>
                    <div className="detail-value">
                      <strong>{starterPackSummary}</strong>
                      <div className="form-helper-text mt-8">
                        {useRecommendedTools
                          ? 'Open Advanced Setup if you want to replace the recommended starter pack.'
                          : 'You are using a custom starter pack from Advanced Setup.'}
                      </div>
                    </div>
                  </div>
                  <div className="detail-row" style={{ borderBottom: 'none', paddingBottom: 0 }}>
                    <div className="detail-label">Approval posture</div>
                    <div className="detail-value">
                      <strong>{approvalPostureLabel(approvalPosture)}</strong>
                      <div className="form-helper-text mt-8">
                        {useRecommendedPosture
                          ? 'This is the recommended starting posture for this runtime.'
                          : 'You are using a custom posture from Advanced Setup.'}
                      </div>
                    </div>
                  </div>
                </div>
              )}
              {!loadingCatalog && curatedOptions.length > 0 && !hasToolSelection ? (
                <div className="form-helper-text mt-16">Pick at least one governed tool in Advanced Setup before continuing.</div>
              ) : null}
            </div>

            <div className="form-card mt-16">
              <button className="btn btn-outline btn-sm" type="button" onClick={() => setAdvancedOpen(openValue => !openValue)}>
                {advancedOpen ? 'Hide Advanced Setup' : 'Open Advanced Setup'}
              </button>
              <div className="form-helper-text mt-8">
                Use this only if you want to customize the starter tools, approval posture, metadata, or review files before connecting the agent.
              </div>

              {advancedOpen ? (
                <>
                  <div className="form-grid form-grid-2 mt-16">
                    <div className="form-group">
                      <label htmlFor="onboarding-env-label">Environment label</label>
                      <input id="onboarding-env-label" value={environmentLabel} onChange={e => setEnvironmentLabel(e.target.value)} placeholder="dev" />
                    </div>
                    <div className="form-group">
                      <label htmlFor="onboarding-owner-name">Owner or team</label>
                      <input id="onboarding-owner-name" value={ownerName} onChange={e => setOwnerName(e.target.value)} placeholder="AI Platform" />
                    </div>
                    <div className="form-group" style={{ gridColumn: '1 / -1' }}>
                      <label htmlFor="onboarding-description">Description</label>
                      <input id="onboarding-description" value={description} onChange={e => setDescription(e.target.value)} placeholder="Optional integration note" />
                    </div>
                  </div>

                  {isRegenerateFlow ? (
                    <div className="form-card mt-16">
                      <h3>Change runtime</h3>
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
                  ) : null}

                  <div className="form-card mt-16">
                    <div className="flex-between">
                      <h3>Starter tools</h3>
                      {curatedOptions.length > 0 ? (
                        <button
                          className="btn btn-outline btn-sm"
                          type="button"
                          onClick={() => {
                            setUseRecommendedTools(true)
                            setSelectedTools(recommendedToolsForRuntime(runtime, connectors))
                          }}
                        >
                          Reset to recommended
                        </button>
                      ) : null}
                    </div>
                    <div className="form-helper-text mb-16">
                      Pick at least one governed tool. The primary path stays disabled until this list has at least one valid action.
                    </div>
                    {loadingCatalog ? (
                      <div className="loading">Loading connector catalog…</div>
                    ) : curatedOptions.length === 0 ? (
                      <div className="form-helper-text">
                        No curated starter actions are currently available in the connector catalog.
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
                  </div>

                  <div className="form-card mt-16">
                    <div className="flex-between">
                      <h3>Approval posture</h3>
                      <button
                        className="btn btn-outline btn-sm"
                        type="button"
                        onClick={() => {
                          setUseRecommendedPosture(true)
                          setApprovalPosture(recommendedApprovalPosture(runtime))
                        }}
                      >
                        Reset to recommended
                      </button>
                    </div>
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
                            onChange={() => {
                              setUseRecommendedPosture(false)
                              setApprovalPosture(option.value)
                            }}
                          />
                          <span className="choice-card-title">{option.label}</span>
                          <span className="choice-card-body">{option.description}</span>
                        </label>
                      ))}
                    </div>
                  </div>

                  {!isRegenerateFlow ? (
                    <div className="form-card mt-16">
                      <div className="flex-between">
                        <div>
                          <h3>Review files first</h3>
                          <div className="form-helper-text mt-8">{previewActionHint}</div>
                        </div>
                        <button className="btn btn-outline" type="button" onClick={() => void handlePreview()} disabled={!canSubmitPreview}>
                          {previewing ? 'Reviewing…' : 'Review starter files'}
                        </button>
                      </div>
                    </div>
                  ) : null}
                </>
              ) : null}
            </div>

            <div className="row-actions row-actions-end mt-16">
              <button className="btn btn-outline" type="button" onClick={onClose}>
                Cancel
              </button>
              {isRegenerateFlow ? (
                <>
                  <button className="btn btn-outline" type="button" onClick={() => void handleRegenerateDefaults()} disabled={!canSubmitRegenerateDefaults}>
                    {creating ? 'Rebuilding…' : 'Rebuild from safe defaults'}
                  </button>
                  <button className="btn btn-primary" type="button" onClick={() => void handleRegenerate()} disabled={!canSubmitRegenerate}>
                    {creating ? 'Rebuilding…' : 'Rebuild last setup'}
                  </button>
                </>
              ) : (
                <button className="btn btn-primary" type="button" onClick={() => void handleCreate()} disabled={!canSubmitCreate}>
                  {creating ? 'Connecting…' : 'Connect agent'}
                </button>
              )}
            </div>
            {!canPreview ? <div className="form-helper-text mt-16">{previewBlockedReason}</div> : null}
            {isRegenerateFlow ? (
              <>
                <div className="form-helper-text mt-8">{regenerateActionHint}</div>
                <div className="form-helper-text mt-8">{regenerateDefaultsHint}</div>
              </>
            ) : (
              <div className="form-helper-text mt-8">{createActionHint}</div>
            )}
          </>
        )}
      </div>
    </div>
  )
}

export type { AgentOption, TenantOption, OnboardingResponse }
