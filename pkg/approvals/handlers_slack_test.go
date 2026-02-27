package approvals

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

type fakeHandlersStore struct {
	granted bool
}

func (f *fakeHandlersStore) CreateRequest(context.Context, CreateApprovalInput) (*ApprovalRequest, error) {
	return &ApprovalRequest{}, nil
}

func (f *fakeHandlersStore) GetRequest(context.Context, string) (*ApprovalRequest, error) {
	return &ApprovalRequest{TenantID: "tenant1", EventID: "evt-1"}, nil
}

func (f *fakeHandlersStore) GrantRequest(_ context.Context, _ string, _ GrantInput) (*ApprovalGrant, error) {
	f.granted = true
	return &ApprovalGrant{ID: "g1"}, nil
}

func (f *fakeHandlersStore) DenyRequest(context.Context, string, DenyInput) error {
	return nil
}

func (f *fakeHandlersStore) ListPending(context.Context, string, int, int) ([]ApprovalRequest, error) {
	return nil, nil
}

func TestVerifySlackRequestFixture(t *testing.T) {
	secret := "test-secret"
	body := []byte("payload=%7B%22type%22%3A%22block_actions%22%7D")
	ts := "1700000000"
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("v0:" + ts + ":" + string(body)))
	sig := "v0=" + hex.EncodeToString(mac.Sum(nil))

	ok := VerifySlackRequest(body, sig, ts, secret, time.Unix(1700000000, 0))
	if !ok {
		t.Fatalf("expected signature verification to pass")
	}
}

func TestSlackInteractionApproveCreatesGrant(t *testing.T) {
	store := &fakeHandlersStore{}
	authz := NewApproverAuthorizer("", "tenant1:u123")
	h := NewHandlers(store, authz, "slack-secret")

	payload := `{"type":"block_actions","user":{"id":"U123","username":"alice"},"actions":[{"value":"approve|req-1|evt-1|tenant1"}]}`
	form := url.Values{}
	form.Set("payload", payload)
	body := []byte(form.Encode())
	ts := fmt.Sprintf("%d", time.Now().Unix())
	mac := hmac.New(sha256.New, []byte("slack-secret"))
	_, _ = mac.Write([]byte("v0:" + ts + ":" + string(body)))
	sig := "v0=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/v1/integrations/slack/interactions", bytes.NewReader(body))
	req.Header.Set("X-Slack-Request-Timestamp", ts)
	req.Header.Set("X-Slack-Signature", sig)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.SlackInteractions(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	if !store.granted {
		t.Fatalf("expected grant to be created")
	}
}
