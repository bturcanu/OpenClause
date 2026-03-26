package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bturcanu/OpenClause/pkg/onboarding"
)

func TestRunInitAgentWritesArtifactsLocalOnly(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	dir := t.TempDir()

	err := run([]string{
		"init-agent",
		"--tenant-id", "tenant-1",
		"--tenant-name", "Alpha Corp",
		"--agent-name", "Support Bot",
		"--runtime", "python",
		"--tools", "slack:slack.channel.list,slack:slack.msg.post",
		"--output-dir", dir,
		"--local-only",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run init-agent: %v stderr=%s", err, stderr.String())
	}

	if !strings.Contains(stdout.String(), "Mode: Local-only generation") {
		t.Fatalf("expected local mode summary, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Wrote:") {
		t.Fatalf("expected written file listing, got %s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "agent.py")); err != nil {
		t.Fatalf("expected agent.py to be written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env.example")); err != nil {
		t.Fatalf("expected .env.example to be written: %v", err)
	}
}

func TestRunBridgeStartRequiresConfig(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	err := run([]string{"bridge", "start"}, stdout, stderr)
	if err == nil || err.Error() != "--config is required" {
		t.Fatalf("expected missing config error, got %v", err)
	}
}

func TestRunBridgeHelp(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	if err := run([]string{"bridge", "help"}, stdout, stderr); err != nil {
		t.Fatalf("run bridge help: %v", err)
	}
	if !strings.Contains(stdout.String(), "openclause bridge start --config") {
		t.Fatalf("expected bridge usage in help, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "openclause bridge chat --config") {
		t.Fatalf("expected bridge chat usage in help, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "[--profile NAME]") {
		t.Fatalf("expected bridge help to mention profile selection, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "openclause bridge doctor --config") {
		t.Fatalf("expected bridge doctor usage in help, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "openclause bridge mcp --config") {
		t.Fatalf("expected bridge mcp usage in help, got %s", stdout.String())
	}
}

func TestRunAuthLoginStoresProfileAndWhoAmI(t *testing.T) {
	t.Setenv("OPENCLAUSE_CONFIG_DIR", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/login" {
			t.Fatalf("unexpected auth path %s", r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode auth login request: %v", err)
		}
		if body["email"] != "admin@openclause.dev" || body["password"] != "Admin123!" {
			t.Fatalf("unexpected auth body %+v", body)
		}
		_ = json.NewEncoder(w).Encode(loginResponse{
			Token:     "stored-token-123",
			SessionID: "sess-1",
			User: authUser{
				ID:    "user-1",
				Email: "admin@openclause.dev",
				Name:  "Admin",
				Roles: []string{"platform_admin"},
			},
		})
	}))
	defer server.Close()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	if err := runWithIO([]string{
		"auth", "login",
		"--server-url", server.URL,
		"--email", "admin@openclause.dev",
		"--password", "Admin123!",
	}, strings.NewReader(""), stdout, stderr); err != nil {
		t.Fatalf("run auth login: %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Token stored for future server-backed onboarding commands.") {
		t.Fatalf("expected login success output, got %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if err := runWithIO([]string{
		"auth", "whoami",
		"--server-url", server.URL,
	}, strings.NewReader(""), stdout, stderr); err != nil {
		t.Fatalf("run auth whoami: %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "admin@openclause.dev") || !strings.Contains(stdout.String(), "platform_admin") {
		t.Fatalf("expected whoami output, got %s", stdout.String())
	}
}

func TestRunAuthLogoutRemovesStoredProfile(t *testing.T) {
	t.Setenv("OPENCLAUSE_CONFIG_DIR", t.TempDir())
	if err := saveStoredAuthConfig(&storedAuthConfig{
		CurrentProfile: "http://console.test",
		Profiles: map[string]storedAuthProfile{
			"http://console.test": {
				ServerURL: "http://console.test",
				Token:     "stored-token-123",
				User:      storedAuthUser{Email: "admin@openclause.dev"},
			},
		},
	}); err != nil {
		t.Fatalf("saveStoredAuthConfig: %v", err)
	}

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	if err := runWithIO([]string{
		"auth", "logout",
		"--server-url", "http://console.test",
	}, strings.NewReader(""), stdout, stderr); err != nil {
		t.Fatalf("run auth logout: %v stderr=%s", err, stderr.String())
	}

	cfg, err := loadStoredAuthConfig()
	if err != nil {
		t.Fatalf("loadStoredAuthConfig: %v", err)
	}
	if len(cfg.Profiles) != 0 {
		t.Fatalf("expected stored profile to be removed, got %+v", cfg.Profiles)
	}
}

func TestRunAuthWhoamiRequiresDisambiguationWhenMultipleProfilesExist(t *testing.T) {
	t.Setenv("OPENCLAUSE_CONFIG_DIR", t.TempDir())
	if err := saveStoredAuthConfig(&storedAuthConfig{
		Profiles: map[string]storedAuthProfile{
			"alpha": {
				ServerURL: "http://alpha.test",
				Token:     "token-alpha",
				User:      storedAuthUser{Email: "alpha@openclause.dev"},
			},
			"beta": {
				ServerURL: "http://beta.test",
				Token:     "token-beta",
				User:      storedAuthUser{Email: "beta@openclause.dev"},
			},
		},
	}); err != nil {
		t.Fatalf("saveStoredAuthConfig: %v", err)
	}

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	err := runWithIO([]string{"auth", "whoami"}, strings.NewReader(""), stdout, stderr)
	if err == nil || !strings.Contains(err.Error(), "multiple stored auth profiles found") {
		t.Fatalf("expected ambiguity error, got err=%v stderr=%s", err, stderr.String())
	}
}

func TestRunInitAgentWritesTypeScriptArtifactsLocalOnly(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	dir := t.TempDir()

	err := run([]string{
		"init-agent",
		"--tenant-id", "tenant-1",
		"--tenant-name", "Alpha Corp",
		"--agent-name", "Node Bot",
		"--runtime", "typescript",
		"--tools", "slack:slack.channel.list,slack:slack.msg.post",
		"--output-dir", dir,
		"--local-only",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run init-agent typescript: %v stderr=%s", err, stderr.String())
	}

	if !strings.Contains(stdout.String(), "TypeScript SDK wrapper") {
		t.Fatalf("expected TypeScript runtime summary, got %s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "agent.ts")); err != nil {
		t.Fatalf("expected agent.ts to be written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "package.onboarding.json")); err != nil {
		t.Fatalf("expected package.onboarding.json to be written: %v", err)
	}
}

func TestRunInitAgentWritesOpenAILocalBridgeArtifactsLocalOnly(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	dir := t.TempDir()

	err := run([]string{
		"init-agent",
		"--tenant-id", "tenant-1",
		"--tenant-name", "Alpha Corp",
		"--agent-name", "Qwen Bot",
		"--runtime", "openai_local",
		"--tools", "postgres:query.readonly",
		"--output-dir", dir,
		"--local-only",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run init-agent openai_local: %v stderr=%s", err, stderr.String())
	}

	if _, err := os.Stat(filepath.Join(dir, "local_model_agent.py")); err != nil {
		t.Fatalf("expected local_model_agent.py to be written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "openclause-bridge.yaml")); err != nil {
		t.Fatalf("expected openclause-bridge.yaml to be written: %v", err)
	}
	if !strings.Contains(stdout.String(), "Local OpenAI-compatible model") {
		t.Fatalf("expected openai_local runtime summary, got %s", stdout.String())
	}
}

func TestRunInitAgentPrintOnlyServerBackedCreate(t *testing.T) {
	server := newCLIOnboardingServer(t, nil)
	defer server.Close()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	err := run([]string{
		"init-agent",
		"--server-url", server.URL,
		"--auth-token", "token-123",
		"--tenant-id", "tenant-1",
		"--agent-name", "Support Bot",
		"--runtime", "python",
		"--tools", "slack:slack.channel.list",
		"--print-only",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run init-agent server print-only: %v stderr=%s", err, stderr.String())
	}

	if !strings.Contains(stdout.String(), "Mode: Server create") {
		t.Fatalf("expected create mode summary, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "One-time API key: sk-oc-demo-raw") {
		t.Fatalf("expected one-time raw key notice, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Verification links:") {
		t.Fatalf("expected verification links, got %s", stdout.String())
	}
}

func TestRunInitAgentWritesServerBackedArtifacts(t *testing.T) {
	server := newCLIOnboardingServer(t, nil)
	defer server.Close()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	dir := t.TempDir()
	err := run([]string{
		"init-agent",
		"--server-url", server.URL,
		"--auth-token", "token-123",
		"--tenant-id", "tenant-1",
		"--agent-name", "Support Bot",
		"--runtime", "python",
		"--tools", "slack:slack.channel.list",
		"--output-dir", dir,
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run init-agent server write: %v stderr=%s", err, stderr.String())
	}

	if !strings.Contains(stdout.String(), "Raw API key was returned once") {
		t.Fatalf("expected raw key write notice, got %s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "agent.py")); err != nil {
		t.Fatalf("expected agent.py to be written: %v", err)
	}
}

func TestRunInitAgentPreviewServerBackedPrintOnly(t *testing.T) {
	server := newCLIOnboardingServer(t, nil)
	defer server.Close()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	err := run([]string{
		"init-agent",
		"--server-url", server.URL,
		"--auth-token", "token-123",
		"--preview",
		"--tenant-id", "tenant-1",
		"--agent-name", "Support Bot",
		"--runtime", "python",
		"--tools", "slack:slack.channel.list",
		"--print-only",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run init-agent preview print-only: %v stderr=%s", err, stderr.String())
	}

	if !strings.Contains(stdout.String(), "Mode: Server preview") {
		t.Fatalf("expected preview mode summary, got %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "One-time API key:") {
		t.Fatalf("did not expect raw key output for preview, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Verification links:") {
		t.Fatalf("expected verification links for preview, got %s", stdout.String())
	}
}

func TestRunInitAgentPreviewServerBackedNoFiles(t *testing.T) {
	server := newCLIOnboardingServer(t, nil)
	defer server.Close()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	err := run([]string{
		"init-agent",
		"--server-url", server.URL,
		"--auth-token", "token-123",
		"--preview",
		"--tenant-id", "tenant-1",
		"--agent-name", "Support Bot",
		"--runtime", "python",
		"--tools", "slack:slack.channel.list",
		"--no-files",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run init-agent preview no-files: %v stderr=%s", err, stderr.String())
	}

	if !strings.Contains(stdout.String(), "No files written (--no-files).") {
		t.Fatalf("expected no-files output, got %s", stdout.String())
	}
}

func TestRunInitAgentRegenerateServerBackedWritesArtifacts(t *testing.T) {
	server := newCLIOnboardingServer(t, nil)
	defer server.Close()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	dir := t.TempDir()
	err := run([]string{
		"init-agent",
		"--server-url", server.URL,
		"--auth-token", "token-123",
		"--regenerate",
		"--tenant-id", "tenant-1",
		"--agent-id", "agent-1",
		"--agent-name", "Support Bot",
		"--runtime", "python",
		"--tools", "slack:slack.channel.list",
		"--output-dir", dir,
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run init-agent regenerate write: %v stderr=%s", err, stderr.String())
	}

	if !strings.Contains(stdout.String(), "Mode: Server regenerate") {
		t.Fatalf("expected regenerate mode summary, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Existing API key reference: sk-oc-ex") {
		t.Fatalf("expected existing key reference notice, got %s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "agent.py")); err != nil {
		t.Fatalf("expected regenerated agent.py to be written: %v", err)
	}
}

func TestRunInitAgentRegenerateWithDefaultsServerBackedPrintOnly(t *testing.T) {
	server := newCLIOnboardingServer(t, func(path string, body map[string]any) {
		if path != "/admin/onboarding/bundles/regenerate-defaults" {
			return
		}
		if got, exists := body["tools"]; exists && got != nil {
			t.Fatalf("did not expect explicit tool values in defaults regenerate payload: %+v", body)
		}
		if got := body["agent_id"]; got != "agent-1" {
			t.Fatalf("expected agent_id in defaults regenerate payload, got %+v", body)
		}
	})
	defer server.Close()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	err := run([]string{
		"init-agent",
		"--server-url", server.URL,
		"--auth-token", "token-123",
		"--regenerate",
		"--use-defaults",
		"--tenant-id", "tenant-1",
		"--agent-id", "agent-1",
		"--output-dir", t.TempDir(),
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run init-agent regenerate defaults: %v stderr=%s", err, stderr.String())
	}

	if !strings.Contains(stdout.String(), "Mode: Server regenerate with defaults") {
		t.Fatalf("expected defaults regenerate mode summary, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Defaults applied:") || !strings.Contains(stdout.String(), "tool=slack:slack.channel.list") {
		t.Fatalf("expected explicit defaults output, got %s", stdout.String())
	}
}

func TestRunInitAgentFailsWhenServerAuthMissing(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	err := run([]string{
		"init-agent",
		"--server-url", "http://example.test",
		"--tenant-id", "tenant-1",
		"--agent-name", "Support Bot",
		"--runtime", "python",
		"--tools", "slack:slack.channel.list",
	}, stdout, stderr)
	if err == nil || !strings.Contains(err.Error(), "--auth-token is required") {
		t.Fatalf("expected missing auth-token failure, got err=%v stderr=%s", err, stderr.String())
	}
}

func TestRunInitAgentUsesStoredAuthToken(t *testing.T) {
	t.Setenv("OPENCLAUSE_CONFIG_DIR", t.TempDir())
	server := newCLIOnboardingServer(t, nil)
	defer server.Close()
	if err := saveStoredAuthConfig(&storedAuthConfig{
		CurrentProfile: server.URL,
		Profiles: map[string]storedAuthProfile{
			server.URL: {
				ServerURL: server.URL,
				Token:     "token-123",
				User:      storedAuthUser{Email: "admin@openclause.dev"},
			},
		},
	}); err != nil {
		t.Fatalf("saveStoredAuthConfig: %v", err)
	}

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	err := run([]string{
		"init-agent",
		"--server-url", server.URL,
		"--tenant-id", "tenant-1",
		"--agent-name", "Support Bot",
		"--runtime", "python",
		"--tools", "slack:slack.channel.list",
		"--print-only",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run init-agent with stored auth: %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Mode: Server create") {
		t.Fatalf("expected server create output, got %s", stdout.String())
	}
}

func TestRunInitAgentUsesStoredAuthTokenFromNamedProfileMatchingServerURL(t *testing.T) {
	t.Setenv("OPENCLAUSE_CONFIG_DIR", t.TempDir())
	server := newCLIOnboardingServer(t, nil)
	defer server.Close()
	if err := saveStoredAuthConfig(&storedAuthConfig{
		CurrentProfile: "sandbox",
		Profiles: map[string]storedAuthProfile{
			"prod-admin": {
				ServerURL: server.URL + "/",
				Token:     "token-123",
				User:      storedAuthUser{Email: "admin@openclause.dev"},
			},
			"sandbox": {
				ServerURL: "http://sandbox.test",
				Token:     "token-sandbox",
				User:      storedAuthUser{Email: "sandbox@openclause.dev"},
			},
		},
	}); err != nil {
		t.Fatalf("saveStoredAuthConfig: %v", err)
	}

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	err := run([]string{
		"init-agent",
		"--server-url", server.URL,
		"--tenant-id", "tenant-1",
		"--agent-name", "Support Bot",
		"--runtime", "python",
		"--tools", "slack:slack.channel.list",
		"--print-only",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run init-agent with named stored auth profile: %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Mode: Server create") {
		t.Fatalf("expected server create output, got %s", stdout.String())
	}
}

func TestResolveCLIAuthTokenPrefersStoredProfileOverEnvFallback(t *testing.T) {
	t.Setenv("OPENCLAUSE_CONFIG_DIR", t.TempDir())
	t.Setenv("OPENCLAUSE_AUTH_TOKEN", "token-env")
	if err := saveStoredAuthConfig(&storedAuthConfig{
		CurrentProfile: "sandbox",
		Profiles: map[string]storedAuthProfile{
			"prod-admin": {
				ServerURL: "http://console.prod.test/",
				Token:     "token-stored",
			},
			"sandbox": {
				ServerURL: "http://console.sandbox.test/",
				Token:     "token-sandbox",
			},
		},
	}); err != nil {
		t.Fatalf("saveStoredAuthConfig: %v", err)
	}

	got, err := resolveCLIAuthToken(&cliConfig{
		serverURL:   "http://console.prod.test",
		authProfile: "prod-admin",
	})
	if err != nil {
		t.Fatalf("resolveCLIAuthToken: %v", err)
	}
	if got != "token-stored" {
		t.Fatalf("expected stored profile token, got %q", got)
	}
}

func TestRunInitAgentFailsWhenPreviewMissingTenantID(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	err := run([]string{
		"init-agent",
		"--server-url", "http://example.test",
		"--auth-token", "token-123",
		"--preview",
		"--agent-name", "Support Bot",
		"--runtime", "python",
		"--tools", "slack:slack.channel.list",
	}, stdout, stderr)
	if err == nil || !strings.Contains(err.Error(), "--tenant-id is required for --preview") {
		t.Fatalf("expected preview tenant failure, got err=%v stderr=%s", err, stderr.String())
	}
}

func TestRunInitAgentFailsWhenRegenerateMissingAgentID(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	err := run([]string{
		"init-agent",
		"--server-url", "http://example.test",
		"--auth-token", "token-123",
		"--regenerate",
		"--tenant-id", "tenant-1",
		"--agent-name", "Support Bot",
		"--runtime", "python",
		"--tools", "slack:slack.channel.list",
	}, stdout, stderr)
	if err == nil || !strings.Contains(err.Error(), "--agent-id is required for --regenerate") {
		t.Fatalf("expected regenerate agent-id failure, got err=%v stderr=%s", err, stderr.String())
	}
}

func TestRunInitAgentFailsWhenPreviewAuthMissing(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	err := run([]string{
		"init-agent",
		"--server-url", "http://example.test",
		"--preview",
		"--tenant-id", "tenant-1",
		"--agent-name", "Support Bot",
		"--runtime", "python",
		"--tools", "slack:slack.channel.list",
	}, stdout, stderr)
	if err == nil || !strings.Contains(err.Error(), "--auth-token is required") {
		t.Fatalf("expected preview auth failure, got err=%v stderr=%s", err, stderr.String())
	}
}

func TestRunInitAgentFailsWhenLocalOnlyUsesNewTenantName(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	err := run([]string{
		"init-agent",
		"--local-only",
		"--new-tenant-name", "Acme",
		"--agent-name", "Support Bot",
		"--runtime", "python",
		"--tools", "slack:slack.channel.list",
	}, stdout, stderr)
	if err == nil || !strings.Contains(err.Error(), "--new-tenant-name is not supported with --local-only") {
		t.Fatalf("expected local-only new tenant failure, got err=%v stderr=%s", err, stderr.String())
	}
}

func TestRunInitAgentFailsWhenPrintOnlyAndNoFilesAreCombined(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	err := run([]string{
		"init-agent",
		"--local-only",
		"--tenant-id", "tenant-1",
		"--agent-name", "Support Bot",
		"--runtime", "python",
		"--tools", "slack:slack.channel.list",
		"--print-only",
		"--no-files",
	}, stdout, stderr)
	if err == nil || !strings.Contains(err.Error(), "--print-only and --no-files cannot be combined") {
		t.Fatalf("expected conflicting output flags failure, got err=%v stderr=%s", err, stderr.String())
	}
}

func TestRunBridgeChatRequiresConfigOrBaseURL(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	err := runWithIO([]string{"bridge", "chat"}, strings.NewReader(""), stdout, stderr)
	if err == nil || !strings.Contains(err.Error(), "--config or --base-url is required") {
		t.Fatalf("expected missing chat target failure, got err=%v stderr=%s", err, stderr.String())
	}
}

func TestRunBridgeMCPRequiresConfig(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	err := runWithIO([]string{"bridge", "mcp"}, strings.NewReader(""), stdout, stderr)
	if err == nil || err.Error() != "--config is required" {
		t.Fatalf("expected missing config error, got %v", err)
	}
}

func TestRunBridgeDoctorRequiresConfig(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	err := runWithIO([]string{"bridge", "doctor"}, strings.NewReader(""), stdout, stderr)
	if err == nil || err.Error() != "--config is required" {
		t.Fatalf("expected missing config error, got %v", err)
	}
}

func TestRunBridgeDoctorReturnsUnderlyingConfigError(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "openclause-bridge.yaml")
	if err := os.WriteFile(configPath, []byte("oops: true\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	err := runWithIO([]string{"bridge", "doctor", "--config", configPath}, strings.NewReader(""), stdout, stderr)
	if err == nil || !strings.Contains(err.Error(), "decode bridge config") {
		t.Fatalf("expected decode error to be returned, got err=%v stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), "report required") {
		t.Fatalf("did not expect masked report error, got stderr=%s", stderr.String())
	}
}

func TestRunBridgeDoctorValidatesGatewayAndOpenAI(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		case "/v1/connectors":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"name": "postgres", "actions": []string{"query.readonly"}}})
		case "/v1/toolcalls":
			if got := r.Header.Get("X-API-Key"); got != "sk-oc-demo" {
				t.Fatalf("expected bridge auth probe api key, got %q", got)
			}
			http.Error(w, `{"message":"invalid request"}`, http.StatusBadRequest)
		default:
			t.Fatalf("unexpected gateway path %s", r.URL.Path)
		}
	}))
	defer gateway.Close()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{"id": "qwen/qwen3.5-9b"}},
			})
		default:
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	configPath := filepath.Join(t.TempDir(), "openclause-bridge.yaml")
	configBody := strings.Join([]string{
		`base_url: "` + gateway.URL + `"`,
		`tenant_id: "tenant-1"`,
		`agent_id: "agent-1"`,
		`api_key: "sk-oc-demo"`,
		`tools:`,
		`  - tool: "postgres"`,
		`    action: "query.readonly"`,
		`    risk_score: 2`,
		`openai:`,
		`  upstream_base_url: "` + upstream.URL + `/v1"`,
		`  model: "qwen/qwen3.5-9b"`,
	}, "\n")
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	err := runWithIO([]string{"bridge", "doctor", "--config", configPath}, strings.NewReader(""), stdout, stderr)
	if err != nil {
		t.Fatalf("run bridge doctor: %v stderr=%s", err, stderr.String())
	}
	out := stdout.String()
	for _, needle := range []string{"gateway.health", "gateway.auth", "bridge.mcp", "openai.model", "Overall status: OK"} {
		if !strings.Contains(out, needle) {
			t.Fatalf("expected doctor output to mention %q, got %s", needle, out)
		}
	}
}

func TestRunBridgeDoctorJSONOutput(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		case "/v1/connectors":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"name": "postgres", "actions": []string{"query.readonly"}}})
		case "/v1/toolcalls":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"invalid request"}`))
		default:
			t.Fatalf("unexpected gateway path %s", r.URL.Path)
		}
	}))
	defer gateway.Close()

	configPath := filepath.Join(t.TempDir(), "openclause-bridge.yaml")
	configBody := strings.Join([]string{
		`base_url: "` + gateway.URL + `"`,
		`tenant_id: "tenant-1"`,
		`agent_id: "agent-1"`,
		`api_key: "sk-oc-demo"`,
		`tools:`,
		`  - tool: "postgres"`,
		`    action: "query.readonly"`,
		`    risk_score: 2`,
	}, "\n")
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	err := runWithIO([]string{"bridge", "doctor", "--config", configPath, "--json"}, strings.NewReader(""), stdout, stderr)
	if err != nil {
		t.Fatalf("run bridge doctor json: %v stderr=%s", err, stderr.String())
	}

	var report map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode doctor json: %v stdout=%s", err, stdout.String())
	}
	if got := report["status"]; got != "warn" {
		t.Fatalf("expected warn status for missing openai config, got %+v", report)
	}
	if _, ok := report["checks"]; !ok {
		t.Fatalf("expected checks in JSON report, got %+v", report)
	}
}

func TestRunBridgeDoctorFailsWhenGatewayRejectsAPIKey(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		case "/v1/connectors":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"name": "postgres", "actions": []string{"query.readonly"}}})
		case "/v1/toolcalls":
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		default:
			t.Fatalf("unexpected gateway path %s", r.URL.Path)
		}
	}))
	defer gateway.Close()

	configPath := filepath.Join(t.TempDir(), "openclause-bridge.yaml")
	configBody := strings.Join([]string{
		`base_url: "` + gateway.URL + `"`,
		`tenant_id: "tenant-1"`,
		`agent_id: "agent-1"`,
		`api_key: "sk-oc-bad"`,
		`tools:`,
		`  - tool: "postgres"`,
		`    action: "query.readonly"`,
		`    risk_score: 2`,
	}, "\n")
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	err := runWithIO([]string{"bridge", "doctor", "--config", configPath}, strings.NewReader(""), stdout, stderr)
	if err == nil || !strings.Contains(err.Error(), "bridge doctor found one or more failures") {
		t.Fatalf("expected doctor failure, got err=%v stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "Gateway rejected the configured API key") {
		t.Fatalf("expected auth recovery guidance, got %s", stdout.String())
	}
}

func TestRunBridgeChatPromptUsesBaseURL(t *testing.T) {
	bridgeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode chat request: %v", err)
		}
		if got := req["model"]; got != "qwen/qwen3.5-9b" {
			t.Fatalf("expected model in chat request, got %+v", req)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"role":    "assistant",
					"content": "The newest three demo users are Alice, Bob, and Charlie.",
				},
			}},
		})
	}))
	defer bridgeServer.Close()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	err := runWithIO([]string{
		"bridge", "chat",
		"--base-url", bridgeServer.URL,
		"--model", "qwen/qwen3.5-9b",
		"--prompt", "fetch demo users",
	}, strings.NewReader(""), stdout, stderr)
	if err != nil {
		t.Fatalf("run bridge chat prompt: %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Alice, Bob, and Charlie") {
		t.Fatalf("expected assistant content, got %s", stdout.String())
	}
}

func TestRunBridgeChatPromptCanTargetNamedProfile(t *testing.T) {
	bridgeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-OpenClause-Profile"); got != "tenant-b" {
			t.Fatalf("expected profile header, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"role":    "assistant",
					"content": "Profile-scoped bridge chat worked.",
				},
			}},
		})
	}))
	defer bridgeServer.Close()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	err := runWithIO([]string{
		"bridge", "chat",
		"--base-url", bridgeServer.URL,
		"--profile", "tenant-b",
		"--model", "qwen/qwen3.5-9b",
		"--prompt", "hello",
	}, strings.NewReader(""), stdout, stderr)
	if err != nil {
		t.Fatalf("run bridge chat prompt with profile: %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Profile-scoped bridge chat worked.") {
		t.Fatalf("expected assistant content, got %s", stdout.String())
	}
}

func TestRunBridgeChatPromptUsesConfigDefaults(t *testing.T) {
	bridgeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode chat request: %v", err)
		}
		if got := req["model"]; got != "qwen/qwen3.5-9b" {
			t.Fatalf("expected model from config, got %+v", req)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"role":    "assistant",
					"content": "Configured bridge chat worked.",
				},
			}},
		})
	}))
	defer bridgeServer.Close()

	configPath := filepath.Join(t.TempDir(), "openclause-bridge.yaml")
	configBody := strings.Join([]string{
		`listen: "` + strings.TrimPrefix(bridgeServer.URL, "http://") + `"`,
		`base_url: "http://localhost:8080"`,
		`tenant_id: "tenant-1"`,
		`agent_id: "agent-1"`,
		`api_key: "sk-oc-demo"`,
		`openai:`,
		`  upstream_base_url: "http://localhost:1234/v1"`,
		`  model: "qwen/qwen3.5-9b"`,
	}, "\n")
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	err := runWithIO([]string{
		"bridge", "chat",
		"--config", configPath,
		"--prompt", "hello",
	}, strings.NewReader(""), stdout, stderr)
	if err != nil {
		t.Fatalf("run bridge chat via config: %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Configured bridge chat worked.") {
		t.Fatalf("expected configured assistant content, got %s", stdout.String())
	}
}

func TestRunBridgeChatPromptUsesSelectedProfileFromConfig(t *testing.T) {
	bridgeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-OpenClause-Profile"); got != "finance" {
			t.Fatalf("expected profile header, got %q", got)
		}
		var req struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode chat request: %v", err)
		}
		if req.Model != "finance-model" {
			t.Fatalf("expected model from selected profile, got %+v", req)
		}
		if len(req.Messages) < 2 {
			t.Fatalf("expected system and user messages, got %+v", req.Messages)
		}
		if req.Messages[0].Role != "system" || req.Messages[0].Content != "Finance-only instructions." {
			t.Fatalf("expected selected profile system prompt, got %+v", req.Messages[0])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"role":    "assistant",
					"content": "Finance profile bridge chat worked.",
				},
			}},
		})
	}))
	defer bridgeServer.Close()

	configPath := filepath.Join(t.TempDir(), "openclause-bridge.yaml")
	configBody := strings.Join([]string{
		`listen: "` + strings.TrimPrefix(bridgeServer.URL, "http://") + `"`,
		`default_profile: "support"`,
		`profiles:`,
		`  support:`,
		`    base_url: "http://localhost:8080"`,
		`    tenant_id: "tenant-support"`,
		`    agent_id: "agent-support"`,
		`    api_key: "sk-support"`,
		`    openai:`,
		`      upstream_base_url: "http://localhost:1234/v1"`,
		`      model: "support-model"`,
		`      system_prompt: "Support-only instructions."`,
		`  finance:`,
		`    base_url: "http://localhost:8080"`,
		`    tenant_id: "tenant-finance"`,
		`    agent_id: "agent-finance"`,
		`    api_key: "sk-finance"`,
		`    openai:`,
		`      upstream_base_url: "http://localhost:1234/v1"`,
		`      model: "finance-model"`,
		`      system_prompt: "Finance-only instructions."`,
	}, "\n")
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	err := runWithIO([]string{
		"bridge", "chat",
		"--config", configPath,
		"--profile", "finance",
		"--prompt", "hello",
	}, strings.NewReader(""), stdout, stderr)
	if err != nil {
		t.Fatalf("run bridge chat via selected profile: %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Finance profile bridge chat worked.") {
		t.Fatalf("expected assistant content from selected profile, got %s", stdout.String())
	}
}

func TestRunBridgeChatInteractiveSupportsResetAndQuit(t *testing.T) {
	var requestBodies []map[string]any
	bridgeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode chat request: %v", err)
		}
		requestBodies = append(requestBodies, req)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"role":    "assistant",
					"content": "OK",
				},
			}},
		})
	}))
	defer bridgeServer.Close()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	input := strings.NewReader("hello\n/reset\nhello again\n/exit\n")
	err := runWithIO([]string{
		"bridge", "chat",
		"--base-url", bridgeServer.URL,
		"--model", "qwen/qwen3.5-9b",
	}, input, stdout, stderr)
	if err != nil {
		t.Fatalf("run interactive bridge chat: %v stderr=%s", err, stderr.String())
	}
	if len(requestBodies) != 2 {
		t.Fatalf("expected two chat requests around reset, got %d", len(requestBodies))
	}
	firstMessages, _ := requestBodies[0]["messages"].([]any)
	secondMessages, _ := requestBodies[1]["messages"].([]any)
	if len(firstMessages) != 1 || len(secondMessages) != 1 {
		t.Fatalf("expected reset to clear history, got first=%+v second=%+v", firstMessages, secondMessages)
	}
	if !strings.Contains(stdout.String(), "Conversation reset.") || !strings.Contains(stdout.String(), "Bye.") {
		t.Fatalf("expected reset and quit feedback, got %s", stdout.String())
	}
}

func TestRunBridgeMCPStdioUsesConfig(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/toolcalls" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"event_id": "evt-1",
			"decision": "allow",
			"reason":   "ok",
			"result": map[string]any{
				"status":      "success",
				"output_json": map[string]any{"row_count": 1},
			},
		})
	}))
	defer upstream.Close()

	configPath := filepath.Join(t.TempDir(), "openclause-bridge.yaml")
	configBody := strings.Join([]string{
		`base_url: "` + upstream.URL + `"`,
		`tenant_id: "tenant-1"`,
		`agent_id: "agent-1"`,
		`api_key: "sk-oc-demo"`,
		`tools:`,
		`  - tool: "postgres"`,
		`    action: "query.readonly"`,
		`    risk_score: 2`,
	}, "\n")
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
	}, "\n")
	err := runWithIO([]string{"bridge", "mcp", "--config", configPath}, strings.NewReader(input), stdout, stderr)
	if err != nil {
		t.Fatalf("run bridge mcp: %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"protocolVersion":"2025-06-18"`) || !strings.Contains(stdout.String(), `"openclause_postgres_query_readonly"`) {
		t.Fatalf("expected MCP stdio responses, got %s", stdout.String())
	}
}

func TestRunBridgeMCPStdioUsesSelectedProfileFromConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "openclause-bridge.yaml")
	configBody := strings.Join([]string{
		`default_profile: "support"`,
		`profiles:`,
		`  support:`,
		`    base_url: "http://localhost:8080"`,
		`    tenant_id: "tenant-support"`,
		`    agent_id: "agent-support"`,
		`    api_key: "sk-support"`,
		`    tools:`,
		`      - tool: "postgres"`,
		`        action: "query.readonly"`,
		`        risk_score: 2`,
		`  finance:`,
		`    base_url: "http://localhost:8080"`,
		`    tenant_id: "tenant-finance"`,
		`    agent_id: "agent-finance"`,
		`    api_key: "sk-finance"`,
		`    tools:`,
		`      - tool: "jira"`,
		`        action: "issue.read"`,
		`        risk_score: 2`,
	}, "\n")
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
	}, "\n")
	err := runWithIO([]string{"bridge", "mcp", "--config", configPath, "--profile", "finance"}, strings.NewReader(input), stdout, stderr)
	if err != nil {
		t.Fatalf("run bridge mcp with selected profile: %v stderr=%s", err, stderr.String())
	}
	body := stdout.String()
	if !strings.Contains(body, `"openclause_jira_issue_read"`) {
		t.Fatalf("expected selected profile MCP tools, got %s", body)
	}
	if strings.Contains(body, `"openclause_postgres_query_readonly"`) {
		t.Fatalf("did not expect default profile tool in selected profile output, got %s", body)
	}
}

func TestRunBridgeChatAcceptsStructuredAssistantContent(t *testing.T) {
	bridgeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"role": "assistant",
					"content": []map[string]any{
						{"type": "text", "text": "Structured "},
						{"type": "text", "text": "reply"},
					},
				},
			}},
		})
	}))
	defer bridgeServer.Close()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	err := runWithIO([]string{
		"bridge", "chat",
		"--base-url", bridgeServer.URL,
		"--model", "qwen/qwen3.5-9b",
		"--prompt", "hello",
	}, strings.NewReader(""), stdout, stderr)
	if err != nil {
		t.Fatalf("run bridge chat structured content: %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Structured reply") {
		t.Fatalf("expected flattened structured content, got %s", stdout.String())
	}
}

func TestBridgeLocalBaseURLPrefersLoopbackForWildcardListeners(t *testing.T) {
	if got := bridgeLocalBaseURL(":8787"); got != "http://127.0.0.1:8787" {
		t.Fatalf("expected loopback base url for bare port, got %q", got)
	}
	if got := bridgeLocalBaseURL("0.0.0.0:8787"); got != "http://127.0.0.1:8787" {
		t.Fatalf("expected loopback base url for wildcard host, got %q", got)
	}
}

func newCLIOnboardingServer(t *testing.T, assertPayload func(path string, body map[string]any)) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			t.Fatalf("unexpected auth header: %s", got)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if assertPayload != nil {
			assertPayload(r.URL.Path, body)
		}

		switch r.URL.Path {
		case "/admin/onboarding/integrations":
			writeJSONResponse(t, w, http.StatusCreated, sampleServerBundleResponse("created"))
		case "/admin/onboarding/bundles/preview":
			writeJSONResponse(t, w, http.StatusOK, sampleServerBundleResponse("preview"))
		case "/admin/onboarding/bundles/regenerate":
			writeJSONResponse(t, w, http.StatusOK, sampleServerBundleResponse("regenerated"))
		case "/admin/onboarding/bundles/regenerate-defaults":
			writeJSONResponse(t, w, http.StatusOK, sampleServerBundleResponse("regenerated_defaults"))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
}

func sampleServerBundleResponse(mode string) onboarding.BundleResponse {
	req := onboarding.BundleRequest{
		BaseURL:         "http://localhost:8080",
		TenantID:        "tenant-1",
		TenantName:      "Alpha Corp",
		AgentID:         "agent-1",
		AgentName:       "Support Bot",
		Runtime:         onboarding.RuntimePython,
		ApprovalPosture: "pilot_safe",
		Tools:           []onboarding.SelectedTool{{Tool: "slack", Action: "slack.channel.list"}},
	}

	switch mode {
	case "created":
		req.APIKey = "sk-oc-demo-raw"
		req.APIKeyMode = onboarding.APIKeyModeRawProvided
		req.APIKeyPrefix = "sk-oc-de"
	case "preview":
		req.AgentID = onboarding.PreviewAgentID(req.AgentName)
		req.APIKey = "${OPENCLAUSE_API_KEY:-generated-on-create}"
		req.APIKeyMode = onboarding.APIKeyModePreview
	case "regenerated":
		req.APIKey = "${OPENCLAUSE_API_KEY:-reuse-existing-key}"
		req.APIKeyMode = onboarding.APIKeyModeExistingKeyRef
		req.APIKeyPrefix = "sk-oc-ex"
	case "regenerated_defaults":
		req.APIKey = "${OPENCLAUSE_API_KEY:-reuse-existing-key}"
		req.APIKeyMode = onboarding.APIKeyModeExistingKeyRef
		req.APIKeyPrefix = "sk-oc-ex"
	}

	bundle, _ := onboarding.BuildBundle(req)
	resp := onboarding.BundleResponse{
		Mode: mode,
		Tenant: onboarding.BundleTenant{
			ID:   "tenant-1",
			Name: "Alpha Corp",
		},
		Agent: onboarding.BundleAgent{
			ID:   req.AgentID,
			Name: "Support Bot",
		},
		Bundle: bundle,
	}

	switch mode {
	case "created":
		resp.Agent.Status = "active"
		resp.APIKey = &onboarding.BundleAPIKey{
			ID:        "key-1",
			Name:      "Support Bot onboarding key",
			KeyPrefix: "sk-oc-de",
			RawKey:    "sk-oc-demo-raw",
		}
	case "preview":
		resp.Agent.Status = "preview"
		resp.Agent.Preview = true
	case "regenerated":
		resp.Agent.Status = "active"
		resp.APIKey = &onboarding.BundleAPIKey{
			ID:        "key-1",
			Name:      "Existing onboarding key",
			KeyPrefix: "sk-oc-ex",
			RawKey:    "",
		}
	case "regenerated_defaults":
		resp.Agent.Status = "active"
		resp.APIKey = &onboarding.BundleAPIKey{
			ID:        "key-1",
			Name:      "Existing onboarding key",
			KeyPrefix: "sk-oc-ex",
		}
		resp.Bundle.AppliedDefaults = []onboarding.BundleDefault{
			{Field: "runtime", Value: "python", Reason: "OpenClause v0.5 golden-path default"},
			{Field: "approval_posture", Value: "pilot_safe", Reason: "Recommended pilot-safe default"},
			{Field: "tool", Value: "slack:slack.channel.list", Reason: "First curated tool available in the connector catalog"},
		}
	}

	return resp
}

func writeJSONResponse(t *testing.T, w http.ResponseWriter, status int, payload onboarding.BundleResponse) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
