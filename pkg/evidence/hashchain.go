package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// ChainHash computes the next hash in the per-tenant chain.
//
//	hash = SHA-256( prevHash || canonicalPayload || canonicalResult )
func ChainHash(prevHash string, canonPayload []byte, canonResult []byte) string {
	h := sha256.New()
	h.Write([]byte(prevHash))
	h.Write(canonPayload)
	if canonResult != nil {
		h.Write(canonResult)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// VerifyChain walks a sequence of events and verifies each hash link.
func VerifyChain(events []ChainEvent) error {
	prev := ""
	for i, ev := range events {
		expected := ChainHash(prev, ev.CanonPayload, ev.CanonResult)
		if ev.Hash != expected {
			return fmt.Errorf("chain broken at index %d (event %s): expected %s, got %s",
				i, ev.EventID, expected, ev.Hash)
		}
		prev = ev.Hash
	}
	return nil
}

// ChainEvent is the minimal shape needed for verification.
type ChainEvent struct {
	EventID      string
	Hash         string
	CanonPayload []byte
	CanonResult  []byte
}
