package approvals

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestBuildApprovalRequestedCloudEvent(t *testing.T) {
	n := NotificationOutbox{
		ID:                "delivery-1",
		ApprovalRequestID: "req-1",
		EventID:           "evt-1",
		TenantID:          "tenant1",
		Tool:              "jira",
		Action:            "issue.create",
		Resource:          "project/OPS",
		RiskScore:         7,
		RiskFactors:       []string{"outside_hours"},
		ApprovalURL:       "http://localhost:8081/v1/approvals/requests/req-1",
		CreatedAt:         time.Now().UTC(),
		TraceID:           "trace-1",
	}
	raw, err := BuildApprovalRequestedCloudEvent(n, "oc://approvals", "summary")
	if err != nil {
		t.Fatalf("build event: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	if body["type"] != "oc.approval.requested" {
		t.Fatalf("unexpected type: %v", body["type"])
	}
}

func TestSignBodyHMACSHA256(t *testing.T) {
	got := SignBodyHMACSHA256([]byte(`{"a":1}`), "secret")
	if got == "" || got[:7] != "sha256=" {
		t.Fatalf("unexpected signature format: %s", got)
	}
}

type fakeNotificationStore struct {
	mu      sync.Mutex
	items   []NotificationOutbox
	sent    map[string]bool
	failed  map[string]bool
	retries map[string]int
	lastErr map[string]string
}

func (f *fakeNotificationStore) ClaimDueNotifications(context.Context, int) ([]NotificationOutbox, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]NotificationOutbox, 0)
	for i := range f.items {
		if f.sent[f.items[i].ID] || f.failed[f.items[i].ID] {
			continue
		}
		f.items[i].Attempts++
		out = append(out, f.items[i])
	}
	return out, nil
}

func (f *fakeNotificationStore) MarkNotificationSent(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent[id] = true
	return nil
}

func (f *fakeNotificationStore) MarkNotificationRetry(_ context.Context, id string, attempts int, _ time.Time, lastErr string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.retries[id] = attempts
	f.lastErr[id] = lastErr
	return nil
}

func (f *fakeNotificationStore) MarkNotificationFailed(_ context.Context, id string, lastErr string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failed[id] = true
	f.lastErr[id] = lastErr
	return nil
}

func TestDispatcherRetriesThenSucceeds(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		if hits == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := &fakeNotificationStore{
		items: []NotificationOutbox{
			{
				ID:                "d1",
				ApprovalRequestID: "r1",
				TenantID:          "tenant1",
				EventID:           "e1",
				Tool:              "slack",
				Action:            "msg.post",
				Resource:          "channel/general",
				ApprovalURL:       "http://localhost/x",
				NotifyKind:        "webhook",
				NotifyURL:         srv.URL,
				SecretRef:         "s1",
				CreatedAt:         time.Now().UTC(),
			},
		},
		sent:    map[string]bool{},
		failed:  map[string]bool{},
		retries: map[string]int{},
		lastErr: map[string]string{},
	}
	d := NewDispatcher(store, "oc://approvals", map[string]string{"s1": "secret"}, "http://localhost:8082", "token")

	if err := d.DispatchOnce(context.Background()); err != nil {
		t.Fatalf("dispatch once #1: %v", err)
	}
	if store.retries["d1"] != 1 {
		t.Fatalf("expected one retry, got %d", store.retries["d1"])
	}

	if err := d.DispatchOnce(context.Background()); err != nil {
		t.Fatalf("dispatch once #2: %v", err)
	}
	if !store.sent["d1"] {
		t.Fatalf("expected sent after retry")
	}
}

func TestDispatcherDeliversSlackNotification(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/exec" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","output_json":{"ok":true}}`))
	}))
	defer srv.Close()

	store := &fakeNotificationStore{
		items: []NotificationOutbox{
			{
				ID:                "d-slack-1",
				ApprovalRequestID: "r1",
				TenantID:          "tenant1",
				EventID:           "e1",
				Tool:              "slack",
				Action:            "msg.post",
				Resource:          "channel/security-approvals",
				RiskScore:         7,
				Reason:            "high risk",
				ApprovalURL:       "http://localhost/x",
				NotifyKind:        "slack",
				SlackChannel:      "#security-approvals",
				CreatedAt:         time.Now().UTC(),
			},
		},
		sent:    map[string]bool{},
		failed:  map[string]bool{},
		retries: map[string]int{},
		lastErr: map[string]string{},
	}
	d := NewDispatcher(store, "oc://approvals", nil, srv.URL, "token")

	if err := d.DispatchOnce(context.Background()); err != nil {
		t.Fatalf("dispatch once: %v", err)
	}
	if !store.sent["d-slack-1"] {
		t.Fatalf("expected slack notification to be marked sent")
	}
	if hits != 1 {
		t.Fatalf("expected one connector delivery, got %d", hits)
	}
}
