package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bturcanu/OpenClause/pkg/console"
	"github.com/go-chi/chi/v5"
)

func TestRequireTenantRole_TenantMismatchReturns403(t *testing.T) {
	api := &ConsoleAPI{}
	called := false

	h := api.requireTenantRole("tenant_admin", func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("tenant_id", "tenant2")

	claims := &console.JWTClaims{Tenant: "tenant1", Roles: []string{"tenant_admin"}}
	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/tenant2/apikeys", nil)
	ctx := context.WithValue(req.Context(), claimsKey{}, claims)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, routeCtx)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rr.Code, rr.Body.String())
	}
	if called {
		t.Fatal("handler should not be called on tenant mismatch")
	}
}

func TestRequireTenantRole_TenantMatchAllows(t *testing.T) {
	api := &ConsoleAPI{}
	called := false

	h := api.requireTenantRole("tenant_admin", func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("tenant_id", "tenant1")

	claims := &console.JWTClaims{Tenant: "tenant1", Roles: []string{"tenant_admin"}}
	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/tenant1/apikeys", nil)
	ctx := context.WithValue(req.Context(), claimsKey{}, claims)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, routeCtx)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !called {
		t.Fatal("handler should be called on tenant match")
	}
}

func TestRequireTenantRole_PlatformAdminBypassesTenant(t *testing.T) {
	api := &ConsoleAPI{}
	called := false

	h := api.requireTenantRole("tenant_admin", func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("tenant_id", "tenant2")

	claims := &console.JWTClaims{Tenant: "", Roles: []string{"platform_admin"}}
	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/tenant2/apikeys", nil)
	ctx := context.WithValue(req.Context(), claimsKey{}, claims)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, routeCtx)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !called {
		t.Fatal("handler should be called for platform_admin bypass")
	}
}

