package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bturcanu/OpenClause/pkg/console"
	"github.com/go-chi/chi/v5"
)

type fakeAlertsStore struct {
	lastCreate *console.AlertRule
	lastUpdate *struct {
		TenantID string
		RuleID   string
		Name     string
		RuleType string
		Config   json.RawMessage
		Enabled  bool
	}
	lastDelete *struct {
		TenantID string
		RuleID   string
	}
	updateErr error
	deleteErr error

	lastListSince *struct {
		TenantID string
		Since    time.Time
		Limit    int
	}

	listRules  []console.AlertRule
	getRule    *console.AlertRule
	listEvents []console.AlertEvent
}

func (f *fakeAlertsStore) ListAlertRules(_ context.Context, tenantID string) ([]console.AlertRule, error) {
	_ = tenantID
	return f.listRules, nil
}

func (f *fakeAlertsStore) CreateAlertRule(_ context.Context, rule console.AlertRule) (*console.AlertRule, error) {
	c := rule
	f.lastCreate = &c
	created := &console.AlertRule{
		ID:        "rule-1",
		TenantID:  rule.TenantID,
		Name:      rule.Name,
		RuleType:  rule.RuleType,
		Config:    rule.Config,
		Enabled:   rule.Enabled,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	return created, nil
}

func (f *fakeAlertsStore) UpdateAlertRule(_ context.Context, tenantID, ruleID, name, ruleType string, config json.RawMessage, enabled bool) error {
	f.lastUpdate = &struct {
		TenantID string
		RuleID   string
		Name     string
		RuleType string
		Config   json.RawMessage
		Enabled  bool
	}{
		TenantID: tenantID,
		RuleID:   ruleID,
		Name:     name,
		RuleType: ruleType,
		Config:   config,
		Enabled:  enabled,
	}
	return f.updateErr
}

func (f *fakeAlertsStore) GetAlertRule(_ context.Context, tenantID, ruleID string) (*console.AlertRule, error) {
	_ = tenantID
	_ = ruleID
	return f.getRule, nil
}

func (f *fakeAlertsStore) ListAlertEvents(_ context.Context, tenantID string, limit, offset int) ([]console.AlertEvent, error) {
	_ = tenantID
	_ = limit
	_ = offset
	return f.listEvents, nil
}

func (f *fakeAlertsStore) DeleteAlertRule(_ context.Context, tenantID, ruleID string) error {
	f.lastDelete = &struct {
		TenantID string
		RuleID   string
	}{
		TenantID: tenantID,
		RuleID:   ruleID,
	}
	return f.deleteErr
}

func (f *fakeAlertsStore) ListAlertEventsSince(_ context.Context, tenantID string, since time.Time, limit int) ([]console.AlertEvent, error) {
	f.lastListSince = &struct {
		TenantID string
		Since    time.Time
		Limit    int
	}{
		TenantID: tenantID,
		Since:    since,
		Limit:    limit,
	}
	return f.listEvents, nil
}

func setRouteParams(req *http.Request, params map[string]string) *http.Request {
	routeCtx := chi.NewRouteContext()
	for k, v := range params {
		routeCtx.URLParams.Add(k, v)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
}

func Test_handleCreateTenantAlertRule_canonicalizesConfigAndCreates(t *testing.T) {
	fs := &fakeAlertsStore{}
	api := &ConsoleAPI{
		log:         slog.Default(),
		alertsStore: fs,
	}

	body := map[string]any{
		"name":    "deny-spike",
		"kind":    "deny_spike",
		"enabled": true,
		"config_json": map[string]any{
			"N": 3,
			"M": 5,
		},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/admin/tenants/tenant1/alerts/rules", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req = setRouteParams(req, map[string]string{"tenant_id": "tenant1"})

	rr := httptest.NewRecorder()
	api.handleCreateTenantAlertRule(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rr.Code, rr.Body.String())
	}
	if fs.lastCreate == nil {
		t.Fatal("expected store CreateAlertRule to be called")
	}
	if fs.lastCreate.TenantID != "tenant1" {
		t.Fatalf("expected tenant_id tenant1, got %q", fs.lastCreate.TenantID)
	}
	if fs.lastCreate.RuleType != "deny_spike" {
		t.Fatalf("expected rule_type deny_spike, got %q", fs.lastCreate.RuleType)
	}

	var cfg map[string]any
	if err := json.Unmarshal(fs.lastCreate.Config, &cfg); err != nil {
		t.Fatalf("config_json was not valid JSON: %v", err)
	}
	if cfg["n"] != float64(3) || cfg["m_minutes"] != float64(5) {
		t.Fatalf("unexpected canonical config: %+v", cfg)
	}
}

func Test_handleCreateAlertRule_UsesBodyTenantAndTrimsName(t *testing.T) {
	fs := &fakeAlertsStore{}
	api := &ConsoleAPI{
		log:         slog.Default(),
		alertsStore: fs,
	}

	body := map[string]any{
		"tenant_id": "tenant1",
		"name":      "  deny-spike  ",
		"kind":      "deny_spike",
		"enabled":   true,
		"config_json": map[string]any{
			"N": 3,
			"M": 5,
		},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/admin/alerts/rules", bytes.NewReader(b))
	req = req.WithContext(context.WithValue(req.Context(), claimsKey{}, &console.JWTClaims{Roles: []string{"platform_admin"}}))

	rr := httptest.NewRecorder()
	api.handleCreateAlertRule(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rr.Code, rr.Body.String())
	}
	if fs.lastCreate == nil {
		t.Fatal("expected CreateAlertRule to be called")
	}
	if fs.lastCreate.TenantID != "tenant1" {
		t.Fatalf("expected tenant1, got %q", fs.lastCreate.TenantID)
	}
	if fs.lastCreate.Name != "deny-spike" {
		t.Fatalf("expected trimmed name, got %q", fs.lastCreate.Name)
	}
}

func Test_handleCreateAlertRule_TenantScopeOverridesBodyTenant(t *testing.T) {
	fs := &fakeAlertsStore{}
	api := &ConsoleAPI{
		log:         slog.Default(),
		alertsStore: fs,
	}

	body := map[string]any{
		"tenant_id": "tenant-other",
		"name":      "rule",
		"kind":      "deny_spike",
		"enabled":   true,
		"config_json": map[string]any{
			"n":         3,
			"m_minutes": 5,
		},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/admin/alerts/rules", bytes.NewReader(b))
	req = req.WithContext(context.WithValue(req.Context(), claimsKey{}, &console.JWTClaims{
		Tenant: "tenant-scoped",
		Roles:  []string{"tenant_admin"},
	}))

	rr := httptest.NewRecorder()
	api.handleCreateAlertRule(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rr.Code, rr.Body.String())
	}
	if fs.lastCreate == nil {
		t.Fatal("expected CreateAlertRule to be called")
	}
	if fs.lastCreate.TenantID != "tenant-scoped" {
		t.Fatalf("expected tenant-scoped, got %q", fs.lastCreate.TenantID)
	}
}

func Test_handleUpdateTenantAlertRule_callsUpdateAndReturnsRule(t *testing.T) {
	fs := &fakeAlertsStore{
		getRule: &console.AlertRule{
			ID:        "rule-1",
			TenantID:  "tenant1",
			Name:      "updated",
			RuleType:  "deny_spike",
			Config:    json.RawMessage(`{"n":3,"m_minutes":10}`),
			Enabled:   false,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		},
	}
	api := &ConsoleAPI{
		log:         slog.Default(),
		alertsStore: fs,
	}

	body := map[string]any{
		"name":    "updated",
		"kind":    "deny_spike",
		"enabled": false,
		"config_json": map[string]any{
			"n":         3,
			"m_minutes": 10,
		},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, "/admin/tenants/tenant1/alerts/rules/rule-1", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req = setRouteParams(req, map[string]string{"tenant_id": "tenant1", "rule_id": "rule-1"})

	rr := httptest.NewRecorder()
	api.handleUpdateTenantAlertRule(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if fs.lastUpdate == nil {
		t.Fatal("expected UpdateAlertRule to be called")
	}
	if fs.lastUpdate.TenantID != "tenant1" || fs.lastUpdate.RuleID != "rule-1" {
		t.Fatalf("unexpected update routing: %+v", fs.lastUpdate)
	}
	if fs.lastUpdate.Enabled != false {
		t.Fatalf("expected enabled=false, got %v", fs.lastUpdate.Enabled)
	}
}

func Test_handleDeleteTenantAlertRule_deletesRule(t *testing.T) {
	fs := &fakeAlertsStore{}
	api := &ConsoleAPI{
		log:         slog.Default(),
		alertsStore: fs,
	}

	req := httptest.NewRequest(http.MethodDelete, "/admin/tenants/tenant1/alerts/rules/rule-1", nil)
	req = setRouteParams(req, map[string]string{"tenant_id": "tenant1", "rule_id": "rule-1"})

	rr := httptest.NewRecorder()
	api.handleDeleteTenantAlertRule(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", rr.Code, rr.Body.String())
	}
	if fs.lastDelete == nil {
		t.Fatal("expected DeleteAlertRule to be called")
	}
	if fs.lastDelete.TenantID != "tenant1" || fs.lastDelete.RuleID != "rule-1" {
		t.Fatalf("unexpected delete routing: %+v", fs.lastDelete)
	}
}

func Test_handleUpdateTenantAlertRule_returnsNotFoundForMissingRule(t *testing.T) {
	fs := &fakeAlertsStore{updateErr: console.ErrAlertRuleNotFound}
	api := &ConsoleAPI{
		log:         slog.Default(),
		alertsStore: fs,
	}

	body := map[string]any{
		"name":    "updated",
		"kind":    "deny_spike",
		"enabled": false,
		"config_json": map[string]any{
			"n":         3,
			"m_minutes": 10,
		},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, "/admin/tenants/tenant1/alerts/rules/rule-1", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req = setRouteParams(req, map[string]string{"tenant_id": "tenant1", "rule_id": "rule-1"})

	rr := httptest.NewRecorder()
	api.handleUpdateTenantAlertRule(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
	var bodyResp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &bodyResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if bodyResp["message"] != "alert rule not found" {
		t.Fatalf("unexpected response: %+v", bodyResp)
	}
}

func Test_handleDeleteTenantAlertRule_returnsNotFoundForMissingRule(t *testing.T) {
	fs := &fakeAlertsStore{deleteErr: console.ErrAlertRuleNotFound}
	api := &ConsoleAPI{
		log:         slog.Default(),
		alertsStore: fs,
	}

	req := httptest.NewRequest(http.MethodDelete, "/admin/tenants/tenant1/alerts/rules/rule-1", nil)
	req = setRouteParams(req, map[string]string{"tenant_id": "tenant1", "rule_id": "rule-1"})

	rr := httptest.NewRecorder()
	api.handleDeleteTenantAlertRule(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
	var bodyResp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &bodyResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if bodyResp["message"] != "alert rule not found" {
		t.Fatalf("unexpected response: %+v", bodyResp)
	}
}

func Test_handleListTenantAlertEvents_usesSinceAndLimit(t *testing.T) {
	nextAttempt := time.Now().UTC().Add(15 * time.Minute).Truncate(time.Second)
	fs := &fakeAlertsStore{
		listEvents: []console.AlertEvent{
			{
				ID:            "ae-1",
				RuleID:        "r-1",
				TenantID:      "tenant1",
				Severity:      "warning",
				Message:       "hello",
				Status:        "pending",
				AttemptCount:  2,
				NextAttemptAt: nextAttempt,
				LastError:     "webhook retry pending",
				CreatedAt:     time.Now().UTC(),
			},
		},
	}
	api := &ConsoleAPI{
		log:         slog.Default(),
		alertsStore: fs,
	}

	since := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/tenant1/alerts/events?limit=5&since="+since, nil)
	req = setRouteParams(req, map[string]string{"tenant_id": "tenant1"})

	rr := httptest.NewRecorder()
	api.handleListTenantAlertEvents(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if fs.lastListSince == nil {
		t.Fatal("expected ListAlertEventsSince to be called")
	}
	if fs.lastListSince.TenantID != "tenant1" {
		t.Fatalf("unexpected tenant_id: %q", fs.lastListSince.TenantID)
	}
	if fs.lastListSince.Limit != 5 {
		t.Fatalf("expected limit=5, got %d", fs.lastListSince.Limit)
	}

	var payload []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("expected one alert event, got %d", len(payload))
	}
	gotNextAttempt, ok := payload[0]["next_attempt_at"].(string)
	if !ok || gotNextAttempt == "" {
		t.Fatalf("expected next_attempt_at in response, got %+v", payload[0])
	}
	if gotNextAttempt != nextAttempt.Format(time.RFC3339) {
		t.Fatalf("expected next_attempt_at %q, got %q", nextAttempt.Format(time.RFC3339), gotNextAttempt)
	}
}

func Test_handleListAlertEvents_GlobalEndpointMatchesTenantScopedMetadata(t *testing.T) {
	nextAttempt := time.Now().UTC().Add(10 * time.Minute).Truncate(time.Second)
	fs := &fakeAlertsStore{
		listEvents: []console.AlertEvent{
			{
				ID:            "ae-global-1",
				RuleID:        "rule-1",
				TenantID:      "tenant1",
				Severity:      "warning",
				Message:       "global event",
				Status:        "pending",
				AttemptCount:  3,
				LastError:     "slack failed",
				NextAttemptAt: nextAttempt,
				CreatedAt:     time.Now().UTC(),
			},
		},
	}
	api := &ConsoleAPI{log: slog.Default(), alertsStore: fs}

	req := httptest.NewRequest(http.MethodGet, "/admin/alerts/events?tenant_id=tenant1&limit=5", nil)
	req = req.WithContext(context.WithValue(req.Context(), claimsKey{}, &console.JWTClaims{Roles: []string{"platform_admin"}}))
	rr := httptest.NewRecorder()

	api.handleListAlertEvents(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var payload []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("expected one event, got %+v", payload)
	}
	if payload[0]["tenant_id"] != "tenant1" || payload[0]["attempt_count"] != float64(3) || payload[0]["last_error"] != "slack failed" {
		t.Fatalf("expected retry metadata to match tenant-scoped endpoint, got %+v", payload[0])
	}
	if payload[0]["next_attempt_at"] != nextAttempt.Format(time.RFC3339) {
		t.Fatalf("expected next_attempt_at %q, got %+v", nextAttempt.Format(time.RFC3339), payload[0]["next_attempt_at"])
	}
}

func TestAlertRuleNotFoundSentinelMatchesWrappedErrors(t *testing.T) {
	err := errors.Join(errors.New("wrapped"), console.ErrAlertRuleNotFound)

	if !errors.Is(err, console.ErrAlertRuleNotFound) {
		t.Fatalf("expected wrapped error to match ErrAlertRuleNotFound")
	}
}
