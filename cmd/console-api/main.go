// Console API serves the admin console for OpenClause.
// It provides JWT-based auth, RBAC, and admin endpoints for managing
// tenants, agents, API keys, policies, analytics, and more.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bturcanu/OpenClause/pkg/approvals"
	"github.com/bturcanu/OpenClause/pkg/config"
	"github.com/bturcanu/OpenClause/pkg/console"
	"github.com/bturcanu/OpenClause/pkg/policy"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxBodyBytes = 1 << 20

// knownInsecureJWTSecret is the default value from early development.
// The server MUST NOT start with this value.
const knownInsecureJWTSecret = "change-me-in-production-openclause-jwt-secret"

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// CRIT-02: Require JWT secret at startup, reject known insecure default.
	jwtSecret := os.Getenv("CONSOLE_JWT_SECRET")
	if jwtSecret == "" || jwtSecret == knownInsecureJWTSecret {
		log.Error("CONSOLE_JWT_SECRET is required and must not be the default insecure value")
		os.Exit(1)
	}
	if len(jwtSecret) < 32 {
		log.Error("CONSOLE_JWT_SECRET must be at least 32 bytes")
		os.Exit(1)
	}

	pool, err := pgxpool.New(ctx, config.PostgresDSN())
	if err != nil {
		log.Error("postgres connect failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	store := console.NewStore(pool)
	approvalsStore := approvals.NewStore(pool)
	approverAuth := approvals.NewApproverAuthorizer(
		os.Getenv("APPROVER_EMAIL_ALLOWLIST"),
		os.Getenv("APPROVER_SLACK_ALLOWLIST"),
	)

	jwtCfg := console.JWTConfig{
		Secret:      jwtSecret,
		Issuer:      "openclause-console",
		ExpiryHours: config.EnvOrInt("CONSOLE_JWT_EXPIRY_HOURS", 24),
	}

	// CRIT-01: Parse allowed CORS origins from env.
	allowedOrigins := parseCORSOrigins(os.Getenv("CONSOLE_CORS_ORIGINS"))

	api := &ConsoleAPI{
		log:            log,
		store:          store,
		jwtCfg:         jwtCfg,
		approvalsStore: approvalsStore,
		approverAuth:   approverAuth,
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(corsMiddleware(allowedOrigins))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("NOT READY"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	r.Post("/auth/login", api.handleLogin)

	r.Group(func(r chi.Router) {
		r.Use(api.jwtAuthMiddleware)

		r.Get("/admin/analytics/overview", api.handleAnalyticsOverview)
		r.Get("/admin/analytics/timeseries", api.handleAnalyticsTimeseries)

		r.Post("/admin/tenants", api.requireRole("platform_admin", api.handleCreateTenant))
		r.Get("/admin/tenants", api.handleListTenants)
		r.Get("/admin/tenants/{tenant_id}", api.handleGetTenant)
		r.Post("/admin/tenants/{tenant_id}/status", api.requireRole("platform_admin", api.handleUpdateTenantStatus))

		r.Post("/admin/tenants/{tenant_id}/agents", api.requireTenantRole("tenant_admin", api.handleCreateAgent))
		r.Get("/admin/tenants/{tenant_id}/agents", api.requireTenantAccess(api.handleListAgents))

		r.Post("/admin/tenants/{tenant_id}/apikeys", api.requireTenantRole("tenant_admin", api.handleCreateAPIKey))
		r.Get("/admin/tenants/{tenant_id}/apikeys", api.requireTenantAccess(api.handleListAPIKeys))
		r.Post("/admin/tenants/{tenant_id}/apikeys/{key_id}/revoke", api.requireTenantRole("tenant_admin", api.handleRevokeAPIKey))

		r.Get("/admin/approvals/pending", api.handleListPendingApprovals)
		r.Post("/admin/approvals/{id}/approve", api.requireRole("approver", api.handleApproveRequest))
		r.Post("/admin/approvals/{id}/deny", api.requireRole("approver", api.handleDenyRequest))

		r.Get("/admin/events", api.handleListEvents)
		r.Get("/admin/events/{event_id}", api.handleGetEventDetail)
		r.Get("/admin/events/export/csv", api.handleExportEventsCSV)

		r.Get("/admin/sessions", api.handleListSessions)
		r.Get("/admin/sessions/{session_id}/timeline", api.handleSessionTimeline)

		r.Get("/admin/policy/versions", api.handleListPolicyVersions)
		r.Post("/admin/policy/versions", api.requireRole("tenant_admin", api.handleCreatePolicyVersion))
		r.Post("/admin/policy/simulate", api.handleSimulatePolicy)

		r.Get("/admin/alerts/rules", api.handleListAlertRules)
		r.Post("/admin/alerts/rules", api.requireRole("tenant_admin", api.handleCreateAlertRule))
		r.Get("/admin/alerts/events", api.handleListAlertEvents)

		r.Get("/v1/connectors", api.handleListConnectors)

		r.Get("/admin/reports/activity", api.handleListEvents)
		r.Get("/admin/reports/export/csv", api.handleExportEventsCSV)
		r.Get("/admin/reports/export/bundle", api.handleExportBundle)
	})

	addr := config.EnvOr("CONSOLE_API_ADDR", ":8090")
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Info("console-api starting", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "error", err)
			cancel()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down console-api")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Error("server shutdown error", "error", err)
	}
}

type ConsoleAPI struct {
	log            *slog.Logger
	store          *console.Store
	jwtCfg         console.JWTConfig
	approvalsStore *approvals.Store
	approverAuth   *approvals.ApproverAuthorizer
}

type claimsKey struct{}

func (api *ConsoleAPI) jwtAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "missing or invalid authorization header")
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		claims, err := console.ValidateToken(api.jwtCfg, token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid token: "+err.Error())
			return
		}
		ctx := context.WithValue(r.Context(), claimsKey{}, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func claimsFromCtx(ctx context.Context) *console.JWTClaims {
	c, _ := ctx.Value(claimsKey{}).(*console.JWTClaims)
	return c
}

func (api *ConsoleAPI) requireRole(role string, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := claimsFromCtx(r.Context())
		if claims == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if hasRole(claims, "platform_admin") {
			handler(w, r)
			return
		}
		if hasRole(claims, role) {
			handler(w, r)
			return
		}
		writeError(w, http.StatusForbidden, "insufficient permissions")
	}
}

func (api *ConsoleAPI) requireTenantRole(role string, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := claimsFromCtx(r.Context())
		if claims == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if hasRole(claims, "platform_admin") {
			handler(w, r)
			return
		}
		tenantID := chi.URLParam(r, "tenant_id")
		if claims.Tenant != "" && claims.Tenant != tenantID {
			writeError(w, http.StatusForbidden, "access denied for this tenant")
			return
		}
		if hasRole(claims, role) {
			handler(w, r)
			return
		}
		writeError(w, http.StatusForbidden, "insufficient permissions")
	}
}

func (api *ConsoleAPI) requireTenantAccess(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := claimsFromCtx(r.Context())
		if claims == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if hasRole(claims, "platform_admin") {
			handler(w, r)
			return
		}
		tenantID := chi.URLParam(r, "tenant_id")
		if claims.Tenant != "" && claims.Tenant != tenantID {
			writeError(w, http.StatusForbidden, "access denied for this tenant")
			return
		}
		handler(w, r)
	}
}

func hasRole(claims *console.JWTClaims, role string) bool {
	for _, r := range claims.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// HIGH-02: tenantScope now returns "!!deny!!" sentinel for non-platform_admin
// users with an empty tenant claim, preventing cross-tenant data leaks.
func tenantScope(claims *console.JWTClaims) string {
	if hasRole(claims, "platform_admin") {
		return ""
	}
	if claims.Tenant == "" {
		return "!!deny!!"
	}
	return claims.Tenant
}

// ──────────────────────────────────────────────────────────────────────────────
// Auth
// ──────────────────────────────────────────────────────────────────────────────

func (api *ConsoleAPI) handleLogin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if in.Email == "" || in.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password required")
		return
	}

	user, roles, err := api.store.AuthenticateUser(r.Context(), in.Email, in.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	roleNames := make([]string, len(roles))
	var scopedTenant string
	for i, role := range roles {
		roleNames[i] = role.Role
		if role.TenantID != nil {
			scopedTenant = *role.TenantID
		}
	}

	// HIGH-02: Reject non-platform_admin users who have no tenant scope.
	if scopedTenant == "" && !containsRole(roleNames, "platform_admin") {
		writeError(w, http.StatusForbidden, "user has no tenant assignment")
		return
	}

	token, err := console.GenerateToken(api.jwtCfg, console.JWTClaims{
		Sub:    user.ID,
		Email:  user.Email,
		Name:   user.Name,
		Roles:  roleNames,
		Tenant: scopedTenant,
	})
	if err != nil {
		api.log.Error("generate token failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user": map[string]any{
			"id":    user.ID,
			"email": user.Email,
			"name":  user.Name,
			"roles": roleNames,
		},
	})
}

func containsRole(roles []string, role string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

// ──────────────────────────────────────────────────────────────────────────────
// Analytics
// ──────────────────────────────────────────────────────────────────────────────

func (api *ConsoleAPI) handleAnalyticsOverview(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	since := parseSince(r, 24*time.Hour)
	overview, err := api.store.GetAnalyticsOverview(r.Context(), tenantScope(claims), since)
	if err != nil {
		api.log.Error("analytics overview failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get analytics")
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

func (api *ConsoleAPI) handleAnalyticsTimeseries(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	since := parseSince(r, 24*time.Hour)
	bucket := 60
	if v := r.URL.Query().Get("bucket_minutes"); v != "" {
		if b, err := strconv.Atoi(v); err == nil && b > 0 && b <= 1440 {
			bucket = b
		}
	}
	data, err := api.store.GetDecisionTimeseries(r.Context(), tenantScope(claims), since, bucket)
	if err != nil {
		api.log.Error("analytics timeseries failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get timeseries")
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// ──────────────────────────────────────────────────────────────────────────────
// Tenants
// ──────────────────────────────────────────────────────────────────────────────

func (api *ConsoleAPI) handleCreateTenant(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in struct {
		Name   string          `json:"name"`
		Config json.RawMessage `json:"config,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if in.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	t, err := api.store.CreateTenant(r.Context(), in.Name, in.Config)
	if err != nil {
		api.log.Error("create tenant failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create tenant")
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (api *ConsoleAPI) handleListTenants(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	if claims.Tenant != "" && !hasRole(claims, "platform_admin") {
		t, err := api.store.GetTenant(r.Context(), claims.Tenant)
		if err != nil || t == nil {
			writeJSON(w, http.StatusOK, []any{})
			return
		}
		writeJSON(w, http.StatusOK, []*console.Tenant{t})
		return
	}
	limit, offset := parsePagination(r)
	tenants, err := api.store.ListTenants(r.Context(), limit, offset)
	if err != nil {
		api.log.Error("list tenants failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list tenants")
		return
	}
	writeJSON(w, http.StatusOK, tenants)
}

func (api *ConsoleAPI) handleGetTenant(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "tenant_id")
	claims := claimsFromCtx(r.Context())
	scope := tenantScope(claims)
	if scope != "" && scope != id {
		writeError(w, http.StatusNotFound, "tenant not found")
		return
	}
	t, err := api.store.GetTenant(r.Context(), id)
	if err != nil {
		api.log.Error("get tenant failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get tenant")
		return
	}
	if t == nil {
		writeError(w, http.StatusNotFound, "tenant not found")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (api *ConsoleAPI) handleUpdateTenantStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "tenant_id")
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if in.Status != "active" && in.Status != "disabled" {
		writeError(w, http.StatusBadRequest, "status must be 'active' or 'disabled'")
		return
	}
	if err := api.store.UpdateTenantStatus(r.Context(), id, in.Status); err != nil {
		api.log.Error("update tenant status failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update tenant status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": in.Status})
}

// ──────────────────────────────────────────────────────────────────────────────
// Agents
// ──────────────────────────────────────────────────────────────────────────────

func (api *ConsoleAPI) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if in.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	agent, err := api.store.CreateAgent(r.Context(), tenantID, in.Name)
	if err != nil {
		api.log.Error("create agent failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create agent")
		return
	}
	writeJSON(w, http.StatusCreated, agent)
}

func (api *ConsoleAPI) handleListAgents(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	limit, offset := parsePagination(r)
	agents, err := api.store.ListAgents(r.Context(), tenantID, limit, offset)
	if err != nil {
		api.log.Error("list agents failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list agents")
		return
	}
	writeJSON(w, http.StatusOK, agents)
}

// ──────────────────────────────────────────────────────────────────────────────
// API Keys
// ──────────────────────────────────────────────────────────────────────────────

func (api *ConsoleAPI) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	result, err := api.store.CreateAPIKey(r.Context(), tenantID, in.Name)
	if err != nil {
		api.log.Error("create api key failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create api key")
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (api *ConsoleAPI) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	keys, err := api.store.ListAPIKeys(r.Context(), tenantID)
	if err != nil {
		api.log.Error("list api keys failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list api keys")
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

func (api *ConsoleAPI) handleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	keyID := chi.URLParam(r, "key_id")
	if err := api.store.RevokeAPIKey(r.Context(), keyID); err != nil {
		api.log.Error("revoke api key failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to revoke api key")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// ──────────────────────────────────────────────────────────────────────────────
// Approvals — HIGH-01: uses approvals store + approver allowlist
// ──────────────────────────────────────────────────────────────────────────────

func (api *ConsoleAPI) handleListPendingApprovals(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	tenant := tenantScope(claims)
	if tenant == "" {
		tenant = r.URL.Query().Get("tenant_id")
	}
	limit, offset := parsePagination(r)
	reqs, err := api.approvalsStore.ListPending(r.Context(), tenant, limit, offset)
	if err != nil {
		api.log.Error("list pending approvals failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list pending approvals")
		return
	}
	writeJSON(w, http.StatusOK, reqs)
}

func (api *ConsoleAPI) handleApproveRequest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := claimsFromCtx(r.Context())
	approver := claims.Email

	req, err := api.approvalsStore.GetRequest(r.Context(), id)
	if err != nil {
		api.log.Error("get approval request failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to approve")
		return
	}
	if req == nil {
		writeError(w, http.StatusNotFound, "approval request not found")
		return
	}

	// HIGH-05: Enforce tenant scoping
	scope := tenantScope(claims)
	if scope != "" && req.TenantID != scope {
		writeError(w, http.StatusNotFound, "approval request not found")
		return
	}

	// HIGH-01: Enforce approver allowlist
	if api.approverAuth != nil && !api.approverAuth.AllowEmail(req.TenantID, approver) {
		writeError(w, http.StatusForbidden, "approver is not allowed for tenant")
		return
	}

	grant, err := api.approvalsStore.GrantRequest(r.Context(), id, approvals.GrantInput{
		Approver: approver,
		MaxUses:  1,
	})
	if err != nil {
		api.log.Error("approve request failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to approve request")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "approved", "grant_id": grant.ID})
}

// MED-05: handleDenyRequest — properly handles decode errors and RowsAffected.
func (api *ConsoleAPI) handleDenyRequest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := claimsFromCtx(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req, err := api.approvalsStore.GetRequest(r.Context(), id)
	if err != nil {
		api.log.Error("get approval request failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to deny")
		return
	}
	if req == nil {
		writeError(w, http.StatusNotFound, "approval request not found")
		return
	}

	// HIGH-05: Enforce tenant scoping
	scope := tenantScope(claims)
	if scope != "" && req.TenantID != scope {
		writeError(w, http.StatusNotFound, "approval request not found")
		return
	}

	if err := api.approvalsStore.DenyRequest(r.Context(), id, approvals.DenyInput{
		Approver: claims.Email,
		Reason:   in.Reason,
	}); err != nil {
		api.log.Error("deny request failed", "error", err)
		writeError(w, http.StatusConflict, "request not found or already resolved")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "denied"})
}

// ──────────────────────────────────────────────────────────────────────────────
// Events / Audit Trail
// ──────────────────────────────────────────────────────────────────────────────

func (api *ConsoleAPI) handleListEvents(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	q := r.URL.Query()
	tenant := tenantScope(claims)
	if tenant == "" {
		tenant = q.Get("tenant_id")
	}
	limit, offset := parsePagination(r)
	events, err := api.store.ListEvents(r.Context(),
		tenant, q.Get("agent_id"), q.Get("tool"), q.Get("action"),
		q.Get("decision"), q.Get("session_id"), limit, offset)
	if err != nil {
		api.log.Error("list events failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list events")
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (api *ConsoleAPI) handleGetEventDetail(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "event_id")
	detail, err := api.store.GetEventDetail(r.Context(), eventID)
	if err != nil {
		api.log.Error("get event detail failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get event")
		return
	}
	if detail == nil {
		writeError(w, http.StatusNotFound, "event not found")
		return
	}
	claims := claimsFromCtx(r.Context())
	scope := tenantScope(claims)
	if scope != "" && detail.TenantID != scope {
		writeError(w, http.StatusNotFound, "event not found")
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (api *ConsoleAPI) handleExportEventsCSV(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	tenant := tenantScope(claims)
	if tenant == "" {
		tenant = r.URL.Query().Get("tenant_id")
	}
	// MED-06: require tenant for CSV export
	if tenant == "" {
		writeError(w, http.StatusBadRequest, "tenant_id required for CSV export")
		return
	}
	since := parseSince(r, 7*24*time.Hour)
	until := time.Now().UTC()
	if v := r.URL.Query().Get("until"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			until = t
		}
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=openclause-events-%s.csv", time.Now().Format("20060102")))
	if err := api.store.ExportEventsCSV(r.Context(), tenant, since, until, w); err != nil {
		api.log.Error("export events csv failed", "error", err)
	}
}

func (api *ConsoleAPI) handleExportBundle(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	tenant := tenantScope(claims)
	if tenant == "" {
		tenant = r.URL.Query().Get("tenant_id")
	}
	if tenant == "" {
		writeError(w, http.StatusBadRequest, "tenant_id required")
		return
	}
	since := parseSince(r, 7*24*time.Hour)
	until := time.Now().UTC()

	events, err := api.store.ListEventsInRange(r.Context(), tenant, since, until, 10000)
	if err != nil {
		api.log.Error("export bundle events failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to export bundle")
		return
	}

	bundle := map[string]any{
		"version":     "1.0",
		"tenant_id":   tenant,
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"since":       since.Format(time.RFC3339),
		"until":       until.Format(time.RFC3339),
		"events":      events,
		"event_count": len(events),
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=openclause-bundle-%s-%s.json", tenant, time.Now().Format("20060102")))
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(bundle); err != nil {
		api.log.Error("encode bundle failed", "error", err)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Sessions
// ──────────────────────────────────────────────────────────────────────────────

func (api *ConsoleAPI) handleListSessions(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	tenant := tenantScope(claims)
	if tenant == "" {
		tenant = r.URL.Query().Get("tenant_id")
	}
	limit, offset := parsePagination(r)
	sessions, err := api.store.ListSessions(r.Context(), tenant, limit, offset)
	if err != nil {
		api.log.Error("list sessions failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list sessions")
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (api *ConsoleAPI) handleSessionTimeline(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "session_id")
	claims := claimsFromCtx(r.Context())
	scope := tenantScope(claims)
	events, err := api.store.GetSessionTimeline(r.Context(), sessionID, scope)
	if err != nil {
		api.log.Error("session timeline failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get session timeline")
		return
	}
	writeJSON(w, http.StatusOK, events)
}

// ──────────────────────────────────────────────────────────────────────────────
// Policy
// ──────────────────────────────────────────────────────────────────────────────

func (api *ConsoleAPI) handleListPolicyVersions(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	tenant := tenantScope(claims)
	versions, err := api.store.ListPolicyVersions(r.Context(), tenant, 50)
	if err != nil {
		api.log.Error("list policy versions failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list policy versions")
		return
	}
	writeJSON(w, http.StatusOK, versions)
}

func (api *ConsoleAPI) handleCreatePolicyVersion(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in struct {
		TenantID   string          `json:"tenant_id"`
		Version    string          `json:"version"`
		PolicyData json.RawMessage `json:"policy_data"`
		Notes      string          `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	tenant := tenantScope(claims)
	if tenant != "" {
		in.TenantID = tenant
	}
	var tenantPtr *string
	if in.TenantID != "" {
		tenantPtr = &in.TenantID
	}
	pv, err := api.store.CreatePolicyVersion(r.Context(), tenantPtr, in.Version, "", claims.Email, in.Notes, in.PolicyData)
	if err != nil {
		api.log.Error("create policy version failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create policy version")
		return
	}
	writeJSON(w, http.StatusCreated, pv)
}

// MED-03: Uses the correct OPA package path via shared constant.
func (api *ConsoleAPI) handleSimulatePolicy(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in struct {
		TenantID  string `json:"tenant_id"`
		AgentID   string `json:"agent_id"`
		Tool      string `json:"tool"`
		Action    string `json:"action"`
		Resource  string `json:"resource"`
		RiskScore int    `json:"risk_score"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	opaURL := config.EnvOr("OPA_URL", "http://localhost:8181")
	body, err := json.Marshal(map[string]any{
		"input": map[string]any{
			"toolcall": map[string]any{
				"tenant_id":  in.TenantID,
				"agent_id":   in.AgentID,
				"tool":       in.Tool,
				"action":     in.Action,
				"resource":   in.Resource,
				"risk_score": in.RiskScore,
			},
			"environment": map[string]any{
				"timestamp": time.Now().UTC(),
			},
		},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build simulation request")
		return
	}

	resp, err := http.Post(opaURL+policy.OPAPolicyPath, "application/json", strings.NewReader(string(body)))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reach policy engine")
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var opaResp map[string]any
	if err := json.Unmarshal(respBody, &opaResp); err != nil {
		api.log.Error("decode OPA simulation response failed", "error", err)
	}

	result := map[string]any{
		"simulation": true,
		"input": map[string]any{
			"tenant_id":  in.TenantID,
			"agent_id":   in.AgentID,
			"tool":       in.Tool,
			"action":     in.Action,
			"resource":   in.Resource,
			"risk_score": in.RiskScore,
		},
		"policy_result": opaResp,
	}
	writeJSON(w, http.StatusOK, result)
}

// ──────────────────────────────────────────────────────────────────────────────
// Alerts
// ──────────────────────────────────────────────────────────────────────────────

func (api *ConsoleAPI) handleListAlertRules(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	tenant := tenantScope(claims)
	if tenant == "" {
		tenant = r.URL.Query().Get("tenant_id")
	}
	rules, err := api.store.ListAlertRules(r.Context(), tenant)
	if err != nil {
		api.log.Error("list alert rules failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list alert rules")
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

func (api *ConsoleAPI) handleCreateAlertRule(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var rule console.AlertRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	claims := claimsFromCtx(r.Context())
	tenant := tenantScope(claims)
	if tenant != "" {
		rule.TenantID = tenant
	}
	if rule.TenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id required")
		return
	}
	result, err := api.store.CreateAlertRule(r.Context(), rule)
	if err != nil {
		api.log.Error("create alert rule failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create alert rule")
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (api *ConsoleAPI) handleListAlertEvents(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	tenant := tenantScope(claims)
	if tenant == "" {
		tenant = r.URL.Query().Get("tenant_id")
	}
	limit, offset := parsePagination(r)
	events, err := api.store.ListAlertEvents(r.Context(), tenant, limit, offset)
	if err != nil {
		api.log.Error("list alert events failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list alert events")
		return
	}
	writeJSON(w, http.StatusOK, events)
}

// ──────────────────────────────────────────────────────────────────────────────
// Connectors — HIGH-06: no internal URLs exposed
// ──────────────────────────────────────────────────────────────────────────────

func (api *ConsoleAPI) handleListConnectors(w http.ResponseWriter, r *http.Request) {
	connectors, err := api.store.ListConnectors(r.Context())
	if err != nil {
		api.log.Error("list connectors failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list connectors")
		return
	}
	writeJSON(w, http.StatusOK, connectors)
}

// ──────────────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("writeJSON encode failed", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func parsePagination(r *http.Request) (limit, offset int) {
	limit = 50
	offset = 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	if limit > 100 {
		limit = 100
	}
	return
}

func parseSince(r *http.Request, defaultDuration time.Duration) time.Time {
	if v := r.URL.Query().Get("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t
		}
	}
	return time.Now().UTC().Add(-defaultDuration)
}

// CRIT-01: Configurable CORS origin allowlist.
func parseCORSOrigins(raw string) map[string]bool {
	m := make(map[string]bool)
	for _, o := range strings.Split(raw, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			m[o] = true
		}
	}
	return m
}

func corsMiddleware(allowed map[string]bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && allowed[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Vary", "Origin")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Max-Age", "86400")
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
