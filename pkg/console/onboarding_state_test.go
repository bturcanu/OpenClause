package console

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bturcanu/OpenClause/internal/testdb"
)

func installFailingOnboardingTrigger(t *testing.T, store *Store, table, triggerName string) {
	installFailingOnboardingTriggerForEvent(t, store, table, triggerName, "INSERT")
}

func installFailingOnboardingTriggerForEvent(t *testing.T, store *Store, table, triggerName, event string) {
	t.Helper()
	ctx := context.Background()
	functionName := "fail_" + strings.ReplaceAll(triggerName, "-", "_")
	if _, err := store.Pool().Exec(ctx, `
		CREATE OR REPLACE FUNCTION `+functionName+`()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			RAISE EXCEPTION '`+triggerName+` failure';
		END;
	$$`); err != nil {
		t.Fatalf("create %s function: %v", triggerName, err)
	}
	if _, err := store.Pool().Exec(ctx, `DROP TRIGGER IF EXISTS `+triggerName+` ON `+table); err != nil {
		t.Fatalf("drop %s trigger: %v", triggerName, err)
	}
	if _, err := store.Pool().Exec(ctx, `
		CREATE TRIGGER `+triggerName+`
		BEFORE `+event+` ON `+table+`
		FOR EACH ROW
		EXECUTE FUNCTION `+functionName+`()`,
	); err != nil {
		t.Fatalf("create %s trigger: %v", triggerName, err)
	}
}

func countRowsByValue(t *testing.T, store *Store, table, column, value string) int {
	t.Helper()
	var count int
	if err := store.Pool().QueryRow(context.Background(), `SELECT COUNT(*) FROM `+table+` WHERE `+column+` = $1`, value).Scan(&count); err != nil {
		t.Fatalf("count %s rows: %v", table, err)
	}
	return count
}

func countAllRows(t *testing.T, store *Store, table string) int {
	t.Helper()
	var count int
	if err := store.Pool().QueryRow(context.Background(), `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil {
		t.Fatalf("count %s rows: %v", table, err)
	}
	return count
}

func TestCreateOnboardingStateRollsBackWhenIntegrationPersistenceFails(t *testing.T) {
	harness := testdb.New(t)
	store := NewStore(harness.Pool())
	installFailingOnboardingTrigger(t, store, "agent_integrations", "fail_agent_integration_insert")

	_, err := store.CreateOnboardingState(context.Background(), OnboardingCreateStateInput{
		NewTenantName: "Transactional Tenant",
		AgentName:     "Transactional Bot",
		APIKeyName:    "Transactional key",
		Integration: AgentIntegrationUpsertInput{
			Mode:            "created",
			Runtime:         "python",
			ApprovalPosture: "pilot_safe",
			Tools: []AgentIntegrationTool{
				{Tool: "postgres", Action: "query.readonly"},
			},
		},
	})
	if err == nil {
		t.Fatal("expected create onboarding state to fail")
	}

	if got := countRowsByValue(t, store, "tenants", "name", "Transactional Tenant"); got != 0 {
		t.Fatalf("expected tenant rollback, found %d rows", got)
	}
	if got := countRowsByValue(t, store, "agents", "name", "Transactional Bot"); got != 0 {
		t.Fatalf("expected agent rollback, found %d rows", got)
	}
	if got := countRowsByValue(t, store, "api_keys", "name", "Transactional key"); got != 0 {
		t.Fatalf("expected api key rollback, found %d rows", got)
	}
}

func TestCreateOnboardingStateRollsBackWhenAPIKeyCreationFails(t *testing.T) {
	harness := testdb.New(t)
	store := NewStore(harness.Pool())
	installFailingOnboardingTrigger(t, store, "api_keys", "fail_api_key_insert")

	_, err := store.CreateOnboardingState(context.Background(), OnboardingCreateStateInput{
		NewTenantName: "API Key Failure Tenant",
		AgentName:     "API Key Failure Bot",
		APIKeyName:    "API Key Failure key",
		Integration: AgentIntegrationUpsertInput{
			Mode:            "created",
			Runtime:         "python",
			ApprovalPosture: "pilot_safe",
			Tools: []AgentIntegrationTool{
				{Tool: "postgres", Action: "query.readonly"},
			},
		},
	})
	if err == nil {
		t.Fatal("expected create onboarding state to fail")
	}

	if got := countRowsByValue(t, store, "tenants", "name", "API Key Failure Tenant"); got != 0 {
		t.Fatalf("expected tenant rollback, found %d rows", got)
	}
	if got := countRowsByValue(t, store, "agents", "name", "API Key Failure Bot"); got != 0 {
		t.Fatalf("expected agent rollback, found %d rows", got)
	}
	if got := countAllRows(t, store, "agent_integrations"); got != 0 {
		t.Fatalf("expected no integration rows, found %d", got)
	}
}

func TestCreateOnboardingStateRollsBackWhenExistingTenantPolicyUpdateFails(t *testing.T) {
	harness := testdb.New(t)
	store := NewStore(harness.Pool())

	tenant, err := store.CreateTenant(context.Background(), "Existing Policy Tenant", json.RawMessage(`{"policy_config":{"max_risk_auto_approve":7}}`))
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	originalConfig := string(tenant.Config)
	installFailingOnboardingTriggerForEvent(t, store, "tenants", "fail_tenant_policy_update", "UPDATE")

	_, err = store.CreateOnboardingState(context.Background(), OnboardingCreateStateInput{
		ExistingTenantID: tenant.ID,
		AgentName:        "Policy Update Failure Bot",
		APIKeyName:       "Policy Update Failure key",
		PolicyConfig: &TenantPolicyConfig{
			MaxRiskAutoApprove:         3,
			ReadActions:                []string{"postgres.query.readonly"},
			RequireDestructiveApproval: true,
		},
		Integration: AgentIntegrationUpsertInput{
			Mode:            "created",
			Runtime:         "python",
			ApprovalPosture: "pilot_safe",
			Tools: []AgentIntegrationTool{
				{Tool: "postgres", Action: "query.readonly"},
			},
		},
	})
	if err == nil {
		t.Fatal("expected create onboarding state to fail")
	}

	if got := countRowsByValue(t, store, "agents", "name", "Policy Update Failure Bot"); got != 0 {
		t.Fatalf("expected agent rollback, found %d rows", got)
	}
	if got := countRowsByValue(t, store, "api_keys", "name", "Policy Update Failure key"); got != 0 {
		t.Fatalf("expected api key rollback, found %d rows", got)
	}
	reloadedTenant, err := store.GetTenant(context.Background(), tenant.ID)
	if err != nil {
		t.Fatalf("GetTenant: %v", err)
	}
	if reloadedTenant == nil || string(reloadedTenant.Config) != originalConfig {
		t.Fatalf("expected tenant config rollback, got %+v", reloadedTenant)
	}
}

func TestPersistOnboardingStateRollsBackLabelsWhenRevisionInsertFails(t *testing.T) {
	harness := testdb.New(t)
	store := NewStore(harness.Pool())

	tenant, err := store.CreateTenant(context.Background(), "Rollback Tenant", nil)
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	initialLabels := json.RawMessage(`{"onboarding":{"runtime":"python","tools":[{"tool":"postgres","action":"query.readonly"}]}}`)
	agent, err := store.CreateAgentWithLabels(context.Background(), tenant.ID, "Rollback Bot", initialLabels)
	if err != nil {
		t.Fatalf("CreateAgentWithLabels: %v", err)
	}
	initialIntegration, err := store.UpsertAgentIntegration(context.Background(), AgentIntegrationUpsertInput{
		TenantID:         tenant.ID,
		AgentID:          agent.ID,
		Mode:             "created",
		Runtime:          "python",
		EnvironmentLabel: "dev",
		ApprovalPosture:  "pilot_safe",
		Tools: []AgentIntegrationTool{
			{Tool: "postgres", Action: "query.readonly"},
		},
	})
	if err != nil {
		t.Fatalf("UpsertAgentIntegration: %v", err)
	}

	installFailingOnboardingTrigger(t, store, "agent_integration_revisions", "fail_agent_integration_revision_insert")

	_, err = store.PersistOnboardingIntegration(context.Background(), tenant.ID, agent.ID, AgentIntegrationUpsertInput{
		Mode:             "regenerated",
		Runtime:          "typescript",
		EnvironmentLabel: "prod",
		ApprovalPosture:  "tenant_default",
		Tools: []AgentIntegrationTool{
			{Tool: "jira", Action: "jira.issue.list"},
		},
	})
	if err == nil {
		t.Fatal("expected persist onboarding state to fail")
	}

	reloadedAgent, err := store.GetAgentByTenantID(context.Background(), tenant.ID, agent.ID)
	if err != nil {
		t.Fatalf("GetAgentByTenantID: %v", err)
	}
	if string(reloadedAgent.Labels) != string(initialLabels) {
		t.Fatalf("expected labels to stay unchanged, got %s", string(reloadedAgent.Labels))
	}

	reloadedIntegration, err := store.GetAgentIntegration(context.Background(), tenant.ID, agent.ID)
	if err != nil {
		t.Fatalf("GetAgentIntegration: %v", err)
	}
	if reloadedIntegration.Runtime != initialIntegration.Runtime || reloadedIntegration.EnvironmentLabel != initialIntegration.EnvironmentLabel {
		t.Fatalf("expected integration rollback, got %+v", reloadedIntegration)
	}
	revisions, err := store.ListAgentIntegrationRevisions(context.Background(), tenant.ID, agent.ID, 10)
	if err != nil {
		t.Fatalf("ListAgentIntegrationRevisions: %v", err)
	}
	if len(revisions) != 1 || revisions[0].Mode != "created" {
		t.Fatalf("expected original revision only, got %+v", revisions)
	}
}
