// Package auth provides authentication and authorization middleware.
package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/bturcanu/OpenClause/pkg/types"
)

type contextKey string

const tenantKey contextKey = "tenant_id"

// TenantFromContext extracts the authenticated tenant ID from the context.
func TenantFromContext(ctx context.Context) string {
	v, _ := ctx.Value(tenantKey).(string)
	return v
}

// KeyLookup abstracts API key validation so both in-memory and DB-backed
// stores can be used interchangeably by the middleware.
type KeyLookup interface {
	Lookup(apiKey string) (tenantID string, ok bool)
}

// TenantStatusChecker verifies a tenant is active. Checked after key lookup
// so that env-based keys still respect tenant disable status in the DB.
type TenantStatusChecker interface {
	IsTenantActive(ctx context.Context, tenantID string) bool
}

// APIKeyAuth returns middleware that validates API keys and sets tenant context.
// If tenantChecker is non-nil, the middleware also verifies the tenant is active
// after a successful key lookup, ensuring disabled tenants are rejected even
// when the key comes from the env-based store.
func APIKeyAuth(keys KeyLookup, tenantChecker TenantStatusChecker) func(http.Handler) http.Handler {
	skipPaths := map[string]bool{
		"/healthz": true,
		"/readyz":  true,
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if skipPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				auth := r.Header.Get("Authorization")
				if strings.HasPrefix(auth, "Bearer ") {
					apiKey = strings.TrimPrefix(auth, "Bearer ")
				}
			}

			if apiKey == "" {
				types.ErrUnauthorized("missing API key").WriteJSON(w)
				return
			}

			tenantID, ok := keys.Lookup(apiKey)
			if !ok {
				types.ErrUnauthorized("invalid API key").WriteJSON(w)
				return
			}

			if tenantChecker != nil && !tenantChecker.IsTenantActive(r.Context(), tenantID) {
				types.ErrForbidden("tenant disabled").WriteJSON(w)
				return
			}

			ctx := context.WithValue(r.Context(), tenantKey, tenantID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
