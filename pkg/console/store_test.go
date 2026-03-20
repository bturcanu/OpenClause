package console

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestClampLimit(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{0, 100},
		{-1, 100},
		{50, 50},
		{100, 100},
		{200, 100},
		{1, 1},
	}
	for _, tt := range tests {
		got := clampLimit(tt.input)
		if got != tt.want {
			t.Errorf("clampLimit(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestClampOffset(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{-5, 0},
		{-1, 0},
		{0, 0},
		{10, 10},
		{999, 999},
	}
	for _, tt := range tests {
		got := clampOffset(tt.input)
		if got != tt.want {
			t.Errorf("clampOffset(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func Test_hashInviteResetToken_DeterministicAndDistinct(t *testing.T) {
	s := &Store{tokenHMACSecret: []byte("test-secret")}

	h1 := s.hashInviteResetToken("token-a")
	h2 := s.hashInviteResetToken("token-a")
	h3 := s.hashInviteResetToken("token-b")

	if h1 == "" {
		t.Fatal("expected non-empty hash")
	}
	if h1 != h2 {
		t.Fatalf("expected deterministic hash, got h1=%q h2=%q", h1, h2)
	}
	if h1 == h3 {
		t.Fatalf("expected distinct tokens to have distinct hashes: h=%q", h1)
	}
}

func Test_hashInviteResetToken_MatchesExpectedHMACSHA256(t *testing.T) {
	secret := []byte("test-secret")
	raw := "token-a"

	s := &Store{tokenHMACSecret: secret}
	got := s.hashInviteResetToken(raw)

	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(raw))
	want := hex.EncodeToString(mac.Sum(nil))

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestCanonicalEmail(t *testing.T) {
	got := canonicalEmail("  User.Name+test@Example.COM ")
	if got != "user.name+test@example.com" {
		t.Fatalf("unexpected canonical email: %q", got)
	}
}
