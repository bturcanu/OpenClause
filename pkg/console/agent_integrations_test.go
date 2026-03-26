package console

import (
	"context"
	"errors"
	"testing"

	"github.com/bturcanu/OpenClause/internal/testdb"
)

func TestUpsertAndGetAgentIntegration(t *testing.T) {
	harness := testdb.New(t)
	store := NewStore(harness.Pool())

	tenant, err := store.CreateTenant(context.Background(), "Pilot Tenant", nil)
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	agent, err := store.CreateAgent(context.Background(), tenant.ID, "Support Bot")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	first, err := store.UpsertAgentIntegration(context.Background(), AgentIntegrationUpsertInput{
		TenantID:         tenant.ID,
		AgentID:          agent.ID,
		Mode:             "created",
		Runtime:          "python",
		EnvironmentLabel: "dev",
		OwnerName:        "AI Platform",
		Description:      "Pilot integration",
		ApprovalPosture:  "pilot_safe",
		Tools: []AgentIntegrationTool{
			{Tool: "slack", Action: "slack.channel.list"},
		},
	})
	if err != nil {
		t.Fatalf("UpsertAgentIntegration create: %v", err)
	}
	second, err := store.UpsertAgentIntegration(context.Background(), AgentIntegrationUpsertInput{
		TenantID:         tenant.ID,
		AgentID:          agent.ID,
		Mode:             "regenerated",
		Runtime:          "typescript",
		EnvironmentLabel: "prod",
		OwnerName:        "Runtime Team",
		ApprovalPosture:  "tenant_default",
		Tools: []AgentIntegrationTool{
			{Tool: "postgres", Action: "query.readonly"},
		},
	})
	if err != nil {
		t.Fatalf("UpsertAgentIntegration update: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected upsert to preserve integration id, got first=%s second=%s", first.ID, second.ID)
	}

	got, err := store.GetAgentIntegration(context.Background(), tenant.ID, agent.ID)
	if err != nil {
		t.Fatalf("GetAgentIntegration: %v", err)
	}
	if got.Runtime != "typescript" || got.EnvironmentLabel != "prod" || got.OwnerName != "Runtime Team" || got.ApprovalPosture != "tenant_default" {
		t.Fatalf("unexpected integration payload: %+v", got)
	}
	if len(got.Tools) != 1 || got.Tools[0].Tool != "postgres" || got.Tools[0].Action != "query.readonly" {
		t.Fatalf("unexpected integration tools: %+v", got.Tools)
	}

	revisions, err := store.ListAgentIntegrationRevisions(context.Background(), tenant.ID, agent.ID, 10)
	if err != nil {
		t.Fatalf("ListAgentIntegrationRevisions: %v", err)
	}
	if len(revisions) != 2 {
		t.Fatalf("expected 2 revisions, got %+v", revisions)
	}
	if revisions[0].Mode != "regenerated" || revisions[1].Mode != "created" {
		t.Fatalf("expected revision modes to be preserved newest-first, got %+v", revisions)
	}
	if revisions[0].IntegrationID != got.ID || revisions[1].IntegrationID != got.ID {
		t.Fatalf("expected revisions to point at current integration id %s, got %+v", got.ID, revisions)
	}
}

func TestGetAgentIntegrationReturnsNotFound(t *testing.T) {
	harness := testdb.New(t)
	store := NewStore(harness.Pool())

	_, err := store.GetAgentIntegration(context.Background(), "tenant-missing", "agent-missing")
	if !errors.Is(err, ErrAgentIntegrationNotFound) {
		t.Fatalf("expected ErrAgentIntegrationNotFound, got %v", err)
	}
}

func TestGetAgentByTenantIDReturnsExactAgent(t *testing.T) {
	harness := testdb.New(t)
	store := NewStore(harness.Pool())

	tenant, err := store.CreateTenant(context.Background(), "Lookup Tenant", nil)
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	for i := 0; i < 205; i++ {
		name := "Agent " + string(rune('A'+(i%26)))
		if _, err := store.CreateAgent(context.Background(), tenant.ID, name); err != nil {
			t.Fatalf("CreateAgent %d: %v", i, err)
		}
	}
	target, err := store.CreateAgent(context.Background(), tenant.ID, "Target Agent")
	if err != nil {
		t.Fatalf("CreateAgent target: %v", err)
	}

	got, err := store.GetAgentByTenantID(context.Background(), tenant.ID, target.ID)
	if err != nil {
		t.Fatalf("GetAgentByTenantID: %v", err)
	}
	if got.ID != target.ID || got.Name != "Target Agent" || got.TenantID != tenant.ID {
		t.Fatalf("unexpected lookup result: %+v", got)
	}
}
