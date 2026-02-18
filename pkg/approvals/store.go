package approvals

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store manages approval requests and grants in Postgres.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new approvals store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// ──────────────────────────────────────────────────────────────────────────────
// Approval Requests
// ──────────────────────────────────────────────────────────────────────────────

// CreateRequest inserts a new pending approval request.
func (s *Store) CreateRequest(ctx context.Context, in CreateApprovalInput) (*ApprovalRequest, error) {
	req := &ApprovalRequest{
		ID:        uuid.NewString(),
		EventID:   in.EventID,
		TenantID:  in.TenantID,
		AgentID:   in.AgentID,
		Tool:      in.Tool,
		Action:    in.Action,
		Resource:  in.Resource,
		RiskScore: in.RiskScore,
		Reason:    in.Reason,
		Status:    "pending",
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour), // default: 24 h
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO approval_requests (
			id, event_id, tenant_id, agent_id, tool, action, resource,
			risk_score, reason, status, created_at, expires_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		req.ID, req.EventID, req.TenantID, req.AgentID,
		req.Tool, req.Action, req.Resource,
		req.RiskScore, req.Reason, req.Status,
		req.CreatedAt, req.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("approvals.CreateRequest: %w", err)
	}
	return req, nil
}

// GetRequest fetches a single approval request.
func (s *Store) GetRequest(ctx context.Context, id string) (*ApprovalRequest, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, event_id, tenant_id, agent_id, tool, action, resource,
		       risk_score, reason, status, created_at, expires_at
		FROM approval_requests WHERE id = $1`, id)

	r := &ApprovalRequest{}
	err := row.Scan(
		&r.ID, &r.EventID, &r.TenantID, &r.AgentID,
		&r.Tool, &r.Action, &r.Resource,
		&r.RiskScore, &r.Reason, &r.Status,
		&r.CreatedAt, &r.ExpiresAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("approvals.GetRequest: %w", err)
	}
	return r, nil
}

// ListPending returns all pending requests for a tenant.
func (s *Store) ListPending(ctx context.Context, tenantID string) ([]ApprovalRequest, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, event_id, tenant_id, agent_id, tool, action, resource,
		       risk_score, reason, status, created_at, expires_at
		FROM approval_requests
		WHERE tenant_id = $1 AND status = 'pending' AND expires_at > NOW()
		ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("approvals.ListPending: %w", err)
	}
	defer rows.Close()

	var reqs []ApprovalRequest
	for rows.Next() {
		var r ApprovalRequest
		if err := rows.Scan(
			&r.ID, &r.EventID, &r.TenantID, &r.AgentID,
			&r.Tool, &r.Action, &r.Resource,
			&r.RiskScore, &r.Reason, &r.Status,
			&r.CreatedAt, &r.ExpiresAt,
		); err != nil {
			return nil, fmt.Errorf("approvals.ListPending scan: %w", err)
		}
		reqs = append(reqs, r)
	}
	return reqs, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Approval Grants
// ──────────────────────────────────────────────────────────────────────────────

// GrantRequest approves a pending request, creating a grant.
func (s *Store) GrantRequest(ctx context.Context, requestID string, in GrantInput) (*ApprovalGrant, error) {
	req, err := s.GetRequest(ctx, requestID)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("approval request %s not found", requestID)
	}
	if req.Status != "pending" {
		return nil, fmt.Errorf("approval request %s is %s, not pending", requestID, req.Status)
	}

	maxUses := in.MaxUses
	if maxUses <= 0 {
		maxUses = 1
	}
	expiry := time.Now().UTC().Add(time.Duration(in.ExpiresInSec) * time.Second)
	if in.ExpiresInSec <= 0 {
		expiry = time.Now().UTC().Add(1 * time.Hour) // default 1 h
	}

	resourcePattern := in.ResourcePattern
	if resourcePattern == "" {
		resourcePattern = req.Resource
	}

	grant := &ApprovalGrant{
		ID:        uuid.NewString(),
		RequestID: requestID,
		TenantID:  req.TenantID,
		Approver:  in.Approver,
		Scope: ApprovalScope{
			Tool:            req.Tool,
			Action:          req.Action,
			ResourcePattern: resourcePattern,
			TenantID:        req.TenantID,
			AgentID:         req.AgentID,
		},
		MaxUses:   maxUses,
		UsesLeft:  maxUses,
		ExpiresAt: expiry,
		GrantedAt: time.Now().UTC(),
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("approvals.GrantRequest begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		UPDATE approval_requests SET status = 'approved' WHERE id = $1`, requestID)
	if err != nil {
		return nil, fmt.Errorf("approvals.GrantRequest update: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO approval_grants (
			id, request_id, tenant_id, approver,
			scope_tool, scope_action, scope_resource_pattern, scope_tenant_id, scope_agent_id,
			max_uses, uses_left, expires_at, granted_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		grant.ID, grant.RequestID, grant.TenantID, grant.Approver,
		grant.Scope.Tool, grant.Scope.Action, grant.Scope.ResourcePattern,
		grant.Scope.TenantID, grant.Scope.AgentID,
		grant.MaxUses, grant.UsesLeft, grant.ExpiresAt, grant.GrantedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("approvals.GrantRequest insert: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("approvals.GrantRequest commit: %w", err)
	}

	return grant, nil
}

// DenyRequest marks a pending request as denied.
func (s *Store) DenyRequest(ctx context.Context, requestID string, in DenyInput) error {
	res, err := s.pool.Exec(ctx, `
		UPDATE approval_requests SET status = 'denied', reason = $2
		WHERE id = $1 AND status = 'pending'`, requestID, in.Reason)
	if err != nil {
		return fmt.Errorf("approvals.DenyRequest: %w", err)
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("approval request %s not found or not pending", requestID)
	}
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Grant consumption (called by gateway)
// ──────────────────────────────────────────────────────────────────────────────

// FindAndConsumeGrant finds a valid grant matching the given scope and atomically
// decrements its usage. Returns the grant or nil.
func (s *Store) FindAndConsumeGrant(ctx context.Context, tenantID, agentID, tool, action, resource string) (*ApprovalGrant, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("approvals.FindAndConsumeGrant begin: %w", err)
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
		SELECT id, request_id, tenant_id, approver,
		       scope_tool, scope_action, scope_resource_pattern, scope_tenant_id, scope_agent_id,
		       max_uses, uses_left, expires_at, granted_at
		FROM approval_grants
		WHERE tenant_id = $1
		  AND uses_left > 0
		  AND expires_at > NOW()
		  AND (scope_tool = $2 OR scope_tool = '*')
		  AND (scope_action = $3 OR scope_action = '*')
		  AND (scope_agent_id = '' OR scope_agent_id = $4)
		ORDER BY granted_at DESC
		LIMIT 1
		FOR UPDATE`, tenantID, tool, action, agentID)

	g := &ApprovalGrant{}
	err = row.Scan(
		&g.ID, &g.RequestID, &g.TenantID, &g.Approver,
		&g.Scope.Tool, &g.Scope.Action, &g.Scope.ResourcePattern,
		&g.Scope.TenantID, &g.Scope.AgentID,
		&g.MaxUses, &g.UsesLeft, &g.ExpiresAt, &g.GrantedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("approvals.FindAndConsumeGrant select: %w", err)
	}

	// Check resource pattern if specified
	if g.Scope.ResourcePattern != "" && g.Scope.ResourcePattern != "*" {
		matched, _ := filepath.Match(g.Scope.ResourcePattern, resource)
		if !matched && !strings.Contains(resource, g.Scope.ResourcePattern) {
			return nil, nil
		}
	}

	// Atomically decrement
	_, err = tx.Exec(ctx, `
		UPDATE approval_grants SET uses_left = uses_left - 1 WHERE id = $1`, g.ID)
	if err != nil {
		return nil, fmt.Errorf("approvals.FindAndConsumeGrant update: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("approvals.FindAndConsumeGrant commit: %w", err)
	}

	g.UsesLeft--
	return g, nil
}
