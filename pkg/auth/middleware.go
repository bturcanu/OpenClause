// Package auth provides authentication and authorization middleware.
package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/agenticaccess/governance/pkg/types"
)

type contextKey string

const tenantKey contextKey = "tenant_id"

// TenantFromContext extracts the authenticated tenant ID from the context.
func TenantFromContext(ctx context.Context) string {
	v, _ := ctx.Value(tenantKey).(string)
	return v
}

// APIKeyAuth returns middleware that validates API keys and sets tenant context.
func APIKeyAuth(keys *KeyStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip health/ready endpoints.
			if strings.HasPrefix(r.URL.Path, "/healthz") ||
				strings.HasPrefix(r.URL.Path, "/readyz") ||
				strings.HasPrefix(r.URL.Path, "/metrics") {
				next.ServeHTTP(w, r)
				return
			}

			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				// Also check Authorization: Bearer
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

			ctx := context.WithValue(r.Context(), tenantKey, tenantID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
