package evidence

import (
	"strings"
	"testing"
)

func TestResolveBundleSigningKeyDerivesStableEd25519KeyFromSecret(t *testing.T) {
	first, err := ResolveBundleSigningKey("", "test-secret")
	if err != nil {
		t.Fatalf("ResolveBundleSigningKey(first): %v", err)
	}
	second, err := ResolveBundleSigningKey("", "test-secret")
	if err != nil {
		t.Fatalf("ResolveBundleSigningKey(second): %v", err)
	}

	if first.KeyID == "" || first.KeyID != second.KeyID {
		t.Fatalf("expected stable key fingerprint, got %q and %q", first.KeyID, second.KeyID)
	}
	if first.PublicKeyBase64 == "" || first.PublicKeyBase64 != second.PublicKeyBase64 {
		t.Fatalf("expected stable public key, got %q and %q", first.PublicKeyBase64, second.PublicKeyBase64)
	}
}

func TestResolveBundleVerificationKeyFallsBackToEmbeddedPublicKey(t *testing.T) {
	signer, err := ResolveBundleSigningKey("", "test-secret")
	if err != nil {
		t.Fatalf("ResolveBundleSigningKey: %v", err)
	}
	verifyKey, err := ResolveBundleVerificationKey("", "", signer.PublicKeyBase64)
	if err != nil {
		t.Fatalf("ResolveBundleVerificationKey: %v", err)
	}
	if verifyKey.KeyID != signer.KeyID {
		t.Fatalf("expected matching key fingerprint, got %q want %q", verifyKey.KeyID, signer.KeyID)
	}
}

func TestVerifyCanonicalPayloadRejectsTamperedSignature(t *testing.T) {
	signer, err := ResolveBundleSigningKey("", "test-secret")
	if err != nil {
		t.Fatalf("ResolveBundleSigningKey: %v", err)
	}
	signature := SignCanonicalPayload([]byte(`{"hello":"world"}`), signer.PrivateKey)
	if err := VerifyCanonicalPayload([]byte(`{"hello":"tampered"}`), signer.PublicKey, signature); err == nil || !strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("expected signature verification failure, got %v", err)
	}
}
