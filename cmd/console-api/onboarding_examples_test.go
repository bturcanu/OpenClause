package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bturcanu/OpenClause/pkg/onboarding"
)

func TestOnboardingResponseExamplesStayAlignedWithShippedContract(t *testing.T) {
	t.Parallel()

	cases := []struct {
		file                     string
		wantMode                 string
		wantRuntime              string
		wantIntegrationRuntime   string
		expectIntegration        bool
		expectAPIKey             bool
		expectRawKey             bool
		expectPreview            bool
		expectAppliedDefaultsMin int
		wantAPIKeyEnv            string
	}{
		{
			file:                   "created-response.json",
			wantMode:               "created",
			wantRuntime:            "python",
			wantIntegrationRuntime: "python",
			expectIntegration:      true,
			expectAPIKey:           true,
			expectRawKey:           true,
			expectPreview:          false,
			wantAPIKeyEnv:          "sk-oc-demo-raw",
		},
		{
			file:              "preview-response.json",
			wantMode:          "preview",
			wantRuntime:       "typescript",
			expectIntegration: false,
			expectAPIKey:      false,
			expectRawKey:      false,
			expectPreview:     true,
			wantAPIKeyEnv:     "${OPENCLAUSE_API_KEY:-generated-on-create}",
		},
		{
			file:                   "regenerated-response.json",
			wantMode:               "regenerated",
			wantRuntime:            "langchain",
			wantIntegrationRuntime: "langchain",
			expectIntegration:      true,
			expectAPIKey:           true,
			expectRawKey:           false,
			expectPreview:          false,
			wantAPIKeyEnv:          "${OPENCLAUSE_API_KEY:-reuse-existing-key}",
		},
		{
			file:                     "regenerated-defaults-response.json",
			wantMode:                 "regenerated_defaults",
			wantRuntime:              "python",
			wantIntegrationRuntime:   "python",
			expectIntegration:        true,
			expectAPIKey:             true,
			expectRawKey:             false,
			expectPreview:            false,
			expectAppliedDefaultsMin: 3,
			wantAPIKeyEnv:            "${OPENCLAUSE_API_KEY:-reuse-existing-key}",
		},
		{
			file:                   "fetched-response.json",
			wantMode:               "fetched",
			wantRuntime:            "typescript",
			wantIntegrationRuntime: "typescript",
			expectIntegration:      true,
			expectAPIKey:           true,
			expectRawKey:           false,
			expectPreview:          false,
			wantAPIKeyEnv:          "${OPENCLAUSE_API_KEY:-reuse-existing-key}",
		},
		{
			file:                     "fetched-defaults-response.json",
			wantMode:                 "fetched_defaults",
			wantRuntime:              "python",
			wantIntegrationRuntime:   "typescript",
			expectIntegration:        true,
			expectAPIKey:             false,
			expectRawKey:             false,
			expectPreview:            false,
			expectAppliedDefaultsMin: 3,
			wantAPIKeyEnv:            "${OPENCLAUSE_API_KEY:-reuse-existing-key}",
		},
	}

	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join("..", "..", "docs", "examples", "onboarding", tc.file)
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read example fixture: %v", err)
			}

			var raw map[string]any
			if err := json.Unmarshal(body, &raw); err != nil {
				t.Fatalf("decode raw example fixture: %v", err)
			}

			var resp onboarding.BundleResponse
			if err := json.Unmarshal(body, &resp); err != nil {
				t.Fatalf("decode example fixture into BundleResponse: %v", err)
			}

			if resp.Mode != tc.wantMode {
				t.Fatalf("expected mode %q, got %q", tc.wantMode, resp.Mode)
			}
			if resp.Bundle == nil {
				t.Fatalf("expected bundle in example fixture")
			}
			if resp.Bundle.Runtime != tc.wantRuntime {
				t.Fatalf("expected runtime %q, got %q", tc.wantRuntime, resp.Bundle.Runtime)
			}
			if resp.Agent.Preview != tc.expectPreview {
				t.Fatalf("expected preview=%v, got %+v", tc.expectPreview, resp.Agent)
			}
			_, rawHasIntegration := raw["integration"]
			if rawHasIntegration != tc.expectIntegration {
				t.Fatalf("expected integration present=%v, got raw=%+v", tc.expectIntegration, raw["integration"])
			}
			if tc.expectIntegration != (resp.Integration != nil) {
				t.Fatalf("expected decoded integration present=%v, got %+v", tc.expectIntegration, resp.Integration)
			}
			if resp.Integration != nil {
				if resp.Integration.TenantID != resp.Tenant.ID || resp.Integration.AgentID != resp.Agent.ID {
					t.Fatalf("expected integration tenant/agent ids to match envelope, got %+v", resp.Integration)
				}
				wantIntegrationRuntime := tc.wantIntegrationRuntime
				if wantIntegrationRuntime == "" {
					wantIntegrationRuntime = tc.wantRuntime
				}
				if resp.Integration.Runtime != wantIntegrationRuntime {
					t.Fatalf("expected integration runtime %q, got %+v", wantIntegrationRuntime, resp.Integration)
				}
				if _, err := time.Parse(time.RFC3339, resp.Integration.CreatedAt); err != nil {
					t.Fatalf("expected RFC3339 integration.created_at, got %q: %v", resp.Integration.CreatedAt, err)
				}
				if _, err := time.Parse(time.RFC3339, resp.Integration.UpdatedAt); err != nil {
					t.Fatalf("expected RFC3339 integration.updated_at, got %q: %v", resp.Integration.UpdatedAt, err)
				}
			}
			if _, err := time.Parse(time.RFC3339, resp.Agent.CreatedAt); err != nil {
				t.Fatalf("expected RFC3339 agent.created_at, got %q: %v", resp.Agent.CreatedAt, err)
			}
			if len(resp.Bundle.VerificationLinks) != 3 {
				t.Fatalf("expected 3 verification links, got %+v", resp.Bundle.VerificationLinks)
			}
			if tc.file == "fetched-response.json" {
				bundleRaw, ok := raw["bundle"].(map[string]any)
				if !ok {
					t.Fatalf("expected fetched bundle fixture to include bundle object, got %+v", raw["bundle"])
				}
				artifacts, ok := bundleRaw["artifacts"].([]any)
				if !ok || len(artifacts) == 0 {
					t.Fatalf("expected fetched bundle fixture to include artifacts, got %+v", raw["bundle"])
				}
				firstArtifact, ok := artifacts[0].(map[string]any)
				if !ok {
					t.Fatalf("expected fetched bundle artifact map, got %+v", artifacts[0])
				}
				if got := firstArtifact["path_hint"]; got != "agent.ts" {
					t.Fatalf("expected fetched TypeScript path_hint agent.ts, got %+v", got)
				}
			}
			if got := resp.Bundle.Environment["OPENCLAUSE_API_KEY"]; got != tc.wantAPIKeyEnv {
				t.Fatalf("expected OPENCLAUSE_API_KEY=%q, got %q", tc.wantAPIKeyEnv, got)
			}
			if !strings.Contains(resp.Bundle.SampleCall, "set -euo pipefail") {
				t.Fatalf("expected sample_call to document guarded shell usage, got %q", resp.Bundle.SampleCall)
			}
			if !strings.Contains(resp.Bundle.SampleCall, "curl -fsS \"$OPENCLAUSE_BASE_URL/v1/toolcalls\"") {
				t.Fatalf("expected sample_call to use fail-closed curl semantics, got %q", resp.Bundle.SampleCall)
			}
			if !strings.Contains(resp.Bundle.SampleCall, "idempotency_key") {
				t.Fatalf("expected sample_call to show request identity fields, got %q", resp.Bundle.SampleCall)
			}

			_, rawHasAPIKey := raw["api_key"]
			if rawHasAPIKey != tc.expectAPIKey {
				t.Fatalf("expected api_key present=%v, got raw=%+v", tc.expectAPIKey, raw["api_key"])
			}
			if tc.expectAPIKey != (resp.APIKey != nil) {
				t.Fatalf("expected decoded api_key present=%v, got %+v", tc.expectAPIKey, resp.APIKey)
			}

			rawHasRawKey := false
			if apiKey, ok := raw["api_key"].(map[string]any); ok {
				_, rawHasRawKey = apiKey["raw_key"]
			}
			if rawHasRawKey != tc.expectRawKey {
				t.Fatalf("expected raw_key present=%v, got raw fixture=%+v", tc.expectRawKey, raw["api_key"])
			}
			if tc.expectRawKey && strings.TrimSpace(resp.APIKey.RawKey) == "" {
				t.Fatalf("expected decoded raw_key to be populated, got %+v", resp.APIKey)
			}
			if !tc.expectRawKey && resp.APIKey != nil && strings.TrimSpace(resp.APIKey.RawKey) != "" {
				t.Fatalf("expected decoded raw_key to be empty, got %+v", resp.APIKey)
			}

			if len(resp.Bundle.AppliedDefaults) < tc.expectAppliedDefaultsMin {
				t.Fatalf("expected at least %d applied defaults, got %+v", tc.expectAppliedDefaultsMin, resp.Bundle.AppliedDefaults)
			}
		})
	}
}

func TestOnboardingIntegrationExamplesStayAlignedWithShippedContract(t *testing.T) {
	t.Parallel()

	recordPath := filepath.Join("..", "..", "docs", "examples", "onboarding", "integration-record.json")
	recordBody, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("read integration example fixture: %v", err)
	}

	var record onboarding.BundleIntegration
	if err := json.Unmarshal(recordBody, &record); err != nil {
		t.Fatalf("decode integration example fixture: %v", err)
	}
	if record.ID == "" || record.TenantID == "" || record.AgentID == "" || record.Runtime == "" {
		t.Fatalf("expected integration example to populate id, tenant_id, agent_id, and runtime, got %+v", record)
	}
	if len(record.Tools) == 0 {
		t.Fatalf("expected integration example to carry persisted tools, got %+v", record)
	}
	if _, err := time.Parse(time.RFC3339, record.CreatedAt); err != nil {
		t.Fatalf("expected RFC3339 integration.created_at, got %q: %v", record.CreatedAt, err)
	}
	if _, err := time.Parse(time.RFC3339, record.UpdatedAt); err != nil {
		t.Fatalf("expected RFC3339 integration.updated_at, got %q: %v", record.UpdatedAt, err)
	}

	revisionsPath := filepath.Join("..", "..", "docs", "examples", "onboarding", "integration-revisions-response.json")
	revisionsBody, err := os.ReadFile(revisionsPath)
	if err != nil {
		t.Fatalf("read integration revisions example fixture: %v", err)
	}

	var revisionsPayload struct {
		TenantID  string                                `json:"tenant_id"`
		AgentID   string                                `json:"agent_id"`
		Revisions []onboarding.BundleIntegrationRevision `json:"revisions"`
		Total     int                                   `json:"total"`
		Limit     int                                   `json:"limit"`
	}
	if err := json.Unmarshal(revisionsBody, &revisionsPayload); err != nil {
		t.Fatalf("decode integration revisions example fixture: %v", err)
	}
	if revisionsPayload.TenantID != "tenant-1" || revisionsPayload.AgentID != "agent-1" {
		t.Fatalf("expected integration revisions fixture to include tenant_id and agent_id, got %+v", revisionsPayload)
	}
	if revisionsPayload.Total != len(revisionsPayload.Revisions) || revisionsPayload.Limit <= 0 {
		t.Fatalf("expected integration revisions fixture to include honest total/limit, got %+v", revisionsPayload)
	}
	if len(revisionsPayload.Revisions) < 2 {
		t.Fatalf("expected at least two integration revisions in fixture, got %+v", revisionsPayload)
	}
	for _, revision := range revisionsPayload.Revisions {
		if revision.ID == "" || revision.IntegrationID == "" || revision.AgentID == "" || revision.Runtime == "" || revision.Mode == "" {
			t.Fatalf("expected integration revision to populate required fields, got %+v", revision)
		}
		if _, err := time.Parse(time.RFC3339, revision.CreatedAt); err != nil {
			t.Fatalf("expected RFC3339 revision.created_at, got %q: %v", revision.CreatedAt, err)
		}
	}
}

func TestOnboardingArchiveResponseExampleDocumentsBinarySemantics(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "docs", "examples", "onboarding", "archive-response.http")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read archive response example: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "HTTP/1.1 200 OK") {
		t.Fatalf("expected HTTP status line in archive example, got %s", text)
	}
	if !strings.Contains(text, "Content-Type: application/zip") {
		t.Fatalf("expected zip content type in archive example, got %s", text)
	}
	if !strings.Contains(text, "Content-Disposition: attachment; filename=\"openclause-alpha-corp-support-bot-created.zip\"") {
		t.Fatalf("expected stable archive filename example, got %s", text)
	}
}
