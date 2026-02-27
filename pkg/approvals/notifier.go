package approvals

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bturcanu/OpenClause/pkg/connectors"
)

const (
	defaultDispatchBatchSize = 100
	maxDispatchBackoff       = 5 * time.Minute
	maxNotificationAttempts  = 10
)

// Summarizer builds human-friendly notification summaries from sanitized fields.
type Summarizer interface {
	Summarize(NotificationOutbox) string
}

// TemplateSummarizer is deterministic and does not use model inference.
type TemplateSummarizer struct{}

func (TemplateSummarizer) Summarize(n NotificationOutbox) string {
	return fmt.Sprintf(
		"Approval requested: %s.%s on %s (risk=%d, reason=%s)",
		n.Tool, n.Action, n.Resource, n.RiskScore, n.Reason,
	)
}

type Dispatcher struct {
	store                 notificationStore
	httpClient            *http.Client
	source                string
	secrets               map[string]string
	summarizer            Summarizer
	slackURL              string
	internalToken         string
	SkipWebhookValidation bool // testing only — disables SSRF URL checks
}

type notificationStore interface {
	ClaimDueNotifications(context.Context, int) ([]NotificationOutbox, error)
	MarkNotificationSent(context.Context, string) error
	MarkNotificationRetry(context.Context, string, int, time.Time, string) error
	MarkNotificationFailed(context.Context, string, string) error
}

func NewDispatcher(store notificationStore, source string, secrets map[string]string, slackURL, internalToken string) *Dispatcher {
	return &Dispatcher{
		store:         store,
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		source:        source,
		secrets:       secrets,
		summarizer:    TemplateSummarizer{},
		slackURL:      strings.TrimRight(slackURL, "/"),
		internalToken: internalToken,
	}
}

func (d *Dispatcher) DispatchOnce(ctx context.Context) error {
	items, err := d.store.ClaimDueNotifications(ctx, defaultDispatchBatchSize)
	if err != nil {
		return err
	}
	for _, item := range items {
		switch strings.ToLower(item.NotifyKind) {
		case "webhook":
			if item.NotifyURL == "" {
				_ = d.store.MarkNotificationFailed(ctx, item.ID, "webhook notify_url is empty")
				continue
			}
			if err := d.deliverWebhook(ctx, item); err != nil {
				if item.Attempts >= maxNotificationAttempts {
					if markErr := d.store.MarkNotificationFailed(ctx, item.ID, "max retries exceeded: "+err.Error()); markErr != nil {
						slog.Error("mark notification failed error", "id", item.ID, "error", markErr)
					}
					continue
				}
				next := time.Now().UTC().Add(backoffForAttempt(item.Attempts))
				if markErr := d.store.MarkNotificationRetry(ctx, item.ID, item.Attempts, next, err.Error()); markErr != nil {
					slog.Error("mark notification retry error", "id", item.ID, "error", markErr)
				}
				continue
			}
			if markErr := d.store.MarkNotificationSent(ctx, item.ID); markErr != nil {
				slog.Error("mark notification sent error", "id", item.ID, "error", markErr)
			}
		case "slack":
			if item.SlackChannel == "" {
				_ = d.store.MarkNotificationFailed(ctx, item.ID, "slack channel is empty")
				continue
			}
			if err := d.deliverSlack(ctx, item); err != nil {
				if item.Attempts >= maxNotificationAttempts {
					if markErr := d.store.MarkNotificationFailed(ctx, item.ID, "max retries exceeded: "+err.Error()); markErr != nil {
						slog.Error("mark notification failed error", "id", item.ID, "error", markErr)
					}
					continue
				}
				next := time.Now().UTC().Add(backoffForAttempt(item.Attempts))
				if markErr := d.store.MarkNotificationRetry(ctx, item.ID, item.Attempts, next, err.Error()); markErr != nil {
					slog.Error("mark notification retry error", "id", item.ID, "error", markErr)
				}
				continue
			}
			if markErr := d.store.MarkNotificationSent(ctx, item.ID); markErr != nil {
				slog.Error("mark notification sent error", "id", item.ID, "error", markErr)
			}
		default:
			_ = d.store.MarkNotificationFailed(ctx, item.ID, "unsupported notify kind")
		}
	}
	return nil
}

func ValidateWebhookURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("only https scheme allowed, got %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("empty hostname")
	}
	ip := net.ParseIP(host)
	if ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("private/loopback IP not allowed: %s", ip)
		}
	}
	return nil
}

func (d *Dispatcher) deliverWebhook(ctx context.Context, item NotificationOutbox) error {
	if !d.SkipWebhookValidation {
		if err := ValidateWebhookURL(item.NotifyURL); err != nil {
			return fmt.Errorf("webhook URL validation: %w", err)
		}
	}
	body, err := BuildApprovalRequestedCloudEvent(item, d.source, d.summarizer.Summarize(item))
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, item.NotifyURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/cloudevents+json")
	req.Header.Set("Ce-Specversion", "1.0")
	req.Header.Set("Ce-Type", "oc.approval.requested")
	req.Header.Set("Ce-Id", item.ID)
	req.Header.Set("Ce-Source", d.source)
	if secret, ok := d.secrets[item.SecretRef]; ok && secret != "" {
		req.Header.Set("X-OC-Signature-256", SignBodyHMACSHA256(body, secret))
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("webhook status=%d", resp.StatusCode)
}

func (d *Dispatcher) deliverSlack(ctx context.Context, item NotificationOutbox) error {
	if d.slackURL == "" {
		return fmt.Errorf("slack connector url is empty")
	}
	params := map[string]any{
		"channel":             item.SlackChannel,
		"tool":                item.Tool,
		"action":              item.Action,
		"resource":            item.Resource,
		"risk_score":          item.RiskScore,
		"reason":              item.Reason,
		"approval_url":        item.ApprovalURL,
		"approval_request_id": item.ApprovalRequestID,
		"event_id":            item.EventID,
		"tenant_id":           item.TenantID,
		"risk_factors":        item.RiskFactors,
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return err
	}
	execReqBody, err := json.Marshal(connectors.ExecRequest{
		EventID:  item.EventID,
		TenantID: item.TenantID,
		Tool:     "slack",
		Action:   "approval.request",
		Params:   paramsJSON,
		Resource: item.Resource,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.slackURL+"/exec", bytes.NewReader(execReqBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if d.internalToken != "" {
		req.Header.Set("X-Internal-Token", d.internalToken)
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("slack connector status=%d", resp.StatusCode)
	}
	var execResp connectors.ExecResponse
	if err := json.NewDecoder(resp.Body).Decode(&execResp); err != nil {
		return err
	}
	if execResp.Status != "success" {
		return fmt.Errorf("slack delivery failed: %s", execResp.Error)
	}
	return nil
}

func backoffForAttempt(attempt int) time.Duration {
	if attempt <= 0 {
		return time.Second
	}
	d := time.Second * time.Duration(1<<min(attempt, 8))
	if d > maxDispatchBackoff {
		return maxDispatchBackoff
	}
	return d
}

func SignBodyHMACSHA256(rawBody []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(rawBody)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

type cloudEvent struct {
	SpecVersion     string         `json:"specversion"`
	ID              string         `json:"id"`
	Type            string         `json:"type"`
	Source          string         `json:"source"`
	Time            string         `json:"time"`
	DataContentType string         `json:"datacontenttype"`
	Data            map[string]any `json:"data"`
}

func BuildApprovalRequestedCloudEvent(n NotificationOutbox, source, summary string) ([]byte, error) {
	ev := cloudEvent{
		SpecVersion:     "1.0",
		ID:              n.ID,
		Type:            "oc.approval.requested",
		Source:          source,
		Time:            time.Now().UTC().Format(time.RFC3339Nano),
		DataContentType: "application/json",
		Data: map[string]any{
			"approval_request_id": n.ApprovalRequestID,
			"event_id":            n.EventID,
			"tenant_id":           n.TenantID,
			"tool":                n.Tool,
			"action":              n.Action,
			"resource":            n.Resource,
			"risk_score":          n.RiskScore,
			"risk_factors":        n.RiskFactors,
			"approval_url":        n.ApprovalURL,
			"created_at":          n.CreatedAt.Format(time.RFC3339),
			"trace_id":            n.TraceID,
			"approver_group":      n.ApproverGroup,
			"summary":             summary,
			"raw": map[string]any{
				"reason": n.Reason,
			},
		},
	}
	return json.Marshal(ev)
}

func ParseSecretRefMap(raw string) map[string]string {
	out := map[string]string{}
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])
		if k != "" && v != "" {
			out[k] = v
		}
	}
	return out
}
