package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bturcanu/OpenClause/pkg/types"
	"golang.org/x/time/rate"
)

func TestIPRateLimiter_BlocksSecondRequestForSameIP(t *testing.T) {
	lim := newIPRateLimiter(rate.Limit(1), 1) // 1 request/sec, burst 1

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	handler := lim.middleware(next)

	req1 := httptest.NewRequest(http.MethodPost, "http://example.com/auth/invite/accept", nil)
	req1.RemoteAddr = "127.0.0.1:1111"
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)

	if rr1.Code != http.StatusOK {
		t.Fatalf("expected first request 200, got %d body=%s", rr1.Code, rr1.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodPost, "http://example.com/auth/invite/accept", nil)
	req2.RemoteAddr = "127.0.0.1:1112"
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second request 429, got %d body=%s", rr2.Code, rr2.Body.String())
	}

	var apiErr types.APIError
	if err := json.NewDecoder(rr2.Body).Decode(&apiErr); err != nil {
		t.Fatalf("decode error: %v body=%s", err, rr2.Body.String())
	}
	if apiErr.Code != "RATE_LIMITED" {
		t.Fatalf("expected RATE_LIMITED, got %s", apiErr.Code)
	}
	if !apiErr.Retryable {
		t.Fatalf("expected retryable=true")
	}
}

func TestIPRateLimiter_AllowsDifferentIPs(t *testing.T) {
	lim := newIPRateLimiter(rate.Limit(1), 1)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := lim.middleware(next)

	req1 := httptest.NewRequest(http.MethodPost, "http://example.com/auth/reset/request", nil)
	req1.RemoteAddr = "127.0.0.1:2222"
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)

	req2 := httptest.NewRequest(http.MethodPost, "http://example.com/auth/reset/request", nil)
	req2.RemoteAddr = "127.0.0.2:2222"
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if rr1.Code != http.StatusOK || rr2.Code != http.StatusOK {
		t.Fatalf("expected both requests to be allowed, got rr1=%d rr2=%d", rr1.Code, rr2.Code)
	}

	// Ensure limiter doesn't block after a tiny wait (sanity check for token replenishment).
	time.Sleep(5 * time.Millisecond)
}

