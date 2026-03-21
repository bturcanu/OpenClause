package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockTenantChecker struct {
	active map[string]bool
}

func (m *mockTenantChecker) IsTenantActive(_ context.Context, tenantID string) bool {
	return m.active[tenantID]
}

func TestAPIKeyAuth_ValidKey(t *testing.T) {
	ks := NewKeyStore("tenant1:sk-abc")
	handler := APIKeyAuth(ks, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := TenantFromContext(r.Context())
		if tenant != "tenant1" {
			t.Errorf("expected tenant1, got %q", tenant)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/v1/test", nil)
	req.Header.Set("X-API-Key", "sk-abc")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestAPIKeyAuth_InvalidKey(t *testing.T) {
	ks := NewKeyStore("tenant1:sk-abc")
	handler := APIKeyAuth(ks, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/v1/test", nil)
	req.Header.Set("X-API-Key", "bad-key")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAPIKeyAuth_MissingKey(t *testing.T) {
	ks := NewKeyStore("tenant1:sk-abc")
	handler := APIKeyAuth(ks, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/v1/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAPIKeyAuth_SkipsHealthEndpoint(t *testing.T) {
	ks := NewKeyStore("")
	handler := APIKeyAuth(ks, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, path := range []string{"/healthz", "/readyz", "/v1/connectors"} {
		req := httptest.NewRequest("GET", path, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 for %s, got %d", path, rr.Code)
		}
	}
}

func TestAPIKeyAuth_BearerToken(t *testing.T) {
	ks := NewKeyStore("tenant1:sk-abc")
	handler := APIKeyAuth(ks, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := TenantFromContext(r.Context())
		if tenant != "tenant1" {
			t.Errorf("expected tenant1, got %q", tenant)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/v1/test", nil)
	req.Header.Set("Authorization", "Bearer sk-abc")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestAPIKeyAuth_TenantDisabled(t *testing.T) {
	ks := NewKeyStore("tenant1:sk-abc")
	checker := &mockTenantChecker{active: map[string]bool{"tenant1": false}}
	handler := APIKeyAuth(ks, checker)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for disabled tenant")
	}))

	req := httptest.NewRequest("GET", "/v1/test", nil)
	req.Header.Set("X-API-Key", "sk-abc")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestAPIKeyAuth_TenantEnabled(t *testing.T) {
	ks := NewKeyStore("tenant1:sk-abc")
	checker := &mockTenantChecker{active: map[string]bool{"tenant1": true}}
	handler := APIKeyAuth(ks, checker)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/v1/test", nil)
	req.Header.Set("X-API-Key", "sk-abc")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}
