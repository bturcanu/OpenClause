package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bturcanu/OpenClause/pkg/console"
)

func TestHandleUpdateAgentStatusPersistsAndScopesByTenant(t *testing.T) {
	fx := newDBAPIFixture(t)
	tenant := mustCreateTenantDB(t, fx.store, "Agents Handler Tenant")
	otherTenant := mustCreateTenantDB(t, fx.store, "Other Handler Tenant")
	agent, err := fx.store.CreateAgent(context.Background(), tenant.ID, "handler-agent")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	req := withRouteParams(
		withClaims(
			httptest.NewRequest(http.MethodPost, "/admin/tenants/"+tenant.ID+"/agents/"+agent.ID+"/status", bytes.NewBufferString(`{"status":"disabled"}`)),
			&console.JWTClaims{Tenant: tenant.ID, Roles: []string{"tenant_admin"}},
		),
		map[string]string{"tenant_id": tenant.ID, "agent_id": agent.ID},
	)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	fx.api.handleUpdateAgentStatus(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	agents, err := fx.store.ListAgents(context.Background(), tenant.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) != 1 || agents[0].Status != "disabled" {
		t.Fatalf("expected handler to disable agent, got %+v", agents)
	}

	listActiveOnlyReq := withRouteParams(
		httptest.NewRequest(http.MethodGet, "/admin/tenants/"+tenant.ID+"/agents?include_disabled=false", nil),
		map[string]string{"tenant_id": tenant.ID},
	)
	listActiveOnlyRR := httptest.NewRecorder()
	fx.api.handleListAgents(listActiveOnlyRR, listActiveOnlyReq)
	if listActiveOnlyRR.Code != http.StatusOK {
		t.Fatalf("expected 200 from filtered list, got %d body=%s", listActiveOnlyRR.Code, listActiveOnlyRR.Body.String())
	}
	var activeOnly []console.Agent
	if err := json.Unmarshal(listActiveOnlyRR.Body.Bytes(), &activeOnly); err != nil {
		t.Fatalf("decode active-only agents: %v", err)
	}
	if len(activeOnly) != 0 {
		t.Fatalf("expected disabled agent to be hidden when include_disabled=false, got %+v", activeOnly)
	}

	listDefaultReq := withRouteParams(
		httptest.NewRequest(http.MethodGet, "/admin/tenants/"+tenant.ID+"/agents", nil),
		map[string]string{"tenant_id": tenant.ID},
	)
	listDefaultRR := httptest.NewRecorder()
	fx.api.handleListAgents(listDefaultRR, listDefaultReq)
	if listDefaultRR.Code != http.StatusOK {
		t.Fatalf("expected 200 from default list, got %d body=%s", listDefaultRR.Code, listDefaultRR.Body.String())
	}
	var allAgents []console.Agent
	if err := json.Unmarshal(listDefaultRR.Body.Bytes(), &allAgents); err != nil {
		t.Fatalf("decode default agents: %v", err)
	}
	if len(allAgents) != 1 || allAgents[0].ID != agent.ID || allAgents[0].Status != "disabled" {
		t.Fatalf("expected default list to include disabled agent with status, got %+v", allAgents)
	}

	wrongTenantReq := withRouteParams(
		withClaims(
			httptest.NewRequest(http.MethodPost, "/admin/tenants/"+otherTenant.ID+"/agents/"+agent.ID+"/status", bytes.NewBufferString(`{"status":"active"}`)),
			&console.JWTClaims{Tenant: otherTenant.ID, Roles: []string{"tenant_admin"}},
		),
		map[string]string{"tenant_id": otherTenant.ID, "agent_id": agent.ID},
	)
	wrongTenantReq.Header.Set("Content-Type", "application/json")
	wrongTenantRR := httptest.NewRecorder()
	fx.api.handleUpdateAgentStatus(wrongTenantRR, wrongTenantReq)
	if wrongTenantRR.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for wrong-tenant status update, got %d body=%s", wrongTenantRR.Code, wrongTenantRR.Body.String())
	}
}

func TestHandleUpdateAgentStatusRejectsInvalidInputs(t *testing.T) {
	fx := newDBAPIFixture(t)
	tenant := mustCreateTenantDB(t, fx.store, "Agents Invalid Tenant")
	agent, err := fx.store.CreateAgent(context.Background(), tenant.ID, "invalid-agent")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	req := withRouteParams(
		httptest.NewRequest(http.MethodPost, "/admin/tenants/"+tenant.ID+"/agents/"+agent.ID+"/status", bytes.NewBufferString(`{"status":"paused"}`)),
		map[string]string{"tenant_id": tenant.ID, "agent_id": agent.ID},
	)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	fx.api.handleUpdateAgentStatus(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid status, got %d body=%s", rr.Code, rr.Body.String())
	}

	badQueryReq := withRouteParams(
		httptest.NewRequest(http.MethodGet, "/admin/tenants/"+tenant.ID+"/agents?include_disabled=maybe", nil),
		map[string]string{"tenant_id": tenant.ID},
	)
	badQueryRR := httptest.NewRecorder()
	fx.api.handleListAgents(badQueryRR, badQueryReq)
	if badQueryRR.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid include_disabled query, got %d body=%s", badQueryRR.Code, badQueryRR.Body.String())
	}
}

func TestHandleListAgentsDefaultIncludesDisabledAndFilterHidesThem(t *testing.T) {
	fx := newDBAPIFixture(t)
	tenant := mustCreateTenantDB(t, fx.store, "Agents Filter Tenant")
	activeAgent, err := fx.store.CreateAgent(context.Background(), tenant.ID, "active-agent")
	if err != nil {
		t.Fatalf("CreateAgent active: %v", err)
	}
	disabledAgent, err := fx.store.CreateAgent(context.Background(), tenant.ID, "disabled-agent")
	if err != nil {
		t.Fatalf("CreateAgent disabled: %v", err)
	}
	if err := fx.store.UpdateAgentStatusForTenant(context.Background(), tenant.ID, disabledAgent.ID, "disabled"); err != nil {
		t.Fatalf("UpdateAgentStatusForTenant: %v", err)
	}

	defaultReq := withRouteParams(
		httptest.NewRequest(http.MethodGet, "/admin/tenants/"+tenant.ID+"/agents", nil),
		map[string]string{"tenant_id": tenant.ID},
	)
	defaultRR := httptest.NewRecorder()
	fx.api.handleListAgents(defaultRR, defaultReq)
	if defaultRR.Code != http.StatusOK {
		t.Fatalf("expected 200 from default list, got %d body=%s", defaultRR.Code, defaultRR.Body.String())
	}
	var defaultAgents []console.Agent
	if err := json.Unmarshal(defaultRR.Body.Bytes(), &defaultAgents); err != nil {
		t.Fatalf("decode default agents: %v", err)
	}
	if len(defaultAgents) != 2 {
		t.Fatalf("expected default list to include active and disabled agents, got %+v", defaultAgents)
	}
	statusByID := map[string]string{}
	for _, agent := range defaultAgents {
		statusByID[agent.ID] = agent.Status
	}
	if statusByID[activeAgent.ID] != "active" || statusByID[disabledAgent.ID] != "disabled" {
		t.Fatalf("unexpected default agent statuses: %+v", statusByID)
	}

	filteredReq := withRouteParams(
		httptest.NewRequest(http.MethodGet, "/admin/tenants/"+tenant.ID+"/agents?include_disabled=false", nil),
		map[string]string{"tenant_id": tenant.ID},
	)
	filteredRR := httptest.NewRecorder()
	fx.api.handleListAgents(filteredRR, filteredReq)
	if filteredRR.Code != http.StatusOK {
		t.Fatalf("expected 200 from filtered list, got %d body=%s", filteredRR.Code, filteredRR.Body.String())
	}
	var filteredAgents []console.Agent
	if err := json.Unmarshal(filteredRR.Body.Bytes(), &filteredAgents); err != nil {
		t.Fatalf("decode filtered agents: %v", err)
	}
	if len(filteredAgents) != 1 || filteredAgents[0].ID != activeAgent.ID || filteredAgents[0].Status != "active" {
		t.Fatalf("expected filtered list to keep only the active agent, got %+v", filteredAgents)
	}
}
