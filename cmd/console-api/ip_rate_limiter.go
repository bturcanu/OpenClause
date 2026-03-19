package main

import (
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/bturcanu/OpenClause/pkg/types"
	"golang.org/x/time/rate"
)

// ipRateLimiter is a small in-memory rate limiter keyed by client IP.
// It is intended for unauthenticated endpoints only.
type ipRateLimiter struct {
	mu        sync.Mutex
	limiters  map[string]*rate.Limiter
	limit     rate.Limit
	burst     int
	maxInUse  int
}

func newIPRateLimiter(limit rate.Limit, burst int) *ipRateLimiter {
	return &ipRateLimiter{
		limiters: make(map[string]*rate.Limiter),
		limit:    limit,
		burst:    burst,
		// Keep memory bounded; evict all when the map grows too large.
		maxInUse: 1024,
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	// Fallback: chi/middleware may leave RemoteAddr empty.
	if r.RemoteAddr == "" {
		return "unknown"
	}
	// Best-effort for IPv4: strip port if present.
	if i := strings.IndexByte(r.RemoteAddr, ':'); i > 0 {
		return r.RemoteAddr[:i]
	}
	return r.RemoteAddr
}

func (l *ipRateLimiter) limiterFor(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.limiters) > l.maxInUse {
		// Simple eviction: clear to cap unbounded growth.
		l.limiters = make(map[string]*rate.Limiter)
	}

	if lim, ok := l.limiters[ip]; ok {
		return lim
	}
	lim := rate.NewLimiter(l.limit, l.burst)
	l.limiters[ip] = lim
	return lim
}

func (l *ipRateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		lim := l.limiterFor(ip)
		if !lim.Allow() {
			// Ensure callers consistently see the structured API error.
			types.ErrRateLimited().WriteJSON(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}
