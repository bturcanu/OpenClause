// Package types defines the canonical tool-call schema used across all services.
package types

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// Limits
// ──────────────────────────────────────────────────────────────────────────────

const (
	MaxParamsBytes         = 64 * 1024 // 64 KB
	MaxResourceBytes       = 2 * 1024  // 2 KB
	MaxIdempotencyKeyBytes = 256
	MaxLabelsCount         = 50
	MaxRiskScore           = 10
	CurrentSchemaVer       = "1.0"
)

// ──────────────────────────────────────────────────────────────────────────────
// ToolCallRequest — the payload sent by an AI agent.
// ──────────────────────────────────────────────────────────────────────────────

type ToolCallRequest struct {
	// Identity
	TenantID string `json:"tenant_id"`
	AgentID  string `json:"agent_id"`

	// Action
	Tool   string `json:"tool"`
	Action string `json:"action"`

	// Inputs
	Params json.RawMessage `json:"params,omitempty"`

	// Target
	Resource string `json:"resource,omitempty"`

	// Risk
	RiskScore   int      `json:"risk_score"`
	RiskFactors []string `json:"risk_factors,omitempty"`

	// Metadata
	UserID    string            `json:"user_id,omitempty"`
	SessionID string            `json:"session_id,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	SourceIP  string            `json:"source_ip,omitempty"`
	TraceID   string            `json:"trace_id,omitempty"`

	// Control
	IdempotencyKey string    `json:"idempotency_key"`
	RequestedAt    time.Time `json:"requested_at"`
	SchemaVersion  string    `json:"schema_version"`
}

// Normalize lowercases tool/action and ensures dotted format.
func (r *ToolCallRequest) Normalize() {
	r.Tool = strings.ToLower(strings.TrimSpace(r.Tool))
	r.Action = strings.ToLower(strings.TrimSpace(r.Action))
}

// Validate enforces all invariants on the request. Also normalizes tool/action.
func (r *ToolCallRequest) Validate() error {
	r.Normalize()

	if r.TenantID == "" {
		return &ValidationError{Field: "tenant_id", Reason: "required"}
	}
	if r.AgentID == "" {
		return &ValidationError{Field: "agent_id", Reason: "required"}
	}
	if r.Tool == "" {
		return &ValidationError{Field: "tool", Reason: "required"}
	}
	if r.Action == "" {
		return &ValidationError{Field: "action", Reason: "required"}
	}
	if r.IdempotencyKey == "" {
		return &ValidationError{Field: "idempotency_key", Reason: "required"}
	}
	if len(r.IdempotencyKey) > MaxIdempotencyKeyBytes {
		return &ValidationError{Field: "idempotency_key", Reason: fmt.Sprintf("exceeds %d bytes", MaxIdempotencyKeyBytes)}
	}
	if r.RiskScore < 0 || r.RiskScore > MaxRiskScore {
		return &ValidationError{Field: "risk_score", Reason: fmt.Sprintf("must be 0–%d", MaxRiskScore)}
	}
	if len(r.Params) > MaxParamsBytes {
		return &ValidationError{Field: "params", Reason: fmt.Sprintf("exceeds %d bytes", MaxParamsBytes)}
	}
	if len(r.Resource) > MaxResourceBytes {
		return &ValidationError{Field: "resource", Reason: fmt.Sprintf("exceeds %d bytes", MaxResourceBytes)}
	}
	if len(r.Labels) > MaxLabelsCount {
		return &ValidationError{Field: "labels", Reason: fmt.Sprintf("exceeds %d entries", MaxLabelsCount)}
	}
	if r.SchemaVersion == "" {
		r.SchemaVersion = CurrentSchemaVer
	} else if r.SchemaVersion != CurrentSchemaVer {
		return &ValidationError{Field: "schema_version", Reason: fmt.Sprintf("unsupported version %q, expected %q", r.SchemaVersion, CurrentSchemaVer)}
	}
	if r.RequestedAt.IsZero() {
		r.RequestedAt = time.Now().UTC()
	}
	return nil
}

// ToolAction returns the combined "tool.action" string.
func (r *ToolCallRequest) ToolAction() string {
	return r.Tool + "." + r.Action
}

// ──────────────────────────────────────────────────────────────────────────────
// ToolCallEnvelope — wraps a request with IDs, timestamps, hashes.
// ──────────────────────────────────────────────────────────────────────────────

type ToolCallEnvelope struct {
	EventID      string          `json:"event_id"`
	Request      ToolCallRequest `json:"request"`
	PayloadJSON  json.RawMessage `json:"payload_json"`
	PayloadCanon []byte          `json:"payload_canon"`
	ReceivedAt   time.Time       `json:"received_at"`

	Decision     Decision      `json:"decision"`
	PolicyResult *PolicyResult `json:"policy_result,omitempty"`

	ExecutionResult *ExecutionResult `json:"execution_result,omitempty"`

	Hash     string `json:"hash"`
	PrevHash string `json:"prev_hash"`
}

// ──────────────────────────────────────────────────────────────────────────────
// Policy I/O
// ──────────────────────────────────────────────────────────────────────────────

type Decision string

const (
	DecisionAllow   Decision = "allow"
	DecisionDeny    Decision = "deny"
	DecisionApprove Decision = "approve"
)

// PolicyInput is sent to OPA for evaluation.
type PolicyInput struct {
	ToolCall    ToolCallRequest   `json:"toolcall"`
	Environment PolicyEnvironment `json:"environment"`
}

type PolicyEnvironment struct {
	Timestamp    time.Time         `json:"timestamp"`
	TenantConfig map[string]string `json:"tenant_config,omitempty"`
}

// PolicyResult is what OPA returns.
type PolicyResult struct {
	Decision      Decision          `json:"decision"`
	Reason        string            `json:"reason"`
	Requirements  map[string]string `json:"requirements,omitempty"`
	RiskOverrides map[string]int    `json:"risk_overrides,omitempty"`
}

// ──────────────────────────────────────────────────────────────────────────────
// Execution result
// ──────────────────────────────────────────────────────────────────────────────

type ExecutionResult struct {
	Status     string          `json:"status"` // "success", "error", "timeout"
	OutputJSON json.RawMessage `json:"output_json,omitempty"`
	Error      string          `json:"error,omitempty"`
	DurationMS int64           `json:"duration_ms"`
}

// ──────────────────────────────────────────────────────────────────────────────
// API response
// ──────────────────────────────────────────────────────────────────────────────

type ToolCallResponse struct {
	EventID     string           `json:"event_id"`
	Decision    Decision         `json:"decision"`
	Reason      string           `json:"reason,omitempty"`
	ApprovalURL string           `json:"approval_url,omitempty"`
	Result      *ExecutionResult `json:"result,omitempty"`
}
