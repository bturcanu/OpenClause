// Console API serves the admin console for OpenClause.
// It provides JWT-based auth, RBAC, and admin endpoints for managing
// tenants, agents, API keys, policies, analytics, and more.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/mail"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bturcanu/OpenClause/pkg/approvals"
	"github.com/bturcanu/OpenClause/pkg/config"
	"github.com/bturcanu/OpenClause/pkg/console"
	"github.com/bturcanu/OpenClause/pkg/policy"
	"github.com/bturcanu/OpenClause/pkg/types"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/time/rate"
)

const maxBodyBytes = 1 << 20

// tenantDenySentinel is returned by tenantScope for non-admin users with no tenant.
// Handlers MUST check for this and return 403 before passing to the DB layer.
const tenantDenySentinel = "!!deny!!"

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
	allowlistSource := config.EnvOr("ALLOWLIST_SOURCE", "db")
	if allowlistSource == "env" || allowlistSource == "both" {
		log.Warn("ALLOWLIST_SOURCE includes env allowlists; approvals authorization uses dev bootstrap fallback")
	}
	rawTokensEnv := os.Getenv("CONSOLE_DEV_LOG_RAW_TOKENS")
	// Default to enabled to avoid regressing local testing; set to "false" in safer deployments.
	devLogRawTokens := rawTokensEnv == "" || strings.EqualFold(rawTokensEnv, "true")
	approverAuth := approvals.NewApproverAuthorizer(
		approvalsStore,
		os.Getenv("APPROVER_EMAIL_ALLOWLIST"),
		os.Getenv("APPROVER_SLACK_ALLOWLIST"),
		allowlistSource,
	)

	jwtCfg := console.JWTConfig{
		Secret:      jwtSecret,
		Issuer:      "openclause-console",
		ExpiryHours: config.EnvOrInt("CONSOLE_JWT_EXPIRY_HOURS", 24),
	}

	// CRIT-01: Parse allowed CORS origins from env.
	allowedOrigins := parseCORSOrigins(os.Getenv("CONSOLE_CORS_ORIGINS"))

	authProvider, err := authProviderFromEnv(AuthProviderDeps{
		log:    log,
		store:  store,
		jwtCfg: jwtCfg,
	})
	if err != nil {
		log.Error("auth provider init failed", "error", err)
		os.Exit(1)
	}

	api := &ConsoleAPI{
		log:                     log,
		store:                   store,
		alertsStore:            store,
		notificationConfigStore: store,
		exportStore:             store,
		jwtCfg:                  jwtCfg,
		approvalsStore:          approvalsStore,
		approverAuth:            approverAuth,
		approverAuthSource:      allowlistSource,
		authProvider:            authProvider,
		devLogRawTokens:         devLogRawTokens,
	}

	// Basic throttling for unauthenticated endpoints to reduce abuse and
	// protect downstream services during bursts.
	unauthLimiter := newIPRateLimiter(rate.Limit(1), 5) // 1 request/sec/IP, burst 5

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

	// ──────────────────────────────────────────────────────────────────────────
	// Tier 1 — First-run setup wizard (unauthenticated)
	// ──────────────────────────────────────────────────────────────────────────
	r.Get("/setup/status", api.handleSetupStatus)
	r.Post("/setup/initialize", api.handleSetupInitialize)

	r.Post("/auth/login", api.handleLogin)
	r.Group(func(r chi.Router) {
		r.Use(unauthLimiter.middleware)
		r.Post("/auth/invite/accept", api.handleInviteAccept)
		r.Post("/auth/reset/request", api.handleResetRequest)
		r.Post("/auth/reset/confirm", api.handleResetConfirm)
	})

	r.Group(func(r chi.Router) {
		r.Use(api.jwtAuthMiddleware)

		r.Get("/admin/analytics/overview", api.handleAnalyticsOverview)
		r.Get("/admin/analytics/timeseries", api.handleAnalyticsTimeseries)

		// Users + RBAC + invites (admin/tenant_admin only, validated in handler).
		r.Get("/admin/users", api.handleListUsers)
		r.Post("/admin/users", api.handleCreateUser)
		r.Post("/admin/users/{id}/roles", api.handleAssignUserRole)
		r.Delete("/admin/users/{id}/roles/{role_id}", api.handleRemoveUserRole)

		r.Post("/admin/invites", api.handleCreateInvite)
		r.Get("/admin/invites", api.handleListInvites)

		r.Post("/admin/tenants", api.requireRole("platform_admin", api.handleCreateTenant))
		r.Get("/admin/tenants", api.handleListTenants)
		r.Get("/admin/tenants/{tenant_id}", api.handleGetTenant)
		r.Post("/admin/tenants/{tenant_id}/status", api.requireRole("platform_admin", api.handleUpdateTenantStatus))

		r.Post("/admin/tenants/{tenant_id}/agents", api.requireTenantRole("tenant_admin", api.handleCreateAgent))
		r.Get("/admin/tenants/{tenant_id}/agents", api.requireTenantAccess(api.handleListAgents))

		r.Post("/admin/tenants/{tenant_id}/apikeys", api.requireTenantRole("tenant_admin", api.handleCreateAPIKey))
		r.Get("/admin/tenants/{tenant_id}/apikeys", api.requireTenantAccess(api.handleListAPIKeys))
		r.Post("/admin/tenants/{tenant_id}/apikeys/{key_id}/revoke", api.requireTenantRole("tenant_admin", api.handleRevokeAPIKey))

		// Per-tenant notification routing configuration (webhook/slack).
		r.Get("/admin/tenants/{tenant_id}/notification-config", api.requireTenantRole("tenant_admin", api.handleGetTenantNotificationConfig))
		r.Put("/admin/tenants/{tenant_id}/notification-config", api.requireTenantRole("tenant_admin", api.handleUpdateTenantNotificationConfig))

		// Approver management (DB-backed)
		r.Get("/admin/tenants/{tenant_id}/approvers", api.requireTenantRole("tenant_admin", api.handleListTenantApprovers))
		r.Post("/admin/tenants/{tenant_id}/approvers", api.requireTenantRole("tenant_admin", api.handleUpsertTenantApprover))
		r.Delete("/admin/tenants/{tenant_id}/approvers/{user_id}", api.requireTenantRole("tenant_admin", api.handleRemoveTenantApprover))

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

		// Alerts (Tier 2 item 8)
		r.Get("/admin/tenants/{tenant_id}/alerts/rules", api.requireTenantRole("tenant_admin", api.handleListTenantAlertRules))
		r.Post("/admin/tenants/{tenant_id}/alerts/rules", api.requireTenantRole("tenant_admin", api.handleCreateTenantAlertRule))
		r.Put("/admin/tenants/{tenant_id}/alerts/rules/{rule_id}", api.requireTenantRole("tenant_admin", api.handleUpdateTenantAlertRule))
		r.Delete("/admin/tenants/{tenant_id}/alerts/rules/{rule_id}", api.requireTenantRole("tenant_admin", api.handleDeleteTenantAlertRule))
		r.Get("/admin/tenants/{tenant_id}/alerts/events", api.requireTenantRole("tenant_admin", api.handleListTenantAlertEvents))

		// Legacy (non-tenant-scoped) endpoints kept for compatibility.
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
	log                     *slog.Logger
	store                   *console.Store
	alertsStore            alertsStore
	notificationConfigStore notificationConfigStore
	exportStore             exportEventsStore
	jwtCfg                  console.JWTConfig
	authProvider            AuthProvider
	approvalsStore          *approvals.Store
	approverAuth            *approvals.ApproverAuthorizer
	approverAuthSource      string

	devLogRawTokens bool
}

type exportEventsStore interface {
	ExportEventsCSV(ctx context.Context, tenantID string, since, until time.Time, w io.Writer) error
	ListEventsInRange(ctx context.Context, tenantID string, since, until time.Time, limit int) ([]console.EventListItem, error)
}

type notificationConfigStore interface {
	GetTenantNotificationConfig(ctx context.Context, tenantID string) (*console.TenantNotificationConfig, bool, error)
	SetTenantNotificationConfig(ctx context.Context, tenantID string, cfg console.TenantNotificationConfig) error
}

type alertsStore interface {
	ListAlertRules(ctx context.Context, tenantID string) ([]console.AlertRule, error)
	CreateAlertRule(ctx context.Context, rule console.AlertRule) (*console.AlertRule, error)
	UpdateAlertRule(ctx context.Context, tenantID, ruleID, name, ruleType string, config json.RawMessage, enabled bool) error
	DeleteAlertRule(ctx context.Context, tenantID, ruleID string) error
	GetAlertRule(ctx context.Context, tenantID, ruleID string) (*console.AlertRule, error)
	ListAlertEventsSince(ctx context.Context, tenantID string, since time.Time, limit int) ([]console.AlertEvent, error)
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

// HIGH-02: tenantScope returns tenantDenySentinel for non-platform_admin users
// with an empty tenant claim. Handlers MUST check for this and return 403
// before passing to the DB layer; otherwise tenant_id='!!deny!!' is queried
// and returns empty results instead of enforcing the security boundary.
func tenantScope(claims *console.JWTClaims) string {
	if hasRole(claims, "platform_admin") {
		return ""
	}
	if claims.Tenant == "" {
		return tenantDenySentinel
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

	res, err := api.authProvider.Login(r.Context(), AuthLoginInput{
		Email:    in.Email,
		Password: in.Password,
	})
	if err != nil {
		if ae, ok := err.(*AuthProviderError); ok {
			writeError(w, ae.Status, ae.Message)
			return
		}
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	writeJSON(w, http.StatusOK, res)
}

// ──────────────────────────────────────────────────────────────────────────────
// Tier 1 — User management + invites + password resets (minimum viable)
// ──────────────────────────────────────────────────────────────────────────────

func generateRandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func validateRoleName(role string) bool {
	switch role {
	case "platform_admin", "tenant_admin", "approver", "viewer":
		return true
	default:
		return false
	}
}

type userWithRoles struct {
	ID          string             `json:"id"`
	Email       string             `json:"email"`
	Name        string             `json:"name"`
	SlackUserID *string            `json:"slack_user_id,omitempty"`
	Status      string             `json:"status"`
	CreatedAt   time.Time          `json:"created_at"`
	Roles       []console.UserRole `json:"roles"`
}

func (api *ConsoleAPI) handleListUsers(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !hasRole(claims, "platform_admin") && !hasRole(claims, "tenant_admin") && !hasRole(claims, "viewer") {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	scope := tenantScope(claims)
	if scope == tenantDenySentinel {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	limit, offset := parsePagination(r)
	emailQuery := r.URL.Query().Get("email")

	var tenantID *string
	if scope != "" {
		tenantID = &scope
	}

	users, err := api.store.ListUsers(r.Context(), tenantID, emailQuery, limit, offset)
	if err != nil {
		api.log.Error("list users failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}

	out := make([]userWithRoles, 0, len(users))
	for _, u := range users {
		roles, err := api.store.GetUserRoles(r.Context(), u.ID)
		if err != nil {
			api.log.Error("get user roles failed", "error", err, "user_id", u.ID)
			writeError(w, http.StatusInternalServerError, "failed to list users")
			return
		}
		out = append(out, userWithRoles{
			ID:          u.ID,
			Email:       u.Email,
			Name:        u.Name,
			SlackUserID: u.SlackUserID,
			Status:      u.Status,
			CreatedAt:   u.CreatedAt,
			Roles:       roles,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"users": out})
}

func (api *ConsoleAPI) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !hasRole(claims, "platform_admin") && !hasRole(claims, "tenant_admin") {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in struct {
		Email       string  `json:"email"`
		Name        string  `json:"name"`
		Password    *string `json:"password,omitempty"`
		SlackUserID *string `json:"slack_user_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if in.Email == "" {
		writeError(w, http.StatusBadRequest, "email required")
		return
	}

	parsedEmail, err := mail.ParseAddress(strings.TrimSpace(in.Email))
	if err != nil || parsedEmail.Address == "" {
		writeError(w, http.StatusBadRequest, "invalid email")
		return
	}
	email := parsedEmail.Address

	var slackUserID *string
	if in.SlackUserID != nil {
		sid, err := validateSlackUserIDOrEmpty(in.SlackUserID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid slack_user_id")
			return
		}
		if sid != "" {
			slackUserID = &sid
		}
	}

	var password *string
	if in.Password != nil && strings.TrimSpace(*in.Password) != "" {
		pw := *in.Password
		password = &pw
	}

	name := strings.TrimSpace(in.Name)

	user, err := api.store.CreateUserBare(r.Context(), email, password, name, slackUserID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeError(w, http.StatusConflict, "email already exists")
			return
		}
		api.log.Error("create user failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	writeJSON(w, http.StatusCreated, user)
}

func (api *ConsoleAPI) handleAssignUserRole(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !hasRole(claims, "platform_admin") && !hasRole(claims, "tenant_admin") {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	userID := chi.URLParam(r, "id")
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in struct {
		Role     string  `json:"role"`
		TenantID *string `json:"tenant_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if in.Role == "" {
		writeError(w, http.StatusBadRequest, "role required")
		return
	}
	if !validateRoleName(in.Role) {
		writeError(w, http.StatusBadRequest, "invalid role")
		return
	}

	var tenantForRole *string
	if in.Role == "platform_admin" {
		tenantForRole = nil
	} else {
		if in.TenantID == nil || strings.TrimSpace(*in.TenantID) == "" {
			writeError(w, http.StatusBadRequest, "tenant_id required for non-platform roles")
			return
		}
		tenant := strings.TrimSpace(*in.TenantID)
		tenantForRole = &tenant

		// Tenant admin can only manage within their own tenant.
		if hasRole(claims, "tenant_admin") {
			if claims.Tenant == tenantDenySentinel || claims.Tenant == "" || claims.Tenant != tenant {
				writeError(w, http.StatusForbidden, "access denied for this tenant")
				return
			}
		}
	}

	// Tenant admin should not be able to assign platform_admin.
	if hasRole(claims, "tenant_admin") && in.Role == "platform_admin" {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	if err := api.store.AssignUserRole(r.Context(), userID, tenantForRole, in.Role); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeError(w, http.StatusConflict, "duplicate role assignment")
			return
		}
		api.log.Error("assign role failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to assign role")
		return
	}

	roles, err := api.store.GetUserRoles(r.Context(), userID)
	if err != nil {
		api.log.Error("get roles failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to assign role")
		return
	}
	for _, rr := range roles {
		if rr.Role != in.Role {
			continue
		}
		if in.Role == "platform_admin" && rr.TenantID == nil {
			writeJSON(w, http.StatusCreated, map[string]any{"status": "assigned", "role_id": rr.ID})
			return
		}
		if in.Role != "platform_admin" && rr.TenantID != nil && tenantForRole != nil && *rr.TenantID == *tenantForRole {
			writeJSON(w, http.StatusCreated, map[string]any{"status": "assigned", "role_id": rr.ID})
			return
		}
	}

	writeJSON(w, http.StatusCreated, map[string]any{"status": "assigned"})
}

func (api *ConsoleAPI) handleRemoveUserRole(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !hasRole(claims, "platform_admin") && !hasRole(claims, "tenant_admin") {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	userID := chi.URLParam(r, "id")
	roleID := chi.URLParam(r, "role_id")
	if roleID == "" {
		writeError(w, http.StatusBadRequest, "role_id required")
		return
	}

	roleRow, err := api.store.GetUserRoleByID(r.Context(), userID, roleID)
	if err != nil {
		api.log.Error("get user role failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to remove role")
		return
	}
	if roleRow == nil {
		writeError(w, http.StatusNotFound, "role assignment not found")
		return
	}

	if hasRole(claims, "tenant_admin") {
		if roleRow.Role == "platform_admin" || roleRow.TenantID == nil || claims.Tenant == "" || claims.Tenant != *roleRow.TenantID {
			writeError(w, http.StatusForbidden, "access denied for this tenant")
			return
		}
	}

	removed, err := api.store.RemoveUserRoleByID(r.Context(), userID, roleID)
	if err != nil {
		api.log.Error("remove role failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to remove role")
		return
	}
	if !removed {
		writeError(w, http.StatusNotFound, "role assignment not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (api *ConsoleAPI) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !hasRole(claims, "platform_admin") && !hasRole(claims, "tenant_admin") {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in struct {
		Email    string  `json:"email"`
		TenantID string  `json:"tenant_id"`
		Role     string  `json:"role"`
		Name     *string `json:"name,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if in.Email == "" || in.TenantID == "" || in.Role == "" {
		writeError(w, http.StatusBadRequest, "email, tenant_id, and role are required")
		return
	}

	parsedEmail, err := mail.ParseAddress(strings.TrimSpace(in.Email))
	if err != nil || parsedEmail.Address == "" {
		writeError(w, http.StatusBadRequest, "invalid email")
		return
	}
	email := parsedEmail.Address

	if !validateRoleName(in.Role) || in.Role == "platform_admin" {
		writeError(w, http.StatusBadRequest, "invalid role")
		return
	}

	// Tenant admin can only create invites within their own tenant.
	if hasRole(claims, "tenant_admin") {
		if claims.Tenant == tenantDenySentinel || claims.Tenant == "" || claims.Tenant != in.TenantID {
			writeError(w, http.StatusForbidden, "access denied for this tenant")
			return
		}
	}

	if t, err := api.store.GetTenant(r.Context(), in.TenantID); err != nil {
		api.log.Error("get tenant failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load tenant")
		return
	} else if t == nil {
		writeError(w, http.StatusNotFound, "tenant not found")
		return
	}

	token, err := generateRandomToken()
	if err != nil {
		api.log.Error("generate invite token failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create invite")
		return
	}
	expiresAt := time.Now().UTC().Add(24 * time.Hour)

	name := ""
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
	}

	if err := api.store.CreateInvite(r.Context(), token, email, in.TenantID, in.Role, name, expiresAt); err != nil {
		api.log.Error("create invite failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create invite")
		return
	}

	if api.devLogRawTokens {
		// Dev bootstrap: log invite accept guidance without affecting client contract.
		api.log.Info(
			"invite created (dev)",
			"email", email,
			"tenant_id", in.TenantID,
			"role", in.Role,
			"accept_url", "/auth/invite/accept?token="+token,
		)
	} else {
		api.log.Info("invite created", "email", email, "tenant_id", in.TenantID, "role", in.Role)
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": token, "expires_at": expiresAt})
}

func (api *ConsoleAPI) handleListInvites(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !hasRole(claims, "platform_admin") && !hasRole(claims, "tenant_admin") {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	limit, offset := parsePagination(r)
	var tenantID *string
	if hasRole(claims, "tenant_admin") {
		if claims.Tenant == tenantDenySentinel || claims.Tenant == "" {
			writeError(w, http.StatusForbidden, "insufficient permissions")
			return
		}
		tenantID = &claims.Tenant
	}

	invites, err := api.store.ListInvites(r.Context(), tenantID, limit, offset)
	if err != nil {
		api.log.Error("list invites failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list invites")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"invites": invites})
}

func (api *ConsoleAPI) handleInviteAccept(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in struct {
		Token    string `json:"token"`
		Password string `json:"password"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if in.Token == "" || in.Password == "" {
		writeError(w, http.StatusBadRequest, "token and password required")
		return
	}

	res, err := api.store.ConsumeInviteAccept(r.Context(), in.Token, in.Password, in.Name)
	if err != nil {
		api.log.Error("invite accept failed", "error", err)
		writeError(w, http.StatusBadRequest, "invalid or expired token")
		return
	}
	if res == nil || res.User == nil {
		writeError(w, http.StatusBadRequest, "invalid or expired token")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "accepted",
		"user":      res.User,
		"tenant_id": res.TenantID,
		"role":      res.Role,
	})
}

func (api *ConsoleAPI) handleResetRequest(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if in.Email == "" {
		writeError(w, http.StatusBadRequest, "email required")
		return
	}

	parsed, err := mail.ParseAddress(strings.TrimSpace(in.Email))
	if err != nil || parsed.Address == "" {
		writeError(w, http.StatusBadRequest, "invalid email")
		return
	}
	email := parsed.Address

	token, err := generateRandomToken()
	if err != nil {
		api.log.Error("generate reset token failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create reset request")
		return
	}
	expiresAt := time.Now().UTC().Add(1 * time.Hour)

	if err := api.store.CreatePasswordReset(r.Context(), token, email, expiresAt); err != nil {
		api.log.Error("create reset token failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create reset request")
		return
	}

	if api.devLogRawTokens {
		api.log.Info("password reset created (dev)", "email", email, "confirm_url", "/reset/confirm?token="+token)
	} else {
		api.log.Info("password reset created", "email", email)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (api *ConsoleAPI) handleResetConfirm(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if in.Token == "" || in.Password == "" {
		writeError(w, http.StatusBadRequest, "token and password required")
		return
	}

	if err := api.store.ConsumePasswordReset(r.Context(), in.Token, in.Password); err != nil {
		writeError(w, http.StatusBadRequest, "invalid or expired token")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

func (api *ConsoleAPI) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	var count int
	if err := api.store.Pool().QueryRow(r.Context(), `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		api.log.Error("setup status count failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to check setup state")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"initialized": count > 0})
}

func (api *ConsoleAPI) handleSetupInitialize(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	var in struct {
		OrgName         string `json:"org_name"`
		Email           string `json:"email"`
		Password        string `json:"password"`
		FirstTenantName string `json:"first_tenant_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if in.Email == "" || in.Password == "" || in.FirstTenantName == "" {
		writeError(w, http.StatusBadRequest, "email, password, and first_tenant_name are required")
		return
	}

	parsedEmail, err := mail.ParseAddress(strings.TrimSpace(in.Email))
	if err != nil || parsedEmail.Address == "" {
		writeError(w, http.StatusBadRequest, "invalid email")
		return
	}
	email := parsedEmail.Address

	// Only allow initialization when DB has zero users.
	var count int
	if err := api.store.Pool().QueryRow(r.Context(), `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		api.log.Error("setup initialize count failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to check setup state")
		return
	}
	if count > 0 {
		writeError(w, http.StatusConflict, "already initialized")
		return
	}

	tenantCfg := setupTenantConfig(in.OrgName)
	tenant, err := api.store.CreateTenant(r.Context(), strings.TrimSpace(in.FirstTenantName), tenantCfg)
	if err != nil {
		api.log.Error("create tenant failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to initialize tenant")
		return
	}

	_, err = api.store.CreateUser(r.Context(), email, in.Password, "Admin", "platform_admin", nil, nil)
	if err != nil {
		api.log.Error("create platform admin failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to initialize admin user")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"initialized": true, "tenant_id": tenant.ID})
}

func setupTenantConfig(orgName string) json.RawMessage {
	orgName = strings.TrimSpace(orgName)
	if orgName == "" {
		return json.RawMessage(`{}`)
	}
	b, err := json.Marshal(map[string]string{"org_name": orgName})
	if err != nil {
		// json.Marshal on a string->string map should not fail; still fail safe.
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(b)
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
	scope := tenantScope(claims)
	if scope == tenantDenySentinel {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}
	since := parseSince(r, 24*time.Hour)
	overview, err := api.store.GetAnalyticsOverview(r.Context(), scope, since)
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
	if !hasRole(claims, "platform_admin") && claims.Tenant == "" {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}
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
	if scope == tenantDenySentinel {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}
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
// Approvers — DB-backed (Tenant-scoped)
// ──────────────────────────────────────────────────────────────────────────────

type upsertTenantApproverInput struct {
	Email       *string `json:"email,omitempty"`
	SlackUserID *string `json:"slack_user_id,omitempty"`
	Name        *string `json:"name,omitempty"`
	Role        *string `json:"role,omitempty"`
}

func validateEmailOrEmpty(email *string) (string, error) {
	if email == nil {
		return "", nil
	}
	addr := strings.TrimSpace(*email)
	if addr == "" {
		return "", nil
	}
	parsed, err := mail.ParseAddress(addr)
	if err != nil || parsed.Address == "" {
		return "", fmt.Errorf("invalid email")
	}
	return parsed.Address, nil
}

func validateSlackUserIDOrEmpty(slackID *string) (string, error) {
	if slackID == nil {
		return "", nil
	}
	raw := strings.TrimSpace(*slackID)
	if raw == "" {
		return "", nil
	}
	// Normalize to uppercase for validation.
	raw = strings.ToUpper(raw)
	// Slack user ids are typically like `U1234567890` and are uppercase alphanumerics.
	re := regexp.MustCompile(`^U[A-Z0-9]{7,}$`)
	if !re.MatchString(raw) {
		return "", fmt.Errorf("invalid slack_user_id")
	}
	return raw, nil
}

func generateTempPassword() (string, error) {
	// Used only for initial bootstrap when a user is created via approver/invite flows.
	// Password resets/invites will set the real credential.
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (api *ConsoleAPI) handleListTenantApprovers(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id required")
		return
	}
	if t, err := api.store.GetTenant(r.Context(), tenantID); err != nil {
		api.log.Error("get tenant failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load tenant")
		return
	} else if t == nil {
		writeError(w, http.StatusNotFound, "tenant not found")
		return
	}

	approvers, err := api.store.ListTenantApprovers(r.Context(), tenantID)
	if err != nil {
		api.log.Error("list tenant approvers failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list approvers")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"allowlist_source": api.approverAuthSource,
		"approvers":        approvers,
	})
}

func (api *ConsoleAPI) handleUpsertTenantApprover(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id required")
		return
	}
	var in upsertTenantApproverInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	role := "approver"
	if in.Role != nil && strings.TrimSpace(*in.Role) != "" {
		if strings.TrimSpace(*in.Role) != "approver" {
			writeError(w, http.StatusBadRequest, "role must be 'approver'")
			return
		}
	}

	email, err := validateEmailOrEmpty(in.Email)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid email")
		return
	}
	slackUserID, err := validateSlackUserIDOrEmpty(in.SlackUserID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid slack_user_id")
		return
	}

	if email == "" && slackUserID == "" {
		writeError(w, http.StatusBadRequest, "email or slack_user_id required")
		return
	}

	if t, err := api.store.GetTenant(r.Context(), tenantID); err != nil {
		api.log.Error("get tenant failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load tenant")
		return
	} else if t == nil {
		writeError(w, http.StatusNotFound, "tenant not found")
		return
	}

	ctx := r.Context()

	var user *console.User
	var slackUserIDPtr *string
	if slackUserID != "" {
		slackUserIDPtr = &slackUserID
	}

	// Identify user:
	switch {
	case email != "":
		u, err := api.store.GetUserByEmail(ctx, email)
		if err != nil {
			api.log.Error("get user by email failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to load user")
			return
		}
		user = u

		// Create user if missing.
		if user == nil {
			password, err := generateTempPassword()
			if err != nil {
				api.log.Error("generate temp password failed", "error", err)
				writeError(w, http.StatusInternalServerError, "failed to create user")
				return
			}
			name := ""
			if in.Name != nil {
				name = strings.TrimSpace(*in.Name)
			}
			user, err = api.store.CreateUser(ctx, email, password, name, role, &tenantID, slackUserIDPtr)
			if err != nil {
				api.log.Error("create user failed", "error", err)
				writeError(w, http.StatusInternalServerError, "failed to create user")
				return
			}
			// CreateUser already assigned the role.
			writeJSON(w, http.StatusCreated, map[string]any{"status": "created", "approver": user})
			return
		}

		// Update slack_user_id if provided and current value is empty.
		if slackUserIDPtr != nil {
			if user.SlackUserID == nil {
				updated, err := api.store.SetUserSlackUserIDIfEmpty(ctx, user.ID, slackUserID)
				if err != nil {
					api.log.Error("set slack_user_id failed", "error", err)
					writeError(w, http.StatusInternalServerError, "failed to update user")
					return
				}
				if !updated {
					writeError(w, http.StatusConflict, "slack_user_id already set")
					return
				}
			} else if *user.SlackUserID != slackUserID {
				writeError(w, http.StatusConflict, "slack_user_id mismatch")
				return
			}
		}

	case slackUserID != "":
		u, err := api.store.GetUserBySlackUserID(ctx, slackUserID)
		if err != nil {
			api.log.Error("get user by slack_user_id failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to load user")
			return
		}
		user = u
		if user == nil {
			writeError(w, http.StatusNotFound, "no user found for provided slack_user_id; provide an email (to create) or link the user first")
			return
		}
	}

	// Assign the approver role for the tenant.
	if err := api.store.AssignTenantRole(ctx, user.ID, tenantID, role); err != nil {
		// Unique constraint on (user_id, tenant_id, role) => duplicate assignment.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeError(w, http.StatusConflict, "duplicate approver role assignment")
			return
		}
		api.log.Error("assign tenant role failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to assign approver role")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "assigned", "approver": user})
}

func (api *ConsoleAPI) handleRemoveTenantApprover(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	userID := chi.URLParam(r, "user_id")
	if tenantID == "" || userID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id and user_id required")
		return
	}

	if t, err := api.store.GetTenant(r.Context(), tenantID); err != nil {
		api.log.Error("get tenant failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load tenant")
		return
	} else if t == nil {
		writeError(w, http.StatusNotFound, "tenant not found")
		return
	}

	removed, err := api.store.RemoveTenantRole(r.Context(), userID, tenantID, "approver")
	if err != nil {
		api.log.Error("remove tenant role failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to remove approver")
		return
	}
	if !removed {
		writeError(w, http.StatusNotFound, "approver assignment not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// ──────────────────────────────────────────────────────────────────────────────
// Approvals — HIGH-01: uses approvals store + approver allowlist
// ──────────────────────────────────────────────────────────────────────────────

func (api *ConsoleAPI) handleListPendingApprovals(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	tenant := tenantScope(claims)
	if tenant == tenantDenySentinel {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}
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
	if api.approverAuth != nil && !api.approverAuth.AllowEmail(r.Context(), req.TenantID, approver) {
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
	if scope == tenantDenySentinel {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}
	if scope != "" && req.TenantID != scope {
		writeError(w, http.StatusNotFound, "approval request not found")
		return
	}

	// HIGH-01: Enforce approver allowlist (same logic as approvals service).
	if api.approverAuth != nil && !api.approverAuth.AllowEmail(r.Context(), req.TenantID, claims.Email) {
		writeError(w, http.StatusForbidden, "approver is not allowed for tenant")
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
	if tenant == tenantDenySentinel {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}
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
	if tenant == tenantDenySentinel {
		types.ErrForbidden("insufficient permissions").WriteJSON(w)
		return
	}
	if tenant == "" {
		tenant = r.URL.Query().Get("tenant_id")
	}
	// MED-06: require tenant for CSV export
	if tenant == "" {
		types.ErrBadRequest("tenant_id required for CSV export").WriteJSON(w)
		return
	}
	since := parseSince(r, 7*24*time.Hour)
	until := time.Now().UTC()
	if v := r.URL.Query().Get("until"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			until = t
		}
	}

	// Atomic export: buffer first, then write response headers/body only on success.
	// This prevents returning HTTP 200 with partial CSV output if ExportEventsCSV fails.
	const maxExportCSVBytes = 50 << 20 // 50 MiB
	var buf bytes.Buffer
	if err := api.exportStore.ExportEventsCSV(r.Context(), tenant, since, until, &buf); err != nil {
		api.log.Error("export events csv failed", "error", err)
		types.ErrInternal("failed to export events csv").WriteJSON(w)
		return
	}
	if buf.Len() > maxExportCSVBytes {
		types.ErrInternal("export exceeds maximum size").WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=openclause-events-%s.csv", time.Now().Format("20060102")))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(buf.Bytes()); err != nil {
		api.log.Error("export events csv write failed", "error", err)
	}
}

func (api *ConsoleAPI) handleExportBundle(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	tenant := tenantScope(claims)
	if tenant == tenantDenySentinel {
		types.ErrForbidden("insufficient permissions").WriteJSON(w)
		return
	}
	if tenant == "" {
		tenant = r.URL.Query().Get("tenant_id")
	}
	if tenant == "" {
		types.ErrBadRequest("tenant_id required").WriteJSON(w)
		return
	}
	since := parseSince(r, 7*24*time.Hour)
	until := time.Now().UTC()

	events, err := api.exportStore.ListEventsInRange(r.Context(), tenant, since, until, 10000)
	if err != nil {
		api.log.Error("export bundle events failed", "error", err)
		types.ErrInternal("failed to export bundle").WriteJSON(w)
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

	// Atomic export: encode fully before writing to response.
	const maxExportBundleBytes = 20 << 20 // 20 MiB
	var buf bytes.Buffer
	if err := encodeBundleJSON(&buf, bundle); err != nil {
		api.log.Error("encode bundle failed", "error", err)
		types.ErrInternal("failed to encode bundle").WriteJSON(w)
		return
	}
	if buf.Len() > maxExportBundleBytes {
		types.ErrInternal("export exceeds maximum size").WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=openclause-bundle-%s-%s.json", tenant, time.Now().Format("20060102")))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(buf.Bytes()); err != nil {
		api.log.Error("export bundle write failed", "error", err)
	}
}

func encodeBundleJSON(w io.Writer, bundle map[string]any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(bundle)
}

// ──────────────────────────────────────────────────────────────────────────────
// Sessions
// ──────────────────────────────────────────────────────────────────────────────

func (api *ConsoleAPI) handleListSessions(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	tenant := tenantScope(claims)
	if tenant == tenantDenySentinel {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}
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
	if tenant == tenantDenySentinel {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}
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
	if tenant == tenantDenySentinel {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}
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
	if tenant == tenantDenySentinel {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}
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
	var apiErr *types.APIError
	switch status {
	case http.StatusBadRequest:
		apiErr = types.ErrBadRequest(msg)
	case http.StatusUnauthorized:
		apiErr = types.ErrUnauthorized(msg)
	case http.StatusForbidden:
		apiErr = types.ErrForbidden(msg)
	case http.StatusNotFound:
		apiErr = types.ErrNotFound(msg)
	case http.StatusConflict:
		apiErr = types.ErrConflict(msg)
	case http.StatusUnprocessableEntity:
		// Keep the same public shape as gateway/generic validation errors.
		apiErr = &types.APIError{Code: "VALIDATION_ERROR", Message: msg, HTTPCode: status}
	default:
		apiErr = &types.APIError{
			Code:      "HTTP_ERROR",
			Message:   msg,
			Retryable: status >= 500,
			HTTPCode:  status,
		}
	}
	apiErr.WriteJSON(w)
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
		// RFC3339Nano accepts timestamps with optional fractional seconds.
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
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
