package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type storedAuthConfig struct {
	CurrentProfile string                       `json:"current_profile,omitempty"`
	Profiles       map[string]storedAuthProfile `json:"profiles,omitempty"`
}

type storedAuthProfile struct {
	ServerURL string         `json:"server_url"`
	Token     string         `json:"token"`
	SessionID string         `json:"session_id,omitempty"`
	User      storedAuthUser `json:"user,omitempty"`
	UpdatedAt string         `json:"updated_at,omitempty"`
}

type storedAuthUser struct {
	ID    string   `json:"id,omitempty"`
	Email string   `json:"email,omitempty"`
	Name  string   `json:"name,omitempty"`
	Roles []string `json:"roles,omitempty"`
}

func openClauseConfigDir() (string, error) {
	if override := strings.TrimSpace(os.Getenv("OPENCLAUSE_CONFIG_DIR")); override != "" {
		return override, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(base, "openclause"), nil
}

func openClauseAuthConfigPath() (string, error) {
	dir, err := openClauseConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "auth.json"), nil
}

func loadStoredAuthConfig() (*storedAuthConfig, error) {
	path, err := openClauseAuthConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &storedAuthConfig{Profiles: map[string]storedAuthProfile{}}, nil
		}
		return nil, fmt.Errorf("read auth config: %w", err)
	}
	var cfg storedAuthConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("decode auth config: %w", err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]storedAuthProfile{}
	}
	return &cfg, nil
}

func saveStoredAuthConfig(cfg *storedAuthConfig) error {
	if cfg == nil {
		return fmt.Errorf("auth config required")
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]storedAuthProfile{}
	}
	path, err := openClauseAuthConfigPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create auth config dir: %w", err)
	}
	encoded, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode auth config: %w", err)
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		return fmt.Errorf("write auth config: %w", err)
	}
	return nil
}

func normalizeProfileName(serverURL, profile string) string {
	if text := strings.TrimSpace(profile); text != "" {
		return text
	}
	return strings.TrimRight(strings.TrimSpace(serverURL), "/")
}

func storeAuthProfile(serverURL, profileName string, resp *loginResponse) error {
	if resp == nil {
		return fmt.Errorf("login response required")
	}
	cfg, err := loadStoredAuthConfig()
	if err != nil {
		return err
	}
	key := normalizeProfileName(serverURL, profileName)
	cfg.Profiles[key] = storedAuthProfile{
		ServerURL: strings.TrimRight(strings.TrimSpace(serverURL), "/"),
		Token:     strings.TrimSpace(resp.Token),
		SessionID: strings.TrimSpace(resp.SessionID),
		User: storedAuthUser{
			ID:    strings.TrimSpace(resp.User.ID),
			Email: strings.TrimSpace(resp.User.Email),
			Name:  strings.TrimSpace(resp.User.Name),
			Roles: append([]string(nil), resp.User.Roles...),
		},
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	cfg.CurrentProfile = key
	return saveStoredAuthConfig(cfg)
}

func resolveStoredAuthToken(serverURL, profileName string) (string, error) {
	cfg, err := loadStoredAuthConfig()
	if err != nil {
		return "", err
	}
	if len(cfg.Profiles) == 0 {
		return "", nil
	}

	if profile := strings.TrimSpace(profileName); profile != "" {
		entry, ok := cfg.Profiles[profile]
		if !ok {
			return "", fmt.Errorf("stored auth profile %q not found", profile)
		}
		return strings.TrimSpace(entry.Token), nil
	}

	if server := strings.TrimRight(strings.TrimSpace(serverURL), "/"); server != "" {
		if entry, ok := cfg.Profiles[server]; ok {
			return strings.TrimSpace(entry.Token), nil
		}
		matches := make([]string, 0, len(cfg.Profiles))
		for key, entry := range cfg.Profiles {
			if strings.TrimRight(strings.TrimSpace(entry.ServerURL), "/") == server {
				matches = append(matches, key)
			}
		}
		switch len(matches) {
		case 0:
			return "", nil
		case 1:
			return strings.TrimSpace(cfg.Profiles[matches[0]].Token), nil
		default:
			sort.Strings(matches)
			return "", fmt.Errorf("multiple stored auth profiles match %s; pass --auth-profile", server)
		}
	}
	if current := strings.TrimSpace(cfg.CurrentProfile); current != "" {
		if entry, ok := cfg.Profiles[current]; ok && strings.TrimSpace(entry.Token) != "" {
			return strings.TrimSpace(entry.Token), nil
		}
	}
	return "", nil
}

func resolveStoredAuthProfileKey(cfg *storedAuthConfig, serverURL, profileName string) (string, error) {
	if cfg == nil || len(cfg.Profiles) == 0 {
		return "", fmt.Errorf("no stored auth profiles")
	}
	if profile := strings.TrimSpace(profileName); profile != "" {
		if _, ok := cfg.Profiles[profile]; ok {
			return profile, nil
		}
		return "", fmt.Errorf("stored auth profile %q not found", profile)
	}
	if server := strings.TrimRight(strings.TrimSpace(serverURL), "/"); server != "" {
		if _, ok := cfg.Profiles[server]; ok {
			return server, nil
		}
		matches := make([]string, 0, len(cfg.Profiles))
		for key, entry := range cfg.Profiles {
			if strings.TrimRight(strings.TrimSpace(entry.ServerURL), "/") == server {
				matches = append(matches, key)
			}
		}
		switch len(matches) {
		case 0:
			return "", fmt.Errorf("no stored auth profile for %s", server)
		case 1:
			return matches[0], nil
		default:
			sort.Strings(matches)
			return "", fmt.Errorf("multiple stored auth profiles match %s; pass --profile", server)
		}
	}
	if current := strings.TrimSpace(cfg.CurrentProfile); current != "" {
		if _, ok := cfg.Profiles[current]; ok {
			return current, nil
		}
	}
	if len(cfg.Profiles) == 1 {
		for key := range cfg.Profiles {
			return key, nil
		}
	}
	for key := range cfg.Profiles {
		_ = key
		break
	}
	return "", fmt.Errorf("multiple stored auth profiles found; pass --profile or --server-url")
}
