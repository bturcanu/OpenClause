package approvals

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestValidateWebhookURLRejectsLocalhostAndPrivateIPs(t *testing.T) {
	cases := []string{
		"https://localhost/webhook",
		"https://127.0.0.1/webhook",
		"https://[::1]/webhook",
	}
	for _, raw := range cases {
		if err := ValidateWebhookURL(raw); err == nil {
			t.Fatalf("expected %q to be rejected", raw)
		}
	}
}

func TestValidateWebhookURLRejectsHostnamesResolvingToPrivateIPs(t *testing.T) {
	origResolve := validateWebhookResolveIPs
	defer func() { validateWebhookResolveIPs = origResolve }()

	validateWebhookResolveIPs = func(host string) ([]net.IP, error) {
		if host != "webhook.example.test" {
			t.Fatalf("unexpected host lookup %q", host)
		}
		return []net.IP{net.ParseIP("10.0.0.5")}, nil
	}

	if err := ValidateWebhookURL("https://webhook.example.test/hook"); err == nil {
		t.Fatal("expected hostname resolving to private IP to be rejected")
	}
}

func TestDispatcherFailsClosedOnRedirectToInsecureWebhookTarget(t *testing.T) {
	origResolve := validateWebhookResolveIPs
	defer func() { validateWebhookResolveIPs = origResolve }()
	validateWebhookResolveIPs = func(host string) ([]net.IP, error) {
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

	store := &fakeNotificationStore{
		items: []NotificationOutbox{{
			ID:                "d-webhook-redirect",
			ApprovalRequestID: "r1",
			TenantID:          "tenant1",
			EventID:           "e1",
			Tool:              "slack",
			Action:            "msg.post",
			Resource:          "channel/general",
			ApprovalURL:       "http://localhost/x",
			NotifyKind:        "webhook",
			NotifyURL:         "https://webhook.example.test/hook",
			SecretRef:         "tenant-secret",
			CreatedAt:         time.Now().UTC(),
		}},
		sent:    map[string]bool{},
		failed:  map[string]bool{},
		retries: map[string]int{},
		lastErr: map[string]string{},
	}
	d := NewDispatcher(store, "oc://approvals", map[string]string{"tenant-secret": "super-secret"}, "http://localhost:8082", "token")
	d.httpClient = &http.Client{Timeout: 10 * time.Second, Transport: transport}

	if err := d.DispatchOnce(context.Background()); err != nil {
		t.Fatalf("DispatchOnce: %v", err)
	}
	if !store.failed["d-webhook-redirect"] {
		t.Fatalf("expected redirect validation failure to mark notification failed")
	}
	if _, ok := store.retries["d-webhook-redirect"]; ok {
		t.Fatalf("expected redirect validation failure not to be retried")
	}
	if got := store.lastErr["d-webhook-redirect"]; !strings.Contains(got, "redirect target validation") || !strings.Contains(got, "only https scheme allowed") {
		t.Fatalf("unexpected failure reason %q", got)
	}
}
