package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/bturcanu/OpenClause/pkg/approvals"
	"github.com/bturcanu/OpenClause/pkg/console"
	"github.com/bturcanu/OpenClause/pkg/types"
	"github.com/go-chi/chi/v5"
)

var (
	defaultTenantNotifOnce sync.Once
	defaultTenantNotif     map[string]*console.TenantNotificationConfig
	defaultTenantNotifErr  error
)

func defaultTenantNotificationConfig(tenantID string) (*console.TenantNotificationConfig, error) {
	defaultTenantNotifOnce.Do(func() {
		defaultTenantNotif = map[string]*console.TenantNotificationConfig{}
		// Best-effort fallback for demo/UI friendliness.
		// If policy bundle files are unavailable (e.g. production), we return empty configs.
		b, err := os.ReadFile("policy/bundles/v0/data.json")
		if err != nil {
			defaultTenantNotifErr = err
			return
		}

		var raw struct {
			Tenants map[string]struct {
				ApproverGroup string               `json:"approver_group"`
				Notify        []types.PolicyNotify `json:"notify"`
			} `json:"tenants"`
		}
		if err := json.Unmarshal(b, &raw); err != nil {
			defaultTenantNotifErr = err
			return
		}

		for tid, t := range raw.Tenants {
			cfg := &console.TenantNotificationConfig{
				ApproverGroup: t.ApproverGroup,
				Notify:        t.Notify,
			}
			defaultTenantNotif[tid] = cfg
		}
	})

	// If we failed to load defaults, don't block the API: return empty config
	// and let user-driven PUT configure it.
	if defaultTenantNotifErr != nil {
		return &console.TenantNotificationConfig{Notify: []types.PolicyNotify{}}, nil
	}
	if cfg := defaultTenantNotif[tenantID]; cfg != nil {
		return cfg, nil
	}
	return &console.TenantNotificationConfig{Notify: []types.PolicyNotify{}}, nil
}

func normalizeTenantNotificationConfig(in console.TenantNotificationConfig) (console.TenantNotificationConfig, error) {
	out := in
	out.ApproverGroup = strings.TrimSpace(out.ApproverGroup)

	// Normalize nil to empty so gateway can interpret “found but empty”.
	if out.Notify == nil {
		out.Notify = []types.PolicyNotify{}
	}

	normalized := make([]types.PolicyNotify, 0, len(out.Notify))
	for _, n := range out.Notify {
		kind := strings.ToLower(strings.TrimSpace(n.Kind))
		switch kind {
		case "slack":
			channel := strings.TrimSpace(n.Channel)
			if channel == "" {
				return console.TenantNotificationConfig{}, fmt.Errorf("slack notify requires channel")
			}
			normalized = append(normalized, types.PolicyNotify{
				Kind:    "slack",
				Channel: channel,
			})
		case "webhook":
			url := strings.TrimSpace(n.URL)
			secretRef := strings.TrimSpace(n.SecretRef)
			if url == "" {
				return console.TenantNotificationConfig{}, fmt.Errorf("webhook notify requires url")
			}
			if secretRef == "" {
				return console.TenantNotificationConfig{}, fmt.Errorf("webhook notify requires secret_ref")
			}
			if err := approvals.ValidateWebhookURL(url); err != nil {
				return console.TenantNotificationConfig{}, fmt.Errorf("webhook URL validation: %w", err)
			}
			normalized = append(normalized, types.PolicyNotify{
				Kind:      "webhook",
				URL:       url,
				SecretRef: secretRef,
			})
		default:
			return console.TenantNotificationConfig{}, fmt.Errorf("unsupported notify kind %q", n.Kind)
		}
	}

	out.Notify = normalized
	return out, nil
}

func (api *ConsoleAPI) handleGetTenantNotificationConfig(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id required")
		return
	}

	cfg, found, err := api.notificationConfigStore.GetTenantNotificationConfig(r.Context(), tenantID)
	if err != nil {
		api.log.Error("get notification config failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load notification config")
		return
	}

	if !found || cfg == nil {
		// UI-friendly defaults. DB is not mutated by GET.
		cfg, _ = defaultTenantNotificationConfig(tenantID)
	}

	writeJSON(w, http.StatusOK, cfg)
}

func (api *ConsoleAPI) handleUpdateTenantNotificationConfig(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id required")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	var in console.TenantNotificationConfig
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	cfg, err := normalizeTenantNotificationConfig(in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := api.notificationConfigStore.SetTenantNotificationConfig(r.Context(), tenantID, cfg); err != nil {
		api.log.Error("update notification config failed", "error", err, "tenant_id", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to update notification config")
		return
	}

	writeJSON(w, http.StatusOK, cfg)
}

// ensure unused imports don't regress during refactors.
var _ = errors.New
var _ = context.Background
var _ = slog.LevelInfo
