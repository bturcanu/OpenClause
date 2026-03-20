package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bturcanu/OpenClause/pkg/connectors"
)

func Test_handleListConnectors_ProxiesGatewayRegistry(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/v1/connectors" {
			t.Fatalf("expected /v1/connectors, got %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]connectors.ConnectorInfo{
			{
				Name:    "slack",
				Type:    "remote",
				Actions: []string{"msg.post", "channel.list"},
				BaseURL: "http://connector-slack:8082",
			},
			{
				Name:    "github",
				Type:    "builtin",
				Actions: []string{"repo.list"},
			},
		})
	}))
	t.Cleanup(gateway.Close)

	api := &ConsoleAPI{
		log:        slog.Default(),
		gatewayURL: gateway.URL,
		httpClient: gateway.Client(),
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/connectors", nil)
	rr := httptest.NewRecorder()
	api.handleListConnectors(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var got []connectors.ConnectorInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 connectors, got %d", len(got))
	}
	if got[0].Name != "slack" || got[0].Type != "remote" {
		t.Fatalf("unexpected first connector: %+v", got[0])
	}
	if got[0].BaseURL != "" {
		t.Fatalf("expected base_url to be stripped, got %q", got[0].BaseURL)
	}
	if len(got[0].Actions) != 2 || got[0].Actions[0] != "msg.post" {
		t.Fatalf("unexpected actions: %+v", got[0].Actions)
	}
}

func Test_handleListConnectors_GatewayFailureReturnsBadGateway(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(gateway.Close)

	api := &ConsoleAPI{
		log:        slog.Default(),
		gatewayURL: gateway.URL,
		httpClient: gateway.Client(),
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/connectors", nil)
	rr := httptest.NewRecorder()
	api.handleListConnectors(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "failed to load connector registry") {
		t.Fatalf("expected generic connector registry error, got body=%s", rr.Body.String())
	}
}

func Test_handleListConnectors_InvalidGatewayJSONReturnsBadGateway(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"not":"an array"}`))
	}))
	t.Cleanup(gateway.Close)

	api := &ConsoleAPI{
		log:        slog.Default(),
		gatewayURL: gateway.URL,
		httpClient: gateway.Client(),
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/connectors", nil)
	rr := httptest.NewRecorder()
	api.handleListConnectors(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d body=%s", rr.Code, rr.Body.String())
	}
}
