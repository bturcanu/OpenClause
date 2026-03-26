package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bturcanu/OpenClause/pkg/alerts"
	"github.com/bturcanu/OpenClause/pkg/approvals"
	"github.com/bturcanu/OpenClause/pkg/config"
	"github.com/bturcanu/OpenClause/pkg/connectors"
	"github.com/bturcanu/OpenClause/pkg/console"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultPollIntervalSec  = 30
	defaultBatchSize        = 100
	maxNotificationAttempts = 10
	maxBackoff              = 5 * time.Minute
)

const knownInsecureInternalToken = "dev-internal-token-change-me"

var alertLookupIPAddrs = func(ctx context.Context, host string) ([]net.IP, error) {
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	out := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		out = append(out, ip.IP)
	}
	return out, nil
}

var alertDialResolvedAddress = func(ctx context.Context, network, address string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return dialer.DialContext(ctx, network, address)
}

type tenantNotificationConfigGetter interface {
	GetTenantNotificationConfig(ctx context.Context, tenantID string) (*console.TenantNotificationConfig, bool, error)
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	pollIntervalSec := config.EnvOrInt("ALERT_WORKER_INTERVAL_SEC", defaultPollIntervalSec)
	batchSize := config.EnvOrInt("ALERT_WORKER_BATCH_SIZE", defaultBatchSize)

	internalToken := os.Getenv("INTERNAL_AUTH_TOKEN")
	if internalToken == "" || internalToken == knownInsecureInternalToken {
		log.Error("INTERNAL_AUTH_TOKEN is required and must not use the default placeholder for slack connector calls")
		os.Exit(1)
	}

	slackConnectorURL := strings.TrimRight(config.EnvOr("CONNECTOR_SLACK_URL", "http://localhost:8082"), "/")
	if slackConnectorURL == "" {
		log.Error("CONNECTOR_SLACK_URL must not be empty")
		os.Exit(1)
	}

	secrets := approvals.ParseSecretRefMap(os.Getenv("WEBHOOK_SECRET_REFS"))

	pool, err := newPostgresPool(ctx)
	if err != nil {
		log.Error("postgres connect failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	store := console.NewStore(pool)

	// Dispatch loop.
	t := time.NewTicker(time.Duration(pollIntervalSec) * time.Second)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("alert-worker shutting down")
			return
		case <-t.C:
			if err := runTick(ctx, store, slackConnectorURL, internalToken, secrets, batchSize); err != nil {
				log.Error("alert-worker tick failed", "error", err)
			}
		}
	}
}

func newPostgresPool(ctx context.Context) (*pgxpool.Pool, error) {
	return pgxpool.New(ctx, config.PostgresDSN())
}

func runTick(
	ctx context.Context,
	store *console.Store,
	slackConnectorURL, internalToken string,
	secrets map[string]string,
	batchSize int,
) error {
	now := time.Now().UTC()

	// 1) Evaluate deny_spike rules and create alert events (deduped).
	rules, err := store.ListEnabledDenySpikeRules(ctx)
	if err != nil {
		return fmt.Errorf("list enabled deny_spike rules: %w", err)
	}
	for _, rule := range rules {
		cfg, err := alerts.ParseDenySpikeConfig(rule.Config)
		if err != nil {
			slog.Error("invalid deny_spike config; skipping rule", "rule_id", rule.ID, "error", err)
			continue
		}

		since := now.Add(-time.Duration(cfg.MMinutes) * time.Minute)
		denyCount, err := store.CountDenyToolEventsInWindow(ctx, rule.TenantID, since)
		if err != nil {
			slog.Error("deny spike count failed", "rule_id", rule.ID, "tenant_id", rule.TenantID, "error", err)
			continue
		}

		exists, err := store.AlertEventExistsInWindow(ctx, rule.TenantID, rule.ID, since)
		if err != nil {
			slog.Error("dedupe check failed", "rule_id", rule.ID, "tenant_id", rule.TenantID, "error", err)
			continue
		}

		if !alerts.ShouldCreateAlertEvent(denyCount, cfg, exists) {
			continue
		}

		eventMessage := fmt.Sprintf("deny spike: %d denies in last %d minutes (threshold %d)", denyCount, cfg.MMinutes, cfg.N)
		ctxJSON, _ := json.Marshal(map[string]any{
			"tenant_id":  rule.TenantID,
			"rule_id":    rule.ID,
			"n":          cfg.N,
			"m_minutes":  cfg.MMinutes,
			"deny_count": denyCount,
			"since":      since.Format(time.RFC3339),
			"window":     cfg.MMinutes,
		})

		event, err := store.CreateAlertEvent(ctx, rule.ID, rule.TenantID, "warning", eventMessage, ctxJSON)
		if err != nil {
			slog.Error("create alert event failed", "rule_id", rule.ID, "tenant_id", rule.TenantID, "error", err)
			continue
		}

		if err := dispatchAlertEvent(ctx, store, event, slackConnectorURL, internalToken, secrets); err != nil {
			attempts := event.AttemptCount + 1
			if attempts > maxNotificationAttempts {
				// Give up but keep a trace.
				_ = store.MarkAlertEventPendingRetry(ctx, event.ID, attempts, time.Now().UTC().Add(1*time.Hour), err.Error())
				continue
			}
			next := time.Now().UTC().Add(backoffForAttempt(attempts))
			if markErr := store.MarkAlertEventPendingRetry(ctx, event.ID, attempts, next, err.Error()); markErr != nil {
				slog.Error("mark alert event retry failed", "event_id", event.ID, "error", markErr)
			}
			continue
		}

		if err := store.MarkAlertEventSent(ctx, event.ID); err != nil {
			slog.Error("mark alert event sent failed", "event_id", event.ID, "error", err)
			continue
		}
	}

	// 2) Retry pending events due (notification backoff).
	pending, err := store.ClaimPendingAlertEventsDue(ctx, batchSize)
	if err != nil {
		return fmt.Errorf("claim pending alert events: %w", err)
	}
	for _, event := range pending {
		if err := dispatchAlertEvent(ctx, store, &event, slackConnectorURL, internalToken, secrets); err != nil {
			attempts := event.AttemptCount + 1
			if attempts > maxNotificationAttempts {
				// Give up but keep a trace.
				_ = store.MarkAlertEventPendingRetry(ctx, event.ID, attempts, time.Now().UTC().Add(1*time.Hour), err.Error())
				continue
			}

			next := time.Now().UTC().Add(backoffForAttempt(attempts))
			if markErr := store.MarkAlertEventPendingRetry(ctx, event.ID, attempts, next, err.Error()); markErr != nil {
				slog.Error("mark retry failed", "event_id", event.ID, "error", markErr)
			}
			continue
		}
		if err := store.MarkAlertEventSent(ctx, event.ID); err != nil {
			slog.Error("mark alert event sent failed", "event_id", event.ID, "error", err)
		}
	}

	return nil
}

func backoffForAttempt(attempt int) time.Duration {
	if attempt <= 0 {
		return time.Second
	}
	d := time.Second * time.Duration(1<<min(attempt, 8))
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func dispatchAlertEvent(
	ctx context.Context,
	cfgGetter tenantNotificationConfigGetter,
	event *console.AlertEvent,
	slackConnectorURL, internalToken string,
	secrets map[string]string,
) error {
	tenantCfg, found, err := cfgGetter.GetTenantNotificationConfig(ctx, event.TenantID)
	if err != nil {
		return fmt.Errorf("load tenant notification config: %w", err)
	}
	if !found || tenantCfg == nil || len(tenantCfg.Notify) == 0 {
		return nil // safe default: no configured sinks
	}

	// Reuse a single HTTP client per dispatch to keep dial overhead low.
	httpClient := &http.Client{Timeout: 10 * time.Second, Transport: safeTransport()}

	// Slack connector calls don't need SSRF transport (it's internal), keep it simple.
	slackClient := &http.Client{Timeout: 10 * time.Second}

	var lastErr error
	deliveredAtLeastOne := false
	for _, n := range tenantCfg.Notify {
		switch strings.ToLower(n.Kind) {
		case "slack":
			if n.Channel == "" {
				lastErr = fmt.Errorf("slack channel is empty")
				continue
			}
			if err := postToSlackConnector(ctx, slackClient, slackConnectorURL, internalToken, event, n.Channel); err != nil {
				lastErr = err
				continue
			}
			deliveredAtLeastOne = true
		case "webhook":
			if n.URL == "" {
				lastErr = fmt.Errorf("webhook url is empty")
				continue
			}
			if err := postToWebhook(ctx, httpClient, n.URL, secrets, n.SecretRef, event); err != nil {
				lastErr = err
				continue
			}
			deliveredAtLeastOne = true
		default:
			lastErr = fmt.Errorf("unsupported notify kind %q", n.Kind)
			continue
		}
	}

	// If multiple sinks are configured, consider the event successfully dispatched
	// once at least one destination receives it. Otherwise, a single failing
	// destination would cause unnecessary retry storms.
	if deliveredAtLeastOne {
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return nil
}

func postToSlackConnector(
	ctx context.Context,
	client *http.Client,
	slackConnectorURL, internalToken string,
	event *console.AlertEvent,
	channel string,
) error {
	type slackMsgParams struct {
		Channel string `json:"channel"`
		Text    string `json:"text"`
	}
	paramsBytes, _ := json.Marshal(slackMsgParams{
		Channel: channel,
		Text:    event.Message,
	})

	execReq := connectors.ExecRequest{
		EventID:  event.ID,
		TenantID: event.TenantID,
		AgentID:  "",
		Tool:     "slack",
		Action:   "msg.post",
		Params:   paramsBytes,
		Resource: "",
	}
	body, _ := json.Marshal(execReq)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, slackConnectorURL+"/exec", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", internalToken)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var execResp connectors.ExecResponse
	if err := json.NewDecoder(resp.Body).Decode(&execResp); err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("slack connector http=%d status=%s err=%s", resp.StatusCode, execResp.Status, execResp.Error)
	}
	if strings.ToLower(execResp.Status) != "success" {
		return fmt.Errorf("slack connector delivery failed: %s", execResp.Error)
	}
	return nil
}

func postToWebhook(
	ctx context.Context,
	client *http.Client,
	rawURL string,
	secrets map[string]string,
	secretRef string,
	event *console.AlertEvent,
) error {
	if err := approvals.ValidateWebhookURL(rawURL); err != nil {
		return err
	}

	payload := map[string]any{
		"specversion":     "1.0",
		"type":            "oc.alert",
		"id":              event.ID,
		"time":            time.Now().UTC().Format(time.RFC3339Nano),
		"datacontenttype": "application/json",
		"data": map[string]any{
			"tenant_id":    event.TenantID,
			"rule_id":      event.RuleID,
			"severity":     event.Severity,
			"message":      event.Message,
			"created_at":   event.CreatedAt.Format(time.RFC3339),
			"context_json": json.RawMessage(event.ContextJSON),
		},
	}

	rawBody, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(rawBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	if secretRef != "" {
		if secret := secrets[secretRef]; secret != "" {
			req.Header.Set("X-OC-Signature-256", approvals.SignBodyHMACSHA256(rawBody, secret))
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook status_code=%d", resp.StatusCode)
	}
	return nil
}

func safeTransport() *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("invalid address: %w", err)
			}
			ips, err := alertLookupIPAddrs(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("dns resolve: %w", err)
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("dns resolve: no IPs for %q", host)
			}
			allowed := make([]net.IP, 0, len(ips))
			for _, ip := range ips {
				if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
					return nil, fmt.Errorf("resolved IP %s is private/loopback — blocked", ip)
				}
				allowed = append(allowed, ip)
			}
			var lastErr error
			for _, ip := range allowed {
				conn, err := alertDialResolvedAddress(ctx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			if lastErr != nil {
				return nil, fmt.Errorf("dial resolved ip: %w", lastErr)
			}
			return nil, fmt.Errorf("dial resolved ip: no connections succeeded")
		},
	}
}
