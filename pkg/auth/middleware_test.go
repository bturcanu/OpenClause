package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIKeyAuth_ValidKey(t *testing.T) {
	ks := NewKeyStore("tenant1:sk-abc")
	handler := APIKeyAuth(ks)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	handler := APIKeyAuth(ks)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	handler := APIKeyAuth(ks)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	handler := APIKeyAuth(ks)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, path := range []string{"/healthz", "/readyz", "/metrics"} {
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
	handler := APIKeyAuth(ks)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
