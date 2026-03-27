package onboarding

import (
	"fmt"
	"sort"
	"strings"
)

type Runtime string

const (
	RuntimePython      Runtime = "python"
	RuntimeTypeScript  Runtime = "typescript"
	RuntimeLangChain   Runtime = "langchain"
	RuntimeOpenAILocal Runtime = "openai_local"
)

type SelectedTool struct {
	Tool   string `json:"tool"`
	Action string `json:"action"`
}

type BundleRequest struct {
	BaseURL          string
	TenantID         string
	TenantName       string
	AgentID          string
	AgentName        string
	APIKey           string
	APIKeyMode       APIKeyMode
	APIKeyPrefix     string
	Runtime          Runtime
	ApprovalPosture  string
	EnvironmentLabel string
	OwnerName        string
	Description      string
	Tools            []SelectedTool
}

type APIKeyMode string

const (
	APIKeyModeRawProvided    APIKeyMode = "raw_provided"
	APIKeyModePreview        APIKeyMode = "preview_placeholder"
	APIKeyModeExistingKeyRef APIKeyMode = "existing_key_reference"
)

type BundleArtifact struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	FileName   string `json:"file_name"`
	PathHint   string `json:"path_hint"`
	Kind       string `json:"kind"`
	Purpose    string `json:"purpose"`
	Writable   bool   `json:"writable"`
	Executable bool   `json:"executable,omitempty"`
	Language   string `json:"language,omitempty"`
	Content    string `json:"content"`
}

type VerificationLink struct {
	Label       string `json:"label"`
	Path        string `json:"path"`
	Description string `json:"description,omitempty"`
}

type Bundle struct {
	Title                 string             `json:"title"`
	Summary               string             `json:"summary"`
	Runtime               string             `json:"runtime"`
	RuntimeLabel          string             `json:"runtime_label"`
	StarterFileName       string             `json:"starter_file_name"`
	Environment           map[string]string  `json:"environment"`
	EnvironmentScript     string             `json:"environment_script"`
	EnvironmentFile       string             `json:"environment_file"`
	StarterSnippet        string             `json:"starter_snippet"`
	ReadmeSnippet         string             `json:"readme_snippet"`
	SampleCall            string             `json:"sample_call"`
	Artifacts             []BundleArtifact   `json:"artifacts"`
	VerificationChecklist []string           `json:"verification_checklist"`
	VerificationLinks     []VerificationLink `json:"verification_links"`
	AppliedDefaults       []BundleDefault    `json:"applied_defaults,omitempty"`
	Notes                 []string           `json:"notes"`
}

func BuildBundle(req BundleRequest) (*Bundle, error) {
	if strings.TrimSpace(req.BaseURL) == "" {
		return nil, fmt.Errorf("base URL required")
	}
	if strings.TrimSpace(req.TenantID) == "" {
		return nil, fmt.Errorf("tenant ID required")
	}
	if strings.TrimSpace(req.AgentID) == "" {
		return nil, fmt.Errorf("agent ID required")
	}
	if strings.TrimSpace(req.APIKey) == "" {
		return nil, fmt.Errorf("API key required")
	}
	if strings.TrimSpace(req.AgentName) == "" {
		return nil, fmt.Errorf("agent name required")
	}
	if len(req.Tools) == 0 {
		return nil, fmt.Errorf("at least one governed tool is required")
	}
	for index, tool := range req.Tools {
		if strings.TrimSpace(tool.Tool) == "" {
			return nil, fmt.Errorf("governed tool %d is missing a tool name", index+1)
		}
		if strings.TrimSpace(tool.Action) == "" {
			return nil, fmt.Errorf("governed tool %d is missing an action", index+1)
		}
	}

	env := map[string]string{
		"OPENCLAUSE_BASE_URL":  strings.TrimRight(req.BaseURL, "/"),
		"OPENCLAUSE_TENANT_ID": req.TenantID,
		"OPENCLAUSE_AGENT_ID":  req.AgentID,
		"OPENCLAUSE_API_KEY":   req.APIKey,
	}

	order := []string{
		"OPENCLAUSE_BASE_URL",
		"OPENCLAUSE_TENANT_ID",
		"OPENCLAUSE_AGENT_ID",
		"OPENCLAUSE_API_KEY",
	}

	if req.Runtime == RuntimeOpenAILocal {
		env["OPENCLAUSE_BRIDGE_URL"] = "http://127.0.0.1:8787"
		env["LOCAL_MODEL_BASE_URL"] = "http://localhost:1234/v1"
		env["LOCAL_MODEL_NAME"] = "replace-with-lmstudio-model-id"
		env["LOCAL_MODEL_API_KEY"] = "not-needed"
		order = append(order,
			"OPENCLAUSE_BRIDGE_URL",
			"LOCAL_MODEL_BASE_URL",
			"LOCAL_MODEL_NAME",
			"LOCAL_MODEL_API_KEY",
		)
	}

	primaryTool := req.Tools[0]
	environmentScript := shellExports(env, order)
	environmentFile := dotenvExports(env, order)
	sample := sampleCall(env, primaryTool, req.ApprovalPosture)
	checklist := verificationChecklist(primaryTool)
	notes := notesFor(req)
	readme := readmeSnippet(req)

	bundle := &Bundle{
		Title:                 fmt.Sprintf("%s onboarding bundle", runtimeLabel(req.Runtime)),
		Summary:               bundleSummary(req),
		Runtime:               string(req.Runtime),
		Environment:           env,
		EnvironmentScript:     environmentScript,
		EnvironmentFile:       environmentFile,
		SampleCall:            sample,
		VerificationChecklist: checklist,
		VerificationLinks:     verificationLinks(req),
		Notes:                 notes,
		ReadmeSnippet:         readme,
	}

	var starter string
	extraArtifacts := []BundleArtifact{}
	switch req.Runtime {
	case RuntimePython:
		bundle.RuntimeLabel = "Python SDK wrapper"
		bundle.StarterFileName = "agent.py"
		starter = pythonSnippet(req)
	case RuntimeTypeScript:
		bundle.RuntimeLabel = "TypeScript SDK wrapper"
		bundle.StarterFileName = "agent.ts"
		starter = typescriptSnippet(req)
		extraArtifacts = append(extraArtifacts, BundleArtifact{
			ID:       "package-snippet",
			Label:    "Package snippet",
			FileName: "package.onboarding.json",
			PathHint: "package.onboarding.json",
			Kind:     "package_snippet",
			Purpose:  "Optional package.json fragment for the TypeScript golden path.",
			Writable: true,
			Language: "json",
			Content:  typescriptPackageSnippet(),
		})
	case RuntimeLangChain:
		bundle.RuntimeLabel = "LangChain"
		bundle.StarterFileName = "langchain_agent.py"
		starter = langchainSnippet(req)
	case RuntimeOpenAILocal:
		bundle.RuntimeLabel = "Local OpenAI-compatible model"
		bundle.StarterFileName = "local_model_agent.py"
		starter = openAILocalSnippet(req)
		extraArtifacts = append(extraArtifacts, BundleArtifact{
			ID:       "bridge-config",
			Label:    "Local bridge config",
			FileName: "openclause-bridge.yaml",
			PathHint: "openclause-bridge.yaml",
			Kind:     "bridge_config",
			Purpose:  "One-time local bridge config so your runtime can target a local OpenClause proxy instead of embedding tenant, agent, and API-key wiring by hand.",
			Writable: true,
			Language: "yaml",
			Content:  bridgeConfigYAML(req),
		}, BundleArtifact{
			ID:         "bridge-launcher",
			Label:      "Start bridge helper",
			FileName:   "start-bridge.sh",
			PathHint:   "start-bridge.sh",
			Kind:       "bridge_launcher",
			Purpose:    "Recommended one-command way to load env and start the local OpenClause bridge for LM Studio and local chat clients.",
			Writable:   true,
			Executable: true,
			Language:   "bash",
			Content:    bridgeStartScript(),
		}, BundleArtifact{
			ID:       "lmstudio-mcp-remote-snippet",
			Label:    "LM Studio MCP snippet (recommended)",
			FileName: "lmstudio.mcp.remote.example.jsonc",
			PathHint: "lmstudio.mcp.remote.example.jsonc",
			Kind:     "mcp_snippet",
			Purpose:  "Recommended LM Studio mcp.json snippet. Keep the local bridge running, then let LM Studio reach it over http://127.0.0.1:8787/mcp.",
			Writable: true,
			Language: "jsonc",
			Content:  lmStudioRemoteMCPSnippet(req),
		}, BundleArtifact{
			ID:       "lmstudio-mcp-snippet",
			Label:    "LM Studio MCP snippet (advanced stdio)",
			FileName: "lmstudio.mcp.example.jsonc",
			PathHint: "lmstudio.mcp.example.jsonc",
			Kind:     "mcp_snippet",
			Purpose:  "Advanced LM Studio mcp.json snippet for stdio MCP when you want LM Studio to launch OpenClause directly instead of keeping the bridge running yourself.",
			Writable: true,
			Language: "jsonc",
			Content:  lmStudioMCPSnippet(req),
		})
	default:
		return nil, fmt.Errorf("unsupported runtime %q", req.Runtime)
	}

	bundle.StarterSnippet = starter
	bundle.Artifacts = []BundleArtifact{
		{
			ID:         "env-script",
			Label:      "Environment shell exports",
			FileName:   "setup-env.sh",
			PathHint:   "setup-env.sh",
			Kind:       "environment_script",
			Purpose:    "Export OpenClause environment variables for local shells.",
			Writable:   true,
			Executable: true,
			Language:   "bash",
			Content:    environmentScript,
		},
		{
			ID:       "env-file",
			Label:    "Environment file example",
			FileName: ".env.example",
			PathHint: ".env.example",
			Kind:     "environment_file",
			Purpose:  "Environment file template for local services and workers.",
			Writable: true,
			Language: "dotenv",
			Content:  environmentFile,
		},
		{
			ID:       "starter",
			Label:    "Starter runtime file",
			FileName: bundle.StarterFileName,
			PathHint: bundle.StarterFileName,
			Kind:     "starter_file",
			Purpose:  "Golden-path runtime starter showing governed tool execution.",
			Writable: true,
			Language: artifactLanguageForRuntime(req.Runtime),
			Content:  starter,
		},
		{
			ID:       "readme",
			Label:    "README snippet",
			FileName: "README.onboarding.md",
			PathHint: "README.onboarding.md",
			Kind:     "readme",
			Purpose:  "Human-readable setup and verification guidance for the integration.",
			Writable: true,
			Language: "markdown",
			Content:  readme,
		},
		{
			ID:         "sample-call",
			Label:      "Smoke test call",
			FileName:   "smoke-test.sh",
			PathHint:   "smoke-test.sh",
			Kind:       "sample_call",
			Purpose:    "First-run governed tool call you can execute after loading env vars.",
			Writable:   true,
			Executable: true,
			Language:   "bash",
			Content:    sample,
		},
	}
	bundle.Artifacts = append(bundle.Artifacts, extraArtifacts...)

	return bundle, nil
}

func artifactLanguageForRuntime(runtime Runtime) string {
	switch runtime {
	case RuntimeTypeScript:
		return "typescript"
	case RuntimePython, RuntimeLangChain, RuntimeOpenAILocal:
		return "python"
	default:
		return "text"
	}
}

func shellExports(env map[string]string, order []string) string {
	lines := make([]string, 0, len(order))
	for _, key := range order {
		if preserveManualEnvOverride(key) {
			lines = append(lines, fmt.Sprintf("export %s=\"${%s:-%s}\"", key, key, env[key]))
			continue
		}
		lines = append(lines, fmt.Sprintf("export %s=\"%s\"", key, env[key]))
	}
	return strings.Join(lines, "\n")
}

func preserveManualEnvOverride(key string) bool {
	return strings.HasPrefix(key, "LOCAL_MODEL_") || key == "OPENCLAUSE_BRIDGE_URL"
}

func dotenvExports(env map[string]string, order []string) string {
	lines := make([]string, 0, len(order))
	for _, key := range order {
		lines = append(lines, fmt.Sprintf("%s=\"%s\"", key, env[key]))
	}
	return strings.Join(lines, "\n")
}

func sampleCall(env map[string]string, tool SelectedTool, approvalPosture string) string {
	riskScore := defaultRiskScore(tool, approvalPosture)
	return fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

# Load environment first: source ./setup-env.sh
# Success looks like a JSON payload with event_id and decision.
# Then open Audit Trail and Sessions for the same tenant + agent and confirm the generated session_id.
# If the decision comes back as "approve", use the generated Approvals link before retrying your runtime path.
for required_var in OPENCLAUSE_BASE_URL OPENCLAUSE_API_KEY OPENCLAUSE_TENANT_ID OPENCLAUSE_AGENT_ID; do
  if [ -z "${!required_var:-}" ]; then
    echo "missing required environment variable: $required_var" >&2
    exit 1
  fi
done

random_id() {
  if command -v uuidgen >/dev/null 2>&1; then
    uuidgen
    return 0
  fi
  if command -v python3 >/dev/null 2>&1; then
    python3 - <<'PY'
import uuid
print(uuid.uuid4())
PY
    return 0
  fi
  printf 'oc-%%s-%%s\n' "$(date +%%s)" "${RANDOM:-0}"
}

trace_id="trace-$(random_id)"
idempotency_key="$(random_id)"

payload="$(cat <<JSON
{
  "tenant_id": "%s",
  "agent_id": "%s",
  "tool": "%s",
  "action": "%s",
  "params": %s,
  "risk_score": %d,
  "user_id": "pilot-user",
  "session_id": "demo-session-$(date +%%s)",
  "trace_id": "$trace_id",
  "idempotency_key": "$idempotency_key"
}
JSON
)"

curl -fsS "$OPENCLAUSE_BASE_URL/v1/toolcalls" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $OPENCLAUSE_API_KEY" \
  -d "$payload"`, env["OPENCLAUSE_TENANT_ID"], env["OPENCLAUSE_AGENT_ID"], tool.Tool, tool.Action, sampleParamsLiteral(tool), riskScore)
}

func defaultRiskScore(tool SelectedTool, approvalPosture string) int {
	if tool.Tool == "postgres" && tool.Action == "query.readonly" {
		return 2
	}
	if approvalPosture == "pilot_safe" {
		return 4
	}
	return 2
}

func sampleParamsLiteral(tool SelectedTool) string {
	switch {
	case tool.Tool == "postgres" && tool.Action == "query.readonly":
		return `{"sql": "select id, name, email, created_at from demo_users order by created_at desc limit 3", "params": []}`
	default:
		return `{"example": true}`
	}
}

func openAILocalUserPrompt(tool SelectedTool) string {
	switch {
	case tool.Tool == "postgres" && tool.Action == "query.readonly":
		return "Use the governed action to fetch the newest 3 demo users with id, name, email, and created_at."
	default:
		return "Run the governed action with example=true"
	}
}

func verificationChecklist(tool SelectedTool) []string {
	return []string{
		fmt.Sprintf("Send one governed %s.%s call with the generated smoke payload.", tool.Tool, tool.Action),
		"Look for one new event in Audit Trail under the expected tenant and agent.",
		"Open Sessions and confirm the same generated session_id appears there for the same tenant and agent.",
		"If the action routes to approval, open Approvals, approve it, and verify execution appears in the same session chain.",
	}
}

func verificationLinks(req BundleRequest) []VerificationLink {
	query := func(parts map[string]string) string {
		items := make([]string, 0, len(parts))
		for key, value := range parts {
			if strings.TrimSpace(value) == "" {
				continue
			}
			items = append(items, fmt.Sprintf("%s=%s", key, value))
		}
		sort.Strings(items)
		if len(items) == 0 {
			return ""
		}
		return "?" + strings.Join(items, "&")
	}

	return []VerificationLink{
		{
			Label:       "Open Audit Trail",
			Path:        "/events" + query(map[string]string{"tenant_id": req.TenantID, "agent_id": req.AgentID}),
			Description: "Audit Trail already supports tenant_id and agent_id query filters.",
		},
		{
			Label:       "Open Sessions",
			Path:        "/sessions" + query(map[string]string{"tenant_id": req.TenantID, "agent_id": req.AgentID}),
			Description: "Sessions narrows the run list to this tenant and agent.",
		},
		{
			Label:       "Open Approvals",
			Path:        "/approvals" + query(map[string]string{"tenant_id": req.TenantID}),
			Description: "Use this when your pilot routes writes or risky actions through approval.",
		},
	}
}

func notesFor(req BundleRequest) []string {
	notes := []string{
		"Generated starter bundle for the v0.5 golden path. Treat it as a working starting point, not a full framework abstraction.",
		"Always send stable session_id, trace_id, and idempotency_key values for governed calls.",
		"Starter snippets mark the values you should replace before routing production traffic through OpenClause.",
	}
	switch req.APIKeyMode {
	case APIKeyModeRawProvided:
		notes = append(notes, "This bundle contains the one-time raw API key returned during create. Save the generated env artifacts now because the full key will not be shown again.")
	case APIKeyModePreview:
		notes = append(notes, "Preview bundles use a placeholder API key value. The real raw key is only shown when you create an integration.")
	case APIKeyModeExistingKeyRef:
		if strings.TrimSpace(req.APIKeyPrefix) != "" {
			notes = append(notes, fmt.Sprintf("Raw API keys are only shown at creation time. Reuse an active key matching prefix %s or rotate one from the tenant API Keys tab.", strings.TrimSpace(req.APIKeyPrefix)))
		} else {
			notes = append(notes, "Raw API keys are only shown at creation time. Reuse an active tenant API key or rotate one from the tenant API Keys tab before running the sample call.")
		}
	}
	if strings.TrimSpace(req.EnvironmentLabel) != "" {
		notes = append(notes, fmt.Sprintf("Environment label recorded for this bundle: %s.", strings.TrimSpace(req.EnvironmentLabel)))
	}
	if strings.TrimSpace(req.OwnerName) != "" {
		notes = append(notes, fmt.Sprintf("Bundle owner/team hint: %s.", strings.TrimSpace(req.OwnerName)))
	}
	if strings.TrimSpace(req.Description) != "" {
		notes = append(notes, fmt.Sprintf("Description carried into onboarding notes: %s.", strings.TrimSpace(req.Description)))
	}
	return notes
}

func bundleSummary(req BundleRequest) string {
	return fmt.Sprintf("Tenant %s · Agent %s · %s", req.TenantID, req.AgentID, runtimeLabel(req.Runtime))
}

func readmeSnippet(req BundleRequest) string {
	toolList := toolComments(req.Tools)
	return fmt.Sprintf(
		"# OpenClause %s onboarding\n\n"+
			"This bundle is generated for tenant %q and agent %q.\n\n"+
			"## What this bundle represents\n\n"+
			"%s\n\n"+
			"## Runtime setup\n\n"+
			"%s\n\n"+
			"## Quick start\n\n"+
			"%s\n\n"+
			"## Approval handling\n\n"+
			"%s\n\n"+
			"## Run context fields you should keep stable\n\n"+
			"- `session_id`: groups related governed calls into one operator-visible run.\n"+
			"- `trace_id`: ties a single request path together across your runtime, OpenClause, and connector logs.\n"+
			"- `idempotency_key`: protects retries and approval resume paths from duplicate execution.\n\n"+
			"## Verification workflow\n\n"+
			"1. Send one call, see one event, see one session.\n"+
			"2. Confirm Audit Trail shows the new event under this tenant and agent.\n"+
			"3. Confirm Sessions shows the same `session_id`.\n"+
			"4. If the action is gated, approve it and confirm execution lands in the same session chain.\n\n"+
			"## First governed tools\n\n"+
			"%s\n\n"+
			"## What to customize next\n\n"+
			"%s\n"+
			"- keep the generated verification links handy until your first governed traffic is visible in Audit Trail and Sessions\n",
		runtimeLabel(req.Runtime),
		req.TenantName,
		req.AgentName,
		credentialModeSummary(req),
		runtimeSetupInstructions(req.Runtime),
		quickStartBody(req),
		approvalHandlingGuidance(req.Runtime, req.ApprovalPosture),
		toolList,
		runtimeCustomizationChecklist(req.Runtime),
	)
}

func runtimeSetupInstructions(runtime Runtime) string {
	switch runtime {
	case RuntimePython:
		return strings.Join([]string{
			"- Create or reuse a virtualenv for the service that will own the tool-execution loop.",
			"- Install the OpenClause Python SDK in that environment before running `agent.py`.",
			"- Keep the generated `governed_call(...)` helper close to your existing tool dispatch path so approvals and retries stay explicit.",
		}, "\n")
	case RuntimeTypeScript:
		return strings.Join([]string{
			"- Merge `package.onboarding.json` into the worker or service that already owns tool execution.",
			"- Run `npm install` and make sure `tsx` or your preferred TypeScript runner is available locally.",
			"- Keep the generated `governedCall(...)` helper near the point where your runtime already chooses tools and risk scores.",
		}, "\n")
	case RuntimeLangChain:
		return strings.Join([]string{
			"- Install the OpenClause Python SDK and the LangChain helper package used by your runtime.",
			"- Start by wrapping one read-like tool and one write-like tool before widening the agent toolset.",
			"- Keep tenant, agent, and tool definitions fixed at construction time; pass real params and requester context per call.",
		}, "\n")
	case RuntimeOpenAILocal:
		return strings.Join([]string{
			"- Create or reuse a virtualenv, then run `python -m pip install --upgrade pip setuptools wheel`.",
			"- Install the local-model dependencies with `python -m pip install openai && python -m pip install --no-build-isolation -e ../sdk/python`.",
			"- Start LM Studio's OpenAI-compatible server, run `curl http://localhost:1234/v1/models`, and copy the returned model id into `LOCAL_MODEL_NAME`.",
			"- Start the local bridge with `./start-bridge.sh`. That is the recommended one-command path because it loads env for you and exposes both the governed tool proxy and an OpenAI-compatible chat host on the same local endpoint.",
			"- For native LM Studio chat-UI tool use, copy `lmstudio.mcp.remote.example.jsonc` into LM Studio's `mcp.json`. That remote MCP path is the recommended default because it is the least fragile once the bridge is already running.",
			"- Keep `lmstudio.mcp.example.jsonc` as the advanced fallback only when you explicitly want LM Studio to launch OpenClause itself over stdio; that path still needs the absolute repo/config paths replaced.",
			"- Replace `LOCAL_MODEL_BASE_URL` only if LM Studio or your local model server is exposed somewhere else.",
			"- Point your OpenAI-compatible chat client at `OPENCLAUSE_BRIDGE_URL/v1` for a more normal chat workflow; the bridge will inject the governed tool surface and route tool calls through OpenClause.",
		}, "\n")
	default:
		return "- Review the generated starter file and adapt it to your runtime before running production traffic through OpenClause."
	}
}

func runtimeSmokeCommand(runtime Runtime) string {
	switch runtime {
	case RuntimeTypeScript:
		return "npx tsx agent.ts"
	case RuntimeLangChain:
		return "python langchain_agent.py"
	case RuntimeOpenAILocal:
		return "python local_model_agent.py"
	case RuntimePython:
		fallthrough
	default:
		return "python agent.py"
	}
}

func quickStartBody(req BundleRequest) string {
	if req.Runtime == RuntimeOpenAILocal {
		return strings.Join([]string{
			"1. Load environment with `source ./setup-env.sh` or copy values from `.env.example`.",
			"2. Start the bridge with `./start-bridge.sh`.",
			"3. Recommended path: copy `lmstudio.mcp.remote.example.jsonc` into LM Studio's `mcp.json`, then talk to your model normally.",
			"4. Alternative paths if you want them:",
			"   - `python local_model_agent.py --smoke` for one governed smoke call",
			"   - `python local_model_agent.py` for an interactive bridge-backed chat session",
			"   - `openclause bridge chat --config ./openclause-bridge.yaml` for a CLI REPL without editing starter code",
			"   - `lmstudio.mcp.example.jsonc` only if you want LM Studio to launch OpenClause itself over stdio",
			"5. Use the verification links in the console to confirm the event, session, and approval behavior landed correctly.",
		}, "\n")
	}

	lines := []string{
		"1. Load environment with `source ./setup-env.sh` or copy values from `.env.example`.",
		fmt.Sprintf("2. Open `%s` and replace the example params, requester identity, and runtime-specific placeholders you do not want to keep.", starterFileName(req.Runtime)),
	}
	if req.Runtime == RuntimeTypeScript {
		lines = append(lines, "2a. Merge `package.onboarding.json` into your local `package.json`, run `npm install`, and use `npm run onboarding:smoke` if you want a first-run helper script.")
	}
	lines = append(lines,
		fmt.Sprintf("3. Run `%s` for a runtime-level smoke check, or use `./smoke-test.sh` for a raw HTTP sanity check.", runtimeSmokeCommand(req.Runtime)),
		"4. Use the verification links in the console to confirm the event, session, and approval behavior landed correctly.",
	)
	return strings.Join(lines, "\n")
}

func approvalHandlingGuidance(runtime Runtime, posture string) string {
	waitGuidance := "Treat `approve` as a real branch in your runtime. The generated starter waits inline so you can see the full approval cycle before deciding whether to hand that work to an async worker."
	switch runtime {
	case RuntimeLangChain:
		waitGuidance = "Start with one read-style tool and one write-style tool. Keep the approval-capable tool visible to operators and avoid burying approval waits inside generic agent retries until your pilot data is stable."
	case RuntimeOpenAILocal:
		waitGuidance = "Let the local model propose arguments, then send those arguments through the governed tool bridge. If the decision comes back as `approve`, wait for operator action before handing the result back to the model."
	}
	return fmt.Sprintf("%s %s", approvalPostureSummary(posture), waitGuidance)
}

func credentialModeSummary(req BundleRequest) string {
	switch req.APIKeyMode {
	case APIKeyModeRawProvided:
		return "This bundle contains the one-time raw API key returned during create. Download or copy the env artifacts now because the full key will not be shown again."
	case APIKeyModePreview:
		return "This is a preview bundle. The API key values are placeholders only, and no credentials were created."
	case APIKeyModeExistingKeyRef:
		if strings.TrimSpace(req.APIKeyPrefix) != "" {
			return fmt.Sprintf("This bundle reuses existing tenant + agent context. No raw key was reissued. Reuse an active key matching prefix %s or rotate one from Tenant Detail -> API Keys.", strings.TrimSpace(req.APIKeyPrefix))
		}
		return "This bundle reuses existing tenant + agent context. No raw key was reissued, and no active key reference is currently available. Create or rotate a key before running the smoke test."
	default:
		return "This bundle reflects the current onboarding context without introducing a new persistence model."
	}
}

func runtimeCustomizationChecklist(runtime Runtime) string {
	base := []string{
		"- replace the example params with real tool inputs",
		"- wire your runtime's own user/session identifiers into the generated helper",
	}
	switch runtime {
	case RuntimeTypeScript:
		base = append(base,
			"- merge the package snippet into the worker that already owns your OpenAI, job, or queue dependencies",
			"- decide whether `waitForApproval(...)` should block inline or dispatch follow-up work to a queue consumer",
		)
	case RuntimeLangChain:
		base = append(base,
			"- swap the sample governed tools for the real LangChain tools you want to pilot first",
			"- keep agent prompts and tool descriptions honest about approvals so the model does not assume writes always execute immediately",
		)
	case RuntimeOpenAILocal:
		base = append(base,
			"- use `./start-bridge.sh` plus `lmstudio.mcp.remote.example.jsonc` as the default LM Studio handoff",
			"- use `python local_model_agent.py` or `openclause bridge chat --config ./openclause-bridge.yaml` for a normal chat loop instead of editing the file for every prompt",
			"- treat `lmstudio.mcp.example.jsonc` as the advanced stdio fallback and `lmstudio.mcp.remote.example.jsonc` as the normal LM Studio path",
			"- treat `openclause.governed_results` as optional bridge metadata when the model mixes governed and client tool calls in one turn; the generated Python starter prints that metadata for you",
			"- replace the sample smoke prompt with your real domain prompt or point your own chat client at the local bridge",
			"- keep your system prompt explicit that governed tool execution may return a deny or pending-approval result instead of an immediate write",
			"- update `openclause-bridge.yaml` if you want to change the configured tool allowlist, default user id, or session prefix without editing runtime code",
		)
	default:
		base = append(base,
			"- decide whether approval waits should block inline or hand off to an async worker in your app",
		)
	}
	return strings.Join(base, "\n")
}

func runtimeLabel(runtime Runtime) string {
	switch runtime {
	case RuntimePython:
		return "Python SDK wrapper"
	case RuntimeTypeScript:
		return "TypeScript SDK wrapper"
	case RuntimeLangChain:
		return "LangChain"
	case RuntimeOpenAILocal:
		return "Local OpenAI-compatible model"
	default:
		return "OpenClause"
	}
}

func starterFileName(runtime Runtime) string {
	switch runtime {
	case RuntimePython:
		return "agent.py"
	case RuntimeTypeScript:
		return "agent.ts"
	case RuntimeLangChain:
		return "langchain_agent.py"
	case RuntimeOpenAILocal:
		return "local_model_agent.py"
	default:
		return "agent.txt"
	}
}

func approvalPostureSummary(posture string) string {
	switch strings.TrimSpace(posture) {
	case "pilot_safe":
		return "Pilot-safe mode assumes read traffic can flow quickly while higher-risk writes may pause for approval."
	case "read_only_first":
		return "Reads-first mode is tuned for lower-risk exploratory traffic before you add write approvals."
	case "tenant_default":
		return "Tenant-default mode avoids extra assumptions and lets the tenant policy decide how requests behave."
	default:
		return "No explicit approval posture hint was selected for this bundle."
	}
}

func toolComments(tools []SelectedTool) string {
	lines := make([]string, 0, len(tools))
	for _, tool := range tools {
		lines = append(lines, fmt.Sprintf("- `%s.%s`", tool.Tool, tool.Action))
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func pythonSnippet(req BundleRequest) string {
	tool := req.Tools[0]
	return fmt.Sprintf(`import json
import os
import uuid

from openclause import OpenClauseClient
from openclause.models import ToolCallRequest


client = OpenClauseClient(
    base_url=os.environ["OPENCLAUSE_BASE_URL"],
    api_key=os.environ["OPENCLAUSE_API_KEY"],
)

TENANT_ID = os.environ["OPENCLAUSE_TENANT_ID"]
AGENT_ID = os.environ["OPENCLAUSE_AGENT_ID"]


def request_context() -> tuple[str, str, str]:
    session_id = f"%s-session-{uuid.uuid4().hex[:8]}"
    trace_id = str(uuid.uuid4())
    idempotency_key = str(uuid.uuid4())
    return session_id, trace_id, idempotency_key


def wait_for_decision(response):
    if response.decision != "approve":
        return response
    print("waiting_for_approval=", response.event_id)
    return client.wait_for_approval(response.event_id)


def governed_call(tool: str, action: str, params: dict, risk_score: int = 2) -> dict:
    session_id, trace_id, idempotency_key = request_context()
    request = ToolCallRequest(
        tenant_id=TENANT_ID,
        agent_id=AGENT_ID,
        tool=tool,
        action=action,
        params=params,
        risk_score=int(risk_score),
        user_id="pilot-user",
        session_id=session_id,
        trace_id=trace_id,
        idempotency_key=idempotency_key,
    )
    response = client.submit_tool_call(request)
    print(
        "event_id=", response.event_id,
        "decision=", response.decision,
        "session_id=", session_id,
        "trace_id=", trace_id,
    )

    response = wait_for_decision(response)
    if response.decision == "deny":
        return {"event_id": response.event_id, "decision": response.decision, "reason": response.reason}

    return {
        "event_id": response.event_id,
        "decision": response.decision,
        "result": response.result.output_json if response.result else None,
    }


if __name__ == "__main__":
    result = governed_call(
        tool="%s",
        action="%s",
        params={"example": True},
        risk_score=4,
    )
    print(json.dumps(result, indent=2))

# Replace before production use:
# - "pilot-user" with your real user or requester id
# - the example params payload with your real tool inputs
# - the session/trace/idempotency strategy if your runtime already has one
# - the inline wait_for_decision(...) path if approvals should resume asynchronously in your app
#
# Suggested first governed tools:
# %s
`, sessionPrefix(req), tool.Tool, tool.Action, toolComments(req.Tools))
}

func typescriptSnippet(req BundleRequest) string {
	tool := req.Tools[0]
	return fmt.Sprintf(`import { randomUUID } from "node:crypto"
import { OpenClauseClient } from "openclause"

const client = new OpenClauseClient({
  baseUrl: process.env.OPENCLAUSE_BASE_URL!,
  apiKey: process.env.OPENCLAUSE_API_KEY!,
})

const tenantId = process.env.OPENCLAUSE_TENANT_ID!
const agentId = process.env.OPENCLAUSE_AGENT_ID!

function requestContext() {
  return {
    sessionId: "%s-session-" + randomUUID().slice(0, 8),
    traceId: randomUUID(),
    idempotencyKey: OpenClauseClient.generateIdempotencyKey(),
  }
}

async function settleDecision(response: Awaited<ReturnType<typeof client.submitToolCall>>) {
  if (response.decision !== "approve") return response
  console.log("waiting_for_approval=", response.event_id)
  return client.waitForApproval(response.event_id, { timeoutMs: 120_000, pollIntervalMs: 2_000 })
}

async function governedCall(tool: string, action: string, params: Record<string, unknown>, riskScore = 2) {
  if (!Number.isInteger(riskScore)) {
    throw new Error("riskScore must stay an integer before sending it to OpenClause")
  }

  const { sessionId, traceId, idempotencyKey } = requestContext()
  let response = await client.submitToolCall({
    tenant_id: tenantId,
    agent_id: agentId,
    tool,
    action,
    params,
    risk_score: riskScore,
    user_id: "pilot-user",
    session_id: sessionId,
    trace_id: traceId,
    idempotency_key: idempotencyKey,
  })

  console.log("event_id=", response.event_id, "decision=", response.decision, "session_id=", sessionId, "trace_id=", traceId)

  response = await settleDecision(response)

  if (response.decision === "deny") {
    return { eventId: response.event_id, decision: response.decision, reason: response.reason }
  }

  return {
    eventId: response.event_id,
    decision: response.decision,
    result: response.result?.output_json ?? null,
  }
}

void governedCall("%s", "%s", { example: true }, 4).then(result => {
  console.log(JSON.stringify(result, null, 2))
})

// Replace before production use:
// - "pilot-user" with your real requester identity
// - the example params payload with your real tool inputs
// - the requestContext strategy if your worker already has stable session/trace ids
// - the inline settleDecision(...) path if approvals should resume in a queue worker
//
// Suggested first governed tools:
// %s
`, sessionPrefix(req), tool.Tool, tool.Action, toolComments(req.Tools))
}

func typescriptPackageSnippet() string {
	return `{
  "scripts": {
    "onboarding:smoke": "tsx agent.ts",
    "onboarding:verify": "bash ./smoke-test.sh"
  },
  "dependencies": {
    "openclause": "latest"
  },
  "devDependencies": {
    "@types/node": "^22.10.0",
    "tsx": "^4.20.0",
    "typescript": "^5.7.0"
  }
}`
}

func langchainSnippet(req BundleRequest) string {
	readTool := req.Tools[0]
	writeTool := req.Tools[0]
	if len(req.Tools) > 1 {
		writeTool = req.Tools[1]
	}
	return fmt.Sprintf(`import json
import os

from openclause import OpenClauseClient
from openclause.langchain import OpenClauseTool


client = OpenClauseClient(
    base_url=os.environ["OPENCLAUSE_BASE_URL"],
    api_key=os.environ["OPENCLAUSE_API_KEY"],
)

TENANT_ID = os.environ["OPENCLAUSE_TENANT_ID"]
AGENT_ID = os.environ["OPENCLAUSE_AGENT_ID"]

def build_tools():
    return [
        OpenClauseTool(
            client=client,
            tool_name="%s",
            action="%s",
            tenant_id=TENANT_ID,
            agent_id=AGENT_ID,
            description="Safe pilot read through OpenClause",
        ),
        OpenClauseTool(
            client=client,
            tool_name="%s",
            action="%s",
            tenant_id=TENANT_ID,
            agent_id=AGENT_ID,
            description="Higher-risk action that may require approval",
        ),
    ]

if __name__ == "__main__":
    tools = build_tools()
    read_result = tools[0]._run({"example": True})
    print(json.dumps({"read_result": read_result}, indent=2))
    # Uncomment the write-path tool once an approver is ready:
    # write_result = tools[1]._run({"example": True})
    # print(json.dumps({"write_result": write_result}, indent=2))

# Fixed at construction time: tenant_id, agent_id, tool_name, action.
# Per-call: params payload and any runtime context your LangChain integration already threads through.
# Replace the example payload before production use and make sure your runtime preserves stable session_id/trace_id/idempotency_key values.
`, readTool.Tool, readTool.Action, writeTool.Tool, writeTool.Action)
}

func openAILocalSnippet(req BundleRequest) string {
	return fmt.Sprintf(`import json
import argparse
import os

from openai import APIConnectionError, OpenAI


bridge_base_url = os.environ.get("OPENCLAUSE_BRIDGE_URL", "http://127.0.0.1:8787").rstrip("/")
chat = OpenAI(
    base_url=bridge_base_url + "/v1",
    api_key=os.environ.get("LOCAL_MODEL_API_KEY", "not-needed"),
)


def assistant_text(message) -> str:
    content = message.content
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        parts = []
        for item in content:
            if isinstance(item, dict) and item.get("type") == "text":
                parts.append(item.get("text", ""))
            elif hasattr(item, "text") and item.text:
                parts.append(item.text)
        return "".join(parts)
    return json.dumps(content)


def extract_governed_results(completion) -> list[dict]:
    raw = completion.model_dump() if hasattr(completion, "model_dump") else completion
    if not isinstance(raw, dict):
        return []
    extension = raw.get("openclause")
    if not isinstance(extension, dict):
        return []
    results = extension.get("governed_results")
    if not isinstance(results, list):
        return []
    return [item for item in results if isinstance(item, dict)]


def request_completion(model: str, messages: list[dict]) -> tuple[str, str, list[dict]]:
    try:
        completion = chat.chat.completions.create(model=model, messages=messages)
    except APIConnectionError as exc:
        raise SystemExit(
            f"Could not reach the local OpenClause bridge at {bridge_base_url}/v1. "
            "Start it with ./start-bridge.sh from this bundle directory, "
            "or use an installed openclause binary if you prefer."
        ) from exc
    message = completion.choices[0].message
    return completion.model, assistant_text(message), extract_governed_results(completion)


def interactive_loop(model: str, system_prompt: str) -> None:
    messages = []
    if system_prompt:
        messages.append({"role": "system", "content": system_prompt})

    print(f"Bridge chat ready at {bridge_base_url}/v1 using model {model}.")
    print("Type a normal prompt, or use /help, /reset, and /exit.")
    while True:
        try:
            prompt = input("You> ").strip()
        except EOFError:
            print("")
            return

        if not prompt:
            continue
        if prompt in {"/exit", "/quit"}:
            print("Bye.")
            return
        if prompt == "/help":
            print("Type a normal prompt to send it through the bridge. Use /reset to clear the conversation.")
            continue
        if prompt == "/reset":
            messages = [{"role": "system", "content": system_prompt}] if system_prompt else []
            print("Conversation reset.")
            continue

        next_messages = messages + [{"role": "user", "content": prompt}]
        model_name, text, governed_results = request_completion(model, next_messages)
        print(f"Assistant> {text}")
        if governed_results:
            print("OpenClause governed results>")
            print(json.dumps(governed_results, indent=2))
        messages = next_messages + [{"role": "assistant", "content": text}]


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Bridge-backed local OpenAI-compatible chat smoke client")
    parser.add_argument("--prompt", help="One-shot user prompt to send through the local bridge")
    parser.add_argument("--system", default="", help="Optional extra system prompt for this client session")
    parser.add_argument("--smoke", action="store_true", help="Run the generated smoke prompt instead of entering interactive mode")
    args = parser.parse_args()

    model = os.environ.get("LOCAL_MODEL_NAME", "replace-with-lmstudio-model-id")
    prompt = args.prompt
    if args.smoke:
        prompt = %q

    if prompt:
        messages = []
        if args.system:
            messages.append({"role": "system", "content": args.system})
        messages.append({"role": "user", "content": prompt})
        model_name, text, governed_results = request_completion(model, messages)
        print(json.dumps({
            "model": model_name,
            "content": text,
            "governed_results": governed_results,
        }, indent=2))
    else:
        interactive_loop(model, args.system)

# Replace before production use:
# - LOCAL_MODEL_BASE_URL / LOCAL_MODEL_NAME if LM Studio or Ollama is exposed somewhere else
# - OPENCLAUSE_BRIDGE_URL only if you want the local bridge somewhere besides http://127.0.0.1:8787
# - mixed governed/client turns may include openclause.governed_results metadata; this starter prints it when present
# - use --smoke for the generated first call, --prompt for one-shot asks, or plain python local_model_agent.py for an interactive loop
# - openclause-bridge.yaml if you want to change which governed actions the model can call
`, openAILocalUserPrompt(req.Tools[0]))
}

func bridgeStartScript() string {
	return `#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$script_dir/setup-env.sh"

if command -v openclause >/dev/null 2>&1; then
  exec openclause bridge start --config "$script_dir/openclause-bridge.yaml"
fi

if [ -f "$script_dir/../go.mod" ] && [ -d "$script_dir/../cmd/openclause" ]; then
  exec go run "$script_dir/../cmd/openclause" bridge start --config "$script_dir/openclause-bridge.yaml"
fi

cat >&2 <<EOF
Could not find the OpenClause CLI.

Do one of the following, then retry:
  1. Install or build the 'openclause' binary so it is available on PATH
  2. Run this bundle from inside an OpenClause repo checkout so ../cmd/openclause exists
  3. Start the bridge manually with:
     go -C /absolute/path/to/OpenClause run ./cmd/openclause bridge start --config "$script_dir/openclause-bridge.yaml"
EOF
exit 1
`
}

func bridgeConfigYAML(req BundleRequest) string {
	lines := []string{
		fmt.Sprintf("listen: %q", "127.0.0.1:8787"),
		fmt.Sprintf("base_url: %q", strings.TrimRight(req.BaseURL, "/")),
		fmt.Sprintf("tenant_id: %q", req.TenantID),
		fmt.Sprintf("agent_id: %q", req.AgentID),
		"api_key: \"env:OPENCLAUSE_API_KEY\"",
		"",
		"defaults:",
		fmt.Sprintf("  user_id: %q", "pilot-user"),
		fmt.Sprintf("  session_prefix: %q", sessionPrefix(req)),
		fmt.Sprintf("  risk_mode: %q", "configured"),
		"",
		"openai:",
		fmt.Sprintf("  upstream_base_url: %q", "env:LOCAL_MODEL_BASE_URL"),
		fmt.Sprintf("  upstream_api_key: %q", "env:LOCAL_MODEL_API_KEY"),
		fmt.Sprintf("  model: %q", "env:LOCAL_MODEL_NAME"),
		fmt.Sprintf("  tool_name: %q", "governed_action"),
		fmt.Sprintf("  system_prompt: %q", bridgeSystemPrompt(req.Tools)),
		"",
		"tools:",
	}
	for _, tool := range req.Tools {
		lines = append(lines,
			fmt.Sprintf("  - tool: %q", tool.Tool),
			fmt.Sprintf("    action: %q", tool.Action),
			fmt.Sprintf("    risk_score: %d", defaultRiskScore(tool, req.ApprovalPosture)),
			fmt.Sprintf("    description: %q", bridgeToolDescription(tool)),
		)
	}
	return strings.Join(lines, "\n")
}

func lmStudioMCPSnippet(req BundleRequest) string {
	serverID := SuggestedAgentID(req.AgentName)
	return fmt.Sprintf(`{
  "mcpServers": {
    "openclause-%s": {
      "command": "/bin/zsh",
      "args": [
        "-lc",
        "export PATH=\"/opt/homebrew/bin:/usr/local/bin:$PATH\"; source /absolute/path/to/setup-env.sh; exec go -C /absolute/path/to/OpenClause run ./cmd/openclause bridge mcp --config /absolute/path/to/openclause-bridge.yaml"
      ]
    }
  }
}

// Advanced fallback:
// Replace both absolute paths:
// - /absolute/path/to/OpenClause -> the OpenClause repo root containing go.mod
// - /absolute/path/to/setup-env.sh and /absolute/path/to/openclause-bridge.yaml -> this bundle's files
//
// If you already built or installed the OpenClause CLI, you can simplify this to:
//   "command": "/bin/zsh"
//   "args": ["-lc", "source /absolute/path/to/setup-env.sh; exec /absolute/path/to/openclause bridge mcp --config /absolute/path/to/openclause-bridge.yaml"]
`, serverID)
}

func lmStudioRemoteMCPSnippet(req BundleRequest) string {
	serverID := SuggestedAgentID(req.AgentName)
	return fmt.Sprintf(`{
  "mcpServers": {
    "openclause-%s-remote": {
      "url": "http://127.0.0.1:8787/mcp"
    }
  }
}

// Recommended default:
// 1. Run ./start-bridge.sh from this bundle directory
// 2. Paste this into LM Studio's mcp.json
// 3. Chat normally in LM Studio
//
// Optional profile routing for multi-profile bridge configs:
// {
//   "mcpServers": {
//     "openclause-%s-remote": {
//       "url": "http://127.0.0.1:8787/mcp",
//       "headers": {
//         "X-OpenClause-Profile": "support"
//       }
//     }
//   }
// }
`, serverID, serverID)
}

func bridgeSystemPrompt(tools []SelectedTool) string {
	ops := make([]string, 0, len(tools))
	for _, tool := range tools {
		ops = append(ops, tool.Tool+"."+tool.Action)
	}
	sort.Strings(ops)
	return "Use governed_action whenever the user asks to work with one of these operations: " + strings.Join(ops, ", ") + ". If a tool result says decision=approve, explain that the action is awaiting operator approval."
}

func bridgeToolDescription(tool SelectedTool) string {
	switch {
	case tool.Tool == "postgres" && tool.Action == "query.readonly":
		return "Generated first pilot read action for local-model governance."
	default:
		return "Generated first pilot tool from onboarding."
	}
}

func sessionPrefix(req BundleRequest) string {
	base := strings.TrimSpace(req.EnvironmentLabel)
	if base == "" {
		base = req.AgentName
	}
	base = strings.ToLower(base)
	base = strings.ReplaceAll(base, " ", "-")
	base = strings.ReplaceAll(base, "_", "-")
	if base == "" {
		return "agent"
	}
	return base
}

func SuggestedAgentID(agentName string) string {
	base := strings.ToLower(strings.TrimSpace(agentName))
	base = strings.ReplaceAll(base, " ", "-")
	base = strings.ReplaceAll(base, "_", "-")
	base = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-':
			return r
		default:
			return -1
		}
	}, base)
	base = strings.Trim(base, "-")
	if base == "" {
		base = "agent"
	}
	return base
}

func PreviewAgentID(agentName string) string {
	return "preview-" + SuggestedAgentID(agentName)
}
