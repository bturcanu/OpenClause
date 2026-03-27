package onboarding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildBundleReturnsStructuredArtifactsAndVerificationLinks(t *testing.T) {
	bundle, err := BuildBundle(BundleRequest{
		BaseURL:          "http://localhost:8080",
		TenantID:         "tenant-1",
		TenantName:       "Alpha Corp",
		AgentID:          "agent-1",
		AgentName:        "Support Bot",
		APIKey:           "sk-oc-demo",
		APIKeyMode:       APIKeyModeRawProvided,
		APIKeyPrefix:     "sk-oc-de",
		Runtime:          RuntimePython,
		ApprovalPosture:  "pilot_safe",
		EnvironmentLabel: "dev",
		OwnerName:        "AI Platform",
		Description:      "Demo onboarding bundle",
		Tools: []SelectedTool{
			{Tool: "slack", Action: "slack.channel.list"},
			{Tool: "slack", Action: "slack.msg.post"},
		},
	})
	if err != nil {
		t.Fatalf("BuildBundle: %v", err)
	}

	if bundle.StarterFileName != "agent.py" {
		t.Fatalf("expected python starter file, got %q", bundle.StarterFileName)
	}
	if bundle.Title == "" || bundle.Summary == "" {
		t.Fatalf("expected title and summary, got %+v", bundle)
	}
	if len(bundle.Artifacts) != 5 {
		t.Fatalf("expected 5 artifacts, got %d", len(bundle.Artifacts))
	}
	if bundle.Artifacts[0].FileName != "setup-env.sh" {
		t.Fatalf("expected shell artifact first, got %+v", bundle.Artifacts[0])
	}
	if bundle.Artifacts[1].FileName != ".env.example" {
		t.Fatalf("expected dotenv artifact second, got %+v", bundle.Artifacts[1])
	}
	if bundle.Artifacts[2].FileName != "agent.py" {
		t.Fatalf("expected starter file artifact, got %+v", bundle.Artifacts[2])
	}
	if !bundle.Artifacts[0].Executable || !bundle.Artifacts[4].Executable {
		t.Fatalf("expected shell artifacts to be marked executable, got env=%+v sample=%+v", bundle.Artifacts[0], bundle.Artifacts[4])
	}
	if !bundle.Artifacts[2].Writable || bundle.Artifacts[2].Purpose == "" || bundle.Artifacts[2].PathHint == "" {
		t.Fatalf("expected cli-ready artifact metadata, got %+v", bundle.Artifacts[2])
	}
	if bundle.ReadmeSnippet == "" || bundle.EnvironmentFile == "" {
		t.Fatalf("expected readme and env file content, got %+v", bundle)
	}
	if bundle.SampleCall == "" || bundle.StarterSnippet == "" {
		t.Fatalf("expected smoke test and starter snippet, got %+v", bundle)
	}
	if !containsAll(bundle.ReadmeSnippet, []string{"What this bundle represents", "Runtime setup", "Approval handling", "Verification workflow", "session_id", "trace_id", "idempotency_key", "What to customize next"}) {
		t.Fatalf("expected production-ready onboarding guidance in README, got %s", bundle.ReadmeSnippet)
	}
	if !containsAll(bundle.StarterSnippet, []string{"request_context", "wait_for_decision", "waiting_for_approval", "pilot-user"}) {
		t.Fatalf("expected richer starter guidance in python snippet, got %s", bundle.StarterSnippet)
	}
	if !containsAll(bundle.SampleCall, []string{"Success looks like", "Audit Trail", "Sessions"}) {
		t.Fatalf("expected smoke test guidance, got %s", bundle.SampleCall)
	}
	if !containsAll(bundle.SampleCall, []string{"#!/usr/bin/env bash", "set -euo pipefail", "random_id()", "missing required environment variable", `"session_id": "demo-session-$(date +%s)"`, `"trace_id": "$trace_id"`, `curl -fsS "$OPENCLAUSE_BASE_URL/v1/toolcalls"`, `-d "$payload"`}) {
		t.Fatalf("expected smoke test payload expansion guidance, got %s", bundle.SampleCall)
	}
	if len(bundle.VerificationLinks) != 3 {
		t.Fatalf("expected 3 verification links, got %d", len(bundle.VerificationLinks))
	}
	if bundle.VerificationLinks[0].Path != "/events?agent_id=agent-1&tenant_id=tenant-1" {
		t.Fatalf("unexpected events verification link: %+v", bundle.VerificationLinks[0])
	}
	if bundle.VerificationLinks[1].Path != "/sessions?agent_id=agent-1&tenant_id=tenant-1" {
		t.Fatalf("unexpected sessions verification link: %+v", bundle.VerificationLinks[1])
	}
	if bundle.VerificationLinks[2].Path != "/approvals?tenant_id=tenant-1" {
		t.Fatalf("unexpected approvals verification link: %+v", bundle.VerificationLinks[2])
	}
}

func TestBuildBundleSupportsTypeScriptGoldenPathArtifacts(t *testing.T) {
	bundle, err := BuildBundle(BundleRequest{
		BaseURL:         "http://localhost:8080",
		TenantID:        "tenant-1",
		TenantName:      "Alpha Corp",
		AgentID:         "agent-ts",
		AgentName:       "Node Bot",
		APIKey:          "sk-oc-ts",
		APIKeyMode:      APIKeyModeRawProvided,
		Runtime:         RuntimeTypeScript,
		ApprovalPosture: "pilot_safe",
		Tools: []SelectedTool{
			{Tool: "slack", Action: "slack.channel.list"},
			{Tool: "slack", Action: "slack.msg.post"},
		},
	})
	if err != nil {
		t.Fatalf("BuildBundle: %v", err)
	}

	if bundle.StarterFileName != "agent.ts" {
		t.Fatalf("expected TypeScript starter file, got %q", bundle.StarterFileName)
	}
	if bundle.RuntimeLabel != "TypeScript SDK wrapper" {
		t.Fatalf("expected TypeScript runtime label, got %q", bundle.RuntimeLabel)
	}
	if !containsAll(bundle.StarterSnippet, []string{"OpenClauseClient", "riskScore", "settleDecision", "waitForApproval"}) {
		t.Fatalf("expected TypeScript starter guidance, got %s", bundle.StarterSnippet)
	}
	if !containsAll(bundle.ReadmeSnippet, []string{"agent.ts", "npx tsx agent.ts", "Runtime setup", "session_id", "trace_id", "idempotency_key"}) {
		t.Fatalf("expected TypeScript README guidance, got %s", bundle.ReadmeSnippet)
	}

	var packageArtifact *BundleArtifact
	for index := range bundle.Artifacts {
		artifact := &bundle.Artifacts[index]
		if artifact.ID == "package-snippet" {
			packageArtifact = artifact
			break
		}
	}
	if packageArtifact == nil {
		t.Fatalf("expected TypeScript package snippet artifact, got %+v", bundle.Artifacts)
	}
	if packageArtifact.FileName != "package.onboarding.json" || packageArtifact.Language != "json" {
		t.Fatalf("unexpected TypeScript package artifact metadata: %+v", packageArtifact)
	}
	if !containsAll(packageArtifact.Content, []string{`"openclause"`, `"tsx"`, `"typescript"`, `"onboarding:verify"`}) {
		t.Fatalf("expected package snippet content, got %s", packageArtifact.Content)
	}
}

func TestBuildBundleSupportsAllGoldenPathRuntimes(t *testing.T) {
	cases := []struct {
		name       string
		runtime    Runtime
		wantLabel  string
		wantFile   string
		wantMarker string
	}{
		{name: "python", runtime: RuntimePython, wantLabel: "Python SDK wrapper", wantFile: "agent.py", wantMarker: "governed_call"},
		{name: "typescript", runtime: RuntimeTypeScript, wantLabel: "TypeScript SDK wrapper", wantFile: "agent.ts", wantMarker: "OpenClauseClient"},
		{name: "langchain", runtime: RuntimeLangChain, wantLabel: "LangChain", wantFile: "langchain_agent.py", wantMarker: "build_tools"},
		{name: "openai_local", runtime: RuntimeOpenAILocal, wantLabel: "Local OpenAI-compatible model", wantFile: "local_model_agent.py", wantMarker: "chat.completions.create"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bundle, err := BuildBundle(BundleRequest{
				BaseURL:         "http://localhost:8080",
				TenantID:        "tenant-1",
				TenantName:      "Alpha Corp",
				AgentID:         "agent-1",
				AgentName:       "Support Bot",
				APIKey:          "sk-oc-demo",
				APIKeyMode:      APIKeyModeRawProvided,
				Runtime:         tc.runtime,
				ApprovalPosture: "pilot_safe",
				Tools: []SelectedTool{
					{Tool: "slack", Action: "slack.channel.list"},
					{Tool: "slack", Action: "slack.msg.post"},
				},
			})
			if err != nil {
				t.Fatalf("BuildBundle(%s): %v", tc.runtime, err)
			}
			if bundle.Runtime != string(tc.runtime) {
				t.Fatalf("expected runtime %q, got %q", tc.runtime, bundle.Runtime)
			}
			if bundle.RuntimeLabel != tc.wantLabel {
				t.Fatalf("expected runtime label %q, got %q", tc.wantLabel, bundle.RuntimeLabel)
			}
			if bundle.StarterFileName != tc.wantFile {
				t.Fatalf("expected starter file %q, got %q", tc.wantFile, bundle.StarterFileName)
			}
			if !strings.Contains(bundle.StarterSnippet, tc.wantMarker) {
				t.Fatalf("expected starter snippet marker %q, got %s", tc.wantMarker, bundle.StarterSnippet)
			}
			if !containsAll(bundle.ReadmeSnippet, []string{"Quick start", "session_id", "trace_id", "idempotency_key"}) {
				t.Fatalf("expected README guidance for %s, got %s", tc.runtime, bundle.ReadmeSnippet)
			}
			if len(bundle.VerificationLinks) != 3 {
				t.Fatalf("expected verification links for %s, got %+v", tc.runtime, bundle.VerificationLinks)
			}
		})
	}
}

func TestBuildBundleSupportsLMStudioFriendlyOpenAILocalEnv(t *testing.T) {
	bundle, err := BuildBundle(BundleRequest{
		BaseURL:         "http://localhost:8080",
		TenantID:        "tenant-1",
		TenantName:      "Alpha Corp",
		AgentID:         "agent-local",
		AgentName:       "Qwen Bot",
		APIKey:          "sk-oc-local",
		APIKeyMode:      APIKeyModeRawProvided,
		Runtime:         RuntimeOpenAILocal,
		ApprovalPosture: "pilot_safe",
		Tools: []SelectedTool{
			{Tool: "postgres", Action: "query.readonly"},
		},
	})
	if err != nil {
		t.Fatalf("BuildBundle openai_local: %v", err)
	}

	if got := bundle.Environment["LOCAL_MODEL_BASE_URL"]; got != "http://localhost:1234/v1" {
		t.Fatalf("expected local model base url env, got %q", got)
	}
	if got := bundle.Environment["OPENCLAUSE_BRIDGE_URL"]; got != "http://127.0.0.1:8787" {
		t.Fatalf("expected local bridge url env, got %q", got)
	}
	if got := bundle.Environment["LOCAL_MODEL_NAME"]; got != "replace-with-lmstudio-model-id" {
		t.Fatalf("expected local model name placeholder, got %q", got)
	}
	if !containsAll(bundle.EnvironmentFile, []string{"OPENCLAUSE_BRIDGE_URL", "LOCAL_MODEL_BASE_URL", "LOCAL_MODEL_NAME", "LOCAL_MODEL_API_KEY"}) {
		t.Fatalf("expected local model env vars in env file, got %s", bundle.EnvironmentFile)
	}
	if !containsAll(bundle.EnvironmentScript, []string{`export OPENCLAUSE_BRIDGE_URL="${OPENCLAUSE_BRIDGE_URL:-http://127.0.0.1:8787}"`, `export LOCAL_MODEL_NAME="${LOCAL_MODEL_NAME:-replace-with-lmstudio-model-id}"`, `export LOCAL_MODEL_BASE_URL="${LOCAL_MODEL_BASE_URL:-http://localhost:1234/v1}"`}) {
		t.Fatalf("expected local model env vars to preserve manual overrides, got %s", bundle.EnvironmentScript)
	}
	if !containsAll(bundle.StarterSnippet, []string{"LOCAL_MODEL_NAME", "OPENCLAUSE_BRIDGE_URL", "bridge_base_url", "chat.chat.completions.create", "argparse", "interactive_loop", "--smoke", "extract_governed_results", "governed_results"}) {
		t.Fatalf("expected env-driven local model snippet, got %s", bundle.StarterSnippet)
	}
	if !containsAll(bundle.StarterSnippet, []string{"def request_completion(model: str, messages: list[dict]) -> tuple[str, str, list[dict]]:", "\n    try:\n", "except APIConnectionError as exc:"}) {
		t.Fatalf("expected valid bridge connection handling indentation, got %s", bundle.StarterSnippet)
	}
	if !containsAll(bundle.ReadmeSnippet, []string{"python -m pip install --upgrade pip setuptools wheel", "python -m pip install --no-build-isolation -e ../sdk/python", "curl http://localhost:1234/v1/models", "LOCAL_MODEL_NAME", "./start-bridge.sh", "OpenAI-compatible chat client", "openclause bridge chat --config ./openclause-bridge.yaml", "lmstudio.mcp.example.jsonc", "lmstudio.mcp.remote.example.jsonc", "recommended default", "openclause.governed_results"}) {
		t.Fatalf("expected LM Studio setup guidance in README, got %s", bundle.ReadmeSnippet)
	}
	if !containsAll(bundle.StarterSnippet, []string{"fetch the newest 3 demo users", `base_url=bridge_base_url + "/v1"`, `Type a normal prompt`, `Conversation reset.`}) {
		t.Fatalf("expected bridge-backed local model starter, got %s", bundle.StarterSnippet)
	}
	if !containsAll(bundle.SampleCall, []string{`"sql": "select id, name, email, created_at from demo_users order by created_at desc limit 3"`, `"params": []`, `"risk_score": 2`}) {
		t.Fatalf("expected postgres readonly sample call, got %s", bundle.SampleCall)
	}

	var bridgeArtifact *BundleArtifact
	var bridgeLauncherArtifact *BundleArtifact
	var lmStudioArtifact *BundleArtifact
	var lmStudioRemoteArtifact *BundleArtifact
	for index := range bundle.Artifacts {
		artifact := &bundle.Artifacts[index]
		if artifact.ID == "bridge-config" {
			bridgeArtifact = artifact
		}
		if artifact.ID == "bridge-launcher" {
			bridgeLauncherArtifact = artifact
		}
		if artifact.ID == "lmstudio-mcp-snippet" {
			lmStudioArtifact = artifact
		}
		if artifact.ID == "lmstudio-mcp-remote-snippet" {
			lmStudioRemoteArtifact = artifact
		}
	}
	if bridgeArtifact == nil {
		t.Fatalf("expected local bridge config artifact, got %+v", bundle.Artifacts)
	}
	if bridgeArtifact.FileName != "openclause-bridge.yaml" || bridgeArtifact.Language != "yaml" {
		t.Fatalf("unexpected bridge artifact metadata: %+v", bridgeArtifact)
	}
	if !containsAll(bridgeArtifact.Content, []string{`base_url: "http://localhost:8080"`, `api_key: "env:OPENCLAUSE_API_KEY"`, `upstream_base_url: "env:LOCAL_MODEL_BASE_URL"`, `model: "env:LOCAL_MODEL_NAME"`, `tool: "postgres"`, `action: "query.readonly"`, `risk_mode: "configured"`, `tool_name: "governed_action"`}) {
		t.Fatalf("expected bridge config content, got %s", bridgeArtifact.Content)
	}
	if bridgeLauncherArtifact == nil {
		t.Fatalf("expected bridge launcher artifact, got %+v", bundle.Artifacts)
	}
	if bridgeLauncherArtifact.FileName != "start-bridge.sh" || !bridgeLauncherArtifact.Executable {
		t.Fatalf("unexpected bridge launcher artifact metadata: %+v", bridgeLauncherArtifact)
	}
	if !containsAll(bridgeLauncherArtifact.Content, []string{`source "$script_dir/setup-env.sh"`, `openclause bridge start --config`, `go run "$script_dir/../cmd/openclause" bridge start --config`, `Could not find the OpenClause CLI.`}) {
		t.Fatalf("expected bridge launcher content, got %s", bridgeLauncherArtifact.Content)
	}
	if lmStudioArtifact == nil {
		t.Fatalf("expected LM Studio MCP snippet artifact, got %+v", bundle.Artifacts)
	}
	if lmStudioArtifact.FileName != "lmstudio.mcp.example.jsonc" || lmStudioArtifact.Language != "jsonc" {
		t.Fatalf("unexpected LM Studio MCP artifact metadata: %+v", lmStudioArtifact)
	}
	if !containsAll(lmStudioArtifact.Content, []string{`"command": "/usr/bin/env"`, `"bash"`, `. /absolute/path/to/setup-env.sh`, `go -C /absolute/path/to/OpenClause run ./cmd/openclause bridge mcp --config /absolute/path/to/openclause-bridge.yaml`}) {
		t.Fatalf("expected LM Studio MCP snippet content, got %s", lmStudioArtifact.Content)
	}
	if lmStudioRemoteArtifact == nil {
		t.Fatalf("expected LM Studio remote MCP snippet artifact, got %+v", bundle.Artifacts)
	}
	if lmStudioRemoteArtifact.FileName != "lmstudio.mcp.remote.example.jsonc" || lmStudioRemoteArtifact.Language != "jsonc" {
		t.Fatalf("unexpected LM Studio remote MCP artifact metadata: %+v", lmStudioRemoteArtifact)
	}
	if !containsAll(lmStudioRemoteArtifact.Content, []string{`"url": "http://127.0.0.1:8787/mcp"`, `"X-OpenClause-Profile": "support"`, "Recommended default", "Run ./start-bridge.sh"}) {
		t.Fatalf("expected LM Studio remote MCP snippet content, got %s", lmStudioRemoteArtifact.Content)
	}
}

func TestBuildBundleIncludesModeSpecificNotes(t *testing.T) {
	rawProvidedBundle, err := BuildBundle(BundleRequest{
		BaseURL:    "http://localhost:8080",
		TenantID:   "tenant-1",
		TenantName: "Alpha Corp",
		AgentID:    "agent-1",
		AgentName:  "Created Bot",
		APIKey:     "sk-oc-created",
		APIKeyMode: APIKeyModeRawProvided,
		Runtime:    RuntimePython,
		Tools:      []SelectedTool{{Tool: "slack", Action: "slack.channel.list"}},
	})
	if err != nil {
		t.Fatalf("BuildBundle raw provided: %v", err)
	}
	if !containsAll(strings.Join(rawProvidedBundle.Notes, " "), []string{"one-time raw API key", "full key will not be shown again"}) {
		t.Fatalf("expected raw-key note guidance, got %+v", rawProvidedBundle.Notes)
	}

	previewBundle, err := BuildBundle(BundleRequest{
		BaseURL:    "http://localhost:8080",
		TenantID:   "tenant-1",
		TenantName: "Alpha Corp",
		AgentID:    PreviewAgentID("Preview Bot"),
		AgentName:  "Preview Bot",
		APIKey:     "${OPENCLAUSE_API_KEY:-generated-on-create}",
		APIKeyMode: APIKeyModePreview,
		Runtime:    RuntimePython,
		Tools:      []SelectedTool{{Tool: "slack", Action: "slack.channel.list"}},
	})
	if err != nil {
		t.Fatalf("BuildBundle preview: %v", err)
	}
	if !containsAll(strings.Join(previewBundle.Notes, " "), []string{"placeholder API key", "create an integration"}) {
		t.Fatalf("expected preview note guidance, got %+v", previewBundle.Notes)
	}
	if !containsAll(previewBundle.ReadmeSnippet, []string{"preview bundle", "no credentials were created"}) {
		t.Fatalf("expected preview credential guidance in README, got %s", previewBundle.ReadmeSnippet)
	}

	existingKeyBundle, err := BuildBundle(BundleRequest{
		BaseURL:      "http://localhost:8080",
		TenantID:     "tenant-1",
		TenantName:   "Alpha Corp",
		AgentID:      "agent-1",
		AgentName:    "Existing Bot",
		APIKey:       "${OPENCLAUSE_API_KEY:-reuse-existing-key}",
		APIKeyMode:   APIKeyModeExistingKeyRef,
		APIKeyPrefix: "sk-oc-ref",
		Runtime:      RuntimePython,
		Tools:        []SelectedTool{{Tool: "slack", Action: "slack.channel.list"}},
	})
	if err != nil {
		t.Fatalf("BuildBundle existing key: %v", err)
	}
	if !containsAll(strings.Join(existingKeyBundle.Notes, " "), []string{"Raw API keys are only shown at creation time", "sk-oc-ref"}) {
		t.Fatalf("expected existing-key note guidance, got %+v", existingKeyBundle.Notes)
	}
	if !containsAll(existingKeyBundle.ReadmeSnippet, []string{"No raw key was reissued", "sk-oc-ref"}) {
		t.Fatalf("expected existing-key credential guidance in README, got %s", existingKeyBundle.ReadmeSnippet)
	}
}

func TestBuildBundleRejectsMissingCriticalInputs(t *testing.T) {
	cases := []struct {
		name string
		req  BundleRequest
		want string
	}{
		{
			name: "missing base URL",
			req: BundleRequest{
				TenantID:  "tenant-1",
				AgentID:   "agent-1",
				AgentName: "Support Bot",
				APIKey:    "sk-oc-demo",
				Runtime:   RuntimePython,
				Tools:     []SelectedTool{{Tool: "slack", Action: "slack.channel.list"}},
			},
			want: "base URL required",
		},
		{
			name: "missing tools",
			req: BundleRequest{
				BaseURL:   "http://localhost:8080",
				TenantID:  "tenant-1",
				AgentID:   "agent-1",
				AgentName: "Support Bot",
				APIKey:    "sk-oc-demo",
				Runtime:   RuntimePython,
			},
			want: "at least one governed tool is required",
		},
		{
			name: "missing tool name",
			req: BundleRequest{
				BaseURL:   "http://localhost:8080",
				TenantID:  "tenant-1",
				AgentID:   "agent-1",
				AgentName: "Support Bot",
				APIKey:    "sk-oc-demo",
				Runtime:   RuntimePython,
				Tools:     []SelectedTool{{Action: "slack.channel.list"}},
			},
			want: "governed tool 1 is missing a tool name",
		},
		{
			name: "missing tool action",
			req: BundleRequest{
				BaseURL:   "http://localhost:8080",
				TenantID:  "tenant-1",
				AgentID:   "agent-1",
				AgentName: "Support Bot",
				APIKey:    "sk-oc-demo",
				Runtime:   RuntimePython,
				Tools:     []SelectedTool{{Tool: "slack"}},
			},
			want: "governed tool 1 is missing an action",
		},
		{
			name: "missing api key",
			req: BundleRequest{
				BaseURL:   "http://localhost:8080",
				TenantID:  "tenant-1",
				AgentID:   "agent-1",
				AgentName: "Support Bot",
				Runtime:   RuntimePython,
				Tools:     []SelectedTool{{Tool: "slack", Action: "slack.channel.list"}},
			},
			want: "API key required",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildBundle(tc.req)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error %q, got %v", tc.want, err)
			}
		})
	}
}

func TestBuildBundleRejectsUnsupportedRuntime(t *testing.T) {
	_, err := BuildBundle(BundleRequest{
		BaseURL:   "http://localhost:8080",
		TenantID:  "tenant-1",
		AgentID:   "agent-1",
		AgentName: "Support Bot",
		APIKey:    "sk-oc-demo",
		Runtime:   Runtime("mystery"),
		Tools:     []SelectedTool{{Tool: "slack", Action: "slack.channel.list"}},
	})
	if err == nil || !strings.Contains(err.Error(), `unsupported runtime "mystery"`) {
		t.Fatalf("expected unsupported runtime error, got %v", err)
	}
}

func TestWriteArtifactsWritesCliReadyBundleFiles(t *testing.T) {
	bundle, err := BuildBundle(BundleRequest{
		BaseURL:      "http://localhost:8080",
		TenantID:     "tenant-1",
		TenantName:   "Alpha Corp",
		AgentID:      "agent-1",
		AgentName:    "Support Bot",
		APIKey:       "${OPENCLAUSE_API_KEY:-reuse-existing-key}",
		APIKeyMode:   APIKeyModeExistingKeyRef,
		APIKeyPrefix: "sk-oc-ab",
		Runtime:      RuntimeLangChain,
		Tools: []SelectedTool{
			{Tool: "slack", Action: "slack.channel.list"},
			{Tool: "slack", Action: "slack.msg.post"},
		},
	})
	if err != nil {
		t.Fatalf("BuildBundle: %v", err)
	}

	dir := t.TempDir()
	written, err := WriteArtifacts(bundle, dir)
	if err != nil {
		t.Fatalf("WriteArtifacts: %v", err)
	}
	if len(written) != len(bundle.Artifacts) {
		t.Fatalf("expected %d written artifacts, got %d", len(bundle.Artifacts), len(written))
	}

	starterPath := filepath.Join(dir, "langchain_agent.py")
	content, err := os.ReadFile(starterPath)
	if err != nil {
		t.Fatalf("ReadFile starter: %v", err)
	}
	if len(content) == 0 {
		t.Fatalf("expected starter artifact content at %s", starterPath)
	}
}

func containsAll(haystack string, needles []string) bool {
	for _, needle := range needles {
		if !strings.Contains(haystack, needle) {
			return false
		}
	}
	return true
}
