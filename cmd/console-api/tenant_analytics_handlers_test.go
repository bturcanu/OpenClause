package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bturcanu/OpenClause/pkg/console"
	"github.com/go-chi/chi/v5"
)

type fakeAnalyticsStore struct {
	lastTenant      string
	lastSince       time.Time
	lastBucket      int
	lastTopAgents   int
	returnSummary   *console.TenantAnalyticsSummary
	returnErr       error
}

func (f *fakeAnalyticsStore) GetTenantAnalyticsSummary(ctx context.Context, tenantID string, since time.Time, bucketMinutes int, topAgents int) (*console.TenantAnalyticsSummary, error) {
	_ = ctx
	f.lastTenant = tenantID
	f.lastSince = since
	f.lastBucket = bucketMinutes
	f.lastTopAgents = topAgents
	return f.returnSummary, f.returnErr
}

func setRouteParamsAnalytics(req *http.Request, params map[string]string) *http.Request {
	routeCtx := chi.NewRouteContext()
	for k, v := range params {
		routeCtx.URLParams.Add(k, v)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
}

func Test_handleTenantAnalyticsSummary_parsesParamsAndCallsStore(t *testing.T) {
	fs := &fakeAnalyticsStore{
		returnSummary: &console.TenantAnalyticsSummary{
			Totals: console.DecisionTotals{},
			Trend:  []console.DecisionTrendBucket{},
			RiskHeatmap: []console.RiskHeatmapRow{},
			PerAgent: []console.AgentBreakdownRow{},
			OnboardingChecklist: console.OnboardingChecklist{},
		},
	}
	api := &ConsoleAPI{
		log:            slog.Default(),
		analyticsStore: fs,
	}

	before := time.Now().UTC()
	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/tenant1/analytics/summary?range=6h&bucket_minutes=30&top_agents=3", bytes.NewReader([]byte{}))
	req.Header.Set("Content-Type", "application/json")
	req = setRouteParamsAnalytics(req, map[string]string{"tenant_id": "tenant1"})
	rr := httptest.NewRecorder()

	api.handleTenantAnalyticsSummary(rr, req)
	after := time.Now().UTC()

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if fs.lastTenant != "tenant1" {
		t.Fatalf("expected tenant_id tenant1, got %q", fs.lastTenant)
	}
	if fs.lastBucket != 30 {
		t.Fatalf("expected bucket_minutes 30, got %d", fs.lastBucket)
	}
	if fs.lastTopAgents != 3 {
		t.Fatalf("expected top_agents 3, got %d", fs.lastTopAgents)
	}

	minSince := before.Add(-6*time.Hour - 2*time.Second)
	maxSince := after.Add(-6*time.Hour + 2*time.Second)
	if fs.lastSince.Before(minSince) || fs.lastSince.After(maxSince) {
		t.Fatalf("since out of expected range: got %s, expected between %s and %s", fs.lastSince, minSince, maxSince)
	}

	var got console.TenantAnalyticsSummary
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("response JSON was invalid: %v body=%s", err, rr.Body.String())
	}
	if got.Totals.TotalEvents != 0 {
		t.Fatalf("unexpected totals in response: %+v", got.Totals)
	}
}

func Test_handleTenantAnalyticsSummary_nilSummaryReturnsStableEmptyJSON(t *testing.T) {
	fs := &fakeAnalyticsStore{returnSummary: nil}
	api := &ConsoleAPI{
		log:            slog.Default(),
		analyticsStore: fs,
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/tenant1/analytics/summary", nil)
	req = setRouteParamsAnalytics(req, map[string]string{"tenant_id": "tenant1"})
	rr := httptest.NewRecorder()

	api.handleTenantAnalyticsSummary(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rr.Body.String())
	}
	if got["totals"] == nil || got["trend"] == nil || got["risk_heatmap"] == nil || got["per_agent"] == nil || got["onboarding_checklist"] == nil {
		t.Fatalf("expected stable empty summary shape, got %+v", got)
	}
}
