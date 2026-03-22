package main

import (
	"context"
	"encoding/json"
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

type fakeAuthSessionStore struct {
	touchActive     bool
	touchErr        error
	touchSessionID  string
	touchUserID     string
	listSessions    []console.AuthSession
	listErr         error
	listTenantID    string
	listUserID      string
	revokeOK        bool
	revokeErr       error
	revokeSessionID string
	revokeTenantID  string
	revokeRevokedBy string
	counts          map[string]int64
	countsErr       error
}

func (f *fakeAuthSessionStore) TouchAuthSession(_ context.Context, sessionID, userID string, _ time.Time) (bool, error) {
	f.touchSessionID = sessionID
	f.touchUserID = userID
	return f.touchActive, f.touchErr
}

func (f *fakeAuthSessionStore) ListAuthSessions(_ context.Context, tenantID, userID string, _, _ int) ([]console.AuthSession, error) {
	f.listTenantID = tenantID
	f.listUserID = userID
	return f.listSessions, f.listErr
}

func (f *fakeAuthSessionStore) ListActiveAuthSessionCounts(_ context.Context, _ string) (map[string]int64, error) {
	return f.counts, f.countsErr
}

func (f *fakeAuthSessionStore) RevokeAuthSession(_ context.Context, sessionID, tenantID, revokedBy string, _ time.Time) (bool, error) {
	f.revokeSessionID = sessionID
	f.revokeTenantID = tenantID
	f.revokeRevokedBy = revokedBy
	return f.revokeOK, f.revokeErr
}

func newTestAuthSessionAPI(store authSessionStore) *ConsoleAPI {
	return &ConsoleAPI{
		log:              slog.New(slog.NewTextHandler(io.Discard, nil)),
		jwtCfg:           testJWTConfig(),
		authSessionStore: store,
	}
}

func TestJWTAuthMiddlewareRejectsRevokedSession(t *testing.T) {
	store := &fakeAuthSessionStore{}
	api := newTestAuthSessionAPI(store)
	token, err := console.GenerateToken(testJWTConfig(), console.JWTClaims{
		Sub:   "user-1",
		SID:   "sess-1",
		Email: "user@example.com",
	})
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	api.jwtAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rr.Code, rr.Body.String())
	}
	if store.touchSessionID != "sess-1" || store.touchUserID != "user-1" {
		t.Fatalf("unexpected touch arguments: session=%q user=%q", store.touchSessionID, store.touchUserID)
	}
}

func TestJWTAuthMiddlewareAllowsLegacyTokenWithoutSessionID(t *testing.T) {
	store := &fakeAuthSessionStore{}
	api := newTestAuthSessionAPI(store)
	token, err := console.GenerateToken(testJWTConfig(), console.JWTClaims{
		Sub:   "user-1",
		Email: "user@example.com",
	})
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	api.jwtAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", rr.Code, rr.Body.String())
	}
	if store.touchSessionID != "" {
		t.Fatalf("expected no auth-session touch for legacy token, got %q", store.touchSessionID)
	}
}

func TestJWTAuthMiddlewareAcceptsCaseInsensitiveTrimmedBearerToken(t *testing.T) {
	store := &fakeAuthSessionStore{}
	api := newTestAuthSessionAPI(store)
	token, err := console.GenerateToken(testJWTConfig(), console.JWTClaims{
		Sub:   "user-1",
		Email: "user@example.com",
	})
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	req.Header.Set("Authorization", "   bearer    "+token+"   ")
	rr := httptest.NewRecorder()

	api.jwtAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleListAuthSessionsScopesTenantAdminRequests(t *testing.T) {
	store := &fakeAuthSessionStore{
		listSessions: []console.AuthSession{{ID: "sess-1", UserID: "user-1"}},
	}
	api := newTestAuthSessionAPI(store)
	claims := &console.JWTClaims{
		Sub:    "admin-1",
		Roles:  []string{"tenant_admin"},
		Tenant: "tenant-1",
	}
	ctx := context.WithValue(context.Background(), claimsKey{}, claims)
	req := httptest.NewRequest(http.MethodGet, "/admin/auth-sessions?user_id=user-1&tenant_id=ignored", nil).WithContext(ctx)
	rr := httptest.NewRecorder()

	api.handleListAuthSessions(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if store.listTenantID != "tenant-1" {
		t.Fatalf("expected tenant scope tenant-1, got %q", store.listTenantID)
	}
	if store.listUserID != "user-1" {
		t.Fatalf("expected user filter user-1, got %q", store.listUserID)
	}
}

func TestHandleRevokeAuthSessionUsesTenantScope(t *testing.T) {
	store := &fakeAuthSessionStore{revokeOK: true}
	api := newTestAuthSessionAPI(store)
	claims := &console.JWTClaims{
		Sub:    "admin-1",
		Roles:  []string{"tenant_admin"},
		Tenant: "tenant-1",
	}
	ctx := context.WithValue(context.Background(), claimsKey{}, claims)
	req := httptest.NewRequest(http.MethodPost, "/admin/auth-sessions/sess-1/revoke", nil).WithContext(ctx)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("session_id", "sess-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rr := httptest.NewRecorder()

	api.handleRevokeAuthSession(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if store.revokeTenantID != "tenant-1" {
		t.Fatalf("expected revoke tenant scope tenant-1, got %q", store.revokeTenantID)
	}
	if store.revokeSessionID != "sess-1" || store.revokeRevokedBy != "admin-1" {
		t.Fatalf("unexpected revoke arguments: session=%q revoked_by=%q", store.revokeSessionID, store.revokeRevokedBy)
	}
}

func TestHandleLogoutRevokesCurrentSession(t *testing.T) {
	store := &fakeAuthSessionStore{revokeOK: true}
	api := newTestAuthSessionAPI(store)
	claims := &console.JWTClaims{
		Sub: "user-1",
		SID: "sess-current",
	}
	ctx := context.WithValue(context.Background(), claimsKey{}, claims)
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil).WithContext(ctx)
	rr := httptest.NewRecorder()

	api.handleLogout(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if store.revokeSessionID != "sess-current" {
		t.Fatalf("expected revoke of sess-current, got %q", store.revokeSessionID)
	}
}

func TestHandleListAuthSessionsReturnsStructuredErrorOnFailure(t *testing.T) {
	store := &fakeAuthSessionStore{listErr: context.DeadlineExceeded}
	api := newTestAuthSessionAPI(store)
	claims := &console.JWTClaims{
		Sub:   "admin-1",
		Roles: []string{"platform_admin"},
	}
	ctx := context.WithValue(context.Background(), claimsKey{}, claims)
	req := httptest.NewRequest(http.MethodGet, "/admin/auth-sessions", nil).WithContext(ctx)
	rr := httptest.NewRecorder()

	api.handleListAuthSessions(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", rr.Code, rr.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected JSON error body, got %v body=%s", err, rr.Body.String())
	}
	if !strings.Contains(payload["message"].(string), "failed to list auth sessions") {
		t.Fatalf("unexpected error message: %v", payload["message"])
	}
}
