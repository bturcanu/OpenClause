package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bturcanu/OpenClause/pkg/console"
	"github.com/bturcanu/OpenClause/pkg/connectors"
	"github.com/bturcanu/OpenClause/pkg/types"
)

type stubTenantNotificationConfigGetter struct {
	cfg   *console.TenantNotificationConfig
	found bool
	err   error
}

func (s stubTenantNotificationConfigGetter) GetTenantNotificationConfig(ctx context.Context, tenantID string) (*console.TenantNotificationConfig, bool, error) {
	return s.cfg, s.found, s.err
}

func Test_dispatchAlertEvent_PartialSinkSuccessReturnsNil(t *testing.T) {
	// Slack connector mock.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/exec" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(connectors.ExecResponse{Status: "success"})
	}))
	t.Cleanup(srv.Close)

	cfg := &console.TenantNotificationConfig{
		Notify: []types.PolicyNotify{
			// First sink fails fast due to missing channel.
			{Kind: "slack", Channel: ""},
			// Second sink succeeds.
			{Kind: "slack", Channel: "#alerts"},
		},
	}

	getter := stubTenantNotificationConfigGetter{cfg: cfg, found: true}
	event := &console.AlertEvent{
		ID:       "evt_1",
		TenantID: "tenant_1",
		Message:  "hello",
	}

	err := dispatchAlertEvent(context.Background(), getter, event, srv.URL, "internal-token", map[string]string{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func Test_dispatchAlertEvent_AllSinksFailReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("slack connector should not be called when all sinks are invalid")
	}))
	t.Cleanup(srv.Close)

	cfg := &console.TenantNotificationConfig{
		Notify: []types.PolicyNotify{
			{Kind: "slack", Channel: ""},         // invalid -> fails
			{Kind: "slack", Channel: ""},         // invalid -> fails
			{Kind: "webhook", URL: ""},           // invalid -> fails
			{Kind: "unsupported-kind", URL: ""}, // invalid -> fails
		},
	}

	getter := stubTenantNotificationConfigGetter{cfg: cfg, found: true}
	event := &console.AlertEvent{
		ID:       "evt_1",
		TenantID: "tenant_1",
		Message:  "hello",
	}

	err := dispatchAlertEvent(context.Background(), getter, event, srv.URL, "internal-token", map[string]string{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

