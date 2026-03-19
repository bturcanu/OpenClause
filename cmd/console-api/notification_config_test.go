package main

import (
	"strings"
	"testing"

	"github.com/bturcanu/OpenClause/pkg/console"
	"github.com/bturcanu/OpenClause/pkg/types"
)

func Test_normalizeTenantNotificationConfig_SlackTrimsAndRequiresChannel(t *testing.T) {
	in := console.TenantNotificationConfig{
		ApproverGroup: "  tenant_admin ",
		Notify: []types.PolicyNotify{
			{Kind: "  SlAcK  ", Channel: "  #alerts  "},
		},
	}

	out, err := normalizeTenantNotificationConfig(in)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if out.ApproverGroup != "tenant_admin" {
		t.Fatalf("expected trimmed approver_group, got %q", out.ApproverGroup)
	}
	if len(out.Notify) != 1 {
		t.Fatalf("expected 1 notify entry, got %d", len(out.Notify))
	}
	if out.Notify[0].Kind != "slack" {
		t.Fatalf("expected kind slack, got %q", out.Notify[0].Kind)
	}
	if out.Notify[0].Channel != "#alerts" {
		t.Fatalf("expected trimmed slack channel, got %q", out.Notify[0].Channel)
	}
}

func Test_normalizeTenantNotificationConfig_SlackEmptyChannelRejected(t *testing.T) {
	_, err := normalizeTenantNotificationConfig(console.TenantNotificationConfig{
		Notify: []types.PolicyNotify{
			{Kind: "slack", Channel: "   "},
		},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "slack notify requires channel") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func Test_normalizeTenantNotificationConfig_WebhookSSRFAndSchemeRejected(t *testing.T) {
	_, err := normalizeTenantNotificationConfig(console.TenantNotificationConfig{
		Notify: []types.PolicyNotify{
			{Kind: "webhook", URL: "http://127.0.0.1:8080/hook", SecretRef: "s1"},
		},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "only https scheme allowed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func Test_normalizeTenantNotificationConfig_WebhookSSRFPrivateIPRejected(t *testing.T) {
	_, err := normalizeTenantNotificationConfig(console.TenantNotificationConfig{
		Notify: []types.PolicyNotify{
			{Kind: "webhook", URL: "https://127.0.0.1:8080/hook", SecretRef: "s1"},
		},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "private/loopback IP not allowed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func Test_normalizeTenantNotificationConfig_WebhookRequiresSecretRef(t *testing.T) {
	_, err := normalizeTenantNotificationConfig(console.TenantNotificationConfig{
		Notify: []types.PolicyNotify{
			{Kind: "webhook", URL: "https://example.com/hook", SecretRef: ""},
		},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "webhook notify requires secret_ref") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func Test_normalizeTenantNotificationConfig_UnsupportedKind(t *testing.T) {
	_, err := normalizeTenantNotificationConfig(console.TenantNotificationConfig{
		Notify: []types.PolicyNotify{
			{Kind: "sms", Channel: "#x"},
		},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported notify kind") {
		t.Fatalf("unexpected error: %v", err)
	}
}
