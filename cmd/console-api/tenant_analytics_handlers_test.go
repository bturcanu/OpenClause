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
	lastTenant    string
	lastSince     time.Time
	lastBucket    int
	lastTopAgents int
	returnSummary *console.TenantAnalyticsSummary
	returnErr     error
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
			Totals:              console.DecisionTotals{},
			Trend:               []console.DecisionTrendBucket{},
			RiskHeatmap:         []console.RiskHeatmapRow{},
			PerAgent:            []console.AgentBreakdownRow{},
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

func Test_handleTenantAnalyticsSummary_returnsJSONValuesFromStore(t *testing.T) {
	fs := &fakeAnalyticsStore{
		returnSummary: &console.TenantAnalyticsSummary{
			RangeStart: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
			RangeEnd:   time.Date(2026, 1, 15, 14, 0, 0, 0, time.UTC),
			Totals: console.DecisionTotals{
				TotalEvents:  10,
				AllowCount:   4,
				DenyCount:    3,
				ApproveCount: 3,
			},
			Trend: []console.DecisionTrendBucket{
				{
					Bucket:       time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
					Total:        3,
					AllowCount:   1,
					DenyCount:    1,
					ApproveCount: 1,
				},
			},
			RiskHeatmap: []console.RiskHeatmapRow{
				{RiskScore: 0},
				{RiskScore: 1},
				{RiskScore: 2, AllowCount: 4, Total: 4},
			},
			PerAgent: []console.AgentBreakdownRow{
				{AgentID: "agent-a", AllowCount: 2, DenyCount: 1, ApproveCount: 2, Total: 5},
				{AgentID: "agent-b", AllowCount: 2, DenyCount: 2, ApproveCount: 1, Total: 5},
			},
			OnboardingChecklist: console.OnboardingChecklist{HasToolcall: true},
		},
	}
	api := &ConsoleAPI{
		log:            slog.Default(),
		analyticsStore: fs,
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/tenant-analytics/analytics/summary", nil)
	req = setRouteParamsAnalytics(req, map[string]string{"tenant_id": "tenant-analytics"})
	rr := httptest.NewRecorder()

	api.handleTenantAnalyticsSummary(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var got struct {
		Totals struct {
			TotalEvents  int `json:"total_events"`
			AllowCount   int `json:"allow_count"`
			DenyCount    int `json:"deny_count"`
			ApproveCount int `json:"approve_count"`
		} `json:"totals"`
		Trend []struct {
			Bucket       string `json:"bucket"`
			Total        int    `json:"total"`
			AllowCount   int    `json:"allow_count"`
			DenyCount    int    `json:"deny_count"`
			ApproveCount int    `json:"approve_count"`
		} `json:"trend"`
		RiskHeatmap []struct {
			RiskScore  int `json:"risk_score"`
			AllowCount int `json:"allow_count"`
			Total      int `json:"total"`
		} `json:"risk_heatmap"`
		PerAgent []struct {
			AgentID      string `json:"agent_id"`
			AllowCount   int    `json:"allow_count"`
			DenyCount    int    `json:"deny_count"`
			ApproveCount int    `json:"approve_count"`
			Total        int    `json:"total"`
		} `json:"per_agent"`
		OnboardingChecklist struct {
			HasToolcall bool `json:"has_toolcall"`
		} `json:"onboarding_checklist"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rr.Body.String())
	}
	if got.Totals.TotalEvents != 10 || got.Totals.AllowCount != 4 || got.Totals.DenyCount != 3 || got.Totals.ApproveCount != 3 {
		t.Fatalf("unexpected totals JSON: %+v", got.Totals)
	}
	if len(got.Trend) != 1 || got.Trend[0].Bucket != "2026-01-15T10:00:00Z" || got.Trend[0].ApproveCount != 1 {
		t.Fatalf("unexpected trend JSON: %+v", got.Trend)
	}
	if len(got.RiskHeatmap) != 3 || got.RiskHeatmap[2].RiskScore != 2 || got.RiskHeatmap[2].AllowCount != 4 || got.RiskHeatmap[2].Total != 4 {
		t.Fatalf("unexpected risk_heatmap JSON: %+v", got.RiskHeatmap)
	}
	if len(got.PerAgent) != 2 || got.PerAgent[0].AgentID != "agent-a" || got.PerAgent[1].AgentID != "agent-b" {
		t.Fatalf("unexpected per_agent JSON: %+v", got.PerAgent)
	}
	if !got.OnboardingChecklist.HasToolcall {
		t.Fatalf("expected onboarding JSON flag to round-trip, got %+v", got.OnboardingChecklist)
	}
}
