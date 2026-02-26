package evidence

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// ChainHash computes the next hash in the per-tenant chain.
// Each field is length-prefixed (8-byte big-endian) for domain separation,
// preventing ambiguity when concatenated (e.g., Hash("ab","cd") != Hash("a","bcd")).
func ChainHash(prevHash string, canonPayload []byte, canonResult []byte) string {
	h := sha256.New()
	writeField(h, []byte(prevHash))
	writeField(h, canonPayload)
	if canonResult != nil {
		writeField(h, canonResult)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// writeField writes a length-prefixed field to the hash.
func writeField(h interface{ Write([]byte) (int, error) }, data []byte) {
	var lenBuf [8]byte
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(data)))
	h.Write(lenBuf[:])
	h.Write(data)
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
