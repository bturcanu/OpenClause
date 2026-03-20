package main

import (
	"bytes"
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/bturcanu/OpenClause/pkg/approvals"
	"github.com/bturcanu/OpenClause/pkg/auth"
	"github.com/bturcanu/OpenClause/pkg/connectors"
	"github.com/bturcanu/OpenClause/pkg/console"
	"github.com/bturcanu/OpenClause/pkg/evidence"
	"github.com/bturcanu/OpenClause/pkg/types"
	"github.com/go-chi/chi/v5"
)

type fakeEvidence struct {
	mu          sync.Mutex
	execLocks   sync.Map // parentEventID -> *sync.Mutex
	events      map[string]*types.ToolCallEnvelope
	byParent    map[string]*types.ToolCallResponse
	linkedPairs map[string]string
	replays     map[string]*evidence.ReplayResponse
	recordErr   error
}

func newFakeEvidence() *fakeEvidence {
	return &fakeEvidence{
		events:      map[string]*types.ToolCallEnvelope{},
		byParent:    map[string]*types.ToolCallResponse{},
		linkedPairs: map[string]string{},
		replays:     map[string]*evidence.ReplayResponse{},
	}
}

func (f *fakeEvidence) RecordEvent(_ context.Context, env *types.ToolCallEnvelope) error {
	if f.recordErr != nil {
		return f.recordErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events[env.EventID] = env
	resp := &types.ToolCallResponse{
		EventID:  env.EventID,
		Decision: env.Decision,
	}
	if env.PolicyResult != nil {
		resp.Reason = env.PolicyResult.Reason
	}
	if env.ExecutionResult != nil {
		resultCopy := *env.ExecutionResult
		resp.Result = &resultCopy
	}
	f.replays[replayKey(env.Request.TenantID, env.Request.IdempotencyKey)] = &evidence.ReplayResponse{
		Response: resp,
	}
	return nil
}

func replayKey(tenantID, idempotencyKey string) string {
	return tenantID + "|" + idempotencyKey
}

func (f *fakeEvidence) CheckIdempotency(_ context.Context, tenantID, idempotencyKey string) (*evidence.ReplayResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.replays[replayKey(tenantID, idempotencyKey)], nil
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

func (f *fakeEvidence) LockParentExecution(_ context.Context, parentEventID string) (func(), error) {
	val, _ := f.execLocks.LoadOrStore(parentEventID, &sync.Mutex{})
	m := val.(*sync.Mutex)
	m.Lock()
	return func() { m.Unlock() }, nil
}

type fakePolicy struct {
	decision      types.Decision
	reason        string
	notify        []types.PolicyNotify
	approverGroup string
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
	return &types.PolicyResult{
		Decision:      d,
		Reason:        r,
		Notify:        f.notify,
		ApproverGroup: f.approverGroup,
	}, nil
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
	mu         sync.Mutex
	usesLeft   int
	createErr  error
	createID   string
	lastCreate *approvals.CreateApprovalInput
}

func (f *fakeApprovals) CreateRequest(_ context.Context, in approvals.CreateApprovalInput) (*approvals.ApprovalRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return nil, f.createErr
	}
	copied := in
	f.lastCreate = &copied
	id := f.createID
	if id == "" {
		id = "req-1"
	}
	return &approvals.ApprovalRequest{ID: id}, nil
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

type fakeGatewayConsoleStore struct {
	policyCfg *console.TenantPolicyConfig
	notifCfg  *console.TenantNotificationConfig
}

func (f *fakeGatewayConsoleStore) GetTenantPolicyConfig(context.Context, string) (*console.TenantPolicyConfig, bool, error) {
	if f.policyCfg == nil {
		return nil, false, nil
	}
	return f.policyCfg, true, nil
}

func (f *fakeGatewayConsoleStore) GetTenantNotificationConfig(context.Context, string) (*console.TenantNotificationConfig, bool, error) {
	if f.notifCfg == nil {
		return nil, false, nil
	}
	return f.notifCfg, true, nil
}

func newTestGateway(fe *fakeEvidence, fc *fakeConnectors, fa *fakeApprovals, pol gatewayPolicy) *Gateway {
	if pol == nil {
		pol = fakePolicy{}
	}
	return &Gateway{
		log:            slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)),
		evidence:       fe,
		policy:         pol,
		connectors:     fc,
		approvals:      fa,
		approvalsURL:   "http://approvals.example",
		rateLimiters:   make(map[string]*list.Element),
		rlList:         list.New(),
		perTenantLimit: 100,
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
	gw := newTestGateway(fe, fc, fa, nil)

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
	gw := newTestGateway(fe, fc, fa, nil)

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
	gw := newTestGateway(fe, fc, fa, fakePolicy{decision: types.DecisionAllow})

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
	gw := newTestGateway(fe, fc, fa, fakePolicy{decision: types.DecisionDeny, reason: "blocked"})

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

type nilPolicy struct{}

func (f nilPolicy) Evaluate(context.Context, types.PolicyInput) (*types.PolicyResult, error) {
	return nil, nil
}

func TestHandleToolCall_PolicyNilResultDefaultsDeny(t *testing.T) {
	fe := newFakeEvidence()
	fc := &fakeConnectors{output: json.RawMessage(`{"ok":true}`)}
	fa := &fakeApprovals{}
	gw := newTestGateway(fe, fc, fa, nilPolicy{})

	body, _ := json.Marshal(types.ToolCallRequest{
		TenantID:       "tenant1",
		AgentID:        "agent-1",
		Tool:           "slack",
		Action:         "msg.post",
		RiskScore:      2,
		IdempotencyKey: "k3",
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
	if resp.Reason != "policy evaluation returned nil" {
		t.Fatalf("expected deny reason, got %q", resp.Reason)
	}

	if len(fe.events) != 1 {
		t.Fatalf("expected exactly 1 recorded evidence event, got %d", len(fe.events))
	}
	for _, env := range fe.events {
		if env.Decision != types.DecisionDeny {
			t.Fatalf("expected recorded decision=deny, got %s", env.Decision)
		}
	}
}

func TestHandleToolCall_BadJSON(t *testing.T) {
	fe := newFakeEvidence()
	gw := newTestGateway(fe, &fakeConnectors{}, &fakeApprovals{}, nil)

	rr := postToolCall(t, gw, []byte(`{invalid json`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestExecute_WrongTenantReturns404(t *testing.T) {
	const parentID = "00000000-0000-0000-0000-000000000003"
	fe := newFakeEvidence()
	fe.events[parentID] = &types.ToolCallEnvelope{
		EventID:  parentID,
		Request:  types.ToolCallRequest{TenantID: "tenant1"},
		Decision: types.DecisionApprove,
	}
	gw := newTestGateway(fe, &fakeConnectors{}, &fakeApprovals{}, nil)

	ks := auth.NewKeyStore("other-tenant:sk-other")
	r := chi.NewRouter()
	r.Use(auth.APIKeyAuth(ks, nil))
	r.Post("/v1/toolcalls/{event_id}/execute", gw.HandleExecuteToolCall)
	req := httptest.NewRequest(http.MethodPost, "/v1/toolcalls/"+parentID+"/execute", http.NoBody)
	req.Header.Set("X-API-Key", "sk-other")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestExecute_AwaitingApprovalReturns409(t *testing.T) {
	const parentID = "00000000-0000-0000-0000-000000000004"
	fe := newFakeEvidence()
	fe.events[parentID] = &types.ToolCallEnvelope{
		EventID: parentID,
		Request: types.ToolCallRequest{
			TenantID: "tenant1",
			AgentID:  "agent-1",
			Tool:     "jira",
			Action:   "issue.delete",
		},
		Decision: types.DecisionApprove,
	}
	fa := &fakeApprovals{usesLeft: 0}
	gw := newTestGateway(fe, &fakeConnectors{}, fa, nil)

	rr := executeRequest(t, gw, parentID)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 (awaiting approval), got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleToolCall_ValidationError(t *testing.T) {
	fe := newFakeEvidence()
	gw := newTestGateway(fe, &fakeConnectors{}, &fakeApprovals{}, nil)

	body, _ := json.Marshal(types.ToolCallRequest{
		TenantID: "tenant1",
	})
	rr := postToolCall(t, gw, body)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleToolCall_IdempotentReplayPreservesExecutionResult(t *testing.T) {
	fe := newFakeEvidence()
	fe.replays[replayKey("tenant1", "k-replay")] = &evidence.ReplayResponse{
		Response: &types.ToolCallResponse{
			EventID:  "evt-replay",
			Decision: types.DecisionAllow,
			Reason:   "ok",
			Result: &types.ExecutionResult{
				Status:     "success",
				OutputJSON: json.RawMessage(`{"ok":true}`),
			},
		},
	}
	gw := newTestGateway(fe, &fakeConnectors{}, &fakeApprovals{}, nil)

	body, _ := json.Marshal(types.ToolCallRequest{
		TenantID:       "tenant1",
		AgentID:        "agent-1",
		Tool:           "slack",
		Action:         "msg.post",
		RiskScore:      1,
		IdempotencyKey: "k-replay",
	})
	rr := postToolCall(t, gw, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	var resp types.ToolCallResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.EventID != "evt-replay" || resp.Result == nil || resp.Result.Status != "success" {
		t.Fatalf("unexpected replay response: %+v", resp)
	}
}

func TestHandleToolCall_IdempotentApproveReplayReconstructsApprovalURL(t *testing.T) {
	fe := newFakeEvidence()
	fe.replays[replayKey("tenant1", "k-approve")] = &evidence.ReplayResponse{
		Response: &types.ToolCallResponse{
			EventID:  "evt-approve",
			Decision: types.DecisionApprove,
			Reason:   "needs review",
		},
		ApprovalRequestID: "req-123",
	}
	gw := newTestGateway(fe, &fakeConnectors{}, &fakeApprovals{}, nil)

	body, _ := json.Marshal(types.ToolCallRequest{
		TenantID:       "tenant1",
		AgentID:        "agent-1",
		Tool:           "jira",
		Action:         "issue.delete",
		RiskScore:      8,
		IdempotencyKey: "k-approve",
	})
	rr := postToolCall(t, gw, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	var resp types.ToolCallResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ApprovalURL != "http://approvals.example/v1/approvals/requests/req-123" {
		t.Fatalf("unexpected approval_url: %q", resp.ApprovalURL)
	}
}

func TestHandleToolCall_DenyEvidenceFailureReturns500(t *testing.T) {
	fe := newFakeEvidence()
	fe.recordErr = errors.New("db down")
	gw := newTestGateway(fe, &fakeConnectors{}, &fakeApprovals{}, fakePolicy{decision: types.DecisionDeny, reason: "blocked"})

	body, _ := json.Marshal(types.ToolCallRequest{
		TenantID:       "tenant1",
		AgentID:        "agent-1",
		Tool:           "slack",
		Action:         "msg.post",
		RiskScore:      2,
		IdempotencyKey: "k-deny-error",
	})
	rr := postToolCall(t, gw, body)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleToolCall_ApproveCreateFailureReturns500(t *testing.T) {
	fe := newFakeEvidence()
	fa := &fakeApprovals{createErr: errors.New("insert failed")}
	gw := newTestGateway(fe, &fakeConnectors{}, fa, fakePolicy{decision: types.DecisionApprove, reason: "needs review"})

	body, _ := json.Marshal(types.ToolCallRequest{
		TenantID:       "tenant1",
		AgentID:        "agent-1",
		Tool:           "jira",
		Action:         "issue.delete",
		RiskScore:      8,
		IdempotencyKey: "k-approve-error",
	})
	rr := postToolCall(t, gw, body)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleToolCall_TenantPolicyConfigDoesNotOverrideOPAResult(t *testing.T) {
	fe := newFakeEvidence()
	fa := &fakeApprovals{}
	gw := newTestGateway(fe, &fakeConnectors{}, fa, fakePolicy{
		decision:      types.DecisionApprove,
		reason:        "opa approve",
		approverGroup: "security-review",
		notify:        []types.PolicyNotify{{Kind: "slack", Channel: "#approvals"}},
	})
	gw.consoleStore = &fakeGatewayConsoleStore{
		policyCfg: &console.TenantPolicyConfig{
			MaxRiskAutoApprove: 5,
			ReadActions:        []string{"slack.msg.post"},
		},
	}

	body, _ := json.Marshal(types.ToolCallRequest{
		TenantID:       "tenant1",
		AgentID:        "agent-1",
		Tool:           "slack",
		Action:         "msg.post",
		RiskScore:      1,
		IdempotencyKey: "k-opa",
	})
	rr := postToolCall(t, gw, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	var resp types.ToolCallResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Decision != types.DecisionApprove || resp.Reason != "opa approve" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if fa.lastCreate == nil {
		t.Fatal("expected approval request to be created")
	}
	if fa.lastCreate.ApproverGroup != "security-review" || len(fa.lastCreate.Notify) != 1 {
		t.Fatalf("expected OPA metadata to be preserved, got %+v", fa.lastCreate)
	}
}

func TestExecute_ReplaysRecordedExecutionWithoutLink(t *testing.T) {
	const parentID = "00000000-0000-0000-0000-000000000005"
	fe := newFakeEvidence()
	fe.events[parentID] = &types.ToolCallEnvelope{
		EventID: parentID,
		Request: types.ToolCallRequest{
			TenantID: "tenant1",
			AgentID:  "agent-1",
			Tool:     "jira",
			Action:   "issue.create",
		},
		Decision: types.DecisionApprove,
	}
	fe.replays[replayKey("tenant1", "exec:"+parentID)] = &evidence.ReplayResponse{
		Response: &types.ToolCallResponse{
			EventID:  "exec-1",
			Decision: types.DecisionAllow,
			Reason:   "approved execution",
			Result:   &types.ExecutionResult{Status: "success"},
		},
	}
	gw := newTestGateway(fe, &fakeConnectors{}, &fakeApprovals{usesLeft: 0}, nil)

	rr := executeRequest(t, gw, parentID)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	var resp types.ToolCallResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.EventID != "exec-1" || resp.Result == nil || resp.Result.Status != "success" {
		t.Fatalf("unexpected replay response: %+v", resp)
	}
}
