package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bturcanu/OpenClause/pkg/config"
	"github.com/bturcanu/OpenClause/pkg/console"
	"github.com/bturcanu/OpenClause/pkg/policy"
	"github.com/bturcanu/OpenClause/pkg/types"
	"github.com/go-chi/chi/v5"
)

func normalizeActions(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{})
	for _, v := range in {
		v = strings.ToLower(strings.TrimSpace(v))
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func validateTenantPolicyConfig(cfg *console.TenantPolicyConfig) error {
	if cfg.MaxRiskAutoApprove < 0 || cfg.MaxRiskAutoApprove > 10 {
		return fmt.Errorf("max_risk_auto_approve must be between 0 and 10")
	}
	cfg.ReadActions = normalizeActions(cfg.ReadActions)
	cfg.WriteActions = normalizeActions(cfg.WriteActions)
	cfg.DestructiveActions = normalizeActions(cfg.DestructiveActions)
	return nil
}

func (api *ConsoleAPI) handleGetTenantPolicyConfig(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	cfg, found, err := api.store.GetTenantPolicyConfig(r.Context(), tenantID)
	if err != nil {
		api.log.Error("get tenant policy config failed", "error", err, "tenant_id", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to load tenant policy config")
		return
	}
	if !found || cfg == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"max_risk_auto_approve":        7,
			"read_actions":                 []string{},
			"write_actions":                []string{},
			"destructive_actions":          []string{},
			"require_destructive_approval": true,
		})
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (api *ConsoleAPI) handleUpsertTenantPolicyConfig(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in console.TenantPolicyConfig
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := validateTenantPolicyConfig(&in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := api.store.SetTenantPolicyConfig(r.Context(), tenantID, in); err != nil {
		api.log.Error("set tenant policy config failed", "error", err, "tenant_id", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to save tenant policy config")
		return
	}
	writeJSON(w, http.StatusOK, in)
}

func (api *ConsoleAPI) handleListTenantPolicyVersions(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	versions, err := api.store.ListPolicyVersions(r.Context(), tenantID, 100)
	if err != nil {
		api.log.Error("list tenant policy versions failed", "error", err, "tenant_id", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to list tenant policy versions")
		return
	}
	writeJSON(w, http.StatusOK, versions)
}

func (api *ConsoleAPI) handleCreateTenantPolicyVersion(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	claims := claimsFromCtx(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in struct {
		Version    string                      `json:"version"`
		Notes      string                      `json:"notes"`
		PolicyData *console.TenantPolicyConfig `json:"policy_data,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(in.Version) == "" {
		writeError(w, http.StatusBadRequest, "version required")
		return
	}

	var cfg console.TenantPolicyConfig
	if in.PolicyData != nil {
		cfg = *in.PolicyData
		if err := validateTenantPolicyConfig(&cfg); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	} else {
		existing, found, err := api.store.GetTenantPolicyConfig(r.Context(), tenantID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load current tenant policy config")
			return
		}
		if found && existing != nil {
			cfg = *existing
		} else {
			cfg = console.TenantPolicyConfig{
				MaxRiskAutoApprove:         7,
				RequireDestructiveApproval: true,
			}
		}
	}

	rawPolicy, _ := json.Marshal(cfg)
	tenant := tenantID
	pv, err := api.store.CreatePolicyVersion(r.Context(), &tenant, in.Version, "", claims.Email, in.Notes, rawPolicy)
	if err != nil {
		api.log.Error("create tenant policy version failed", "error", err, "tenant_id", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to create tenant policy version")
		return
	}
	writeJSON(w, http.StatusCreated, pv)
}

func (api *ConsoleAPI) handleRollbackTenantPolicyVersion(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	claims := claimsFromCtx(r.Context())
	versionIDStr := chi.URLParam(r, "version_id")
	versionID, err := strconv.ParseInt(versionIDStr, 10, 64)
	if err != nil || versionID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid version_id")
		return
	}

	pv, err := api.store.GetPolicyVersion(r.Context(), versionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load policy version")
		return
	}
	if pv == nil {
		writeError(w, http.StatusNotFound, "policy version not found")
		return
	}
	if pv.TenantID == nil || *pv.TenantID != tenantID {
		writeError(w, http.StatusForbidden, "policy version does not belong to tenant")
		return
	}

	var cfg console.TenantPolicyConfig
	if err := json.Unmarshal(pv.PolicyData, &cfg); err != nil {
		writeError(w, http.StatusBadRequest, "selected policy version has invalid policy_data")
		return
	}
	if err := validateTenantPolicyConfig(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := api.store.SetTenantPolicyConfig(r.Context(), tenantID, cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to rollback tenant policy config")
		return
	}

	nowVersion := "rollback-" + time.Now().UTC().Format("20060102150405")
	rawPolicy, _ := json.Marshal(cfg)
	tenant := tenantID
	rollbackVersion, err := api.store.CreatePolicyVersion(
		r.Context(), &tenant, nowVersion, "", claims.Email, "rollback to version "+pv.Version, rawPolicy,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create rollback policy version")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"rolled_back_to":  pv,
		"created_version": rollbackVersion,
	})
}

func (api *ConsoleAPI) handleSimulateTenantPolicy(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in struct {
		AgentID      string                      `json:"agent_id"`
		Tool         string                      `json:"tool"`
		Action       string                      `json:"action"`
		Resource     string                      `json:"resource"`
		RiskScore    int                         `json:"risk_score"`
		PolicyConfig *console.TenantPolicyConfig `json:"policy_config,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	cfg := console.TenantPolicyConfig{
		MaxRiskAutoApprove:         7,
		RequireDestructiveApproval: true,
	}
	if in.PolicyConfig != nil {
		cfg = *in.PolicyConfig
	} else {
		existing, found, err := api.store.GetTenantPolicyConfig(r.Context(), tenantID)
		if err != nil {
			api.log.Error("get tenant policy config failed", "error", err, "tenant_id", tenantID)
			writeError(w, http.StatusInternalServerError, "failed to load tenant policy config")
			return
		}
		if found && existing != nil {
			cfg = *existing
		}
	}
	if err := validateTenantPolicyConfig(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	opaURL := config.EnvOr("OPA_URL", "http://localhost:8181")
	body, err := json.Marshal(map[string]any{
		"input": map[string]any{
			"toolcall": map[string]any{
				"tenant_id":  tenantID,
				"agent_id":   in.AgentID,
				"tool":       in.Tool,
				"action":     in.Action,
				"resource":   in.Resource,
				"risk_score": in.RiskScore,
			},
			"environment": map[string]any{
				"timestamp":     time.Now().UTC(),
				"tenant_config": cfg.ToPolicyInputMap(),
			},
		},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build simulation request")
		return
	}

	opaResp, apiErr := api.callPolicySimulationOPA(r.Context(), opaURL, body)
	if apiErr != nil {
		apiErr.WriteJSON(w)
		return
	}
	override := policy.EvaluateWithRuleBuilder(types.ToolCallRequest{
		TenantID:  tenantID,
		AgentID:   in.AgentID,
		Tool:      in.Tool,
		Action:    in.Action,
		Resource:  in.Resource,
		RiskScore: in.RiskScore,
	}, policy.RuleBuilderConfig{
		MaxRiskAutoApprove:         cfg.MaxRiskAutoApprove,
		ReadActions:                cfg.ReadActions,
		WriteActions:               cfg.WriteActions,
		DestructiveActions:         cfg.DestructiveActions,
		RequireDestructiveApproval: cfg.RequireDestructiveApproval,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"simulation":    true,
		"tenant_id":     tenantID,
		"policy_config": cfg,
		"policy_result": map[string]any{
			"opa_result": opaResp,
			"result": map[string]any{
				"decision": override.Decision,
				"reason":   override.Reason,
			},
		},
	})
}

func (api *ConsoleAPI) callPolicySimulationOPA(ctx context.Context, opaURL string, body []byte) (map[string]any, *types.APIError) {
	client := api.httpClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(opaURL, "/")+policy.OPAPolicyPath, strings.NewReader(string(body)))
	if err != nil {
		return nil, types.ErrInternal("failed to build simulation request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, &types.APIError{
			Code:      "BAD_GATEWAY",
			Message:   "failed to reach policy engine",
			HTTPCode:  http.StatusBadGateway,
			Retryable: true,
			Details: map[string]any{
				"stage":      "reachability",
				"target":     req.URL.String(),
				"suggestion": "Check OPA_URL, confirm the policy engine is running, and verify the policy bundle is loaded.",
			},
		}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		api.log.Error("read OPA simulation response failed", "error", err)
		return nil, &types.APIError{
			Code:      "BAD_GATEWAY",
			Message:   "failed to read policy engine response",
			HTTPCode:  http.StatusBadGateway,
			Retryable: true,
			Details: map[string]any{
				"stage":      "response_read",
				"target":     req.URL.String(),
				"suggestion": "Check policy engine logs for truncated or oversized responses.",
			},
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyExcerpt := strings.TrimSpace(string(respBody))
		if len(bodyExcerpt) > 240 {
			bodyExcerpt = bodyExcerpt[:240] + "..."
		}
		api.log.Error("policy engine returned non-success status", "status", resp.StatusCode, "body", strings.TrimSpace(string(respBody)))
		return nil, &types.APIError{
			Code:      "BAD_GATEWAY",
			Message:   fmt.Sprintf("policy engine returned %d", resp.StatusCode),
			HTTPCode:  http.StatusBadGateway,
			Retryable: resp.StatusCode >= 500,
			Details: map[string]any{
				"stage":           "upstream_status",
				"target":          req.URL.String(),
				"upstream_status": resp.StatusCode,
				"body_excerpt":    bodyExcerpt,
				"suggestion":      "Check the OPA process, policy bundle load, and recent OPA logs before retrying policy simulation.",
			},
		}
	}
	var opaResp map[string]any
	if err := json.Unmarshal(respBody, &opaResp); err != nil {
		api.log.Error("decode OPA simulation response failed", "error", err)
		bodyExcerpt := strings.TrimSpace(string(respBody))
		if len(bodyExcerpt) > 240 {
			bodyExcerpt = bodyExcerpt[:240] + "..."
		}
		return nil, &types.APIError{
			Code:      "BAD_GATEWAY",
			Message:   "invalid policy engine response",
			HTTPCode:  http.StatusBadGateway,
			Retryable: true,
			Details: map[string]any{
				"stage":        "decode",
				"target":       req.URL.String(),
				"body_excerpt": bodyExcerpt,
				"suggestion":   "Check whether the policy engine is returning JSON and whether the policy path is correct.",
			},
		}
	}
	return opaResp, nil
}
