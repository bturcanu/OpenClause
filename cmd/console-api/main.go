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
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bturcanu/OpenClause/pkg/config"
	"github.com/bturcanu/OpenClause/pkg/console"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxBodyBytes = 1 << 20

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	pool, err := pgxpool.New(ctx, buildPostgresDSN())
	if err != nil {
		log.Error("postgres connect failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	store := console.NewStore(pool)
	jwtCfg := console.JWTConfig{
		Secret:      config.EnvOr("CONSOLE_JWT_SECRET", "change-me-in-production-openclause-jwt-secret"),
		Issuer:      "openclause-console",
		ExpiryHours: config.EnvOrInt("CONSOLE_JWT_EXPIRY_HOURS", 24),
	}

	api := &ConsoleAPI{
		log:    log,
		store:  store,
		jwtCfg: jwtCfg,
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(corsMiddleware)

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

		// Analytics
		r.Get("/admin/analytics/overview", api.handleAnalyticsOverview)
		r.Get("/admin/analytics/timeseries", api.handleAnalyticsTimeseries)

		// Tenants
		r.Post("/admin/tenants", api.requireRole("platform_admin", api.handleCreateTenant))
		r.Get("/admin/tenants", api.handleListTenants)
		r.Get("/admin/tenants/{tenant_id}", api.handleGetTenant)
		r.Post("/admin/tenants/{tenant_id}/status", api.requireRole("platform_admin", api.handleUpdateTenantStatus))

		// Agents
		r.Post("/admin/tenants/{tenant_id}/agents", api.requireTenantRole("tenant_admin", api.handleCreateAgent))
		r.Get("/admin/tenants/{tenant_id}/agents", api.requireTenantAccess(api.handleListAgents))

		// API Keys
		r.Post("/admin/tenants/{tenant_id}/apikeys", api.requireTenantRole("tenant_admin", api.handleCreateAPIKey))
		r.Get("/admin/tenants/{tenant_id}/apikeys", api.requireTenantAccess(api.handleListAPIKeys))
		r.Post("/admin/tenants/{tenant_id}/apikeys/{key_id}/revoke", api.requireTenantRole("tenant_admin", api.handleRevokeAPIKey))

		// Approvals
		r.Get("/admin/approvals/pending", api.handleListPendingApprovals)
		r.Post("/admin/approvals/{id}/approve", api.requireRole("approver", api.handleApproveRequest))
		r.Post("/admin/approvals/{id}/deny", api.requireRole("approver", api.handleDenyRequest))

		// Events / Audit Trail
		r.Get("/admin/events", api.handleListEvents)
		r.Get("/admin/events/{event_id}", api.handleGetEventDetail)
		r.Get("/admin/events/export/csv", api.handleExportEventsCSV)

		// Sessions
		r.Get("/admin/sessions", api.handleListSessions)
		r.Get("/admin/sessions/{session_id}/timeline", api.handleSessionTimeline)

		// Policy
		r.Get("/admin/policy/versions", api.handleListPolicyVersions)
		r.Post("/admin/policy/versions", api.requireRole("tenant_admin", api.handleCreatePolicyVersion))
		r.Post("/admin/policy/simulate", api.handleSimulatePolicy)

		// Alerts
		r.Get("/admin/alerts/rules", api.handleListAlertRules)
		r.Post("/admin/alerts/rules", api.requireRole("tenant_admin", api.handleCreateAlertRule))
		r.Get("/admin/alerts/events", api.handleListAlertEvents)

		// Connectors
		r.Get("/v1/connectors", api.handleListConnectors)

		// Reports
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
	log    *slog.Logger
	store  *console.Store
	jwtCfg console.JWTConfig
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

func tenantScope(claims *console.JWTClaims) string {
	if hasRole(claims, "platform_admin") {
		return ""
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
// Approvals
// ──────────────────────────────────────────────────────────────────────────────

func (api *ConsoleAPI) handleListPendingApprovals(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	tenant := tenantScope(claims)
	if tenant == "" {
		tenant = r.URL.Query().Get("tenant_id")
	}
	limit, offset := parsePagination(r)
	ctx := r.Context()

	rows, err := api.store.Pool().Query(ctx, `
		SELECT id, event_id, tenant_id, agent_id, tool, action, resource,
		       risk_score, reason, deny_reason, status, created_at, expires_at
		FROM approval_requests
		WHERE ($1 = '' OR tenant_id = $1) AND status = 'pending' AND expires_at > NOW()
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`, tenant, limit, offset)
	if err != nil {
		api.log.Error("list pending approvals failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list pending approvals")
		return
	}
	defer rows.Close()

	type approval struct {
		ID         string    `json:"id"`
		EventID    string    `json:"event_id"`
		TenantID   string    `json:"tenant_id"`
		AgentID    string    `json:"agent_id"`
		Tool       string    `json:"tool"`
		Action     string    `json:"action"`
		Resource   string    `json:"resource"`
		RiskScore  int       `json:"risk_score"`
		Reason     string    `json:"reason"`
		DenyReason string    `json:"deny_reason,omitempty"`
		Status     string    `json:"status"`
		CreatedAt  time.Time `json:"created_at"`
		ExpiresAt  time.Time `json:"expires_at"`
	}
	results := make([]approval, 0)
	for rows.Next() {
		var a approval
		if err := rows.Scan(
			&a.ID, &a.EventID, &a.TenantID, &a.AgentID,
			&a.Tool, &a.Action, &a.Resource,
			&a.RiskScore, &a.Reason, &a.DenyReason, &a.Status,
			&a.CreatedAt, &a.ExpiresAt,
		); err != nil {
			api.log.Error("scan pending approval failed", "error", err)
			continue
		}
		results = append(results, a)
	}
	writeJSON(w, http.StatusOK, results)
}

func (api *ConsoleAPI) handleApproveRequest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := claimsFromCtx(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	ctx := r.Context()
	approver := claims.Email

	_, err := api.store.Pool().Exec(ctx, `
		UPDATE approval_requests SET status = 'approved', updated_at = NOW()
		WHERE id = $1 AND status = 'pending' AND expires_at > NOW()`, id)
	if err != nil {
		api.log.Error("approve request failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to approve")
		return
	}

	// Get request details for creating the grant
	var tenantID, agentID, tool, action, resource string
	row := api.store.Pool().QueryRow(ctx, `
		SELECT tenant_id, agent_id, tool, action, resource FROM approval_requests WHERE id = $1`, id)
	if err := row.Scan(&tenantID, &agentID, &tool, &action, &resource); err != nil {
		api.log.Error("fetch approval details failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create grant")
		return
	}

	grantID := fmt.Sprintf("grant-%s", id[:8])
	now := time.Now().UTC()
	_, err = api.store.Pool().Exec(ctx, `
		INSERT INTO approval_grants (
			id, request_id, tenant_id, approver,
			scope_tool, scope_action, scope_resource_pattern, scope_tenant_id, scope_agent_id,
			max_uses, uses_left, expires_at, granted_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT DO NOTHING`,
		grantID, id, tenantID, approver,
		tool, action, resource, tenantID, agentID,
		1, 1, now.Add(time.Hour), now)
	if err != nil {
		api.log.Error("create grant failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create grant")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "approved", "grant_id": grantID})
}

func (api *ConsoleAPI) handleDenyRequest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := claimsFromCtx(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)

	_, err := api.store.Pool().Exec(r.Context(), `
		UPDATE approval_requests SET status = 'denied', deny_reason = $2, denied_by = $3, updated_at = NOW()
		WHERE id = $1 AND status = 'pending'`, id, in.Reason, claims.Email)
	if err != nil {
		api.log.Error("deny request failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to deny")
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

	events, err := api.store.ListEvents(r.Context(), tenant, "", "", "", "", "", 1000, 0)
	if err != nil {
		api.log.Error("export bundle events failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to export bundle")
		return
	}

	bundle := map[string]any{
		"version":    "1.0",
		"tenant_id":  tenant,
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"since":      since.Format(time.RFC3339),
		"until":      until.Format(time.RFC3339),
		"events":     events,
		"event_count": len(events),
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=openclause-bundle-%s-%s.json", tenant, time.Now().Format("20060102")))
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(bundle)
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
	events, err := api.store.GetSessionTimeline(r.Context(), sessionID)
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
	body, _ := json.Marshal(map[string]any{
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

	resp, err := http.Post(opaURL+"/v1/data/openclause/main", "application/json", strings.NewReader(string(body)))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reach policy engine")
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var opaResp map[string]any
	_ = json.Unmarshal(respBody, &opaResp)

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
// Connectors
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
	_ = json.NewEncoder(w).Encode(v)
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
	if limit > 200 {
		limit = 200
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

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
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

func buildPostgresDSN() string {
	sslmode := config.EnvOr("POSTGRES_SSLMODE", "disable")
	u := &url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(config.EnvOr("POSTGRES_USER", "openclause"), config.EnvOr("POSTGRES_PASSWORD", "changeme")),
		Host:     net.JoinHostPort(config.EnvOr("POSTGRES_HOST", "localhost"), config.EnvOr("POSTGRES_PORT", "5432")),
		Path:     config.EnvOr("POSTGRES_DB", "openclause"),
		RawQuery: "sslmode=" + url.QueryEscape(sslmode),
	}
	return u.String()
}
