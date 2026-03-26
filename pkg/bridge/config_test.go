package bridge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveConfigResolvesEnvAndDefaults(t *testing.T) {
	t.Setenv("OPENCLAUSE_API_KEY", "sk-oc-bridge")
	t.Setenv("LOCAL_MODEL_BASE_URL", "http://localhost:1234/v1")
	t.Setenv("LOCAL_MODEL_NAME", "qwen/qwen3.5-9b")

	cfg, err := ResolveConfig(Config{
		BaseURL:  "http://localhost:8080/",
		TenantID: "tenant-1",
		AgentID:  "Agent One",
		APIKey:   "env:OPENCLAUSE_API_KEY",
		Tools: []ToolConfig{
			{Tool: "Slack", Action: "Msg.Post", RiskScore: 4},
		},
		OpenAI: OpenAIConfig{
			UpstreamBaseURL: "env:LOCAL_MODEL_BASE_URL",
			Model:           "env:LOCAL_MODEL_NAME",
			ToolName:        "governed_action",
		},
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}

	if cfg.Listen != DefaultListenAddr {
		t.Fatalf("expected default listen addr, got %q", cfg.Listen)
	}
	if cfg.BaseURL != "http://localhost:8080" {
		t.Fatalf("expected trimmed base URL, got %q", cfg.BaseURL)
	}
	if cfg.APIKey != "sk-oc-bridge" {
		t.Fatalf("expected resolved env api key, got %q", cfg.APIKey)
	}
	if cfg.Defaults.RiskMode != "configured" {
		t.Fatalf("expected configured risk mode, got %q", cfg.Defaults.RiskMode)
	}
	if cfg.Defaults.SessionPrefix != "agent-one" {
		t.Fatalf("expected session prefix derived from agent id, got %q", cfg.Defaults.SessionPrefix)
	}
	if tool, ok := cfg.LookupTool("slack", "msg.post"); !ok || tool.RiskScore != 4 {
		t.Fatalf("expected normalized tool lookup, got %+v ok=%v", tool, ok)
	}
	if !cfg.OpenAI.Enabled || cfg.OpenAI.UpstreamBaseURL != "http://localhost:1234/v1" || cfg.OpenAI.Model != "qwen/qwen3.5-9b" {
		t.Fatalf("expected resolved openai config, got %+v", cfg.OpenAI)
	}
}

func TestResolveConfigRejectsMissingEnvReference(t *testing.T) {
	_, err := ResolveConfig(Config{
		BaseURL:  "http://localhost:8080",
		TenantID: "tenant-1",
		AgentID:  "agent-1",
		APIKey:   "env:MISSING_KEY",
	})
	if err == nil || err.Error() != `bridge env reference "MISSING_KEY" is empty` {
		t.Fatalf("expected missing env error, got %v", err)
	}
}

func TestLoadConfigFileRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bridge.yaml")
	if err := os.WriteFile(path, []byte("base_url: http://localhost:8080\ntenant_id: tenant-1\nagent_id: agent-1\napi_key: sk\noops: true\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := LoadConfigFile(path); err == nil {
		t.Fatalf("expected unknown field error")
	}
}

func TestResolveConfigRejectsDuplicateTools(t *testing.T) {
	t.Setenv("OPENCLAUSE_API_KEY", "sk-oc-bridge")
	_, err := ResolveConfig(Config{
		BaseURL:  "http://localhost:8080",
		TenantID: "tenant-1",
		AgentID:  "agent-1",
		APIKey:   "env:OPENCLAUSE_API_KEY",
		Tools: []ToolConfig{
			{Tool: "slack", Action: "msg.post", RiskScore: 4},
			{Tool: "slack", Action: "msg.post", RiskScore: 5},
		},
	})
	if err == nil || err.Error() != "duplicate bridge tool config for slack:msg.post" {
		t.Fatalf("expected duplicate tool error, got %v", err)
	}
}

func TestResolveConfigRejectsOpenAIConfigWithoutModel(t *testing.T) {
	_, err := ResolveConfig(Config{
		BaseURL:  "http://localhost:8080",
		TenantID: "tenant-1",
		AgentID:  "agent-1",
		APIKey:   "sk-oc-bridge",
		OpenAI: OpenAIConfig{
			UpstreamBaseURL: "http://localhost:1234/v1",
		},
	})
	if err == nil || err.Error() != `bridge profile "default" openai.model required when openai config is present` {
		t.Fatalf("expected missing model error, got %v", err)
	}
}

func TestResolveConfigSupportsMultipleProfiles(t *testing.T) {
	cfg, err := ResolveConfig(Config{
		DefaultProfile: "tenant-b",
		Profiles: map[string]ProfileConfig{
			"tenant-a": {
				BaseURL:  "http://localhost:8080",
				TenantID: "tenant-a",
				AgentID:  "agent-a",
				APIKey:   "sk-a",
				Tools:    []ToolConfig{{Tool: "postgres", Action: "query.readonly", RiskScore: 2}},
			},
			"tenant-b": {
				BaseURL:  "http://localhost:8080",
				TenantID: "tenant-b",
				AgentID:  "agent-b",
				APIKey:   "sk-b",
				Tools:    []ToolConfig{{Tool: "slack", Action: "msg.post", RiskScore: 4}},
			},
		},
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}

	if cfg.DefaultProfile != "tenant-b" || cfg.TenantID != "tenant-b" || cfg.AgentID != "agent-b" {
		t.Fatalf("expected default profile aliases to follow tenant-b, got %+v", cfg)
	}
	if len(cfg.Profiles) != 2 {
		t.Fatalf("expected two resolved profiles, got %d", len(cfg.Profiles))
	}
	if profile, ok := cfg.ResolveProfile("tenant-a"); !ok || profile.AgentID != "agent-a" {
		t.Fatalf("expected tenant-a profile lookup, got %+v ok=%v", profile, ok)
	}
}
