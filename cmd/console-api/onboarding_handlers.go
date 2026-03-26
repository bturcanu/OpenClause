package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bturcanu/OpenClause/pkg/connectors"
	"github.com/bturcanu/OpenClause/pkg/console"
	"github.com/bturcanu/OpenClause/pkg/onboarding"
)

type onboardingIntegrationInput = onboarding.IntegrationInput
type onboardingBundleResponse = onboarding.BundleResponse
type onboardingPreviewInput = onboarding.IntegrationInput
type onboardingRegenerateInput = onboarding.RegenerateInput

func (api *ConsoleAPI) handleCreateOnboardingIntegration(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := requireOnboardingMutationClaims(claims); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in onboardingIntegrationInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	runtime := onboarding.Runtime(strings.TrimSpace(in.Runtime))
	if !isSupportedOnboardingRuntime(runtime) {
		writeError(w, http.StatusBadRequest, "runtime must be one of: python, typescript, langchain, openai_local")
		return
	}

	agentName, err := normalizeRequiredName(in.AgentName, "agent_name")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if len(in.Tools) == 0 {
		writeError(w, http.StatusBadRequest, "at least one tool selection is required")
		return
	}

	requestedTenantID := strings.TrimSpace(in.TenantID)
	requestedNewTenantName := strings.TrimSpace(in.NewTenantName)
	var existingTenant *console.Tenant
	if requestedNewTenantName != "" {
		if requestedTenantID != "" {
			writeError(w, http.StatusBadRequest, "provide tenant_id or new_tenant_name, not both")
			return
		}
		if !hasRole(claims, "platform_admin") {
			writeError(w, http.StatusForbidden, "insufficient permissions to create a tenant")
			return
		}
		requestedNewTenantName, err = normalizeRequiredName(requestedNewTenantName, "new_tenant_name")
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	} else {
		existingTenant, _, err = api.resolveOnboardingTenant(r.Context(), claims, requestedTenantID, "")
		if err != nil {
			status := http.StatusBadRequest
			switch {
			case strings.Contains(err.Error(), "insufficient permissions"):
				status = http.StatusForbidden
			case strings.Contains(err.Error(), "tenant not found"):
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}
	}

	catalog, err := api.fetchConnectorCatalog(r.Context())
	if err != nil {
		api.log.Error("load connector catalog for onboarding failed", "error", err)
		writeError(w, http.StatusBadGateway, "failed to load connector registry")
		return
	}
	if err := validateOnboardingTools(catalog, in.Tools); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tools := normalizeOnboardingTools(catalog, in.Tools)
	baseURL, err := api.requireOnboardingBundleBaseURL()
	if err != nil {
		api.log.Error("onboarding bundle base URL unavailable for create", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	posture := strings.TrimSpace(in.ApprovalPosture)

	keyName := strings.TrimSpace(agentName + " onboarding key")
	var existingPolicy *console.TenantPolicyConfig
	if posture != "" && posture != "tenant_default" && existingTenant != nil {
		cfg, found, err := api.store.GetTenantPolicyConfig(r.Context(), existingTenant.ID)
		if err != nil {
			api.log.Error("load onboarding starter policy failed", "error", err, "tenant_id", existingTenant.ID)
			writeError(w, http.StatusInternalServerError, "failed to apply onboarding starter policy")
			return
		}
		if found {
			existingPolicy = cfg
		}
	}
	policyConfig, _ := buildOnboardingStarterPolicy(existingPolicy, tools, posture)
	createState, err := api.store.CreateOnboardingState(r.Context(), console.OnboardingCreateStateInput{
		ExistingTenantID: requestedTenantID,
		NewTenantName:    requestedNewTenantName,
		AgentName:        agentName,
		APIKeyName:       keyName,
		PolicyConfig:     policyConfig,
		Integration: console.AgentIntegrationUpsertInput{
			Mode:             "created",
			Runtime:          string(runtime),
			EnvironmentLabel: strings.TrimSpace(in.EnvironmentLabel),
			OwnerName:        strings.TrimSpace(in.OwnerName),
			Description:      strings.TrimSpace(in.Description),
			ApprovalPosture:  posture,
			Tools:            toConsoleIntegrationTools(tools),
		},
	})
	if err != nil {
		api.log.Error("create onboarding state failed", "error", err, "tenant_id", requestedTenantID, "new_tenant_name", requestedNewTenantName, "agent_name", agentName)
		writeError(w, http.StatusInternalServerError, "failed to create onboarding integration")
		return
	}
	tenant := createState.Tenant
	createdTenant := createState.CreatedTenant
	agent := createState.Agent
	keyResult := createState.APIKey
	integration := createState.Integration

	bundle, err := onboarding.BuildBundle(onboarding.BundleRequest{
		BaseURL:          baseURL,
		TenantID:         tenant.ID,
		TenantName:       tenant.Name,
		AgentID:          agent.ID,
		AgentName:        agent.Name,
		APIKey:           keyResult.RawKey,
		APIKeyMode:       onboarding.APIKeyModeRawProvided,
		APIKeyPrefix:     keyResult.APIKey.KeyPrefix,
		Runtime:          runtime,
		ApprovalPosture:  posture,
		EnvironmentLabel: strings.TrimSpace(in.EnvironmentLabel),
		OwnerName:        strings.TrimSpace(in.OwnerName),
		Description:      strings.TrimSpace(in.Description),
		Tools:            tools,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to build onboarding bundle: %v", err))
		return
	}

	var resp onboardingBundleResponse
	resp.Mode = "created"
	resp.Tenant.ID = tenant.ID
	resp.Tenant.Name = tenant.Name
	resp.Tenant.Created = createdTenant
	resp.Agent.ID = agent.ID
	resp.Agent.Name = agent.Name
	resp.Agent.Status = agent.Status
	resp.Agent.CreatedAt = agent.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
	resp.Agent.Preview = false
	resp.Integration = toBundleIntegration(integration)
	resp.APIKey = &onboarding.BundleAPIKey{}
	resp.APIKey.ID = keyResult.APIKey.ID
	resp.APIKey.Name = keyResult.APIKey.Name
	resp.APIKey.KeyPrefix = keyResult.APIKey.KeyPrefix
	resp.APIKey.RawKey = keyResult.RawKey
	if keyResult.APIKey.ExpiresAt != nil {
		expires := keyResult.APIKey.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")
		resp.APIKey.ExpiresAt = &expires
	}
	resp.Bundle = bundle

	writeJSON(w, http.StatusCreated, resp)
}

func (api *ConsoleAPI) handlePreviewOnboardingBundle(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in onboardingPreviewInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	runtime := onboarding.Runtime(strings.TrimSpace(in.Runtime))
	if !isSupportedOnboardingRuntime(runtime) {
		writeError(w, http.StatusBadRequest, "runtime must be one of: python, typescript, langchain, openai_local")
		return
	}

	agentName, err := normalizeRequiredName(in.AgentName, "agent_name")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(in.NewTenantName) != "" {
		writeError(w, http.StatusBadRequest, "preview requires tenant_id; new_tenant_name is not supported")
		return
	}
	if len(in.Tools) == 0 {
		writeError(w, http.StatusBadRequest, "at least one tool selection is required")
		return
	}

	tenant, _, err := api.resolveOnboardingTenant(r.Context(), claims, strings.TrimSpace(in.TenantID), "")
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case strings.Contains(err.Error(), "insufficient permissions"):
			status = http.StatusForbidden
		case strings.Contains(err.Error(), "tenant not found"):
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}

	catalog, err := api.fetchConnectorCatalog(r.Context())
	if err != nil {
		api.log.Error("load connector catalog for onboarding preview failed", "error", err)
		writeError(w, http.StatusBadGateway, "failed to load connector registry")
		return
	}
	if err := validateOnboardingTools(catalog, in.Tools); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tools := normalizeOnboardingTools(catalog, in.Tools)
	baseURL, err := api.requireOnboardingBundleBaseURL()
	if err != nil {
		api.log.Error("onboarding bundle base URL unavailable for preview", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	bundle, err := onboarding.BuildBundle(onboarding.BundleRequest{
		BaseURL:          baseURL,
		TenantID:         tenant.ID,
		TenantName:       tenant.Name,
		AgentID:          onboarding.PreviewAgentID(agentName),
		AgentName:        agentName,
		APIKey:           "${OPENCLAUSE_API_KEY:-generated-on-create}",
		APIKeyMode:       onboarding.APIKeyModePreview,
		Runtime:          runtime,
		ApprovalPosture:  strings.TrimSpace(in.ApprovalPosture),
		EnvironmentLabel: strings.TrimSpace(in.EnvironmentLabel),
		OwnerName:        strings.TrimSpace(in.OwnerName),
		Description:      strings.TrimSpace(in.Description),
		Tools:            tools,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to build onboarding bundle: %v", err))
		return
	}

	var resp onboardingBundleResponse
	resp.Mode = "preview"
	resp.Tenant.ID = tenant.ID
	resp.Tenant.Name = tenant.Name
	resp.Tenant.Created = false
	resp.Agent.ID = onboarding.PreviewAgentID(agentName)
	resp.Agent.Name = agentName
	resp.Agent.Status = "preview"
	resp.Agent.CreatedAt = time.Now().UTC().Format("2006-01-02T15:04:05Z07:00")
	resp.Agent.Preview = true
	resp.Bundle = bundle

	writeJSON(w, http.StatusOK, resp)
}

func (api *ConsoleAPI) handleRegenerateOnboardingBundle(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := requireOnboardingMutationClaims(claims); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in onboardingRegenerateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	resp, status, err := api.buildRegeneratedBundleResponse(r.Context(), claims, in, false)
	if err != nil {
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (api *ConsoleAPI) handleRegenerateOnboardingBundleDefaults(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := requireOnboardingMutationClaims(claims); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in onboardingRegenerateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	resp, status, err := api.buildRegeneratedBundleResponse(r.Context(), claims, in, true)
	if err != nil {
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (api *ConsoleAPI) handleArchiveOnboardingBundle(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in onboarding.BundleResponse
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if in.Bundle == nil {
		writeError(w, http.StatusBadRequest, "bundle required")
		return
	}

	archive, err := onboarding.ArchiveBundle(in.Bundle)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	filename := onboarding.BundleArchiveName(&in)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(archive)
}

func (api *ConsoleAPI) handleGetAgentIntegration(w http.ResponseWriter, r *http.Request) {
	tenantID := strings.TrimSpace(r.PathValue("tenant_id"))
	agentID := strings.TrimSpace(r.PathValue("agent_id"))
	if tenantID == "" || agentID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id and agent_id are required")
		return
	}

	agent, err := api.resolveOnboardingAgent(r.Context(), tenantID, agentID)
	if err != nil {
		if strings.Contains(err.Error(), "agent not found") {
			writeError(w, http.StatusNotFound, "agent not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	integration, err := api.store.GetAgentIntegration(r.Context(), tenantID, agent.ID)
	if err != nil {
		if errors.Is(err, console.ErrAgentIntegrationNotFound) {
			writeError(w, http.StatusNotFound, "integration not found")
			return
		}
		api.log.Error("load agent integration failed", "error", err, "tenant_id", tenantID, "agent_id", agentID)
		writeError(w, http.StatusInternalServerError, "failed to load integration")
		return
	}
	writeJSON(w, http.StatusOK, toBundleIntegration(integration))
}

func (api *ConsoleAPI) handleListAgentIntegrationRevisions(w http.ResponseWriter, r *http.Request) {
	tenantID := strings.TrimSpace(r.PathValue("tenant_id"))
	agentID := strings.TrimSpace(r.PathValue("agent_id"))
	if tenantID == "" || agentID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id and agent_id are required")
		return
	}

	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		} else if parsed < 100 {
			limit = parsed
		} else {
			limit = 100
		}
	}

	agent, err := api.resolveOnboardingAgent(r.Context(), tenantID, agentID)
	if err != nil {
		if strings.Contains(err.Error(), "agent not found") {
			writeError(w, http.StatusNotFound, "agent not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := api.store.GetAgentIntegration(r.Context(), tenantID, agent.ID); err != nil {
		if errors.Is(err, console.ErrAgentIntegrationNotFound) {
			writeError(w, http.StatusNotFound, "integration not found")
			return
		}
		api.log.Error("load agent integration revisions failed", "error", err, "tenant_id", tenantID, "agent_id", agentID)
		writeError(w, http.StatusInternalServerError, "failed to load integration revisions")
		return
	}

	revisions, err := api.store.ListAgentIntegrationRevisions(r.Context(), tenantID, agentID, limit)
	if err != nil {
		api.log.Error("list agent integration revisions failed", "error", err, "tenant_id", tenantID, "agent_id", agentID)
		writeError(w, http.StatusInternalServerError, "failed to load integration revisions")
		return
	}

	resp := make([]onboarding.BundleIntegrationRevision, 0, len(revisions))
	for _, item := range revisions {
		resp = append(resp, toBundleIntegrationRevision(&item))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tenant_id": tenantID,
		"agent_id":  agentID,
		"revisions": resp,
		"total":     len(resp),
		"limit":     limit,
	})
}

func (api *ConsoleAPI) handleGetAgentIntegrationBundle(w http.ResponseWriter, r *http.Request) {
	claims := claimsFromCtx(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	tenantID := strings.TrimSpace(r.PathValue("tenant_id"))
	agentID := strings.TrimSpace(r.PathValue("agent_id"))
	if tenantID == "" || agentID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id and agent_id are required")
		return
	}

	useDefaults := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("defaults")), "true")
	resp, status, err := api.buildSavedIntegrationBundleResponse(r.Context(), claims, tenantID, agentID, useDefaults)
	if err != nil {
		writeError(w, status, err.Error())
		return
	}
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("archive")), "true") {
		archive, err := onboarding.ArchiveBundle(resp.Bundle)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		filename := onboarding.BundleArchiveName(resp)
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(archive)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func requireOnboardingMutationClaims(claims *console.JWTClaims) error {
	if hasRole(claims, "platform_admin") || hasRole(claims, "tenant_admin") {
		return nil
	}
	return fmt.Errorf("insufficient permissions")
}

func (api *ConsoleAPI) resolveOnboardingTenant(ctx context.Context, claims *console.JWTClaims, tenantID, newTenantName string) (*console.Tenant, bool, error) {
	scope := tenantScope(claims)
	if scope == tenantDenySentinel {
		return nil, false, fmt.Errorf("insufficient permissions")
	}

	if tenantID != "" && newTenantName != "" {
		return nil, false, fmt.Errorf("provide tenant_id or new_tenant_name, not both")
	}

	if newTenantName != "" {
		if !hasRole(claims, "platform_admin") {
			return nil, false, fmt.Errorf("insufficient permissions to create a tenant")
		}
		name, err := normalizeRequiredName(newTenantName, "new_tenant_name")
		if err != nil {
			return nil, false, err
		}
		tenant, err := api.store.CreateTenant(ctx, name, nil)
		if err != nil {
			return nil, false, fmt.Errorf("failed to create tenant")
		}
		return tenant, true, nil
	}

	if tenantID == "" {
		tenantID = scope
	}
	if tenantID == "" {
		return nil, false, fmt.Errorf("tenant_id or new_tenant_name required")
	}
	if scope != "" && scope != tenantID {
		return nil, false, fmt.Errorf("insufficient permissions for this tenant")
	}

	tenant, err := api.store.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to load tenant")
	}
	if tenant == nil {
		return nil, false, fmt.Errorf("tenant not found")
	}
	return tenant, false, nil
}

func validateOnboardingTools(catalog []connectors.ConnectorInfo, tools []onboarding.SelectedTool) error {
	for _, tool := range tools {
		toolName := strings.TrimSpace(tool.Tool)
		action := strings.TrimSpace(tool.Action)
		if toolName == "" || action == "" {
			return fmt.Errorf("tool selections require both tool and action")
		}

		// Validate using the same canonicalization logic used during bundle generation.
		// This ensures inputs like "slack.channel.list" (prefixed) and case variations
		// are accepted if they can be normalized to a real connector action.
		var entryActions []string
		foundTool := false
		for _, entry := range catalog {
			if strings.EqualFold(strings.TrimSpace(entry.Name), toolName) {
				entryActions = entry.Actions
				foundTool = true
				break
			}
		}
		if !foundTool {
			return fmt.Errorf("unknown tool %q", toolName)
		}

		canonical := canonicalCatalogAction(toolName, action, entryActions)
		if canonical == "" {
			return fmt.Errorf("unknown action %q for tool %q", action, toolName)
		}

		actionOK := false
		for _, candidate := range entryActions {
			if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(canonical)) {
				actionOK = true
				break
			}
		}
		if !actionOK {
			return fmt.Errorf("unknown action %q for tool %q", action, toolName)
		}
	}

	return nil
}

func (api *ConsoleAPI) resolveOnboardingAgent(ctx context.Context, tenantID, agentID string) (*console.Agent, error) {
	if strings.TrimSpace(agentID) == "" {
		return nil, fmt.Errorf("agent_id required")
	}
	agent, err := api.store.GetAgentByTenantID(ctx, tenantID, agentID)
	if err != nil {
		if errors.Is(err, console.ErrAgentNotFound) {
			return nil, fmt.Errorf("agent not found")
		}
		return nil, fmt.Errorf("failed to load tenant agents")
	}
	return agent, nil
}

func (api *ConsoleAPI) resolveOnboardingAPIKeyReference(ctx context.Context, tenantID string) (*console.APIKey, error) {
	keys, err := api.store.ListAPIKeys(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	for _, key := range keys {
		if key.Status == "active" && key.IsPrimary {
			copy := key
			return &copy, nil
		}
	}
	for _, key := range keys {
		if key.Status == "active" {
			copy := key
			return &copy, nil
		}
	}
	return nil, nil
}

func (api *ConsoleAPI) buildRegeneratedBundleResponse(ctx context.Context, claims *console.JWTClaims, in onboardingRegenerateInput, useDefaults bool) (*onboardingBundleResponse, int, error) {
	tenant, _, err := api.resolveOnboardingTenant(ctx, claims, strings.TrimSpace(in.TenantID), "")
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case strings.Contains(err.Error(), "insufficient permissions"):
			status = http.StatusForbidden
		case strings.Contains(err.Error(), "tenant not found"):
			status = http.StatusNotFound
		}
		return nil, status, err
	}

	agent, err := api.resolveOnboardingAgent(ctx, tenant.ID, strings.TrimSpace(in.AgentID))
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "agent not found") {
			status = http.StatusNotFound
		}
		return nil, status, err
	}

	catalog, err := api.fetchConnectorCatalog(ctx)
	if err != nil {
		api.log.Error("load connector catalog for onboarding regenerate failed", "error", err)
		return nil, http.StatusBadGateway, fmt.Errorf("failed to load connector registry")
	}

	runtime := onboarding.Runtime(strings.TrimSpace(in.Runtime))
	tools := in.Tools
	posture := strings.TrimSpace(in.ApprovalPosture)
	appliedDefaults := []onboarding.BundleDefault{}
	persistedIntegration, err := api.store.GetAgentIntegration(ctx, tenant.ID, agent.ID)
	if err != nil && !errors.Is(err, console.ErrAgentIntegrationNotFound) {
		api.log.Error("load onboarding integration for regenerate failed", "error", err, "tenant_id", tenant.ID, "agent_id", agent.ID)
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to load onboarding integration")
	}

	if useDefaults {
		runtime, posture, tools, appliedDefaults, err = defaultRegenerateConfig(catalog, persistedIntegration)
		if err != nil {
			return nil, http.StatusConflict, err
		}
	} else {
		if !isSupportedOnboardingRuntime(runtime) {
			return nil, http.StatusBadRequest, fmt.Errorf("runtime must be one of: python, typescript, langchain, openai_local")
		}
		if len(tools) == 0 {
			return nil, http.StatusBadRequest, fmt.Errorf("at least one tool selection is required")
		}
	}

	if err := validateOnboardingTools(catalog, tools); err != nil {
		return nil, http.StatusBadRequest, err
	}
	tools = normalizeOnboardingTools(catalog, tools)
	baseURL, err := api.requireOnboardingBundleBaseURL()
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}

	environmentLabel := firstNonEmpty(strings.TrimSpace(in.EnvironmentLabel), integrationValue(persistedIntegration, func(integration *console.AgentIntegration) string { return integration.EnvironmentLabel }))
	ownerName := firstNonEmpty(strings.TrimSpace(in.OwnerName), integrationValue(persistedIntegration, func(integration *console.AgentIntegration) string { return integration.OwnerName }))
	description := firstNonEmpty(strings.TrimSpace(in.Description), integrationValue(persistedIntegration, func(integration *console.AgentIntegration) string { return integration.Description }))

	integration, err := api.store.PersistOnboardingIntegration(ctx, tenant.ID, agent.ID, console.AgentIntegrationUpsertInput{
		Mode:             map[bool]string{true: "regenerated_defaults", false: "regenerated"}[useDefaults],
		Runtime:          string(runtime),
		EnvironmentLabel: environmentLabel,
		OwnerName:        ownerName,
		Description:      description,
		ApprovalPosture:  posture,
		Tools:            toConsoleIntegrationTools(tools),
	})
	if err != nil {
		api.log.Error("persist onboarding integration for regenerate failed", "error", err, "tenant_id", tenant.ID, "agent_id", agent.ID)
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to persist onboarding integration")
	}

	keyRef, err := api.resolveOnboardingAPIKeyReference(ctx, tenant.ID)
	if err != nil {
		api.log.Error("load api key reference for onboarding regenerate failed", "error", err, "tenant_id", tenant.ID)
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to load api key reference")
	}

	bundle, err := onboarding.BuildBundle(onboarding.BundleRequest{
		BaseURL:          baseURL,
		TenantID:         tenant.ID,
		TenantName:       tenant.Name,
		AgentID:          agent.ID,
		AgentName:        agent.Name,
		APIKey:           "${OPENCLAUSE_API_KEY:-reuse-existing-key}",
		APIKeyMode:       onboarding.APIKeyModeExistingKeyRef,
		APIKeyPrefix:     apiKeyPrefixOrEmpty(keyRef),
		Runtime:          runtime,
		ApprovalPosture:  posture,
		EnvironmentLabel: environmentLabel,
		OwnerName:        ownerName,
		Description:      description,
		Tools:            tools,
	})
	if err != nil {
		return nil, http.StatusBadRequest, fmt.Errorf("failed to build onboarding bundle: %v", err)
	}
	if len(appliedDefaults) > 0 {
		bundle.AppliedDefaults = appliedDefaults
	}
	if keyRef == nil {
		bundle.Notes = append(bundle.Notes, "No active API key was found for this tenant. Create or rotate an API key from the tenant API Keys tab before running the smoke test.")
	}

	var resp onboardingBundleResponse
	if useDefaults {
		resp.Mode = "regenerated_defaults"
	} else {
		resp.Mode = "regenerated"
	}
	resp.Tenant.ID = tenant.ID
	resp.Tenant.Name = tenant.Name
	resp.Tenant.Created = false
	resp.Agent.ID = agent.ID
	resp.Agent.Name = agent.Name
	resp.Agent.Status = agent.Status
	resp.Agent.CreatedAt = agent.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
	resp.Agent.Preview = false
	resp.Integration = toBundleIntegration(integration)
	if keyRef != nil {
		resp.APIKey = &onboarding.BundleAPIKey{ID: keyRef.ID, Name: keyRef.Name, KeyPrefix: keyRef.KeyPrefix}
		if keyRef.ExpiresAt != nil {
			expires := keyRef.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")
			resp.APIKey.ExpiresAt = &expires
		}
	}
	resp.Bundle = bundle
	return &resp, http.StatusOK, nil
}

func (api *ConsoleAPI) buildSavedIntegrationBundleResponse(ctx context.Context, claims *console.JWTClaims, tenantID, agentID string, useDefaults bool) (*onboardingBundleResponse, int, error) {
	tenant, _, err := api.resolveOnboardingTenant(ctx, claims, tenantID, "")
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case strings.Contains(err.Error(), "insufficient permissions"):
			status = http.StatusForbidden
		case strings.Contains(err.Error(), "tenant not found"):
			status = http.StatusNotFound
		}
		return nil, status, err
	}

	agent, err := api.resolveOnboardingAgent(ctx, tenant.ID, agentID)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "agent not found") {
			status = http.StatusNotFound
		}
		return nil, status, err
	}

	persistedIntegration, err := api.store.GetAgentIntegration(ctx, tenant.ID, agent.ID)
	if err != nil {
		if errors.Is(err, console.ErrAgentIntegrationNotFound) {
			return nil, http.StatusNotFound, fmt.Errorf("integration not found")
		}
		api.log.Error("load onboarding integration for saved bundle failed", "error", err, "tenant_id", tenant.ID, "agent_id", agent.ID)
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to load onboarding integration")
	}

	catalog, err := api.fetchConnectorCatalog(ctx)
	if err != nil {
		api.log.Error("load connector catalog for saved bundle failed", "error", err)
		return nil, http.StatusBadGateway, fmt.Errorf("failed to load connector registry")
	}

	runtime := onboarding.Runtime(strings.TrimSpace(persistedIntegration.Runtime))
	posture := strings.TrimSpace(persistedIntegration.ApprovalPosture)
	tools := toOnboardingSelectedTools(persistedIntegration.Tools)
	appliedDefaults := []onboarding.BundleDefault{}
	if useDefaults {
		runtime, posture, tools, appliedDefaults, err = defaultRegenerateConfig(catalog, persistedIntegration)
		if err != nil {
			return nil, http.StatusConflict, err
		}
	} else {
		if !isSupportedOnboardingRuntime(runtime) {
			return nil, http.StatusConflict, fmt.Errorf("saved integration runtime is no longer supported")
		}
		if len(tools) == 0 {
			return nil, http.StatusConflict, fmt.Errorf("saved integration does not include any governed tools")
		}
	}

	if err := validateOnboardingTools(catalog, tools); err != nil {
		return nil, http.StatusConflict, err
	}
	tools = normalizeOnboardingTools(catalog, tools)
	baseURL, err := api.requireOnboardingBundleBaseURL()
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	keyRef, err := api.resolveOnboardingAPIKeyReference(ctx, tenant.ID)
	if err != nil {
		api.log.Error("load api key reference for saved bundle failed", "error", err, "tenant_id", tenant.ID)
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to load api key reference")
	}

	bundle, err := onboarding.BuildBundle(onboarding.BundleRequest{
		BaseURL:          baseURL,
		TenantID:         tenant.ID,
		TenantName:       tenant.Name,
		AgentID:          agent.ID,
		AgentName:        agent.Name,
		APIKey:           "${OPENCLAUSE_API_KEY:-reuse-existing-key}",
		APIKeyMode:       onboarding.APIKeyModeExistingKeyRef,
		APIKeyPrefix:     apiKeyPrefixOrEmpty(keyRef),
		Runtime:          runtime,
		ApprovalPosture:  posture,
		EnvironmentLabel: persistedIntegration.EnvironmentLabel,
		OwnerName:        persistedIntegration.OwnerName,
		Description:      persistedIntegration.Description,
		Tools:            tools,
	})
	if err != nil {
		return nil, http.StatusBadRequest, fmt.Errorf("failed to build onboarding bundle: %v", err)
	}
	if len(appliedDefaults) > 0 {
		bundle.AppliedDefaults = appliedDefaults
	}
	if keyRef == nil {
		bundle.Notes = append(bundle.Notes, "No active API key was found for this tenant. Create or rotate an API key from the tenant API Keys tab before running the smoke test.")
	}

	resp := &onboarding.BundleResponse{
		Mode: map[bool]string{true: "fetched_defaults", false: "fetched"}[useDefaults],
		Tenant: onboarding.BundleTenant{
			ID:      tenant.ID,
			Name:    tenant.Name,
			Created: false,
		},
		Agent: onboarding.BundleAgent{
			ID:        agent.ID,
			Name:      agent.Name,
			Status:    agent.Status,
			CreatedAt: agent.CreatedAt.Format(time.RFC3339),
			Preview:   false,
		},
		Integration: toBundleIntegration(persistedIntegration),
		Bundle:      bundle,
	}
	if keyRef != nil {
		resp.APIKey = &onboarding.BundleAPIKey{
			ID:        keyRef.ID,
			Name:      keyRef.Name,
			KeyPrefix: keyRef.KeyPrefix,
		}
		if keyRef.ExpiresAt != nil {
			expires := keyRef.ExpiresAt.Format(time.RFC3339)
			resp.APIKey.ExpiresAt = &expires
		}
	}
	return resp, http.StatusOK, nil
}

func defaultRegenerateConfig(catalog []connectors.ConnectorInfo, integration *console.AgentIntegration) (onboarding.Runtime, string, []onboarding.SelectedTool, []onboarding.BundleDefault, error) {
	runtime := onboarding.RuntimePython
	posture := "pilot_safe"
	curated := []onboarding.SelectedTool{
		{Tool: "slack", Action: "slack.channel.list"},
		{Tool: "slack", Action: "slack.msg.post"},
		{Tool: "jira", Action: "jira.issue.list"},
		{Tool: "jira", Action: "jira.issue.create"},
		{Tool: "postgres", Action: "query.readonly"},
		{Tool: "github", Action: "issue.create"},
		{Tool: "email", Action: "send"},
		{Tool: "webhook", Action: "post"},
	}

	allowed := make(map[string]map[string]struct{}, len(catalog))
	for _, entry := range catalog {
		actionSet := make(map[string]struct{}, len(entry.Actions))
		for _, action := range entry.Actions {
			actionSet[strings.TrimSpace(action)] = struct{}{}
		}
		allowed[strings.TrimSpace(entry.Name)] = actionSet
	}

	defaults := []onboarding.BundleDefault{}
	selected := make([]onboarding.SelectedTool, 0, 2)
	if integration != nil {
		if persistedRuntime := onboarding.Runtime(strings.TrimSpace(integration.Runtime)); isSupportedOnboardingRuntime(persistedRuntime) {
			runtime = persistedRuntime
			defaults = append(defaults, onboarding.BundleDefault{
				Field:  "runtime",
				Value:  string(runtime),
				Reason: "Persisted integration record for this agent",
			})
		}
		if persistedPosture := strings.TrimSpace(integration.ApprovalPosture); persistedPosture != "" {
			posture = persistedPosture
			defaults = append(defaults, onboarding.BundleDefault{
				Field:  "approval_posture",
				Value:  posture,
				Reason: "Persisted integration record for this agent",
			})
		}
		for _, tool := range integration.Tools {
			actions, ok := allowed[strings.TrimSpace(tool.Tool)]
			if !ok {
				continue
			}
			action := strings.TrimSpace(tool.Action)
			if _, ok := actions[action]; !ok {
				continue
			}
			selected = append(selected, onboarding.SelectedTool{
				Tool:   strings.TrimSpace(tool.Tool),
				Action: action,
			})
		}
		if len(selected) > 0 {
			for _, tool := range selected {
				defaults = append(defaults, onboarding.BundleDefault{
					Field:  "tool",
					Value:  formatOnboardingToolSelection(tool),
					Reason: "Persisted integration record for this agent",
				})
			}
			return runtime, posture, selected, defaults, nil
		}
	}

	selected = make([]onboarding.SelectedTool, 0, 2)
	for _, candidate := range curated {
		actions, ok := allowed[candidate.Tool]
		if !ok {
			continue
		}
		if _, ok := actions[candidate.Action]; ok {
			selected = append(selected, candidate)
		}
		if len(selected) == 2 {
			break
		}
	}
	if len(selected) == 0 {
		return "", "", nil, nil, fmt.Errorf("no curated onboarding defaults are available in the current connector catalog")
	}

	defaults = []onboarding.BundleDefault{
		{Field: "runtime", Value: string(runtime), Reason: "OpenClause v0.5 golden-path default"},
		{Field: "approval_posture", Value: posture, Reason: "Recommended pilot-safe default"},
	}
	for _, tool := range selected {
		defaults = append(defaults, onboarding.BundleDefault{
			Field:  "tool",
			Value:  formatOnboardingToolSelection(tool),
			Reason: "First curated tool available in the connector catalog",
		})
	}
	return runtime, posture, selected, defaults, nil
}

func normalizeOnboardingTools(catalog []connectors.ConnectorInfo, tools []onboarding.SelectedTool) []onboarding.SelectedTool {
	if len(tools) == 0 {
		return nil
	}
	normalized := make([]onboarding.SelectedTool, 0, len(tools))
	for _, tool := range tools {
		entry := onboarding.SelectedTool{
			Tool:   strings.TrimSpace(tool.Tool),
			Action: strings.TrimSpace(tool.Action),
		}
		if entry.Tool == "" || entry.Action == "" {
			continue
		}
		for _, connector := range catalog {
			if !strings.EqualFold(strings.TrimSpace(connector.Name), entry.Tool) {
				continue
			}
			// Canonicalize the tool name to the connector's registered name casing,
			// so generated bundles submit a tool value that the gateway connector
			// registry can match (case-sensitive).
			entry.Tool = strings.TrimSpace(connector.Name)
			entry.Action = canonicalCatalogAction(entry.Tool, entry.Action, connector.Actions)
			break
		}
		normalized = append(normalized, entry)
	}
	return normalized
}

func canonicalCatalogAction(tool, action string, catalogActions []string) string {
	trimmed := strings.TrimSpace(action)
	if trimmed == "" {
		return ""
	}
	toolPrefix := strings.ToLower(strings.TrimSpace(tool)) + "."
	normalized := strings.ToLower(trimmed)
	for _, candidate := range catalogActions {
		candidate = strings.TrimSpace(candidate)
		if strings.EqualFold(candidate, trimmed) {
			return candidate
		}
		if strings.EqualFold(toolPrefix+candidate, normalized) {
			return candidate
		}
	}
	return trimmed
}

func normalizePolicyToolAction(tool onboarding.SelectedTool) string {
	toolName := strings.ToLower(strings.TrimSpace(tool.Tool))
	action := strings.ToLower(strings.TrimSpace(tool.Action))
	if toolName == "" || action == "" {
		return ""
	}
	if strings.HasPrefix(action, toolName+".") {
		return action
	}
	return toolName + "." + action
}

func onboardingActionLooksRead(tool onboarding.SelectedTool) bool {
	action := normalizePolicyToolAction(tool)
	readMarkers := []string{
		".list",
		".get",
		".read",
		".readonly",
		".info",
		".describe",
		".search",
		".query.readonly",
		".repo.readme",
	}
	for _, marker := range readMarkers {
		if strings.Contains(action, marker) {
			return true
		}
	}
	return false
}

func mergePolicyAction(actions []string, action string) []string {
	action = strings.TrimSpace(action)
	if action == "" {
		return actions
	}
	for _, existing := range actions {
		if strings.EqualFold(strings.TrimSpace(existing), action) {
			return actions
		}
	}
	return append(actions, action)
}

func defaultOnboardingStarterPolicy(posture string) console.TenantPolicyConfig {
	cfg := console.TenantPolicyConfig{
		MaxRiskAutoApprove:         4,
		RequireDestructiveApproval: true,
		ReadActions:                []string{},
		WriteActions:               []string{},
		DestructiveActions:         []string{},
	}
	if strings.TrimSpace(posture) == "read_only_first" {
		cfg.MaxRiskAutoApprove = 4
	}
	return cfg
}

func buildOnboardingStarterPolicy(existing *console.TenantPolicyConfig, tools []onboarding.SelectedTool, posture string) (*console.TenantPolicyConfig, bool) {
	posture = strings.TrimSpace(posture)
	if posture == "" || posture == "tenant_default" {
		return nil, false
	}

	var cfg console.TenantPolicyConfig
	if existing != nil {
		cfg = *existing
		cfg.ReadActions = append([]string(nil), existing.ReadActions...)
		cfg.WriteActions = append([]string(nil), existing.WriteActions...)
		cfg.DestructiveActions = append([]string(nil), existing.DestructiveActions...)
	} else {
		cfg = defaultOnboardingStarterPolicy(posture)
	}

	if cfg.MaxRiskAutoApprove <= 0 {
		cfg.MaxRiskAutoApprove = 4
	}
	if posture == "pilot_safe" {
		cfg.RequireDestructiveApproval = true
	}

	for _, tool := range tools {
		canonical := normalizePolicyToolAction(tool)
		if canonical == "" {
			continue
		}
		if onboardingActionLooksRead(tool) {
			cfg.ReadActions = mergePolicyAction(cfg.ReadActions, canonical)
			continue
		}
		switch posture {
		case "read_only_first":
			// Leave write-like actions denied until the operator is ready to add them.
		case "pilot_safe":
			cfg.DestructiveActions = mergePolicyAction(cfg.DestructiveActions, canonical)
		default:
			cfg.WriteActions = mergePolicyAction(cfg.WriteActions, canonical)
		}
	}

	return &cfg, true
}

func formatOnboardingToolSelection(tool onboarding.SelectedTool) string {
	return fmt.Sprintf("%s:%s", strings.TrimSpace(tool.Tool), strings.TrimSpace(tool.Action))
}

func isSupportedOnboardingRuntime(runtime onboarding.Runtime) bool {
	return runtime == onboarding.RuntimePython ||
		runtime == onboarding.RuntimeTypeScript ||
		runtime == onboarding.RuntimeLangChain ||
		runtime == onboarding.RuntimeOpenAILocal
}

func apiKeyPrefixOrEmpty(key *console.APIKey) string {
	if key == nil {
		return ""
	}
	return key.KeyPrefix
}

func integrationValue(integration *console.AgentIntegration, getter func(*console.AgentIntegration) string) string {
	if integration == nil {
		return ""
	}
	return strings.TrimSpace(getter(integration))
}

func toConsoleIntegrationTools(tools []onboarding.SelectedTool) []console.AgentIntegrationTool {
	out := make([]console.AgentIntegrationTool, 0, len(tools))
	for _, tool := range tools {
		out = append(out, console.AgentIntegrationTool{
			Tool:   strings.TrimSpace(tool.Tool),
			Action: strings.TrimSpace(tool.Action),
		})
	}
	return out
}

func toBundleIntegration(integration *console.AgentIntegration) *onboarding.BundleIntegration {
	if integration == nil {
		return nil
	}
	tools := make([]onboarding.SelectedTool, 0, len(integration.Tools))
	for _, tool := range integration.Tools {
		tools = append(tools, onboarding.SelectedTool{
			Tool:   strings.TrimSpace(tool.Tool),
			Action: strings.TrimSpace(tool.Action),
		})
	}
	return &onboarding.BundleIntegration{
		ID:               integration.ID,
		TenantID:         integration.TenantID,
		AgentID:          integration.AgentID,
		Runtime:          integration.Runtime,
		EnvironmentLabel: integration.EnvironmentLabel,
		OwnerName:        integration.OwnerName,
		Description:      integration.Description,
		ApprovalPosture:  integration.ApprovalPosture,
		Tools:            tools,
		CreatedAt:        integration.CreatedAt.Format(time.RFC3339),
		UpdatedAt:        integration.UpdatedAt.Format(time.RFC3339),
	}
}

func toBundleIntegrationRevision(revision *console.AgentIntegrationRevision) onboarding.BundleIntegrationRevision {
	if revision == nil {
		return onboarding.BundleIntegrationRevision{}
	}
	tools := make([]onboarding.SelectedTool, 0, len(revision.Tools))
	for _, tool := range revision.Tools {
		tools = append(tools, onboarding.SelectedTool{
			Tool:   tool.Tool,
			Action: tool.Action,
		})
	}
	return onboarding.BundleIntegrationRevision{
		ID:               revision.ID,
		IntegrationID:    revision.IntegrationID,
		TenantID:         revision.TenantID,
		AgentID:          revision.AgentID,
		Mode:             revision.Mode,
		Runtime:          revision.Runtime,
		EnvironmentLabel: revision.EnvironmentLabel,
		OwnerName:        revision.OwnerName,
		Description:      revision.Description,
		ApprovalPosture:  revision.ApprovalPosture,
		Tools:            tools,
		CreatedAt:        revision.CreatedAt.Format(time.RFC3339),
	}
}

func toOnboardingSelectedTools(tools []console.AgentIntegrationTool) []onboarding.SelectedTool {
	selected := make([]onboarding.SelectedTool, 0, len(tools))
	for _, tool := range tools {
		if strings.TrimSpace(tool.Tool) == "" || strings.TrimSpace(tool.Action) == "" {
			continue
		}
		selected = append(selected, onboarding.SelectedTool{
			Tool:   strings.TrimSpace(tool.Tool),
			Action: strings.TrimSpace(tool.Action),
		})
	}
	return selected
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (api *ConsoleAPI) onboardingBundleBaseURL() string {
	baseURL := strings.TrimRight(strings.TrimSpace(api.publicGatewayURL), "/")
	if baseURL != "" {
		return baseURL
	}
	return strings.TrimRight(strings.TrimSpace(api.gatewayURL), "/")
}

func (api *ConsoleAPI) requireOnboardingBundleBaseURL() (string, error) {
	baseURL := api.onboardingBundleBaseURL()
	if strings.TrimSpace(baseURL) == "" {
		return "", fmt.Errorf("onboarding bundle base URL is not configured")
	}
	return baseURL, nil
}
