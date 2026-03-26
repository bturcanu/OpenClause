package builtins

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bturcanu/OpenClause/pkg/connectors"
)

func TestValidateWebhookURLRejectsLocalhostAndPrivateIPs(t *testing.T) {
	cases := []string{
		"http://example.com/webhook",
		"not-a-url",
		"https://localhost/webhook",
		"https://127.0.0.1/webhook",
		"https://[::1]/webhook",
	}
	for _, raw := range cases {
		if err := validateWebhookURL(raw); err == nil {
			t.Fatalf("expected %q to be rejected", raw)
		}
	}
}

func TestValidateWebhookURLRejectsHostnamesResolvingToPrivateIPs(t *testing.T) {
	origResolveIPs := resolveIPs
	defer func() { resolveIPs = origResolveIPs }()

	resolveIPs = func(host string) ([]net.IP, error) {
		if host != "webhook.example.test" {
			t.Fatalf("unexpected host lookup %q", host)
		}
		return []net.IP{net.ParseIP("10.0.0.5")}, nil
	}

	if err := validateWebhookURL("https://webhook.example.test/hook"); err == nil {
		t.Fatal("expected hostname resolving to private IP to be rejected")
	}
}

func TestWebhookConnectorMockModeReturnsDeterministicSuccess(t *testing.T) {
	connector := &WebhookConnector{Mock: true}
	params, err := json.Marshal(map[string]any{
		"url": "https://example.com/webhook",
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	resp := connector.Exec(context.Background(), connectors.ExecRequest{
		Action: "post",
		Params: params,
	})
	if resp.Status != "success" {
		t.Fatalf("expected success, got %#v", resp)
	}

	var out map[string]any
	if err := json.Unmarshal(resp.OutputJSON, &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out["mock"] != true {
		t.Fatalf("expected mock=true, got %v", out["mock"])
	}
}

func TestWebhookConnectorRejectsInvalidParamsAndUnsupportedAction(t *testing.T) {
	connector := &WebhookConnector{}

	resp := connector.Exec(context.Background(), connectors.ExecRequest{
		Action: "post",
		Params: json.RawMessage(`{`),
	})
	if resp.Status != "error" || !strings.Contains(resp.Error, "invalid params") {
		t.Fatalf("expected invalid params error, got %#v", resp)
	}

	resp = connector.Exec(context.Background(), connectors.ExecRequest{
		Action: "unsupported",
	})
	if resp.Status != "error" || !strings.Contains(resp.Error, "unsupported action") {
		t.Fatalf("expected unsupported action error, got %#v", resp)
	}
}

func TestSafeTransportDialsVerifiedResolvedIP(t *testing.T) {
	origLookupIPAddrs := lookupIPAddrs
	origDialResolvedAddress := dialResolvedAddress
	defer func() {
		lookupIPAddrs = origLookupIPAddrs
		dialResolvedAddress = origDialResolvedAddress
	}()

	lookupIPAddrs = func(ctx context.Context, host string) ([]net.IP, error) {
		if host != "webhook.example.test" {
			t.Fatalf("unexpected host lookup %q", host)
		}
		return []net.IP{net.ParseIP("8.8.8.8")}, nil
	}

	var gotAddress string
	dialResolvedAddress = func(ctx context.Context, network, address string) (net.Conn, error) {
		gotAddress = address
		return &nopConn{}, nil
	}

	tr := safeTransport()
	conn, err := tr.DialContext(context.Background(), "tcp", "webhook.example.test:443")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn == nil {
		t.Fatal("expected a connection")
	}
	_ = conn.Close()
	if gotAddress != "8.8.8.8:443" {
		t.Fatalf("expected dial to resolved IP, got %q", gotAddress)
	}
}

func TestSafeTransportTriesMultipleResolvedIPsUntilOneConnects(t *testing.T) {
	origLookupIPAddrs := lookupIPAddrs
	origDialResolvedAddress := dialResolvedAddress
	defer func() {
		lookupIPAddrs = origLookupIPAddrs
		dialResolvedAddress = origDialResolvedAddress
	}()

	lookupIPAddrs = func(ctx context.Context, host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("1.1.1.1"), net.ParseIP("8.8.8.8")}, nil
	}

	var attempts []string
	dialResolvedAddress = func(ctx context.Context, network, address string) (net.Conn, error) {
		attempts = append(attempts, address)
		if len(attempts) == 1 {
			return nil, context.DeadlineExceeded
		}
		return &nopConn{}, nil
	}

	tr := safeTransport()
	conn, err := tr.DialContext(context.Background(), "tcp", "webhook.example.test:443")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = conn.Close()
	if got := strings.Join(attempts, ","); got != "1.1.1.1:443,8.8.8.8:443" {
		t.Fatalf("unexpected dial attempts %q", got)
	}
}

type nopConn struct{}

func (c *nopConn) Read(_ []byte) (int, error)       { return 0, io.EOF }
func (c *nopConn) Write(b []byte) (int, error)      { return len(b), nil }
func (c *nopConn) Close() error                     { return nil }
func (c *nopConn) LocalAddr() net.Addr              { return &net.IPAddr{} }
func (c *nopConn) RemoteAddr() net.Addr             { return &net.IPAddr{} }
func (c *nopConn) SetDeadline(time.Time) error      { return nil }
func (c *nopConn) SetReadDeadline(time.Time) error  { return nil }
func (c *nopConn) SetWriteDeadline(time.Time) error { return nil }

func TestWebhookConnectorRealPostForwardsRequestAndResponse(t *testing.T) {
	origResolveIPs := resolveIPs
	defer func() { resolveIPs = origResolveIPs }()
	resolveIPs = func(host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("8.8.8.8")}, nil
	}

	var gotMethod string
	var gotPath string
	var gotContentType string
	var gotHeader string
	var gotBody string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		gotHeader = r.Header.Get("X-Test")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("X-Reply", "ok")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"delivered":true}`))
	}))
	defer srv.Close()

	transport := srv.Client().Transport.(*http.Transport).Clone()
	transport.TLSClientConfig.InsecureSkipVerify = true
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialer := &net.Dialer{}
		return dialer.DialContext(ctx, network, srv.Listener.Addr().String())
	}
	client := &http.Client{Transport: transport}

	connector := &WebhookConnector{Client: client}
	params, err := json.Marshal(map[string]any{
		"url":     "https://webhook.example.test/hook",
		"headers": map[string]string{"X-Test": "yes"},
		"body":    json.RawMessage(`{"hello":"world"}`),
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	resp := connector.Exec(context.Background(), connectors.ExecRequest{
		Action: "post",
		Params: params,
	})
	if resp.Status != "success" {
		t.Fatalf("expected success, got %#v", resp)
	}

	var out map[string]any
	if err := json.Unmarshal(resp.OutputJSON, &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out["mock"] != false {
		t.Fatalf("expected mock=false, got %v", out["mock"])
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("expected POST, got %s", gotMethod)
	}
	if gotPath != "/hook" {
		t.Fatalf("expected /hook, got %s", gotPath)
	}
	if gotContentType != "application/json" {
		t.Fatalf("expected application/json, got %q", gotContentType)
	}
	if gotHeader != "yes" {
		t.Fatalf("expected forwarded header, got %q", gotHeader)
	}
	if strings.TrimSpace(gotBody) != `{"hello":"world"}` {
		t.Fatalf("unexpected body %q", gotBody)
	}
	if status := out["status_code"]; status != float64(http.StatusCreated) {
		t.Fatalf("expected status_code=%d, got %v", http.StatusCreated, status)
	}
}

func TestWebhookConnectorRealPostFailsOnNon2xx(t *testing.T) {
	origResolveIPs := resolveIPs
	defer func() { resolveIPs = origResolveIPs }()
	resolveIPs = func(host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("8.8.8.8")}, nil
	}

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("bad gateway"))
	}))
	defer srv.Close()

	transport := srv.Client().Transport.(*http.Transport).Clone()
	transport.TLSClientConfig.InsecureSkipVerify = true
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialer := &net.Dialer{}
		return dialer.DialContext(ctx, network, srv.Listener.Addr().String())
	}
	client := &http.Client{Transport: transport}

	connector := &WebhookConnector{Client: client}
	params, err := json.Marshal(map[string]any{
		"url":  "https://webhook.example.test/hook",
		"body": json.RawMessage(`{"hello":"world"}`),
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	resp := connector.Exec(context.Background(), connectors.ExecRequest{
		Action: "post",
		Params: params,
	})
	if resp.Status != "error" {
		t.Fatalf("expected error, got %#v", resp)
	}
	if !strings.Contains(resp.Error, "status_code=502") {
		t.Fatalf("unexpected error %q", resp.Error)
	}
}

func TestWebhookConnectorRejectsRedirectToInsecureURL(t *testing.T) {
	origResolveIPs := resolveIPs
	defer func() { resolveIPs = origResolveIPs }()
	resolveIPs = func(host string) ([]net.IP, error) {
		if host != "webhook.example.test" {
			t.Fatalf("unexpected host lookup %q", host)
		}
		return []net.IP{net.ParseIP("8.8.8.8")}, nil
	}

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1/internal", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()

	transport := srv.Client().Transport.(*http.Transport).Clone()
	transport.TLSClientConfig.InsecureSkipVerify = true
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialer := &net.Dialer{}
		return dialer.DialContext(ctx, network, srv.Listener.Addr().String())
	}
	client := &http.Client{Transport: transport}

	connector := &WebhookConnector{Client: client}
	params, err := json.Marshal(map[string]any{
		"url":  "https://webhook.example.test/hook",
		"body": json.RawMessage(`{"hello":"world"}`),
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	resp := connector.Exec(context.Background(), connectors.ExecRequest{
		Action: "post",
		Params: params,
	})
	if resp.Status != "error" {
		t.Fatalf("expected error, got %#v", resp)
	}
	if !strings.Contains(resp.Error, "redirect target validation") || !strings.Contains(resp.Error, "only https scheme allowed") {
		t.Fatalf("expected redirect validation failure, got %#v", resp)
	}
}
