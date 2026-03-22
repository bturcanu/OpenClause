package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bturcanu/OpenClause/pkg/console"
	"github.com/bturcanu/OpenClause/pkg/types"
	"github.com/go-chi/chi/v5"
)

type stubNotificationConfigStore struct {
	getFn func(ctx context.Context, tenantID string) (*console.TenantNotificationConfig, bool, error)
	setFn func(ctx context.Context, tenantID string, cfg console.TenantNotificationConfig) error

	lastSetTenantID string
	lastSetCfg      *console.TenantNotificationConfig
}

func (s *stubNotificationConfigStore) GetTenantNotificationConfig(ctx context.Context, tenantID string) (*console.TenantNotificationConfig, bool, error) {
	return s.getFn(ctx, tenantID)
}

func (s *stubNotificationConfigStore) SetTenantNotificationConfig(ctx context.Context, tenantID string, cfg console.TenantNotificationConfig) error {
	s.lastSetTenantID = tenantID
	s.lastSetCfg = &console.TenantNotificationConfig{
		ApproverGroup: cfg.ApproverGroup,
		Notify:        cfg.Notify,
	}
	return s.setFn(ctx, tenantID, cfg)
}

func Test_handleGetTenantNotificationConfig_ReturnsStoredConfig(t *testing.T) {
	cfg := &console.TenantNotificationConfig{
		ApproverGroup: "tenant_admin",
		Notify: []types.PolicyNotify{
			{Kind: "slack", Channel: "#alerts"},
		},
	}

	stub := &stubNotificationConfigStore{
		getFn: func(ctx context.Context, tenantID string) (*console.TenantNotificationConfig, bool, error) {
			return cfg, true, nil
		},
		setFn: func(ctx context.Context, tenantID string, cfg console.TenantNotificationConfig) error {
			t.Fatal("SetTenantNotificationConfig should not be called during GET")
			return nil
		},
	}

	api := &ConsoleAPI{
		log:                     slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{})),
		notificationConfigStore: stub,
	}

	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("tenant_id", "tenant1")
	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/tenant1/notification-config", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rr := httptest.NewRecorder()

	api.handleGetTenantNotificationConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	// We validate response structure lightly (parsing JSON ensures valid schema).
	var got console.TenantNotificationConfig
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if got.ApproverGroup != "tenant_admin" {
		t.Fatalf("expected approver_group, got %q", got.ApproverGroup)
	}
	if len(got.Notify) != 1 || got.Notify[0].Kind != "slack" || got.Notify[0].Channel != "#alerts" {
		t.Fatalf("unexpected notify payload: %#v", got.Notify)
	}
}

func Test_handleUpdateTenantNotificationConfig_NormalizesAndPersists(t *testing.T) {
	stub := &stubNotificationConfigStore{
		getFn: func(ctx context.Context, tenantID string) (*console.TenantNotificationConfig, bool, error) {
			return nil, false, nil
		},
		setFn: func(ctx context.Context, tenantID string, cfg console.TenantNotificationConfig) error {
			return nil
		},
	}

	api := &ConsoleAPI{
		log:                     slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{})),
		notificationConfigStore: stub,
	}

	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("tenant_id", "tenant1")

	// Intentionally messy input to verify normalization trims + lowercases.
	payload := map[string]any{
		"approver_group": "  tenant_admin ",
		"notify": []map[string]any{
			{"kind": "  SlAcK  ", "channel": "  #alerts  "},
		},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPut, "/admin/tenants/tenant1/notification-config", io.NopCloser(bytes.NewReader(body)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rr := httptest.NewRecorder()

	api.handleUpdateTenantNotificationConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if stub.lastSetTenantID != "tenant1" {
		t.Fatalf("expected tenant1 to be persisted, got %q", stub.lastSetTenantID)
	}
	if stub.lastSetCfg == nil {
		t.Fatal("expected lastSetCfg to be captured")
	}
	if stub.lastSetCfg.ApproverGroup != "tenant_admin" {
		t.Fatalf("expected normalized approver_group, got %q", stub.lastSetCfg.ApproverGroup)
	}
	if len(stub.lastSetCfg.Notify) != 1 {
		t.Fatalf("expected 1 notify entry, got %d", len(stub.lastSetCfg.Notify))
	}
	if stub.lastSetCfg.Notify[0].Kind != "slack" || stub.lastSetCfg.Notify[0].Channel != "#alerts" {
		t.Fatalf("unexpected normalized notify payload: %#v", stub.lastSetCfg.Notify[0])
	}
}

func Test_handleUpdateTenantNotificationConfig_StoreFailureReturns500(t *testing.T) {
	stub := &stubNotificationConfigStore{
		getFn: func(ctx context.Context, tenantID string) (*console.TenantNotificationConfig, bool, error) {
			return nil, false, nil
		},
		setFn: func(ctx context.Context, tenantID string, cfg console.TenantNotificationConfig) error {
			return errors.New("write failed")
		},
	}

	api := &ConsoleAPI{
		log:                     slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{})),
		notificationConfigStore: stub,
	}

	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("tenant_id", "tenant1")
	body := []byte(`{"notify":[{"kind":"slack","channel":"#alerts"}]}`)

	req := httptest.NewRequest(http.MethodPut, "/admin/tenants/tenant1/notification-config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rr := httptest.NewRecorder()

	api.handleUpdateTenantNotificationConfig(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", rr.Code, rr.Body.String())
	}
}
