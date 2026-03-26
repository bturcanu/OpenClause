package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/bturcanu/OpenClause/pkg/console"
	"github.com/bturcanu/OpenClause/pkg/evidence"
)

type exportBundleManifest struct {
	Version         string `json:"version"`
	GeneratedAt     string `json:"generated_at"`
	EventCount      int    `json:"event_count"`
	PayloadSHA256   string `json:"payload_sha256"`
	EventsSHA256    string `json:"events_sha256"`
	StartPrevHash   string `json:"start_prev_hash,omitempty"`
	EndHash         string `json:"end_hash,omitempty"`
	ChainContiguous bool   `json:"chain_contiguous"`
	SignatureScheme string `json:"signature_scheme"`
	SigningKeyID    string `json:"signing_key_id,omitempty"`
	PublicKey       string `json:"public_key,omitempty"`
}

type evidenceBundle struct {
	Version           string                `json:"version"`
	TenantID          string                `json:"tenant_id"`
	ExportedAt        string                `json:"exported_at"`
	Since             string                `json:"since"`
	Until             string                `json:"until"`
	Events            []console.EventDetail `json:"events"`
	EventCount        int                   `json:"event_count"`
	Manifest          exportBundleManifest  `json:"manifest"`
	ManifestSignature string                `json:"manifest_signature"`
}

type verifyConfig struct {
	BundlePath          string
	PublicKey           string
	PublicKeyFile       string
	RequireSigningKeyID string
}

func run(bundlePath string) error {
	return runWithConfig(verifyConfig{BundlePath: bundlePath})
}

func runWithConfig(cfg verifyConfig) error {
	if strings.TrimSpace(cfg.BundlePath) == "" {
		return fmt.Errorf("--bundle flag is required")
	}
	if strings.TrimSpace(cfg.PublicKey) != "" && strings.TrimSpace(cfg.PublicKeyFile) != "" {
		return fmt.Errorf("--public-key and --public-key-file cannot be combined")
	}

	data, err := os.ReadFile(cfg.BundlePath)
	if err != nil {
		return fmt.Errorf("read bundle file: %w", err)
	}

	var bundle evidenceBundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	if bundle.Version == "" {
		return fmt.Errorf("missing required field: version")
	}
	if bundle.TenantID == "" {
		return fmt.Errorf("missing required field: tenant_id")
	}
	if bundle.Events == nil {
		return fmt.Errorf("missing required field: events")
	}
	if bundle.Manifest.Version == "" {
		return fmt.Errorf("missing required field: manifest.version")
	}
	if strings.TrimSpace(bundle.ManifestSignature) == "" {
		return fmt.Errorf("missing required field: manifest_signature")
	}
	if bundle.Manifest.SignatureScheme != evidence.SignatureSchemeEd25519 {
		return fmt.Errorf("unsupported manifest signature scheme: %s", bundle.Manifest.SignatureScheme)
	}
	if bundle.EventCount != len(bundle.Events) {
		return fmt.Errorf("event_count mismatch: expected %d got %d", len(bundle.Events), bundle.EventCount)
	}
	if !exportBundleChainContiguous(bundle.Events) {
		return fmt.Errorf("event hash chain is not contiguous inside the bundle")
	}

	payload := struct {
		Version    string                `json:"version"`
		TenantID   string                `json:"tenant_id"`
		ExportedAt string                `json:"exported_at"`
		Since      string                `json:"since"`
		Until      string                `json:"until"`
		Events     []console.EventDetail `json:"events"`
		EventCount int                   `json:"event_count"`
	}{
		Version:    bundle.Version,
		TenantID:   bundle.TenantID,
		ExportedAt: bundle.ExportedAt,
		Since:      bundle.Since,
		Until:      bundle.Until,
		Events:     bundle.Events,
		EventCount: bundle.EventCount,
	}
	_, payloadHash, err := evidence.HashPayload(payload)
	if err != nil {
		return fmt.Errorf("hash bundle payload: %w", err)
	}
	if payloadHash != bundle.Manifest.PayloadSHA256 {
		return fmt.Errorf("payload hash mismatch")
	}
	_, eventsHash, err := evidence.HashPayload(bundle.Events)
	if err != nil {
		return fmt.Errorf("hash events: %w", err)
	}
	if eventsHash != bundle.Manifest.EventsSHA256 {
		return fmt.Errorf("events hash mismatch")
	}
	if len(bundle.Events) > 0 {
		if bundle.Manifest.StartPrevHash != bundle.Events[0].PrevHash {
			return fmt.Errorf("start_prev_hash mismatch")
		}
		if bundle.Manifest.EndHash != bundle.Events[len(bundle.Events)-1].Hash {
			return fmt.Errorf("end_hash mismatch")
		}
	}

	manifestCanon, _, err := evidence.HashPayload(bundle.Manifest)
	if err != nil {
		return fmt.Errorf("hash manifest: %w", err)
	}
	publicKeyValue := strings.TrimSpace(cfg.PublicKey)
	if strings.TrimSpace(cfg.PublicKeyFile) != "" {
		data, err := os.ReadFile(strings.TrimSpace(cfg.PublicKeyFile))
		if err != nil {
			return fmt.Errorf("read public key file: %w", err)
		}
		publicKeyValue = strings.TrimSpace(string(data))
	}
	verifyKey, err := evidence.ResolveBundleVerificationKey(
		firstNonEmpty(publicKeyValue, os.Getenv("EVIDENCE_BUNDLE_SIGNING_PUBLIC_KEY")),
		os.Getenv("CONSOLE_JWT_SECRET"),
		bundle.Manifest.PublicKey,
	)
	if err != nil {
		return err
	}
	if required := strings.TrimSpace(cfg.RequireSigningKeyID); required != "" && verifyKey.KeyID != required {
		return fmt.Errorf("required signing_key_id mismatch")
	}
	if bundle.Manifest.SigningKeyID != "" && verifyKey.KeyID != bundle.Manifest.SigningKeyID {
		return fmt.Errorf("manifest signing_key_id mismatch")
	}
	if err := evidence.VerifyCanonicalPayload(manifestCanon, verifyKey.PublicKey, bundle.ManifestSignature); err != nil {
		return fmt.Errorf("manifest signature mismatch: %w", err)
	}

	fmt.Printf("Tenant:      %s\n", bundle.TenantID)
	fmt.Printf("Event count: %d\n", len(bundle.Events))
	fmt.Printf("Time range:  %s to %s\n", bundle.Since, bundle.Until)
	fmt.Printf("End hash:    %s\n", bundle.Manifest.EndHash)
	fmt.Printf("Signing key: %s\n", verifyKey.KeyID)
	fmt.Println("Bundle verification: PASS")
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func exportBundleChainContiguous(events []console.EventDetail) bool {
	if len(events) <= 1 {
		return true
	}
	for index := 1; index < len(events); index++ {
		if events[index].PrevHash != events[index-1].Hash {
			return false
		}
	}
	return true
}

func main() {
	bundlePath := flag.String("bundle", "", "path to evidence bundle JSON file")
	publicKey := flag.String("public-key", "", "base64-encoded ed25519 public key to pin during verification")
	publicKeyFile := flag.String("public-key-file", "", "path to a file containing the base64-encoded ed25519 public key")
	requireSigningKeyID := flag.String("require-signing-key-id", "", "expected signing_key_id to pin during verification")
	flag.Parse()

	if err := runWithConfig(verifyConfig{
		BundlePath:          *bundlePath,
		PublicKey:           *publicKey,
		PublicKeyFile:       *publicKeyFile,
		RequireSigningKeyID: *requireSigningKeyID,
	}); err != nil {
		fmt.Printf("Bundle verification: FAIL - %s\n", err)
		os.Exit(1)
	}
}
