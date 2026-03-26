package builtins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/bturcanu/OpenClause/pkg/connectors"
)

// WebhookConnector provides an outbound HTTP POST webhook action.
// In mock mode it returns a deterministic success response.
// In real mode it validates the target URL against SSRF before posting.
type WebhookConnector struct {
	Mock   bool
	Client *http.Client
}

func (c *WebhookConnector) Name() string { return "webhook" }

func (c *WebhookConnector) Actions() []string {
	return []string{"post"}
}

func (c *WebhookConnector) Exec(ctx context.Context, req connectors.ExecRequest) connectors.ExecResponse {
	switch req.Action {
	case "post":
		return c.post(ctx, req)
	default:
		return connectors.ExecResponse{Status: "error", Error: "unsupported action: " + req.Action}
	}
}

func (c *WebhookConnector) post(ctx context.Context, req connectors.ExecRequest) connectors.ExecResponse {
	var p struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
		Body    json.RawMessage   `json:"body"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return connectors.ExecResponse{Status: "error", Error: "invalid params: " + err.Error()}
	}
	if p.URL == "" {
		return connectors.ExecResponse{Status: "error", Error: "url is required"}
	}

	if c.Mock {
		out, _ := json.Marshal(map[string]any{
			"url":         p.URL,
			"status_code": 200,
			"mock":        true,
		})
		return connectors.ExecResponse{Status: "success", OutputJSON: out}
	}

	if err := validateWebhookURL(p.URL); err != nil {
		return connectors.ExecResponse{Status: "error", Error: "url validation: " + err.Error()}
	}

	body := bytes.NewReader(p.Body)
	if len(p.Body) == 0 {
		body = bytes.NewReader(nil)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.URL, body)
	if err != nil {
		return connectors.ExecResponse{Status: "error", Error: "build request: " + err.Error()}
	}
	if len(p.Body) > 0 {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	for k, v := range p.Headers {
		httpReq.Header.Set(k, v)
	}

	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second, Transport: safeTransport()}
	}
	client = cloneWebhookClientWithRedirectValidation(client, validateWebhookURL)
	resp, err := client.Do(httpReq)
	if err != nil {
		return connectors.ExecResponse{Status: "error", Error: "webhook request: " + err.Error()}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return connectors.ExecResponse{Status: "error", Error: "read response: " + err.Error()}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return connectors.ExecResponse{Status: "error", Error: fmt.Sprintf("webhook status_code=%d: %s", resp.StatusCode, string(respBody))}
	}

	out, _ := json.Marshal(map[string]any{
		"url":           p.URL,
		"status_code":   resp.StatusCode,
		"response_body": string(respBody),
		"mock":          false,
	})
	return connectors.ExecResponse{Status: "success", OutputJSON: out}
}

var resolveIPs = net.LookupIP
var lookupIPAddrs = func(ctx context.Context, host string) ([]net.IP, error) {
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

var dialResolvedAddress = func(ctx context.Context, network, address string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return dialer.DialContext(ctx, network, address)
}

func safeTransport() *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("invalid address: %w", err)
			}
			ips, err := lookupIPAddrs(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("dns resolve: %w", err)
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("dns resolve: no IPs for %q", host)
			}
			allowed := make([]net.IP, 0, len(ips))
			for _, ip := range ips {
				if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
					return nil, fmt.Errorf("resolved IP %s is private/loopback blocked", ip)
				}
				allowed = append(allowed, ip)
			}
			var lastErr error
			for _, ip := range allowed {
				conn, err := dialResolvedAddress(ctx, network, net.JoinHostPort(ip.String(), port))
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

func cloneWebhookClientWithRedirectValidation(base *http.Client, validate func(string) error) *http.Client {
	if base == nil {
		base = &http.Client{Timeout: 10 * time.Second, Transport: safeTransport()}
	}
	clone := *base
	previous := base.CheckRedirect
	clone.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := validate(req.URL.String()); err != nil {
			return fmt.Errorf("redirect target validation: %w", err)
		}
		if previous != nil {
			return previous(req, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("stopped after %d redirects", len(via))
		}
		return nil
	}
	return &clone
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
	ips, err := resolveIPs(host)
	if err != nil {
		return fmt.Errorf("resolve host %q: %w", host, err)
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("private/loopback IP not allowed: %s", ip)
		}
	}
	return nil
}
