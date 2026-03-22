package main

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestNewInviteEmailSenderFromEnv_DefaultsToLoggedSender(t *testing.T) {
	t.Setenv("SMTP_HOST", "")
	t.Setenv("SMTP_PORT", "")
	t.Setenv("SMTP_USER", "")
	t.Setenv("SMTP_PASS", "")
	t.Setenv("SMTP_FROM", "")

	sender := newInviteEmailSenderFromEnv(slog.New(slog.NewTextHandler(io.Discard, nil)))
	result, err := sender.SendInvite(context.Background(), InviteEmailMessage{
		To:         "invitee@example.com",
		TenantName: "Demo Org",
		Role:       "viewer",
		AcceptURL:  "https://console.example.com/invite/accept?token=abc",
		ExpiresAt:  time.Unix(100, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("expected logged sender to succeed, got %v", err)
	}
	if result.Status != InviteEmailStatusLogged {
		t.Fatalf("expected logged status, got %q", result.Status)
	}
}

func TestNewInviteEmailSenderFromEnv_MisconfiguredSenderReturnsSafeFailure(t *testing.T) {
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_USER", "")
	t.Setenv("SMTP_PASS", "")
	t.Setenv("SMTP_FROM", "")

	sender := newInviteEmailSenderFromEnv(slog.New(slog.NewTextHandler(io.Discard, nil)))
	result, err := sender.SendInvite(context.Background(), InviteEmailMessage{
		To:         "invitee@example.com",
		TenantName: "Demo Org",
		Role:       "viewer",
		AcceptURL:  "https://console.example.com/invite/accept?token=abc",
		ExpiresAt:  time.Unix(100, 0).UTC(),
	})
	if err == nil {
		t.Fatal("expected misconfigured sender to return an error")
	}
	if result.Status != InviteEmailStatusFailed {
		t.Fatalf("expected failed status, got %q", result.Status)
	}
	if result.ErrorMessage != "SMTP invite delivery is misconfigured" {
		t.Fatalf("unexpected safe error message: %q", result.ErrorMessage)
	}
}
