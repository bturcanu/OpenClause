// Package approvals provides the data model and handlers for the approvals service.
package approvals

import (
	"time"

	"github.com/bturcanu/OpenClause/pkg/types"
)

// ──────────────────────────────────────────────────────────────────────────────
// ApprovalRequest — created when policy says "approve".
// ──────────────────────────────────────────────────────────────────────────────

type ApprovalRequest struct {
	ID         string    `json:"id"`
	EventID    string    `json:"event_id"`
	TenantID   string    `json:"tenant_id"`
	AgentID    string    `json:"agent_id"`
	Tool       string    `json:"tool"`
	Action     string    `json:"action"`
	Resource   string    `json:"resource,omitempty"`
	RiskScore  int       `json:"risk_score"`
	Reason     string    `json:"reason"`
	DenyReason string    `json:"deny_reason,omitempty"`
	Status     string    `json:"status"` // "pending", "approved", "denied", "expired"
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// ──────────────────────────────────────────────────────────────────────────────
// ApprovalGrant — created when a human approves a request.
// ──────────────────────────────────────────────────────────────────────────────

type ApprovalGrant struct {
	ID        string        `json:"id"`
	RequestID string        `json:"request_id"`
	TenantID  string        `json:"tenant_id"`
	Approver  string        `json:"approver"`
	Scope     ApprovalScope `json:"scope"`
	MaxUses   int           `json:"max_uses"`
	UsesLeft  int           `json:"uses_left"`
	ExpiresAt time.Time     `json:"expires_at"`
	GrantedAt time.Time     `json:"granted_at"`
}

// ──────────────────────────────────────────────────────────────────────────────
// ApprovalScope — defines what the grant authorizes.
// ──────────────────────────────────────────────────────────────────────────────

type ApprovalScope struct {
	Tool            string `json:"tool"`             // exact or "*"
	Action          string `json:"action"`           // exact or "*"
	ResourcePattern string `json:"resource_pattern"` // glob pattern
	TenantID        string `json:"tenant_id"`
	AgentID         string `json:"agent_id,omitempty"` // optional restriction
}

// ──────────────────────────────────────────────────────────────────────────────
// API payloads
// ──────────────────────────────────────────────────────────────────────────────

type CreateApprovalInput struct {
	EventID         string               `json:"event_id"`
	TenantID        string               `json:"tenant_id"`
	AgentID         string               `json:"agent_id"`
	Tool            string               `json:"tool"`
	Action          string               `json:"action"`
	Resource        string               `json:"resource,omitempty"`
	RiskScore       int                  `json:"risk_score"`
	RiskFactors     []string             `json:"risk_factors,omitempty"`
	Reason          string               `json:"reason"`
	TraceID         string               `json:"trace_id,omitempty"`
	ApproverGroup   string               `json:"approver_group,omitempty"`
	Notify          []types.PolicyNotify `json:"notify,omitempty"`
	ApprovalBaseURL string               `json:"approval_base_url,omitempty"`
}

type GrantInput struct {
	Approver        string `json:"approver"`
	MaxUses         int    `json:"max_uses"`
	ExpiresInSec    int    `json:"expires_in_sec"` // seconds from now
	ResourcePattern string `json:"resource_pattern,omitempty"`
}

type DenyInput struct {
	Approver string `json:"approver"`
	Reason   string `json:"reason"`
}

type NotificationOutbox struct {
	ID                string
	ApprovalRequestID string
	TenantID          string
	EventID           string
	TraceID           string
	Tool              string
	Action            string
	Resource          string
	RiskScore         int
	RiskFactors       []string
	Reason            string
	ApprovalURL       string
	ApproverGroup     string
	NotifyKind        string
	NotifyURL         string
	SecretRef         string
	SlackChannel      string
	Attempts          int
	Status            string
	NextAttemptAt     time.Time
	CreatedAt         time.Time
}
