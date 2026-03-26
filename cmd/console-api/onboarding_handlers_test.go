package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bturcanu/OpenClause/pkg/connectors"
	"github.com/bturcanu/OpenClause/pkg/console"
	"github.com/bturcanu/OpenClause/pkg/onboarding"
)

func newOnboardingAPIFixture(t *testing.T, connectorPayload string) *dbAPIFixture {
	t.Helper()
	fx := newDBAPIFixture(t)
	fx.api.gatewayURL = "http://gateway.example.test"
	fx.api.publicGatewayURL = "http://localhost:8080"
	fx.api.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "http://gateway.example.test/v1/connectors" {
				t.Fatalf("unexpected connector request URL: %s", req.URL.String())
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewBufferString(connectorPayload)),
			}, nil
		}),
	}
	return fx
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func countRowsMatchingValue(t *testing.T, store *console.Store, table, column, value string) int {
	t.Helper()
	var count int
	if err := store.Pool().QueryRow(context.Background(), `SELECT COUNT(*) FROM `+table+` WHERE `+column+` = $1`, value).Scan(&count); err != nil {
		t.Fatalf("count rows in %s: %v", table, err)
	}
	return count
}

func TestHandleCreateOnboardingIntegrationCreatesTenantAgentKeyAndBundle(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[{"name":"slack","actions":["slack.channel.list","slack.msg.post"],"type":"remote"},{"name":"github","actions":["issue.create"],"type":"builtin"}]`)

	body := `{
		"runtime":"python",
		"new_tenant_name":"Pilot Tenant",
		"agent_name":"Support Bot",
		"environment_label":"dev",
		"owner_name":"AI Platform",
		"approval_posture":"pilot_safe",
		"tools":[
			{"tool":"slack","action":"slack.channel.list"},
			{"tool":"slack","action":"slack.msg.post"}
		]
	}`

	req := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/onboarding/integrations", bytes.NewBufferString(body)),
		&console.JWTClaims{Roles: []string{"platform_admin"}},
	)
	rr := httptest.NewRecorder()
	fx.api.handleCreateOnboardingIntegration(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"raw_key":"`) {
		t.Fatalf("expected create response JSON to include raw_key, got %s", rr.Body.String())
	}

	var resp struct {
		Mode   string `json:"mode"`
		Tenant struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Created bool   `json:"created"`
		} `json:"tenant"`
		Agent struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"agent"`
		Integration *struct {
			ID               string `json:"id"`
			TenantID         string `json:"tenant_id"`
			AgentID          string `json:"agent_id"`
			Runtime          string `json:"runtime"`
			EnvironmentLabel string `json:"environment_label"`
			OwnerName        string `json:"owner_name"`
			ApprovalPosture  string `json:"approval_posture"`
		} `json:"integration"`
		APIKey *struct {
			RawKey string `json:"raw_key"`
		} `json:"api_key"`
		Bundle struct {
			Runtime         string            `json:"runtime"`
			StarterFileName string            `json:"starter_file_name"`
			Environment     map[string]string `json:"environment"`
			EnvironmentFile string            `json:"environment_file"`
			StarterSnippet  string            `json:"starter_snippet"`
			ReadmeSnippet   string            `json:"readme_snippet"`
			SampleCall      string            `json:"sample_call"`
			Artifacts       []struct {
				FileName string `json:"file_name"`
				Kind     string `json:"kind"`
			} `json:"artifacts"`
			VerificationLinks []struct {
				Label string `json:"label"`
				Path  string `json:"path"`
			} `json:"verification_links"`
		} `json:"bundle"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !resp.Tenant.Created || resp.Tenant.ID == "" || resp.Tenant.Name != "Pilot Tenant" {
		t.Fatalf("unexpected tenant payload: %+v", resp.Tenant)
	}
	if resp.Mode != "created" {
		t.Fatalf("expected created mode, got %q", resp.Mode)
	}
	if resp.Agent.ID == "" || resp.Agent.Name != "Support Bot" {
		t.Fatalf("unexpected agent payload: %+v", resp.Agent)
	}
	if resp.Integration == nil || resp.Integration.AgentID != resp.Agent.ID || resp.Integration.TenantID != resp.Tenant.ID {
		t.Fatalf("expected integration payload to round-trip created tenant/agent, got %+v", resp.Integration)
	}
	if resp.Integration.Runtime != "python" || resp.Integration.EnvironmentLabel != "dev" || resp.Integration.OwnerName != "AI Platform" || resp.Integration.ApprovalPosture != "pilot_safe" {
		t.Fatalf("unexpected integration payload: %+v", resp.Integration)
	}
	if resp.APIKey == nil || resp.APIKey.RawKey == "" {
		t.Fatalf("expected raw api key in onboarding response")
	}
	if resp.Bundle.Runtime != "python" || resp.Bundle.StarterFileName != "agent.py" {
		t.Fatalf("unexpected bundle metadata: %+v", resp.Bundle)
	}
	if got := resp.Bundle.Environment["OPENCLAUSE_TENANT_ID"]; got != resp.Tenant.ID {
		t.Fatalf("expected bundle tenant env %q, got %q", resp.Tenant.ID, got)
	}
	if got := resp.Bundle.Environment["OPENCLAUSE_BASE_URL"]; got != "http://localhost:8080" {
		t.Fatalf("expected public gateway base url in bundle env, got %q", got)
	}
	if got := resp.Bundle.Environment["OPENCLAUSE_AGENT_ID"]; got != resp.Agent.ID {
		t.Fatalf("expected bundle agent env %q, got %q", resp.Agent.ID, got)
	}
	if !strings.Contains(resp.Bundle.StarterSnippet, "governed_call") {
		t.Fatalf("expected python starter snippet, got %s", resp.Bundle.StarterSnippet)
	}
	if !strings.Contains(resp.Bundle.EnvironmentFile, "OPENCLAUSE_API_KEY") {
		t.Fatalf("expected .env example content, got %s", resp.Bundle.EnvironmentFile)
	}
	if !strings.Contains(resp.Bundle.ReadmeSnippet, "Quick start") {
		t.Fatalf("expected readme snippet, got %s", resp.Bundle.ReadmeSnippet)
	}
	if len(resp.Bundle.Artifacts) < 4 {
		t.Fatalf("expected richer bundle artifacts, got %+v", resp.Bundle.Artifacts)
	}
	if len(resp.Bundle.VerificationLinks) != 3 {
		t.Fatalf("expected verification links, got %+v", resp.Bundle.VerificationLinks)
	}
	if !strings.Contains(resp.Bundle.SampleCall, `"tool": "slack"`) {
		t.Fatalf("expected sample call to target selected tool, got %s", resp.Bundle.SampleCall)
	}

	agents, err := fx.store.ListAgents(context.Background(), resp.Tenant.ID, 20, 0)
	if err != nil {
		t.Fatalf("ListAgents after create: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected one created agent, got %+v", agents)
	}
	if string(agents[0].Labels) != "{}" {
		t.Fatalf("expected create to avoid mirrored onboarding labels, got %s", string(agents[0].Labels))
	}
	integration, err := fx.store.GetAgentIntegration(context.Background(), resp.Tenant.ID, resp.Agent.ID)
	if err != nil {
		t.Fatalf("GetAgentIntegration after create: %v", err)
	}
	if integration.ID != resp.Integration.ID || integration.Runtime != "python" || integration.EnvironmentLabel != "dev" || integration.OwnerName != "AI Platform" || integration.ApprovalPosture != "pilot_safe" || len(integration.Tools) != 2 {
		t.Fatalf("expected persisted integration record, got %+v", integration)
	}
}

func TestHandleCreateOnboardingIntegrationFailsBeforePersistenceWhenBundleBaseURLIsMissing(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[{"name":"slack","actions":["slack.channel.list"],"type":"remote"}]`)
	fx.api.publicGatewayURL = ""
	fx.api.gatewayURL = ""
	fx.api.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "http://localhost:8080/v1/connectors" {
				t.Fatalf("unexpected connector request URL: %s", req.URL.String())
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewBufferString(`[{"name":"slack","actions":["slack.channel.list"],"type":"remote"}]`)),
			}, nil
		}),
	}

	body := `{
		"runtime":"python",
		"new_tenant_name":"Missing Base URL Tenant",
		"agent_name":"Missing Base URL Bot",
		"tools":[{"tool":"slack","action":"slack.channel.list"}]
	}`

	req := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/onboarding/integrations", bytes.NewBufferString(body)),
		&console.JWTClaims{Roles: []string{"platform_admin"}},
	)
	rr := httptest.NewRecorder()
	fx.api.handleCreateOnboardingIntegration(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "onboarding bundle base URL is not configured") {
		t.Fatalf("expected bundle base url error, got %s", rr.Body.String())
	}
	if got := countRowsMatchingValue(t, fx.store, "tenants", "name", "Missing Base URL Tenant"); got != 0 {
		t.Fatalf("expected no tenant rows, found %d", got)
	}
	if got := countRowsMatchingValue(t, fx.store, "agents", "name", "Missing Base URL Bot"); got != 0 {
		t.Fatalf("expected no agent rows, found %d", got)
	}
}

func TestHandleCreateOnboardingIntegrationAppliesStarterPolicyForPilotSafe(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[{"name":"postgres","actions":["query.readonly"],"type":"builtin"},{"name":"github","actions":["issue.create"],"type":"builtin"}]`)

	body := `{
		"runtime":"openai_local",
		"new_tenant_name":"Pilot Policy Tenant",
		"agent_name":"Qwen Bot",
		"approval_posture":"pilot_safe",
		"tools":[
			{"tool":"postgres","action":"query.readonly"},
			{"tool":"github","action":"issue.create"}
		]
	}`

	req := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/onboarding/integrations", bytes.NewBufferString(body)),
		&console.JWTClaims{Roles: []string{"platform_admin"}},
	)
	rr := httptest.NewRecorder()
	fx.api.handleCreateOnboardingIntegration(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Tenant struct {
			ID string `json:"id"`
		} `json:"tenant"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	cfg, found, err := fx.store.GetTenantPolicyConfig(context.Background(), resp.Tenant.ID)
	if err != nil {
		t.Fatalf("GetTenantPolicyConfig: %v", err)
	}
	if !found || cfg == nil {
		t.Fatalf("expected starter policy config to be persisted")
	}
	if cfg.MaxRiskAutoApprove != 4 {
		t.Fatalf("expected starter policy max risk 4, got %+v", cfg)
	}
	if !cfg.RequireDestructiveApproval {
		t.Fatalf("expected destructive approval to stay enabled, got %+v", cfg)
	}
	if len(cfg.ReadActions) != 1 || cfg.ReadActions[0] != "postgres.query.readonly" {
		t.Fatalf("expected postgres readonly action in read allowlist, got %+v", cfg.ReadActions)
	}
	if len(cfg.DestructiveActions) != 1 || cfg.DestructiveActions[0] != "github.issue.create" {
		t.Fatalf("expected github create action in approval allowlist, got %+v", cfg.DestructiveActions)
	}
}

func TestHandleCreateOnboardingIntegrationCanonicalizesPrefixedActionNames(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[{"name":"slack","actions":["channel.list","msg.post"],"type":"remote"}]`)

	body := `{
		"runtime":"python",
		"new_tenant_name":"Prefixed Tool Tenant",
		"agent_name":"Prefixed Bot",
		"approval_posture":"tenant_default",
		"tools":[{"tool":"Slack","action":"slack.channel.list"}]
	}`

	req := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/onboarding/integrations", bytes.NewBufferString(body)),
		&console.JWTClaims{Roles: []string{"platform_admin"}},
	)
	rr := httptest.NewRecorder()
	fx.api.handleCreateOnboardingIntegration(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Tenant struct {
			ID string `json:"id"`
		} `json:"tenant"`
		Bundle struct {
			Environment map[string]string `json:"environment"`
		} `json:"bundle"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := resp.Bundle.Environment["OPENCLAUSE_API_KEY"]; got == "" {
		t.Fatalf("expected bundle env to be populated")
	}

	agents, err := fx.store.ListAgents(context.Background(), resp.Tenant.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected one created agent, got %+v", agents)
	}
	integration, err := fx.store.GetAgentIntegration(context.Background(), resp.Tenant.ID, agents[0].ID)
	if err != nil {
		t.Fatalf("GetAgentIntegration: %v", err)
	}
	if len(integration.Tools) != 1 || integration.Tools[0].Tool != "slack" || integration.Tools[0].Action != "channel.list" {
		t.Fatalf("expected canonicalized persisted integration tool selection, got %+v", integration.Tools)
	}
}

func TestHandleGetAgentIntegrationReturnsPersistedRecord(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[{"name":"postgres","actions":["query.readonly"],"type":"builtin"}]`)

	body := `{
		"runtime":"typescript",
		"new_tenant_name":"Lifecycle Tenant",
		"agent_name":"Lifecycle Bot",
		"environment_label":"prod",
		"owner_name":"Runtime Team",
		"description":"Lifecycle test",
		"approval_posture":"pilot_safe",
		"tools":[{"tool":"postgres","action":"query.readonly"}]
	}`
	createReq := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/onboarding/integrations", bytes.NewBufferString(body)),
		&console.JWTClaims{Roles: []string{"platform_admin"}},
	)
	createRec := httptest.NewRecorder()
	fx.api.handleCreateOnboardingIntegration(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create to succeed, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	var created onboarding.BundleResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	req := withClaims(
		httptest.NewRequest(http.MethodGet, "/admin/tenants/"+created.Tenant.ID+"/agents/"+created.Agent.ID+"/integration", http.NoBody),
		&console.JWTClaims{Roles: []string{"platform_admin"}},
	)
	req.SetPathValue("tenant_id", created.Tenant.ID)
	req.SetPathValue("agent_id", created.Agent.ID)
	rec := httptest.NewRecorder()
	fx.api.handleGetAgentIntegration(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var integration onboarding.BundleIntegration
	if err := json.Unmarshal(rec.Body.Bytes(), &integration); err != nil {
		t.Fatalf("decode integration response: %v", err)
	}
	if integration.ID == "" || integration.Runtime != "typescript" || integration.Description != "Lifecycle test" {
		t.Fatalf("unexpected integration payload: %+v", integration)
	}
}

func TestHandleGetAgentIntegrationReturnsNotFoundWithoutBackfillingLegacyMetadata(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[{"name":"postgres","actions":["query.readonly"],"type":"builtin"}]`)
	tenant := mustCreateTenantDB(t, fx.store, "Legacy Tenant")
	agent, err := fx.store.CreateAgent(context.Background(), tenant.ID, "Legacy Bot")
	if err != nil {
		t.Fatalf("CreateAgentWithLabels: %v", err)
	}

	req := withClaims(
		httptest.NewRequest(http.MethodGet, "/admin/tenants/"+tenant.ID+"/agents/"+agent.ID+"/integration", http.NoBody),
		&console.JWTClaims{Roles: []string{"platform_admin"}},
	)
	req.SetPathValue("tenant_id", tenant.ID)
	req.SetPathValue("agent_id", agent.ID)
	rec := httptest.NewRecorder()
	fx.api.handleGetAgentIntegration(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "integration not found") {
		t.Fatalf("expected integration-not-found guidance, got %s", rec.Body.String())
	}
	stored, err := fx.store.GetAgentIntegration(context.Background(), tenant.ID, agent.ID)
	if !errors.Is(err, console.ErrAgentIntegrationNotFound) {
		t.Fatalf("expected no integration to be created, got stored=%+v err=%v", stored, err)
	}
}

func TestHandleGetAgentIntegrationResolvesAgentsBeyondFirstPage(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[{"name":"postgres","actions":["query.readonly"],"type":"builtin"}]`)
	tenant := mustCreateTenantDB(t, fx.store, "Paged Tenant")

	for i := 0; i < 205; i++ {
		name := "Paged Agent " + strconv.Itoa(i)
		if _, err := fx.store.CreateAgent(context.Background(), tenant.ID, name); err != nil {
			t.Fatalf("CreateAgent(%d): %v", i, err)
		}
	}
	target, err := fx.store.CreateAgent(context.Background(), tenant.ID, "Target Agent")
	if err != nil {
		t.Fatalf("CreateAgentWithLabels: %v", err)
	}

	req := withClaims(
		httptest.NewRequest(http.MethodGet, "/admin/tenants/"+tenant.ID+"/agents/"+target.ID+"/integration", http.NoBody),
		&console.JWTClaims{Roles: []string{"platform_admin"}},
	)
	req.SetPathValue("tenant_id", tenant.ID)
	req.SetPathValue("agent_id", target.ID)
	rec := httptest.NewRecorder()
	fx.api.handleGetAgentIntegration(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for agent beyond first page, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleListAgentIntegrationRevisionsReturnsHistory(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[{"name":"postgres","actions":["query.readonly"],"type":"builtin"},{"name":"slack","actions":["slack.channel.list"],"type":"builtin"}]`)

	createBody := `{
		"runtime":"python",
		"new_tenant_name":"Revision Tenant",
		"agent_name":"Revision Bot",
		"approval_posture":"pilot_safe",
		"tools":[{"tool":"postgres","action":"query.readonly"}]
	}`
	createReq := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/onboarding/integrations", bytes.NewBufferString(createBody)),
		&console.JWTClaims{Roles: []string{"platform_admin"}},
	)
	createRec := httptest.NewRecorder()
	fx.api.handleCreateOnboardingIntegration(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create to succeed, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	var created onboarding.BundleResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	regenerateBody := `{
		"tenant_id":"` + created.Tenant.ID + `",
		"agent_id":"` + created.Agent.ID + `",
		"runtime":"python",
		"approval_posture":"tenant_default",
		"tools":[{"tool":"slack","action":"slack.channel.list"}]
	}`
	regenerateReq := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/onboarding/bundles/regenerate", bytes.NewBufferString(regenerateBody)),
		&console.JWTClaims{Roles: []string{"platform_admin"}},
	)
	regenerateRec := httptest.NewRecorder()
	fx.api.handleRegenerateOnboardingBundle(regenerateRec, regenerateReq)
	if regenerateRec.Code != http.StatusOK {
		t.Fatalf("expected regenerate to succeed, got %d body=%s", regenerateRec.Code, regenerateRec.Body.String())
	}

	req := withClaims(
		httptest.NewRequest(http.MethodGet, "/admin/tenants/"+created.Tenant.ID+"/agents/"+created.Agent.ID+"/integration/revisions?limit=5", http.NoBody),
		&console.JWTClaims{Roles: []string{"platform_admin"}},
	)
	req.SetPathValue("tenant_id", created.Tenant.ID)
	req.SetPathValue("agent_id", created.Agent.ID)
	rec := httptest.NewRecorder()
	fx.api.handleListAgentIntegrationRevisions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Revisions []onboarding.BundleIntegrationRevision `json:"revisions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode revisions response: %v", err)
	}
	if len(payload.Revisions) < 2 {
		t.Fatalf("expected at least 2 revisions, got %+v", payload.Revisions)
	}
	if payload.Revisions[0].Mode != "regenerated" || payload.Revisions[1].Mode != "created" {
		t.Fatalf("expected newest-first revision modes, got %+v", payload.Revisions)
	}
}

func TestHandleGetAgentIntegrationBundleReturnsSavedBundleAndArchive(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[{"name":"postgres","actions":["query.readonly"],"type":"builtin"}]`)

	body := `{
		"runtime":"openai_local",
		"new_tenant_name":"Bundle Tenant",
		"agent_name":"Bundle Bot",
		"approval_posture":"pilot_safe",
		"tools":[{"tool":"postgres","action":"query.readonly"}]
	}`
	createReq := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/onboarding/integrations", bytes.NewBufferString(body)),
		&console.JWTClaims{Roles: []string{"platform_admin"}},
	)
	createRec := httptest.NewRecorder()
	fx.api.handleCreateOnboardingIntegration(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create to succeed, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	var created onboarding.BundleResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	req := withClaims(
		httptest.NewRequest(http.MethodGet, "/admin/tenants/"+created.Tenant.ID+"/agents/"+created.Agent.ID+"/integration/bundle", http.NoBody),
		&console.JWTClaims{Roles: []string{"platform_admin"}},
	)
	req.SetPathValue("tenant_id", created.Tenant.ID)
	req.SetPathValue("agent_id", created.Agent.ID)
	rec := httptest.NewRecorder()
	fx.api.handleGetAgentIntegrationBundle(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var bundleResp onboarding.BundleResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &bundleResp); err != nil {
		t.Fatalf("decode saved bundle response: %v", err)
	}
	if bundleResp.Mode != "fetched" || bundleResp.Integration == nil || bundleResp.Bundle == nil {
		t.Fatalf("unexpected saved bundle response: %+v", bundleResp)
	}
	if bundleResp.Bundle.Runtime != "openai_local" {
		t.Fatalf("expected saved bundle runtime, got %+v", bundleResp.Bundle)
	}

	archiveReq := withClaims(
		httptest.NewRequest(http.MethodGet, "/admin/tenants/"+created.Tenant.ID+"/agents/"+created.Agent.ID+"/integration/bundle?defaults=true&archive=true", http.NoBody),
		&console.JWTClaims{Roles: []string{"platform_admin"}},
	)
	archiveReq.SetPathValue("tenant_id", created.Tenant.ID)
	archiveReq.SetPathValue("agent_id", created.Agent.ID)
	archiveRec := httptest.NewRecorder()
	fx.api.handleGetAgentIntegrationBundle(archiveRec, archiveReq)
	if archiveRec.Code != http.StatusOK {
		t.Fatalf("expected archive fetch to succeed, got %d body=%s", archiveRec.Code, archiveRec.Body.String())
	}
	if got := archiveRec.Header().Get("Content-Type"); got != "application/zip" {
		t.Fatalf("expected zip content type, got %q", got)
	}
	reader, err := zip.NewReader(bytes.NewReader(archiveRec.Body.Bytes()), int64(archiveRec.Body.Len()))
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	if len(reader.File) == 0 {
		t.Fatalf("expected archive files, got none")
	}
}

func TestHandleGetAgentIntegrationBundleArchiveUsesFetchedDefaultsFilename(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[{"name":"postgres","actions":["query.readonly"],"type":"builtin"}]`)

	body := `{
		"runtime":"python",
		"new_tenant_name":"Archive Tenant",
		"agent_name":"Archive Bot",
		"approval_posture":"pilot_safe",
		"tools":[{"tool":"postgres","action":"query.readonly"}]
	}`
	createReq := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/onboarding/integrations", bytes.NewBufferString(body)),
		&console.JWTClaims{Roles: []string{"platform_admin"}},
	)
	createRec := httptest.NewRecorder()
	fx.api.handleCreateOnboardingIntegration(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create to succeed, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	var created onboarding.BundleResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	req := withClaims(
		httptest.NewRequest(http.MethodGet, "/admin/tenants/"+created.Tenant.ID+"/agents/"+created.Agent.ID+"/integration/bundle?defaults=true&archive=true", http.NoBody),
		&console.JWTClaims{Roles: []string{"platform_admin"}},
	)
	req.SetPathValue("tenant_id", created.Tenant.ID)
	req.SetPathValue("agent_id", created.Agent.ID)
	rec := httptest.NewRecorder()
	fx.api.handleGetAgentIntegrationBundle(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected archive fetch to succeed, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, "fetched_defaults") {
		t.Fatalf("expected fetched_defaults archive filename, got %q", got)
	}
}

func TestHandleGetAgentIntegrationBundleReturnsNotFoundWithoutBackfillingLegacyMetadata(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[{"name":"postgres","actions":["query.readonly"],"type":"builtin"}]`)
	tenant := mustCreateTenantDB(t, fx.store, "Legacy Bundle Tenant")
	agent, err := fx.store.CreateAgent(context.Background(), tenant.ID, "Legacy Bundle Bot")
	if err != nil {
		t.Fatalf("CreateAgentWithLabels: %v", err)
	}

	req := withClaims(
		httptest.NewRequest(http.MethodGet, "/admin/tenants/"+tenant.ID+"/agents/"+agent.ID+"/integration/bundle", http.NoBody),
		&console.JWTClaims{Roles: []string{"platform_admin"}},
	)
	req.SetPathValue("tenant_id", tenant.ID)
	req.SetPathValue("agent_id", agent.ID)
	rec := httptest.NewRecorder()
	fx.api.handleGetAgentIntegrationBundle(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "integration not found") {
		t.Fatalf("expected integration-not-found guidance, got %s", rec.Body.String())
	}

	revisionsReq := withClaims(
		httptest.NewRequest(http.MethodGet, "/admin/tenants/"+tenant.ID+"/agents/"+agent.ID+"/integration/revisions?limit=5", http.NoBody),
		&console.JWTClaims{Roles: []string{"platform_admin"}},
	)
	revisionsReq.SetPathValue("tenant_id", tenant.ID)
	revisionsReq.SetPathValue("agent_id", agent.ID)
	revisionsRec := httptest.NewRecorder()
	fx.api.handleListAgentIntegrationRevisions(revisionsRec, revisionsReq)
	if revisionsRec.Code != http.StatusNotFound {
		t.Fatalf("expected revisions 404, got %d body=%s", revisionsRec.Code, revisionsRec.Body.String())
	}
	stored, err := fx.store.GetAgentIntegration(context.Background(), tenant.ID, agent.ID)
	if !errors.Is(err, console.ErrAgentIntegrationNotFound) {
		t.Fatalf("expected no integration to be created, got stored=%+v err=%v", stored, err)
	}
}

func TestHandleCreateOnboardingIntegrationLeavesPolicyUntouchedForTenantDefault(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[{"name":"postgres","actions":["query.readonly"],"type":"builtin"}]`)
	tenant := mustCreateTenantDB(t, fx.store, "Existing Tenant")

	body := `{
		"runtime":"python",
		"tenant_id":"` + tenant.ID + `",
		"agent_name":"Tenant Default Bot",
		"approval_posture":"tenant_default",
		"tools":[{"tool":"postgres","action":"query.readonly"}]
	}`

	req := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/onboarding/integrations", bytes.NewBufferString(body)),
		&console.JWTClaims{Tenant: tenant.ID, Roles: []string{"tenant_admin"}},
	)
	rr := httptest.NewRecorder()
	fx.api.handleCreateOnboardingIntegration(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rr.Code, rr.Body.String())
	}

	cfg, found, err := fx.store.GetTenantPolicyConfig(context.Background(), tenant.ID)
	if err != nil {
		t.Fatalf("GetTenantPolicyConfig: %v", err)
	}
	if found || cfg != nil {
		t.Fatalf("expected tenant_default onboarding to leave policy untouched, got %+v", cfg)
	}
}

func TestHandlePreviewOnboardingBundleCanonicalizesPrefixedActionNames(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[{"name":"slack","actions":["channel.list"],"type":"remote"}]`)
	tenant := mustCreateTenantDB(t, fx.store, "Existing Tenant")

	body := `{
		"runtime":"python",
		"tenant_id":"` + tenant.ID + `",
		"agent_name":"Preview Bot",
		"tools":[{"tool":"slack","action":"slack.channel.list"}]
	}`

	req := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/onboarding/bundles/preview", bytes.NewBufferString(body)),
		&console.JWTClaims{Tenant: tenant.ID, Roles: []string{"tenant_admin"}},
	)
	rr := httptest.NewRecorder()
	fx.api.handlePreviewOnboardingBundle(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Bundle struct {
			SampleCall string `json:"sample_call"`
		} `json:"bundle"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(resp.Bundle.SampleCall, `"action": "channel.list"`) {
		t.Fatalf("expected canonical action in sample call, got %s", resp.Bundle.SampleCall)
	}
}

func TestHandlePreviewOnboardingBundleValidatesCaseInsensitiveToolAndAction(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[{"name":"slack","actions":["channel.list"],"type":"remote"}]`)
	tenant := mustCreateTenantDB(t, fx.store, "Existing Tenant")

	body := `{
		"runtime":"python",
		"tenant_id":"` + tenant.ID + `",
		"agent_name":"Preview Bot",
		"tools":[{"tool":"Slack","action":"Slack.Channel.List"}]
	}`

	req := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/onboarding/bundles/preview", bytes.NewBufferString(body)),
		&console.JWTClaims{Tenant: tenant.ID, Roles: []string{"tenant_admin"}},
	)
	rr := httptest.NewRecorder()
	fx.api.handlePreviewOnboardingBundle(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Bundle struct {
			SampleCall string `json:"sample_call"`
		} `json:"bundle"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !strings.Contains(resp.Bundle.SampleCall, `"tool": "slack"`) {
		t.Fatalf("expected canonical tool in sample call, got %s", resp.Bundle.SampleCall)
	}
	if !strings.Contains(resp.Bundle.SampleCall, `"action": "channel.list"`) {
		t.Fatalf("expected canonical action in sample call, got %s", resp.Bundle.SampleCall)
	}
}

func TestHandleCreateOnboardingIntegrationRejectsUnknownToolAction(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[{"name":"slack","actions":["slack.channel.list"],"type":"remote"}]`)
	tenant := mustCreateTenantDB(t, fx.store, "Existing Tenant")

	body := `{
		"runtime":"langchain",
		"tenant_id":"` + tenant.ID + `",
		"agent_name":"Ops Bot",
		"tools":[{"tool":"slack","action":"slack.msg.post"}]
	}`

	req := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/onboarding/integrations", bytes.NewBufferString(body)),
		&console.JWTClaims{Tenant: tenant.ID, Roles: []string{"tenant_admin"}},
	)
	rr := httptest.NewRecorder()
	fx.api.handleCreateOnboardingIntegration(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "unknown action") {
		t.Fatalf("expected unknown action error, got %s", rr.Body.String())
	}
}

func TestHandleCreateOnboardingIntegrationRequiresMutationPermissions(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[{"name":"slack","actions":["slack.channel.list"],"type":"remote"}]`)
	tenant := mustCreateTenantDB(t, fx.store, "Existing Tenant")

	body := `{
		"runtime":"python",
		"tenant_id":"` + tenant.ID + `",
		"agent_name":"Viewer Bot",
		"tools":[{"tool":"slack","action":"slack.channel.list"}]
	}`

	req := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/onboarding/integrations", bytes.NewBufferString(body)),
		&console.JWTClaims{Tenant: tenant.ID, Roles: []string{"viewer"}},
	)
	rr := httptest.NewRecorder()
	fx.api.handleCreateOnboardingIntegration(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "insufficient permissions") {
		t.Fatalf("expected permission error, got %s", rr.Body.String())
	}
}

func TestHandlePreviewOnboardingBundleReturnsArtifactsWithoutCreatingState(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[{"name":"slack","actions":["slack.channel.list","slack.msg.post"],"type":"remote"}]`)
	tenant := mustCreateTenantDB(t, fx.store, "Existing Tenant")

	beforeAgents, err := fx.store.ListAgents(context.Background(), tenant.ID, 20, 0)
	if err != nil {
		t.Fatalf("ListAgents before preview: %v", err)
	}
	beforeKeys, err := fx.store.ListAPIKeys(context.Background(), tenant.ID)
	if err != nil {
		t.Fatalf("ListAPIKeys before preview: %v", err)
	}

	body := `{
		"runtime":"python",
		"tenant_id":"` + tenant.ID + `",
		"agent_name":"Preview Bot",
		"environment_label":"staging",
		"approval_posture":"pilot_safe",
		"tools":[{"tool":"slack","action":"slack.channel.list"}]
	}`

	req := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/onboarding/bundles/preview", bytes.NewBufferString(body)),
		&console.JWTClaims{Tenant: tenant.ID, Roles: []string{"tenant_admin"}},
	)
	rr := httptest.NewRecorder()
	fx.api.handlePreviewOnboardingBundle(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), `"api_key"`) {
		t.Fatalf("expected preview response JSON to omit api_key, got %s", rr.Body.String())
	}

	var resp struct {
		Mode   string `json:"mode"`
		Tenant struct {
			ID string `json:"id"`
		} `json:"tenant"`
		Agent struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			CreatedAt string `json:"created_at"`
			Preview   bool   `json:"preview"`
		} `json:"agent"`
		APIKey any `json:"api_key"`
		Bundle struct {
			Environment map[string]string `json:"environment"`
			Artifacts   []struct {
				FileName string `json:"file_name"`
				Kind     string `json:"kind"`
			} `json:"artifacts"`
			VerificationLinks []struct {
				Path string `json:"path"`
			} `json:"verification_links"`
		} `json:"bundle"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode preview response: %v", err)
	}

	if resp.Mode != "preview" {
		t.Fatalf("expected preview mode, got %q", resp.Mode)
	}
	if !resp.Agent.Preview || resp.Agent.ID != "preview-preview-bot" {
		t.Fatalf("expected preview agent metadata, got %+v", resp.Agent)
	}
	if strings.TrimSpace(resp.Agent.CreatedAt) == "" {
		t.Fatalf("expected synthetic preview created_at, got %+v", resp.Agent)
	}
	if _, err := time.Parse(time.RFC3339, resp.Agent.CreatedAt); err != nil {
		t.Fatalf("expected RFC3339 preview created_at, got %q: %v", resp.Agent.CreatedAt, err)
	}
	if resp.APIKey != nil {
		t.Fatalf("expected preview response not to include api key, got %+v", resp.APIKey)
	}
	if got := resp.Bundle.Environment["OPENCLAUSE_AGENT_ID"]; got != "preview-preview-bot" {
		t.Fatalf("expected synthetic agent id in preview env, got %q", got)
	}
	if got := resp.Bundle.Environment["OPENCLAUSE_API_KEY"]; got != "${OPENCLAUSE_API_KEY:-generated-on-create}" {
		t.Fatalf("expected preview api key placeholder, got %q", got)
	}
	if len(resp.Bundle.Artifacts) < 4 {
		t.Fatalf("expected bundle artifacts in preview response, got %+v", resp.Bundle.Artifacts)
	}
	if len(resp.Bundle.VerificationLinks) != 3 {
		t.Fatalf("expected verification links in preview response, got %+v", resp.Bundle.VerificationLinks)
	}

	afterAgents, err := fx.store.ListAgents(context.Background(), tenant.ID, 20, 0)
	if err != nil {
		t.Fatalf("ListAgents after preview: %v", err)
	}
	afterKeys, err := fx.store.ListAPIKeys(context.Background(), tenant.ID)
	if err != nil {
		t.Fatalf("ListAPIKeys after preview: %v", err)
	}
	if len(afterAgents) != len(beforeAgents) {
		t.Fatalf("expected preview not to create agents, before=%d after=%d", len(beforeAgents), len(afterAgents))
	}
	if len(afterKeys) != len(beforeKeys) {
		t.Fatalf("expected preview not to create api keys, before=%d after=%d", len(beforeKeys), len(afterKeys))
	}
}

func TestHandlePreviewOnboardingBundleRejectsInlineTenantCreation(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[{"name":"slack","actions":["slack.channel.list"],"type":"remote"}]`)

	body := `{
		"runtime":"python",
		"new_tenant_name":"Preview Tenant",
		"agent_name":"Preview Bot",
		"tools":[{"tool":"slack","action":"slack.channel.list"}]
	}`

	req := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/onboarding/bundles/preview", bytes.NewBufferString(body)),
		&console.JWTClaims{Roles: []string{"platform_admin"}},
	)
	rr := httptest.NewRecorder()
	fx.api.handlePreviewOnboardingBundle(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "preview requires tenant_id") {
		t.Fatalf("expected preview tenant guidance, got %s", rr.Body.String())
	}
}

func TestHandleCreateOnboardingIntegrationSupportsTypeScriptRuntime(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[{"name":"slack","actions":["slack.channel.list","slack.msg.post"],"type":"remote"}]`)
	tenant := mustCreateTenantDB(t, fx.store, "Existing Tenant")

	body := `{
		"runtime":"typescript",
		"tenant_id":"` + tenant.ID + `",
		"agent_name":"Node Bot",
		"approval_posture":"pilot_safe",
		"tools":[{"tool":"slack","action":"slack.channel.list"}]
	}`

	req := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/onboarding/integrations", bytes.NewBufferString(body)),
		&console.JWTClaims{Tenant: tenant.ID, Roles: []string{"tenant_admin"}},
	)
	rr := httptest.NewRecorder()
	fx.api.handleCreateOnboardingIntegration(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Bundle struct {
			Runtime         string `json:"runtime"`
			RuntimeLabel    string `json:"runtime_label"`
			StarterFileName string `json:"starter_file_name"`
			StarterSnippet  string `json:"starter_snippet"`
			Artifacts       []struct {
				ID       string `json:"id"`
				FileName string `json:"file_name"`
			} `json:"artifacts"`
		} `json:"bundle"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode TypeScript create response: %v", err)
	}

	if resp.Bundle.Runtime != "typescript" || resp.Bundle.StarterFileName != "agent.ts" {
		t.Fatalf("unexpected TypeScript bundle metadata: %+v", resp.Bundle)
	}
	if !strings.Contains(resp.Bundle.StarterSnippet, "OpenClauseClient") {
		t.Fatalf("expected TypeScript starter snippet, got %s", resp.Bundle.StarterSnippet)
	}
	foundPackageArtifact := false
	for _, artifact := range resp.Bundle.Artifacts {
		if artifact.ID == "package-snippet" && artifact.FileName == "package.onboarding.json" {
			foundPackageArtifact = true
			break
		}
	}
	if !foundPackageArtifact {
		t.Fatalf("expected TypeScript package snippet artifact, got %+v", resp.Bundle.Artifacts)
	}
}

func TestHandleRegenerateOnboardingBundleReusesExistingStateAndOmitsRawKey(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[{"name":"slack","actions":["slack.channel.list","slack.msg.post"],"type":"remote"}]`)
	tenant := mustCreateTenantDB(t, fx.store, "Existing Tenant")
	agent, err := fx.store.CreateAgent(context.Background(), tenant.ID, "Support Bot")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	keyResult, err := fx.store.CreateAPIKey(context.Background(), tenant.ID, "Support Bot key", nil)
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	beforeAgents, err := fx.store.ListAgents(context.Background(), tenant.ID, 20, 0)
	if err != nil {
		t.Fatalf("ListAgents before regenerate: %v", err)
	}
	beforeKeys, err := fx.store.ListAPIKeys(context.Background(), tenant.ID)
	if err != nil {
		t.Fatalf("ListAPIKeys before regenerate: %v", err)
	}

	body := `{
		"runtime":"python",
		"tenant_id":"` + tenant.ID + `",
		"agent_id":"` + agent.ID + `",
		"environment_label":"prod",
		"approval_posture":"tenant_default",
		"tools":[{"tool":"slack","action":"slack.msg.post"}]
	}`

	req := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/onboarding/bundles/regenerate", bytes.NewBufferString(body)),
		&console.JWTClaims{Tenant: tenant.ID, Roles: []string{"tenant_admin"}},
	)
	rr := httptest.NewRecorder()
	fx.api.handleRegenerateOnboardingBundle(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), `"raw_key"`) {
		t.Fatalf("expected regenerate response JSON to omit raw_key, got %s", rr.Body.String())
	}

	var resp struct {
		Mode  string `json:"mode"`
		Agent struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Preview bool   `json:"preview"`
		} `json:"agent"`
		APIKey *struct {
			ID        string `json:"id"`
			KeyPrefix string `json:"key_prefix"`
			RawKey    string `json:"raw_key"`
		} `json:"api_key"`
		Bundle struct {
			Environment map[string]string `json:"environment"`
			Notes       []string          `json:"notes"`
		} `json:"bundle"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode regenerate response: %v", err)
	}

	if resp.Mode != "regenerated" {
		t.Fatalf("expected regenerated mode, got %q", resp.Mode)
	}
	if resp.Agent.ID != agent.ID || resp.Agent.Preview {
		t.Fatalf("expected persisted agent metadata, got %+v", resp.Agent)
	}
	if resp.APIKey == nil || resp.APIKey.KeyPrefix != keyResult.APIKey.KeyPrefix {
		t.Fatalf("expected existing key prefix, got %+v", resp.APIKey)
	}
	if resp.APIKey.RawKey != "" {
		t.Fatalf("expected regenerated bundle to omit raw key, got %+v", resp.APIKey)
	}
	if got := resp.Bundle.Environment["OPENCLAUSE_API_KEY"]; got != "${OPENCLAUSE_API_KEY:-reuse-existing-key}" {
		t.Fatalf("expected existing-key placeholder, got %q", got)
	}
	if !strings.Contains(strings.Join(resp.Bundle.Notes, " "), "Raw API keys are only shown at creation time") {
		t.Fatalf("expected raw key guidance note, got %+v", resp.Bundle.Notes)
	}

	afterAgents, err := fx.store.ListAgents(context.Background(), tenant.ID, 20, 0)
	if err != nil {
		t.Fatalf("ListAgents after regenerate: %v", err)
	}
	afterKeys, err := fx.store.ListAPIKeys(context.Background(), tenant.ID)
	if err != nil {
		t.Fatalf("ListAPIKeys after regenerate: %v", err)
	}
	if len(afterAgents) != len(beforeAgents) {
		t.Fatalf("expected regenerate not to create agents, before=%d after=%d", len(beforeAgents), len(afterAgents))
	}
	if len(afterKeys) != len(beforeKeys) {
		t.Fatalf("expected regenerate not to create api keys, before=%d after=%d", len(beforeKeys), len(afterKeys))
	}
	if string(afterAgents[0].Labels) != "{}" {
		t.Fatalf("expected regenerate not to mirror onboarding metadata into labels, got %s", string(afterAgents[0].Labels))
	}
	integration, err := fx.store.GetAgentIntegration(context.Background(), tenant.ID, agent.ID)
	if err != nil {
		t.Fatalf("GetAgentIntegration after regenerate: %v", err)
	}
	if integration.Runtime != "python" || integration.EnvironmentLabel != "prod" || integration.ApprovalPosture != "tenant_default" {
		t.Fatalf("expected regenerate to persist integration state, got %+v", integration)
	}
	if len(integration.Tools) != 1 || integration.Tools[0].Action != "slack.msg.post" {
		t.Fatalf("expected regenerate to persist selected tool, got %+v", integration.Tools)
	}
}

func TestHandleRegenerateOnboardingBundleDefaultsUsesCuratedDefaults(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[{"name":"slack","actions":["slack.channel.list","slack.msg.post"],"type":"remote"}]`)
	tenant := mustCreateTenantDB(t, fx.store, "Existing Tenant")
	agent, err := fx.store.CreateAgent(context.Background(), tenant.ID, "Support Bot")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if _, err := fx.store.CreateAPIKey(context.Background(), tenant.ID, "Support Bot key", nil); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	body := `{
		"tenant_id":"` + tenant.ID + `",
		"agent_id":"` + agent.ID + `"
	}`

	req := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/onboarding/bundles/regenerate-defaults", bytes.NewBufferString(body)),
		&console.JWTClaims{Tenant: tenant.ID, Roles: []string{"tenant_admin"}},
	)
	rr := httptest.NewRecorder()
	fx.api.handleRegenerateOnboardingBundleDefaults(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), `"raw_key"`) {
		t.Fatalf("expected defaults regenerate response JSON to omit raw_key, got %s", rr.Body.String())
	}

	var resp struct {
		Mode   string `json:"mode"`
		APIKey *struct {
			KeyPrefix string `json:"key_prefix"`
		} `json:"api_key"`
		Bundle struct {
			Runtime         string                     `json:"runtime"`
			AppliedDefaults []onboarding.BundleDefault `json:"applied_defaults"`
			Environment     map[string]string          `json:"environment"`
		} `json:"bundle"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode defaults regenerate response: %v", err)
	}

	if resp.Mode != "regenerated_defaults" {
		t.Fatalf("expected regenerated_defaults mode, got %q", resp.Mode)
	}
	if resp.Bundle.Runtime != "python" {
		t.Fatalf("expected python runtime default, got %+v", resp.Bundle)
	}
	if len(resp.Bundle.AppliedDefaults) < 3 {
		t.Fatalf("expected applied defaults metadata, got %+v", resp.Bundle.AppliedDefaults)
	}
	if got := resp.Bundle.AppliedDefaults[2].Value; got != "slack:slack.channel.list" {
		t.Fatalf("expected formatted tool default, got %+v", resp.Bundle.AppliedDefaults)
	}
	if resp.APIKey == nil || resp.APIKey.KeyPrefix == "" {
		t.Fatalf("expected existing key reference, got %+v", resp.APIKey)
	}
	if got := resp.Bundle.Environment["OPENCLAUSE_API_KEY"]; got != "${OPENCLAUSE_API_KEY:-reuse-existing-key}" {
		t.Fatalf("expected existing key placeholder, got %q", got)
	}

	agents, err := fx.store.ListAgents(context.Background(), tenant.ID, 20, 0)
	if err != nil {
		t.Fatalf("ListAgents after regenerate-defaults: %v", err)
	}
	if string(agents[0].Labels) != "{}" {
		t.Fatalf("expected defaults regenerate not to mirror onboarding metadata into labels, got %s", string(agents[0].Labels))
	}
	integration, err := fx.store.GetAgentIntegration(context.Background(), tenant.ID, agent.ID)
	if err != nil {
		t.Fatalf("GetAgentIntegration after defaults regenerate: %v", err)
	}
	if integration.Runtime != "python" || integration.ApprovalPosture != "pilot_safe" {
		t.Fatalf("expected defaults regenerate to persist integration, got %+v", integration)
	}
}

func TestHandleRegenerateOnboardingBundleRequiresMutationPermissions(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[{"name":"slack","actions":["slack.channel.list"],"type":"remote"}]`)
	tenant := mustCreateTenantDB(t, fx.store, "Existing Tenant")
	agent, err := fx.store.CreateAgent(context.Background(), tenant.ID, "Support Bot")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	body := `{
		"runtime":"python",
		"tenant_id":"` + tenant.ID + `",
		"agent_id":"` + agent.ID + `",
		"tools":[{"tool":"slack","action":"slack.channel.list"}]
	}`

	req := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/onboarding/bundles/regenerate", bytes.NewBufferString(body)),
		&console.JWTClaims{Tenant: tenant.ID, Roles: []string{"viewer"}},
	)
	rr := httptest.NewRecorder()
	fx.api.handleRegenerateOnboardingBundle(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "insufficient permissions") {
		t.Fatalf("expected permission error, got %s", rr.Body.String())
	}
}

func TestHandleRegenerateOnboardingBundleDefaultsRequiresMutationPermissions(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[{"name":"slack","actions":["slack.channel.list"],"type":"remote"}]`)
	tenant := mustCreateTenantDB(t, fx.store, "Existing Tenant")
	agent, err := fx.store.CreateAgent(context.Background(), tenant.ID, "Support Bot")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	body := `{
		"tenant_id":"` + tenant.ID + `",
		"agent_id":"` + agent.ID + `"
	}`

	req := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/onboarding/bundles/regenerate-defaults", bytes.NewBufferString(body)),
		&console.JWTClaims{Tenant: tenant.ID, Roles: []string{"viewer"}},
	)
	rr := httptest.NewRecorder()
	fx.api.handleRegenerateOnboardingBundleDefaults(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "insufficient permissions") {
		t.Fatalf("expected permission error, got %s", rr.Body.String())
	}
}

func TestHandleRegenerateOnboardingBundleDefaultsPrefersPersistedIntegration(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[{"name":"jira","actions":["jira.issue.list","jira.issue.create"],"type":"remote"}]`)
	tenant := mustCreateTenantDB(t, fx.store, "Existing Tenant")
	agent, err := fx.store.CreateAgent(context.Background(), tenant.ID, "Support Bot")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if _, err := fx.store.UpsertAgentIntegration(context.Background(), console.AgentIntegrationUpsertInput{
		TenantID:         tenant.ID,
		AgentID:          agent.ID,
		Mode:             "created",
		Runtime:          "typescript",
		EnvironmentLabel: "prod",
		ApprovalPosture:  "tenant_default",
		Tools: []console.AgentIntegrationTool{
			{Tool: "jira", Action: "jira.issue.list"},
			{Tool: "jira", Action: "jira.issue.create"},
		},
	}); err != nil {
		t.Fatalf("UpsertAgentIntegration: %v", err)
	}
	if _, err := fx.store.CreateAPIKey(context.Background(), tenant.ID, "Support Bot key", nil); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	body := `{
		"tenant_id":"` + tenant.ID + `",
		"agent_id":"` + agent.ID + `"
	}`

	req := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/onboarding/bundles/regenerate-defaults", bytes.NewBufferString(body)),
		&console.JWTClaims{Tenant: tenant.ID, Roles: []string{"tenant_admin"}},
	)
	rr := httptest.NewRecorder()
	fx.api.handleRegenerateOnboardingBundleDefaults(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Bundle struct {
			Runtime         string                     `json:"runtime"`
			AppliedDefaults []onboarding.BundleDefault `json:"applied_defaults"`
		} `json:"bundle"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode defaults regenerate response: %v", err)
	}
	if resp.Bundle.Runtime != "typescript" {
		t.Fatalf("expected persisted runtime, got %+v", resp.Bundle)
	}
	if len(resp.Bundle.AppliedDefaults) < 4 || resp.Bundle.AppliedDefaults[0].Reason != "Persisted integration record for this agent" {
		t.Fatalf("expected persisted integration defaults, got %+v", resp.Bundle.AppliedDefaults)
	}
}

func TestHandleRegenerateOnboardingBundleDoesNotMutateIntegrationWhenBundleBaseURLIsMissing(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[{"name":"jira","actions":["jira.issue.list","jira.issue.create"],"type":"remote"}]`)
	fx.api.publicGatewayURL = ""
	fx.api.gatewayURL = ""
	fx.api.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "http://localhost:8080/v1/connectors" {
				t.Fatalf("unexpected connector request URL: %s", req.URL.String())
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewBufferString(`[{"name":"jira","actions":["jira.issue.list","jira.issue.create"],"type":"remote"}]`)),
			}, nil
		}),
	}

	tenant := mustCreateTenantDB(t, fx.store, "Missing Bundle URL Tenant")
	agent, err := fx.store.CreateAgent(context.Background(), tenant.ID, "Missing Bundle URL Bot")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	original, err := fx.store.UpsertAgentIntegration(context.Background(), console.AgentIntegrationUpsertInput{
		TenantID:         tenant.ID,
		AgentID:          agent.ID,
		Mode:             "created",
		Runtime:          "python",
		EnvironmentLabel: "dev",
		ApprovalPosture:  "pilot_safe",
		Tools: []console.AgentIntegrationTool{
			{Tool: "jira", Action: "jira.issue.list"},
		},
	})
	if err != nil {
		t.Fatalf("UpsertAgentIntegration: %v", err)
	}
	if _, err := fx.store.CreateAPIKey(context.Background(), tenant.ID, "Existing key", nil); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	body := `{
		"tenant_id":"` + tenant.ID + `",
		"agent_id":"` + agent.ID + `",
		"runtime":"typescript",
		"environment_label":"prod",
		"approval_posture":"tenant_default",
		"tools":[{"tool":"jira","action":"jira.issue.create"}]
	}`

	req := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/onboarding/bundles/regenerate", bytes.NewBufferString(body)),
		&console.JWTClaims{Tenant: tenant.ID, Roles: []string{"tenant_admin"}},
	)
	rr := httptest.NewRecorder()
	fx.api.handleRegenerateOnboardingBundle(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "onboarding bundle base URL is not configured") {
		t.Fatalf("expected bundle base url error, got %s", rr.Body.String())
	}
	reloaded, err := fx.store.GetAgentIntegration(context.Background(), tenant.ID, agent.ID)
	if err != nil {
		t.Fatalf("GetAgentIntegration: %v", err)
	}
	if reloaded.Runtime != original.Runtime || reloaded.EnvironmentLabel != original.EnvironmentLabel || len(reloaded.Tools) != len(original.Tools) || reloaded.Tools[0].Action != original.Tools[0].Action {
		t.Fatalf("expected persisted integration to stay unchanged, got %+v", reloaded)
	}
}

func TestHandleRegenerateOnboardingBundleDefaultsFailsWhenCuratedDefaultsUnavailable(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[{"name":"custom","actions":["unsafe.write"],"type":"remote"}]`)
	tenant := mustCreateTenantDB(t, fx.store, "Existing Tenant")
	agent, err := fx.store.CreateAgent(context.Background(), tenant.ID, "Support Bot")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	body := `{
		"tenant_id":"` + tenant.ID + `",
		"agent_id":"` + agent.ID + `"
	}`

	req := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/onboarding/bundles/regenerate-defaults", bytes.NewBufferString(body)),
		&console.JWTClaims{Tenant: tenant.ID, Roles: []string{"tenant_admin"}},
	)
	rr := httptest.NewRecorder()
	fx.api.handleRegenerateOnboardingBundleDefaults(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "no curated onboarding defaults are available") {
		t.Fatalf("expected curated-default failure guidance, got %s", rr.Body.String())
	}
}

func TestDefaultRegenerateConfigSkipsMissingCuratedCatalogEntries(t *testing.T) {
	runtime, posture, tools, defaults, err := defaultRegenerateConfig([]connectors.ConnectorInfo{
		{Name: "github", Actions: []string{"issue.create"}, Type: "builtin"},
	}, nil)
	if err != nil {
		t.Fatalf("defaultRegenerateConfig: %v", err)
	}
	if runtime != onboarding.RuntimePython {
		t.Fatalf("expected python runtime default, got %q", runtime)
	}
	if posture != "pilot_safe" {
		t.Fatalf("expected pilot_safe posture, got %q", posture)
	}
	if len(tools) != 1 || tools[0].Tool != "github" || tools[0].Action != "issue.create" {
		t.Fatalf("expected only github curated default, got %+v", tools)
	}
	if len(defaults) != 3 || defaults[2].Value != "github:issue.create" {
		t.Fatalf("expected formatted github default, got %+v", defaults)
	}
}

func TestDefaultRegenerateConfigPrefersPersistedIntegrationWhenValid(t *testing.T) {
	runtime, posture, tools, defaults, err := defaultRegenerateConfig([]connectors.ConnectorInfo{
		{Name: "postgres", Actions: []string{"query.readonly"}, Type: "builtin"},
	}, &console.AgentIntegration{
		Runtime:         "openai_local",
		ApprovalPosture: "pilot_safe",
		Tools: []console.AgentIntegrationTool{
			{Tool: "postgres", Action: "query.readonly"},
		},
	})
	if err != nil {
		t.Fatalf("defaultRegenerateConfig persisted integration: %v", err)
	}
	if runtime != onboarding.RuntimeOpenAILocal {
		t.Fatalf("expected integration runtime, got %q", runtime)
	}
	if posture != "pilot_safe" {
		t.Fatalf("expected integration posture, got %q", posture)
	}
	if len(tools) != 1 || tools[0].Tool != "postgres" || tools[0].Action != "query.readonly" {
		t.Fatalf("expected integration tool selection, got %+v", tools)
	}
	if len(defaults) < 3 || defaults[0].Reason != "Persisted integration record for this agent" {
		t.Fatalf("expected integration defaults guidance, got %+v", defaults)
	}
}

func TestHandleRegenerateOnboardingBundleOmitsEmptyAPIKeyReferenceWhenNoActiveKeyExists(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[{"name":"slack","actions":["slack.msg.post"],"type":"remote"}]`)
	tenant := mustCreateTenantDB(t, fx.store, "Existing Tenant")
	agent, err := fx.store.CreateAgent(context.Background(), tenant.ID, "Support Bot")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	body := `{
		"runtime":"python",
		"tenant_id":"` + tenant.ID + `",
		"agent_id":"` + agent.ID + `",
		"approval_posture":"tenant_default",
		"tools":[{"tool":"slack","action":"slack.msg.post"}]
	}`

	req := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/onboarding/bundles/regenerate", bytes.NewBufferString(body)),
		&console.JWTClaims{Tenant: tenant.ID, Roles: []string{"tenant_admin"}},
	)
	rr := httptest.NewRecorder()
	fx.api.handleRegenerateOnboardingBundle(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), `"api_key"`) {
		t.Fatalf("expected api_key to be omitted when no active key exists, got %s", rr.Body.String())
	}

	var resp struct {
		APIKey any `json:"api_key"`
		Bundle struct {
			Notes []string `json:"notes"`
		} `json:"bundle"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode regenerate response without key: %v", err)
	}

	if resp.APIKey != nil {
		t.Fatalf("expected api_key to be omitted when no active key exists, got %+v", resp.APIKey)
	}
	if !strings.Contains(strings.Join(resp.Bundle.Notes, " "), "No active API key was found") {
		t.Fatalf("expected missing-key note, got %+v", resp.Bundle.Notes)
	}
}

func TestHandleArchiveOnboardingBundleReturnsZip(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[]`)
	body := `{
		"mode":"created",
		"tenant":{"id":"tenant-1","name":"Alpha Corp","created":false},
		"agent":{"id":"agent-1","name":"Support Bot","status":"active","created_at":"2026-03-23T12:00:00Z","preview":false},
		"bundle":{
			"title":"Python bundle",
			"summary":"Tenant tenant-1 · Agent agent-1 · Python",
			"runtime":"python",
			"runtime_label":"Python SDK wrapper",
			"starter_file_name":"agent.py",
			"environment":{},
			"environment_script":"export A=1",
			"environment_file":"A=1",
			"starter_snippet":"print('ok')",
			"readme_snippet":"# Readme",
			"sample_call":"curl",
			"artifacts":[
				{"id":"env-script","label":"Env","file_name":"setup-env.sh","path_hint":"setup-env.sh","kind":"environment_script","purpose":"Env","writable":true,"content":"export A=1"},
				{"id":"starter","label":"Starter","file_name":"agent.py","path_hint":"agent.py","kind":"starter_file","purpose":"Starter","writable":true,"content":"print('ok')"}
			],
			"verification_checklist":["step"],
			"verification_links":[],
			"notes":[]
		}
	}`

	req := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/onboarding/bundles/archive", bytes.NewBufferString(body)),
		&console.JWTClaims{Roles: []string{"platform_admin"}},
	)
	rr := httptest.NewRecorder()
	fx.api.handleArchiveOnboardingBundle(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "application/zip" {
		t.Fatalf("expected zip content type, got %q", got)
	}
	if got := rr.Header().Get("Content-Disposition"); !strings.Contains(got, `openclause-alpha-corp-support-bot-created.zip`) {
		t.Fatalf("expected stable archive filename, got %q", got)
	}
	reader, err := zip.NewReader(bytes.NewReader(rr.Body.Bytes()), int64(rr.Body.Len()))
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}
	if len(reader.File) != 2 {
		t.Fatalf("expected 2 files in archive, got %d", len(reader.File))
	}
}

func TestHandleArchiveOnboardingBundleRejectsMissingBundle(t *testing.T) {
	fx := newOnboardingAPIFixture(t, `[]`)
	req := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/onboarding/bundles/archive", strings.NewReader(`{"mode":"preview"}`)),
		&console.JWTClaims{Roles: []string{"platform_admin"}},
	)
	rr := httptest.NewRecorder()
	fx.api.handleArchiveOnboardingBundle(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "bundle required") {
		t.Fatalf("expected bundle required error, got %s", rr.Body.String())
	}
}
