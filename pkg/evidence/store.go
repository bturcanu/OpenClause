package evidence

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/agenticaccess/governance/pkg/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store persists tool-call events and execution results in Postgres.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new evidence store backed by the given connection pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// ──────────────────────────────────────────────────────────────────────────────
// Write path
// ──────────────────────────────────────────────────────────────────────────────

// RecordEvent inserts a tool_event row and returns the computed hash.
func (s *Store) RecordEvent(ctx context.Context, env *types.ToolCallEnvelope) error {
	// Fetch the previous hash for this tenant.
	prevHash, err := s.lastHash(ctx, env.Request.TenantID)
	if err != nil {
		return fmt.Errorf("evidence.RecordEvent last hash: %w", err)
	}

	canonPayload, err := CanonicalJSON(env.Request)
	if err != nil {
		return fmt.Errorf("evidence.RecordEvent canonical: %w", err)
	}

	var canonResult []byte
	if env.ExecutionResult != nil {
		canonResult, err = CanonicalJSON(env.ExecutionResult)
		if err != nil {
			return fmt.Errorf("evidence.RecordEvent canonical result: %w", err)
		}
	}

	hash := ChainHash(prevHash, canonPayload, canonResult)
	env.Hash = hash
	env.PrevHash = prevHash
	env.PayloadCanon = canonPayload

	policyJSON, _ := json.Marshal(env.PolicyResult)

	_, err = s.pool.Exec(ctx, `
		INSERT INTO tool_events (
			event_id, tenant_id, agent_id, tool, action,
			payload_json, payload_canon,
			risk_score, decision, policy_result,
			idempotency_key, session_id, user_id, source_ip, trace_id,
			received_at, requested_at,
			hash, prev_hash
		) VALUES (
			$1,$2,$3,$4,$5,
			$6,$7,
			$8,$9,$10,
			$11,$12,$13,$14,$15,
			$16,$17,
			$18,$19
		)`,
		env.EventID, env.Request.TenantID, env.Request.AgentID,
		env.Request.Tool, env.Request.Action,
		env.PayloadJSON, canonPayload,
		env.Request.RiskScore, string(env.Decision), policyJSON,
		env.Request.IdempotencyKey, env.Request.SessionID, env.Request.UserID,
		env.Request.SourceIP, env.Request.TraceID,
		env.ReceivedAt, env.Request.RequestedAt,
		hash, prevHash,
	)
	if err != nil {
		return fmt.Errorf("evidence.RecordEvent insert: %w", err)
	}

	// If there is an execution result, record it too.
	if env.ExecutionResult != nil {
		_, err = s.pool.Exec(ctx, `
			INSERT INTO tool_results (event_id, tenant_id, status, output_json, error_msg, duration_ms, result_canon)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			env.EventID, env.Request.TenantID,
			env.ExecutionResult.Status, env.ExecutionResult.OutputJSON,
			env.ExecutionResult.Error, env.ExecutionResult.DurationMS, canonResult,
		)
		if err != nil {
			return fmt.Errorf("evidence.RecordEvent insert result: %w", err)
		}
	}

	return nil
}

// CheckIdempotency returns a prior response if one exists for (tenant, key).
func (s *Store) CheckIdempotency(ctx context.Context, tenantID, idempotencyKey string) (*types.ToolCallResponse, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT event_id, decision
		FROM tool_events
		WHERE tenant_id = $1 AND idempotency_key = $2
		LIMIT 1`, tenantID, idempotencyKey)

	var eventID string
	var decision string
	err := row.Scan(&eventID, &decision)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("evidence.CheckIdempotency: %w", err)
	}
	return &types.ToolCallResponse{
		EventID:  eventID,
		Decision: types.Decision(decision),
		Reason:   "idempotent replay",
	}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Read path
// ──────────────────────────────────────────────────────────────────────────────

// GetEvent retrieves a single event by ID.
func (s *Store) GetEvent(ctx context.Context, eventID string) (*types.ToolCallEnvelope, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT event_id, tenant_id, agent_id, tool, action,
		       payload_json, payload_canon, risk_score,
		       decision, policy_result,
		       idempotency_key, session_id, user_id, source_ip, trace_id,
		       received_at, requested_at, hash, prev_hash
		FROM tool_events WHERE event_id = $1`, eventID)

	var env types.ToolCallEnvelope
	var policyJSON []byte
	err := row.Scan(
		&env.EventID,
		&env.Request.TenantID, &env.Request.AgentID,
		&env.Request.Tool, &env.Request.Action,
		&env.PayloadJSON, &env.PayloadCanon, &env.Request.RiskScore,
		&env.Decision, &policyJSON,
		&env.Request.IdempotencyKey, &env.Request.SessionID,
		&env.Request.UserID, &env.Request.SourceIP, &env.Request.TraceID,
		&env.ReceivedAt, &env.Request.RequestedAt,
		&env.Hash, &env.PrevHash,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("evidence.GetEvent: %w", err)
	}
	if len(policyJSON) > 0 {
		env.PolicyResult = &types.PolicyResult{}
		_ = json.Unmarshal(policyJSON, env.PolicyResult)
	}
	return &env, nil
}

// GetChainEvents returns events for chain verification in chronological order.
func (s *Store) GetChainEvents(ctx context.Context, tenantID string, since time.Time) ([]ChainEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT e.event_id, e.hash, e.payload_canon, COALESCE(r.result_canon, ''::bytea)
		FROM tool_events e
		LEFT JOIN tool_results r ON r.event_id = e.event_id
		WHERE e.tenant_id = $1 AND e.received_at >= $2
		ORDER BY e.received_at ASC`, tenantID, since)
	if err != nil {
		return nil, fmt.Errorf("evidence.GetChainEvents: %w", err)
	}
	defer rows.Close()

	var events []ChainEvent
	for rows.Next() {
		var ev ChainEvent
		if err := rows.Scan(&ev.EventID, &ev.Hash, &ev.CanonPayload, &ev.CanonResult); err != nil {
			return nil, fmt.Errorf("evidence.GetChainEvents scan: %w", err)
		}
		events = append(events, ev)
	}
	return events, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────────────

func (s *Store) lastHash(ctx context.Context, tenantID string) (string, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT hash FROM tool_events
		WHERE tenant_id = $1
		ORDER BY received_at DESC LIMIT 1`, tenantID)

	var h string
	err := row.Scan(&h)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return h, err
}
