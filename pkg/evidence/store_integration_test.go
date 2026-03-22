package evidence

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/bturcanu/OpenClause/internal/testdb"
	"github.com/bturcanu/OpenClause/pkg/approvals"
	"github.com/bturcanu/OpenClause/pkg/console"
	"github.com/bturcanu/OpenClause/pkg/types"
)

type evidenceStores struct {
	store     *Store
	console   *console.Store
	approvals *approvals.Store
	ctx       context.Context
}

func newEvidenceStores(t *testing.T) *evidenceStores {
	t.Helper()
	h := testdb.New(t)
	return &evidenceStores{
		store:     NewStore(h.Pool()),
		console:   console.NewStore(h.Pool()),
		approvals: approvals.NewStore(h.Pool()),
		ctx:       context.Background(),
	}
}

func createTenantForEvidence(t *testing.T, stores *evidenceStores, name string) *console.Tenant {
	t.Helper()
	tenant, err := stores.console.CreateTenant(stores.ctx, name, nil)
	if err != nil {
		t.Fatalf("CreateTenant(%q): %v", name, err)
	}
	return tenant
}

func mustRecordEvent(t *testing.T, store *Store, ctx context.Context, env *types.ToolCallEnvelope) {
	t.Helper()
	if env.PayloadJSON == nil {
		b, err := json.Marshal(env.Request)
		if err != nil {
			t.Fatalf("json.Marshal payload: %v", err)
		}
		env.PayloadJSON = b
	}
	if err := store.RecordEvent(ctx, env); err != nil {
		t.Fatalf("RecordEvent(%s): %v", env.EventID, err)
	}
}

func TestStoreRecordEventMaintainsHashChainAndRequestFields(t *testing.T) {
	stores := newEvidenceStores(t)
	tenant := createTenantForEvidence(t, stores, "Evidence Tenant")

	first := &types.ToolCallEnvelope{
		EventID: "11111111-1111-1111-1111-111111111111",
		Request: types.ToolCallRequest{
			TenantID:       tenant.ID,
			AgentID:        "agent-1",
			Tool:           "slack",
			Action:         "msg.post",
			Resource:       "channel/general",
			IdempotencyKey: "chain-one",
			SessionID:      "session-1",
			TraceID:        "trace-1",
			RequestedAt:    time.Unix(100, 0).UTC(),
		},
		ReceivedAt:   time.Unix(100, 0).UTC(),
		Decision:     types.DecisionDeny,
		PolicyResult: &types.PolicyResult{Decision: types.DecisionDeny, Reason: "blocked"},
	}
	second := &types.ToolCallEnvelope{
		EventID: "22222222-2222-2222-2222-222222222222",
		Request: types.ToolCallRequest{
			TenantID:       tenant.ID,
			AgentID:        "agent-1",
			Tool:           "slack",
			Action:         "msg.post",
			Resource:       "channel/general",
			IdempotencyKey: "chain-two",
			SessionID:      "session-2",
			TraceID:        "trace-2",
			RequestedAt:    time.Unix(200, 0).UTC(),
		},
		ReceivedAt: time.Unix(200, 0).UTC(),
		Decision:   types.DecisionAllow,
		PolicyResult: &types.PolicyResult{
			Decision: types.DecisionAllow,
			Reason:   "ok",
		},
		ExecutionResult: &types.ExecutionResult{
			Status:     "success",
			OutputJSON: json.RawMessage(`{"ok":true}`),
			DurationMS: 17,
		},
	}

	mustRecordEvent(t, stores.store, stores.ctx, first)
	mustRecordEvent(t, stores.store, stores.ctx, second)

	if first.Hash == "" || second.Hash == "" {
		t.Fatalf("expected hashes to be populated, first=%q second=%q", first.Hash, second.Hash)
	}
	if first.PrevHash != "" {
		t.Fatalf("expected first event prev hash empty, got %q", first.PrevHash)
	}
	if second.PrevHash != first.Hash {
		t.Fatalf("expected second prev hash %q, got %q", first.Hash, second.PrevHash)
	}

	got, err := stores.store.GetEvent(stores.ctx, second.EventID)
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	if got == nil {
		t.Fatalf("expected event detail")
	}
	if got.Request.SessionID != "session-2" || got.Request.TraceID != "trace-2" || got.Request.Resource != "channel/general" {
		t.Fatalf("expected request fields to round-trip, got %+v", got.Request)
	}
	if got.ExecutionResult == nil || got.ExecutionResult.Status != "success" || string(got.ExecutionResult.OutputJSON) != `{"ok":true}` {
		t.Fatalf("expected execution result to round-trip, got %+v", got.ExecutionResult)
	}

	chain, err := stores.store.GetChainEvents(stores.ctx, tenant.ID, 0)
	if err != nil {
		t.Fatalf("GetChainEvents: %v", err)
	}
	if len(chain) != 2 {
		t.Fatalf("expected two chain events, got %+v", chain)
	}
	if chain[0].EventID != first.EventID || chain[1].EventID != second.EventID {
		t.Fatalf("expected chain order to match insertion order, got %+v", chain)
	}
	if err := VerifyChain(chain); err != nil {
		t.Fatalf("expected valid chain, got %v", err)
	}
}

func TestStoreCheckIdempotencyReturnsApprovalReplayAndLatestExecutionResult(t *testing.T) {
	stores := newEvidenceStores(t)
	tenant := createTenantForEvidence(t, stores, "Replay Tenant")

	approveEnv := &types.ToolCallEnvelope{
		EventID: "33333333-3333-3333-3333-333333333333",
		Request: types.ToolCallRequest{
			TenantID:       tenant.ID,
			AgentID:        "agent-1",
			Tool:           "jira",
			Action:         "issue.delete",
			Resource:       "project/OPS",
			IdempotencyKey: "approval-replay",
			TraceID:        "trace-approve",
			RequestedAt:    time.Now().UTC(),
		},
		ReceivedAt:   time.Now().UTC(),
		Decision:     types.DecisionApprove,
		PolicyResult: &types.PolicyResult{Decision: types.DecisionApprove, Reason: "needs review"},
	}
	mustRecordEvent(t, stores.store, stores.ctx, approveEnv)
	req, err := stores.approvals.CreateRequest(stores.ctx, approvals.CreateApprovalInput{
		EventID:         approveEnv.EventID,
		TenantID:        tenant.ID,
		AgentID:         "agent-1",
		Tool:            "jira",
		Action:          "issue.delete",
		Resource:        "project/OPS",
		Reason:          "needs review",
		ApprovalBaseURL: "https://approvals.example.com",
	})
	if err != nil {
		t.Fatalf("CreateRequest for approval replay: %v", err)
	}

	approveReplay, err := stores.store.CheckIdempotency(stores.ctx, tenant.ID, "approval-replay")
	if err != nil {
		t.Fatalf("CheckIdempotency approval replay: %v", err)
	}
	if approveReplay == nil || approveReplay.Response == nil {
		t.Fatalf("expected approval replay response, got %+v", approveReplay)
	}
	if approveReplay.Response.Decision != types.DecisionApprove || approveReplay.Response.Reason != "needs review" || approveReplay.ApprovalRequestID != req.ID {
		t.Fatalf("unexpected approval replay: %+v", approveReplay)
	}

	allowEnv := &types.ToolCallEnvelope{
		EventID: "44444444-4444-4444-4444-444444444444",
		Request: types.ToolCallRequest{
			TenantID:       tenant.ID,
			AgentID:        "agent-1",
			Tool:           "slack",
			Action:         "msg.post",
			Resource:       "channel/general",
			IdempotencyKey: "allow-replay",
			TraceID:        "trace-allow",
			RequestedAt:    time.Now().UTC(),
		},
		ReceivedAt: time.Now().UTC(),
		Decision:   types.DecisionAllow,
		PolicyResult: &types.PolicyResult{
			Decision: types.DecisionAllow,
			Reason:   "executed",
		},
		ExecutionResult: &types.ExecutionResult{
			Status:     "success",
			OutputJSON: json.RawMessage(`{"version":1}`),
			DurationMS: 11,
		},
	}
	mustRecordEvent(t, stores.store, stores.ctx, allowEnv)

	if _, err := stores.console.Pool().Exec(stores.ctx, `DROP INDEX idx_tool_results_event`); err != nil {
		t.Fatalf("drop unique result index: %v", err)
	}
	if _, err := stores.console.Pool().Exec(stores.ctx, `
		INSERT INTO tool_results (event_id, tenant_id, status, output_json, error_msg, duration_ms, result_canon, created_at)
		VALUES ($1, $2, 'error', '{"version":2}'::jsonb, 'connector failed', 99, $3, $4)`,
		allowEnv.EventID, tenant.ID, []byte(`{"status":"error"}`), time.Now().UTC().Add(time.Second),
	); err != nil {
		t.Fatalf("insert newer tool result: %v", err)
	}

	gotEvent, err := stores.store.GetEvent(stores.ctx, allowEnv.EventID)
	if err != nil {
		t.Fatalf("GetEvent latest result: %v", err)
	}
	if gotEvent == nil || gotEvent.ExecutionResult == nil {
		t.Fatalf("expected event with latest execution result, got %+v", gotEvent)
	}
	if gotEvent.ExecutionResult.Status != "error" || gotEvent.ExecutionResult.Error != "connector failed" || string(gotEvent.ExecutionResult.OutputJSON) != `{"version":2}` {
		t.Fatalf("expected newest execution result to win, got %+v", gotEvent.ExecutionResult)
	}

	replay, err := stores.store.CheckIdempotency(stores.ctx, tenant.ID, "allow-replay")
	if err != nil {
		t.Fatalf("CheckIdempotency latest result: %v", err)
	}
	if replay == nil || replay.Response == nil || replay.Response.Result == nil {
		t.Fatalf("expected replay with execution result, got %+v", replay)
	}
	if replay.Response.Result.Status != "error" || replay.Response.Result.Error != "connector failed" || string(replay.Response.Result.OutputJSON) != `{"version":2}` {
		t.Fatalf("expected replay to use latest result row, got %+v", replay.Response.Result)
	}
}

func TestStoreExecutionLockAndLinkAreIdempotent(t *testing.T) {
	stores := newEvidenceStores(t)
	tenant := createTenantForEvidence(t, stores, "Lock Tenant")

	parent := &types.ToolCallEnvelope{
		EventID: "55555555-5555-5555-5555-555555555555",
		Request: types.ToolCallRequest{
			TenantID:       tenant.ID,
			AgentID:        "agent-1",
			Tool:           "slack",
			Action:         "msg.post",
			Resource:       "channel/general",
			IdempotencyKey: "parent",
			RequestedAt:    time.Now().UTC(),
		},
		ReceivedAt:   time.Now().UTC(),
		Decision:     types.DecisionApprove,
		PolicyResult: &types.PolicyResult{Decision: types.DecisionApprove, Reason: "needs review"},
	}
	execution := &types.ToolCallEnvelope{
		EventID: "66666666-6666-6666-6666-666666666666",
		Request: types.ToolCallRequest{
			TenantID:       tenant.ID,
			AgentID:        "agent-1",
			Tool:           "slack",
			Action:         "msg.post",
			Resource:       "channel/general",
			IdempotencyKey: "exec:" + parent.EventID,
			RequestedAt:    time.Now().UTC(),
		},
		ReceivedAt: time.Now().UTC(),
		Decision:   types.DecisionAllow,
		PolicyResult: &types.PolicyResult{
			Decision: types.DecisionAllow,
			Reason:   "approved execution",
		},
		ExecutionResult: &types.ExecutionResult{Status: "success"},
	}
	mustRecordEvent(t, stores.store, stores.ctx, parent)
	mustRecordEvent(t, stores.store, stores.ctx, execution)

	firstUnlock, err := stores.store.LockParentExecution(stores.ctx, parent.EventID)
	if err != nil {
		t.Fatalf("LockParentExecution first: %v", err)
	}

	acquiredSecond := make(chan struct{})
	go func() {
		unlock, err := stores.store.LockParentExecution(context.Background(), parent.EventID)
		if err != nil {
			t.Errorf("LockParentExecution second: %v", err)
			close(acquiredSecond)
			return
		}
		close(acquiredSecond)
		unlock()
	}()

	select {
	case <-acquiredSecond:
		t.Fatalf("expected second lock acquisition to block until first unlocks")
	case <-time.After(150 * time.Millisecond):
	}

	firstUnlock()

	select {
	case <-acquiredSecond:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected second lock to acquire after unlock")
	}

	linked, err := stores.store.LinkExecutionToParent(stores.ctx, parent.EventID, execution.EventID, "grant-1")
	if err != nil {
		t.Fatalf("LinkExecutionToParent first: %v", err)
	}
	if !linked {
		t.Fatalf("expected first link attempt to succeed")
	}
	linked, err = stores.store.LinkExecutionToParent(stores.ctx, parent.EventID, execution.EventID, "grant-1")
	if err != nil {
		t.Fatalf("LinkExecutionToParent second: %v", err)
	}
	if linked {
		t.Fatalf("expected second link attempt to be idempotent false")
	}
}
