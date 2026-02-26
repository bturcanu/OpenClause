package approvals

import (
	"context"
	"fmt"
	"path"
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
	if in.TenantID == "" || in.EventID == "" || in.Tool == "" || in.Action == "" {
		return nil, fmt.Errorf("approvals.CreateRequest: tenant_id, event_id, tool, and action are required")
	}

	now := time.Now().UTC()
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
		CreatedAt: now,
		ExpiresAt: now.Add(24 * time.Hour),
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
		       risk_score, reason, deny_reason, status, created_at, expires_at
		FROM approval_requests WHERE id = $1`, id)

	r := &ApprovalRequest{}
	err := row.Scan(
		&r.ID, &r.EventID, &r.TenantID, &r.AgentID,
		&r.Tool, &r.Action, &r.Resource,
		&r.RiskScore, &r.Reason, &r.DenyReason, &r.Status,
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

const defaultPendingLimit = 200

// ListPending returns pending requests for a tenant (paginated).
func (s *Store) ListPending(ctx context.Context, tenantID string, limit, offset int) ([]ApprovalRequest, error) {
	if limit <= 0 || limit > defaultPendingLimit {
		limit = defaultPendingLimit
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, event_id, tenant_id, agent_id, tool, action, resource,
		       risk_score, reason, deny_reason, status, created_at, expires_at
		FROM approval_requests
		WHERE tenant_id = $1 AND status = 'pending' AND expires_at > NOW()
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("approvals.ListPending: %w", err)
	}
	defer rows.Close()

	reqs := make([]ApprovalRequest, 0)
	for rows.Next() {
		var r ApprovalRequest
		if err := rows.Scan(
			&r.ID, &r.EventID, &r.TenantID, &r.AgentID,
			&r.Tool, &r.Action, &r.Resource,
			&r.RiskScore, &r.Reason, &r.DenyReason, &r.Status,
			&r.CreatedAt, &r.ExpiresAt,
		); err != nil {
			return nil, fmt.Errorf("approvals.ListPending scan: %w", err)
		}
		reqs = append(reqs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("approvals.ListPending iteration: %w", err)
	}
	return reqs, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Approval Grants
// ──────────────────────────────────────────────────────────────────────────────

// GrantRequest approves a pending request, creating a grant.
// The status check is performed inside the transaction to eliminate TOCTOU races.
func (s *Store) GrantRequest(ctx context.Context, requestID string, in GrantInput) (*ApprovalGrant, error) {
	if in.Approver == "" {
		return nil, fmt.Errorf("approvals.GrantRequest: approver is required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("approvals.GrantRequest begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Lock and check status atomically inside the transaction.
	res, err := tx.Exec(ctx, `
		UPDATE approval_requests SET status = 'approved', updated_at = NOW()
		WHERE id = $1 AND status = 'pending'`, requestID)
	if err != nil {
		return nil, fmt.Errorf("approvals.GrantRequest update: %w", err)
	}
	if res.RowsAffected() == 0 {
		return nil, fmt.Errorf("approval request %s not found or not pending", requestID)
	}

	// Fetch the request details for the grant scope.
	row := tx.QueryRow(ctx, `
		SELECT tenant_id, agent_id, tool, action, resource
		FROM approval_requests WHERE id = $1`, requestID)
	var tenantID, agentID, tool, action, resource string
	if err := row.Scan(&tenantID, &agentID, &tool, &action, &resource); err != nil {
		return nil, fmt.Errorf("approvals.GrantRequest fetch: %w", err)
	}

	maxUses := in.MaxUses
	if maxUses <= 0 {
		maxUses = 1
	}
	now := time.Now().UTC()
	expiry := now.Add(1 * time.Hour)
	if in.ExpiresInSec > 0 {
		expiry = now.Add(time.Duration(in.ExpiresInSec) * time.Second)
	}

	resourcePattern := in.ResourcePattern
	if resourcePattern == "" {
		resourcePattern = resource
	}

	grant := &ApprovalGrant{
		ID:        uuid.NewString(),
		RequestID: requestID,
		TenantID:  tenantID,
		Approver:  in.Approver,
		Scope: ApprovalScope{
			Tool:            tool,
			Action:          action,
			ResourcePattern: resourcePattern,
			TenantID:        tenantID,
			AgentID:         agentID,
		},
		MaxUses:   maxUses,
		UsesLeft:  maxUses,
		ExpiresAt: expiry,
		GrantedAt: now,
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
// The original reason is preserved; deny_reason stores the denier's rationale.
func (s *Store) DenyRequest(ctx context.Context, requestID string, in DenyInput) error {
	if in.Approver == "" {
		return fmt.Errorf("approvals.DenyRequest: approver is required")
	}
	res, err := s.pool.Exec(ctx, `
		UPDATE approval_requests SET status = 'denied', deny_reason = $2, updated_at = NOW()
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
// decrements its usage. Iterates through all candidates (not just LIMIT 1) to
// ensure resource-pattern mismatches don't hide valid grants.
func (s *Store) FindAndConsumeGrant(ctx context.Context, tenantID, agentID, tool, action, resource string) (*ApprovalGrant, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("approvals.FindAndConsumeGrant begin: %w", err)
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
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
		FOR UPDATE`, tenantID, tool, action, agentID)
	if err != nil {
		return nil, fmt.Errorf("approvals.FindAndConsumeGrant query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		g := &ApprovalGrant{}
		if err := rows.Scan(
			&g.ID, &g.RequestID, &g.TenantID, &g.Approver,
			&g.Scope.Tool, &g.Scope.Action, &g.Scope.ResourcePattern,
			&g.Scope.TenantID, &g.Scope.AgentID,
			&g.MaxUses, &g.UsesLeft, &g.ExpiresAt, &g.GrantedAt,
		); err != nil {
			return nil, fmt.Errorf("approvals.FindAndConsumeGrant scan: %w", err)
		}

		if !matchResource(g.Scope.ResourcePattern, resource) {
			continue
		}

		// Close the rows cursor before executing the UPDATE in the same tx.
		rows.Close()

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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("approvals.FindAndConsumeGrant iteration: %w", err)
	}

	return nil, nil
}

// matchResource checks whether a resource matches a grant's resource pattern.
// Uses path.Match which is OS-independent (unlike filepath.Match).
// Empty or "*" patterns match everything.
func matchResource(pattern, resource string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	matched, err := path.Match(pattern, resource)
	if err != nil {
		return false
	}
	return matched
}
