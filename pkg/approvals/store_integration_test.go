package approvals

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bturcanu/OpenClause/internal/testdb"
	"github.com/bturcanu/OpenClause/pkg/console"
	"github.com/bturcanu/OpenClause/pkg/evidence"
	"github.com/bturcanu/OpenClause/pkg/types"
)

type approvalStores struct {
	console  *console.Store
	approvals *Store
	evidence *evidence.Store
	ctx      context.Context
}

func newApprovalStores(t *testing.T) *approvalStores {
	t.Helper()
	h := testdb.New(t)
	return &approvalStores{
		console:  console.NewStore(h.Pool()),
		approvals: NewStore(h.Pool()),
		evidence: evidence.NewStore(h.Pool()),
		ctx:      context.Background(),
	}
}

func mustCreateTenant(t *testing.T, ctx context.Context, store *console.Store, name string) *console.Tenant {
	t.Helper()
	tenant, err := store.CreateTenant(ctx, name, nil)
	if err != nil {
		t.Fatalf("CreateTenant(%q): %v", name, err)
	}
	return tenant
}

func seedApprovalEvent(t *testing.T, stores *approvalStores, tenantID, eventID, key string) {
	t.Helper()
	env := &types.ToolCallEnvelope{
		EventID: eventID,
		Request: types.ToolCallRequest{
			TenantID:       tenantID,
			AgentID:        "agent-1",
			Tool:           "slack",
			Action:         "msg.post",
			Resource:       "channel/general",
			IdempotencyKey: key,
			TraceID:        "trace-" + key,
			RequestedAt:    time.Now().UTC(),
		},
		PayloadJSON:  mustMarshalJSON(t, map[string]any{"tenant_id": tenantID, "agent_id": "agent-1", "tool": "slack", "action": "msg.post", "resource": "channel/general", "idempotency_key": key, "trace_id": "trace-" + key}),
		ReceivedAt:   time.Now().UTC(),
		Decision:     types.DecisionApprove,
		PolicyResult: &types.PolicyResult{Decision: types.DecisionApprove, Reason: "needs approval"},
	}
	if err := stores.evidence.RecordEvent(stores.ctx, env); err != nil {
		t.Fatalf("RecordEvent(%s): %v", eventID, err)
	}
}

func mustMarshalJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return b
}

func TestStoreCreateListPendingAndTransitions(t *testing.T) {
	stores := newApprovalStores(t)
	tenant := mustCreateTenant(t, stores.ctx, stores.console, "Approvals Tenant")

	seedApprovalEvent(t, stores, tenant.ID, "11111111-1111-1111-1111-111111111111", "event-one")
	seedApprovalEvent(t, stores, tenant.ID, "22222222-2222-2222-2222-222222222222", "event-two")

	reqOne, err := stores.approvals.CreateRequest(stores.ctx, CreateApprovalInput{
		EventID:         "11111111-1111-1111-1111-111111111111",
		TenantID:        tenant.ID,
		AgentID:         "agent-1",
		Tool:            "slack",
		Action:          "msg.post",
		Resource:        "channel/general",
		RiskScore:       7,
		Reason:          "needs review",
		TraceID:         "trace-one",
		Notify:          []types.PolicyNotify{{Kind: "webhook", URL: "https://example.com/hook", SecretRef: "s1"}},
		ApprovalBaseURL: "https://approvals.example.com",
	})
	if err != nil {
		t.Fatalf("CreateRequest reqOne: %v", err)
	}
	reqTwo, err := stores.approvals.CreateRequest(stores.ctx, CreateApprovalInput{
		EventID:         "22222222-2222-2222-2222-222222222222",
		TenantID:        tenant.ID,
		AgentID:         "agent-1",
		Tool:            "slack",
		Action:          "msg.post",
		Resource:        "channel/general",
		RiskScore:       8,
		Reason:          "deny this one",
		TraceID:         "trace-two",
		ApprovalBaseURL: "https://approvals.example.com",
	})
	if err != nil {
		t.Fatalf("CreateRequest reqTwo: %v", err)
	}
	if _, err := stores.console.Pool().Exec(stores.ctx, `UPDATE approval_requests SET created_at = $2 WHERE id = $1`, reqOne.ID, time.Unix(100, 0).UTC()); err != nil {
		t.Fatalf("update reqOne created_at: %v", err)
	}
	if _, err := stores.console.Pool().Exec(stores.ctx, `UPDATE approval_requests SET created_at = $2 WHERE id = $1`, reqTwo.ID, time.Unix(200, 0).UTC()); err != nil {
		t.Fatalf("update reqTwo created_at: %v", err)
	}

	pending, err := stores.approvals.ListPending(stores.ctx, tenant.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(pending) != 2 || pending[0].ID != reqTwo.ID || pending[1].ID != reqOne.ID {
		t.Fatalf("expected newest-first pending list, got %+v", pending)
	}

	grant, err := stores.approvals.GrantRequest(stores.ctx, reqOne.ID, GrantInput{Approver: "alice@example.com"})
	if err != nil {
		t.Fatalf("GrantRequest: %v", err)
	}
	if grant == nil || grant.RequestID != reqOne.ID || grant.UsesLeft != 1 {
		t.Fatalf("unexpected grant: %+v", grant)
	}

	if err := stores.approvals.DenyRequest(stores.ctx, reqTwo.ID, DenyInput{Approver: "bob@example.com", Reason: "too risky"}); err != nil {
		t.Fatalf("DenyRequest: %v", err)
	}

	gotReqOne, err := stores.approvals.GetRequest(stores.ctx, reqOne.ID)
	if err != nil {
		t.Fatalf("GetRequest reqOne: %v", err)
	}
	if gotReqOne == nil || gotReqOne.Status != "approved" {
		t.Fatalf("expected approved request, got %+v", gotReqOne)
	}
	gotReqTwo, err := stores.approvals.GetRequest(stores.ctx, reqTwo.ID)
	if err != nil {
		t.Fatalf("GetRequest reqTwo: %v", err)
	}
	if gotReqTwo == nil || gotReqTwo.Status != "denied" || gotReqTwo.DenyReason != "too risky" {
		t.Fatalf("expected denied request with reason, got %+v", gotReqTwo)
	}

	pending, err = stores.approvals.ListPending(stores.ctx, tenant.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListPending after transitions: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending requests after grant/deny, got %+v", pending)
	}

	consumed, err := stores.approvals.FindAndConsumeGrant(stores.ctx, tenant.ID, "agent-1", "slack", "msg.post", "channel/general")
	if err != nil {
		t.Fatalf("FindAndConsumeGrant: %v", err)
	}
	if consumed == nil || consumed.ID != grant.ID || consumed.UsesLeft != 0 {
		t.Fatalf("expected one-time grant to be consumed exactly once, got %+v", consumed)
	}
	consumedAgain, err := stores.approvals.FindAndConsumeGrant(stores.ctx, tenant.ID, "agent-1", "slack", "msg.post", "channel/general")
	if err != nil {
		t.Fatalf("FindAndConsumeGrant second call: %v", err)
	}
	if consumedAgain != nil {
		t.Fatalf("expected consumed grant to disappear, got %+v", consumedAgain)
	}
}

func TestStoreConcurrentActionsResolveOnlyOnce(t *testing.T) {
	stores := newApprovalStores(t)
	tenant := mustCreateTenant(t, stores.ctx, stores.console, "Racy Tenant")

	seedApprovalEvent(t, stores, tenant.ID, "33333333-3333-3333-3333-333333333333", "race-grant")
	req, err := stores.approvals.CreateRequest(stores.ctx, CreateApprovalInput{
		EventID:         "33333333-3333-3333-3333-333333333333",
		TenantID:        tenant.ID,
		AgentID:         "agent-1",
		Tool:            "slack",
		Action:          "msg.post",
		Resource:        "channel/general",
		ApprovalBaseURL: "https://approvals.example.com",
	})
	if err != nil {
		t.Fatalf("CreateRequest grant race: %v", err)
	}

	var grantSuccesses atomic.Int32
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func(i int) {
			defer wg.Done()
			_, err := stores.approvals.GrantRequest(stores.ctx, req.ID, GrantInput{Approver: "approver-" + string(rune('a'+i))})
			if err == nil {
				grantSuccesses.Add(1)
			}
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)

	var conflictCount int
	for err := range errs {
		if err == nil {
			continue
		}
		if !errors.Is(err, ErrApprovalRequestNotPendingOrExpired) {
			t.Fatalf("expected conflict sentinel, got %v", err)
		}
		conflictCount++
	}
	if grantSuccesses.Load() != 1 || conflictCount != 1 {
		t.Fatalf("expected one successful grant and one conflict, got successes=%d conflicts=%d", grantSuccesses.Load(), conflictCount)
	}

	var grantRows int
	if err := stores.console.Pool().QueryRow(stores.ctx, `SELECT COUNT(*) FROM approval_grants WHERE request_id = $1`, req.ID).Scan(&grantRows); err != nil {
		t.Fatalf("count grant rows: %v", err)
	}
	if grantRows != 1 {
		t.Fatalf("expected exactly one grant row, got %d", grantRows)
	}

	seedApprovalEvent(t, stores, tenant.ID, "44444444-4444-4444-4444-444444444444", "race-mixed")
	mixedReq, err := stores.approvals.CreateRequest(stores.ctx, CreateApprovalInput{
		EventID:         "44444444-4444-4444-4444-444444444444",
		TenantID:        tenant.ID,
		AgentID:         "agent-1",
		Tool:            "slack",
		Action:          "msg.post",
		Resource:        "channel/general",
		ApprovalBaseURL: "https://approvals.example.com",
	})
	if err != nil {
		t.Fatalf("CreateRequest mixed race: %v", err)
	}

	results := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := stores.approvals.GrantRequest(stores.ctx, mixedReq.ID, GrantInput{Approver: "grant@example.com"})
		results <- err
	}()
	go func() {
		defer wg.Done()
		results <- stores.approvals.DenyRequest(stores.ctx, mixedReq.ID, DenyInput{Approver: "deny@example.com", Reason: "deny race"})
	}()
	wg.Wait()
	close(results)

	var mixedSuccesses int
	var mixedConflicts int
	for err := range results {
		if err == nil {
			mixedSuccesses++
			continue
		}
		if !errors.Is(err, ErrApprovalRequestNotPendingOrExpired) {
			t.Fatalf("expected race conflict sentinel, got %v", err)
		}
		mixedConflicts++
	}
	if mixedSuccesses != 1 || mixedConflicts != 1 {
		t.Fatalf("expected exactly one terminal action in mixed race, got successes=%d conflicts=%d", mixedSuccesses, mixedConflicts)
	}
	gotReq, err := stores.approvals.GetRequest(stores.ctx, mixedReq.ID)
	if err != nil {
		t.Fatalf("GetRequest mixed race: %v", err)
	}
	if gotReq == nil || (gotReq.Status != "approved" && gotReq.Status != "denied") {
		t.Fatalf("expected mixed race request to resolve once, got %+v", gotReq)
	}
}

func TestStoreExpiryAndNotificationClaiming(t *testing.T) {
	stores := newApprovalStores(t)
	tenant := mustCreateTenant(t, stores.ctx, stores.console, "Expiry Tenant")

	seedApprovalEvent(t, stores, tenant.ID, "55555555-5555-5555-5555-555555555555", "expired")
	expiredReq, err := stores.approvals.CreateRequest(stores.ctx, CreateApprovalInput{
		EventID:         "55555555-5555-5555-5555-555555555555",
		TenantID:        tenant.ID,
		AgentID:         "agent-1",
		Tool:            "slack",
		Action:          "msg.post",
		Resource:        "channel/general",
		ApprovalBaseURL: "https://approvals.example.com",
	})
	if err != nil {
		t.Fatalf("CreateRequest expired: %v", err)
	}
	if _, err := stores.console.Pool().Exec(stores.ctx, `UPDATE approval_requests SET expires_at = $2 WHERE id = $1`, expiredReq.ID, time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatalf("expire request: %v", err)
	}

	pending, err := stores.approvals.ListPending(stores.ctx, tenant.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListPending after expiry: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected expired request to be excluded from pending list, got %+v", pending)
	}
	if _, err := stores.approvals.GrantRequest(stores.ctx, expiredReq.ID, GrantInput{Approver: "expired@example.com"}); !errors.Is(err, ErrApprovalRequestNotPendingOrExpired) {
		t.Fatalf("expected expired grant to be rejected with sentinel, got %v", err)
	}
	if err := stores.approvals.DenyRequest(stores.ctx, expiredReq.ID, DenyInput{Approver: "expired@example.com"}); !errors.Is(err, ErrApprovalRequestNotPendingOrExpired) {
		t.Fatalf("expected expired deny to be rejected with sentinel, got %v", err)
	}

	seedApprovalEvent(t, stores, tenant.ID, "66666666-6666-6666-6666-666666666666", "claimable")
	claimableReq, err := stores.approvals.CreateRequest(stores.ctx, CreateApprovalInput{
		EventID:         "66666666-6666-6666-6666-666666666666",
		TenantID:        tenant.ID,
		AgentID:         "agent-1",
		Tool:            "slack",
		Action:          "msg.post",
		Resource:        "channel/general",
		Notify:          []types.PolicyNotify{{Kind: "webhook", URL: "https://example.com/hook", SecretRef: "s1"}},
		ApprovalBaseURL: "https://approvals.example.com",
	})
	if err != nil {
		t.Fatalf("CreateRequest claimable: %v", err)
	}
	if claimableReq == nil {
		t.Fatalf("expected claimable request")
	}

	claims := make(chan []NotificationOutbox, 2)
	wg := sync.WaitGroup{}
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			items, err := stores.approvals.ClaimDueNotifications(stores.ctx, 1)
			if err != nil {
				t.Errorf("ClaimDueNotifications: %v", err)
				return
			}
			claims <- items
		}()
	}
	wg.Wait()
	close(claims)

	var totalClaimed int
	for items := range claims {
		totalClaimed += len(items)
	}
	if totalClaimed != 1 {
		t.Fatalf("expected exactly one claimed notification across racing workers, got %d", totalClaimed)
	}
}
