package console

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateAndValidateToken(t *testing.T) {
	cfg := JWTConfig{Secret: "test-secret-key", Issuer: "openclause-test", ExpiryHours: 1}
	claims := JWTClaims{
		Sub:    "user-123",
		Email:  "alice@example.com",
		Name:   "Alice",
		Roles:  []string{"admin"},
		Tenant: "tenant-abc",
	}

	token, err := GenerateToken(cfg, claims)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	got, err := ValidateToken(cfg, token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}

	if got.Sub != claims.Sub {
		t.Errorf("Sub = %q, want %q", got.Sub, claims.Sub)
	}
	if got.Email != claims.Email {
		t.Errorf("Email = %q, want %q", got.Email, claims.Email)
	}
	if got.Tenant != claims.Tenant {
		t.Errorf("Tenant = %q, want %q", got.Tenant, claims.Tenant)
	}
	if got.Iss != cfg.Issuer {
		t.Errorf("Iss = %q, want %q", got.Iss, cfg.Issuer)
	}
	if got.Exp <= got.Iat {
		t.Errorf("Exp (%d) should be after Iat (%d)", got.Exp, got.Iat)
	}
}

func TestTokenExpiry(t *testing.T) {
	cfg := JWTConfig{Secret: "test-secret-key", Issuer: "openclause-test", ExpiryHours: 0}
	claims := JWTClaims{Sub: "user-456", Email: "bob@example.com"}

	token, err := GenerateToken(cfg, claims)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	time.Sleep(1100 * time.Millisecond)

	_, err = ValidateToken(cfg, token)
	if err == nil {
		t.Fatal("expected validation to fail for expired token")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("expected expiry error, got: %v", err)
	}
}

func TestInvalidSignature(t *testing.T) {
	cfg := JWTConfig{Secret: "test-secret-key", Issuer: "openclause-test", ExpiryHours: 1}
	claims := JWTClaims{Sub: "user-789"}

	token, err := GenerateToken(cfg, claims)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	tampered := token[:len(token)-4] + "XXXX"

	_, err = ValidateToken(cfg, tampered)
	if err == nil {
		t.Fatal("expected validation to fail for tampered token")
	}
	if !strings.Contains(err.Error(), "signature") {
		t.Errorf("expected signature error, got: %v", err)
	}
}

func TestInvalidIssuer(t *testing.T) {
	genCfg := JWTConfig{Secret: "shared-secret", Issuer: "issuer-a", ExpiryHours: 1}
	valCfg := JWTConfig{Secret: "shared-secret", Issuer: "issuer-b", ExpiryHours: 1}
	claims := JWTClaims{Sub: "user-000"}

	token, err := GenerateToken(genCfg, claims)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	_, err = ValidateToken(valCfg, token)
	if err == nil {
		t.Fatal("expected validation to fail for wrong issuer")
	}
	if !strings.Contains(err.Error(), "issuer") {
		t.Errorf("expected issuer error, got: %v", err)
	}
}
