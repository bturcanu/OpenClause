package evidence

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/bturcanu/OpenClause/pkg/types"
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

// RecordEvent inserts a tool_event row (and optional tool_result) atomically
// within a single transaction. A per-tenant advisory lock serialises hash-chain
// appends so concurrent writers cannot fork the chain.
func (s *Store) RecordEvent(ctx context.Context, env *types.ToolCallEnvelope) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("evidence.RecordEvent begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Per-tenant advisory lock to serialise chain appends.
	lockID := tenantLockID(env.Request.TenantID)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockID); err != nil {
		return fmt.Errorf("evidence.RecordEvent advisory lock: %w", err)
	}

	prevHash, err := s.lastHashTx(ctx, tx, env.Request.TenantID)
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

	policyJSON, err := json.Marshal(env.PolicyResult)
	if err != nil {
		return fmt.Errorf("evidence.RecordEvent marshal policy: %w", err)
	}

	_, err = tx.Exec(ctx, `
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
		return fmt.Errorf("evidence.RecordEvent insert event: %w", err)
	}

	if env.ExecutionResult != nil {
		_, err = tx.Exec(ctx, `
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

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("evidence.RecordEvent commit: %w", err)
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
		if err := json.Unmarshal(policyJSON, env.PolicyResult); err != nil {
			return nil, fmt.Errorf("evidence.GetEvent unmarshal policy: %w", err)
		}
	}
	return &env, nil
}

// GetChainEvents returns events for chain verification in chronological order.
func (s *Store) GetChainEvents(ctx context.Context, tenantID string, since time.Time) ([]ChainEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT e.event_id, e.hash, e.payload_canon, r.result_canon
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("evidence.GetChainEvents iteration: %w", err)
	}
	return events, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────────────

// lastHashTx fetches the latest hash for a tenant inside an existing transaction.
func (s *Store) lastHashTx(ctx context.Context, tx pgx.Tx, tenantID string) (string, error) {
	row := tx.QueryRow(ctx, `
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

// tenantLockID produces a deterministic int64 advisory-lock ID from a tenant string.
func tenantLockID(tenantID string) int64 {
	h := fnv.New64a()
	h.Write([]byte(tenantID))
	b := h.Sum(nil)
	return int64(binary.BigEndian.Uint64(b))
}
