package policy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bturcanu/OpenClause/pkg/types"
)

func TestEvaluate_AllowDecision(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"result": map[string]any{
				"decision": "allow",
				"reason":   "low risk read",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	result, err := client.Evaluate(context.Background(), types.PolicyInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Decision != types.DecisionAllow {
		t.Errorf("expected allow, got %s", result.Decision)
	}
	if result.Reason != "low risk read" {
		t.Errorf("expected reason 'low risk read', got %q", result.Reason)
	}
}

func TestEvaluate_DefaultDenyOnEmptyDecision(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"result": map[string]any{
				"decision": "",
				"reason":   "",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	result, err := client.Evaluate(context.Background(), types.PolicyInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Decision != types.DecisionDeny {
		t.Errorf("expected deny for empty decision, got %s", result.Decision)
	}
}

func TestEvaluate_DefaultDenyOnUnknownDecision(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"result": map[string]any{
				"decision": "escalate",
				"reason":   "custom",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	result, err := client.Evaluate(context.Background(), types.PolicyInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Decision != types.DecisionDeny {
		t.Errorf("expected deny for unknown decision, got %s", result.Decision)
	}
}

func TestEvaluate_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("opa error"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.Evaluate(context.Background(), types.PolicyInput{})
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestEvaluate_TrimsTrailingSlashFromBaseURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		resp := map[string]any{
			"result": map[string]any{
				"decision": "allow",
				"reason":   "ok",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL + "/")
	if _, err := client.Evaluate(context.Background(), types.PolicyInput{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != OPAPolicyPath {
		t.Fatalf("expected path %s, got %s", OPAPolicyPath, gotPath)
	}
}

func TestEvaluate_MapsPolicyMetadataFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"result": map[string]any{
				"decision":       "approve",
				"reason":         "needs approval",
				"requirements":   map[string]string{"slack": "approval"},
				"risk_overrides": map[string]int{"jira.issue.create": 9},
				"notify": []map[string]any{
					{"kind": "webhook", "url": "https://example.com/hook"},
				},
				"approver_group": "security",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	result, err := client.Evaluate(context.Background(), types.PolicyInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Decision != types.DecisionApprove {
		t.Fatalf("expected approve, got %s", result.Decision)
	}
	if result.Requirements["slack"] != "approval" {
		t.Fatalf("unexpected requirements: %+v", result.Requirements)
	}
	if result.RiskOverrides["jira.issue.create"] != 9 {
		t.Fatalf("unexpected risk overrides: %+v", result.RiskOverrides)
	}
	if len(result.Notify) != 1 || result.Notify[0].Kind != "webhook" {
		t.Fatalf("unexpected notify payload: %+v", result.Notify)
	}
	if result.ApproverGroup != "security" {
		t.Fatalf("unexpected approver group: %q", result.ApproverGroup)
	}
}

func TestEvaluate_InvalidJSONReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.Evaluate(context.Background(), types.PolicyInput{})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
