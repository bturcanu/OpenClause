package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/bturcanu/OpenClause/pkg/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

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

	env.Hash = hash
	env.PrevHash = prevHash
	env.PayloadCanon = canonPayload

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
		       received_at, requested_at, hash, prev_hash,
		       r.status, r.output_json, r.error_msg, r.duration_ms
		FROM tool_events e
		LEFT JOIN tool_results r ON r.event_id = e.event_id
		WHERE e.event_id = $1`, eventID)

	var env types.ToolCallEnvelope
	var tenantID, agentID, tool, action string
	var riskScore int
	var idempotencyKey, sessionID, userID, sourceIP, traceID string
	var requestedAt time.Time
	var policyJSON []byte
	var resultStatus *string
	var resultOutput []byte
	var resultError *string
	var resultDuration *int64
	err := row.Scan(
		&env.EventID,
		&tenantID, &agentID,
		&tool, &action,
		&env.PayloadJSON, &env.PayloadCanon, &riskScore,
		&env.Decision, &policyJSON,
		&idempotencyKey, &sessionID,
		&userID, &sourceIP, &traceID,
		&env.ReceivedAt, &requestedAt,
		&env.Hash, &env.PrevHash,
		&resultStatus, &resultOutput, &resultError, &resultDuration,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("evidence.GetEvent: %w", err)
	}
	// Rebuild the full original request from persisted payload_json so fields
	// such as params/resource/risk_factors are preserved for resume execution.
	if len(env.PayloadJSON) > 0 {
		if err := json.Unmarshal(env.PayloadJSON, &env.Request); err != nil {
			return nil, fmt.Errorf("evidence.GetEvent unmarshal payload: %w", err)
		}
	}
	// Overlay canonical DB columns as source-of-truth.
	env.Request.TenantID = tenantID
	env.Request.AgentID = agentID
	env.Request.Tool = tool
	env.Request.Action = action
	env.Request.RiskScore = riskScore
	env.Request.IdempotencyKey = idempotencyKey
	env.Request.SessionID = sessionID
	env.Request.UserID = userID
	env.Request.SourceIP = sourceIP
	env.Request.TraceID = traceID
	env.Request.RequestedAt = requestedAt

	if len(policyJSON) > 0 {
		env.PolicyResult = &types.PolicyResult{}
		if err := json.Unmarshal(policyJSON, env.PolicyResult); err != nil {
			return nil, fmt.Errorf("evidence.GetEvent unmarshal policy: %w", err)
		}
	}
	if resultStatus != nil {
		env.ExecutionResult = &types.ExecutionResult{
			Status: *resultStatus,
		}
		if len(resultOutput) > 0 {
			env.ExecutionResult.OutputJSON = resultOutput
		}
		if resultError != nil {
			env.ExecutionResult.Error = *resultError
		}
		if resultDuration != nil {
			env.ExecutionResult.DurationMS = *resultDuration
		}
	}
	return &env, nil
}

// GetExecutionByParentEvent returns the execution response for a previously
// resumed approval flow, if one exists.
func (s *Store) GetExecutionByParentEvent(ctx context.Context, parentEventID string) (*types.ToolCallResponse, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT e.event_id, e.decision, e.policy_result,
		       r.status, r.output_json, r.error_msg, r.duration_ms
		FROM tool_executions x
		JOIN tool_events e ON e.event_id = x.execution_event_id
		LEFT JOIN tool_results r ON r.event_id = e.event_id
		WHERE x.parent_event_id = $1`, parentEventID)

	var eventID string
	var decision types.Decision
	var policyJSON []byte
	var status *string
	var output []byte
	var errMsg *string
	var duration *int64

	err := row.Scan(&eventID, &decision, &policyJSON, &status, &output, &errMsg, &duration)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("evidence.GetExecutionByParentEvent: %w", err)
	}

	resp := &types.ToolCallResponse{
		EventID:  eventID,
		Decision: decision,
		Reason:   "idempotent execute replay",
	}
	if status != nil {
		resp.Result = &types.ExecutionResult{Status: *status}
		if len(output) > 0 {
			resp.Result.OutputJSON = output
		}
		if errMsg != nil {
			resp.Result.Error = *errMsg
		}
		if duration != nil {
			resp.Result.DurationMS = *duration
		}
	}
	return resp, nil
}

// LinkExecutionToParent stores the append-only relation between original
// approval-needed event and the execution event created by /execute.
// Returns (linked=true) when this call created the link, otherwise false if
// another concurrent request already linked it.
func (s *Store) LinkExecutionToParent(ctx context.Context, parentEventID, executionEventID, consumedGrantID string) (bool, error) {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO tool_executions(parent_event_id, execution_event_id, consumed_grant_id)
		VALUES ($1, $2, $3)`, parentEventID, executionEventID, consumedGrantID)
	if err == nil {
		return true, nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return false, nil
	}
	return false, fmt.Errorf("evidence.LinkExecutionToParent: %w", err)
}

// GetChainEvents returns events for chain verification in insertion order.
// The returned window starts strictly after afterSeq.
func (s *Store) GetChainEvents(ctx context.Context, tenantID string, afterSeq int64) ([]ChainEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT e.event_seq, e.event_id, e.prev_hash, e.hash, e.payload_canon, r.result_canon, e.received_at
		FROM tool_events e
		LEFT JOIN tool_results r ON r.event_id = e.event_id
		WHERE e.tenant_id = $1
		  AND e.event_seq > $2
		ORDER BY e.event_seq ASC`, tenantID, afterSeq)
	if err != nil {
		return nil, fmt.Errorf("evidence.GetChainEvents: %w", err)
	}
	defer rows.Close()

	var events []ChainEvent
	for rows.Next() {
		var ev ChainEvent
		if err := rows.Scan(&ev.EventSeq, &ev.EventID, &ev.PrevHash, &ev.Hash, &ev.CanonPayload, &ev.CanonResult, &ev.ReceivedAt); err != nil {
			return nil, fmt.Errorf("evidence.GetChainEvents scan: %w", err)
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("evidence.GetChainEvents iteration: %w", err)
	}
	return events, nil
}

// ListTenantIDs returns all tenant IDs known to the system.
func (s *Store) ListTenantIDs(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT id FROM tenants ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("evidence.ListTenantIDs: %w", err)
	}
	defer rows.Close()

	out := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("evidence.ListTenantIDs scan: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("evidence.ListTenantIDs iteration: %w", err)
	}
	return out, nil
}

// GetArchiveCheckpoint returns archival position for a tenant.
func (s *Store) GetArchiveCheckpoint(ctx context.Context, tenantID string) (time.Time, string, int64, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT last_archived_at, last_hash, last_event_seq
		FROM evidence_archive_checkpoints
		WHERE tenant_id = $1`, tenantID)
	var ts time.Time
	var h string
	var seq int64
	err := row.Scan(&ts, &h, &seq)
	if err == pgx.ErrNoRows {
		return time.Unix(0, 0).UTC(), "", 0, nil
	}
	if err != nil {
		return time.Time{}, "", 0, fmt.Errorf("evidence.GetArchiveCheckpoint: %w", err)
	}
	return ts, h, seq, nil
}

// UpsertArchiveCheckpoint advances archival position after successful upload.
func (s *Store) UpsertArchiveCheckpoint(ctx context.Context, tenantID string, archivedAt time.Time, hash string, seq int64) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO evidence_archive_checkpoints(tenant_id, last_archived_at, last_hash, last_event_seq, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (tenant_id) DO UPDATE
		SET last_archived_at = EXCLUDED.last_archived_at,
		    last_hash = EXCLUDED.last_hash,
		    last_event_seq = EXCLUDED.last_event_seq,
		    updated_at = NOW()`,
		tenantID, archivedAt, hash, seq,
	)
	if err != nil {
		return fmt.Errorf("evidence.UpsertArchiveCheckpoint: %w", err)
	}
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────────────

// lastHashTx fetches the latest hash for a tenant inside an existing transaction.
func (s *Store) lastHashTx(ctx context.Context, tx pgx.Tx, tenantID string) (string, error) {
	row := tx.QueryRow(ctx, `
		SELECT hash FROM tool_events
		WHERE tenant_id = $1
		ORDER BY event_seq DESC LIMIT 1`, tenantID)

	var h string
	err := row.Scan(&h)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return h, err
}

const evidenceLockNamespace = 0x4F43_4556 // "OCEV" — OpenClause evidence

// tenantLockID produces a deterministic int64 advisory-lock ID from a tenant string.
func tenantLockID(tenantID string) int64 {
	h := fnv.New32a()
	h.Write([]byte(tenantID))
	return int64(evidenceLockNamespace)<<32 | int64(h.Sum32())
}
