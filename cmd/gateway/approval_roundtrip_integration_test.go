package main

import (
	"bytes"
	"container/list"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/bturcanu/OpenClause/internal/testdb"
	"github.com/bturcanu/OpenClause/pkg/approvals"
	"github.com/bturcanu/OpenClause/pkg/console"
	"github.com/bturcanu/OpenClause/pkg/evidence"
	"github.com/bturcanu/OpenClause/pkg/types"
)

func TestApprovalRoundtripFlowsIntoConsoleSessionTimeline(t *testing.T) {
	ctx := context.Background()
	h := testdb.New(t)
	consoleStore := console.NewStore(h.Pool())
	approvalsStore := approvals.NewStore(h.Pool())
	evidenceStore := evidence.NewStore(h.Pool())

	tenant, err := consoleStore.CreateTenant(ctx, "Roundtrip Tenant", nil)
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}

	fakeConnector := &fakeConnectors{output: json.RawMessage(`{"ok":true,"ticket":"OPS-42"}`)}
	gw := &Gateway{
		log:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		evidence:       evidenceStore,
		policy:         fakePolicy{decision: types.DecisionApprove, reason: "manual review required"},
		connectors:     fakeConnector,
		approvals:      approvalsStore,
		approvalsURL:   "https://approvals.example.com",
		consoleStore:   consoleStore,
		rateLimiters:   make(map[string]*list.Element),
		rlList:         list.New(),
		perTenantLimit: 100,
	}

	requestedAt := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	sessionID := "session-approval-roundtrip"
	traceID := "trace-approval-roundtrip"
	body, err := json.Marshal(types.ToolCallRequest{
		TenantID:       tenant.ID,
		AgentID:        "agent-roundtrip",
		Tool:           "jira",
		Action:         "issue.create",
		Resource:       "project/OPS",
		RiskScore:      8,
		RiskFactors:    []string{"write_action", "prod_project"},
		UserID:         "user-roundtrip",
		SessionID:      sessionID,
		TraceID:        traceID,
		IdempotencyKey: "approval-roundtrip-1",
		RequestedAt:    requestedAt,
		Labels: map[string]string{
			"user_name":  "Round Trip User",
			"user_email": "roundtrip@example.com",
		},
	})
	if err != nil {
		t.Fatalf("marshal toolcall request: %v", err)
	}

	approvalRespRecorder := postToolCall(t, gw, body)
	if approvalRespRecorder.Code != 200 {
		t.Fatalf("expected approve response 200, got %d body=%s", approvalRespRecorder.Code, approvalRespRecorder.Body.String())
	}

	var approvalResp types.ToolCallResponse
	if err := json.NewDecoder(approvalRespRecorder.Body).Decode(&approvalResp); err != nil {
		t.Fatalf("decode approval response: %v", err)
	}
	if approvalResp.Decision != types.DecisionApprove || approvalResp.EventID == "" {
		t.Fatalf("unexpected approval response: %+v", approvalResp)
	}
	if !strings.Contains(approvalResp.ApprovalURL, "/v1/approvals/requests/") {
		t.Fatalf("expected approval URL to point at request, got %q", approvalResp.ApprovalURL)
	}

	pending, err := approvalsStore.ListPending(ctx, tenant.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected one pending approval request, got %+v", pending)
	}
	if pending[0].EventID != approvalResp.EventID || pending[0].SessionID != sessionID || pending[0].TraceID != traceID {
		t.Fatalf("expected pending approval to link back to session/trace, got %+v", pending[0])
	}

	awaitingRR := executeRequest(t, gw, approvalResp.EventID)
	if awaitingRR.Code != 409 {
		t.Fatalf("expected execute before grant to 409, got %d body=%s", awaitingRR.Code, awaitingRR.Body.String())
	}

	grant, err := approvalsStore.GrantRequest(ctx, pending[0].ID, approvals.GrantInput{Approver: "approver@example.com"})
	if err != nil {
		t.Fatalf("GrantRequest: %v", err)
	}
	if grant == nil || grant.ID == "" {
		t.Fatalf("expected approval grant to be created, got %+v", grant)
	}

	execRR := executeRequest(t, gw, approvalResp.EventID)
	if execRR.Code != 200 {
		t.Fatalf("expected execute after grant to 200, got %d body=%s", execRR.Code, execRR.Body.String())
	}
	var execResp types.ToolCallResponse
	if err := json.NewDecoder(execRR.Body).Decode(&execResp); err != nil {
		t.Fatalf("decode execute response: %v", err)
	}
	if execResp.Decision != types.DecisionAllow || execResp.EventID == "" || execResp.Result == nil || execResp.Result.Status != "success" {
		t.Fatalf("unexpected execute response: %+v", execResp)
	}
	if fakeConnector.calls != 1 {
		t.Fatalf("expected connector to run once after approval, got %d calls", fakeConnector.calls)
	}

	linkedExecution, err := evidenceStore.GetExecutionByParentEvent(ctx, approvalResp.EventID)
	if err != nil {
		t.Fatalf("GetExecutionByParentEvent: %v", err)
	}
	if linkedExecution == nil || linkedExecution.EventID != execResp.EventID {
		t.Fatalf("expected parent event to link to execution event %q, got %+v", execResp.EventID, linkedExecution)
	}

	session, err := consoleStore.GetSession(ctx, sessionID, tenant.ID, "")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session == nil {
		t.Fatalf("expected session detail for %q", sessionID)
	}
	if session.EventCount != 2 || session.ApproveCount != 1 || session.AllowCount != 1 || session.DenyCount != 0 {
		t.Fatalf("unexpected session summary: %+v", session)
	}
	if session.TraceID != traceID || session.LastEventID != execResp.EventID {
		t.Fatalf("expected session to carry trace and latest execution event, got %+v", session)
	}

	timeline, err := consoleStore.GetSessionTimeline(ctx, sessionID, tenant.ID, "")
	if err != nil {
		t.Fatalf("GetSessionTimeline: %v", err)
	}
	if len(timeline) != 1 {
		t.Fatalf("expected timeline to collapse execution under parent event, got %+v", timeline)
	}
	item := timeline[0]
	if item.EventID != approvalResp.EventID || item.Decision != string(types.DecisionApprove) {
		t.Fatalf("expected timeline item to be the approval event, got %+v", item)
	}
	if item.Approval == nil || item.Approval.ID != pending[0].ID || item.Approval.Status != "approved" {
		t.Fatalf("expected approved approval summary on timeline item, got %+v", item.Approval)
	}
	if item.Execution == nil || item.Execution.EventID != execResp.EventID || item.Execution.Status != "success" {
		t.Fatalf("expected execution summary to be linked onto parent event, got %+v", item.Execution)
	}
	if item.SessionID != sessionID || item.TraceID != traceID {
		t.Fatalf("expected session and trace linkage to round-trip, got %+v", item)
	}

	var csv bytes.Buffer
	if err := consoleStore.ExportSessionCSV(ctx, sessionID, tenant.ID, "", &csv); err != nil {
		t.Fatalf("ExportSessionCSV: %v", err)
	}
	csvText := csv.String()
	if !strings.Contains(csvText, sessionID) || !strings.Contains(csvText, pending[0].ID) || !strings.Contains(csvText, execResp.EventID) {
		t.Fatalf("expected session CSV to include session, approval, and execution linkage, got %q", csvText)
	}
}
