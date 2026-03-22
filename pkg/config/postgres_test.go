package config

import (
	"errors"
	"testing"
)

func TestRequireEnvReturnsValue(t *testing.T) {
	t.Setenv("OPENCLAUSE_TEST_ENV", "configured")

	got, err := RequireEnv("OPENCLAUSE_TEST_ENV")
	if err != nil {
		t.Fatalf("RequireEnv: %v", err)
	}
	if got != "configured" {
		t.Fatalf("expected configured, got %q", got)
	}
}

func TestRequireEnvReturnsTypedErrorWhenMissing(t *testing.T) {
	t.Setenv("OPENCLAUSE_TEST_ENV_MISSING", "")

	_, err := RequireEnv("OPENCLAUSE_TEST_ENV_MISSING")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrRequiredEnvNotSet) {
		t.Fatalf("expected ErrRequiredEnvNotSet, got %v", err)
	}
}
