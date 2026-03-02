package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DBKeyStore validates API keys against the database.
// It first looks up by key_prefix, then verifies the full SHA-256 hash.
type DBKeyStore struct {
	pool *pgxpool.Pool
}

func NewDBKeyStore(pool *pgxpool.Pool) *DBKeyStore {
	return &DBKeyStore{pool: pool}
}

// Lookup validates an API key against the database.
// Returns the tenant ID if the key is valid and active.
func (db *DBKeyStore) Lookup(apiKey string) (tenantID string, ok bool) {
	h := sha256.Sum256([]byte(apiKey))
	computedHash := hex.EncodeToString(h[:])

	prefix := apiKey
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := db.pool.Query(ctx,
		`SELECT ak.id, ak.tenant_id, ak.key_hash
		 FROM api_keys ak
		 JOIN tenants t ON t.id = ak.tenant_id AND t.status = 'active'
		 WHERE ak.key_prefix = $1 AND ak.status = 'active'`,
		prefix,
	)
	if err != nil {
		return "", false
	}
	defer rows.Close()

	for rows.Next() {
		var id, rowTenant, rowHash string
		if err := rows.Scan(&id, &rowTenant, &rowHash); err != nil {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(rowHash), []byte(computedHash)) == 1 {
			go db.touchLastUsed(id)
			return rowTenant, true
		}
	}

	return "", false
}

func (db *DBKeyStore) touchLastUsed(keyID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, _ = db.pool.Exec(ctx,
		`UPDATE api_keys SET last_used_at = $1 WHERE id = $2`,
		time.Now().UTC(), keyID,
	)
}

// CompositeKeyStore tries multiple KeyLookup stores in order.
type CompositeKeyStore struct {
	stores []KeyLookup
}

func NewCompositeKeyStore(stores ...KeyLookup) *CompositeKeyStore {
	return &CompositeKeyStore{stores: stores}
}

func (c *CompositeKeyStore) Lookup(apiKey string) (tenantID string, ok bool) {
	for _, s := range c.stores {
		if tenantID, ok = s.Lookup(apiKey); ok {
			return
		}
	}
	return "", false
}
