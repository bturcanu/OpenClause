package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bturcanu/OpenClause/internal/testdb"
	"github.com/bturcanu/OpenClause/pkg/console"
	"github.com/bturcanu/OpenClause/pkg/evidence"
	"github.com/bturcanu/OpenClause/pkg/types"
	"github.com/go-chi/chi/v5"
)

type dbAPIFixture struct {
	harness  *testdb.Harness
	store    *console.Store
	evidence *evidence.Store
	api      *ConsoleAPI
}

func newDBAPIFixture(t *testing.T) *dbAPIFixture {
	t.Helper()
	t.Setenv("INVITE_RESET_TOKEN_HMAC_SECRET", "test-invite-secret")

	h := testdb.New(t)
	store := console.NewStore(h.Pool())
	api := &ConsoleAPI{
		log:                     slog.New(slog.NewTextHandler(io.Discard, nil)),
		store:                   store,
		inviteStore:             store,
		sessionsStore:           store,
		analyticsStore:          store,
		alertsStore:             store,
		notificationConfigStore: store,
		exportStore:             store,
		authSessionStore:        store,
		publicBaseURL:           "https://console.example.com",
	}
	return &dbAPIFixture{
		harness:  h,
		store:    store,
		evidence: evidence.NewStore(h.Pool()),
		api:      api,
	}
}

func withClaims(req *http.Request, claims *console.JWTClaims) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), claimsKey{}, claims))
}

func withRouteParams(req *http.Request, params map[string]string) *http.Request {
	routeCtx := chi.NewRouteContext()
	for key, value := range params {
		routeCtx.URLParams.Add(key, value)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
}

func mustCreateTenantDB(t *testing.T, store *console.Store, name string) *console.Tenant {
	t.Helper()
	tenant, err := store.CreateTenant(context.Background(), name, nil)
	if err != nil {
		t.Fatalf("CreateTenant(%q): %v", name, err)
	}
	return tenant
}

func TestHandleSetupStatusReportsInitializedStateAndFailure(t *testing.T) {
	t.Run("uninitialized then initialized", func(t *testing.T) {
		fx := newDBAPIFixture(t)

		req := httptest.NewRequest(http.MethodGet, "/setup/status", nil)
		rr := httptest.NewRecorder()
		fx.api.handleSetupStatus(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		var payload map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode uninitialized payload: %v", err)
		}
		if payload["initialized"] != false {
			t.Fatalf("expected initialized=false, got %+v", payload)
		}

		if _, err := fx.store.CreateUser(context.Background(), "admin@example.com", "Admin123!", "Admin", "platform_admin", nil, nil); err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		rr = httptest.NewRecorder()
		fx.api.handleSetupStatus(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 after init, got %d body=%s", rr.Code, rr.Body.String())
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode initialized payload: %v", err)
		}
		if payload["initialized"] != true {
			t.Fatalf("expected initialized=true, got %+v", payload)
		}
	})

	t.Run("store failure maps to 500", func(t *testing.T) {
		fx := newDBAPIFixture(t)
		fx.store.Pool().Close()

		req := httptest.NewRequest(http.MethodGet, "/setup/status", nil)
		rr := httptest.NewRecorder()
		fx.api.handleSetupStatus(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d body=%s", rr.Code, rr.Body.String())
		}
	})
}

func TestHandleSetupInitializeValidatesAndIsIdempotent(t *testing.T) {
	fx := newDBAPIFixture(t)

	invalidReq := httptest.NewRequest(http.MethodPost, "/setup/initialize", bytes.NewBufferString(`{"org_name":"Acme","email":"admin@example.com","password":"Admin123!","first_tenant_name":"   "}`))
	invalidReq.Header.Set("Content-Type", "application/json")
	invalidRR := httptest.NewRecorder()
	fx.api.handleSetupInitialize(invalidRR, invalidReq)
	if invalidRR.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for blank tenant name, got %d body=%s", invalidRR.Code, invalidRR.Body.String())
	}

	body := `{"org_name":"Acme Org","email":"admin@example.com","password":"Admin123!","first_tenant_name":"  First Tenant  "}`
	req := httptest.NewRequest(http.MethodPost, "/setup/initialize", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	fx.api.handleSetupInitialize(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode setup payload: %v", err)
	}
	if payload["initialized"] != true {
		t.Fatalf("expected initialized=true, got %+v", payload)
	}
	tenantID, _ := payload["tenant_id"].(string)
	if tenantID == "" {
		t.Fatalf("expected tenant_id in setup response, got %+v", payload)
	}

	user, err := fx.store.GetUserByEmail(context.Background(), "admin@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if user == nil || user.Name != "Admin" || user.Status != "active" {
		t.Fatalf("unexpected created admin user: %+v", user)
	}
	if _, _, err := fx.store.AuthenticateUser(context.Background(), "admin@example.com", "Admin123!"); err != nil {
		t.Fatalf("expected setup password to work: %v", err)
	}
	tenant, err := fx.store.GetTenant(context.Background(), tenantID)
	if err != nil {
		t.Fatalf("GetTenant: %v", err)
	}
	if tenant == nil || tenant.Name != "First Tenant" {
		t.Fatalf("unexpected created tenant: %+v", tenant)
	}

	secondRR := httptest.NewRecorder()
	fx.api.handleSetupInitialize(secondRR, req)
	if secondRR.Code != http.StatusConflict {
		t.Fatalf("expected 409 for second initialization, got %d body=%s", secondRR.Code, secondRR.Body.String())
	}
}

func TestHandleResetConfirmSuccessAndMissingUserContract(t *testing.T) {
	t.Run("success updates password", func(t *testing.T) {
		fx := newDBAPIFixture(t)
		if _, err := fx.store.CreateUser(context.Background(), "reset@example.com", "OldPass123!", "Reset User", "platform_admin", nil, nil); err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		if err := fx.store.CreatePasswordReset(context.Background(), "reset-token", "reset@example.com", time.Now().UTC().Add(time.Hour)); err != nil {
			t.Fatalf("CreatePasswordReset: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/auth/reset/confirm", bytes.NewBufferString(`{"token":"reset-token","password":"NewPass123!"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		fx.api.handleResetConfirm(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		if _, _, err := fx.store.AuthenticateUser(context.Background(), "reset@example.com", "NewPass123!"); err != nil {
			t.Fatalf("expected new password to authenticate: %v", err)
		}
	})

	t.Run("missing user is a 400 not a 500", func(t *testing.T) {
		fx := newDBAPIFixture(t)
		if err := fx.store.CreatePasswordReset(context.Background(), "missing-user-token", "missing@example.com", time.Now().UTC().Add(time.Hour)); err != nil {
			t.Fatalf("CreatePasswordReset: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/auth/reset/confirm", bytes.NewBufferString(`{"token":"missing-user-token","password":"NewPass123!"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		fx.api.handleResetConfirm(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
		}
	})
}

func TestTenantAgentAndAPIKeyHandlersWithRealStore(t *testing.T) {
	fx := newDBAPIFixture(t)

	createTenantReq := withClaims(
		httptest.NewRequest(http.MethodPost, "/admin/tenants", bytes.NewBufferString(`{"name":"  Tenant One  "}`)),
		&console.JWTClaims{Roles: []string{"platform_admin"}},
	)
	createTenantReq.Header.Set("Content-Type", "application/json")
	createTenantRR := httptest.NewRecorder()
	fx.api.handleCreateTenant(createTenantRR, createTenantReq)
	if createTenantRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", createTenantRR.Code, createTenantRR.Body.String())
	}
	var createdTenant console.Tenant
	if err := json.Unmarshal(createTenantRR.Body.Bytes(), &createdTenant); err != nil {
		t.Fatalf("decode created tenant: %v", err)
	}
	if createdTenant.Name != "Tenant One" || createdTenant.Status != "active" {
		t.Fatalf("unexpected created tenant: %+v", createdTenant)
	}

	secondTenant := mustCreateTenantDB(t, fx.store, "Tenant Two")

	listTenantReq := withClaims(
		httptest.NewRequest(http.MethodGet, "/admin/tenants?limit=10", nil),
		&console.JWTClaims{Roles: []string{"platform_admin"}},
	)
	listTenantRR := httptest.NewRecorder()
	fx.api.handleListTenants(listTenantRR, listTenantReq)
	if listTenantRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", listTenantRR.Code, listTenantRR.Body.String())
	}
	var allTenants []console.Tenant
	if err := json.Unmarshal(listTenantRR.Body.Bytes(), &allTenants); err != nil {
		t.Fatalf("decode tenant list: %v", err)
	}
	if len(allTenants) != 2 {
		t.Fatalf("expected two tenants, got %+v", allTenants)
	}

	scopedListReq := withClaims(
		httptest.NewRequest(http.MethodGet, "/admin/tenants", nil),
		&console.JWTClaims{Tenant: secondTenant.ID, Roles: []string{"tenant_admin"}},
	)
	scopedListRR := httptest.NewRecorder()
	fx.api.handleListTenants(scopedListRR, scopedListReq)
	if scopedListRR.Code != http.StatusOK {
		t.Fatalf("expected 200 for tenant-scoped list, got %d body=%s", scopedListRR.Code, scopedListRR.Body.String())
	}
	var scopedTenants []console.Tenant
	if err := json.Unmarshal(scopedListRR.Body.Bytes(), &scopedTenants); err != nil {
		t.Fatalf("decode scoped tenant list: %v", err)
	}
	if len(scopedTenants) != 1 || scopedTenants[0].ID != secondTenant.ID {
		t.Fatalf("expected tenant-scoped list to return only %q, got %+v", secondTenant.ID, scopedTenants)
	}

	updateStatusReq := withRouteParams(
		withClaims(
			httptest.NewRequest(http.MethodPost, "/admin/tenants/"+createdTenant.ID+"/status", bytes.NewBufferString(`{"status":"disabled"}`)),
			&console.JWTClaims{Roles: []string{"platform_admin"}},
		),
		map[string]string{"tenant_id": createdTenant.ID},
	)
	updateStatusReq.Header.Set("Content-Type", "application/json")
	updateStatusRR := httptest.NewRecorder()
	fx.api.handleUpdateTenantStatus(updateStatusRR, updateStatusReq)
	if updateStatusRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", updateStatusRR.Code, updateStatusRR.Body.String())
	}
	disabledTenant, err := fx.store.GetTenant(context.Background(), createdTenant.ID)
	if err != nil {
		t.Fatalf("GetTenant after status update: %v", err)
	}
	if disabledTenant == nil || disabledTenant.Status != "disabled" {
		t.Fatalf("expected tenant to be disabled, got %+v", disabledTenant)
	}

	missingAgentReq := withRouteParams(
		httptest.NewRequest(http.MethodPost, "/admin/tenants/missing/agents", bytes.NewBufferString(`{"name":"worker"}`)),
		map[string]string{"tenant_id": "missing"},
	)
	missingAgentReq.Header.Set("Content-Type", "application/json")
	missingAgentRR := httptest.NewRecorder()
	fx.api.handleCreateAgent(missingAgentRR, missingAgentReq)
	if missingAgentRR.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing tenant agent create, got %d body=%s", missingAgentRR.Code, missingAgentRR.Body.String())
	}

	createAgentReq := withRouteParams(
		httptest.NewRequest(http.MethodPost, "/admin/tenants/"+secondTenant.ID+"/agents", bytes.NewBufferString(`{"name":"  worker-1  "}`)),
		map[string]string{"tenant_id": secondTenant.ID},
	)
	createAgentReq.Header.Set("Content-Type", "application/json")
	createAgentRR := httptest.NewRecorder()
	fx.api.handleCreateAgent(createAgentRR, createAgentReq)
	if createAgentRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", createAgentRR.Code, createAgentRR.Body.String())
	}

	listAgentsReq := withRouteParams(
		httptest.NewRequest(http.MethodGet, "/admin/tenants/"+secondTenant.ID+"/agents", nil),
		map[string]string{"tenant_id": secondTenant.ID},
	)
	listAgentsRR := httptest.NewRecorder()
	fx.api.handleListAgents(listAgentsRR, listAgentsReq)
	if listAgentsRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", listAgentsRR.Code, listAgentsRR.Body.String())
	}
	var agents []console.Agent
	if err := json.Unmarshal(listAgentsRR.Body.Bytes(), &agents); err != nil {
		t.Fatalf("decode agent list: %v", err)
	}
	if len(agents) != 1 || agents[0].Name != "worker-1" {
		t.Fatalf("unexpected agent list: %+v", agents)
	}

	createKeyReq := withRouteParams(
		httptest.NewRequest(http.MethodPost, "/admin/tenants/"+secondTenant.ID+"/apikeys", bytes.NewBufferString(`{"name":"  primary key  "}`)),
		map[string]string{"tenant_id": secondTenant.ID},
	)
	createKeyReq.Header.Set("Content-Type", "application/json")
	createKeyRR := httptest.NewRecorder()
	fx.api.handleCreateAPIKey(createKeyRR, createKeyReq)
	if createKeyRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", createKeyRR.Code, createKeyRR.Body.String())
	}
	var createdKey console.APIKeyCreateResult
	if err := json.Unmarshal(createKeyRR.Body.Bytes(), &createdKey); err != nil {
		t.Fatalf("decode api key create result: %v", err)
	}
	if createdKey.RawKey == "" || !createdKey.APIKey.IsPrimary || createdKey.APIKey.Name != "primary key" {
		t.Fatalf("unexpected created api key: %+v", createdKey)
	}

	listKeysReq := withRouteParams(
		httptest.NewRequest(http.MethodGet, "/admin/tenants/"+secondTenant.ID+"/apikeys", nil),
		map[string]string{"tenant_id": secondTenant.ID},
	)
	listKeysRR := httptest.NewRecorder()
	fx.api.handleListAPIKeys(listKeysRR, listKeysReq)
	if listKeysRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", listKeysRR.Code, listKeysRR.Body.String())
	}
	var keys []console.APIKey
	if err := json.Unmarshal(listKeysRR.Body.Bytes(), &keys); err != nil {
		t.Fatalf("decode api keys list: %v", err)
	}
	if len(keys) != 1 || keys[0].ID != createdKey.APIKey.ID || keys[0].Status != "active" {
		t.Fatalf("unexpected api key list: %+v", keys)
	}

	revokeReq := withRouteParams(
		httptest.NewRequest(http.MethodPost, "/admin/tenants/"+secondTenant.ID+"/apikeys/"+createdKey.APIKey.ID+"/revoke", nil),
		map[string]string{"tenant_id": secondTenant.ID, "key_id": createdKey.APIKey.ID},
	)
	revokeRR := httptest.NewRecorder()
	fx.api.handleRevokeAPIKey(revokeRR, revokeReq)
	if revokeRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", revokeRR.Code, revokeRR.Body.String())
	}

	rotateReq := withRouteParams(
		httptest.NewRequest(http.MethodPost, "/admin/tenants/"+secondTenant.ID+"/apikeys/rotate", bytes.NewBufferString(`{"name":"rotated-key","make_primary":true,"revoke_old_primary":true}`)),
		map[string]string{"tenant_id": secondTenant.ID},
	)
	rotateReq.Header.Set("Content-Type", "application/json")
	rotateRR := httptest.NewRecorder()
	fx.api.handleRotateAPIKey(rotateRR, rotateReq)
	if rotateRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rotateRR.Code, rotateRR.Body.String())
	}
	var rotatedKey console.APIKeyCreateResult
	if err := json.Unmarshal(rotateRR.Body.Bytes(), &rotatedKey); err != nil {
		t.Fatalf("decode rotated api key: %v", err)
	}
	if rotatedKey.RawKey == "" || !rotatedKey.APIKey.IsPrimary {
		t.Fatalf("unexpected rotated api key: %+v", rotatedKey)
	}
}

func TestEventHandlersFilterAndReturnPolicyAndHashFields(t *testing.T) {
	fx := newDBAPIFixture(t)
	tenant := mustCreateTenantDB(t, fx.store, "Audit Tenant")

	localBefore := time.Date(2026, 3, 20, 11, 0, 0, 0, time.Local)
	localInside := time.Date(2026, 3, 20, 12, 30, 0, 0, time.Local)
	localAfter := time.Date(2026, 3, 20, 14, 0, 0, 0, time.Local)

	beforeEnv := &types.ToolCallEnvelope{
		EventID: "11111111-1111-1111-1111-111111111111",
		Request: types.ToolCallRequest{
			TenantID:       tenant.ID,
			AgentID:        "agent-a",
			Tool:           "slack",
			Action:         "msg.post",
			IdempotencyKey: "before-window",
			SessionID:      "sess-before",
			TraceID:        "trace-before",
			RequestedAt:    localBefore.UTC(),
		},
		PayloadJSON:  []byte(`{"tenant_id":"` + tenant.ID + `","agent_id":"agent-a","tool":"slack","action":"msg.post","idempotency_key":"before-window","session_id":"sess-before","trace_id":"trace-before"}`),
		ReceivedAt:   localBefore.UTC(),
		Decision:     types.DecisionDeny,
		PolicyResult: &types.PolicyResult{Decision: types.DecisionDeny, Reason: "before"},
	}
	insideEnv := &types.ToolCallEnvelope{
		EventID: "22222222-2222-2222-2222-222222222222",
		Request: types.ToolCallRequest{
			TenantID:       tenant.ID,
			AgentID:        "agent-a",
			Tool:           "slack",
			Action:         "msg.post",
			IdempotencyKey: "inside-window",
			SessionID:      "sess-123",
			TraceID:        "trace-123",
			RequestedAt:    localInside.UTC(),
		},
		PayloadJSON: []byte(`{"tenant_id":"` + tenant.ID + `","agent_id":"agent-a","tool":"slack","action":"msg.post","idempotency_key":"inside-window","session_id":"sess-123","trace_id":"trace-123"}`),
		ReceivedAt:  localInside.UTC(),
		Decision:    types.DecisionApprove,
		PolicyResult: &types.PolicyResult{
			Decision: types.DecisionApprove,
			Reason:   "needs approval",
		},
		ExecutionResult: &types.ExecutionResult{
			Status:     "success",
			OutputJSON: json.RawMessage(`{"ok":true}`),
			DurationMS: 42,
		},
	}
	afterEnv := &types.ToolCallEnvelope{
		EventID: "33333333-3333-3333-3333-333333333333",
		Request: types.ToolCallRequest{
			TenantID:       tenant.ID,
			AgentID:        "agent-a",
			Tool:           "slack",
			Action:         "msg.post",
			IdempotencyKey: "after-window",
			SessionID:      "sess-after",
			TraceID:        "trace-after",
			RequestedAt:    localAfter.UTC(),
		},
		PayloadJSON:  []byte(`{"tenant_id":"` + tenant.ID + `","agent_id":"agent-a","tool":"slack","action":"msg.post","idempotency_key":"after-window","session_id":"sess-after","trace_id":"trace-after"}`),
		ReceivedAt:   localAfter.UTC(),
		Decision:     types.DecisionAllow,
		PolicyResult: &types.PolicyResult{Decision: types.DecisionAllow, Reason: "after"},
	}

	for _, env := range []*types.ToolCallEnvelope{beforeEnv, insideEnv, afterEnv} {
		if err := fx.evidence.RecordEvent(context.Background(), env); err != nil {
			t.Fatalf("RecordEvent(%s): %v", env.EventID, err)
		}
	}

	since := time.Date(2026, 3, 20, 12, 0, 0, 0, time.Local).Format("2006-01-02T15:04:05")
	until := time.Date(2026, 3, 20, 13, 0, 0, 0, time.Local).Format("2006-01-02T15:04:05")
	listReq := withClaims(
		httptest.NewRequest(http.MethodGet, "/admin/events?since="+since+"&until="+until+"&trace_id=trace-123", nil),
		&console.JWTClaims{Tenant: tenant.ID, Roles: []string{"tenant_admin"}},
	)
	listRR := httptest.NewRecorder()
	fx.api.handleListEvents(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", listRR.Code, listRR.Body.String())
	}
	var listed []map[string]any
	if err := json.Unmarshal(listRR.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode event list: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected one filtered event, got %+v", listed)
	}
	if listed[0]["event_id"] != insideEnv.EventID || listed[0]["session_id"] != "sess-123" || listed[0]["trace_id"] != "trace-123" {
		t.Fatalf("unexpected filtered event payload: %+v", listed[0])
	}

	detailReq := withClaims(
		withRouteParams(
			httptest.NewRequest(http.MethodGet, "/admin/events/"+insideEnv.EventID, nil),
			map[string]string{"event_id": insideEnv.EventID},
		),
		&console.JWTClaims{Tenant: tenant.ID, Roles: []string{"tenant_admin"}},
	)
	detailRR := httptest.NewRecorder()
	fx.api.handleGetEventDetail(detailRR, detailReq)
	if detailRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", detailRR.Code, detailRR.Body.String())
	}
	var detail map[string]any
	if err := json.Unmarshal(detailRR.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode event detail: %v", err)
	}
	if detail["hash"] != insideEnv.Hash || detail["prev_hash"] != insideEnv.PrevHash {
		t.Fatalf("expected hash chain fields to round-trip, got %+v", detail)
	}
	policyResult, ok := detail["policy_result"].(map[string]any)
	if !ok || policyResult["reason"] != "needs approval" {
		t.Fatalf("expected policy_result.reason to round-trip, got %+v", detail["policy_result"])
	}
}
