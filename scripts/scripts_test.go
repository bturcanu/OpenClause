package scripts_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateEnvScriptPassesForHealthyConfig(t *testing.T) {
	envPath := writeTempEnv(t, map[string]string{
		"POSTGRES_PASSWORD":      "super-secret-password",
		"INTERNAL_AUTH_TOKEN":    "super-secret-internal-token-1234567890",
		"CONSOLE_JWT_SECRET":     "super-secret-console-jwt-secret-1234567890",
		"OPA_URL":                "http://localhost:8181",
		"APPROVALS_URL":          "http://localhost:8081",
		"PUBLIC_APPROVALS_URL":   "https://localhost:8081",
		"GATEWAY_URL":            "http://localhost:8080",
		"PUBLIC_GATEWAY_URL":     "http://localhost:8080",
		"PUBLIC_BASE_URL":        "https://localhost:3000",
		"POSTGRES_SSLMODE":       "require",
		"EVIDENCE_S3_ACCESS_KEY": "not-placeholder",
		"EVIDENCE_S3_SECRET_KEY": "not-placeholder",
		"SLACK_BOT_TOKEN":        "not-placeholder",
		"JIRA_API_TOKEN":         "not-placeholder",
	})

	out := runBash(t, "./validate-env.sh", "--file", envPath)
	if !strings.Contains(out, "env validation passed") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestValidateEnvScriptFailsStrictOnPlaceholders(t *testing.T) {
	envPath := writeTempEnv(t, map[string]string{
		"POSTGRES_PASSWORD":    "changeme",
		"INTERNAL_AUTH_TOKEN":  "dev-internal-token-change-me",
		"CONSOLE_JWT_SECRET":   "change-me-in-production-openclause-jwt-secret",
		"OPA_URL":              "http://localhost:8181",
		"APPROVALS_URL":        "http://localhost:8081",
		"PUBLIC_APPROVALS_URL": "https://localhost:8081",
		"GATEWAY_URL":          "http://localhost:8080",
		"PUBLIC_GATEWAY_URL":   "http://localhost:8080",
		"PUBLIC_BASE_URL":      "https://localhost:3000",
		"POSTGRES_SSLMODE":     "disable",
	})

	cmd := exec.Command("bash", "./validate-env.sh", "--file", envPath, "--strict")
	cmd.Dir = "."
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected strict validation failure, output=%s", string(out))
	}
	if !strings.Contains(string(out), "placeholder/dev value") {
		t.Fatalf("expected placeholder warning/error, output=%s", string(out))
	}
}

func TestValidateEnvScriptWarnsOnPlaceholderConsoleJWTSecret(t *testing.T) {
	envPath := writeTempEnv(t, map[string]string{
		"POSTGRES_PASSWORD":    "super-secret-password",
		"INTERNAL_AUTH_TOKEN":  "super-secret-internal-token-1234567890",
		"CONSOLE_JWT_SECRET":   "change-me-in-production-openclause-jwt-secret",
		"OPA_URL":              "http://localhost:8181",
		"APPROVALS_URL":        "http://localhost:8081",
		"PUBLIC_APPROVALS_URL": "https://localhost:8081",
		"GATEWAY_URL":          "http://localhost:8080",
		"PUBLIC_GATEWAY_URL":   "http://localhost:8080",
		"PUBLIC_BASE_URL":      "https://localhost:3000",
	})

	out := runBash(t, "./validate-env.sh", "--file", envPath)
	if !strings.Contains(out, "CONSOLE_JWT_SECRET uses placeholder/dev value") {
		t.Fatalf("expected placeholder warning for CONSOLE_JWT_SECRET, output=%s", out)
	}
}

func TestPostStartSmokeScriptPassesAgainstLocalServers(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			_, _ = w.Write([]byte("OK"))
		case "/v1/connectors":
			_, _ = w.Write([]byte(`[{"name":"postgres"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer gateway.Close()

	healthServer := func() *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/healthz":
				_, _ = w.Write([]byte("OK"))
			case "/health":
				_, _ = w.Write([]byte(`{}`))
			case "/setup/status":
				_, _ = w.Write([]byte(`{"initialized":true}`))
			default:
				http.NotFound(w, r)
			}
		}))
		return srv
	}

	approvals := healthServer()
	defer approvals.Close()
	slack := healthServer()
	defer slack.Close()
	jira := healthServer()
	defer jira.Close()
	consoleAPI := healthServer()
	defer consoleAPI.Close()
	opa := healthServer()
	defer opa.Close()

	cmd := exec.Command("bash", "./post-start-smoke.sh")
	cmd.Dir = "."
	cmd.Env = append(os.Environ(),
		"GATEWAY_URL="+gateway.URL,
		"APPROVALS_URL="+approvals.URL,
		"CONNECTOR_SLACK_URL="+slack.URL,
		"CONNECTOR_JIRA_URL="+jira.URL,
		"CONSOLE_API_URL="+consoleAPI.URL,
		"OPA_URL="+opa.URL,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected smoke to pass, err=%v output=%s", err, string(out))
	}
	if !strings.Contains(string(out), "post-start smoke passed") {
		t.Fatalf("unexpected smoke output: %s", string(out))
	}
}

func TestPostStartSmokeScriptFailsWhenOPAHealthBodyIsUnexpected(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			_, _ = w.Write([]byte("OK"))
		case "/v1/connectors":
			_, _ = w.Write([]byte(`[{"name":"postgres"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer gateway.Close()

	defaultHealth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			_, _ = w.Write([]byte("OK"))
		case "/setup/status":
			_, _ = w.Write([]byte(`{"initialized":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer defaultHealth.Close()

	opa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer opa.Close()

	cmd := exec.Command("bash", "./post-start-smoke.sh")
	cmd.Dir = "."
	cmd.Env = append(os.Environ(),
		"GATEWAY_URL="+gateway.URL,
		"APPROVALS_URL="+defaultHealth.URL,
		"CONNECTOR_SLACK_URL="+defaultHealth.URL,
		"CONNECTOR_JIRA_URL="+defaultHealth.URL,
		"CONSOLE_API_URL="+defaultHealth.URL,
		"OPA_URL="+opa.URL,
	)

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected smoke to fail when OPA body is invalid, output=%s", string(out))
	}
	if !strings.Contains(string(out), "OPA health response") {
		t.Fatalf("expected OPA marker failure, output=%s", string(out))
	}
}

func TestEnsureSetupInitializedBootstrapsFreshInstance(t *testing.T) {
	t.Helper()

	type initializeRequest struct {
		OrgName         string `json:"org_name"`
		Email           string `json:"email"`
		Password        string `json:"password"`
		FirstTenantName string `json:"first_tenant_name"`
	}

	var (
		initialized    bool
		initializeHits int
		received       initializeRequest
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/setup/status":
			_, _ = w.Write([]byte(`{"initialized":false}`))
		case "/setup/initialize":
			initializeHits++
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			initialized = true
			_, _ = w.Write([]byte(`{"initialized":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cmd := exec.Command("bash", "-lc", "source ./smoke-lib.sh && ensure_setup_initialized \"$BASE_URL\" \"$ADMIN_EMAIL\" \"$ADMIN_PASSWORD\" \"$ORG_NAME\" \"$FIRST_TENANT_NAME\"")
	cmd.Dir = "."
	cmd.Env = append(os.Environ(),
		"BASE_URL="+server.URL,
		"ADMIN_EMAIL=admin@openclause.dev",
		"ADMIN_PASSWORD=Admin123!",
		"ORG_NAME=Smoke Org",
		"FIRST_TENANT_NAME=Smoke Tenant",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ensure_setup_initialized failed: %v output=%s", err, string(out))
	}
	if !initialized || initializeHits != 1 {
		t.Fatalf("expected one setup initialize call, initialized=%v hits=%d", initialized, initializeHits)
	}
	if received.Email != "admin@openclause.dev" || received.Password != "Admin123!" {
		t.Fatalf("unexpected initialize credentials: %+v", received)
	}
	if received.OrgName != "Smoke Org" || received.FirstTenantName != "Smoke Tenant" {
		t.Fatalf("unexpected initialize payload: %+v", received)
	}
}

func TestEnsureSetupInitializedSkipsAlreadyInitializedInstance(t *testing.T) {
	t.Helper()

	var initializeHits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/setup/status":
			_, _ = w.Write([]byte(`{"initialized":true}`))
		case "/setup/initialize":
			initializeHits++
			_, _ = w.Write([]byte(`{"initialized":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cmd := exec.Command("bash", "-lc", "source ./smoke-lib.sh && ensure_setup_initialized \"$BASE_URL\" \"$ADMIN_EMAIL\" \"$ADMIN_PASSWORD\" \"$ORG_NAME\" \"$FIRST_TENANT_NAME\"")
	cmd.Dir = "."
	cmd.Env = append(os.Environ(),
		"BASE_URL="+server.URL,
		"ADMIN_EMAIL=admin@openclause.dev",
		"ADMIN_PASSWORD=Admin123!",
		"ORG_NAME=Smoke Org",
		"FIRST_TENANT_NAME=Smoke Tenant",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ensure_setup_initialized failed: %v output=%s", err, string(out))
	}
	if initializeHits != 0 {
		t.Fatalf("expected no initialize calls, got %d", initializeHits)
	}
}

func TestLoginConsoleAdminReturnsToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/login" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"token":"tok-123"}`))
	}))
	defer server.Close()

	cmd := exec.Command("bash", "-lc", "source ./smoke-lib.sh && login_console_admin \"$BASE_URL\" \"$ADMIN_EMAIL\" \"$ADMIN_PASSWORD\"")
	cmd.Dir = "."
	cmd.Env = append(os.Environ(),
		"BASE_URL="+server.URL,
		"ADMIN_EMAIL=admin@openclause.dev",
		"ADMIN_PASSWORD=Admin123!",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("login_console_admin failed: %v output=%s", err, string(out))
	}
	if strings.TrimSpace(string(out)) != "tok-123" {
		t.Fatalf("expected token output, got %q", string(out))
	}
}

func TestLoginConsoleAdminFailsWhenTokenMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/login" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"token":null}`))
	}))
	defer server.Close()

	cmd := exec.Command("bash", "-lc", "source ./smoke-lib.sh && login_console_admin \"$BASE_URL\" \"$ADMIN_EMAIL\" \"$ADMIN_PASSWORD\"")
	cmd.Dir = "."
	cmd.Env = append(os.Environ(),
		"BASE_URL="+server.URL,
		"ADMIN_EMAIL=admin@openclause.dev",
		"ADMIN_PASSWORD=Admin123!",
	)

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected login_console_admin to fail, output=%s", string(out))
	}
}

func writeTempEnv(t *testing.T, values map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")

	var b strings.Builder
	for k, v := range values {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(v)
		b.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	return path
}

func runBash(t *testing.T, script string, args ...string) string {
	t.Helper()

	fullArgs := append([]string{script}, args...)
	cmd := exec.Command("bash", fullArgs...)
	cmd.Dir = "."
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command %v failed: %v output=%s", fullArgs, err, string(out))
	}
	return string(out)
}
