package main

import (
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func Test_setupTenantConfig_PersistsOrgName(t *testing.T) {
	cfg := setupTenantConfig("  Acme Co  ")
	var m map[string]string
	if err := json.Unmarshal(cfg, &m); err != nil {
		t.Fatalf("unmarshal cfg: %v", err)
	}
	if m["org_name"] != "Acme Co" {
		t.Fatalf("expected org_name %q, got %q", "Acme Co", m["org_name"])
	}
}

func Test_setupTenantConfig_EmptyOrgName(t *testing.T) {
	cfg := setupTenantConfig("")
	var m map[string]string
	if err := json.Unmarshal(cfg, &m); err != nil {
		t.Fatalf("unmarshal cfg: %v", err)
	}
	if _, ok := m["org_name"]; ok {
		t.Fatalf("expected no org_name key for empty input")
	}
}

func Test_normalizeSetupFirstTenantName_RejectsBlankAfterTrim(t *testing.T) {
	if _, err := normalizeSetupFirstTenantName("   "); err == nil {
		t.Fatal("expected error for blank tenant name")
	}
}

func Test_inviteAcceptPageURL_UsesConsoleRoute(t *testing.T) {
	got := inviteAcceptPageURL("tok en")
	if got != "/invite/accept?token=tok+en" {
		t.Fatalf("unexpected invite accept url: %q", got)
	}
}

func Test_passwordResetPageURL_UsesConsoleRoute(t *testing.T) {
	got := passwordResetPageURL("tok en")
	if got != "/reset?token=tok+en" {
		t.Fatalf("unexpected reset url: %q", got)
	}
}

func Test_normalizeRequiredName_TrimsValue(t *testing.T) {
	got, err := normalizeRequiredName("  Demo Tenant  ", "name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Demo Tenant" {
		t.Fatalf("unexpected normalized name: %q", got)
	}
}

func Test_normalizeRequiredName_RejectsBlankAfterTrim(t *testing.T) {
	if _, err := normalizeRequiredName("   ", "name"); err == nil {
		t.Fatal("expected error for blank name")
	}
}

func Test_isForeignKeyViolation(t *testing.T) {
	if !isForeignKeyViolation(&pgconn.PgError{Code: "23503"}) {
		t.Fatal("expected foreign key violation to be detected")
	}
	if isForeignKeyViolation(nil) {
		t.Fatal("did not expect nil to be a foreign key violation")
	}
}
