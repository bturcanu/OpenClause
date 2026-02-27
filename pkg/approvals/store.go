package approvals

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
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

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("approvals.CreateRequest begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	_, err = tx.Exec(ctx, `
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
		return nil, fmt.Errorf("approvals.CreateRequest insert request: %w", err)
	}

	approvalURL := buildApprovalURL(in.ApprovalBaseURL, req.ID)
	riskFactorsJSON, err := json.Marshal(in.RiskFactors)
	if err != nil {
		return nil, fmt.Errorf("approvals.CreateRequest marshal risk factors: %w", err)
	}

	for _, n := range in.Notify {
		if n.Kind == "" {
			continue
		}
		outboxID := uuid.NewString()
		_, err = tx.Exec(ctx, `
			INSERT INTO approval_notification_outbox (
				id, approval_request_id, tenant_id, event_id, trace_id, tool, action, resource,
				risk_score, risk_factors, reason, approver_group, approval_url,
				notify_kind, notify_url, secret_ref, slack_channel,
				status, attempt_count, next_attempt_at, created_at, updated_at
			) VALUES (
				$1,$2,$3,$4,$5,$6,$7,$8,
				$9,$10,$11,$12,$13,
				$14,$15,$16,$17,
				'pending',0,NOW(),NOW(),NOW()
			)`,
			outboxID, req.ID, req.TenantID, req.EventID, in.TraceID, req.Tool, req.Action, req.Resource,
			req.RiskScore, riskFactorsJSON, req.Reason, in.ApproverGroup, approvalURL,
			n.Kind, n.URL, n.SecretRef, n.Channel,
		)
		if err != nil {
			return nil, fmt.Errorf("approvals.CreateRequest insert outbox: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("approvals.CreateRequest commit: %w", err)
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
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

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
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

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

// ClaimDueNotifications claims pending due rows for delivery using row-level
// locking so concurrent workers cannot deliver the same ID twice.
func (s *Store) ClaimDueNotifications(ctx context.Context, limit int) ([]NotificationOutbox, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		WITH due AS (
			SELECT id
			FROM approval_notification_outbox
			WHERE status = 'pending'
			  AND next_attempt_at <= NOW()
			ORDER BY created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE approval_notification_outbox o
		SET status = 'processing',
		    attempt_count = o.attempt_count + 1,
		    updated_at = NOW()
		FROM due
		WHERE o.id = due.id
		RETURNING o.id, o.approval_request_id, o.tenant_id, o.event_id, o.trace_id, o.tool, o.action, o.resource,
		          o.risk_score, o.risk_factors, o.reason, o.approver_group, o.approval_url,
		          o.notify_kind, o.notify_url, o.secret_ref, o.slack_channel,
		          o.attempt_count, o.status, o.next_attempt_at, o.created_at`, limit)
	if err != nil {
		return nil, fmt.Errorf("approvals.ClaimDueNotifications: %w", err)
	}
	defer rows.Close()

	out := make([]NotificationOutbox, 0)
	for rows.Next() {
		var n NotificationOutbox
		var riskFactors []byte
		if err := rows.Scan(
			&n.ID, &n.ApprovalRequestID, &n.TenantID, &n.EventID, &n.TraceID,
			&n.Tool, &n.Action, &n.Resource, &n.RiskScore, &riskFactors,
			&n.Reason, &n.ApproverGroup, &n.ApprovalURL,
			&n.NotifyKind, &n.NotifyURL, &n.SecretRef, &n.SlackChannel,
			&n.Attempts, &n.Status, &n.NextAttemptAt, &n.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("approvals.ClaimDueNotifications scan: %w", err)
		}
		if len(riskFactors) > 0 {
			if err := json.Unmarshal(riskFactors, &n.RiskFactors); err != nil {
				return nil, fmt.Errorf("approvals.ClaimDueNotifications unmarshal risk factors: %w", err)
			}
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("approvals.ClaimDueNotifications iteration: %w", err)
	}
	return out, nil
}

// MarkNotificationSent marks an outbox record as delivered.
func (s *Store) MarkNotificationSent(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE approval_notification_outbox
		SET status = 'sent', sent_at = NOW(), updated_at = NOW(), last_error = ''
		WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("approvals.MarkNotificationSent: %w", err)
	}
	return nil
}

// MarkNotificationRetry schedules another delivery attempt with backoff.
func (s *Store) MarkNotificationRetry(ctx context.Context, id string, attempts int, nextAttemptAt time.Time, lastErr string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE approval_notification_outbox
		SET status = 'pending', attempt_count = $2, next_attempt_at = $3, last_error = $4, updated_at = NOW()
		WHERE id = $1`, id, attempts, nextAttemptAt, lastErr)
	if err != nil {
		return fmt.Errorf("approvals.MarkNotificationRetry: %w", err)
	}
	return nil
}

// MarkNotificationFailed marks an outbox row terminally failed.
func (s *Store) MarkNotificationFailed(ctx context.Context, id string, lastErr string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE approval_notification_outbox
		SET status = 'failed', last_error = $2, updated_at = NOW()
		WHERE id = $1`, id, lastErr)
	if err != nil {
		return fmt.Errorf("approvals.MarkNotificationFailed: %w", err)
	}
	return nil
}

func buildApprovalURL(baseURL, requestID string) string {
	base := strings.TrimRight(baseURL, "/")
	if base == "" {
		base = "http://localhost:8081"
	}
	return fmt.Sprintf("%s/v1/approvals/requests/%s", base, requestID)
}
