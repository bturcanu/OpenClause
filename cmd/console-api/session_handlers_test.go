package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bturcanu/OpenClause/pkg/console"
	"github.com/go-chi/chi/v5"
)

type fakeSessionsStore struct {
	listFilters   console.SessionFilters
	listSessions  []console.Session
	listErr       error
	getSessionID  string
	getScope      string
	getTenantHint string
	getSession    *console.Session
	getErr        error
	timelineID    string
	timelineScope string
	timelineHint  string
	timeline      []console.SessionTimelineEvent
	timelineErr   error
	exportID      string
	exportScope   string
	exportHint    string
	exportErr     error
}

func (f *fakeSessionsStore) ListSessions(_ context.Context, filters console.SessionFilters) ([]console.Session, error) {
	f.listFilters = filters
	return f.listSessions, f.listErr
}

func (f *fakeSessionsStore) GetSession(_ context.Context, sessionID, tenantScope, tenantHint string) (*console.Session, error) {
	f.getSessionID = sessionID
	f.getScope = tenantScope
	f.getTenantHint = tenantHint
	return f.getSession, f.getErr
}

func (f *fakeSessionsStore) GetSessionTimeline(_ context.Context, sessionID, tenantScope, tenantHint string) ([]console.SessionTimelineEvent, error) {
	f.timelineID = sessionID
	f.timelineScope = tenantScope
	f.timelineHint = tenantHint
	return f.timeline, f.timelineErr
}

func (f *fakeSessionsStore) ExportSessionCSV(_ context.Context, sessionID, tenantScope, tenantHint string, w io.Writer) error {
	f.exportID = sessionID
	f.exportScope = tenantScope
	f.exportHint = tenantHint
	if f.exportErr != nil {
		return f.exportErr
	}
	_, _ = io.WriteString(w, "session_id,event_id\nsess-1,evt-1\n")
	return nil
}

func newTestSessionsAPI(store sessionsStore) *ConsoleAPI {
	return &ConsoleAPI{
		log:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		sessionsStore: store,
	}
}

func TestHandleListSessionsScopesTenantAdminAndPassesFilters(t *testing.T) {
	store := &fakeSessionsStore{
		listSessions: []console.Session{{ID: "sess-1"}},
	}
	api := newTestSessionsAPI(store)
	claims := &console.JWTClaims{
		Sub:    "admin-1",
		Roles:  []string{"tenant_admin"},
		Tenant: "tenant-1",
	}
	ctx := context.WithValue(context.Background(), claimsKey{}, claims)
	req := httptest.NewRequest(http.MethodGet, "/admin/sessions?tenant_id=ignored&user_id=user-1&trace_id=trace-1&risk_min=3&decision=deny", nil).WithContext(ctx)
	rr := httptest.NewRecorder()

	api.handleListSessions(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if store.listFilters.TenantID != "tenant-1" {
		t.Fatalf("expected tenant scope tenant-1, got %q", store.listFilters.TenantID)
	}
	if store.listFilters.UserID != "user-1" || store.listFilters.TraceID != "trace-1" {
		t.Fatalf("unexpected filters: %+v", store.listFilters)
	}
	if store.listFilters.RiskMin == nil || *store.listFilters.RiskMin != 3 {
		t.Fatalf("expected risk_min=3, got %+v", store.listFilters.RiskMin)
	}
	if store.listFilters.Decision != "deny" {
		t.Fatalf("expected decision deny, got %q", store.listFilters.Decision)
	}
}

func TestHandleGetSessionRequiresTenantHintForAmbiguousPlatformSession(t *testing.T) {
	store := &fakeSessionsStore{getErr: &console.SessionTenantAmbiguityError{Candidates: []string{"tenant-a", "tenant-b"}}}
	api := newTestSessionsAPI(store)
	claims := &console.JWTClaims{
		Sub:   "admin-1",
		Roles: []string{"platform_admin"},
	}
	ctx := context.WithValue(context.Background(), claimsKey{}, claims)
	req := httptest.NewRequest(http.MethodGet, "/admin/sessions/sess-1", nil).WithContext(ctx)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("session_id", "sess-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rr := httptest.NewRecorder()

	api.handleGetSession(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Code       string   `json:"code"`
		Message    string   `json:"message"`
		Error      string   `json:"error"`
		Candidates []string `json:"candidates"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rr.Body.String())
	}
	if payload.Message != "tenant_id required" || payload.Error != "tenant_id required" {
		t.Fatalf("unexpected ambiguity payload: %+v", payload)
	}
	if len(payload.Candidates) != 2 || payload.Candidates[0] != "tenant-a" || payload.Candidates[1] != "tenant-b" {
		t.Fatalf("unexpected candidates: %+v", payload.Candidates)
	}
}

func TestHandleSessionTimelineReturnsTenantCandidatesForAmbiguousPlatformSession(t *testing.T) {
	store := &fakeSessionsStore{timelineErr: &console.SessionTenantAmbiguityError{Candidates: []string{"tenant-a", "tenant-b"}}}
	api := newTestSessionsAPI(store)
	claims := &console.JWTClaims{Sub: "admin-1", Roles: []string{"platform_admin"}}
	ctx := context.WithValue(context.Background(), claimsKey{}, claims)
	req := httptest.NewRequest(http.MethodGet, "/admin/sessions/sess-1/timeline", nil).WithContext(ctx)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("session_id", "sess-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rr := httptest.NewRecorder()

	api.handleSessionTimeline(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Candidates []string `json:"candidates"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rr.Body.String())
	}
	if len(payload.Candidates) != 2 {
		t.Fatalf("expected candidates in response, got %+v", payload)
	}
}

func TestHandleSessionTimelinePassesTenantHint(t *testing.T) {
	store := &fakeSessionsStore{
		timeline: []console.SessionTimelineEvent{{EventListItem: console.EventListItem{EventID: "evt-1"}}},
	}
	api := newTestSessionsAPI(store)
	claims := &console.JWTClaims{Sub: "admin-1", Roles: []string{"platform_admin"}}
	ctx := context.WithValue(context.Background(), claimsKey{}, claims)
	req := httptest.NewRequest(http.MethodGet, "/admin/sessions/sess-1/timeline?tenant_id=tenant-1", nil).WithContext(ctx)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("session_id", "sess-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rr := httptest.NewRecorder()

	api.handleSessionTimeline(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if store.timelineID != "sess-1" || store.timelineHint != "tenant-1" {
		t.Fatalf("unexpected timeline args: id=%q tenant=%q", store.timelineID, store.timelineHint)
	}
}

func TestHandleExportSessionJSONReturnsSessionBundle(t *testing.T) {
	store := &fakeSessionsStore{
		getSession: &console.Session{ID: "sess-1", TenantID: "tenant-1", LastEventAt: time.Unix(10, 0).UTC()},
		timeline:   []console.SessionTimelineEvent{{EventListItem: console.EventListItem{EventID: "evt-1", SessionID: "sess-1"}}},
	}
	api := newTestSessionsAPI(store)
	claims := &console.JWTClaims{Sub: "admin-1", Roles: []string{"platform_admin"}}
	ctx := context.WithValue(context.Background(), claimsKey{}, claims)
	req := httptest.NewRequest(http.MethodGet, "/admin/sessions/sess-1/export/json?tenant_id=tenant-1", nil).WithContext(ctx)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("session_id", "sess-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rr := httptest.NewRecorder()

	api.handleExportSessionJSON(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Session console.Session                `json:"session"`
		Events  []console.SessionTimelineEvent `json:"events"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rr.Body.String())
	}
	if payload.Session.ID != "sess-1" || len(payload.Events) != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestHandleExportSessionCSVMapsStructuredError(t *testing.T) {
	store := &fakeSessionsStore{exportErr: errors.New("boom")}
	api := newTestSessionsAPI(store)
	claims := &console.JWTClaims{Sub: "admin-1", Roles: []string{"tenant_admin"}, Tenant: "tenant-1"}
	ctx := context.WithValue(context.Background(), claimsKey{}, claims)
	req := httptest.NewRequest(http.MethodGet, "/admin/sessions/sess-1/export/csv", nil).WithContext(ctx)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("session_id", "sess-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rr := httptest.NewRecorder()

	api.handleExportSessionCSV(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "failed to export session") {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}
