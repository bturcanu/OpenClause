package evidence

import (
	"context"
	"log/slog"

	"github.com/agenticaccess/governance/pkg/types"
)

// Logger wraps the Store and emits structured logs alongside DB writes.
type Logger struct {
	store *Store
	log   *slog.Logger
}

// NewLogger creates an evidence logger backed by the given store.
func NewLogger(store *Store, log *slog.Logger) *Logger {
	return &Logger{store: store, log: log}
}

// RecordEvent persists and logs the event.
func (l *Logger) RecordEvent(ctx context.Context, env *types.ToolCallEnvelope) error {
	if err := l.store.RecordEvent(ctx, env); err != nil {
		l.log.ErrorContext(ctx, "evidence record failed",
			"event_id", env.EventID,
			"tenant_id", env.Request.TenantID,
			"error", err,
		)
		return err
	}

	l.log.InfoContext(ctx, "tool_event recorded",
		"event_id", env.EventID,
		"tenant_id", env.Request.TenantID,
		"agent_id", env.Request.AgentID,
		"tool", env.Request.Tool,
		"action", env.Request.Action,
		"decision", string(env.Decision),
		"risk_score", env.Request.RiskScore,
		"hash", env.Hash,
	)
	return nil
}

// CheckIdempotency delegates to the store.
func (l *Logger) CheckIdempotency(ctx context.Context, tenantID, key string) (*types.ToolCallResponse, error) {
	resp, err := l.store.CheckIdempotency(ctx, tenantID, key)
	if err != nil {
		return nil, err
	}
	if resp != nil {
		l.log.InfoContext(ctx, "idempotency hit",
			"tenant_id", tenantID,
			"idempotency_key", key,
			"event_id", resp.EventID,
		)
	}
	return resp, nil
}

// GetEvent delegates to the store.
func (l *Logger) GetEvent(ctx context.Context, eventID string) (*types.ToolCallEnvelope, error) {
	return l.store.GetEvent(ctx, eventID)
}
