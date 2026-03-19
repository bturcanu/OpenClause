package console

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/bturcanu/OpenClause/pkg/types"
)

// ──────────────────────────────────────────────────────────────────────────────
// Types
// ──────────────────────────────────────────────────────────────────────────────

type Tenant struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Status    string          `json:"status"`
	Config    json.RawMessage `json:"config"`
	CreatedAt time.Time       `json:"created_at"`
}

// TenantNotificationConfig is the DB-persisted per-tenant routing config used
// to build approval notification outbox entries.
//
// Stored inside tenants.config under `notification_config`.
type TenantNotificationConfig struct {
	ApproverGroup string               `json:"approver_group,omitempty"`
	Notify        []types.PolicyNotify `json:"notify,omitempty"`
}

type Agent struct {
	ID        string          `json:"id"`
	TenantID  string          `json:"tenant_id"`
	Name      string          `json:"name"`
	Status    string          `json:"status"`
	Labels    json.RawMessage `json:"labels,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

type APIKey struct {
	ID         string     `json:"id"`
	TenantID   string     `json:"tenant_id"`
	Name       string     `json:"name"`
	KeyPrefix  string     `json:"key_prefix"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type APIKeyCreateResult struct {
	APIKey
	RawKey string `json:"raw_key"`
}

type User struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	Name        string    `json:"name"`
	SlackUserID *string   `json:"slack_user_id,omitempty"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type UserRole struct {
	ID       string  `json:"id"`
	UserID   string  `json:"user_id"`
	TenantID *string `json:"tenant_id,omitempty"`
	Role     string  `json:"role"`
}

type AnalyticsOverview struct {
	TotalEvents      int64 `json:"total_events"`
	AllowCount       int64 `json:"allow_count"`
	DenyCount        int64 `json:"deny_count"`
	ApproveCount     int64 `json:"approve_count"`
	PendingApprovals int64 `json:"pending_approvals"`
	ActiveTenants    int64 `json:"active_tenants"`
	ActiveAgents     int64 `json:"active_agents"`
}

type DecisionTotals struct {
	TotalEvents  int64 `json:"total_events"`
	AllowCount   int64 `json:"allow_count"`
	DenyCount    int64 `json:"deny_count"`
	ApproveCount int64 `json:"approve_count"`
}

type DecisionTrendBucket struct {
	Bucket       time.Time `json:"bucket"`
	Total        int64     `json:"total"`
	AllowCount   int64     `json:"allow_count"`
	DenyCount    int64     `json:"deny_count"`
	ApproveCount int64     `json:"approve_count"`
}

type RiskHeatmapRow struct {
	RiskScore    int   `json:"risk_score"`
	AllowCount   int64 `json:"allow_count"`
	DenyCount    int64 `json:"deny_count"`
	ApproveCount int64 `json:"approve_count"`
	Total        int64 `json:"total"`
}

type AgentBreakdownRow struct {
	AgentID      string `json:"agent_id"`
	AllowCount   int64  `json:"allow_count"`
	DenyCount    int64  `json:"deny_count"`
	ApproveCount int64  `json:"approve_count"`
	Total        int64  `json:"total"`
}

type OnboardingChecklist struct {
	HasAPIKey    bool `json:"has_api_key"`
	HasApprover  bool `json:"has_approver"`
	HasToolcall  bool `json:"has_toolcall"`
	HasApproval  bool `json:"has_approval"`
	HasExecution bool `json:"has_execution"`
}

type TenantAnalyticsSummary struct {
	RangeStart           time.Time             `json:"range_start"`
	RangeEnd             time.Time             `json:"range_end"`
	Totals               DecisionTotals        `json:"totals"`
	Trend                []DecisionTrendBucket `json:"trend"`
	RiskHeatmap          []RiskHeatmapRow      `json:"risk_heatmap"`
	PerAgent             []AgentBreakdownRow   `json:"per_agent"`
	OnboardingChecklist  OnboardingChecklist   `json:"onboarding_checklist"`
}

type EventListItem struct {
	EventID    string    `json:"event_id"`
	TenantID   string    `json:"tenant_id"`
	AgentID    string    `json:"agent_id"`
	Tool       string    `json:"tool"`
	Action     string    `json:"action"`
	Resource   string    `json:"resource"`
	RiskScore  int       `json:"risk_score"`
	Decision   string    `json:"decision"`
	SessionID  string    `json:"session_id"`
	TraceID    string    `json:"trace_id"`
	ReceivedAt time.Time `json:"received_at"`
}

type EventDetail struct {
	EventListItem
	PayloadJSON  json.RawMessage `json:"payload_json"`
	PolicyResult json.RawMessage `json:"policy_result,omitempty"`
	Hash         string          `json:"hash"`
	PrevHash     string          `json:"prev_hash"`
	Result       *EventResult    `json:"result,omitempty"`
}

type EventResult struct {
	Status     string          `json:"status"`
	OutputJSON json.RawMessage `json:"output_json,omitempty"`
	ErrorMsg   string          `json:"error_msg,omitempty"`
	DurationMS int64           `json:"duration_ms"`
}

type Session struct {
	ID         string     `json:"id"`
	TenantID   string     `json:"tenant_id"`
	AgentID    string     `json:"agent_id"`
	StartedAt  time.Time  `json:"started_at"`
	EndedAt    *time.Time `json:"ended_at,omitempty"`
	EventCount int64      `json:"event_count,omitempty"`
}

type PolicyVersion struct {
	ID         int64           `json:"id"`
	TenantID   *string         `json:"tenant_id,omitempty"`
	BundleHash string          `json:"bundle_hash"`
	Version    string          `json:"version"`
	PolicyData json.RawMessage `json:"policy_data,omitempty"`
	DeployedBy string          `json:"deployed_by,omitempty"`
	DeployedAt time.Time       `json:"deployed_at"`
	Notes      string          `json:"notes,omitempty"`
}

type AlertRule struct {
	ID        string          `json:"id"`
	TenantID  string          `json:"tenant_id"`
	Name      string          `json:"name"`
	RuleType  string          `json:"rule_type"`
	Config    json.RawMessage `json:"config"`
	Enabled   bool            `json:"enabled"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type AlertEvent struct {
	ID            string          `json:"id"`
	RuleID        string          `json:"rule_id"`
	TenantID      string          `json:"tenant_id"`
	Severity      string          `json:"severity"`
	Message       string          `json:"message"`
	ContextJSON   json.RawMessage `json:"context_json,omitempty"`
	Status        string          `json:"status"`
	DeliveredAt   *time.Time      `json:"delivered_at,omitempty"`
	AttemptCount  int             `json:"attempt_count"`
	NextAttemptAt time.Time       `json:"next_attempt_at,omitempty"`
	LastError     string          `json:"last_error,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

// ──────────────────────────────────────────────────────────────────────────────
// Store
// ──────────────────────────────────────────────────────────────────────────────

type Store struct {
	pool            *pgxpool.Pool
	tokenHMACSecret []byte
}

func NewStore(pool *pgxpool.Pool) *Store {
	// Keyed hash secret for invite + password reset tokens.
	// Fallbacks keep local dev functional without requiring extra env vars.
	secret := os.Getenv("INVITE_RESET_TOKEN_HMAC_SECRET")
	if secret == "" {
		secret = os.Getenv("CONSOLE_JWT_SECRET")
	}
	return &Store{
		pool:            pool,
		tokenHMACSecret: []byte(secret),
	}
}

func (s *Store) hashInviteResetToken(rawToken string) string {
	mac := hmac.New(sha256.New, s.tokenHMACSecret)
	_, _ = mac.Write([]byte(rawToken))
	return hex.EncodeToString(mac.Sum(nil))
}

// Pool exposes the underlying connection pool for ad-hoc queries
// (e.g., approval grant creation in the console-api).
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

const maxLimit = 100

func clampLimit(limit int) int {
	if limit <= 0 || limit > maxLimit {
		return maxLimit
	}
	return limit
}

func clampOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

// ──────────────────────────────────────────────────────────────────────────────
// Tenants
// ──────────────────────────────────────────────────────────────────────────────

func (s *Store) CreateTenant(ctx context.Context, name string, config json.RawMessage) (*Tenant, error) {
	t := &Tenant{
		ID:     uuid.NewString(),
		Name:   name,
		Status: "active",
		Config: config,
	}
	if len(t.Config) == 0 {
		t.Config = json.RawMessage(`{}`)
	}

	err := s.pool.QueryRow(ctx, `
		INSERT INTO tenants (id, name, status, config)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at`,
		t.ID, t.Name, t.Status, t.Config,
	).Scan(&t.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("console.CreateTenant: %w", err)
	}
	return t, nil
}

func (s *Store) ListTenants(ctx context.Context, limit, offset int) ([]Tenant, error) {
	limit = clampLimit(limit)
	offset = clampOffset(offset)

	rows, err := s.pool.Query(ctx, `
		SELECT id, name, status, config, created_at
		FROM tenants
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("console.ListTenants: %w", err)
	}
	defer rows.Close()

	out := make([]Tenant, 0)
	for rows.Next() {
		var t Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.Status, &t.Config, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("console.ListTenants scan: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.ListTenants iteration: %w", err)
	}
	return out, nil
}

func (s *Store) GetTenant(ctx context.Context, id string) (*Tenant, error) {
	var t Tenant
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, status, config, created_at
		FROM tenants WHERE id = $1`, id,
	).Scan(&t.ID, &t.Name, &t.Status, &t.Config, &t.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("console.GetTenant: %w", err)
	}
	return &t, nil
}

func (s *Store) GetTenantNotificationConfig(ctx context.Context, tenantID string) (*TenantNotificationConfig, bool, error) {
	t, err := s.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, false, err
	}
	if t == nil {
		return nil, false, nil
	}

	// tenants.config is a JSON object that may or may not contain the
	// notification_config payload.
	type tenantConfigWrapper struct {
		NotificationConfig *TenantNotificationConfig `json:"notification_config,omitempty"`
	}
	var w tenantConfigWrapper
	if len(t.Config) == 0 || string(t.Config) == "{}" {
		return nil, false, nil
	}
	if err := json.Unmarshal(t.Config, &w); err != nil {
		return nil, false, fmt.Errorf("console.GetTenantNotificationConfig unmarshal: %w", err)
	}
	if w.NotificationConfig == nil {
		return nil, false, nil
	}
	return w.NotificationConfig, true, nil
}

func (s *Store) SetTenantNotificationConfig(ctx context.Context, tenantID string, cfg TenantNotificationConfig) error {
	b, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("console.SetTenantNotificationConfig marshal: %w", err)
	}

	res, err := s.pool.Exec(ctx, `
		UPDATE tenants
		SET config = jsonb_set(
			COALESCE(config, '{}'::jsonb),
			'{notification_config}',
			$1::jsonb,
			true
		)
		WHERE id = $2`, b, tenantID)
	if err != nil {
		return fmt.Errorf("console.SetTenantNotificationConfig update: %w", err)
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("console.SetTenantNotificationConfig: tenant %s not found", tenantID)
	}
	return nil
}

func (s *Store) UpdateTenantStatus(ctx context.Context, id, status string) error {
	res, err := s.pool.Exec(ctx, `
		UPDATE tenants SET status = $2 WHERE id = $1`, id, status)
	if err != nil {
		return fmt.Errorf("console.UpdateTenantStatus: %w", err)
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("console.UpdateTenantStatus: tenant %s not found", id)
	}
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Agents
// ──────────────────────────────────────────────────────────────────────────────

func (s *Store) CreateAgent(ctx context.Context, tenantID, name string) (*Agent, error) {
	a := &Agent{
		ID:       uuid.NewString(),
		TenantID: tenantID,
		Name:     name,
		Status:   "active",
		Labels:   json.RawMessage(`{}`),
	}

	err := s.pool.QueryRow(ctx, `
		INSERT INTO agents (id, tenant_id, name, status, labels)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at`,
		a.ID, a.TenantID, a.Name, a.Status, a.Labels,
	).Scan(&a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("console.CreateAgent: %w", err)
	}
	return a, nil
}

func (s *Store) ListAgents(ctx context.Context, tenantID string, limit, offset int) ([]Agent, error) {
	limit = clampLimit(limit)
	offset = clampOffset(offset)

	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, name, status, labels, created_at
		FROM agents
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("console.ListAgents: %w", err)
	}
	defer rows.Close()

	out := make([]Agent, 0)
	for rows.Next() {
		var a Agent
		if err := rows.Scan(&a.ID, &a.TenantID, &a.Name, &a.Status, &a.Labels, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("console.ListAgents scan: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.ListAgents iteration: %w", err)
	}
	return out, nil
}

func (s *Store) UpdateAgentStatus(ctx context.Context, id, status string) error {
	res, err := s.pool.Exec(ctx, `
		UPDATE agents SET status = $2 WHERE id = $1`, id, status)
	if err != nil {
		return fmt.Errorf("console.UpdateAgentStatus: %w", err)
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("console.UpdateAgentStatus: agent %s not found", id)
	}
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// API Keys
// ──────────────────────────────────────────────────────────────────────────────

func generateAPIKey() (raw string, prefix string, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", "", fmt.Errorf("generate random bytes: %w", err)
	}
	raw = "sk-oc-" + hex.EncodeToString(b)
	prefix = raw[:8]
	h := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(h[:])
	return raw, prefix, hash, nil
}

func hashAPIKey(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func (s *Store) CreateAPIKey(ctx context.Context, tenantID, name string) (*APIKeyCreateResult, error) {
	raw, prefix, keyHash, err := generateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("console.CreateAPIKey: %w", err)
	}

	k := APIKey{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		Name:      name,
		KeyPrefix: prefix,
		Status:    "active",
	}

	err = s.pool.QueryRow(ctx, `
		INSERT INTO api_keys (id, tenant_id, name, key_prefix, key_hash, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at`,
		k.ID, k.TenantID, k.Name, k.KeyPrefix, keyHash, k.Status,
	).Scan(&k.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("console.CreateAPIKey: %w", err)
	}

	return &APIKeyCreateResult{APIKey: k, RawKey: raw}, nil
}

func (s *Store) ListAPIKeys(ctx context.Context, tenantID string) ([]APIKey, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, name, key_prefix, status, created_at, revoked_at, last_used_at
		FROM api_keys
		WHERE tenant_id = $1
		ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("console.ListAPIKeys: %w", err)
	}
	defer rows.Close()

	out := make([]APIKey, 0)
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.TenantID, &k.Name, &k.KeyPrefix, &k.Status,
			&k.CreatedAt, &k.RevokedAt, &k.LastUsedAt); err != nil {
			return nil, fmt.Errorf("console.ListAPIKeys scan: %w", err)
		}
		out = append(out, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.ListAPIKeys iteration: %w", err)
	}
	return out, nil
}

func (s *Store) RevokeAPIKey(ctx context.Context, keyID string) error {
	res, err := s.pool.Exec(ctx, `
		UPDATE api_keys SET status = 'revoked', revoked_at = NOW()
		WHERE id = $1 AND status = 'active'`, keyID)
	if err != nil {
		return fmt.Errorf("console.RevokeAPIKey: %w", err)
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("console.RevokeAPIKey: key %s not found or already revoked", keyID)
	}
	return nil
}

func (s *Store) LookupAPIKey(ctx context.Context, rawKey string) (tenantID string, keyID string, err error) {
	if len(rawKey) < 8 {
		return "", "", fmt.Errorf("console.LookupAPIKey: invalid key format")
	}
	prefix := rawKey[:8]
	computedHash := hashAPIKey(rawKey)

	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, key_hash
		FROM api_keys
		WHERE key_prefix = $1 AND status = 'active'`, prefix)
	if err != nil {
		return "", "", fmt.Errorf("console.LookupAPIKey: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, tid, storedHash string
		if err := rows.Scan(&id, &tid, &storedHash); err != nil {
			return "", "", fmt.Errorf("console.LookupAPIKey scan: %w", err)
		}
		if subtle.ConstantTimeCompare([]byte(computedHash), []byte(storedHash)) == 1 {
			rows.Close()
			_, _ = s.pool.Exec(ctx, `
				UPDATE api_keys SET last_used_at = NOW() WHERE id = $1`, id)
			return tid, id, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", "", fmt.Errorf("console.LookupAPIKey iteration: %w", err)
	}
	return "", "", fmt.Errorf("console.LookupAPIKey: key not found")
}

func (s *Store) RotateAPIKeys(ctx context.Context, tenantID string) (*APIKeyCreateResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("console.RotateAPIKeys begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	_, err = tx.Exec(ctx, `
		UPDATE api_keys SET status = 'revoked', revoked_at = NOW()
		WHERE tenant_id = $1 AND status = 'active'`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("console.RotateAPIKeys revoke: %w", err)
	}

	raw, prefix, keyHash, err := generateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("console.RotateAPIKeys: %w", err)
	}

	k := APIKey{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		Name:      "rotated-key",
		KeyPrefix: prefix,
		Status:    "active",
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO api_keys (id, tenant_id, name, key_prefix, key_hash, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at`,
		k.ID, k.TenantID, k.Name, k.KeyPrefix, keyHash, k.Status,
	).Scan(&k.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("console.RotateAPIKeys insert: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("console.RotateAPIKeys commit: %w", err)
	}

	return &APIKeyCreateResult{APIKey: k, RawKey: raw}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Users
// ──────────────────────────────────────────────────────────────────────────────

func (s *Store) CreateUser(ctx context.Context, email, password, name, role string, tenantID *string, slackUserID *string) (*User, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return nil, fmt.Errorf("console.CreateUser hash password: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("console.CreateUser begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	u := &User{
		ID:          uuid.NewString(),
		Email:       email,
		Name:        name,
		SlackUserID: slackUserID,
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO users (id, email, password_hash, name, slack_user_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING status, created_at`,
		u.ID, u.Email, string(hashed), u.Name, u.SlackUserID,
	).Scan(&u.Status, &u.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("console.CreateUser insert user: %w", err)
	}

	roleID := uuid.NewString()
	_, err = tx.Exec(ctx, `
		INSERT INTO user_roles (id, user_id, tenant_id, role)
		VALUES ($1, $2, $3, $4)`,
		roleID, u.ID, tenantID, role)
	if err != nil {
		return nil, fmt.Errorf("console.CreateUser insert role: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("console.CreateUser commit: %w", err)
	}
	return u, nil
}

func (s *Store) AuthenticateUser(ctx context.Context, email, password string) (*User, []UserRole, error) {
	var u User
	var passwordHash *string
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, name, status, created_at
		FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &passwordHash, &u.Name, &u.Status, &u.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil, fmt.Errorf("console.AuthenticateUser: invalid credentials")
	}
	if err != nil {
		return nil, nil, fmt.Errorf("console.AuthenticateUser: %w", err)
	}

	if u.Status != "active" {
		return nil, nil, fmt.Errorf("console.AuthenticateUser: user account disabled")
	}

	if passwordHash == nil || *passwordHash == "" {
		return nil, nil, fmt.Errorf("console.AuthenticateUser: invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(*passwordHash), []byte(password)); err != nil {
		return nil, nil, fmt.Errorf("console.AuthenticateUser: invalid credentials")
	}

	roles, err := s.GetUserRoles(ctx, u.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("console.AuthenticateUser: %w", err)
	}
	return &u, roles, nil
}

func (s *Store) GetUserRoles(ctx context.Context, userID string) ([]UserRole, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, tenant_id, role
		FROM user_roles WHERE user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("console.GetUserRoles: %w", err)
	}
	defer rows.Close()

	out := make([]UserRole, 0)
	for rows.Next() {
		var r UserRole
		if err := rows.Scan(&r.ID, &r.UserID, &r.TenantID, &r.Role); err != nil {
			return nil, fmt.Errorf("console.GetUserRoles scan: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.GetUserRoles iteration: %w", err)
	}
	return out, nil
}

// GetUserRoleByID fetches a specific user_roles row by its assignment id.
func (s *Store) GetUserRoleByID(ctx context.Context, userID, roleAssignmentID string) (*UserRole, error) {
	var r UserRole
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, tenant_id, role
		FROM user_roles
		WHERE id = $1 AND user_id = $2`, roleAssignmentID, userID,
	).Scan(&r.ID, &r.UserID, &r.TenantID, &r.Role)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("console.GetUserRoleByID: %w", err)
	}
	return &r, nil
}

func (s *Store) GetUser(ctx context.Context, id string) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, name, slack_user_id, status, created_at
		FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Email, &u.Name, &u.SlackUserID, &u.Status, &u.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("console.GetUser: %w", err)
	}
	return &u, nil
}

// GetUserByEmail returns the user for an email (case-insensitive).
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, name, slack_user_id, status, created_at
		FROM users
		WHERE lower(email) = lower($1)`, email,
	).Scan(&u.ID, &u.Email, &u.Name, &u.SlackUserID, &u.Status, &u.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("console.GetUserByEmail: %w", err)
	}
	return &u, nil
}

// GetUserBySlackUserID returns the user linked to a Slack user id.
func (s *Store) GetUserBySlackUserID(ctx context.Context, slackUserID string) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, name, slack_user_id, status, created_at
		FROM users
		WHERE slack_user_id = $1`, slackUserID,
	).Scan(&u.ID, &u.Email, &u.Name, &u.SlackUserID, &u.Status, &u.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("console.GetUserBySlackUserID: %w", err)
	}
	return &u, nil
}

// SetUserSlackUserIDIfEmpty links a user to a Slack user id only if slack_user_id is currently NULL.
func (s *Store) SetUserSlackUserIDIfEmpty(ctx context.Context, userID string, slackUserID string) (bool, error) {
	res, err := s.pool.Exec(ctx, `
		UPDATE users
		SET slack_user_id = $2
		WHERE id = $1 AND slack_user_id IS NULL`, userID, slackUserID,
	)
	if err != nil {
		return false, fmt.Errorf("console.SetUserSlackUserIDIfEmpty: %w", err)
	}
	return res.RowsAffected() > 0, nil
}

// ListTenantApprovers lists all users with role='approver' scoped to a tenant.
func (s *Store) ListTenantApprovers(ctx context.Context, tenantID string) ([]User, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.email, u.name, u.slack_user_id, u.status, u.created_at
		FROM user_roles ur
		JOIN users u ON u.id = ur.user_id
		WHERE ur.tenant_id = $1 AND ur.role = 'approver'
		ORDER BY u.created_at DESC`, tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("console.ListTenantApprovers: %w", err)
	}
	defer rows.Close()

	out := make([]User, 0)
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.SlackUserID, &u.Status, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("console.ListTenantApprovers scan: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.ListTenantApprovers iteration: %w", err)
	}
	return out, nil
}

// AssignTenantRole inserts a role assignment into user_roles.
func (s *Store) AssignTenantRole(ctx context.Context, userID, tenantID, role string) error {
	roleID := uuid.NewString()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO user_roles (id, user_id, tenant_id, role)
		VALUES ($1, $2, $3, $4)`, roleID, userID, tenantID, role)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// Caller can treat unique violations as conflict/duplicate.
			return err
		}
		return fmt.Errorf("console.AssignTenantRole: %w", err)
	}
	return nil
}

// RemoveTenantRole removes a role assignment from user_roles.
func (s *Store) RemoveTenantRole(ctx context.Context, userID, tenantID, role string) (bool, error) {
	res, err := s.pool.Exec(ctx, `
		DELETE FROM user_roles
		WHERE user_id = $1 AND tenant_id = $2 AND role = $3`, userID, tenantID, role)
	if err != nil {
		return false, fmt.Errorf("console.RemoveTenantRole: %w", err)
	}
	return res.RowsAffected() > 0, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Users: management primitives for user CRUD + invites/resets
// ──────────────────────────────────────────────────────────────────────────────

// CreateUserBare creates a user record without assigning any roles.
// password may be nil to create a user without credentials (invite/reset flow).
func (s *Store) CreateUserBare(ctx context.Context, email string, password *string, name string, slackUserID *string) (*User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("console.CreateUserBare begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	u := &User{
		ID:          uuid.NewString(),
		Email:       email,
		Name:        name,
		SlackUserID: slackUserID,
	}

	var passwordHash any
	if password != nil && strings.TrimSpace(*password) != "" {
		hashed, err := bcrypt.GenerateFromPassword([]byte(*password), 10)
		if err != nil {
			return nil, fmt.Errorf("console.CreateUserBare hash password: %w", err)
		}
		passwordHash = string(hashed)
	} else {
		passwordHash = nil
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO users (id, email, password_hash, name, slack_user_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING status, created_at`,
		u.ID, u.Email, passwordHash, u.Name, u.SlackUserID,
	).Scan(&u.Status, &u.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("console.CreateUserBare insert user: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("console.CreateUserBare commit: %w", err)
	}
	return u, nil
}

// SetUserPassword updates the user's password (hashed) and ensures status is active.
func (s *Store) SetUserPassword(ctx context.Context, userID string, password string) error {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return fmt.Errorf("console.SetUserPassword hash password: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE users
		SET password_hash = $2, status = 'active'
		WHERE id = $1`, userID, string(hashed),
	)
	if err != nil {
		return fmt.Errorf("console.SetUserPassword update: %w", err)
	}
	return nil
}

// AssignUserRole assigns a user role for a tenant.
// For role='platform_admin', tenantID must be nil.
func (s *Store) AssignUserRole(ctx context.Context, userID string, tenantID *string, role string) error {
	roleID := uuid.NewString()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO user_roles (id, user_id, tenant_id, role)
		VALUES ($1, $2, $3, $4)`, roleID, userID, tenantID, role)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return err
		}
		return fmt.Errorf("console.AssignUserRole: %w", err)
	}
	return nil
}

// RemoveUserRoleByID removes a user role assignment by its role assignment id.
func (s *Store) RemoveUserRoleByID(ctx context.Context, userID, roleAssignmentID string) (bool, error) {
	res, err := s.pool.Exec(ctx, `
		DELETE FROM user_roles
		WHERE user_id = $1 AND id = $2`, userID, roleAssignmentID,
	)
	if err != nil {
		return false, fmt.Errorf("console.RemoveUserRoleByID: %w", err)
	}
	return res.RowsAffected() > 0, nil
}

func (s *Store) ListUsers(ctx context.Context, tenantID *string, emailQuery string, limit, offset int) ([]User, error) {
	limit = clampLimit(limit)
	offset = clampOffset(offset)

	emailQuery = strings.TrimSpace(emailQuery)
	var rows pgx.Rows
	var err error
	if tenantID != nil && *tenantID != "" {
		if emailQuery == "" {
			rows, err = s.pool.Query(ctx, `
				SELECT DISTINCT u.id, u.email, u.name, u.slack_user_id, u.status, u.created_at
				FROM users u
				JOIN user_roles ur ON ur.user_id = u.id
				WHERE ur.tenant_id = $1
				ORDER BY u.created_at DESC
				LIMIT $2 OFFSET $3`, *tenantID, limit, offset)
		} else {
			rows, err = s.pool.Query(ctx, `
				SELECT DISTINCT u.id, u.email, u.name, u.slack_user_id, u.status, u.created_at
				FROM users u
				JOIN user_roles ur ON ur.user_id = u.id
				WHERE ur.tenant_id = $1 AND lower(u.email) LIKE lower($2) || '%'
				ORDER BY u.created_at DESC
				LIMIT $3 OFFSET $4`, *tenantID, emailQuery, limit, offset)
		}
	} else {
		if emailQuery == "" {
			rows, err = s.pool.Query(ctx, `
				SELECT u.id, u.email, u.name, u.slack_user_id, u.status, u.created_at
				FROM users u
				ORDER BY u.created_at DESC
				LIMIT $1 OFFSET $2`, limit, offset)
		} else {
			rows, err = s.pool.Query(ctx, `
				SELECT u.id, u.email, u.name, u.slack_user_id, u.status, u.created_at
				FROM users u
				WHERE lower(u.email) LIKE lower($1) || '%'
				ORDER BY u.created_at DESC
				LIMIT $2 OFFSET $3`, emailQuery, limit, offset)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("console.ListUsers: %w", err)
	}
	defer rows.Close()

	out := make([]User, 0)
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.SlackUserID, &u.Status, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("console.ListUsers scan: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.ListUsers iteration: %w", err)
	}
	return out, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Invites & password resets (minimum viable)
// ──────────────────────────────────────────────────────────────────────────────

type Invite struct {
	Token     string    `json:"token"`
	Email     string    `json:"email"`
	TenantID  string    `json:"tenant_id"`
	Role      string    `json:"role"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// InviteAcceptResult is the structured response payload for the invite acceptance flow.
// It includes the created/updated user plus the assigned tenant-scoped role metadata.
type InviteAcceptResult struct {
	User     *User  `json:"user"`
	TenantID string `json:"tenant_id"`
	Role     string `json:"role"`
}

func (s *Store) CreateInvite(ctx context.Context, token, email, tenantID, role, name string, expiresAt time.Time) error {
	tokenHash := s.hashInviteResetToken(token)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO invites (token, email, tenant_id, role, name, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)`, tokenHash, email, tenantID, role, name, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("console.CreateInvite: %w", err)
	}
	return nil
}

func (s *Store) GetInvite(ctx context.Context, token string) (*Invite, error) {
	var inv Invite
	tokenHash := s.hashInviteResetToken(token)
	err := s.pool.QueryRow(ctx, `
		SELECT token, email, tenant_id, role, name, created_at, expires_at
		FROM invites
		WHERE token = $1 AND expires_at > NOW()`, tokenHash,
	).Scan(&inv.Token, &inv.Email, &inv.TenantID, &inv.Role, &inv.Name, &inv.CreatedAt, &inv.ExpiresAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("console.GetInvite: %w", err)
	}
	return &inv, nil
}

func (s *Store) ConsumeInviteAccept(ctx context.Context, token, password, name string) (*InviteAcceptResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("console.ConsumeInviteAccept begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	tokenHash := s.hashInviteResetToken(token)

	var inv Invite
	err = tx.QueryRow(ctx, `
		SELECT token, email, tenant_id, role, name, expires_at
		FROM invites
		WHERE token = $1 AND expires_at > NOW()`, tokenHash,
	).Scan(&inv.Token, &inv.Email, &inv.TenantID, &inv.Role, &inv.Name, &inv.ExpiresAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("console.ConsumeInviteAccept select invite: %w", err)
	}

	// Find existing user.
	var u User
	err = tx.QueryRow(ctx, `
		SELECT id, email, name, slack_user_id, status, created_at
		FROM users
		WHERE lower(email) = lower($1)`, inv.Email,
	).Scan(&u.ID, &u.Email, &u.Name, &u.SlackUserID, &u.Status, &u.CreatedAt)
	if err != nil && err != pgx.ErrNoRows {
		return nil, fmt.Errorf("console.ConsumeInviteAccept select user: %w", err)
	}
	userSelectErr := err

	passToSet := password
	if passToSet == "" {
		return nil, fmt.Errorf("console.ConsumeInviteAccept password required")
	}

	hashed, bcryptErr := bcrypt.GenerateFromPassword([]byte(passToSet), 10)
	if bcryptErr != nil {
		return nil, fmt.Errorf("console.ConsumeInviteAccept hash password: %w", bcryptErr)
	}

	acceptName := strings.TrimSpace(name)
	if acceptName == "" {
		acceptName = strings.TrimSpace(inv.Name)
	}

	userID := ""
	if userSelectErr == pgx.ErrNoRows {
		userID = uuid.NewString()
		// INSERT doesn't return rows; use Exec to ensure the user row exists
		// before we create user_roles in the same transaction.
		_, err = tx.Exec(ctx, `
			INSERT INTO users (id, email, password_hash, name, slack_user_id, status)
			VALUES ($1, $2, $3, $4, NULL, 'active')`,
			userID, inv.Email, string(hashed), acceptName,
		)
		if err != nil {
			return nil, fmt.Errorf("console.ConsumeInviteAccept insert user: %w", err)
		}
		u = User{
			ID:          userID,
			Email:       inv.Email,
			Name:        acceptName,
			SlackUserID: nil,
			Status:      "active",
			CreatedAt:   time.Now().UTC(),
		}
	} else {
		userID = u.ID
		_, err = tx.Exec(ctx, `
			UPDATE users
			SET password_hash = $2, name = $3, status = 'active'
			WHERE id = $1`, userID, string(hashed), acceptName,
		)
		if err != nil {
			return nil, fmt.Errorf("console.ConsumeInviteAccept update user: %w", err)
		}
		u.Name = acceptName
		u.Status = "active"
	}

	tenantID := inv.TenantID
	// Assign role; tolerate unique violations (role already assigned).
	roleID := uuid.NewString()
	// IMPORTANT: In Postgres, even if we "tolerate" a unique violation,
	// the failed statement aborts the entire transaction. Use ON CONFLICT
	// to avoid transaction aborts so subsequent DELETE/COMMIT succeed.
	_, err = tx.Exec(ctx, `
		INSERT INTO user_roles (id, user_id, tenant_id, role)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, tenant_id, role) DO NOTHING`, roleID, userID, tenantID, inv.Role)
	if err != nil {
		return nil, fmt.Errorf("console.ConsumeInviteAccept assign role: %w", err)
	}

	_, err = tx.Exec(ctx, `DELETE FROM invites WHERE token = $1`, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("console.ConsumeInviteAccept delete invite: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("console.ConsumeInviteAccept commit: %w", err)
	}

	return &InviteAcceptResult{
		User:     &u,
		TenantID: inv.TenantID,
		Role:     inv.Role,
	}, nil
}

func (s *Store) ListInvites(ctx context.Context, tenantID *string, limit, offset int) ([]Invite, error) {
	limit = clampLimit(limit)
	offset = clampOffset(offset)

	var rows pgx.Rows
	var err error
	if tenantID != nil && *tenantID != "" {
		rows, err = s.pool.Query(ctx, `
			SELECT token, email, tenant_id, role, name, created_at, expires_at
			FROM invites
			WHERE tenant_id = $1 AND expires_at > NOW()
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3`, *tenantID, limit, offset)
	} else {
		rows, err = s.pool.Query(ctx, `
			SELECT token, email, tenant_id, role, name, created_at, expires_at
			FROM invites
			WHERE expires_at > NOW()
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2`, limit, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("console.ListInvites: %w", err)
	}
	defer rows.Close()

	out := make([]Invite, 0)
	for rows.Next() {
		var inv Invite
		if err := rows.Scan(&inv.Token, &inv.Email, &inv.TenantID, &inv.Role, &inv.Name, &inv.CreatedAt, &inv.ExpiresAt); err != nil {
			return nil, fmt.Errorf("console.ListInvites scan: %w", err)
		}
		out = append(out, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.ListInvites iteration: %w", err)
	}
	return out, nil
}

func (s *Store) CreatePasswordReset(ctx context.Context, token, email string, expiresAt time.Time) error {
	tokenHash := s.hashInviteResetToken(token)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO password_resets (token, email, expires_at)
		VALUES ($1, $2, $3)`, tokenHash, email, expiresAt)
	if err != nil {
		return fmt.Errorf("console.CreatePasswordReset: %w", err)
	}
	return nil
}

func (s *Store) ConsumePasswordReset(ctx context.Context, token, password string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("console.ConsumePasswordReset begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	tokenHash := s.hashInviteResetToken(token)

	var email string
	err = tx.QueryRow(ctx, `
		SELECT email
		FROM password_resets
		WHERE token = $1 AND expires_at > NOW()`, tokenHash,
	).Scan(&email)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("invalid or expired password reset token")
	}
	if err != nil {
		return fmt.Errorf("console.ConsumePasswordReset select reset token: %w", err)
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return fmt.Errorf("console.ConsumePasswordReset hash password: %w", err)
	}

	res, err := tx.Exec(ctx, `
		UPDATE users
		SET password_hash = $2, status = 'active'
		WHERE lower(email) = lower($1)`, email, string(hashed))
	if err != nil {
		return fmt.Errorf("console.ConsumePasswordReset update user: %w", err)
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("password reset token email does not map to a user")
	}

	_, err = tx.Exec(ctx, `DELETE FROM password_resets WHERE token = $1`, tokenHash)
	if err != nil {
		return fmt.Errorf("console.ConsumePasswordReset delete token: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("console.ConsumePasswordReset commit: %w", err)
	}
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Analytics
// ──────────────────────────────────────────────────────────────────────────────

func (s *Store) GetAnalyticsOverview(ctx context.Context, tenantID string, since time.Time) (*AnalyticsOverview, error) {
	var o AnalyticsOverview

	// Event counts — optionally scoped to a tenant.
	var eventQuery string
	var eventArgs []any
	if tenantID != "" {
		eventQuery = `
			SELECT
				COUNT(*),
				COUNT(*) FILTER (WHERE decision = 'allow'),
				COUNT(*) FILTER (WHERE decision = 'deny'),
				COUNT(*) FILTER (WHERE decision = 'approve')
			FROM tool_events
			WHERE tenant_id = $1 AND received_at >= $2`
		eventArgs = []any{tenantID, since}
	} else {
		eventQuery = `
			SELECT
				COUNT(*),
				COUNT(*) FILTER (WHERE decision = 'allow'),
				COUNT(*) FILTER (WHERE decision = 'deny'),
				COUNT(*) FILTER (WHERE decision = 'approve')
			FROM tool_events
			WHERE received_at >= $1`
		eventArgs = []any{since}
	}

	err := s.pool.QueryRow(ctx, eventQuery, eventArgs...).Scan(
		&o.TotalEvents, &o.AllowCount, &o.DenyCount, &o.ApproveCount)
	if err != nil {
		return nil, fmt.Errorf("console.GetAnalyticsOverview events: %w", err)
	}

	// Pending approvals.
	var pendingQuery string
	var pendingArgs []any
	if tenantID != "" {
		pendingQuery = `
			SELECT COUNT(*) FROM approval_requests
			WHERE tenant_id = $1 AND status = 'pending' AND expires_at > NOW()`
		pendingArgs = []any{tenantID}
	} else {
		pendingQuery = `
			SELECT COUNT(*) FROM approval_requests
			WHERE status = 'pending' AND expires_at > NOW()`
	}

	err = s.pool.QueryRow(ctx, pendingQuery, pendingArgs...).Scan(&o.PendingApprovals)
	if err != nil {
		return nil, fmt.Errorf("console.GetAnalyticsOverview pending: %w", err)
	}

	// Active tenants + agents.
	err = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM tenants WHERE status = 'active'`).Scan(&o.ActiveTenants)
	if err != nil {
		return nil, fmt.Errorf("console.GetAnalyticsOverview tenants: %w", err)
	}

	if tenantID != "" {
		err = s.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM agents WHERE tenant_id = $1 AND status = 'active'`,
			tenantID).Scan(&o.ActiveAgents)
	} else {
		err = s.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM agents WHERE status = 'active'`).Scan(&o.ActiveAgents)
	}
	if err != nil {
		return nil, fmt.Errorf("console.GetAnalyticsOverview agents: %w", err)
	}

	return &o, nil
}

func (s *Store) GetDecisionTimeseries(ctx context.Context, tenantID string, since time.Time, bucketMinutes int) ([]map[string]any, error) {
	if bucketMinutes <= 0 {
		bucketMinutes = 60
	}
	bucketSec := int64(bucketMinutes) * 60

	var query string
	var args []any
	if tenantID != "" {
		query = `
			SELECT
				to_timestamp(floor(extract(epoch FROM received_at) / $3) * $3) AS bucket,
				COUNT(*) AS total,
				COUNT(*) FILTER (WHERE decision = 'allow') AS allow_count,
				COUNT(*) FILTER (WHERE decision = 'deny') AS deny_count,
				COUNT(*) FILTER (WHERE decision = 'approve') AS approve_count
			FROM tool_events
			WHERE tenant_id = $1 AND received_at >= $2
			GROUP BY bucket
			ORDER BY bucket ASC`
		args = []any{tenantID, since, bucketSec}
	} else {
		query = `
			SELECT
				to_timestamp(floor(extract(epoch FROM received_at) / $2) * $2) AS bucket,
				COUNT(*) AS total,
				COUNT(*) FILTER (WHERE decision = 'allow') AS allow_count,
				COUNT(*) FILTER (WHERE decision = 'deny') AS deny_count,
				COUNT(*) FILTER (WHERE decision = 'approve') AS approve_count
			FROM tool_events
			WHERE received_at >= $1
			GROUP BY bucket
			ORDER BY bucket ASC`
		args = []any{since, bucketSec}
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("console.GetDecisionTimeseries: %w", err)
	}
	defer rows.Close()

	out := make([]map[string]any, 0)
	for rows.Next() {
		var bucket time.Time
		var total, allow, deny, approve int64
		if err := rows.Scan(&bucket, &total, &allow, &deny, &approve); err != nil {
			return nil, fmt.Errorf("console.GetDecisionTimeseries scan: %w", err)
		}
		out = append(out, map[string]any{
			"bucket":        bucket,
			"total":         total,
			"allow_count":   allow,
			"deny_count":    deny,
			"approve_count": approve,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.GetDecisionTimeseries iteration: %w", err)
	}
	return out, nil
}

func (s *Store) GetTenantAnalyticsSummary(ctx context.Context, tenantID string, since time.Time, bucketMinutes int, topAgents int) (*TenantAnalyticsSummary, error) {
	if bucketMinutes <= 0 {
		bucketMinutes = 60
	}
	if topAgents <= 0 {
		topAgents = 5
	}
	if topAgents > 50 {
		topAgents = 50
	}

	now := time.Now().UTC()
	bucketSec := int64(bucketMinutes) * 60

	// Trend: allow/deny/approve counts in time buckets.
	rows, err := s.pool.Query(ctx, `
		SELECT
			to_timestamp(floor(extract(epoch FROM received_at) / $3) * $3) AS bucket,
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE decision = 'allow') AS allow_count,
			COUNT(*) FILTER (WHERE decision = 'deny') AS deny_count,
			COUNT(*) FILTER (WHERE decision = 'approve') AS approve_count
		FROM tool_events
		WHERE tenant_id = $1 AND received_at >= $2
		GROUP BY bucket
		ORDER BY bucket ASC`, tenantID, since, bucketSec,
	)
	if err != nil {
		return nil, fmt.Errorf("console.GetTenantAnalyticsSummary trend: %w", err)
	}
	defer rows.Close()

	trend := make([]DecisionTrendBucket, 0)
	var totals DecisionTotals
	for rows.Next() {
		var bucket time.Time
		var total, allow, deny, approve int64
		if err := rows.Scan(&bucket, &total, &allow, &deny, &approve); err != nil {
			return nil, fmt.Errorf("console.GetTenantAnalyticsSummary trend scan: %w", err)
		}
		trend = append(trend, DecisionTrendBucket{
			Bucket:       bucket,
			Total:        total,
			AllowCount:   allow,
			DenyCount:    deny,
			ApproveCount: approve,
		})
		totals.TotalEvents += total
		totals.AllowCount += allow
		totals.DenyCount += deny
		totals.ApproveCount += approve
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.GetTenantAnalyticsSummary trend iteration: %w", err)
	}

	// Risk heatmap: per-risk-score decision counts (0..10).
	riskHeatmap := make([]RiskHeatmapRow, 0, 11)
	for risk := 0; risk <= 10; risk++ {
		riskHeatmap = append(riskHeatmap, RiskHeatmapRow{RiskScore: risk})
	}
	riskRows, err := s.pool.Query(ctx, `
		SELECT
			risk_score,
			COUNT(*) FILTER (WHERE decision = 'allow') AS allow_count,
			COUNT(*) FILTER (WHERE decision = 'deny') AS deny_count,
			COUNT(*) FILTER (WHERE decision = 'approve') AS approve_count,
			COUNT(*) AS total
		FROM tool_events
		WHERE tenant_id = $1 AND received_at >= $2
		GROUP BY risk_score
		ORDER BY risk_score ASC`, tenantID, since,
	)
	if err != nil {
		return nil, fmt.Errorf("console.GetTenantAnalyticsSummary risk heatmap: %w", err)
	}
	defer riskRows.Close()
	for riskRows.Next() {
		var risk int
		var allow, deny, approve, total int64
		if err := riskRows.Scan(&risk, &allow, &deny, &approve, &total); err != nil {
			return nil, fmt.Errorf("console.GetTenantAnalyticsSummary risk heatmap scan: %w", err)
		}
		if risk >= 0 && risk <= 10 {
			riskHeatmap[risk] = RiskHeatmapRow{
				RiskScore:    risk,
				AllowCount:   allow,
				DenyCount:    deny,
				ApproveCount: approve,
				Total:        total,
			}
		}
	}
	if err := riskRows.Err(); err != nil {
		return nil, fmt.Errorf("console.GetTenantAnalyticsSummary risk heatmap iteration: %w", err)
	}

	// Per-agent breakdown: top agents by total events.
	perAgent := make([]AgentBreakdownRow, 0)
	agentRows, err := s.pool.Query(ctx, `
		SELECT
			agent_id,
			COUNT(*) FILTER (WHERE decision = 'allow') AS allow_count,
			COUNT(*) FILTER (WHERE decision = 'deny') AS deny_count,
			COUNT(*) FILTER (WHERE decision = 'approve') AS approve_count,
			COUNT(*) AS total
		FROM tool_events
		WHERE tenant_id = $1 AND received_at >= $2
		GROUP BY agent_id
		ORDER BY total DESC
		LIMIT $3`, tenantID, since, topAgents,
	)
	if err != nil {
		return nil, fmt.Errorf("console.GetTenantAnalyticsSummary per-agent: %w", err)
	}
	defer agentRows.Close()
	for agentRows.Next() {
		var row AgentBreakdownRow
		if err := agentRows.Scan(&row.AgentID, &row.AllowCount, &row.DenyCount, &row.ApproveCount, &row.Total); err != nil {
			return nil, fmt.Errorf("console.GetTenantAnalyticsSummary per-agent scan: %w", err)
		}
		perAgent = append(perAgent, row)
	}
	if err := agentRows.Err(); err != nil {
		return nil, fmt.Errorf("console.GetTenantAnalyticsSummary per-agent iteration: %w", err)
	}

	// Onboarding checklist: activity presence for common tenant setup steps.
	var onboarding OnboardingChecklist
	err = s.pool.QueryRow(ctx, `
		SELECT
			EXISTS(SELECT 1 FROM api_keys WHERE tenant_id = $1 AND status = 'active') AS has_api_key,
			EXISTS(SELECT 1 FROM user_roles WHERE tenant_id = $1 AND role = 'approver') AS has_approver,
			EXISTS(SELECT 1 FROM tool_events WHERE tenant_id = $1 AND received_at >= $2) AS has_toolcall,
			EXISTS(SELECT 1 FROM approval_requests WHERE tenant_id = $1 AND created_at >= $2) AS has_approval,
			EXISTS(
				SELECT 1
				FROM tool_executions te
				JOIN tool_events e ON e.event_id = te.execution_event_id
				WHERE e.tenant_id = $1 AND e.received_at >= $2
			) AS has_execution`, tenantID, since,
	).Scan(&onboarding.HasAPIKey, &onboarding.HasApprover, &onboarding.HasToolcall, &onboarding.HasApproval, &onboarding.HasExecution)
	if err != nil {
		return nil, fmt.Errorf("console.GetTenantAnalyticsSummary onboarding: %w", err)
	}

	return &TenantAnalyticsSummary{
		RangeStart:          since,
		RangeEnd:            now,
		Totals:               totals,
		Trend:                trend,
		RiskHeatmap:          riskHeatmap,
		PerAgent:             perAgent,
		OnboardingChecklist: onboarding,
	}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Events / Audit Trail
// ──────────────────────────────────────────────────────────────────────────────

func (s *Store) ListEvents(ctx context.Context, tenantID, agentID, tool, action, decision, sessionID string, limit, offset int) ([]EventListItem, error) {
	limit = clampLimit(limit)
	offset = clampOffset(offset)

	var clauses []string
	var args []any
	argIdx := 1

	addFilter := func(col, val string) {
		if val != "" {
			clauses = append(clauses, fmt.Sprintf("%s = $%d", col, argIdx))
			args = append(args, val)
			argIdx++
		}
	}

	addFilter("tenant_id", tenantID)
	addFilter("agent_id", agentID)
	addFilter("tool", tool)
	addFilter("action", action)
	addFilter("decision", decision)
	addFilter("session_id", sessionID)

	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}

	query := fmt.Sprintf(`
		SELECT event_id, tenant_id, agent_id, tool, action,
		       COALESCE(payload_json->>'resource', ''), risk_score,
		       decision, session_id, trace_id, received_at
		FROM tool_events
		%s
		ORDER BY received_at DESC
		LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("console.ListEvents: %w", err)
	}
	defer rows.Close()

	out := make([]EventListItem, 0)
	for rows.Next() {
		var e EventListItem
		if err := rows.Scan(&e.EventID, &e.TenantID, &e.AgentID, &e.Tool, &e.Action,
			&e.Resource, &e.RiskScore, &e.Decision, &e.SessionID, &e.TraceID,
			&e.ReceivedAt); err != nil {
			return nil, fmt.Errorf("console.ListEvents scan: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.ListEvents iteration: %w", err)
	}
	return out, nil
}

// ListEventsInRange returns events for a tenant within a time range (for exports/bundles).
func (s *Store) ListEventsInRange(ctx context.Context, tenantID string, since, until time.Time, limit int) ([]EventListItem, error) {
	if limit <= 0 || limit > 10000 {
		limit = 10000
	}
	rows, err := s.pool.Query(ctx, `
		SELECT event_id, tenant_id, agent_id, tool, action,
		       COALESCE(payload_json->>'resource', ''), risk_score,
		       decision, session_id, trace_id, received_at
		FROM tool_events
		WHERE tenant_id = $1 AND received_at >= $2 AND received_at <= $3
		ORDER BY received_at ASC
		LIMIT $4`, tenantID, since, until, limit)
	if err != nil {
		return nil, fmt.Errorf("console.ListEventsInRange: %w", err)
	}
	defer rows.Close()

	out := make([]EventListItem, 0)
	for rows.Next() {
		var e EventListItem
		if err := rows.Scan(&e.EventID, &e.TenantID, &e.AgentID, &e.Tool, &e.Action,
			&e.Resource, &e.RiskScore, &e.Decision, &e.SessionID, &e.TraceID,
			&e.ReceivedAt); err != nil {
			return nil, fmt.Errorf("console.ListEventsInRange scan: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.ListEventsInRange iteration: %w", err)
	}
	return out, nil
}

func (s *Store) GetEventDetail(ctx context.Context, eventID string) (*EventDetail, error) {
	var d EventDetail
	var policyResult []byte
	var resultStatus *string
	var resultOutput []byte
	var resultError *string
	var resultDuration *int64

	err := s.pool.QueryRow(ctx, `
		SELECT e.event_id, e.tenant_id, e.agent_id, e.tool, e.action,
		       COALESCE(e.payload_json->>'resource', ''), e.risk_score,
		       e.decision, e.session_id, e.trace_id, e.received_at,
		       e.payload_json, e.policy_result, e.hash, e.prev_hash,
		       r.status, r.output_json, r.error_msg, r.duration_ms
		FROM tool_events e
		LEFT JOIN tool_results r ON r.event_id = e.event_id
		WHERE e.event_id = $1`, eventID,
	).Scan(
		&d.EventID, &d.TenantID, &d.AgentID, &d.Tool, &d.Action,
		&d.Resource, &d.RiskScore,
		&d.Decision, &d.SessionID, &d.TraceID, &d.ReceivedAt,
		&d.PayloadJSON, &policyResult, &d.Hash, &d.PrevHash,
		&resultStatus, &resultOutput, &resultError, &resultDuration,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("console.GetEventDetail: %w", err)
	}

	if len(policyResult) > 0 {
		d.PolicyResult = policyResult
	}

	if resultStatus != nil {
		d.Result = &EventResult{
			Status: *resultStatus,
		}
		if len(resultOutput) > 0 {
			d.Result.OutputJSON = resultOutput
		}
		if resultError != nil {
			d.Result.ErrorMsg = *resultError
		}
		if resultDuration != nil {
			d.Result.DurationMS = *resultDuration
		}
	}

	return &d, nil
}

func (s *Store) ExportEventsCSV(ctx context.Context, tenantID string, since, until time.Time, w io.Writer) error {
	rows, err := s.pool.Query(ctx, `
		SELECT event_id, tenant_id, agent_id, tool, action,
		       COALESCE(payload_json->>'resource', ''), risk_score,
		       decision, session_id, trace_id, received_at
		FROM tool_events
		WHERE tenant_id = $1 AND received_at >= $2 AND received_at <= $3
		ORDER BY received_at ASC`, tenantID, since, until)
	if err != nil {
		return fmt.Errorf("console.ExportEventsCSV: %w", err)
	}
	defer rows.Close()

	cw := csv.NewWriter(w)
	defer cw.Flush()

	header := []string{"event_id", "tenant_id", "agent_id", "tool", "action",
		"resource", "risk_score", "decision", "session_id", "trace_id", "received_at"}
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("console.ExportEventsCSV write header: %w", err)
	}

	for rows.Next() {
		var e EventListItem
		if err := rows.Scan(&e.EventID, &e.TenantID, &e.AgentID, &e.Tool, &e.Action,
			&e.Resource, &e.RiskScore, &e.Decision, &e.SessionID, &e.TraceID,
			&e.ReceivedAt); err != nil {
			return fmt.Errorf("console.ExportEventsCSV scan: %w", err)
		}
		record := []string{
			e.EventID, e.TenantID, e.AgentID, e.Tool, e.Action,
			e.Resource, strconv.Itoa(e.RiskScore), e.Decision,
			e.SessionID, e.TraceID, e.ReceivedAt.Format(time.RFC3339),
		}
		if err := cw.Write(record); err != nil {
			return fmt.Errorf("console.ExportEventsCSV write row: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("console.ExportEventsCSV iteration: %w", err)
	}
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Sessions
// ──────────────────────────────────────────────────────────────────────────────

func (s *Store) ListSessions(ctx context.Context, tenantID string, limit, offset int) ([]Session, error) {
	limit = clampLimit(limit)
	offset = clampOffset(offset)

	var query string
	var args []any
	if tenantID != "" {
		query = `
			SELECT s.id, s.tenant_id, s.agent_id, s.started_at, s.ended_at,
			       COALESCE(ec.cnt, 0)
			FROM sessions s
			LEFT JOIN (
				SELECT session_id, COUNT(*) AS cnt
				FROM tool_events
				WHERE tenant_id = $1 AND session_id != ''
				GROUP BY session_id
			) ec ON ec.session_id = s.id
			WHERE s.tenant_id = $1
			ORDER BY s.started_at DESC
			LIMIT $2 OFFSET $3`
		args = []any{tenantID, limit, offset}
	} else {
		query = `
			SELECT s.id, s.tenant_id, s.agent_id, s.started_at, s.ended_at,
			       COALESCE(ec.cnt, 0)
			FROM sessions s
			LEFT JOIN (
				SELECT session_id, COUNT(*) AS cnt
				FROM tool_events
				WHERE session_id != ''
				GROUP BY session_id
			) ec ON ec.session_id = s.id
			ORDER BY s.started_at DESC
			LIMIT $1 OFFSET $2`
		args = []any{limit, offset}
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("console.ListSessions: %w", err)
	}
	defer rows.Close()

	out := make([]Session, 0)
	for rows.Next() {
		var sess Session
		if err := rows.Scan(&sess.ID, &sess.TenantID, &sess.AgentID,
			&sess.StartedAt, &sess.EndedAt, &sess.EventCount); err != nil {
			return nil, fmt.Errorf("console.ListSessions scan: %w", err)
		}
		out = append(out, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.ListSessions iteration: %w", err)
	}
	return out, nil
}

func (s *Store) GetSessionTimeline(ctx context.Context, sessionID string, tenantScope string) ([]EventListItem, error) {
	query := `
		SELECT event_id, tenant_id, agent_id, tool, action,
		       COALESCE(payload_json->>'resource', ''), risk_score,
		       decision, session_id, trace_id, received_at
		FROM tool_events
		WHERE session_id = $1`
	args := []any{sessionID}
	if tenantScope != "" {
		query += ` AND tenant_id = $2`
		args = append(args, tenantScope)
	}
	query += ` ORDER BY received_at ASC`
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("console.GetSessionTimeline: %w", err)
	}
	defer rows.Close()

	out := make([]EventListItem, 0)
	for rows.Next() {
		var e EventListItem
		if err := rows.Scan(&e.EventID, &e.TenantID, &e.AgentID, &e.Tool, &e.Action,
			&e.Resource, &e.RiskScore, &e.Decision, &e.SessionID, &e.TraceID,
			&e.ReceivedAt); err != nil {
			return nil, fmt.Errorf("console.GetSessionTimeline scan: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.GetSessionTimeline iteration: %w", err)
	}
	return out, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Policy Versions
// ──────────────────────────────────────────────────────────────────────────────

func (s *Store) CreatePolicyVersion(ctx context.Context, tenantID *string, version, bundleHash, deployedBy, notes string, policyData json.RawMessage) (*PolicyVersion, error) {
	pv := &PolicyVersion{
		TenantID:   tenantID,
		Version:    version,
		BundleHash: bundleHash,
		DeployedBy: deployedBy,
		Notes:      notes,
		PolicyData: policyData,
	}
	if len(pv.PolicyData) == 0 {
		pv.PolicyData = json.RawMessage(`{}`)
	}

	err := s.pool.QueryRow(ctx, `
		INSERT INTO policy_versions (tenant_id, version, bundle_hash, deployed_by, notes, policy_data)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, deployed_at`,
		pv.TenantID, pv.Version, pv.BundleHash, pv.DeployedBy, pv.Notes, pv.PolicyData,
	).Scan(&pv.ID, &pv.DeployedAt)
	if err != nil {
		return nil, fmt.Errorf("console.CreatePolicyVersion: %w", err)
	}
	return pv, nil
}

func (s *Store) ListPolicyVersions(ctx context.Context, tenantID string, limit int) ([]PolicyVersion, error) {
	limit = clampLimit(limit)

	var query string
	var args []any
	if tenantID != "" {
		query = `
			SELECT id, tenant_id, bundle_hash, version, policy_data, deployed_by, deployed_at, notes
			FROM policy_versions
			WHERE tenant_id = $1
			ORDER BY deployed_at DESC
			LIMIT $2`
		args = []any{tenantID, limit}
	} else {
		query = `
			SELECT id, tenant_id, bundle_hash, version, policy_data, deployed_by, deployed_at, notes
			FROM policy_versions
			ORDER BY deployed_at DESC
			LIMIT $1`
		args = []any{limit}
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("console.ListPolicyVersions: %w", err)
	}
	defer rows.Close()

	out := make([]PolicyVersion, 0)
	for rows.Next() {
		var pv PolicyVersion
		if err := rows.Scan(&pv.ID, &pv.TenantID, &pv.BundleHash, &pv.Version,
			&pv.PolicyData, &pv.DeployedBy, &pv.DeployedAt, &pv.Notes); err != nil {
			return nil, fmt.Errorf("console.ListPolicyVersions scan: %w", err)
		}
		out = append(out, pv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.ListPolicyVersions iteration: %w", err)
	}
	return out, nil
}

func (s *Store) GetPolicyVersion(ctx context.Context, id int64) (*PolicyVersion, error) {
	var pv PolicyVersion
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, bundle_hash, version, policy_data, deployed_by, deployed_at, notes
		FROM policy_versions WHERE id = $1`, id,
	).Scan(&pv.ID, &pv.TenantID, &pv.BundleHash, &pv.Version,
		&pv.PolicyData, &pv.DeployedBy, &pv.DeployedAt, &pv.Notes)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("console.GetPolicyVersion: %w", err)
	}
	return &pv, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Alert Rules
// ──────────────────────────────────────────────────────────────────────────────

func (s *Store) CreateAlertRule(ctx context.Context, rule AlertRule) (*AlertRule, error) {
	rule.ID = uuid.NewString()
	if len(rule.Config) == 0 {
		rule.Config = json.RawMessage(`{}`)
	}

	err := s.pool.QueryRow(ctx, `
		INSERT INTO alert_rules (id, tenant_id, name, rule_type, config, enabled)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at, updated_at`,
		rule.ID, rule.TenantID, rule.Name, rule.RuleType, rule.Config, rule.Enabled,
	).Scan(&rule.CreatedAt, &rule.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("console.CreateAlertRule: %w", err)
	}
	return &rule, nil
}

func (s *Store) ListAlertRules(ctx context.Context, tenantID string) ([]AlertRule, error) {
	var query string
	var args []any
	if tenantID != "" {
		query = `SELECT id, tenant_id, name, rule_type, config, enabled, created_at, updated_at
			FROM alert_rules WHERE tenant_id = $1 ORDER BY created_at DESC`
		args = []any{tenantID}
	} else {
		query = `SELECT id, tenant_id, name, rule_type, config, enabled, created_at, updated_at
			FROM alert_rules ORDER BY created_at DESC LIMIT 200`
	}
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("console.ListAlertRules: %w", err)
	}
	defer rows.Close()

	out := make([]AlertRule, 0)
	for rows.Next() {
		var r AlertRule
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Name, &r.RuleType,
			&r.Config, &r.Enabled, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("console.ListAlertRules scan: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.ListAlertRules iteration: %w", err)
	}
	return out, nil
}

func (s *Store) UpdateAlertRule(ctx context.Context, tenantID, id string, name string, ruleType string, config json.RawMessage, enabled bool) error {
	if len(config) == 0 {
		config = json.RawMessage(`{}`)
	}

	res, err := s.pool.Exec(ctx, `
		UPDATE alert_rules
		SET name = $2,
		    rule_type = $3,
		    config = $4,
		    enabled = $5,
		    updated_at = NOW()
		WHERE tenant_id = $1 AND id = $6`, tenantID, name, ruleType, config, enabled, id)
	if err != nil {
		return fmt.Errorf("console.UpdateAlertRule: %w", err)
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("console.UpdateAlertRule: rule %s not found for tenant", id)
	}
	return nil
}

func (s *Store) CreateAlertEvent(ctx context.Context, ruleID, tenantID, severity, message string, contextJSON json.RawMessage) (*AlertEvent, error) {
	ae := &AlertEvent{
		ID:           uuid.NewString(),
		RuleID:       ruleID,
		TenantID:     tenantID,
		Severity:     severity,
		Message:      message,
		ContextJSON:  contextJSON,
		Status:       "pending",
		AttemptCount: 0,
		LastError:    "",
	}
	if len(ae.ContextJSON) == 0 {
		ae.ContextJSON = json.RawMessage(`{}`)
	}

	err := s.pool.QueryRow(ctx, `
		INSERT INTO alert_events (id, rule_id, tenant_id, severity, message, details)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING status, delivered_at, attempt_count, next_attempt_at, last_error, created_at`,
		ae.ID, ae.RuleID, ae.TenantID, ae.Severity, ae.Message, ae.ContextJSON,
	).Scan(&ae.Status, &ae.DeliveredAt, &ae.AttemptCount, &ae.NextAttemptAt, &ae.LastError, &ae.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("console.CreateAlertEvent: %w", err)
	}
	return ae, nil
}

func (s *Store) ListAlertEvents(ctx context.Context, tenantID string, limit, offset int) ([]AlertEvent, error) {
	limit = clampLimit(limit)
	offset = clampOffset(offset)

	var query string
	var args []any
	if tenantID != "" {
		query = `SELECT id, rule_id, tenant_id, severity, message, details, status, delivered_at, attempt_count, last_error, created_at
			FROM alert_events WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
		args = []any{tenantID, limit, offset}
	} else {
		query = `SELECT id, rule_id, tenant_id, severity, message, details, status, delivered_at, attempt_count, last_error, created_at
			FROM alert_events ORDER BY created_at DESC LIMIT $1 OFFSET $2`
		args = []any{limit, offset}
	}
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("console.ListAlertEvents: %w", err)
	}
	defer rows.Close()

	out := make([]AlertEvent, 0)
	for rows.Next() {
		var ae AlertEvent
		if err := rows.Scan(&ae.ID, &ae.RuleID, &ae.TenantID, &ae.Severity,
			&ae.Message, &ae.ContextJSON, &ae.Status, &ae.DeliveredAt, &ae.AttemptCount, &ae.LastError, &ae.CreatedAt); err != nil {
			return nil, fmt.Errorf("console.ListAlertEvents scan: %w", err)
		}
		out = append(out, ae)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.ListAlertEvents iteration: %w", err)
	}
	return out, nil
}

func (s *Store) GetAlertRule(ctx context.Context, tenantID, ruleID string) (*AlertRule, error) {
	var r AlertRule
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, rule_type, config, enabled, created_at, updated_at
		FROM alert_rules
		WHERE tenant_id = $1 AND id = $2`, tenantID, ruleID,
	).Scan(&r.ID, &r.TenantID, &r.Name, &r.RuleType, &r.Config, &r.Enabled, &r.CreatedAt, &r.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("console.GetAlertRule: %w", err)
	}
	return &r, nil
}

func (s *Store) DeleteAlertRule(ctx context.Context, tenantID, ruleID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("console.DeleteAlertRule begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // commit path is a no-op rollback

	// Delete events first to avoid FK issues (alert_events.rule_id -> alert_rules.id).
	if _, err := tx.Exec(ctx, `
		DELETE FROM alert_events
		WHERE tenant_id = $1 AND rule_id = $2`, tenantID, ruleID); err != nil {
		return fmt.Errorf("console.DeleteAlertRule delete events: %w", err)
	}

	res, err := tx.Exec(ctx, `
		DELETE FROM alert_rules
		WHERE tenant_id = $1 AND id = $2`, tenantID, ruleID)
	if err != nil {
		return fmt.Errorf("console.DeleteAlertRule delete rule: %w", err)
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("console.DeleteAlertRule: rule %s not found", ruleID)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("console.DeleteAlertRule commit: %w", err)
	}
	return nil
}

func (s *Store) ListEnabledDenySpikeRules(ctx context.Context) ([]AlertRule, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, name, rule_type, config, enabled, created_at, updated_at
		FROM alert_rules
		WHERE enabled = true AND rule_type = 'deny_spike'
		ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("console.ListEnabledDenySpikeRules: %w", err)
	}
	defer rows.Close()

	out := make([]AlertRule, 0)
	for rows.Next() {
		var r AlertRule
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Name, &r.RuleType, &r.Config, &r.Enabled, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("console.ListEnabledDenySpikeRules scan: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.ListEnabledDenySpikeRules iteration: %w", err)
	}
	return out, nil
}

func (s *Store) CountDenyToolEventsInWindow(ctx context.Context, tenantID string, since time.Time) (int, error) {
	var count int
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM tool_events
		WHERE tenant_id = $1
		  AND decision = 'deny'
		  AND received_at >= $2`, tenantID, since,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("console.CountDenyToolEventsInWindow: %w", err)
	}
	return count, nil
}

func (s *Store) AlertEventExistsInWindow(ctx context.Context, tenantID, ruleID string, since time.Time) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM alert_events
			WHERE tenant_id = $1
			  AND rule_id = $2
			  AND created_at >= $3
			LIMIT 1
		)`, tenantID, ruleID, since,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("console.AlertEventExistsInWindow: %w", err)
	}
	return exists, nil
}

func (s *Store) ListAlertEventsSince(ctx context.Context, tenantID string, since time.Time, limit int) ([]AlertEvent, error) {
	limit = clampLimit(limit)
	rows, err := s.pool.Query(ctx, `
		SELECT id, rule_id, tenant_id, severity, message, details, status, delivered_at, attempt_count, last_error, created_at
		FROM alert_events
		WHERE tenant_id = $1
		  AND created_at >= $2
		ORDER BY created_at DESC
		LIMIT $3`, tenantID, since, limit)
	if err != nil {
		return nil, fmt.Errorf("console.ListAlertEventsSince: %w", err)
	}
	defer rows.Close()

	out := make([]AlertEvent, 0)
	for rows.Next() {
		var ae AlertEvent
		if err := rows.Scan(&ae.ID, &ae.RuleID, &ae.TenantID, &ae.Severity,
			&ae.Message, &ae.ContextJSON, &ae.Status, &ae.DeliveredAt, &ae.AttemptCount, &ae.LastError, &ae.CreatedAt); err != nil {
			return nil, fmt.Errorf("console.ListAlertEventsSince scan: %w", err)
		}
		out = append(out, ae)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.ListAlertEventsSince iteration: %w", err)
	}
	return out, nil
}

func (s *Store) ClaimPendingAlertEventsDue(ctx context.Context, limit int) ([]AlertEvent, error) {
	limit = clampLimit(limit)
	rows, err := s.pool.Query(ctx, `
		SELECT id, rule_id, tenant_id, severity, message, details, status, delivered_at, attempt_count, last_error, next_attempt_at, created_at
		FROM alert_events
		WHERE status = 'pending'
		  AND next_attempt_at <= NOW()
		ORDER BY created_at ASC
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("console.ClaimPendingAlertEventsDue: %w", err)
	}
	defer rows.Close()

	out := make([]AlertEvent, 0)
	for rows.Next() {
		var ae AlertEvent
		var nextAttemptAt time.Time
		if err := rows.Scan(&ae.ID, &ae.RuleID, &ae.TenantID, &ae.Severity,
			&ae.Message, &ae.ContextJSON, &ae.Status, &ae.DeliveredAt, &ae.AttemptCount, &ae.LastError, &nextAttemptAt, &ae.CreatedAt); err != nil {
			return nil, fmt.Errorf("console.ClaimPendingAlertEventsDue scan: %w", err)
		}
		ae.NextAttemptAt = nextAttemptAt
		out = append(out, ae)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.ClaimPendingAlertEventsDue iteration: %w", err)
	}
	return out, nil
}

func (s *Store) MarkAlertEventSent(ctx context.Context, eventID string) error {
	res, err := s.pool.Exec(ctx, `
		UPDATE alert_events
		SET status = 'sent',
		    delivered_at = NOW(),
		    notified = true,
		    last_error = ''
		WHERE id = $1`, eventID)
	if err != nil {
		return fmt.Errorf("console.MarkAlertEventSent: %w", err)
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("console.MarkAlertEventSent: no rows updated for id %s", eventID)
	}
	return nil
}

func (s *Store) MarkAlertEventPendingRetry(ctx context.Context, eventID string, attempts int, next time.Time, lastErr string) error {
	res, err := s.pool.Exec(ctx, `
		UPDATE alert_events
		SET status = 'pending',
		    attempt_count = $2,
		    next_attempt_at = $3,
		    last_error = $4,
		    notified = false
		WHERE id = $1`, eventID, attempts, next, lastErr)
	if err != nil {
		return fmt.Errorf("console.MarkAlertEventPendingRetry: %w", err)
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("console.MarkAlertEventPendingRetry: no rows updated for id %s", eventID)
	}
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Usage Counters
// ──────────────────────────────────────────────────────────────────────────────

type UsageCounter struct {
	TenantID       string `json:"tenant_id"`
	CounterDate    string `json:"counter_date"`
	Requests       int64  `json:"requests"`
	Approvals      int64  `json:"approvals"`
	Executions     int64  `json:"executions"`
	ConnectorCalls int64  `json:"connector_calls"`
}

var validUsageFields = map[string]string{
	"requests":        "requests",
	"approvals":       "approvals",
	"executions":      "executions",
	"connector_calls": "connector_calls",
}

// IncrementUsageCounter atomically increments a daily usage counter for the
// given tenant. Field must be one of: requests, approvals, executions,
// connector_calls.
func (s *Store) IncrementUsageCounter(ctx context.Context, tenantID string, field string) error {
	col, ok := validUsageFields[field]
	if !ok {
		return fmt.Errorf("console.IncrementUsageCounter: invalid field %q", field)
	}

	today := time.Now().UTC().Format("2006-01-02")
	query := fmt.Sprintf(`
		INSERT INTO usage_counters (tenant_id, counter_date, %s)
		VALUES ($1, $2, 1)
		ON CONFLICT (tenant_id, counter_date)
		DO UPDATE SET %s = usage_counters.%s + 1`, col, col, col)

	_, err := s.pool.Exec(ctx, query, tenantID, today)
	if err != nil {
		return fmt.Errorf("console.IncrementUsageCounter: %w", err)
	}
	return nil
}

// GetUsageCounters returns daily usage counters for a tenant since the given
// timestamp, ordered by date ascending.
func (s *Store) GetUsageCounters(ctx context.Context, tenantID string, since time.Time) ([]UsageCounter, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT tenant_id, counter_date::text, requests, approvals, executions, connector_calls
		FROM usage_counters
		WHERE tenant_id = $1 AND counter_date >= $2::date
		ORDER BY counter_date ASC`, tenantID, since)
	if err != nil {
		return nil, fmt.Errorf("console.GetUsageCounters: %w", err)
	}
	defer rows.Close()

	out := make([]UsageCounter, 0)
	for rows.Next() {
		var uc UsageCounter
		if err := rows.Scan(&uc.TenantID, &uc.CounterDate, &uc.Requests,
			&uc.Approvals, &uc.Executions, &uc.ConnectorCalls); err != nil {
			return nil, fmt.Errorf("console.GetUsageCounters scan: %w", err)
		}
		out = append(out, uc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.GetUsageCounters iteration: %w", err)
	}
	return out, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Connectors (metadata from observed tool events)
// ──────────────────────────────────────────────────────────────────────────────

func (s *Store) ListConnectors(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT tool, array_agg(DISTINCT action ORDER BY action) AS actions, COUNT(*) AS event_count
		FROM tool_events
		GROUP BY tool
		ORDER BY tool`)
	if err != nil {
		return nil, fmt.Errorf("console.ListConnectors: %w", err)
	}
	defer rows.Close()

	out := make([]map[string]any, 0)
	for rows.Next() {
		var tool string
		var actions []string
		var eventCount int64
		if err := rows.Scan(&tool, &actions, &eventCount); err != nil {
			return nil, fmt.Errorf("console.ListConnectors scan: %w", err)
		}
		out = append(out, map[string]any{
			"tool":        tool,
			"actions":     actions,
			"event_count": eventCount,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.ListConnectors iteration: %w", err)
	}
	return out, nil
}
