package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
}
