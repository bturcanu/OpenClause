package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/bturcanu/OpenClause/pkg/approvals"
	"github.com/bturcanu/OpenClause/pkg/connectors"
	"github.com/bturcanu/OpenClause/pkg/types"
	"github.com/go-chi/chi/v5"
	"golang.org/x/time/rate"
)

type fakeEvidence struct {
	mu          sync.Mutex
	events      map[string]*types.ToolCallEnvelope
	byParent    map[string]*types.ToolCallResponse
	linkedPairs map[string]string
}

func newFakeEvidence() *fakeEvidence {
	return &fakeEvidence{
		events:      map[string]*types.ToolCallEnvelope{},
		byParent:    map[string]*types.ToolCallResponse{},
		linkedPairs: map[string]string{},
	}
}

func (f *fakeEvidence) RecordEvent(_ context.Context, env *types.ToolCallEnvelope) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events[env.EventID] = env
	return nil
}

func (f *fakeEvidence) CheckIdempotency(context.Context, string, string) (*types.ToolCallResponse, error) {
	return nil, nil
}

func (f *fakeEvidence) GetEvent(_ context.Context, eventID string) (*types.ToolCallEnvelope, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.events[eventID], nil
}

func (f *fakeEvidence) GetExecutionByParentEvent(_ context.Context, parentEventID string) (*types.ToolCallResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.byParent[parentEventID], nil
}

func (f *fakeEvidence) LinkExecutionToParent(_ context.Context, parentEventID, executionEventID, _ string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.linkedPairs[parentEventID]; ok {
		return false, nil
	}
	env := f.events[executionEventID]
	f.linkedPairs[parentEventID] = executionEventID
	f.byParent[parentEventID] = &types.ToolCallResponse{
		EventID:  executionEventID,
		Decision: types.DecisionAllow,
		Reason:   "idempotent execute replay",
		Result:   env.ExecutionResult,
	}
	return true, nil
}

type fakePolicy struct {
	decision types.Decision
	reason   string
}

func (f fakePolicy) Evaluate(context.Context, types.PolicyInput) (*types.PolicyResult, error) {
	d := f.decision
	if d == "" {
		d = types.DecisionAllow
	}
	r := f.reason
	if r == "" {
		r = "ok"
	}
	return &types.PolicyResult{Decision: d, Reason: r}, nil
}

type fakeConnectors struct {
	mu     sync.Mutex
	calls  int
	delay  time.Duration
	output json.RawMessage
}

func (f *fakeConnectors) Exec(_ context.Context, _ connectors.ExecRequest) (*connectors.ExecResponse, error) {
	time.Sleep(f.delay)
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	return &connectors.ExecResponse{
		Status:     "success",
		OutputJSON: f.output,
	}, nil
}

type fakeApprovals struct {
	mu       sync.Mutex
	usesLeft int
}

func (f *fakeApprovals) CreateRequest(context.Context, approvals.CreateApprovalInput) (*approvals.ApprovalRequest, error) {
	return &approvals.ApprovalRequest{}, nil
}

func (f *fakeApprovals) FindAndConsumeGrant(_ context.Context, _, _, _, _, _ string) (*approvals.ApprovalGrant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.usesLeft <= 0 {
		return nil, nil
	}
	f.usesLeft--
	return &approvals.ApprovalGrant{ID: "grant-1"}, nil
}

func newExecuteGateway(fe *fakeEvidence, fc *fakeConnectors, fa *fakeApprovals) *Gateway {
	return &Gateway{
		log:          slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)),
		evidence:     fe,
		policy:       fakePolicy{},
		connectors:   fc,
		approvals:    fa,
		rateLimiters: make(map[string]*rate.Limiter),
	}
}

func executeRequest(t *testing.T, gw *Gateway, eventID string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Post("/v1/toolcalls/{event_id}/execute", gw.HandleExecuteToolCall)
	req := httptest.NewRequest(http.MethodPost, "/v1/toolcalls/"+eventID+"/execute", http.NoBody)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

func TestExecuteHappyPathAndIdempotentReplay(t *testing.T) {
	const parentID = "00000000-0000-0000-0000-000000000001"
	fe := newFakeEvidence()
	fe.events[parentID] = &types.ToolCallEnvelope{
		EventID: parentID,
		Request: types.ToolCallRequest{
			TenantID: "tenant1",
			AgentID:  "agent-1",
			Tool:     "slack",
			Action:   "msg.post",
			Resource: "channel/general",
		},
		Decision: types.DecisionApprove,
	}
	fc := &fakeConnectors{output: json.RawMessage(`{"ok":true}`)}
	fa := &fakeApprovals{usesLeft: 1}
	gw := newExecuteGateway(fe, fc, fa)

	first := executeRequest(t, gw, parentID)
	if first.Code != http.StatusOK {
		t.Fatalf("first execute status=%d body=%s", first.Code, first.Body.String())
	}
	var firstResp types.ToolCallResponse
	if err := json.NewDecoder(first.Body).Decode(&firstResp); err != nil {
		t.Fatalf("decode first response: %v", err)
	}
	if firstResp.Decision != types.DecisionAllow || firstResp.Result == nil {
		t.Fatalf("unexpected first response: %+v", firstResp)
	}

	second := executeRequest(t, gw, parentID)
	if second.Code != http.StatusOK {
		t.Fatalf("second execute status=%d body=%s", second.Code, second.Body.String())
	}
	var secondResp types.ToolCallResponse
	if err := json.NewDecoder(second.Body).Decode(&secondResp); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	if secondResp.EventID != firstResp.EventID {
		t.Fatalf("expected replay event_id %s got %s", firstResp.EventID, secondResp.EventID)
	}
}

func TestExecuteConcurrentCallsConsumeGrantSafely(t *testing.T) {
	const parentID = "00000000-0000-0000-0000-000000000002"
	fe := newFakeEvidence()
	fe.events[parentID] = &types.ToolCallEnvelope{
		EventID: parentID,
		Request: types.ToolCallRequest{
			TenantID: "tenant1",
			AgentID:  "agent-1",
			Tool:     "jira",
			Action:   "issue.create",
			Resource: "project/OPS",
		},
		Decision: types.DecisionApprove,
	}
	fc := &fakeConnectors{
		delay:  120 * time.Millisecond,
		output: json.RawMessage(`{"id":"123"}`),
	}
	fa := &fakeApprovals{usesLeft: 1}
	gw := newExecuteGateway(fe, fc, fa)

	var wg sync.WaitGroup
	wg.Add(2)
	results := make([]*httptest.ResponseRecorder, 2)
	for i := range 2 {
		go func(idx int) {
			defer wg.Done()
			results[idx] = executeRequest(t, gw, parentID)
		}(i)
	}
	wg.Wait()

	// One request must succeed; the other is deterministic-safe conflict or replay.
	okCount := 0
	conflictCount := 0
	for _, rr := range results {
		switch rr.Code {
		case http.StatusOK:
			okCount++
		case http.StatusConflict:
			conflictCount++
		default:
			t.Fatalf("unexpected status code=%d body=%s", rr.Code, rr.Body.String())
		}
	}
	if okCount == 0 {
		t.Fatalf("expected at least one successful execution")
	}
	if okCount+conflictCount != 2 {
		t.Fatalf("expected two terminal responses, got ok=%d conflict=%d", okCount, conflictCount)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// HandleToolCall (POST /v1/toolcalls) tests
// ──────────────────────────────────────────────────────────────────────────────

func postToolCall(t *testing.T, gw *Gateway, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Post("/v1/toolcalls", gw.HandleToolCall)
	req := httptest.NewRequest(http.MethodPost, "/v1/toolcalls", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

func TestHandleToolCall_AllowPath(t *testing.T) {
	fe := newFakeEvidence()
	fc := &fakeConnectors{output: json.RawMessage(`{"ok":true}`)}
	fa := &fakeApprovals{}
	gw := &Gateway{
		log:            slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)),
		evidence:       fe,
		policy:         fakePolicy{decision: types.DecisionAllow},
		connectors:     fc,
		approvals:      fa,
		rateLimiters:   make(map[string]*rate.Limiter),
		perTenantLimit: 100,
	}

	body, _ := json.Marshal(types.ToolCallRequest{
		TenantID:       "tenant1",
		AgentID:        "agent-1",
		Tool:           "slack",
		Action:         "msg.post",
		RiskScore:      2,
		IdempotencyKey: "k1",
	})
	rr := postToolCall(t, gw, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	var resp types.ToolCallResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Decision != types.DecisionAllow {
		t.Fatalf("expected allow, got %s", resp.Decision)
	}
	if resp.Result == nil {
		t.Fatal("expected execution result")
	}
}

func TestHandleToolCall_DenyPath(t *testing.T) {
	fe := newFakeEvidence()
	fc := &fakeConnectors{}
	fa := &fakeApprovals{}
	gw := &Gateway{
		log:            slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)),
		evidence:       fe,
		policy:         fakePolicy{decision: types.DecisionDeny, reason: "blocked"},
		connectors:     fc,
		approvals:      fa,
		rateLimiters:   make(map[string]*rate.Limiter),
		perTenantLimit: 100,
	}

	body, _ := json.Marshal(types.ToolCallRequest{
		TenantID:       "tenant1",
		AgentID:        "agent-1",
		Tool:           "slack",
		Action:         "msg.post",
		RiskScore:      2,
		IdempotencyKey: "k2",
	})
	rr := postToolCall(t, gw, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	var resp types.ToolCallResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Decision != types.DecisionDeny {
		t.Fatalf("expected deny, got %s", resp.Decision)
	}
}

func TestHandleToolCall_BadJSON(t *testing.T) {
	fe := newFakeEvidence()
	gw := &Gateway{
		log:            slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)),
		evidence:       fe,
		policy:         fakePolicy{},
		connectors:     &fakeConnectors{},
		approvals:      &fakeApprovals{},
		rateLimiters:   make(map[string]*rate.Limiter),
		perTenantLimit: 100,
	}

	rr := postToolCall(t, gw, []byte(`{invalid json`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleToolCall_ValidationError(t *testing.T) {
	fe := newFakeEvidence()
	gw := &Gateway{
		log:            slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)),
		evidence:       fe,
		policy:         fakePolicy{},
		connectors:     &fakeConnectors{},
		approvals:      &fakeApprovals{},
		rateLimiters:   make(map[string]*rate.Limiter),
		perTenantLimit: 100,
	}

	body, _ := json.Marshal(types.ToolCallRequest{
		TenantID: "tenant1",
	})
	rr := postToolCall(t, gw, body)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 got %d body=%s", rr.Code, rr.Body.String())
	}
}
