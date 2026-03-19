package main

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bturcanu/OpenClause/pkg/console"
	"github.com/go-chi/chi/v5"
)

func parseRangeDuration(r *http.Request, defaultDuration time.Duration) time.Duration {
	v := strings.TrimSpace(r.URL.Query().Get("range"))
	if v == "" {
		return defaultDuration
	}

	// Allow raw integer as "hours".
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return time.Duration(n) * time.Hour
	}

	// Support "<num>d" for days (not supported by time.ParseDuration).
	if strings.HasSuffix(strings.ToLower(v), "d") {
		raw := strings.TrimSpace(v[:len(v)-1])
		f, err := strconv.ParseFloat(raw, 64)
		if err == nil && f > 0 {
			d := time.Duration(f * 24.0 * float64(time.Hour))
			return d
		}
	}

	// For h/m/s use time.ParseDuration.
	if d, err := time.ParseDuration(v); err == nil && d > 0 {
		return d
	}

	return defaultDuration
}

func parseBucketMinutes(r *http.Request, defaultValue int) int {
	if v := strings.TrimSpace(r.URL.Query().Get("bucket_minutes")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 5 && n <= 1440 {
			return n
		}
	}
	return defaultValue
}

func parseTopAgents(r *http.Request, defaultValue int) int {
	if v := strings.TrimSpace(r.URL.Query().Get("top_agents")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 50 {
			return n
		}
	}
	return defaultValue
}

func (api *ConsoleAPI) handleTenantAnalyticsSummary(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id required")
		return
	}

	now := time.Now().UTC()
	dur := parseRangeDuration(r, 24*time.Hour)
	// Clamp to avoid accidental huge scans.
	const maxRange = 30 * 24 * time.Hour
	if dur > maxRange {
		dur = maxRange
	}
	since := now.Add(-dur)

	bucketMinutes := parseBucketMinutes(r, 60)
	topAgents := parseTopAgents(r, 5)

	summary, err := api.analyticsStore.GetTenantAnalyticsSummary(r.Context(), tenantID, since, bucketMinutes, topAgents)
	if err != nil {
		api.log.Error("tenant analytics summary failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get tenant analytics")
		return
	}

	// Defensive: ensure non-nil and consistent JSON shape.
	if summary == nil {
		summary = &console.TenantAnalyticsSummary{}
	}

	writeJSON(w, http.StatusOK, summary)
}

