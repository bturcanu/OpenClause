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
		json.NewEncoder(w).Encode(resp)
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
		json.NewEncoder(w).Encode(resp)
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
		json.NewEncoder(w).Encode(resp)
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
		w.Write([]byte("opa error"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.Evaluate(context.Background(), types.PolicyInput{})
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}
