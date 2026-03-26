package onboarding

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBundleResponseOmitsAPIKeyWhenNil(t *testing.T) {
	resp := BundleResponse{
		Mode:   "preview",
		Tenant: BundleTenant{ID: "tenant-1", Name: "Alpha Corp"},
		Agent:  BundleAgent{ID: "preview-agent", Name: "Preview Bot", Status: "preview", CreatedAt: "2026-03-24T10:00:00Z", Preview: true},
		Bundle: &Bundle{Title: "Preview bundle"},
	}

	encoded, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if strings.Contains(string(encoded), `"api_key"`) {
		t.Fatalf("expected api_key to be omitted when nil, got %s", string(encoded))
	}
	if strings.Contains(string(encoded), `"integration"`) {
		t.Fatalf("expected integration to be omitted when nil, got %s", string(encoded))
	}
}

func TestBundleAPIKeyOmitsRawKeyWhenEmptyAndIncludesItWhenPresent(t *testing.T) {
	resp := BundleResponse{
		Mode:   "regenerated",
		Tenant: BundleTenant{ID: "tenant-1", Name: "Alpha Corp"},
		Agent:  BundleAgent{ID: "agent-1", Name: "Support Bot", Status: "active", CreatedAt: "2026-03-24T10:00:00Z", Preview: false},
		APIKey: &BundleAPIKey{ID: "key-1", Name: "Existing key", KeyPrefix: "sk-oc-ref"},
		Bundle: &Bundle{Title: "Regenerated bundle"},
	}

	encoded, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal regenerate: %v", err)
	}
	if strings.Contains(string(encoded), `"raw_key"`) {
		t.Fatalf("expected raw_key to be omitted when empty, got %s", string(encoded))
	}
	if !strings.Contains(string(encoded), `"key_prefix":"sk-oc-ref"`) {
		t.Fatalf("expected key prefix to remain present, got %s", string(encoded))
	}

	resp.APIKey.RawKey = "sk-oc-demo-raw"
	encoded, err = json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal create-like response: %v", err)
	}
	if !strings.Contains(string(encoded), `"raw_key":"sk-oc-demo-raw"`) {
		t.Fatalf("expected raw_key to be present when provided, got %s", string(encoded))
	}
}

func TestBundleResponseIncludesIntegrationWhenPresent(t *testing.T) {
	resp := BundleResponse{
		Mode:   "created",
		Tenant: BundleTenant{ID: "tenant-1", Name: "Alpha Corp"},
		Agent:  BundleAgent{ID: "agent-1", Name: "Support Bot", Status: "active", CreatedAt: "2026-03-24T10:00:00Z", Preview: false},
		Integration: &BundleIntegration{
			ID:               "integration-1",
			TenantID:         "tenant-1",
			AgentID:          "agent-1",
			Runtime:          "python",
			EnvironmentLabel: "staging",
			OwnerName:        "Ops",
			ApprovalPosture:  "pilot_safe",
			Tools:            []SelectedTool{{Tool: "postgres", Action: "query.readonly"}},
			CreatedAt:        "2026-03-24T10:00:00Z",
			UpdatedAt:        "2026-03-24T10:00:00Z",
		},
		Bundle: &Bundle{Title: "Created bundle"},
	}

	encoded, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	text := string(encoded)
	if !strings.Contains(text, `"integration"`) || !strings.Contains(text, `"runtime":"python"`) || !strings.Contains(text, `"approval_posture":"pilot_safe"`) {
		t.Fatalf("expected integration fields to be encoded, got %s", text)
	}
}

func TestBundleIntegrationRevisionAlwaysSerializesMode(t *testing.T) {
	revision := BundleIntegrationRevision{
		ID:            "revision-1",
		IntegrationID: "integration-1",
		TenantID:      "tenant-1",
		AgentID:       "agent-1",
		Mode:          "regenerated",
		Runtime:       "python",
		CreatedAt:     "2026-03-26T00:00:00Z",
	}

	encoded, err := json.Marshal(revision)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"mode":"regenerated"`) {
		t.Fatalf("expected integration revision mode to be serialized, got %s", string(encoded))
	}
}
