package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"strings"
)

func TestResolveStoredAuthTokenPrefersExplicitProfileThenServerThenCurrent(t *testing.T) {
	t.Setenv("OPENCLAUSE_CONFIG_DIR", t.TempDir())
	if err := saveStoredAuthConfig(&storedAuthConfig{
		CurrentProfile: "current",
		Profiles: map[string]storedAuthProfile{
			"explicit": {
				ServerURL: "http://explicit.test",
				Token:     "token-explicit",
			},
			"http://server.test": {
				ServerURL: "http://server.test/",
				Token:     "token-server",
			},
			"current": {
				ServerURL: "http://current.test",
				Token:     "token-current",
			},
		},
	}); err != nil {
		t.Fatalf("saveStoredAuthConfig: %v", err)
	}

	got, err := resolveStoredAuthToken("http://server.test/", "explicit")
	if err != nil {
		t.Fatalf("resolveStoredAuthToken explicit: %v", err)
	}
	if got != "token-explicit" {
		t.Fatalf("expected explicit profile token, got %q", got)
	}

	got, err = resolveStoredAuthToken("http://server.test/", "")
	if err != nil {
		t.Fatalf("resolveStoredAuthToken server fallback: %v", err)
	}
	if got != "token-server" {
		t.Fatalf("expected server-url token fallback, got %q", got)
	}

	got, err = resolveStoredAuthToken("", "")
	if err != nil {
		t.Fatalf("resolveStoredAuthToken current fallback: %v", err)
	}
	if got != "token-current" {
		t.Fatalf("expected current profile token, got %q", got)
	}
}

func TestResolveStoredAuthTokenMatchesCustomNamedProfileByServerURL(t *testing.T) {
	t.Setenv("OPENCLAUSE_CONFIG_DIR", t.TempDir())
	if err := saveStoredAuthConfig(&storedAuthConfig{
		CurrentProfile: "sandbox",
		Profiles: map[string]storedAuthProfile{
			"prod-admin": {
				ServerURL: "http://console.prod.test/",
				Token:     "token-prod",
			},
			"sandbox": {
				ServerURL: "http://console.sandbox.test/",
				Token:     "token-sandbox",
			},
		},
	}); err != nil {
		t.Fatalf("saveStoredAuthConfig: %v", err)
	}

	got, err := resolveStoredAuthToken("http://console.prod.test", "")
	if err != nil {
		t.Fatalf("resolveStoredAuthToken custom profile by server URL: %v", err)
	}
	if got != "token-prod" {
		t.Fatalf("expected token from server-matched custom profile, got %q", got)
	}
}

func TestResolveStoredAuthTokenDoesNotFallBackToCurrentProfileForDifferentServer(t *testing.T) {
	t.Setenv("OPENCLAUSE_CONFIG_DIR", t.TempDir())
	if err := saveStoredAuthConfig(&storedAuthConfig{
		CurrentProfile: "current",
		Profiles: map[string]storedAuthProfile{
			"current": {
				ServerURL: "http://console.current.test",
				Token:     "token-current",
			},
			"other": {
				ServerURL: "http://console.other.test",
				Token:     "token-other",
			},
		},
	}); err != nil {
		t.Fatalf("saveStoredAuthConfig: %v", err)
	}

	got, err := resolveStoredAuthToken("http://console.missing.test", "")
	if err != nil {
		t.Fatalf("resolveStoredAuthToken mismatched server: %v", err)
	}
	if got != "" {
		t.Fatalf("expected no token when server URL does not match a stored profile, got %q", got)
	}
}

func TestResolveStoredAuthTokenFailsWhenExplicitProfileIsMissing(t *testing.T) {
	t.Setenv("OPENCLAUSE_CONFIG_DIR", t.TempDir())
	if err := saveStoredAuthConfig(&storedAuthConfig{
		CurrentProfile: "current",
		Profiles: map[string]storedAuthProfile{
			"current": {
				ServerURL: "http://console.current.test",
				Token:     "token-current",
			},
		},
	}); err != nil {
		t.Fatalf("saveStoredAuthConfig: %v", err)
	}

	_, err := resolveStoredAuthToken("http://console.current.test", "missing")
	if err == nil || !strings.Contains(err.Error(), `stored auth profile "missing" not found`) {
		t.Fatalf("expected explicit missing profile error, got %v", err)
	}
}

func TestResolveStoredAuthTokenRequiresDisambiguationWhenServerMatchesMultipleProfiles(t *testing.T) {
	t.Setenv("OPENCLAUSE_CONFIG_DIR", t.TempDir())
	if err := saveStoredAuthConfig(&storedAuthConfig{
		Profiles: map[string]storedAuthProfile{
			"prod-admin": {
				ServerURL: "http://console.prod.test/",
				Token:     "token-prod-admin",
			},
			"prod-ops": {
				ServerURL: "http://console.prod.test/",
				Token:     "token-prod-ops",
			},
		},
	}); err != nil {
		t.Fatalf("saveStoredAuthConfig: %v", err)
	}

	_, err := resolveStoredAuthToken("http://console.prod.test", "")
	if err == nil || !strings.Contains(err.Error(), "multiple stored auth profiles match http://console.prod.test") {
		t.Fatalf("expected server ambiguity error, got %v", err)
	}
}

func TestResolveStoredAuthProfileKeyMatchesProfileNameAndServerURL(t *testing.T) {
	cfg := &storedAuthConfig{
		CurrentProfile: "current",
		Profiles: map[string]storedAuthProfile{
			"explicit": {
				ServerURL: "http://explicit.test/",
				Token:     "token-explicit",
			},
			"current": {
				ServerURL: "http://current.test/",
				Token:     "token-current",
			},
		},
	}

	key, err := resolveStoredAuthProfileKey(cfg, "", "explicit")
	if err != nil {
		t.Fatalf("resolveStoredAuthProfileKey explicit: %v", err)
	}
	if key != "explicit" {
		t.Fatalf("expected explicit profile key, got %q", key)
	}

	key, err = resolveStoredAuthProfileKey(cfg, "http://current.test", "")
	if err != nil {
		t.Fatalf("resolveStoredAuthProfileKey server url: %v", err)
	}
	if key != "current" {
		t.Fatalf("expected server URL match, got %q", key)
	}
}

func TestResolveStoredAuthProfileKeyRequiresDisambiguationWhenMultipleProfilesExist(t *testing.T) {
	cfg := &storedAuthConfig{
		Profiles: map[string]storedAuthProfile{
			"alpha": {
				ServerURL: "http://alpha.test/",
				Token:     "token-alpha",
			},
			"beta": {
				ServerURL: "http://beta.test/",
				Token:     "token-beta",
			},
		},
	}

	_, err := resolveStoredAuthProfileKey(cfg, "", "")
	if err == nil || !strings.Contains(err.Error(), "multiple stored auth profiles found") {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
}

func TestResolveStoredAuthProfileKeyRequiresDisambiguationWhenServerMatchesMultipleProfiles(t *testing.T) {
	cfg := &storedAuthConfig{
		Profiles: map[string]storedAuthProfile{
			"alpha": {
				ServerURL: "http://console.prod.test/",
				Token:     "token-alpha",
			},
			"beta": {
				ServerURL: "http://console.prod.test/",
				Token:     "token-beta",
			},
		},
	}

	_, err := resolveStoredAuthProfileKey(cfg, "http://console.prod.test", "")
	if err == nil || !strings.Contains(err.Error(), "multiple stored auth profiles match http://console.prod.test") {
		t.Fatalf("expected server ambiguity error, got %v", err)
	}
}

func TestRunAuthLoginSupportsPasswordStdin(t *testing.T) {
	t.Setenv("OPENCLAUSE_CONFIG_DIR", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/login" {
			t.Fatalf("unexpected auth path %s", r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode auth request: %v", err)
		}
		if body["password"] != "Admin123!" {
			t.Fatalf("expected password stdin value, got %+v", body)
		}
		_ = json.NewEncoder(w).Encode(loginResponse{
			Token:     "stored-token-stdin",
			SessionID: "sess-stdin",
			User: authUser{
				ID:    "user-stdin",
				Email: "admin@openclause.dev",
				Name:  "Admin",
				Roles: []string{"platform_admin"},
			},
		})
	}))
	defer server.Close()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	if err := runWithIO([]string{
		"auth", "login",
		"--server-url", server.URL,
		"--email", "admin@openclause.dev",
		"--password-stdin",
	}, strings.NewReader("Admin123!\n"), stdout, stderr); err != nil {
		t.Fatalf("run auth login with password stdin: %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Profile:") || !strings.Contains(stdout.String(), "Token stored for future server-backed onboarding commands.") {
		t.Fatalf("expected login output, got %s", stdout.String())
	}
}
