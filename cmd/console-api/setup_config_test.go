package main

import (
	"encoding/json"
	"testing"
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

