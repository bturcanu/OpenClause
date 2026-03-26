package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bturcanu/OpenClause/pkg/console"
)

func Test_handleSimulateTenantPolicy_InvalidOPAJSONReturnsBadGateway(t *testing.T) {
	opa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{this is not valid json`))
	}))
	t.Cleanup(opa.Close)
	t.Setenv("OPA_URL", opa.URL)

	api := &ConsoleAPI{
		log: slog.Default(),
	}

	body := map[string]any{
		"agent_id":   "agent-1",
		"tool":       "slack",
		"action":     "msg.post",
		"resource":   "channel:#alerts",
		"risk_score": 3,
		"policy_config": map[string]any{
			"max_risk_auto_approve":        7,
			"read_actions":                 []string{},
			"write_actions":                []string{},
			"destructive_actions":          []string{},
			"require_destructive_approval": true,
		},
	}
	rawBody, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/admin/tenants/tenant1/policy/simulate", bytes.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	req = setRouteParams(req, map[string]string{"tenant_id": "tenant1"})
	rr := httptest.NewRecorder()

	api.handleSimulateTenantPolicy(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "invalid policy engine response") {
		t.Fatalf("expected invalid policy engine response error, got body=%s", rr.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode api error payload: %v", err)
	}
	details, ok := payload["details"].(map[string]any)
	if !ok || details["stage"] != "decode" {
		t.Fatalf("expected decode-stage details, got %#v", payload["details"])
	}
}

func TestHandleSimulatePolicy_ForbidsTenantAdminSimulatingAnotherTenant(t *testing.T) {
	api := &ConsoleAPI{log: slog.Default()}

	body := `{"tenant_id":"tenant-2","agent_id":"agent-1","tool":"slack","action":"msg.post","resource":"channel:#alerts","risk_score":3}`
	req := httptest.NewRequest(http.MethodPost, "/admin/policy/simulate", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), claimsKey{}, &console.JWTClaims{
		Tenant: "tenant-1",
		Roles:  []string{"tenant_admin"},
	}))
	rr := httptest.NewRecorder()

	api.handleSimulatePolicy(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "access denied for this tenant") {
		t.Fatalf("expected tenant access error, got body=%s", rr.Body.String())
	}
}

func TestHandleSimulatePolicy_NonSuccessOPAStatusReturnsBadGateway(t *testing.T) {
	opa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
	}))
	t.Cleanup(opa.Close)
	t.Setenv("OPA_URL", opa.URL)

	api := &ConsoleAPI{
		log:        slog.Default(),
		httpClient: opa.Client(),
	}

	body := `{"tenant_id":"tenant-1","agent_id":"agent-1","tool":"slack","action":"msg.post","resource":"channel:#alerts","risk_score":3}`
	req := httptest.NewRequest(http.MethodPost, "/admin/policy/simulate", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), claimsKey{}, &console.JWTClaims{
		Roles: []string{"platform_admin"},
	}))
	rr := httptest.NewRecorder()

	api.handleSimulatePolicy(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "policy engine returned 500") {
		t.Fatalf("expected OPA status error, got body=%s", rr.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode api error payload: %v", err)
	}
	details, ok := payload["details"].(map[string]any)
	if !ok || details["upstream_status"] != float64(http.StatusInternalServerError) {
		t.Fatalf("expected upstream_status details, got %#v", payload["details"])
	}
}

func TestHandleSimulateTenantPolicy_NonSuccessOPAStatusReturnsBadGateway(t *testing.T) {
	opa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
	}))
	t.Cleanup(opa.Close)
	t.Setenv("OPA_URL", opa.URL)

	api := &ConsoleAPI{
		log:        slog.Default(),
		httpClient: opa.Client(),
	}

	body := map[string]any{
		"agent_id":   "agent-1",
		"tool":       "slack",
		"action":     "msg.post",
		"resource":   "channel:#alerts",
		"risk_score": 3,
		"policy_config": map[string]any{
			"max_risk_auto_approve":        7,
			"read_actions":                 []string{},
			"write_actions":                []string{},
			"destructive_actions":          []string{},
			"require_destructive_approval": true,
		},
	}
	rawBody, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/admin/tenants/tenant1/policy/simulate", bytes.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	req = setRouteParams(req, map[string]string{"tenant_id": "tenant1"})
	rr := httptest.NewRecorder()

	api.handleSimulateTenantPolicy(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "policy engine returned 500") {
		t.Fatalf("expected OPA status error, got body=%s", rr.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode api error payload: %v", err)
	}
	details, ok := payload["details"].(map[string]any)
	if !ok || details["stage"] != "upstream_status" {
		t.Fatalf("expected upstream_status details, got %#v", payload["details"])
	}
}

func TestHandleSimulateTenantPolicy_StoreFailureDoesNotSilentlyFallBackToDefaults(t *testing.T) {
	fx := newDBAPIFixture(t)
	tenant := mustCreateTenantDB(t, fx.store, "Policy Tenant")
	fx.store.Pool().Close()

	req := httptest.NewRequest(http.MethodPost, "/admin/tenants/"+tenant.ID+"/policy/simulate", bytes.NewBufferString(`{"agent_id":"agent-1","tool":"slack","action":"msg.post","resource":"channel:#alerts","risk_score":3}`))
	req.Header.Set("Content-Type", "application/json")
	req = setRouteParams(req, map[string]string{"tenant_id": tenant.ID})
	rr := httptest.NewRecorder()

	fx.api.handleSimulateTenantPolicy(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "failed to load tenant policy config") {
		t.Fatalf("expected tenant policy config load failure, got body=%s", rr.Body.String())
	}
}
