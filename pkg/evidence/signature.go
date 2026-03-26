package evidence

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

const SignatureSchemeEd25519 = "ed25519"

type BundleSigningKey struct {
	PublicKey       ed25519.PublicKey
	PrivateKey      ed25519.PrivateKey
	PublicKeyBase64 string
	KeyID           string
}

type BundleVerificationKey struct {
	PublicKey ed25519.PublicKey
	KeyID     string
}

func ResolveBundleSigningKey(privateKeyValue, fallbackSecret string) (BundleSigningKey, error) {
	if value := strings.TrimSpace(privateKeyValue); value != "" {
		privateKey, publicKey, err := decodeEd25519PrivateKey(value)
		if err != nil {
			return BundleSigningKey{}, err
		}
		return newBundleSigningKey(privateKey, publicKey), nil
	}
	if secret := strings.TrimSpace(fallbackSecret); secret != "" {
		sum := sha256.Sum256([]byte(secret))
		privateKey := ed25519.NewKeyFromSeed(sum[:])
		publicKey := privateKey.Public().(ed25519.PublicKey)
		return newBundleSigningKey(privateKey, publicKey), nil
	}
	return BundleSigningKey{}, fmt.Errorf("EVIDENCE_BUNDLE_SIGNING_PRIVATE_KEY or CONSOLE_JWT_SECRET is required to sign evidence bundles")
}

func ResolveBundleVerificationKey(publicKeyValue, fallbackSecret, embeddedPublicKey string) (BundleVerificationKey, error) {
	if value := strings.TrimSpace(publicKeyValue); value != "" {
		publicKey, err := decodeEd25519PublicKey(value)
		if err != nil {
			return BundleVerificationKey{}, err
		}
		return BundleVerificationKey{PublicKey: publicKey, KeyID: BundleKeyFingerprint(publicKey)}, nil
	}
	if value := strings.TrimSpace(embeddedPublicKey); value != "" {
		publicKey, err := decodeEd25519PublicKey(value)
		if err != nil {
			return BundleVerificationKey{}, err
		}
		return BundleVerificationKey{PublicKey: publicKey, KeyID: BundleKeyFingerprint(publicKey)}, nil
	}
	if secret := strings.TrimSpace(fallbackSecret); secret != "" {
		sum := sha256.Sum256([]byte(secret))
		privateKey := ed25519.NewKeyFromSeed(sum[:])
		publicKey := privateKey.Public().(ed25519.PublicKey)
		return BundleVerificationKey{PublicKey: publicKey, KeyID: BundleKeyFingerprint(publicKey)}, nil
	}
	return BundleVerificationKey{}, fmt.Errorf("EVIDENCE_BUNDLE_SIGNING_PUBLIC_KEY, embedded manifest public_key, or CONSOLE_JWT_SECRET is required to verify evidence bundles")
}

func SignCanonicalPayload(canon []byte, privateKey ed25519.PrivateKey) string {
	if len(canon) == 0 || len(privateKey) != ed25519.PrivateKeySize {
		return ""
	}
	return base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canon))
}

func VerifyCanonicalPayload(canon []byte, publicKey ed25519.PublicKey, signature string) error {
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid ed25519 public key length")
	}
	rawSignature, err := decodeBase64Flexible(signature)
	if err != nil {
		return fmt.Errorf("decode manifest signature: %w", err)
	}
	if len(rawSignature) != ed25519.SignatureSize {
		return fmt.Errorf("invalid ed25519 signature length")
	}
	if !ed25519.Verify(publicKey, canon, rawSignature) {
		return fmt.Errorf("ed25519 signature verification failed")
	}
	return nil
}

func BundleKeyFingerprint(publicKey ed25519.PublicKey) string {
	return "sha256:" + HashBytes(publicKey)
}

func newBundleSigningKey(privateKey ed25519.PrivateKey, publicKey ed25519.PublicKey) BundleSigningKey {
	return BundleSigningKey{
		PublicKey:       publicKey,
		PrivateKey:      privateKey,
		PublicKeyBase64: base64.StdEncoding.EncodeToString(publicKey),
		KeyID:           BundleKeyFingerprint(publicKey),
	}
}

func decodeEd25519PrivateKey(value string) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	raw, err := decodeBase64Flexible(value)
	if err != nil {
		return nil, nil, fmt.Errorf("decode ed25519 private key: %w", err)
	}
	switch len(raw) {
	case ed25519.SeedSize:
		privateKey := ed25519.NewKeyFromSeed(raw)
		return privateKey, privateKey.Public().(ed25519.PublicKey), nil
	case ed25519.PrivateKeySize:
		privateKey := ed25519.PrivateKey(raw)
		return privateKey, privateKey.Public().(ed25519.PublicKey), nil
	default:
		return nil, nil, fmt.Errorf("ed25519 private key must be base64-encoded 32-byte seed or 64-byte private key")
	}
}

func decodeEd25519PublicKey(value string) (ed25519.PublicKey, error) {
	raw, err := decodeBase64Flexible(value)
	if err != nil {
		return nil, fmt.Errorf("decode ed25519 public key: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("ed25519 public key must be base64-encoded 32 bytes")
	}
	return ed25519.PublicKey(raw), nil
}

func decodeBase64Flexible(value string) ([]byte, error) {
	trimmed := strings.TrimSpace(value)
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}
	var lastErr error
	for _, encoding := range encodings {
		decoded, err := encoding.DecodeString(trimmed)
		if err == nil {
			return decoded, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("invalid base64 value")
	}
	return nil, lastErr
}
