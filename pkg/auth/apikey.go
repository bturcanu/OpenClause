package auth

import (
	"strings"
	"sync"
)

// KeyStore maps API keys to tenant IDs. Thread-safe.
type KeyStore struct {
	mu   sync.RWMutex
	keys map[string]string // apiKey → tenantID
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
			ks.keys[key] = tenant
		}
	}
	return ks
}

// Lookup returns the tenant ID for a given API key.
func (ks *KeyStore) Lookup(apiKey string) (tenantID string, ok bool) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	tenantID, ok = ks.keys[apiKey]
	return
}
