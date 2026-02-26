package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
)

// KeyStore maps hashed API keys to tenant IDs. Thread-safe.
// Keys are stored as SHA-256 hashes to protect against memory dumps.
type KeyStore struct {
	mu   sync.RWMutex
	keys map[string]string // SHA-256(apiKey) → tenantID
}

// NewKeyStore creates a KeyStore from a comma-separated "tenant:key" string.
// Example: "tenant1:sk-abc,tenant2:sk-def"
func NewKeyStore(raw string) *KeyStore {
	ks := &KeyStore{keys: make(map[string]string)}
	if raw == "" {
		return ks
	}
	for _, pair := range strings.Split(raw, ",") {
		parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		if len(parts) == 2 {
			tenant := strings.TrimSpace(parts[0])
			key := strings.TrimSpace(parts[1])
			ks.keys[hashKey(key)] = tenant
		}
	}
	return ks
}

// Lookup returns the tenant ID for a given API key.
func (ks *KeyStore) Lookup(apiKey string) (tenantID string, ok bool) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	tenantID, ok = ks.keys[hashKey(apiKey)]
	return
}

func hashKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}
