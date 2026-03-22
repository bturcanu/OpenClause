package console

import (
	"errors"
	"testing"
)

func TestStoreAgentStatusToggleAndHideDisabledFilter(t *testing.T) {
	store, ctx := newIntegrationStore(t)
	tenant := mustCreateTenant(t, ctx, store, "Agents Tenant")
	otherTenant := mustCreateTenant(t, ctx, store, "Other Tenant")

	disabledAgent, err := store.CreateAgent(ctx, tenant.ID, "disabled-agent")
	if err != nil {
		t.Fatalf("CreateAgent disabled-agent: %v", err)
	}
	activeAgent, err := store.CreateAgent(ctx, tenant.ID, "active-agent")
	if err != nil {
		t.Fatalf("CreateAgent active-agent: %v", err)
	}
	if _, err := store.CreateAgent(ctx, otherTenant.ID, "other-tenant-agent"); err != nil {
		t.Fatalf("CreateAgent other tenant: %v", err)
	}

	if err := store.UpdateAgentStatusForTenant(ctx, tenant.ID, disabledAgent.ID, "disabled"); err != nil {
		t.Fatalf("UpdateAgentStatusForTenant disable: %v", err)
	}

	allAgents, err := store.ListAgents(ctx, tenant.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListAgents default: %v", err)
	}
	if len(allAgents) != 2 {
		t.Fatalf("expected default list to include disabled agents, got %+v", allAgents)
	}

	var sawDisabled, sawActive bool
	for _, agent := range allAgents {
		switch agent.ID {
		case disabledAgent.ID:
			sawDisabled = true
			if agent.Status != "disabled" {
				t.Fatalf("expected disabled agent status to persist, got %+v", agent)
			}
		case activeAgent.ID:
			sawActive = true
			if agent.Status != "active" {
				t.Fatalf("expected active agent to stay active, got %+v", agent)
			}
		}
	}
	if !sawDisabled || !sawActive {
		t.Fatalf("expected both tenant agents in default list, got %+v", allAgents)
	}

	activeOnly, err := store.ListAgentsFiltered(ctx, tenant.ID, false, 10, 0)
	if err != nil {
		t.Fatalf("ListAgentsFiltered active-only: %v", err)
	}
	if len(activeOnly) != 1 || activeOnly[0].ID != activeAgent.ID || activeOnly[0].Status != "active" {
		t.Fatalf("expected only active agent when disabled are hidden, got %+v", activeOnly)
	}

	if err := store.UpdateAgentStatusForTenant(ctx, otherTenant.ID, disabledAgent.ID, "active"); !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("expected wrong-tenant update to return ErrAgentNotFound, got %v", err)
	}
}

func TestStoreUpdateAgentStatusLegacyMethodStillUpdatesByID(t *testing.T) {
	store, ctx := newIntegrationStore(t)
	tenant := mustCreateTenant(t, ctx, store, "Legacy Agent Status Tenant")
	agent, err := store.CreateAgent(ctx, tenant.ID, "legacy-agent")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	if err := store.UpdateAgentStatus(ctx, agent.ID, "disabled"); err != nil {
		t.Fatalf("UpdateAgentStatus legacy path: %v", err)
	}

	agents, err := store.ListAgents(ctx, tenant.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) != 1 || agents[0].Status != "disabled" {
		t.Fatalf("expected legacy updater to keep working, got %+v", agents)
	}
}
