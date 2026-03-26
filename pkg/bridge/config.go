package bridge

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

const DefaultListenAddr = "127.0.0.1:8787"

type Config struct {
	Listen         string                   `yaml:"listen"`
	DefaultProfile string                   `yaml:"default_profile,omitempty"`
	Profiles       map[string]ProfileConfig `yaml:"profiles,omitempty"`

	BaseURL  string         `yaml:"base_url"`
	TenantID string         `yaml:"tenant_id"`
	AgentID  string         `yaml:"agent_id"`
	APIKey   string         `yaml:"api_key"`
	Defaults DefaultsConfig `yaml:"defaults"`
	Tools    []ToolConfig   `yaml:"tools"`
	OpenAI   OpenAIConfig   `yaml:"openai"`
}

type ProfileConfig struct {
	BaseURL  string         `yaml:"base_url"`
	TenantID string         `yaml:"tenant_id"`
	AgentID  string         `yaml:"agent_id"`
	APIKey   string         `yaml:"api_key"`
	Defaults DefaultsConfig `yaml:"defaults"`
	Tools    []ToolConfig   `yaml:"tools"`
	OpenAI   OpenAIConfig   `yaml:"openai"`
}

type DefaultsConfig struct {
	UserID        string `yaml:"user_id"`
	SessionPrefix string `yaml:"session_prefix"`
	RiskMode      string `yaml:"risk_mode"`
}

type ToolConfig struct {
	Tool        string   `yaml:"tool"`
	Action      string   `yaml:"action"`
	RiskScore   int      `yaml:"risk_score"`
	RiskFactors []string `yaml:"risk_factors,omitempty"`
	Resource    string   `yaml:"resource,omitempty"`
	Description string   `yaml:"description,omitempty"`
}

type OpenAIConfig struct {
	UpstreamBaseURL string `yaml:"upstream_base_url"`
	UpstreamAPIKey  string `yaml:"upstream_api_key"`
	Model           string `yaml:"model"`
	ToolName        string `yaml:"tool_name"`
	SystemPrompt    string `yaml:"system_prompt,omitempty"`
}

type ResolvedConfig struct {
	Listen         string
	DefaultProfile string
	Profiles       map[string]*ResolvedProfile

	// Legacy convenience aliases for the default profile.
	BaseURL  string
	TenantID string
	AgentID  string
	APIKey   string
	Defaults DefaultsConfig
	Tools    []ToolConfig
	OpenAI   ResolvedOpenAIConfig

	toolIndex map[string]ToolConfig
}

type ResolvedProfile struct {
	Name     string
	BaseURL  string
	TenantID string
	AgentID  string
	APIKey   string
	Defaults DefaultsConfig
	Tools    []ToolConfig
	OpenAI   ResolvedOpenAIConfig

	toolIndex map[string]ToolConfig
}

type ResolvedOpenAIConfig struct {
	Enabled         bool
	UpstreamBaseURL string
	UpstreamAPIKey  string
	Model           string
	ToolName        string
	SystemPrompt    string
}

func LoadConfigFile(path string) (*ResolvedConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read bridge config: %w", err)
	}
	var raw Config
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode bridge config: %w", err)
	}
	return ResolveConfig(raw)
}

func ResolveConfig(raw Config) (*ResolvedConfig, error) {
	listen := strings.TrimSpace(raw.Listen)
	if listen == "" {
		listen = DefaultListenAddr
	}

	profileConfigs := map[string]ProfileConfig{}
	switch {
	case len(raw.Profiles) > 0:
		if hasLegacyProfileConfig(raw) {
			return nil, fmt.Errorf("bridge config cannot mix root profile fields with profiles")
		}
		for name, cfg := range raw.Profiles {
			profileName := strings.TrimSpace(name)
			if profileName == "" {
				return nil, fmt.Errorf("bridge profiles require non-empty names")
			}
			profileConfigs[profileName] = cfg
		}
	default:
		profileConfigs["default"] = ProfileConfig{
			BaseURL:  raw.BaseURL,
			TenantID: raw.TenantID,
			AgentID:  raw.AgentID,
			APIKey:   raw.APIKey,
			Defaults: raw.Defaults,
			Tools:    raw.Tools,
			OpenAI:   raw.OpenAI,
		}
	}

	resolvedProfiles := make(map[string]*ResolvedProfile, len(profileConfigs))
	profileNames := make([]string, 0, len(profileConfigs))
	for name, cfg := range profileConfigs {
		resolved, err := resolveProfile(name, cfg)
		if err != nil {
			return nil, err
		}
		resolvedProfiles[name] = resolved
		profileNames = append(profileNames, name)
	}
	sort.Strings(profileNames)

	defaultProfile := strings.TrimSpace(raw.DefaultProfile)
	switch {
	case defaultProfile != "":
		if _, ok := resolvedProfiles[defaultProfile]; !ok {
			return nil, fmt.Errorf("bridge default_profile %q not found", defaultProfile)
		}
	case len(profileNames) == 1:
		defaultProfile = profileNames[0]
	default:
		return nil, fmt.Errorf("bridge default_profile is required when multiple profiles are configured")
	}

	defaultRuntime := resolvedProfiles[defaultProfile]
	cfg := &ResolvedConfig{
		Listen:         listen,
		DefaultProfile: defaultProfile,
		Profiles:       resolvedProfiles,
		BaseURL:        defaultRuntime.BaseURL,
		TenantID:       defaultRuntime.TenantID,
		AgentID:        defaultRuntime.AgentID,
		APIKey:         defaultRuntime.APIKey,
		Defaults:       defaultRuntime.Defaults,
		Tools:          append([]ToolConfig(nil), defaultRuntime.Tools...),
		OpenAI:         defaultRuntime.OpenAI,
		toolIndex:      map[string]ToolConfig{},
	}
	for key, value := range defaultRuntime.toolIndex {
		cfg.toolIndex[key] = value
	}
	return cfg, nil
}

func resolveProfile(name string, raw ProfileConfig) (*ResolvedProfile, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(raw.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("bridge profile %q base_url required", name)
	}
	tenantID := strings.TrimSpace(raw.TenantID)
	if tenantID == "" {
		return nil, fmt.Errorf("bridge profile %q tenant_id required", name)
	}
	agentID := strings.TrimSpace(raw.AgentID)
	if agentID == "" {
		return nil, fmt.Errorf("bridge profile %q agent_id required", name)
	}
	apiKey, err := resolveValue(strings.TrimSpace(raw.APIKey))
	if err != nil {
		return nil, err
	}
	if apiKey == "" {
		return nil, fmt.Errorf("bridge profile %q api_key required", name)
	}

	defaults := raw.Defaults
	defaults.RiskMode = strings.TrimSpace(strings.ToLower(defaults.RiskMode))
	if defaults.RiskMode == "" {
		defaults.RiskMode = "configured"
	}
	if defaults.RiskMode != "configured" && defaults.RiskMode != "request" {
		return nil, fmt.Errorf("bridge profile %q defaults.risk_mode must be one of: configured, request", name)
	}
	defaults.SessionPrefix = sanitizeSessionPrefix(defaults.SessionPrefix)
	if defaults.SessionPrefix == "" {
		defaults.SessionPrefix = sanitizeSessionPrefix(agentID)
	}

	profile := &ResolvedProfile{
		Name:      name,
		BaseURL:   baseURL,
		TenantID:  tenantID,
		AgentID:   agentID,
		APIKey:    apiKey,
		Defaults:  defaults,
		Tools:     make([]ToolConfig, 0, len(raw.Tools)),
		toolIndex: map[string]ToolConfig{},
	}
	for _, item := range raw.Tools {
		tool := strings.ToLower(strings.TrimSpace(item.Tool))
		action := strings.ToLower(strings.TrimSpace(item.Action))
		if tool == "" || action == "" {
			return nil, fmt.Errorf("bridge profile %q tools require both tool and action", name)
		}
		item.Tool = tool
		item.Action = action
		key := tool + ":" + action
		if _, exists := profile.toolIndex[key]; exists {
			return nil, fmt.Errorf("duplicate bridge tool config for %s", key)
		}
		profile.toolIndex[key] = item
		profile.Tools = append(profile.Tools, item)
	}

	if rawOpenAI := raw.OpenAI; hasOpenAIConfig(rawOpenAI) {
		upstreamBaseURLValue, err := resolveValue(strings.TrimSpace(rawOpenAI.UpstreamBaseURL))
		if err != nil {
			return nil, err
		}
		upstreamBaseURL := strings.TrimRight(strings.TrimSpace(upstreamBaseURLValue), "/")
		if upstreamBaseURL == "" {
			return nil, fmt.Errorf("bridge profile %q openai.upstream_base_url required when openai config is present", name)
		}
		upstreamAPIKey, err := resolveValue(strings.TrimSpace(rawOpenAI.UpstreamAPIKey))
		if err != nil {
			return nil, err
		}
		model, err := resolveValue(strings.TrimSpace(rawOpenAI.Model))
		if err != nil {
			return nil, err
		}
		profile.OpenAI = ResolvedOpenAIConfig{
			Enabled:         true,
			UpstreamBaseURL: upstreamBaseURL,
			UpstreamAPIKey:  upstreamAPIKey,
			Model:           strings.TrimSpace(model),
			ToolName:        strings.TrimSpace(rawOpenAI.ToolName),
			SystemPrompt:    strings.TrimSpace(rawOpenAI.SystemPrompt),
		}
		if profile.OpenAI.Model == "" {
			return nil, fmt.Errorf("bridge profile %q openai.model required when openai config is present", name)
		}
		if profile.OpenAI.ToolName == "" {
			profile.OpenAI.ToolName = "governed_action"
		}
	}

	return profile, nil
}

func (c *ResolvedConfig) LookupTool(tool, action string) (ToolConfig, bool) {
	if c == nil {
		return ToolConfig{}, false
	}
	item, ok := c.toolIndex[strings.ToLower(strings.TrimSpace(tool))+":"+strings.ToLower(strings.TrimSpace(action))]
	return item, ok
}

func (c *ResolvedConfig) ResolveProfile(name string) (*ResolvedProfile, bool) {
	if c == nil {
		return nil, false
	}
	if strings.TrimSpace(name) == "" {
		name = c.DefaultProfile
	}
	profile, ok := c.Profiles[strings.TrimSpace(name)]
	return profile, ok
}

func (p *ResolvedProfile) LookupTool(tool, action string) (ToolConfig, bool) {
	if p == nil {
		return ToolConfig{}, false
	}
	item, ok := p.toolIndex[strings.ToLower(strings.TrimSpace(tool))+":"+strings.ToLower(strings.TrimSpace(action))]
	return item, ok
}

func resolveValue(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	if !strings.HasPrefix(value, "env:") {
		return value, nil
	}
	name := strings.TrimSpace(strings.TrimPrefix(value, "env:"))
	if name == "" {
		return "", fmt.Errorf("bridge env reference is missing a variable name")
	}
	resolved := strings.TrimSpace(os.Getenv(name))
	if resolved == "" {
		return "", fmt.Errorf("bridge env reference %q is empty", name)
	}
	return resolved, nil
}

func hasOpenAIConfig(cfg OpenAIConfig) bool {
	return strings.TrimSpace(cfg.UpstreamBaseURL) != "" ||
		strings.TrimSpace(cfg.UpstreamAPIKey) != "" ||
		strings.TrimSpace(cfg.Model) != "" ||
		strings.TrimSpace(cfg.ToolName) != ""
}

func hasLegacyProfileConfig(cfg Config) bool {
	return strings.TrimSpace(cfg.BaseURL) != "" ||
		strings.TrimSpace(cfg.TenantID) != "" ||
		strings.TrimSpace(cfg.AgentID) != "" ||
		strings.TrimSpace(cfg.APIKey) != "" ||
		len(cfg.Tools) > 0 ||
		hasOpenAIConfig(cfg.OpenAI) ||
		strings.TrimSpace(cfg.Defaults.UserID) != "" ||
		strings.TrimSpace(cfg.Defaults.SessionPrefix) != "" ||
		strings.TrimSpace(cfg.Defaults.RiskMode) != ""
}

func sanitizeSessionPrefix(value string) string {
	text := strings.ToLower(strings.TrimSpace(value))
	text = strings.ReplaceAll(text, " ", "-")
	text = strings.ReplaceAll(text, "_", "-")
	text = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-':
			return r
		default:
			return -1
		}
	}, text)
	return strings.Trim(text, "-")
}
