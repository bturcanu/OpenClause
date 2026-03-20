package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/bturcanu/OpenClause/pkg/alerts"
	"github.com/bturcanu/OpenClause/pkg/console"
	"github.com/go-chi/chi/v5"
)

type alertRuleCreateInput struct {
	TenantID   string          `json:"tenant_id,omitempty"`
	Name       string          `json:"name"`
	Kind       string          `json:"kind"`
	Enabled    *bool           `json:"enabled,omitempty"`
	ConfigJSON json.RawMessage `json:"config_json"`
}

type alertRuleUpdateInput struct {
	Name       string          `json:"name"`
	Kind       string          `json:"kind"`
	Enabled    bool            `json:"enabled"`
	ConfigJSON json.RawMessage `json:"config_json"`
}

type alertRuleResponse struct {
	ID         string          `json:"id"`
	TenantID   string          `json:"tenant_id"`
	Name       string          `json:"name"`
	Kind       string          `json:"kind"`
	Enabled    bool            `json:"enabled"`
	ConfigJSON json.RawMessage `json:"config_json"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

func toAlertRuleResponse(r *console.AlertRule) alertRuleResponse {
	return alertRuleResponse{
		ID:         r.ID,
		TenantID:   r.TenantID,
		Name:       r.Name,
		Kind:       r.RuleType,
		Enabled:    r.Enabled,
		ConfigJSON: r.Config,
		CreatedAt:  r.CreatedAt,
		UpdatedAt:  r.UpdatedAt,
	}
}

func buildAlertRule(tenantID string, in alertRuleCreateInput) (*console.AlertRule, error) {
	name, err := normalizeRequiredName(in.Name, "name")
	if err != nil {
		return nil, err
	}

	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		tenantID = strings.TrimSpace(in.TenantID)
	}
	if tenantID == "" {
		return nil, errBadRequest("tenant_id required")
	}

	kind := strings.ToLower(strings.TrimSpace(in.Kind))
	if kind == "" {
		return nil, errBadRequest("kind required")
	}
	if kind != "deny_spike" {
		return nil, errBadRequest("unsupported alert kind")
	}
	if len(in.ConfigJSON) == 0 {
		return nil, errBadRequest("config_json required")
	}

	cfg, err := alerts.ParseDenySpikeConfig(in.ConfigJSON)
	if err != nil {
		return nil, err
	}

	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}

	canonCfg := map[string]any{"n": cfg.N, "m_minutes": cfg.MMinutes}
	canonBytes, _ := json.Marshal(canonCfg)

	return &console.AlertRule{
		TenantID: tenantID,
		Name:     name,
		RuleType: kind,
		Config:   canonBytes,
		Enabled:  enabled,
	}, nil
}

func errBadRequest(msg string) error {
	return &badRequestError{msg: msg}
}

type badRequestError struct {
	msg string
}

func (e *badRequestError) Error() string {
	return e.msg
}

// handleListTenantAlertRules handles:
//
//	GET /admin/tenants/{tenant_id}/alerts/rules
func (api *ConsoleAPI) handleListTenantAlertRules(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	rules, err := api.alertsStore.ListAlertRules(r.Context(), tenantID)
	if err != nil {
		api.log.Error("list alert rules failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list alert rules")
		return
	}
	out := make([]alertRuleResponse, 0, len(rules))
	for i := range rules {
		rr := rules[i]
		out = append(out, toAlertRuleResponse(&rr))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateTenantAlertRule handles:
//
//	POST /admin/tenants/{tenant_id}/alerts/rules
func (api *ConsoleAPI) handleCreateTenantAlertRule(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in alertRuleCreateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	ruleInput, err := buildAlertRule(tenantID, in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	rule, err := api.alertsStore.CreateAlertRule(r.Context(), *ruleInput)
	if err != nil {
		api.log.Error("create alert rule failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create alert rule")
		return
	}
	writeJSON(w, http.StatusCreated, toAlertRuleResponse(rule))
}

// handleUpdateTenantAlertRule handles:
//
//	PUT /admin/tenants/{tenant_id}/alerts/rules/{rule_id}
func (api *ConsoleAPI) handleUpdateTenantAlertRule(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	ruleID := chi.URLParam(r, "rule_id")

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in alertRuleUpdateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if ruleID == "" {
		writeError(w, http.StatusBadRequest, "rule_id required")
		return
	}
	name, err := normalizeRequiredName(in.Name, "name")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	kind := strings.ToLower(strings.TrimSpace(in.Kind))
	if kind == "" {
		writeError(w, http.StatusBadRequest, "kind required")
		return
	}
	if kind != "deny_spike" {
		writeError(w, http.StatusBadRequest, "unsupported alert kind")
		return
	}
	if len(in.ConfigJSON) == 0 {
		writeError(w, http.StatusBadRequest, "config_json required")
		return
	}

	cfg, err := alerts.ParseDenySpikeConfig(in.ConfigJSON)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	canonCfg := map[string]any{"n": cfg.N, "m_minutes": cfg.MMinutes}
	canonBytes, _ := json.Marshal(canonCfg)

	if err := api.alertsStore.UpdateAlertRule(r.Context(), tenantID, ruleID, name, kind, canonBytes, in.Enabled); err != nil {
		api.log.Error("update alert rule failed", "error", err, "tenant_id", tenantID, "rule_id", ruleID)
		writeError(w, http.StatusInternalServerError, "failed to update alert rule")
		return
	}
	rule, err := api.alertsStore.GetAlertRule(r.Context(), tenantID, ruleID)
	if err != nil || rule == nil {
		writeError(w, http.StatusNotFound, "alert rule not found")
		return
	}
	writeJSON(w, http.StatusOK, toAlertRuleResponse(rule))
}

// handleDeleteTenantAlertRule handles:
//
//	DELETE /admin/tenants/{tenant_id}/alerts/rules/{rule_id}
func (api *ConsoleAPI) handleDeleteTenantAlertRule(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	ruleID := chi.URLParam(r, "rule_id")
	if ruleID == "" {
		writeError(w, http.StatusBadRequest, "rule_id required")
		return
	}
	if err := api.alertsStore.DeleteAlertRule(r.Context(), tenantID, ruleID); err != nil {
		api.log.Error("delete alert rule failed", "error", err, "tenant_id", tenantID, "rule_id", ruleID)
		writeError(w, http.StatusInternalServerError, "failed to delete alert rule")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListTenantAlertEvents handles:
//
//	GET /admin/tenants/{tenant_id}/alerts/events?limit=&since=
func (api *ConsoleAPI) handleListTenantAlertEvents(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")

	// Parse `since` with a safe default: last 24h.
	since := parseSince(r, 24*time.Hour)
	limit, _ := parsePagination(r)
	events, err := api.alertsStore.ListAlertEventsSince(r.Context(), tenantID, since, limit)
	if err != nil {
		api.log.Error("list alert events failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list alert events")
		return
	}
	writeJSON(w, http.StatusOK, events)
}
