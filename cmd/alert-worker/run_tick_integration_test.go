package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bturcanu/OpenClause/internal/testdb"
	"github.com/bturcanu/OpenClause/pkg/console"
	"github.com/bturcanu/OpenClause/pkg/connectors"
	"github.com/bturcanu/OpenClause/pkg/types"
)

func newAlertWorkerStore(t *testing.T) (*console.Store, context.Context) {
	t.Helper()
	h := testdb.New(t)
	return console.NewStore(h.Pool()), context.Background()
}

func createAlertTenant(t *testing.T, store *console.Store, ctx context.Context) *console.Tenant {
	t.Helper()
	tenant, err := store.CreateTenant(ctx, "Alert Worker Tenant", nil)
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	if err := store.SetTenantNotificationConfig(ctx, tenant.ID, console.TenantNotificationConfig{
		Notify: []types.PolicyNotify{{Kind: "slack", Channel: "#alerts"}},
	}); err != nil {
		t.Fatalf("SetTenantNotificationConfig: %v", err)
	}
	return tenant
}

func createAlertRuleAndEvent(t *testing.T, store *console.Store, ctx context.Context, tenantID string) *console.AlertEvent {
	t.Helper()
	rule, err := store.CreateAlertRule(ctx, console.AlertRule{
		TenantID: tenantID,
		Name:     "deny-spike",
		RuleType: "deny_spike",
		Config:   json.RawMessage(`{"n":3,"m_minutes":5}`),
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}
	event, err := store.CreateAlertEvent(ctx, rule.ID, tenantID, "warning", "threshold exceeded", json.RawMessage(`{"count":3}`))
	if err != nil {
		t.Fatalf("CreateAlertEvent: %v", err)
	}
	if _, err := store.Pool().Exec(ctx, `UPDATE alert_events SET next_attempt_at = $2 WHERE id = $1`, event.ID, time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatalf("make alert event due: %v", err)
	}
	return event
}

func TestRunTickRetriesDueAlertEventsAndPersistsLastError(t *testing.T) {
	store, ctx := newAlertWorkerStore(t)
	tenant := createAlertTenant(t, store, ctx)
	event := createAlertRuleAndEvent(t, store, ctx, tenant.ID)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(connectors.ExecResponse{Status: "error", Error: "connector down"})
	}))
	t.Cleanup(srv.Close)

	before := time.Now().UTC()
	if err := runTick(ctx, store, srv.URL, "internal-token", nil, 10); err != nil {
		t.Fatalf("runTick: %v", err)
	}

	events, err := store.ListAlertEvents(ctx, tenant.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListAlertEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one alert event, got %+v", events)
	}
	got := events[0]
	if got.ID != event.ID || got.Status != "pending" || got.AttemptCount != 1 {
		t.Fatalf("expected pending retried event, got %+v", got)
	}
	if got.LastError == "" {
		t.Fatalf("expected last_error to be persisted after failure")
	}
	if !got.NextAttemptAt.After(before) {
		t.Fatalf("expected next_attempt_at to be scheduled in the future, got %s", got.NextAttemptAt)
	}
}

func TestRunTickMarksDueAlertEventsSentOnSuccessfulDispatch(t *testing.T) {
	store, ctx := newAlertWorkerStore(t)
	tenant := createAlertTenant(t, store, ctx)
	event := createAlertRuleAndEvent(t, store, ctx, tenant.ID)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(connectors.ExecResponse{Status: "success"})
	}))
	t.Cleanup(srv.Close)

	if err := runTick(ctx, store, srv.URL, "internal-token", nil, 10); err != nil {
		t.Fatalf("runTick: %v", err)
	}

	events, err := store.ListAlertEventsSince(ctx, tenant.ID, time.Now().UTC().Add(-time.Hour), 10)
	if err != nil {
		t.Fatalf("ListAlertEventsSince: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one alert event, got %+v", events)
	}
	got := events[0]
	if got.ID != event.ID || got.Status != "sent" || got.DeliveredAt == nil {
		t.Fatalf("expected sent alert event, got %+v", got)
	}
	if got.LastError != "" {
		t.Fatalf("expected successful send to clear last_error, got %+v", got)
	}
}
