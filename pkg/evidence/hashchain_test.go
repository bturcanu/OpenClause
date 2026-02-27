package evidence

import (
	"testing"
)

func TestChainHash_Deterministic(t *testing.T) {
	prev := "abc123"
	payload := []byte(`{"action":"test"}`)
	result := []byte(`{"status":"ok"}`)

	h1 := ChainHash(prev, payload, result)
	h2 := ChainHash(prev, payload, result)

	if h1 != h2 {
		t.Errorf("non-deterministic chain hash: %s != %s", h1, h2)
	}
}

func TestChainHash_DiffersWithDiffInput(t *testing.T) {
	h1 := ChainHash("", []byte("a"), nil)
	h2 := ChainHash("", []byte("b"), nil)

	if h1 == h2 {
		t.Error("different payloads should produce different hashes")
	}
}

func TestChainHash_DomainSeparation(t *testing.T) {
	// With length-prefixed fields, "ab"+"cd" != "a"+"bcd".
	h1 := ChainHash("ab", []byte("cd"), nil)
	h2 := ChainHash("a", []byte("bcd"), nil)

	if h1 == h2 {
		t.Error("domain separation failed: Hash(ab,cd) == Hash(a,bcd)")
	}
}

func TestChainHash_EmptyInputs(t *testing.T) {
	h := ChainHash("", []byte{}, nil)
	if h == "" {
		t.Error("expected non-empty hash for empty inputs")
	}
	if len(h) != 64 {
		t.Errorf("expected SHA-256 hex length 64, got %d", len(h))
	}
}

func TestChainHash_NilResult(t *testing.T) {
	h1 := ChainHash("prev", []byte("payload"), nil)
	h2 := ChainHash("prev", []byte("payload"), []byte("result"))

	if h1 == h2 {
		t.Error("nil result should differ from non-nil result")
	}
}

func TestVerifyChain_Valid(t *testing.T) {
	ev1Payload := []byte(`{"event":1}`)
	ev1Hash := ChainHash("", ev1Payload, nil)

	ev2Payload := []byte(`{"event":2}`)
	ev2Result := []byte(`{"ok":true}`)
	ev2Hash := ChainHash(ev1Hash, ev2Payload, ev2Result)

	events := []ChainEvent{
		{EventID: "e1", PrevHash: "", Hash: ev1Hash, CanonPayload: ev1Payload},
		{EventID: "e2", PrevHash: ev1Hash, Hash: ev2Hash, CanonPayload: ev2Payload, CanonResult: ev2Result},
	}

	if err := VerifyChain(events); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyChain_Broken(t *testing.T) {
	ev1Payload := []byte(`{"event":1}`)
	ev1Hash := ChainHash("", ev1Payload, nil)

	events := []ChainEvent{
		{EventID: "e1", PrevHash: "", Hash: ev1Hash, CanonPayload: ev1Payload},
		{EventID: "e2", PrevHash: ev1Hash, Hash: "tampered-hash", CanonPayload: []byte(`{"event":2}`)},
	}

	if err := VerifyChain(events); err == nil {
		t.Fatal("expected chain verification to fail")
	}
}

func TestVerifyChain_Empty(t *testing.T) {
	if err := VerifyChain(nil); err != nil {
		t.Fatalf("empty chain should verify: %v", err)
	}
}

func TestVerifyChain_SingleEvent(t *testing.T) {
	payload := []byte(`{"single":true}`)
	h := ChainHash("", payload, nil)
	events := []ChainEvent{{EventID: "e1", Hash: h, CanonPayload: payload}}
	if err := VerifyChain(events); err != nil {
		t.Fatalf("single event chain should verify: %v", err)
	}
}

func TestVerifyChainFrom_WithStartingHash(t *testing.T) {
	ev1Payload := []byte(`{"event":1}`)
	ev1Hash := ChainHash("", ev1Payload, nil)
	ev2Payload := []byte(`{"event":2}`)
	ev2Hash := ChainHash(ev1Hash, ev2Payload, nil)
	events := []ChainEvent{{EventID: "e2", PrevHash: ev1Hash, Hash: ev2Hash, CanonPayload: ev2Payload}}
	if err := VerifyChainFrom(ev1Hash, events); err != nil {
		t.Fatalf("chain from starting hash should verify: %v", err)
	}
}
