package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bturcanu/OpenClause/pkg/console"
	"github.com/bturcanu/OpenClause/pkg/evidence"
)

func TestRunAcceptsValidSignedBundle(t *testing.T) {
	path := writeEvidenceBundleFixture(t, "verify-secret", false, false)

	if err := run(path); err != nil {
		t.Fatalf("run(valid bundle): %v", err)
	}
}

func TestRunRejectsTamperedPayloadHash(t *testing.T) {
	path := writeEvidenceBundleFixture(t, "verify-secret", true, false)

	err := run(path)
	if err == nil || !strings.Contains(err.Error(), "payload hash mismatch") {
		t.Fatalf("expected payload hash mismatch, got %v", err)
	}
}

func TestRunRequiresVerificationKey(t *testing.T) {
	path := writeEvidenceBundleFixture(t, "verify-secret", false, true)
	t.Setenv("EVIDENCE_BUNDLE_SIGNING_PUBLIC_KEY", "")
	t.Setenv("CONSOLE_JWT_SECRET", "")

	err := run(path)
	if err == nil || !strings.Contains(err.Error(), "EVIDENCE_BUNDLE_SIGNING_PUBLIC_KEY, embedded manifest public_key, or CONSOLE_JWT_SECRET is required") {
		t.Fatalf("expected missing verification key error, got %v", err)
	}
}

func TestRunRejectsSignatureMismatch(t *testing.T) {
	path := writeEvidenceBundleFixture(t, "verify-secret", false, false)
	t.Setenv("EVIDENCE_BUNDLE_SIGNING_PUBLIC_KEY", evidencePublicKeyForSecret(t, "other-secret"))

	err := run(path)
	if err == nil || !strings.Contains(err.Error(), "signing_key_id mismatch") {
		t.Fatalf("expected signing_key_id mismatch, got %v", err)
	}
}

func TestRunWithConfigAcceptsPinnedPublicKeyFile(t *testing.T) {
	path := writeEvidenceBundleFixture(t, "verify-secret", false, false)
	keyFile := filepath.Join(t.TempDir(), "evidence.pub")
	if err := os.WriteFile(keyFile, []byte(evidencePublicKeyForSecret(t, "verify-secret")), 0o644); err != nil {
		t.Fatalf("WriteFile(public key): %v", err)
	}

	if err := runWithConfig(verifyConfig{
		BundlePath:    path,
		PublicKeyFile: keyFile,
	}); err != nil {
		t.Fatalf("runWithConfig(public key file): %v", err)
	}
}

func TestRunWithConfigRejectsUnexpectedRequiredSigningKeyID(t *testing.T) {
	path := writeEvidenceBundleFixture(t, "verify-secret", false, false)

	err := runWithConfig(verifyConfig{
		BundlePath:          path,
		PublicKey:           evidencePublicKeyForSecret(t, "verify-secret"),
		RequireSigningKeyID: "sha256:not-the-real-key",
	})
	if err == nil || !strings.Contains(err.Error(), "required signing_key_id mismatch") {
		t.Fatalf("expected required signing_key_id mismatch, got %v", err)
	}
}

func TestRunWithConfigRejectsConflictingPinnedPublicKeyInputs(t *testing.T) {
	path := writeEvidenceBundleFixture(t, "verify-secret", false, false)
	keyFile := filepath.Join(t.TempDir(), "evidence.pub")
	if err := os.WriteFile(keyFile, []byte(evidencePublicKeyForSecret(t, "verify-secret")), 0o644); err != nil {
		t.Fatalf("WriteFile(public key): %v", err)
	}

	err := runWithConfig(verifyConfig{
		BundlePath:    path,
		PublicKey:     evidencePublicKeyForSecret(t, "verify-secret"),
		PublicKeyFile: keyFile,
	})
	if err == nil || !strings.Contains(err.Error(), "--public-key and --public-key-file cannot be combined") {
		t.Fatalf("expected conflicting pinned key input error, got %v", err)
	}
}

func writeEvidenceBundleFixture(t *testing.T, secret string, tamper bool, omitPublicKey bool) string {
	t.Helper()

	events := []console.EventDetail{{
		EventListItem: console.EventListItem{
			EventID:  "evt-1",
			TenantID: "tenant-1",
			AgentID:  "agent-1",
			Tool:     "postgres",
			Action:   "query.readonly",
			Reason:   "allowed for pilot",
		},
		Hash:     "hash-1",
		PrevHash: "hash-0",
	}}

	payload := struct {
		Version    string                `json:"version"`
		TenantID   string                `json:"tenant_id"`
		ExportedAt string                `json:"exported_at"`
		Since      string                `json:"since"`
		Until      string                `json:"until"`
		Events     []console.EventDetail `json:"events"`
		EventCount int                   `json:"event_count"`
	}{
		Version:    "1.1",
		TenantID:   "tenant-1",
		ExportedAt: "2026-03-26T12:00:00Z",
		Since:      "2026-03-26T00:00:00Z",
		Until:      "2026-03-26T12:00:00Z",
		Events:     events,
		EventCount: len(events),
	}
	_, payloadHash, err := evidence.HashPayload(payload)
	if err != nil {
		t.Fatalf("HashPayload(payload): %v", err)
	}
	_, eventsHash, err := evidence.HashPayload(events)
	if err != nil {
		t.Fatalf("HashPayload(events): %v", err)
	}
	signer, err := evidence.ResolveBundleSigningKey("", secret)
	if err != nil {
		t.Fatalf("ResolveBundleSigningKey: %v", err)
	}
	manifest := exportBundleManifest{
		Version:         "1",
		GeneratedAt:     payload.ExportedAt,
		EventCount:      len(events),
		PayloadSHA256:   payloadHash,
		EventsSHA256:    eventsHash,
		StartPrevHash:   events[0].PrevHash,
		EndHash:         events[len(events)-1].Hash,
		ChainContiguous: true,
		SignatureScheme: evidence.SignatureSchemeEd25519,
		SigningKeyID:    signer.KeyID,
		PublicKey:       signer.PublicKeyBase64,
	}
	if omitPublicKey {
		manifest.PublicKey = ""
	}
	if tamper {
		manifest.PayloadSHA256 = "tampered"
	}
	manifestCanon, _, err := evidence.HashPayload(manifest)
	if err != nil {
		t.Fatalf("HashPayload(manifest): %v", err)
	}
	bundle := evidenceBundle{
		Version:           payload.Version,
		TenantID:          payload.TenantID,
		ExportedAt:        payload.ExportedAt,
		Since:             payload.Since,
		Until:             payload.Until,
		Events:            payload.Events,
		EventCount:        payload.EventCount,
		Manifest:          manifest,
		ManifestSignature: evidence.SignCanonicalPayload(manifestCanon, signer.PrivateKey),
	}

	data, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("json.Marshal(bundle): %v", err)
	}
	path := filepath.Join(t.TempDir(), "bundle.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(bundle): %v", err)
	}
	return path
}

func evidencePublicKeyForSecret(t *testing.T, secret string) string {
	t.Helper()
	signer, err := evidence.ResolveBundleSigningKey("", secret)
	if err != nil {
		t.Fatalf("ResolveBundleSigningKey(%q): %v", secret, err)
	}
	return signer.PublicKeyBase64
}
