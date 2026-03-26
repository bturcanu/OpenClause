package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bturcanu/OpenClause/pkg/console"
)

type fakeInviteStore struct {
	tenant            *console.Tenant
	getTenantErr      error
	createdToken      string
	createdEmail      string
	createdTenantID   string
	createdRole       string
	createdName       string
	createdExpiresAt  time.Time
	createErr         error
	updatedToken      string
	updatedStatus     string
	updatedSentAt     *time.Time
	updatedEmailError string
	updateStatusErr   error
	listInvites       []console.Invite
	listErr           error
	consumedInvite    *console.InviteAcceptResult
	consumeErr        error
	consumeCalled     bool
}

func (f *fakeInviteStore) GetTenant(_ context.Context, _ string) (*console.Tenant, error) {
	return f.tenant, f.getTenantErr
}

func (f *fakeInviteStore) CreateInvite(_ context.Context, token, email, tenantID, role, name string, expiresAt time.Time) error {
	f.createdToken = token
	f.createdEmail = email
	f.createdTenantID = tenantID
	f.createdRole = role
	f.createdName = name
	f.createdExpiresAt = expiresAt
	return f.createErr
}

func (f *fakeInviteStore) UpdateInviteEmailStatus(_ context.Context, token, status string, sentAt *time.Time, emailError string) error {
	f.updatedToken = token
	f.updatedStatus = status
	f.updatedSentAt = sentAt
	f.updatedEmailError = emailError
	return f.updateStatusErr
}

func (f *fakeInviteStore) ListInvites(_ context.Context, _ *string, _, _ int) ([]console.Invite, error) {
	return f.listInvites, f.listErr
}

func (f *fakeInviteStore) ConsumeInviteAccept(_ context.Context, _, _, _ string) (*console.InviteAcceptResult, error) {
	f.consumeCalled = true
	return f.consumedInvite, f.consumeErr
}

type fakeInviteEmailSender struct {
	lastMessage InviteEmailMessage
	result      InviteEmailSendResult
	err         error
}

func (f *fakeInviteEmailSender) SendInvite(_ context.Context, msg InviteEmailMessage) (InviteEmailSendResult, error) {
	f.lastMessage = msg
	return f.result, f.err
}

func newTestInviteAPI(store inviteStore, sender InviteEmailSender) *ConsoleAPI {
	return &ConsoleAPI{
		log:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		inviteStore:       store,
		inviteEmailSender: sender,
		publicBaseURL:     "https://console.example.com",
	}
}

func TestHandleCreateInviteSendsEmailWithAbsoluteLink(t *testing.T) {
	store := &fakeInviteStore{
		tenant: &console.Tenant{ID: "tenant-1", Name: "Demo Org"},
	}
	sender := &fakeInviteEmailSender{
		result: InviteEmailSendResult{Status: InviteEmailStatusSent},
	}
	api := newTestInviteAPI(store, sender)

	body := []byte(`{"email":"invitee@example.com","tenant_id":"tenant-1","role":"approver","name":"Taylor"}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/invites", bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), claimsKey{}, &console.JWTClaims{
		Sub:   "admin-1",
		Roles: []string{"platform_admin"},
	})
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	api.handleCreateInvite(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rr.Code, rr.Body.String())
	}
	if store.createdEmail != "invitee@example.com" || store.createdTenantID != "tenant-1" || store.createdRole != "approver" {
		t.Fatalf("unexpected create invite args: email=%q tenant=%q role=%q", store.createdEmail, store.createdTenantID, store.createdRole)
	}
	if store.createdToken == "" {
		t.Fatal("expected raw invite token to be created")
	}
	if sender.lastMessage.To != "invitee@example.com" {
		t.Fatalf("expected sender recipient invitee@example.com, got %q", sender.lastMessage.To)
	}
	if sender.lastMessage.AcceptURL == "" || len(sender.lastMessage.AcceptURL) < len("https://console.example.com/invite/") || sender.lastMessage.AcceptURL[:len("https://console.example.com/invite/")] != "https://console.example.com/invite/" {
		t.Fatalf("expected absolute accept url, got %q", sender.lastMessage.AcceptURL)
	}
	if store.updatedStatus != InviteEmailStatusSent || store.updatedToken != store.createdToken {
		t.Fatalf("unexpected persisted email status: token=%q status=%q", store.updatedToken, store.updatedStatus)
	}
	if store.updatedSentAt == nil {
		t.Fatal("expected sent_at to be recorded")
	}

	var payload struct {
		Token       string `json:"token"`
		AcceptURL   string `json:"accept_url"`
		EmailStatus string `json:"email_status"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rr.Body.String())
	}
	if payload.Token != store.createdToken {
		t.Fatalf("expected response token %q, got %q", store.createdToken, payload.Token)
	}
	if payload.EmailStatus != InviteEmailStatusSent {
		t.Fatalf("expected email_status sent, got %q", payload.EmailStatus)
	}
	if payload.AcceptURL != sender.lastMessage.AcceptURL {
		t.Fatalf("expected accept_url %q, got %q", sender.lastMessage.AcceptURL, payload.AcceptURL)
	}
}

func TestHandleCreateInviteReturnsFailureStatusWhenEmailSendFails(t *testing.T) {
	store := &fakeInviteStore{
		tenant: &console.Tenant{ID: "tenant-1", Name: "Demo Org"},
	}
	sender := &fakeInviteEmailSender{
		result: InviteEmailSendResult{
			Status:       InviteEmailStatusFailed,
			ErrorMessage: "SMTP delivery failed",
		},
		err: errors.New("dial smtp: connection refused"),
	}
	api := newTestInviteAPI(store, sender)

	body := []byte(`{"email":"invitee@example.com","tenant_id":"tenant-1","role":"viewer"}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/invites", bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), claimsKey{}, &console.JWTClaims{
		Sub:   "admin-1",
		Roles: []string{"platform_admin"},
	})
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	api.handleCreateInvite(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rr.Code, rr.Body.String())
	}
	if store.createdToken == "" {
		t.Fatal("expected invite row to be created even on email failure")
	}
	if store.updatedStatus != InviteEmailStatusFailed {
		t.Fatalf("expected failed email status to be recorded, got %q", store.updatedStatus)
	}
	if store.updatedSentAt != nil {
		t.Fatalf("expected no sent_at on failed delivery, got %v", store.updatedSentAt)
	}
	if store.updatedEmailError != "SMTP delivery failed" {
		t.Fatalf("expected stored email error, got %q", store.updatedEmailError)
	}

	var payload struct {
		Token       string `json:"token"`
		AcceptURL   string `json:"accept_url"`
		EmailStatus string `json:"email_status"`
		EmailError  string `json:"email_error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rr.Body.String())
	}
	if payload.Token == "" || payload.AcceptURL == "" {
		t.Fatalf("expected recovery fields in response, got %+v", payload)
	}
	if payload.EmailStatus != InviteEmailStatusFailed || payload.EmailError != "SMTP delivery failed" {
		t.Fatalf("unexpected response payload: %+v", payload)
	}
}

func TestHandleListInvitesOmitsRawTokenAndReturnsEmailStatus(t *testing.T) {
	sentAt := time.Unix(100, 0).UTC()
	store := &fakeInviteStore{
		listInvites: []console.Invite{{
			Email:       "invitee@example.com",
			TenantID:    "tenant-1",
			Role:        "viewer",
			CreatedAt:   time.Unix(10, 0).UTC(),
			ExpiresAt:   time.Unix(20, 0).UTC(),
			EmailStatus: InviteEmailStatusLogged,
			EmailSentAt: &sentAt,
		}},
	}
	api := newTestInviteAPI(store, &fakeInviteEmailSender{})
	req := httptest.NewRequest(http.MethodGet, "/admin/invites", nil)
	ctx := context.WithValue(req.Context(), claimsKey{}, &console.JWTClaims{
		Sub:   "admin-1",
		Roles: []string{"platform_admin"},
	})
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	api.handleListInvites(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Invites []map[string]any `json:"invites"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rr.Body.String())
	}
	if len(payload.Invites) != 1 {
		t.Fatalf("expected 1 invite, got %+v", payload.Invites)
	}
	if _, ok := payload.Invites[0]["token"]; ok {
		t.Fatalf("expected raw token to be omitted from list response, got %+v", payload.Invites[0]["token"])
	}
	if payload.Invites[0]["email_status"] != InviteEmailStatusLogged {
		t.Fatalf("expected email_status logged, got %+v", payload.Invites[0]["email_status"])
	}
}

func TestHandleInviteAcceptRejectsInvalidToken(t *testing.T) {
	store := &fakeInviteStore{consumeErr: console.ErrInviteTokenInvalid}
	api := newTestInviteAPI(store, &fakeInviteEmailSender{})

	body := []byte(`{"token":"bad-token","password":"Admin123!"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/invite/accept", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	api.handleInviteAccept(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleInviteAcceptRejectsWhitespaceOnlyPassword(t *testing.T) {
	store := &fakeInviteStore{}
	api := newTestInviteAPI(store, &fakeInviteEmailSender{})

	body := []byte(`{"token":"tok-123","password":"   "}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/invite/accept", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	api.handleInviteAccept(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if store.consumeCalled {
		t.Fatal("expected store to not be called for whitespace-only password")
	}
}

func TestHandleInviteAcceptReturnsServerErrorOnStoreFailure(t *testing.T) {
	store := &fakeInviteStore{consumeErr: errors.New("db unavailable")}
	api := newTestInviteAPI(store, &fakeInviteEmailSender{})

	body := []byte(`{"token":"tok-123","password":"Admin123!"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/invite/accept", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	api.handleInviteAccept(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestResetConfirmErrorStatus(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "invalid token",
			err:        console.ErrResetTokenInvalid,
			wantStatus: http.StatusBadRequest,
			wantMsg:    "invalid or expired token",
		},
		{
			name:       "missing user",
			err:        console.ErrResetUserNotFound,
			wantStatus: http.StatusBadRequest,
			wantMsg:    "invalid or expired token",
		},
		{
			name:       "internal failure",
			err:        errors.New("db down"),
			wantStatus: http.StatusInternalServerError,
			wantMsg:    "failed to confirm reset",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStatus, gotMsg := resetConfirmErrorStatus(tt.err)
			if gotStatus != tt.wantStatus || gotMsg != tt.wantMsg {
				t.Fatalf("expected (%d, %q), got (%d, %q)", tt.wantStatus, tt.wantMsg, gotStatus, gotMsg)
			}
		})
	}
}
