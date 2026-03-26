package console

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/bturcanu/OpenClause/internal/testdb"
	"github.com/bturcanu/OpenClause/pkg/evidence"
	"github.com/bturcanu/OpenClause/pkg/types"
)

func newIntegrationStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	t.Setenv("INVITE_RESET_TOKEN_HMAC_SECRET", "test-invite-secret")
	h := testdb.New(t)
	return NewStore(h.Pool()), context.Background()
}

func mustCreateTenant(t *testing.T, ctx context.Context, store *Store, name string) *Tenant {
	t.Helper()
	tenant, err := store.CreateTenant(ctx, name, nil)
	if err != nil {
		t.Fatalf("CreateTenant(%q): %v", name, err)
	}
	return tenant
}

func TestStoreCreateInviteHashesTokenAndConsumeInviteAcceptCreatesScopedUser(t *testing.T) {
	store, ctx := newIntegrationStore(t)
	tenant := mustCreateTenant(t, ctx, store, "Acme")

	rawToken := "invite-raw-token"
	if err := store.CreateInvite(ctx, rawToken, "invitee@example.com", tenant.ID, "viewer", "Taylor", time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	var storedToken string
	if err := store.Pool().QueryRow(ctx, `SELECT token FROM invites`).Scan(&storedToken); err != nil {
		t.Fatalf("select stored invite token: %v", err)
	}
	if storedToken == rawToken {
		t.Fatalf("expected invite token to be hashed at rest")
	}
	if storedToken != store.hashInviteResetToken(rawToken) {
		t.Fatalf("expected stored invite token hash %q, got %q", store.hashInviteResetToken(rawToken), storedToken)
	}

	invites, err := store.ListInvites(ctx, &tenant.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListInvites: %v", err)
	}
	if len(invites) != 1 || invites[0].Token != storedToken {
		t.Fatalf("expected hashed invite to be listed once, got %+v", invites)
	}

	res, err := store.ConsumeInviteAccept(ctx, rawToken, "Admin123!", "Taylor Accepted")
	if err != nil {
		t.Fatalf("ConsumeInviteAccept: %v", err)
	}
	if res == nil || res.User == nil {
		t.Fatalf("expected invite accept result with user, got %+v", res)
	}
	if res.TenantID != tenant.ID || res.Role != "viewer" {
		t.Fatalf("unexpected invite accept scope: %+v", res)
	}
	if res.User.Email != "invitee@example.com" || res.User.Name != "Taylor Accepted" || res.User.Status != "active" {
		t.Fatalf("unexpected accepted user: %+v", res.User)
	}

	user, roles, err := store.AuthenticateUser(ctx, "invitee@example.com", "Admin123!")
	if err != nil {
		t.Fatalf("AuthenticateUser after invite accept: %v", err)
	}
	if user.ID != res.User.ID {
		t.Fatalf("expected accepted user id %q, got %q", res.User.ID, user.ID)
	}
	if len(roles) != 1 || roles[0].Role != "viewer" || roles[0].TenantID == nil || *roles[0].TenantID != tenant.ID {
		t.Fatalf("unexpected accepted roles: %+v", roles)
	}

	var remainingInvites int
	if err := store.Pool().QueryRow(ctx, `SELECT COUNT(*) FROM invites`).Scan(&remainingInvites); err != nil {
		t.Fatalf("count invites: %v", err)
	}
	if remainingInvites != 0 {
		t.Fatalf("expected invite to be deleted after acceptance, got %d rows", remainingInvites)
	}
}

func TestStoreConsumePasswordResetUpdatesPasswordAndDeletesToken(t *testing.T) {
	store, ctx := newIntegrationStore(t)

	user, err := store.CreateUser(ctx, "reset@example.com", "OldPass123!", "Reset User", "platform_admin", nil, nil)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	rawToken := "reset-raw-token"
	if err := store.CreatePasswordReset(ctx, rawToken, user.Email, time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("CreatePasswordReset: %v", err)
	}

	var storedToken string
	if err := store.Pool().QueryRow(ctx, `SELECT token FROM password_resets`).Scan(&storedToken); err != nil {
		t.Fatalf("select stored reset token: %v", err)
	}
	if storedToken == rawToken {
		t.Fatalf("expected reset token to be hashed at rest")
	}
	if storedToken != store.hashInviteResetToken(rawToken) {
		t.Fatalf("expected stored reset token hash %q, got %q", store.hashInviteResetToken(rawToken), storedToken)
	}

	if err := store.ConsumePasswordReset(ctx, rawToken, "NewPass123!"); err != nil {
		t.Fatalf("ConsumePasswordReset: %v", err)
	}

	if _, _, err := store.AuthenticateUser(ctx, user.Email, "OldPass123!"); err == nil {
		t.Fatalf("expected old password to stop working after reset")
	}
	if _, _, err := store.AuthenticateUser(ctx, user.Email, "NewPass123!"); err != nil {
		t.Fatalf("expected new password to work after reset: %v", err)
	}

	var remainingResets int
	if err := store.Pool().QueryRow(ctx, `SELECT COUNT(*) FROM password_resets`).Scan(&remainingResets); err != nil {
		t.Fatalf("count password resets: %v", err)
	}
	if remainingResets != 0 {
		t.Fatalf("expected password reset token to be deleted, got %d rows", remainingResets)
	}
}

func TestStoreListTenantsPagingAndStatusTransitions(t *testing.T) {
	store, ctx := newIntegrationStore(t)

	first := mustCreateTenant(t, ctx, store, "First")
	second := mustCreateTenant(t, ctx, store, "Second")
	third := mustCreateTenant(t, ctx, store, "Third")

	if _, err := store.Pool().Exec(ctx, `UPDATE tenants SET created_at = $2 WHERE id = $1`, first.ID, time.Unix(100, 0).UTC()); err != nil {
		t.Fatalf("update first tenant timestamp: %v", err)
	}
	if _, err := store.Pool().Exec(ctx, `UPDATE tenants SET created_at = $2 WHERE id = $1`, second.ID, time.Unix(200, 0).UTC()); err != nil {
		t.Fatalf("update second tenant timestamp: %v", err)
	}
	if _, err := store.Pool().Exec(ctx, `UPDATE tenants SET created_at = $2 WHERE id = $1`, third.ID, time.Unix(300, 0).UTC()); err != nil {
		t.Fatalf("update third tenant timestamp: %v", err)
	}

	if err := store.UpdateTenantStatus(ctx, second.ID, "disabled"); err != nil {
		t.Fatalf("UpdateTenantStatus: %v", err)
	}
	gotSecond, err := store.GetTenant(ctx, second.ID)
	if err != nil {
		t.Fatalf("GetTenant: %v", err)
	}
	if gotSecond == nil || gotSecond.Status != "disabled" {
		t.Fatalf("expected second tenant to be disabled, got %+v", gotSecond)
	}

	tenants, err := store.ListTenants(ctx, 2, 1)
	if err != nil {
		t.Fatalf("ListTenants: %v", err)
	}
	if len(tenants) != 2 {
		t.Fatalf("expected 2 tenants in paged response, got %d", len(tenants))
	}
	if tenants[0].ID != second.ID || tenants[1].ID != first.ID {
		t.Fatalf("expected tenants ordered newest-first with offset applied, got %+v", tenants)
	}
}

func TestStoreListEventsInRangeIncludesPolicyReason(t *testing.T) {
	store, ctx := newIntegrationStore(t)
	tenant := mustCreateTenant(t, ctx, store, "Export Tenant")
	eventTime := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	env := &types.ToolCallEnvelope{
		EventID: "00000000-0000-0000-0000-000000000111",
		Request: types.ToolCallRequest{
			TenantID:       tenant.ID,
			AgentID:        "agent-export",
			Tool:           "slack",
			Action:         "msg.post",
			Resource:       "channels/general",
			RiskScore:      4,
			UserID:         "user-export",
			SessionID:      "export-session",
			TraceID:        "export-trace",
			IdempotencyKey: "export-1",
			RequestedAt:    eventTime,
		},
		PayloadJSON: []byte(`{"tenant_id":"` + tenant.ID + `","agent_id":"agent-export","tool":"slack","action":"msg.post","resource":"channels/general","session_id":"export-session","trace_id":"export-trace"}`),
		ReceivedAt:  eventTime,
		Decision:    types.DecisionAllow,
		PolicyResult: &types.PolicyResult{
			Decision: types.DecisionAllow,
			Reason:   "export fixture reason",
		},
	}
	if err := evidence.NewStore(store.Pool()).RecordEvent(ctx, env); err != nil {
		t.Fatalf("RecordEvent: %v", err)
	}

	events, err := store.ListEventsInRange(ctx, tenant.ID, eventTime.Add(-time.Hour), eventTime.Add(time.Hour), 10)
	if err != nil {
		t.Fatalf("ListEventsInRange: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 exported event, got %+v", events)
	}
	if events[0].Reason != "export fixture reason" {
		t.Fatalf("expected exported policy reason, got %+v", events[0])
	}
}

func TestStoreAgentsAndAPIKeysLifecycle(t *testing.T) {
	store, ctx := newIntegrationStore(t)
	tenant := mustCreateTenant(t, ctx, store, "Tenant With Agents")

	olderAgent, err := store.CreateAgent(ctx, tenant.ID, "older-agent")
	if err != nil {
		t.Fatalf("CreateAgent older: %v", err)
	}
	newerAgent, err := store.CreateAgent(ctx, tenant.ID, "newer-agent")
	if err != nil {
		t.Fatalf("CreateAgent newer: %v", err)
	}
	if _, err := store.Pool().Exec(ctx, `UPDATE agents SET created_at = $2 WHERE id = $1`, olderAgent.ID, time.Unix(100, 0).UTC()); err != nil {
		t.Fatalf("update older agent timestamp: %v", err)
	}
	if _, err := store.Pool().Exec(ctx, `UPDATE agents SET created_at = $2 WHERE id = $1`, newerAgent.ID, time.Unix(200, 0).UTC()); err != nil {
		t.Fatalf("update newer agent timestamp: %v", err)
	}

	agents, err := store.ListAgents(ctx, tenant.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) != 2 || agents[0].ID != newerAgent.ID || agents[1].ID != olderAgent.ID {
		t.Fatalf("expected tenant-scoped agents ordered newest-first, got %+v", agents)
	}

	firstKey, err := store.CreateAPIKey(ctx, tenant.ID, "first-key", nil)
	if err != nil {
		t.Fatalf("CreateAPIKey first: %v", err)
	}
	if !firstKey.APIKey.IsPrimary || firstKey.RawKey == "" {
		t.Fatalf("expected first api key to be primary with raw key, got %+v", firstKey)
	}
	secondKey, err := store.CreateAPIKey(ctx, tenant.ID, "second-key", nil)
	if err != nil {
		t.Fatalf("CreateAPIKey second: %v", err)
	}
	if secondKey.APIKey.IsPrimary {
		t.Fatalf("expected second api key not to become primary while first is active")
	}

	if err := store.RevokeAPIKeyForTenant(ctx, tenant.ID, secondKey.APIKey.ID); err != nil {
		t.Fatalf("RevokeAPIKeyForTenant: %v", err)
	}
	keysAfterRevoke, err := store.ListAPIKeys(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("ListAPIKeys after revoke: %v", err)
	}
	var revokedFound bool
	for _, key := range keysAfterRevoke {
		if key.ID == secondKey.APIKey.ID {
			revokedFound = true
			if key.Status != "revoked" || key.IsPrimary {
				t.Fatalf("expected revoked key to be non-primary, got %+v", key)
			}
		}
	}
	if !revokedFound {
		t.Fatalf("expected revoked key %q to remain listable", secondKey.APIKey.ID)
	}

	rotated, err := store.RotateAPIKeysPrimary(ctx, tenant.ID, "rotated", nil, true, true)
	if err != nil {
		t.Fatalf("RotateAPIKeysPrimary: %v", err)
	}
	if !rotated.APIKey.IsPrimary || rotated.RawKey == "" {
		t.Fatalf("expected rotated key to be primary with raw value, got %+v", rotated)
	}

	finalKeys, err := store.ListAPIKeys(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("ListAPIKeys final: %v", err)
	}
	var activePrimaryCount int
	var oldPrimaryRevoked bool
	for _, key := range finalKeys {
		if key.Status == "active" && key.IsPrimary {
			activePrimaryCount++
			if key.ID != rotated.APIKey.ID {
				t.Fatalf("expected rotated key to be the only active primary, got %+v", key)
			}
		}
		if key.ID == firstKey.APIKey.ID {
			oldPrimaryRevoked = key.Status == "revoked" && !key.IsPrimary
		}
	}
	if activePrimaryCount != 1 {
		t.Fatalf("expected exactly one active primary key, got %d", activePrimaryCount)
	}
	if !oldPrimaryRevoked {
		t.Fatalf("expected old primary key to be revoked after rotation")
	}

	tenantID, keyID, err := store.LookupAPIKey(ctx, rotated.RawKey)
	if err != nil {
		t.Fatalf("LookupAPIKey for rotated key: %v", err)
	}
	if tenantID != tenant.ID || keyID != rotated.APIKey.ID {
		t.Fatalf("unexpected lookup result: tenant=%q key=%q", tenantID, keyID)
	}
	if _, _, err := store.LookupAPIKey(ctx, firstKey.RawKey); err == nil {
		t.Fatalf("expected revoked primary key lookup to fail")
	}
}

func TestStoreGetTenantAnalyticsSummaryEmptyStateHasStableShape(t *testing.T) {
	store, ctx := newIntegrationStore(t)
	tenant := mustCreateTenant(t, ctx, store, "Analytics Tenant")

	summary, err := store.GetTenantAnalyticsSummary(ctx, tenant.ID, time.Now().UTC().Add(-24*time.Hour), 60, 5)
	if err != nil {
		t.Fatalf("GetTenantAnalyticsSummary: %v", err)
	}
	if summary == nil {
		t.Fatalf("expected non-nil summary")
	}
	if summary.Totals.TotalEvents != 0 || summary.Totals.AllowCount != 0 || summary.Totals.DenyCount != 0 || summary.Totals.ApproveCount != 0 {
		t.Fatalf("expected zero totals for empty tenant, got %+v", summary.Totals)
	}
	if len(summary.Trend) != 0 {
		t.Fatalf("expected empty trend for empty tenant, got %+v", summary.Trend)
	}
	if len(summary.RiskHeatmap) != 11 {
		t.Fatalf("expected stable risk heatmap buckets 0..10, got %d", len(summary.RiskHeatmap))
	}
	for risk, row := range summary.RiskHeatmap {
		if row.RiskScore != risk || row.Total != 0 || row.AllowCount != 0 || row.DenyCount != 0 || row.ApproveCount != 0 {
			t.Fatalf("unexpected empty-state risk row %d: %+v", risk, row)
		}
	}
	if summary.OnboardingChecklist.HasAPIKey || summary.OnboardingChecklist.HasApprover || summary.OnboardingChecklist.HasToolcall || summary.OnboardingChecklist.HasApproval || summary.OnboardingChecklist.HasExecution {
		t.Fatalf("expected empty onboarding checklist, got %+v", summary.OnboardingChecklist)
	}
	if summary.PilotHealth.Status != "setup_required" || len(summary.PilotHealth.TopConnectorFailures) != 0 || len(summary.PilotHealth.TopDenyReasons) != 0 || len(summary.PilotHealth.NextActions) == 0 {
		t.Fatalf("expected stable empty-state pilot health, got %+v", summary.PilotHealth)
	}
}

func TestStoreAlertEventsRetryMetadataLifecycle(t *testing.T) {
	store, ctx := newIntegrationStore(t)
	tenant := mustCreateTenant(t, ctx, store, "Alerts Tenant")
	rule, err := store.CreateAlertRule(ctx, AlertRule{
		TenantID: tenant.ID,
		Name:     "deny-spike",
		RuleType: "deny_spike",
		Config:   json.RawMessage(`{"n":3,"m_minutes":5}`),
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	event, err := store.CreateAlertEvent(ctx, rule.ID, tenant.ID, "warning", "threshold exceeded", json.RawMessage(`{"count":3}`))
	if err != nil {
		t.Fatalf("CreateAlertEvent: %v", err)
	}
	if event.Status != "pending" || event.AttemptCount != 0 || event.LastError != "" {
		t.Fatalf("unexpected new alert event: %+v", event)
	}

	nextAttempt := time.Now().UTC().Add(10 * time.Minute).Round(time.Second)
	if err := store.MarkAlertEventPendingRetry(ctx, event.ID, 2, nextAttempt, "webhook timeout"); err != nil {
		t.Fatalf("MarkAlertEventPendingRetry: %v", err)
	}

	events, err := store.ListAlertEvents(ctx, tenant.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListAlertEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one alert event, got %+v", events)
	}
	if events[0].AttemptCount != 2 || events[0].LastError != "webhook timeout" {
		t.Fatalf("expected retry metadata to persist, got %+v", events[0])
	}
	if events[0].NextAttemptAt.Round(time.Second) != nextAttempt {
		t.Fatalf("expected next attempt %s, got %s", nextAttempt, events[0].NextAttemptAt)
	}

	if _, err := store.Pool().Exec(ctx, `UPDATE alert_events SET next_attempt_at = $2 WHERE id = $1`, event.ID, time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatalf("make alert event due: %v", err)
	}
	due, err := store.ClaimPendingAlertEventsDue(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimPendingAlertEventsDue: %v", err)
	}
	if len(due) != 1 || due[0].ID != event.ID {
		t.Fatalf("expected event to become due once, got %+v", due)
	}

	if err := store.MarkAlertEventSent(ctx, event.ID); err != nil {
		t.Fatalf("MarkAlertEventSent: %v", err)
	}
	since := time.Now().UTC().Add(-time.Hour)
	eventsSince, err := store.ListAlertEventsSince(ctx, tenant.ID, since, 10)
	if err != nil {
		t.Fatalf("ListAlertEventsSince: %v", err)
	}
	if len(eventsSince) != 1 {
		t.Fatalf("expected one alert event since cutoff, got %+v", eventsSince)
	}
	if eventsSince[0].Status != "sent" || eventsSince[0].DeliveredAt == nil || eventsSince[0].LastError != "" {
		t.Fatalf("expected sent alert event with cleared error, got %+v", eventsSince[0])
	}
}
