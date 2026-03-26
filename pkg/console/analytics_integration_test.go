package console

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/bturcanu/OpenClause/pkg/evidence"
	"github.com/bturcanu/OpenClause/pkg/types"
)

type analyticsFixture struct {
	store    *Store
	evidence *evidence.Store
	ctx      context.Context
	tenantID string
	since    time.Time
}

func newAnalyticsFixture(t *testing.T) *analyticsFixture {
	t.Helper()

	store, ctx := newIntegrationStore(t)
	tenant := mustCreateTenant(t, ctx, store, "Analytics Tenant")
	evStore := evidence.NewStore(store.Pool())

	for _, agentID := range []string{"agent-a", "agent-b"} {
		if _, err := store.Pool().Exec(ctx, `
			INSERT INTO agents (id, tenant_id, name, status, labels)
			VALUES ($1, $2, $3, 'active', '{}'::jsonb)`,
			agentID, tenant.ID, agentID,
		); err != nil {
			t.Fatalf("insert analytics agent %s: %v", agentID, err)
		}
	}

	since := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	events := []struct {
		eventID   string
		agentID   string
		decision  types.Decision
		riskScore int
		at        time.Time
	}{
		{eventID: "00000000-0000-0000-0000-000000000001", agentID: "agent-a", decision: types.DecisionAllow, riskScore: 2, at: since.Add(5 * time.Minute)},
		{eventID: "00000000-0000-0000-0000-000000000002", agentID: "agent-b", decision: types.DecisionDeny, riskScore: 8, at: since.Add(20 * time.Minute)},
		{eventID: "00000000-0000-0000-0000-000000000003", agentID: "agent-a", decision: types.DecisionApprove, riskScore: 6, at: since.Add(40 * time.Minute)},
		{eventID: "00000000-0000-0000-0000-000000000004", agentID: "agent-b", decision: types.DecisionAllow, riskScore: 2, at: since.Add(70 * time.Minute)},
		{eventID: "00000000-0000-0000-0000-000000000005", agentID: "agent-a", decision: types.DecisionDeny, riskScore: 8, at: since.Add(85 * time.Minute)},
		{eventID: "00000000-0000-0000-0000-000000000006", agentID: "agent-b", decision: types.DecisionApprove, riskScore: 6, at: since.Add(125 * time.Minute)},
		{eventID: "00000000-0000-0000-0000-000000000007", agentID: "agent-a", decision: types.DecisionAllow, riskScore: 2, at: since.Add(155 * time.Minute)},
		{eventID: "00000000-0000-0000-0000-000000000008", agentID: "agent-b", decision: types.DecisionDeny, riskScore: 8, at: since.Add(170 * time.Minute)},
		{eventID: "00000000-0000-0000-0000-000000000009", agentID: "agent-a", decision: types.DecisionApprove, riskScore: 6, at: since.Add(195 * time.Minute)},
		{eventID: "00000000-0000-0000-0000-000000000010", agentID: "agent-b", decision: types.DecisionAllow, riskScore: 2, at: since.Add(225 * time.Minute)},
	}

	for i, event := range events {
		reason := "analytics fixture"
		if event.decision == types.DecisionDeny {
			reason = "tool/action mismatch in tenant policy"
		}
		req := types.ToolCallRequest{
			TenantID:       tenant.ID,
			AgentID:        event.agentID,
			Tool:           "slack",
			Action:         "msg.post",
			Resource:       "channels/general",
			RiskScore:      event.riskScore,
			UserID:         "user-analytics",
			SessionID:      "analytics-session",
			TraceID:        "analytics-trace",
			IdempotencyKey: fmt.Sprintf("analytics-%02d", i+1),
			RequestedAt:    event.at,
			Labels: map[string]string{
				"user_name":  "Analytics User",
				"user_email": "analytics@example.com",
			},
		}
		payloadJSON, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("marshal analytics payload %s: %v", event.eventID, err)
		}
		env := &types.ToolCallEnvelope{
			EventID:      event.eventID,
			Request:      req,
			PayloadJSON:  payloadJSON,
			ReceivedAt:   event.at,
			Decision:     event.decision,
			PolicyResult: &types.PolicyResult{Decision: event.decision, Reason: reason},
		}
		if err := evStore.RecordEvent(ctx, env); err != nil {
			t.Fatalf("RecordEvent(%s): %v", event.eventID, err)
		}
	}

	return &analyticsFixture{
		store:    store,
		evidence: evStore,
		ctx:      ctx,
		tenantID: tenant.ID,
		since:    since,
	}
}

func TestStoreGetDecisionTimeseriesUsesDeterministicBuckets(t *testing.T) {
	fx := newAnalyticsFixture(t)

	series, err := fx.store.GetDecisionTimeseries(fx.ctx, fx.tenantID, fx.since, 60)
	if err != nil {
		t.Fatalf("GetDecisionTimeseries: %v", err)
	}

	if len(series) != 4 {
		t.Fatalf("expected 4 hourly buckets, got %+v", series)
	}

	expected := []struct {
		bucket       time.Time
		total        int64
		allowCount   int64
		denyCount    int64
		approveCount int64
	}{
		{bucket: fx.since, total: 3, allowCount: 1, denyCount: 1, approveCount: 1},
		{bucket: fx.since.Add(time.Hour), total: 2, allowCount: 1, denyCount: 1, approveCount: 0},
		{bucket: fx.since.Add(2 * time.Hour), total: 3, allowCount: 1, denyCount: 1, approveCount: 1},
		{bucket: fx.since.Add(3 * time.Hour), total: 2, allowCount: 1, denyCount: 0, approveCount: 1},
	}

	for i, want := range expected {
		got := series[i]
		bucket, ok := got["bucket"].(time.Time)
		if !ok {
			t.Fatalf("bucket %d was not a time.Time: %#v", i, got["bucket"])
		}
		if !bucket.Equal(want.bucket) {
			t.Fatalf("bucket %d: expected %s, got %s", i, want.bucket, bucket)
		}
		if got["total"] != want.total || got["allow_count"] != want.allowCount || got["deny_count"] != want.denyCount || got["approve_count"] != want.approveCount {
			t.Fatalf("bucket %d: expected totals %+v, got %+v", i, want, got)
		}
	}
}

func TestStoreGetTenantAnalyticsSummarySeededDataIsDeterministic(t *testing.T) {
	fx := newAnalyticsFixture(t)
	before := time.Now().UTC()

	summary, err := fx.store.GetTenantAnalyticsSummary(fx.ctx, fx.tenantID, fx.since, 60, 5)
	if err != nil {
		t.Fatalf("GetTenantAnalyticsSummary: %v", err)
	}
	after := time.Now().UTC()

	if summary == nil {
		t.Fatalf("expected non-nil summary")
	}
	if !summary.RangeStart.Equal(fx.since) {
		t.Fatalf("expected range_start %s, got %s", fx.since, summary.RangeStart)
	}
	if summary.RangeEnd.Before(before) || summary.RangeEnd.After(after) {
		t.Fatalf("expected range_end between %s and %s, got %s", before, after, summary.RangeEnd)
	}
	if summary.Trend == nil || summary.RiskHeatmap == nil || summary.PerAgent == nil {
		t.Fatalf("expected stable non-nil slices, got summary=%+v", summary)
	}

	if summary.Totals.TotalEvents != 10 || summary.Totals.AllowCount != 4 || summary.Totals.DenyCount != 3 || summary.Totals.ApproveCount != 3 {
		t.Fatalf("unexpected totals: %+v", summary.Totals)
	}

	expectedTrend := []DecisionTrendBucket{
		{Bucket: fx.since, Total: 3, AllowCount: 1, DenyCount: 1, ApproveCount: 1},
		{Bucket: fx.since.Add(time.Hour), Total: 2, AllowCount: 1, DenyCount: 1, ApproveCount: 0},
		{Bucket: fx.since.Add(2 * time.Hour), Total: 3, AllowCount: 1, DenyCount: 1, ApproveCount: 1},
		{Bucket: fx.since.Add(3 * time.Hour), Total: 2, AllowCount: 1, DenyCount: 0, ApproveCount: 1},
	}
	if len(summary.Trend) != len(expectedTrend) {
		t.Fatalf("expected %d trend buckets, got %+v", len(expectedTrend), summary.Trend)
	}
	for i, want := range expectedTrend {
		got := summary.Trend[i]
		if !got.Bucket.Equal(want.Bucket) || got.Total != want.Total || got.AllowCount != want.AllowCount || got.DenyCount != want.DenyCount || got.ApproveCount != want.ApproveCount {
			t.Fatalf("trend bucket %d mismatch: want %+v got %+v", i, want, got)
		}
	}

	if len(summary.PerAgent) != 2 {
		t.Fatalf("expected two per-agent rows, got %+v", summary.PerAgent)
	}
	expectedPerAgent := []AgentBreakdownRow{
		{AgentID: "agent-a", AllowCount: 2, DenyCount: 1, ApproveCount: 2, Total: 5},
		{AgentID: "agent-b", AllowCount: 2, DenyCount: 2, ApproveCount: 1, Total: 5},
	}
	for i, want := range expectedPerAgent {
		if summary.PerAgent[i] != want {
			t.Fatalf("per_agent row %d mismatch: want %+v got %+v", i, want, summary.PerAgent[i])
		}
	}

	if len(summary.RiskHeatmap) != 11 {
		t.Fatalf("expected 11 risk heatmap rows, got %d", len(summary.RiskHeatmap))
	}
	expectedRiskRows := map[int]RiskHeatmapRow{
		2: {RiskScore: 2, AllowCount: 4, DenyCount: 0, ApproveCount: 0, Total: 4},
		6: {RiskScore: 6, AllowCount: 0, DenyCount: 0, ApproveCount: 3, Total: 3},
		8: {RiskScore: 8, AllowCount: 0, DenyCount: 3, ApproveCount: 0, Total: 3},
	}
	for risk, want := range expectedRiskRows {
		if summary.RiskHeatmap[risk] != want {
			t.Fatalf("risk row %d mismatch: want %+v got %+v", risk, want, summary.RiskHeatmap[risk])
		}
	}
	if summary.RiskHeatmap[1].Total != 0 || summary.RiskHeatmap[10].Total != 0 {
		t.Fatalf("expected untouched risk rows to remain zeroed, got row1=%+v row10=%+v", summary.RiskHeatmap[1], summary.RiskHeatmap[10])
	}

	if !summary.OnboardingChecklist.HasToolcall {
		t.Fatalf("expected toolcall onboarding flag to be true, got %+v", summary.OnboardingChecklist)
	}
	if summary.OnboardingChecklist.HasAPIKey || summary.OnboardingChecklist.HasApprover || summary.OnboardingChecklist.HasApproval || summary.OnboardingChecklist.HasExecution {
		t.Fatalf("expected only toolcall onboarding flag to be set, got %+v", summary.OnboardingChecklist)
	}
}

func TestStoreGetTenantAnalyticsSummaryIncludesPilotHealthSignals(t *testing.T) {
	fx := newAnalyticsFixture(t)

	if _, err := fx.store.CreateAPIKey(fx.ctx, fx.tenantID, "pilot-key", nil); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if _, err := fx.store.Pool().Exec(fx.ctx, `
		INSERT INTO users (id, email, name, status)
		VALUES ('approver-1', 'approver@example.com', 'Approver One', 'active')`); err != nil {
		t.Fatalf("insert approver user: %v", err)
	}
	if _, err := fx.store.Pool().Exec(fx.ctx, `
		INSERT INTO user_roles (id, user_id, tenant_id, role)
		VALUES ('role-1', 'approver-1', $1, 'approver')`, fx.tenantID); err != nil {
		t.Fatalf("insert approver role: %v", err)
	}

	if _, err := fx.store.Pool().Exec(fx.ctx, `
		INSERT INTO tool_results (event_id, tenant_id, status, output_json, error_msg, duration_ms)
		VALUES
			('00000000-0000-0000-0000-000000000001', $1, 'success', '{"ok":true}'::jsonb, '', 120),
			('00000000-0000-0000-0000-000000000002', $1, 'error', NULL, 'connector unavailable', 350),
			('00000000-0000-0000-0000-000000000004', $1, 'success', '{"ok":true}'::jsonb, '', 180)`, fx.tenantID); err != nil {
		t.Fatalf("insert tool results: %v", err)
	}

	approvedCreatedAt := fx.since.Add(250 * time.Minute)
	approvedResolvedAt := approvedCreatedAt.Add(2 * time.Minute)
	if _, err := fx.store.Pool().Exec(fx.ctx, `
		INSERT INTO approval_requests (id, event_id, tenant_id, agent_id, tool, action, resource, risk_score, reason, status, created_at, updated_at, expires_at)
		VALUES
			('approval-approved', '00000000-0000-0000-0000-000000000009', $1, 'agent-a', 'slack', 'msg.post', 'channels/general', 6, 'operator review', 'approved', $2, $3, $4),
			('approval-pending', '00000000-0000-0000-0000-000000000006', $1, 'agent-b', 'slack', 'msg.post', 'channels/general', 6, 'pending follow-up', 'pending', $5, NULL, $6)`,
		fx.tenantID,
		approvedCreatedAt,
		approvedResolvedAt,
		approvedCreatedAt.Add(30*time.Minute),
		fx.since.Add(110*time.Minute),
		fx.since.Add(140*time.Minute),
	); err != nil {
		t.Fatalf("insert approval requests: %v", err)
	}
	if _, err := fx.store.Pool().Exec(fx.ctx, `
		INSERT INTO approval_grants (
			id, request_id, tenant_id, approver,
			scope_tool, scope_action, scope_resource_pattern, scope_tenant_id, scope_agent_id,
			max_uses, uses_left, expires_at, granted_at
		)
		VALUES ('grant-1', 'approval-approved', $1, 'approver@example.com', 'slack', 'msg.post', 'channels/general', $1, 'agent-a', 1, 1, $2, $3)`,
		fx.tenantID,
		approvedResolvedAt.Add(15*time.Minute),
		approvedResolvedAt,
	); err != nil {
		t.Fatalf("insert approval grant: %v", err)
	}

	summary, err := fx.store.GetTenantAnalyticsSummary(fx.ctx, fx.tenantID, fx.since, 60, 5)
	if err != nil {
		t.Fatalf("GetTenantAnalyticsSummary: %v", err)
	}

	if !summary.OnboardingChecklist.HasAPIKey || !summary.OnboardingChecklist.HasApprover || !summary.OnboardingChecklist.HasApproval {
		t.Fatalf("expected onboarding checklist to reflect pilot setup, got %+v", summary.OnboardingChecklist)
	}
	if !summary.OnboardingChecklist.HasExecution {
		t.Fatalf("expected onboarding checklist to include execution activity, got %+v", summary.OnboardingChecklist)
	}
	if summary.PilotHealth.Status == "" || summary.PilotHealth.StatusReason == "" {
		t.Fatalf("expected pilot health status and reason, got %+v", summary.PilotHealth)
	}
	if summary.PilotHealth.LastEvent == nil || summary.PilotHealth.LastEvent.EventID != "00000000-0000-0000-0000-000000000010" {
		t.Fatalf("expected last event summary, got %+v", summary.PilotHealth.LastEvent)
	}
	if summary.PilotHealth.LastSession == nil || summary.PilotHealth.LastSession.SessionID != "analytics-session" {
		t.Fatalf("expected last session summary, got %+v", summary.PilotHealth.LastSession)
	}
	if summary.PilotHealth.LastApproval == nil || summary.PilotHealth.LastApproval.RequestID != "approval-approved" {
		t.Fatalf("expected latest approval summary, got %+v", summary.PilotHealth.LastApproval)
	}
	if summary.PilotHealth.LastApproval.LatencyMS == nil || *summary.PilotHealth.LastApproval.LatencyMS != int64(120000) {
		t.Fatalf("expected approval latency, got %+v", summary.PilotHealth.LastApproval)
	}
	if summary.PilotHealth.PendingApprovals != 1 || summary.PilotHealth.OldestPendingApprovalAt == nil {
		t.Fatalf("expected pending approval summary, got %+v", summary.PilotHealth)
	}
	if summary.PilotHealth.ExecutionSuccessCount != 2 || summary.PilotHealth.ExecutionTotal != 3 {
		t.Fatalf("expected execution totals, got %+v", summary.PilotHealth)
	}
	if summary.PilotHealth.ExecutionSuccessRate < 0.66 || summary.PilotHealth.ExecutionSuccessRate > 0.67 {
		t.Fatalf("expected execution success rate near 0.666, got %+v", summary.PilotHealth)
	}
	if summary.PilotHealth.MissingSessionCount != 0 || summary.PilotHealth.MissingTraceCount != 0 {
		t.Fatalf("expected full request context coverage, got %+v", summary.PilotHealth)
	}
	if len(summary.PilotHealth.TopConnectorFailures) != 1 || summary.PilotHealth.TopConnectorFailures[0].ErrorMessage != "connector unavailable" {
		t.Fatalf("expected connector failure summary, got %+v", summary.PilotHealth.TopConnectorFailures)
	}
	if len(summary.PilotHealth.TopDenyReasons) == 0 || summary.PilotHealth.TopDenyReasons[0].Reason != "tool/action mismatch in tenant policy" {
		t.Fatalf("expected deny reason summary, got %+v", summary.PilotHealth.TopDenyReasons)
	}
	if len(summary.PilotHealth.NextActions) == 0 {
		t.Fatalf("expected next actions, got %+v", summary.PilotHealth)
	}
}

func TestStoreListEventsReturnsPolicyResultReason(t *testing.T) {
	fx := newAnalyticsFixture(t)

	events, err := fx.store.ListEvents(fx.ctx, EventListFilters{
		TenantID: fx.tenantID,
		Limit:    20,
	})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 10 {
		t.Fatalf("expected 10 events, got %d", len(events))
	}

	var found bool
	for _, event := range events {
		if event.EventID == "00000000-0000-0000-0000-000000000002" {
			found = true
			if event.Reason != "tool/action mismatch in tenant policy" {
				t.Fatalf("expected policy reason to round-trip, got %+v", event)
			}
		}
	}
	if !found {
		t.Fatalf("expected to find seeded denied event in event list, got %+v", events)
	}
}

func TestStoreGetDecisionTimeseriesSupportsSmallerBucketIntervalsWithoutLosingTotals(t *testing.T) {
	fx := newAnalyticsFixture(t)

	series, err := fx.store.GetDecisionTimeseries(fx.ctx, fx.tenantID, fx.since, 30)
	if err != nil {
		t.Fatalf("GetDecisionTimeseries(30m): %v", err)
	}

	expected := []struct {
		bucket       time.Time
		total        int64
		allowCount   int64
		denyCount    int64
		approveCount int64
	}{
		{bucket: fx.since, total: 2, allowCount: 1, denyCount: 1, approveCount: 0},
		{bucket: fx.since.Add(30 * time.Minute), total: 1, allowCount: 0, denyCount: 0, approveCount: 1},
		{bucket: fx.since.Add(60 * time.Minute), total: 1, allowCount: 1, denyCount: 0, approveCount: 0},
		{bucket: fx.since.Add(90 * time.Minute), total: 1, allowCount: 0, denyCount: 1, approveCount: 0},
		{bucket: fx.since.Add(2 * time.Hour), total: 1, allowCount: 0, denyCount: 0, approveCount: 1},
		{bucket: fx.since.Add(150 * time.Minute), total: 2, allowCount: 1, denyCount: 1, approveCount: 0},
		{bucket: fx.since.Add(3 * time.Hour), total: 1, allowCount: 0, denyCount: 0, approveCount: 1},
		{bucket: fx.since.Add(210 * time.Minute), total: 1, allowCount: 1, denyCount: 0, approveCount: 0},
	}

	if len(series) != len(expected) {
		t.Fatalf("expected %d half-hour buckets, got %+v", len(expected), series)
	}

	var totalEvents int64
	for i, want := range expected {
		got := series[i]
		bucket, ok := got["bucket"].(time.Time)
		if !ok {
			t.Fatalf("bucket %d was not a time.Time: %#v", i, got["bucket"])
		}
		if !bucket.Equal(want.bucket) {
			t.Fatalf("bucket %d: expected %s, got %s", i, want.bucket, bucket)
		}
		if got["total"] != want.total || got["allow_count"] != want.allowCount || got["deny_count"] != want.denyCount || got["approve_count"] != want.approveCount {
			t.Fatalf("bucket %d: expected totals %+v, got %+v", i, want, got)
		}
		total, ok := got["total"].(int64)
		if !ok {
			t.Fatalf("bucket %d total was not int64: %#v", i, got["total"])
		}
		totalEvents += total
	}

	if totalEvents != 10 {
		t.Fatalf("expected half-hour buckets to preserve 10 total events, got %d", totalEvents)
	}
}

func TestStoreGetDecisionTimeseriesSupportsLargerBucketIntervalsWithoutLosingTotals(t *testing.T) {
	fx := newAnalyticsFixture(t)

	series, err := fx.store.GetDecisionTimeseries(fx.ctx, fx.tenantID, fx.since, 120)
	if err != nil {
		t.Fatalf("GetDecisionTimeseries(120m): %v", err)
	}

	expected := []struct {
		bucket       time.Time
		total        int64
		allowCount   int64
		denyCount    int64
		approveCount int64
	}{
		{bucket: fx.since, total: 5, allowCount: 2, denyCount: 2, approveCount: 1},
		{bucket: fx.since.Add(2 * time.Hour), total: 5, allowCount: 2, denyCount: 1, approveCount: 2},
	}

	if len(series) != len(expected) {
		t.Fatalf("expected %d two-hour buckets, got %+v", len(expected), series)
	}

	var totalEvents int64
	for i, want := range expected {
		got := series[i]
		bucket, ok := got["bucket"].(time.Time)
		if !ok {
			t.Fatalf("bucket %d was not a time.Time: %#v", i, got["bucket"])
		}
		if !bucket.Equal(want.bucket) {
			t.Fatalf("bucket %d: expected %s, got %s", i, want.bucket, bucket)
		}
		if got["total"] != want.total || got["allow_count"] != want.allowCount || got["deny_count"] != want.denyCount || got["approve_count"] != want.approveCount {
			t.Fatalf("bucket %d: expected totals %+v, got %+v", i, want, got)
		}
		total, ok := got["total"].(int64)
		if !ok {
			t.Fatalf("bucket %d total was not int64: %#v", i, got["total"])
		}
		totalEvents += total
	}

	if totalEvents != 10 {
		t.Fatalf("expected two-hour buckets to preserve 10 total events, got %d", totalEvents)
	}
}

func TestStoreGetDecisionTimeseriesBucketMatrixPreservesTotalsAndAlignment(t *testing.T) {
	fx := newAnalyticsFixture(t)

	for _, bucketMinutes := range []int{5, 10, 15, 20, 30, 45, 60, 90, 120, 180} {
		t.Run(fmt.Sprintf("%dm", bucketMinutes), func(t *testing.T) {
			series, err := fx.store.GetDecisionTimeseries(fx.ctx, fx.tenantID, fx.since, bucketMinutes)
			if err != nil {
				t.Fatalf("GetDecisionTimeseries(%dm): %v", bucketMinutes, err)
			}

			var totalEvents int64
			lastBucket := fx.since.Add(-time.Minute)
			for i, got := range series {
				bucket, ok := got["bucket"].(time.Time)
				if !ok {
					t.Fatalf("bucket %d was not a time.Time: %#v", i, got["bucket"])
				}
				if bucket.Before(lastBucket) {
					t.Fatalf("bucket %d regressed in time: last=%s current=%s", i, lastBucket, bucket)
				}
				offset := bucket.Sub(fx.since)
				step := time.Duration(bucketMinutes) * time.Minute
				if offset < 0 || offset%step != 0 {
					t.Fatalf("bucket %d was misaligned for %d-minute interval: since=%s bucket=%s", i, bucketMinutes, fx.since, bucket)
				}
				total, ok := got["total"].(int64)
				if !ok {
					t.Fatalf("bucket %d total was not int64: %#v", i, got["total"])
				}
				totalEvents += total
				lastBucket = bucket
			}

			if totalEvents != 10 {
				t.Fatalf("%d-minute buckets should preserve 10 total events, got %d", bucketMinutes, totalEvents)
			}
		})
	}
}
