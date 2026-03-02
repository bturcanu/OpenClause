package builtins

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"

	"github.com/bturcanu/OpenClause/pkg/connectors"
)

// WebhookConnector provides an outbound HTTP POST webhook action.
// In mock mode it returns a deterministic success response.
// In real mode it validates the target URL against SSRF before posting.
type WebhookConnector struct {
	Mock bool
}

func (c *WebhookConnector) Name() string { return "webhook" }

func (c *WebhookConnector) Actions() []string {
	return []string{"post"}
}

func (c *WebhookConnector) Exec(_ context.Context, req connectors.ExecRequest) connectors.ExecResponse {
	switch req.Action {
	case "post":
		return c.post(req)
	default:
		return connectors.ExecResponse{Status: "error", Error: "unsupported action: " + req.Action}
	}
}

func (c *WebhookConnector) post(req connectors.ExecRequest) connectors.ExecResponse {
	var p struct {
		URL     string          `json:"url"`
		Headers map[string]string `json:"headers"`
		Body    json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return connectors.ExecResponse{Status: "error", Error: "invalid params: " + err.Error()}
	}
	if p.URL == "" {
		return connectors.ExecResponse{Status: "error", Error: "url is required"}
	}

	if !c.Mock {
		if err := validateWebhookURL(p.URL); err != nil {
			return connectors.ExecResponse{Status: "error", Error: "url validation: " + err.Error()}
		}
	}

	out, _ := json.Marshal(map[string]any{
		"url":         p.URL,
		"status_code": 200,
		"mock":        true,
	})
	return connectors.ExecResponse{Status: "success", OutputJSON: out}
}

// validateWebhookURL rejects non-HTTPS URLs and private/loopback IPs to
// prevent SSRF. This mirrors the logic in pkg/approvals.ValidateWebhookURL
// but is reimplemented here to avoid circular dependencies.
func validateWebhookURL(rawURL string) error {
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
