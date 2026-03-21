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

var (
	ErrTenantNotFound        = errors.New("tenant not found")
	ErrAPIKeyNotFound        = errors.New("api key not found")
	ErrAPIKeyAlreadyRevoked  = errors.New("api key already revoked")
	ErrAlertRuleNotFound     = errors.New("alert rule not found")
	ErrSessionTenantRequired = errors.New("tenant_id required for ambiguous session_id")
)

type SessionTenantAmbiguityError struct {
	Candidates []string
}

func (e *SessionTenantAmbiguityError) Error() string {
	return ErrSessionTenantRequired.Error()
}

func (e *SessionTenantAmbiguityError) Is(target error) bool {
	return target == ErrSessionTenantRequired
}

func SessionTenantCandidates(err error) []string {
	var ambiguity *SessionTenantAmbiguityError
	if !errors.As(err, &ambiguity) || len(ambiguity.Candidates) == 0 {
		return nil
	}
	out := make([]string, len(ambiguity.Candidates))
	copy(out, ambiguity.Candidates)
	return out
}

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

type TenantPolicyConfig struct {
	MaxRiskAutoApprove         int      `json:"max_risk_auto_approve"`
	ReadActions                []string `json:"read_actions,omitempty"`
	WriteActions               []string `json:"write_actions,omitempty"`
	DestructiveActions         []string `json:"destructive_actions,omitempty"`
	RequireDestructiveApproval bool     `json:"require_destructive_approval"`
}

func (c TenantPolicyConfig) ToPolicyInputMap() map[string]string {
	out := map[string]string{
		"max_risk_auto_approve":        strconv.Itoa(c.MaxRiskAutoApprove),
		"require_destructive_approval": strconv.FormatBool(c.RequireDestructiveApproval),
	}
	if len(c.ReadActions) > 0 {
		out["read_actions_csv"] = strings.Join(c.ReadActions, ",")
	}
	if len(c.WriteActions) > 0 {
		out["write_actions_csv"] = strings.Join(c.WriteActions, ",")
	}
	if len(c.DestructiveActions) > 0 {
		out["destructive_actions_csv"] = strings.Join(c.DestructiveActions, ",")
	}
	return out
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
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	IsPrimary  bool       `json:"is_primary"`
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
	RangeStart          time.Time             `json:"range_start"`
	RangeEnd            time.Time             `json:"range_end"`
	Totals              DecisionTotals        `json:"totals"`
	Trend               []DecisionTrendBucket `json:"trend"`
	RiskHeatmap         []RiskHeatmapRow      `json:"risk_heatmap"`
	PerAgent            []AgentBreakdownRow   `json:"per_agent"`
	OnboardingChecklist OnboardingChecklist   `json:"onboarding_checklist"`
}

type EventListItem struct {
	EventID    string    `json:"event_id"`
	TenantID   string    `json:"tenant_id"`
	AgentID    string    `json:"agent_id"`
	UserID     string    `json:"user_id,omitempty"`
	UserName   string    `json:"user_name,omitempty"`
	UserEmail  string    `json:"user_email,omitempty"`
	Tool       string    `json:"tool"`
	Action     string    `json:"action"`
	Resource   string    `json:"resource"`
	RiskScore  int       `json:"risk_score"`
	Decision   string    `json:"decision"`
	SessionID  string    `json:"session_id"`
	TraceID    string    `json:"trace_id"`
	ReceivedAt time.Time `json:"received_at"`
}

type EventListFilters struct {
	TenantID  string
	AgentID   string
	UserID    string
	TraceID   string
	Tool      string
	Action    string
	Decision  string
	SessionID string
	RiskMin   *int
	RiskMax   *int
	Since     *time.Time
	Until     *time.Time
	Limit     int
	Offset    int
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
	ID            string     `json:"id"`
	TenantID      string     `json:"tenant_id"`
	AgentID       string     `json:"agent_id"`
	UserID        string     `json:"user_id,omitempty"`
	UserName      string     `json:"user_name,omitempty"`
	UserEmail     string     `json:"user_email,omitempty"`
	TraceID       string     `json:"trace_id,omitempty"`
	StartedAt     time.Time  `json:"started_at"`
	LastEventAt   time.Time  `json:"last_event_at"`
	EndedAt       *time.Time `json:"ended_at,omitempty"`
	EventCount    int64      `json:"event_count"`
	AllowCount    int64      `json:"allow_count"`
	DenyCount     int64      `json:"deny_count"`
	ApproveCount  int64      `json:"approve_count"`
	LastEventID   string     `json:"last_event_id,omitempty"`
	LastTool      string     `json:"last_tool,omitempty"`
	LastAction    string     `json:"last_action,omitempty"`
	LastDecision  string     `json:"last_decision,omitempty"`
	LastResource  string     `json:"last_resource,omitempty"`
	LastRiskScore int        `json:"last_risk_score,omitempty"`
}

type SessionFilters struct {
	TenantID  string
	SessionID string
	AgentID   string
	UserID    string
	TraceID   string
	Tool      string
	Action    string
	Decision  string
	RiskMin   *int
	RiskMax   *int
	Since     *time.Time
	Until     *time.Time
	Limit     int
	Offset    int
}

type SessionApprovalSummary struct {
	ID         string    `json:"id"`
	Status     string    `json:"status"`
	Reason     string    `json:"reason,omitempty"`
	DenyReason string    `json:"deny_reason,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type SessionExecutionSummary struct {
	EventID      string          `json:"event_id"`
	ReceivedAt   time.Time       `json:"received_at"`
	Status       string          `json:"status"`
	OutputJSON   json.RawMessage `json:"output_json,omitempty"`
	ErrorMsg     string          `json:"error_msg,omitempty"`
	DurationMS   int64           `json:"duration_ms"`
	PolicyReason string          `json:"policy_reason,omitempty"`
}

type SessionTimelineEvent struct {
	EventListItem
	PolicyReason string                   `json:"policy_reason,omitempty"`
	RiskFactors  []string                 `json:"risk_factors,omitempty"`
	Approval     *SessionApprovalSummary  `json:"approval,omitempty"`
	Execution    *SessionExecutionSummary `json:"execution,omitempty"`
	Explain      string                   `json:"explain"`
}

type sessionTimelineRow struct {
	EventID          string
	ParentEventID    string
	TenantID         string
	AgentID          string
	UserID           string
	UserName         string
	UserEmail        string
	Tool             string
	Action           string
	Resource         string
	RiskScore        int
	Decision         string
	SessionID        string
	TraceID          string
	ReceivedAt       time.Time
	PayloadJSON      []byte
	PolicyResultJSON []byte
	ResultStatus     *string
	ResultOutput     []byte
	ResultError      *string
	ResultDuration   *int64
	ApprovalID       *string
	ApprovalStatus   *string
	ApprovalReason   *string
	ApprovalDeny     *string
	ApprovalCreated  *time.Time
	ApprovalExpires  *time.Time
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

func (s *Store) GetTenantPolicyConfig(ctx context.Context, tenantID string) (*TenantPolicyConfig, bool, error) {
	t, err := s.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, false, err
	}
	if t == nil {
		return nil, false, nil
	}

	type tenantConfigWrapper struct {
		PolicyConfig *TenantPolicyConfig `json:"policy_config,omitempty"`
	}
	var w tenantConfigWrapper
	if len(t.Config) == 0 || string(t.Config) == "{}" {
		return nil, false, nil
	}
	if err := json.Unmarshal(t.Config, &w); err != nil {
		return nil, false, fmt.Errorf("console.GetTenantPolicyConfig unmarshal: %w", err)
	}
	if w.PolicyConfig == nil {
		return nil, false, nil
	}
	return w.PolicyConfig, true, nil
}

func (s *Store) SetTenantPolicyConfig(ctx context.Context, tenantID string, cfg TenantPolicyConfig) error {
	b, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("console.SetTenantPolicyConfig marshal: %w", err)
	}

	res, err := s.pool.Exec(ctx, `
		UPDATE tenants
		SET config = jsonb_set(
			COALESCE(config, '{}'::jsonb),
			'{policy_config}',
			$1::jsonb,
			true
		)
		WHERE id = $2`, b, tenantID)
	if err != nil {
		return fmt.Errorf("console.SetTenantPolicyConfig update: %w", err)
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("console.SetTenantPolicyConfig: tenant %s not found", tenantID)
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
		return fmt.Errorf("console.UpdateTenantStatus: %w: %s", ErrTenantNotFound, id)
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

func (s *Store) CreateAPIKey(ctx context.Context, tenantID, name string, expiresAt *time.Time) (*APIKeyCreateResult, error) {
	raw, prefix, keyHash, err := generateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("console.CreateAPIKey: %w", err)
	}

	// If the tenant already has an active primary key, new keys default to non-primary.
	var hasActivePrimary bool
	if err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM api_keys
			WHERE tenant_id = $1 AND status = 'active' AND is_primary = true
		)`, tenantID,
	).Scan(&hasActivePrimary); err != nil {
		return nil, fmt.Errorf("console.CreateAPIKey primary check: %w", err)
	}

	k := APIKey{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		Name:      name,
		KeyPrefix: prefix,
		Status:    "active",
		ExpiresAt: expiresAt,
		IsPrimary: !hasActivePrimary,
	}

	err = s.pool.QueryRow(ctx, `
		INSERT INTO api_keys (id, tenant_id, name, key_prefix, key_hash, status, expires_at, is_primary)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at`,
		k.ID, k.TenantID, k.Name, k.KeyPrefix, keyHash, k.Status, k.ExpiresAt, k.IsPrimary,
	).Scan(&k.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("console.CreateAPIKey: %w", err)
	}

	return &APIKeyCreateResult{APIKey: k, RawKey: raw}, nil
}

func (s *Store) ListAPIKeys(ctx context.Context, tenantID string) ([]APIKey, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, name, key_prefix, status, created_at, expires_at, is_primary, revoked_at, last_used_at
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
			&k.CreatedAt, &k.ExpiresAt, &k.IsPrimary, &k.RevokedAt, &k.LastUsedAt); err != nil {
			return nil, fmt.Errorf("console.ListAPIKeys scan: %w", err)
		}
		out = append(out, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.ListAPIKeys iteration: %w", err)
	}
	return out, nil
}

func (s *Store) RevokeAPIKeyForTenant(ctx context.Context, tenantID, keyID string) error {
	var status string
	err := s.pool.QueryRow(ctx, `
		SELECT status
		FROM api_keys
		WHERE id = $1 AND tenant_id = $2`, keyID, tenantID).Scan(&status)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("console.RevokeAPIKeyForTenant: %w: %s", ErrAPIKeyNotFound, keyID)
	}
	if err != nil {
		return fmt.Errorf("console.RevokeAPIKeyForTenant lookup: %w", err)
	}
	if status != "active" {
		return fmt.Errorf("console.RevokeAPIKeyForTenant: %w: %s", ErrAPIKeyAlreadyRevoked, keyID)
	}

	res, err := s.pool.Exec(ctx, `
		UPDATE api_keys
		SET status = 'revoked', revoked_at = NOW(), is_primary = false
		WHERE id = $1 AND tenant_id = $2 AND status = 'active'`, keyID, tenantID)
	if err != nil {
		return fmt.Errorf("console.RevokeAPIKeyForTenant: %w", err)
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("console.RevokeAPIKeyForTenant: %w: %s", ErrAPIKeyAlreadyRevoked, keyID)
	}
	return nil
}

// RotateAPIKeysPrimary implements the UX workflow:
// create new key -> (optionally) mark it as primary -> (optionally) revoke
// the previous active primary key.
func (s *Store) RotateAPIKeysPrimary(
	ctx context.Context,
	tenantID, name string,
	expiresAt *time.Time,
	makePrimary bool,
	revokeOldPrimary bool,
) (*APIKeyCreateResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("console.RotateAPIKeysPrimary begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var oldPrimaryID string
	err = tx.QueryRow(ctx, `
		SELECT id FROM api_keys
		WHERE tenant_id = $1 AND status = 'active' AND is_primary = true
		LIMIT 1`, tenantID,
	).Scan(&oldPrimaryID)
	if err != nil {
		if err != pgx.ErrNoRows {
			return nil, fmt.Errorf("console.RotateAPIKeysPrimary old primary: %w", err)
		}
		oldPrimaryID = ""
	}

	// If the caller wants the new key to become primary, clear primary flags
	// for all active keys before inserting (avoids unique-index conflicts).
	if makePrimary {
		if _, err := tx.Exec(ctx, `
			UPDATE api_keys
			SET is_primary = false
			WHERE tenant_id = $1 AND status = 'active'`, tenantID,
		); err != nil {
			return nil, fmt.Errorf("console.RotateAPIKeysPrimary clear primary: %w", err)
		}
	}

	raw, prefix, keyHash, err := generateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("console.RotateAPIKeysPrimary generate: %w", err)
	}

	newKeyID := uuid.NewString()
	k := APIKey{
		ID:         newKeyID,
		TenantID:   tenantID,
		Name:       name,
		KeyPrefix:  prefix,
		Status:     "active",
		ExpiresAt:  expiresAt,
		IsPrimary:  makePrimary,
		RevokedAt:  nil,
		LastUsedAt: nil,
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO api_keys (id, tenant_id, name, key_prefix, key_hash, status, expires_at, is_primary)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at`, k.ID, k.TenantID, k.Name, k.KeyPrefix, keyHash, k.Status, k.ExpiresAt, k.IsPrimary,
	).Scan(&k.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("console.RotateAPIKeysPrimary insert: %w", err)
	}

	if revokeOldPrimary && oldPrimaryID != "" && oldPrimaryID != newKeyID {
		if _, err := tx.Exec(ctx, `
			UPDATE api_keys
			SET status = 'revoked', revoked_at = NOW(), is_primary = false
			WHERE id = $1 AND tenant_id = $2 AND status = 'active'`, oldPrimaryID, tenantID,
		); err != nil {
			return nil, fmt.Errorf("console.RotateAPIKeysPrimary revoke old primary: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("console.RotateAPIKeysPrimary commit: %w", err)
	}

	return &APIKeyCreateResult{APIKey: k, RawKey: raw}, nil
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
		WHERE key_prefix = $1
		  AND status = 'active'
		  AND (expires_at IS NULL OR expires_at > NOW())`, prefix)
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
		UPDATE api_keys SET status = 'revoked', revoked_at = NOW(), is_primary = false
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
		INSERT INTO api_keys (id, tenant_id, name, key_prefix, key_hash, status, is_primary)
		VALUES ($1, $2, $3, $4, $5, $6, true)
		RETURNING created_at`,
		k.ID, k.TenantID, k.Name, k.KeyPrefix, keyHash, k.Status,
	).Scan(&k.CreatedAt)
	k.IsPrimary = true
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
	email = canonicalEmail(email)
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
	email = canonicalEmail(email)
	var u User
	var passwordHash *string
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, name, status, created_at
		FROM users WHERE lower(email) = lower($1)`, email,
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
	email = canonicalEmail(email)
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
	Token       string     `json:"token,omitempty"`
	Email       string     `json:"email"`
	TenantID    string     `json:"tenant_id"`
	Role        string     `json:"role"`
	Name        string     `json:"name"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
	EmailStatus string     `json:"email_status,omitempty"`
	EmailSentAt *time.Time `json:"email_sent_at,omitempty"`
	EmailError  string     `json:"email_error,omitempty"`
}

// InviteAcceptResult is the structured response payload for the invite acceptance flow.
// It includes the created/updated user plus the assigned tenant-scoped role metadata.
type InviteAcceptResult struct {
	User     *User  `json:"user"`
	TenantID string `json:"tenant_id"`
	Role     string `json:"role"`
}

func (s *Store) CreateInvite(ctx context.Context, token, email, tenantID, role, name string, expiresAt time.Time) error {
	email = canonicalEmail(email)
	tokenHash := s.hashInviteResetToken(token)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO invites (token, email, tenant_id, role, name, expires_at, email_status, email_last_error)
		VALUES ($1, $2, $3, $4, $5, $6, 'pending', '')`, tokenHash, email, tenantID, role, name, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("console.CreateInvite: %w", err)
	}
	return nil
}

func (s *Store) UpdateInviteEmailStatus(ctx context.Context, token, status string, sentAt *time.Time, emailError string) error {
	tokenHash := s.hashInviteResetToken(token)
	_, err := s.pool.Exec(ctx, `
		UPDATE invites
		SET email_status = $2,
		    email_sent_at = $3,
		    email_last_error = $4
		WHERE token = $1`,
		tokenHash, status, sentAt, emailError,
	)
	if err != nil {
		return fmt.Errorf("console.UpdateInviteEmailStatus: %w", err)
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
		var createdAt time.Time
		err = tx.QueryRow(ctx, `
			INSERT INTO users (id, email, password_hash, name, slack_user_id, status)
			VALUES ($1, $2, $3, $4, NULL, 'active')
			RETURNING created_at`,
			userID, inv.Email, string(hashed), acceptName,
		).Scan(&createdAt)
		if err != nil {
			return nil, fmt.Errorf("console.ConsumeInviteAccept insert user: %w", err)
		}
		u = User{
			ID:          userID,
			Email:       inv.Email,
			Name:        acceptName,
			SlackUserID: nil,
			Status:      "active",
			CreatedAt:   createdAt,
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
			SELECT '' AS token, email, tenant_id, role, name, created_at, expires_at, email_status, email_sent_at, email_last_error
			FROM invites
			WHERE tenant_id = $1 AND expires_at > NOW()
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3`, *tenantID, limit, offset)
	} else {
		rows, err = s.pool.Query(ctx, `
			SELECT '' AS token, email, tenant_id, role, name, created_at, expires_at, email_status, email_sent_at, email_last_error
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
		if err := rows.Scan(&inv.Token, &inv.Email, &inv.TenantID, &inv.Role, &inv.Name, &inv.CreatedAt, &inv.ExpiresAt, &inv.EmailStatus, &inv.EmailSentAt, &inv.EmailError); err != nil {
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
	email = canonicalEmail(email)
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

func canonicalEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
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
		Totals:              totals,
		Trend:               trend,
		RiskHeatmap:         riskHeatmap,
		PerAgent:            perAgent,
		OnboardingChecklist: onboarding,
	}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Events / Audit Trail
// ──────────────────────────────────────────────────────────────────────────────

func (s *Store) ListEvents(ctx context.Context, filters EventListFilters) ([]EventListItem, error) {
	limit := clampLimit(filters.Limit)
	offset := clampOffset(filters.Offset)

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

	addFilter("e.tenant_id", filters.TenantID)
	addFilter("e.agent_id", filters.AgentID)
	addFilter("e.user_id", filters.UserID)
	addFilter("e.trace_id", filters.TraceID)
	addFilter("e.tool", filters.Tool)
	addFilter("e.action", filters.Action)
	addFilter("e.decision", filters.Decision)
	addFilter("e.session_id", filters.SessionID)
	if filters.RiskMin != nil {
		clauses = append(clauses, fmt.Sprintf("e.risk_score >= $%d", argIdx))
		args = append(args, *filters.RiskMin)
		argIdx++
	}
	if filters.RiskMax != nil {
		clauses = append(clauses, fmt.Sprintf("e.risk_score <= $%d", argIdx))
		args = append(args, *filters.RiskMax)
		argIdx++
	}
	if filters.Since != nil {
		clauses = append(clauses, fmt.Sprintf("e.received_at >= $%d", argIdx))
		args = append(args, *filters.Since)
		argIdx++
	}
	if filters.Until != nil {
		clauses = append(clauses, fmt.Sprintf("e.received_at <= $%d", argIdx))
		args = append(args, *filters.Until)
		argIdx++
	}

	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}

	query := fmt.Sprintf(`
		SELECT e.event_id, e.tenant_id, e.agent_id,
		       COALESCE(e.user_id, ''),
		       COALESCE(e.payload_json->'labels'->>'user_name', ''),
		       COALESCE(e.payload_json->'labels'->>'user_email', ''),
		       e.tool, e.action,
		       COALESCE(e.payload_json->>'resource', ''), e.risk_score,
		       e.decision, e.session_id, e.trace_id, e.received_at
		FROM tool_events e
		%s
		ORDER BY e.received_at DESC, e.event_seq DESC
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
		if err := rows.Scan(
			&e.EventID, &e.TenantID, &e.AgentID,
			&e.UserID, &e.UserName, &e.UserEmail,
			&e.Tool, &e.Action, &e.Resource, &e.RiskScore,
			&e.Decision, &e.SessionID, &e.TraceID, &e.ReceivedAt,
		); err != nil {
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
		SELECT event_id, tenant_id, agent_id,
		       COALESCE(user_id, ''),
		       COALESCE(payload_json->'labels'->>'user_name', ''),
		       COALESCE(payload_json->'labels'->>'user_email', ''),
		       tool, action,
		       COALESCE(payload_json->>'resource', ''), risk_score,
		       decision, session_id, trace_id, received_at
		FROM tool_events
		WHERE tenant_id = $1 AND received_at >= $2 AND received_at <= $3
		ORDER BY received_at ASC, event_seq ASC
		LIMIT $4`, tenantID, since, until, limit)
	if err != nil {
		return nil, fmt.Errorf("console.ListEventsInRange: %w", err)
	}
	defer rows.Close()

	out := make([]EventListItem, 0)
	for rows.Next() {
		var e EventListItem
		if err := rows.Scan(
			&e.EventID, &e.TenantID, &e.AgentID,
			&e.UserID, &e.UserName, &e.UserEmail,
			&e.Tool, &e.Action, &e.Resource, &e.RiskScore,
			&e.Decision, &e.SessionID, &e.TraceID, &e.ReceivedAt,
		); err != nil {
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
		SELECT e.event_id, e.tenant_id, e.agent_id,
		       COALESCE(e.user_id, ''),
		       COALESCE(e.payload_json->'labels'->>'user_name', ''),
		       COALESCE(e.payload_json->'labels'->>'user_email', ''),
		       e.tool, e.action,
		       COALESCE(e.payload_json->>'resource', ''), e.risk_score,
		       e.decision, e.session_id, e.trace_id, e.received_at,
		       e.payload_json, e.policy_result, e.hash, e.prev_hash,
		       r.status, r.output_json, r.error_msg, r.duration_ms
		FROM tool_events e
		LEFT JOIN LATERAL (
			SELECT r.status, r.output_json, r.error_msg, r.duration_ms
			FROM tool_results r
			WHERE r.event_id = e.event_id
			ORDER BY r.created_at DESC, r.id DESC
			LIMIT 1
		) r ON true
		WHERE e.event_id = $1`, eventID,
	).Scan(
		&d.EventID, &d.TenantID, &d.AgentID,
		&d.UserID, &d.UserName, &d.UserEmail,
		&d.Tool, &d.Action, &d.Resource, &d.RiskScore,
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
		SELECT event_id, tenant_id, agent_id,
		       COALESCE(user_id, ''),
		       COALESCE(payload_json->'labels'->>'user_name', ''),
		       COALESCE(payload_json->'labels'->>'user_email', ''),
		       tool, action,
		       COALESCE(payload_json->>'resource', ''), risk_score,
		       decision, session_id, trace_id, received_at
		FROM tool_events
		WHERE tenant_id = $1 AND received_at >= $2 AND received_at <= $3
		ORDER BY received_at ASC, event_seq ASC`, tenantID, since, until)
	if err != nil {
		return fmt.Errorf("console.ExportEventsCSV: %w", err)
	}
	defer rows.Close()

	cw := csv.NewWriter(w)
	defer cw.Flush()

	header := []string{"event_id", "tenant_id", "agent_id", "user_id", "user_name", "user_email", "tool", "action",
		"resource", "risk_score", "decision", "session_id", "trace_id", "received_at"}
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("console.ExportEventsCSV write header: %w", err)
	}

	for rows.Next() {
		var e EventListItem
		if err := rows.Scan(
			&e.EventID, &e.TenantID, &e.AgentID,
			&e.UserID, &e.UserName, &e.UserEmail,
			&e.Tool, &e.Action, &e.Resource, &e.RiskScore,
			&e.Decision, &e.SessionID, &e.TraceID, &e.ReceivedAt,
		); err != nil {
			return fmt.Errorf("console.ExportEventsCSV scan: %w", err)
		}
		record := []string{
			e.EventID, e.TenantID, e.AgentID, e.UserID, e.UserName, e.UserEmail,
			e.Tool, e.Action, e.Resource, strconv.Itoa(e.RiskScore), e.Decision,
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

func (s *Store) ListSessions(ctx context.Context, filters SessionFilters) ([]Session, error) {
	limit := clampLimit(filters.Limit)
	offset := clampOffset(filters.Offset)

	clauses := []string{"e.session_id != ''"}
	args := make([]any, 0, 12)
	argIdx := 1
	addFilter := func(expr, value string) {
		if value == "" {
			return
		}
		clauses = append(clauses, fmt.Sprintf("%s = $%d", expr, argIdx))
		args = append(args, value)
		argIdx++
	}

	addFilter("e.tenant_id", filters.TenantID)
	addFilter("e.session_id", filters.SessionID)
	addFilter("e.agent_id", filters.AgentID)
	addFilter("e.user_id", filters.UserID)
	addFilter("e.trace_id", filters.TraceID)
	addFilter("e.tool", filters.Tool)
	addFilter("e.action", filters.Action)
	addFilter("e.decision", filters.Decision)
	if filters.RiskMin != nil {
		clauses = append(clauses, fmt.Sprintf("e.risk_score >= $%d", argIdx))
		args = append(args, *filters.RiskMin)
		argIdx++
	}
	if filters.RiskMax != nil {
		clauses = append(clauses, fmt.Sprintf("e.risk_score <= $%d", argIdx))
		args = append(args, *filters.RiskMax)
		argIdx++
	}
	if filters.Since != nil {
		clauses = append(clauses, fmt.Sprintf("e.received_at >= $%d", argIdx))
		args = append(args, *filters.Since)
		argIdx++
	}
	if filters.Until != nil {
		clauses = append(clauses, fmt.Sprintf("e.received_at <= $%d", argIdx))
		args = append(args, *filters.Until)
		argIdx++
	}

	query := fmt.Sprintf(`
		WITH matching_sessions AS (
			SELECT DISTINCT e.tenant_id, e.session_id
			FROM tool_events e
			WHERE %s
		),
		session_events AS (
			SELECT e.event_id, e.tenant_id, e.session_id, e.agent_id, e.user_id, e.trace_id,
			       COALESCE(e.payload_json->'labels'->>'user_name', '') AS user_name,
			       COALESCE(e.payload_json->'labels'->>'user_email', '') AS user_email,
			       e.tool, e.action, COALESCE(e.payload_json->>'resource', '') AS resource,
			       e.risk_score, e.decision, e.received_at,
			       ROW_NUMBER() OVER (
			       	PARTITION BY e.tenant_id, e.session_id
			       	ORDER BY e.received_at DESC, e.event_seq DESC
			       ) AS recency_rank
			FROM tool_events e
			JOIN matching_sessions ms
			  ON ms.tenant_id = e.tenant_id
			 AND ms.session_id = e.session_id
			WHERE e.session_id != ''
		)
		SELECT session_id AS id, tenant_id,
		       COALESCE(MAX(CASE WHEN recency_rank = 1 THEN agent_id END), '') AS agent_id,
		       COALESCE(MAX(CASE WHEN recency_rank = 1 THEN user_id END), '') AS user_id,
		       COALESCE(MAX(CASE WHEN recency_rank = 1 THEN user_name END), '') AS user_name,
		       COALESCE(MAX(CASE WHEN recency_rank = 1 THEN user_email END), '') AS user_email,
		       COALESCE(MAX(CASE WHEN recency_rank = 1 THEN trace_id END), '') AS trace_id,
		       MIN(received_at) AS started_at,
		       MAX(received_at) AS last_event_at,
		       COUNT(*) AS event_count,
		       COUNT(*) FILTER (WHERE decision = 'allow') AS allow_count,
		       COUNT(*) FILTER (WHERE decision = 'deny') AS deny_count,
		       COUNT(*) FILTER (WHERE decision = 'approve') AS approve_count,
		       COALESCE(MAX(CASE WHEN recency_rank = 1 THEN event_id END), '') AS last_event_id,
		       COALESCE(MAX(CASE WHEN recency_rank = 1 THEN tool END), '') AS last_tool,
		       COALESCE(MAX(CASE WHEN recency_rank = 1 THEN action END), '') AS last_action,
		       COALESCE(MAX(CASE WHEN recency_rank = 1 THEN decision END), '') AS last_decision,
		       COALESCE(MAX(CASE WHEN recency_rank = 1 THEN resource END), '') AS last_resource,
		       COALESCE(MAX(CASE WHEN recency_rank = 1 THEN risk_score END), 0) AS last_risk_score
		FROM session_events
		GROUP BY tenant_id, session_id
		ORDER BY MAX(received_at) DESC
		LIMIT $%d OFFSET $%d`, strings.Join(clauses, " AND "), argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("console.ListSessions: %w", err)
	}
	defer rows.Close()

	out := make([]Session, 0)
	for rows.Next() {
		var sess Session
		if err := rows.Scan(
			&sess.ID, &sess.TenantID, &sess.AgentID,
			&sess.UserID, &sess.UserName, &sess.UserEmail, &sess.TraceID,
			&sess.StartedAt, &sess.LastEventAt, &sess.EventCount,
			&sess.AllowCount, &sess.DenyCount, &sess.ApproveCount,
			&sess.LastEventID, &sess.LastTool, &sess.LastAction, &sess.LastDecision,
			&sess.LastResource, &sess.LastRiskScore,
		); err != nil {
			return nil, fmt.Errorf("console.ListSessions scan: %w", err)
		}
		last := sess.LastEventAt
		sess.EndedAt = &last
		out = append(out, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.ListSessions iteration: %w", err)
	}
	return out, nil
}

func (s *Store) GetSession(ctx context.Context, sessionID, tenantScope, tenantHint string) (*Session, error) {
	tenantID, err := s.resolveSessionTenant(ctx, sessionID, tenantScope, tenantHint)
	if err != nil {
		return nil, err
	}
	sessions, err := s.ListSessions(ctx, SessionFilters{
		TenantID:  tenantID,
		SessionID: sessionID,
		Limit:     2,
	})
	if err != nil {
		return nil, fmt.Errorf("console.GetSession: %w", err)
	}
	if len(sessions) == 0 {
		return nil, nil
	}
	return &sessions[0], nil
}

func (s *Store) GetSessionTimeline(ctx context.Context, sessionID, tenantScope, tenantHint string) ([]SessionTimelineEvent, error) {
	tenantID, err := s.resolveSessionTenant(ctx, sessionID, tenantScope, tenantHint)
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT e.event_id,
		       COALESCE(parent.parent_event_id, '') AS parent_event_id,
		       e.tenant_id, e.agent_id,
		       COALESCE(e.user_id, ''),
		       COALESCE(e.payload_json->'labels'->>'user_name', ''),
		       COALESCE(e.payload_json->'labels'->>'user_email', ''),
		       e.tool, e.action,
		       COALESCE(e.payload_json->>'resource', ''), e.risk_score,
		       e.decision, e.session_id, e.trace_id, e.received_at,
		       e.payload_json, e.policy_result,
		       r.status, r.output_json, r.error_msg, r.duration_ms,
		       ar.id, ar.status, ar.reason, ar.deny_reason, ar.created_at, ar.expires_at
		FROM tool_events e
		LEFT JOIN LATERAL (
			SELECT r.status, r.output_json, r.error_msg, r.duration_ms
			FROM tool_results r
			WHERE r.event_id = e.event_id
			ORDER BY r.created_at DESC, r.id DESC
			LIMIT 1
		) r ON true
		LEFT JOIN LATERAL (
			SELECT ar.id, ar.status, ar.reason, ar.deny_reason, ar.created_at, ar.expires_at
			FROM approval_requests ar
			WHERE ar.event_id = e.event_id
			ORDER BY ar.created_at DESC, ar.id DESC
			LIMIT 1
		) ar ON true
		LEFT JOIN LATERAL (
			SELECT parent.parent_event_id
			FROM tool_executions parent
			WHERE parent.execution_event_id = e.event_id
			ORDER BY parent.created_at DESC, parent.parent_event_id DESC
			LIMIT 1
		) parent ON true
		WHERE e.tenant_id = $1 AND e.session_id = $2
		ORDER BY e.received_at ASC, e.event_seq ASC`, tenantID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("console.GetSessionTimeline: %w", err)
	}
	defer rows.Close()

	scanned := make([]sessionTimelineRow, 0)

	for rows.Next() {
		var row sessionTimelineRow
		if err := rows.Scan(
			&row.EventID, &row.ParentEventID,
			&row.TenantID, &row.AgentID,
			&row.UserID, &row.UserName, &row.UserEmail,
			&row.Tool, &row.Action, &row.Resource, &row.RiskScore,
			&row.Decision, &row.SessionID, &row.TraceID, &row.ReceivedAt,
			&row.PayloadJSON, &row.PolicyResultJSON,
			&row.ResultStatus, &row.ResultOutput, &row.ResultError, &row.ResultDuration,
			&row.ApprovalID, &row.ApprovalStatus, &row.ApprovalReason, &row.ApprovalDeny, &row.ApprovalCreated, &row.ApprovalExpires,
		); err != nil {
			return nil, fmt.Errorf("console.GetSessionTimeline scan: %w", err)
		}
		scanned = append(scanned, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.GetSessionTimeline iteration: %w", err)
	}
	return buildSessionTimeline(scanned), nil
}

func (s *Store) ExportSessionCSV(ctx context.Context, sessionID, tenantScope, tenantHint string, w io.Writer) error {
	timeline, err := s.GetSessionTimeline(ctx, sessionID, tenantScope, tenantHint)
	if err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	defer cw.Flush()

	header := []string{
		"session_id", "event_id", "tenant_id", "agent_id", "user_id", "user_name", "user_email",
		"tool", "action", "resource", "decision", "risk_score", "policy_reason", "risk_factors",
		"approval_id", "approval_status", "execution_event_id", "execution_status", "trace_id", "received_at",
	}
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("console.ExportSessionCSV write header: %w", err)
	}
	for _, item := range timeline {
		record := []string{
			item.SessionID,
			item.EventID,
			item.TenantID,
			item.AgentID,
			item.UserID,
			item.UserName,
			item.UserEmail,
			item.Tool,
			item.Action,
			item.Resource,
			item.Decision,
			strconv.Itoa(item.RiskScore),
			item.PolicyReason,
			strings.Join(item.RiskFactors, "; "),
			"",
			"",
			"",
			"",
			item.TraceID,
			item.ReceivedAt.Format(time.RFC3339),
		}
		if item.Approval != nil {
			record[14] = item.Approval.ID
			record[15] = item.Approval.Status
		}
		if item.Execution != nil {
			record[16] = item.Execution.EventID
			record[17] = item.Execution.Status
		}
		if err := cw.Write(record); err != nil {
			return fmt.Errorf("console.ExportSessionCSV write row: %w", err)
		}
	}
	return nil
}

func (s *Store) resolveSessionTenant(ctx context.Context, sessionID, tenantScope, tenantHint string) (string, error) {
	if tenantScope != "" {
		return tenantScope, nil
	}
	if tenantHint != "" {
		return tenantHint, nil
	}

	tenants, err := s.ListSessionTenantCandidates(ctx, sessionID, 10)
	if err != nil {
		return "", fmt.Errorf("console.resolveSessionTenant: %w", err)
	}
	switch len(tenants) {
	case 0:
		return "", nil
	case 1:
		return tenants[0], nil
	default:
		return "", &SessionTenantAmbiguityError{Candidates: tenants}
	}
}

func (s *Store) ListSessionTenantCandidates(ctx context.Context, sessionID string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT tenant_id
		FROM tool_events
		WHERE session_id = $1
		ORDER BY tenant_id
		LIMIT $2`, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("console.ListSessionTenantCandidates: %w", err)
	}
	defer rows.Close()

	tenants := make([]string, 0, limit)
	for rows.Next() {
		var tenantID string
		if err := rows.Scan(&tenantID); err != nil {
			return nil, fmt.Errorf("console.ListSessionTenantCandidates scan: %w", err)
		}
		tenants = append(tenants, tenantID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.ListSessionTenantCandidates iteration: %w", err)
	}
	return tenants, nil
}

func sessionDetailsFromPayload(payloadJSON, policyResultJSON []byte) (string, []string) {
	type sessionPayload struct {
		RiskFactors []string `json:"risk_factors"`
	}
	type sessionPolicyResult struct {
		Reason string `json:"reason"`
	}

	var policyReason string
	if len(policyResultJSON) > 0 {
		var pr sessionPolicyResult
		if err := json.Unmarshal(policyResultJSON, &pr); err == nil {
			policyReason = strings.TrimSpace(pr.Reason)
		}
	}

	riskFactors := make([]string, 0)
	if len(payloadJSON) > 0 {
		var payload sessionPayload
		if err := json.Unmarshal(payloadJSON, &payload); err == nil && len(payload.RiskFactors) > 0 {
			riskFactors = append(riskFactors, payload.RiskFactors...)
		}
	}
	return policyReason, riskFactors
}

func sessionExecutionFromRow(eventID string, receivedAt time.Time, status *string, outputJSON []byte, errMsg *string, durationMS *int64, policyReason string) *SessionExecutionSummary {
	if status == nil {
		return nil
	}
	exec := &SessionExecutionSummary{
		EventID:      eventID,
		ReceivedAt:   receivedAt,
		Status:       *status,
		PolicyReason: policyReason,
	}
	if len(outputJSON) > 0 {
		exec.OutputJSON = outputJSON
	}
	if errMsg != nil {
		exec.ErrorMsg = *errMsg
	}
	if durationMS != nil {
		exec.DurationMS = *durationMS
	}
	return exec
}

func sessionApprovalFromRow(row sessionTimelineRow) *SessionApprovalSummary {
	if row.ApprovalID == nil || row.ApprovalStatus == nil || row.ApprovalCreated == nil || row.ApprovalExpires == nil {
		return nil
	}
	return &SessionApprovalSummary{
		ID:         *row.ApprovalID,
		Status:     *row.ApprovalStatus,
		Reason:     stringValue(row.ApprovalReason),
		DenyReason: stringValue(row.ApprovalDeny),
		CreatedAt:  *row.ApprovalCreated,
		ExpiresAt:  *row.ApprovalExpires,
	}
}

func buildSessionTimeline(rows []sessionTimelineRow) []SessionTimelineEvent {
	items := make([]*SessionTimelineEvent, 0, len(rows))
	index := make(map[string]*SessionTimelineEvent, len(rows))
	pendingExecutions := make(map[string]*SessionExecutionSummary)

	for _, row := range rows {
		policyReason, riskFactors := sessionDetailsFromPayload(row.PayloadJSON, row.PolicyResultJSON)
		execution := sessionExecutionFromRow(row.EventID, row.ReceivedAt, row.ResultStatus, row.ResultOutput, row.ResultError, row.ResultDuration, policyReason)

		if row.ParentEventID != "" {
			if execution == nil {
				continue
			}
			if parent, ok := index[row.ParentEventID]; ok {
				parent.Execution = execution
				parent.Explain = buildSessionExplain(parent)
			} else {
				pendingExecutions[row.ParentEventID] = execution
			}
			continue
		}

		item, exists := index[row.EventID]
		if !exists {
			item = &SessionTimelineEvent{
				EventListItem: EventListItem{
					EventID:    row.EventID,
					TenantID:   row.TenantID,
					AgentID:    row.AgentID,
					UserID:     row.UserID,
					UserName:   row.UserName,
					UserEmail:  row.UserEmail,
					Tool:       row.Tool,
					Action:     row.Action,
					Resource:   row.Resource,
					RiskScore:  row.RiskScore,
					Decision:   row.Decision,
					SessionID:  row.SessionID,
					TraceID:    row.TraceID,
					ReceivedAt: row.ReceivedAt,
				},
			}
			items = append(items, item)
			index[item.EventID] = item
		}

		if item.PolicyReason == "" {
			item.PolicyReason = policyReason
		}
		if len(item.RiskFactors) == 0 && len(riskFactors) > 0 {
			item.RiskFactors = append(item.RiskFactors, riskFactors...)
		}
		if approval := sessionApprovalFromRow(row); approval != nil {
			item.Approval = approval
		}
		if execution != nil {
			item.Execution = execution
		}
		if pending := pendingExecutions[item.EventID]; pending != nil {
			item.Execution = pending
			delete(pendingExecutions, item.EventID)
		}
		item.Explain = buildSessionExplain(item)
	}

	out := make([]SessionTimelineEvent, 0, len(items))
	for _, item := range items {
		out = append(out, *item)
	}
	return out
}

func buildSessionExplain(item *SessionTimelineEvent) string {
	var parts []string
	actor := sessionActorLabel(item.UserID, item.UserName, item.UserEmail)
	switch {
	case actor != "" && item.AgentID != "":
		parts = append(parts, fmt.Sprintf("Requested by %s via %s.", actor, item.AgentID))
	case actor != "":
		parts = append(parts, fmt.Sprintf("Requested by %s.", actor))
	case item.AgentID != "":
		parts = append(parts, fmt.Sprintf("Requested via %s.", item.AgentID))
	}

	action := fmt.Sprintf("%s.%s", item.Tool, item.Action)
	switch item.Decision {
	case "allow":
		if item.PolicyReason != "" {
			parts = append(parts, fmt.Sprintf("%s was allowed because %s.", action, item.PolicyReason))
		} else {
			parts = append(parts, fmt.Sprintf("%s was allowed.", action))
		}
	case "deny":
		if item.PolicyReason != "" {
			parts = append(parts, fmt.Sprintf("%s was blocked because %s.", action, item.PolicyReason))
		} else {
			parts = append(parts, fmt.Sprintf("%s was blocked by policy.", action))
		}
	case "approve":
		if item.PolicyReason != "" {
			parts = append(parts, fmt.Sprintf("%s was sent for approval because %s.", action, item.PolicyReason))
		} else {
			parts = append(parts, fmt.Sprintf("%s was sent for approval.", action))
		}
	default:
		parts = append(parts, fmt.Sprintf("%s returned decision %s.", action, item.Decision))
	}

	if len(item.RiskFactors) > 0 {
		parts = append(parts, fmt.Sprintf("Risk factors: %s.", strings.Join(item.RiskFactors, ", ")))
	}
	if item.Approval != nil {
		approvalSummary := fmt.Sprintf("Approval %s is %s.", item.Approval.ID, item.Approval.Status)
		if item.Approval.Status == "denied" && item.Approval.DenyReason != "" {
			approvalSummary = fmt.Sprintf("Approval %s was denied: %s.", item.Approval.ID, item.Approval.DenyReason)
		}
		parts = append(parts, approvalSummary)
	}
	if item.Execution != nil {
		switch item.Execution.Status {
		case "success":
			parts = append(parts, "Execution finished successfully.")
		case "error", "timeout":
			if item.Execution.ErrorMsg != "" {
				parts = append(parts, fmt.Sprintf("Execution finished with %s: %s.", item.Execution.Status, item.Execution.ErrorMsg))
			} else {
				parts = append(parts, fmt.Sprintf("Execution finished with %s.", item.Execution.Status))
			}
		default:
			parts = append(parts, fmt.Sprintf("Execution status: %s.", item.Execution.Status))
		}
	}
	return strings.Join(parts, " ")
}

func sessionActorLabel(userID, userName, userEmail string) string {
	switch {
	case strings.TrimSpace(userName) != "" && strings.TrimSpace(userEmail) != "":
		return fmt.Sprintf("%s (%s)", userName, userEmail)
	case strings.TrimSpace(userName) != "":
		return userName
	case strings.TrimSpace(userEmail) != "":
		return userEmail
	case strings.TrimSpace(userID) != "":
		return userID
	default:
		return ""
	}
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
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
		return ErrAlertRuleNotFound
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
		query = `SELECT id, rule_id, tenant_id, severity, message, details, status, delivered_at, attempt_count, last_error, next_attempt_at, created_at
			FROM alert_events WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
		args = []any{tenantID, limit, offset}
	} else {
		query = `SELECT id, rule_id, tenant_id, severity, message, details, status, delivered_at, attempt_count, last_error, next_attempt_at, created_at
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
			&ae.Message, &ae.ContextJSON, &ae.Status, &ae.DeliveredAt, &ae.AttemptCount, &ae.LastError, &ae.NextAttemptAt, &ae.CreatedAt); err != nil {
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
		return ErrAlertRuleNotFound
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
		SELECT id, rule_id, tenant_id, severity, message, details, status, delivered_at, attempt_count, last_error, next_attempt_at, created_at
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
			&ae.Message, &ae.ContextJSON, &ae.Status, &ae.DeliveredAt, &ae.AttemptCount, &ae.LastError, &ae.NextAttemptAt, &ae.CreatedAt); err != nil {
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
