package console

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// JWTConfig holds JWT signing configuration.
type JWTConfig struct {
	Secret      string
	Issuer      string
	ExpiryHours int
}

// JWTClaims represents the claims in a JWT token.
type JWTClaims struct {
	Sub    string   `json:"sub"`
	SID    string   `json:"sid,omitempty"`
	Email  string   `json:"email"`
	Name   string   `json:"name"`
	Roles  []string `json:"roles"`
	Tenant string   `json:"tenant,omitempty"`
	Iss    string   `json:"iss"`
	Iat    int64    `json:"iat"`
	Exp    int64    `json:"exp"`
}

// GenerateToken creates a new HS256 JWT token.
func GenerateToken(cfg JWTConfig, claims JWTClaims) (string, error) {
	claims.Iss = cfg.Issuer
	claims.Iat = time.Now().Unix()
	claims.Exp = time.Now().Add(time.Duration(cfg.ExpiryHours) * time.Hour).Unix()

	header := []byte(`{"alg":"HS256","typ":"JWT"}`)
	headerEnc := base64.RawURLEncoding.EncodeToString(header)

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}
	payloadEnc := base64.RawURLEncoding.EncodeToString(payload)

	signingInput := headerEnc + "." + payloadEnc

	mac := hmac.New(sha256.New, []byte(cfg.Secret))
	mac.Write([]byte(signingInput))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return signingInput + "." + sig, nil
}

// ValidateToken parses and validates a JWT token.
func ValidateToken(cfg JWTConfig, tokenStr string) (*JWTClaims, error) {
	parts := strings.SplitN(tokenStr, ".", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	// Verify signature
	mac := hmac.New(sha256.New, []byte(cfg.Secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	expectedSig := mac.Sum(nil)

	actualSig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode signature: %w", err)
	}

	if !hmac.Equal(actualSig, expectedSig) {
		return nil, fmt.Errorf("invalid signature")
	}

	// Decode claims
	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}

	var claims JWTClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, fmt.Errorf("unmarshal claims: %w", err)
	}

	if time.Now().Unix() > claims.Exp {
		return nil, fmt.Errorf("token expired")
	}

	if claims.Iss != cfg.Issuer {
		return nil, fmt.Errorf("invalid issuer: got %q, want %q", claims.Iss, cfg.Issuer)
	}

	return &claims, nil
}
