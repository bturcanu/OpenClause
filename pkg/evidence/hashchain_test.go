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

func TestVerifyChain_Valid(t *testing.T) {
	ev1Payload := []byte(`{"event":1}`)
	ev1Hash := ChainHash("", ev1Payload, nil)

	ev2Payload := []byte(`{"event":2}`)
	ev2Result := []byte(`{"ok":true}`)
	ev2Hash := ChainHash(ev1Hash, ev2Payload, ev2Result)

	events := []ChainEvent{
		{EventID: "e1", Hash: ev1Hash, CanonPayload: ev1Payload},
		{EventID: "e2", Hash: ev2Hash, CanonPayload: ev2Payload, CanonResult: ev2Result},
	}

	if err := VerifyChain(events); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyChain_Broken(t *testing.T) {
	ev1Payload := []byte(`{"event":1}`)
	ev1Hash := ChainHash("", ev1Payload, nil)

	events := []ChainEvent{
		{EventID: "e1", Hash: ev1Hash, CanonPayload: ev1Payload},
		{EventID: "e2", Hash: "tampered-hash", CanonPayload: []byte(`{"event":2}`)},
	}

	if err := VerifyChain(events); err == nil {
		t.Fatal("expected chain verification to fail")
	}
}
