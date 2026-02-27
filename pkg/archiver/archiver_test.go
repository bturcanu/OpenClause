package archiver

import (
	"context"
	"testing"
	"time"

	"github.com/bturcanu/OpenClause/pkg/evidence"
)

type fakeStore struct {
	checkpoint time.Time
	hash       string
	events     []evidence.ChainEvent
}

func (f *fakeStore) GetArchiveCheckpoint(context.Context, string) (time.Time, string, int64, error) {
	return f.checkpoint, f.hash, 0, nil
}

func (f *fakeStore) GetChainEvents(context.Context, string, int64) ([]evidence.ChainEvent, error) {
	return f.events, nil
}

func (f *fakeStore) UpsertArchiveCheckpoint(_ context.Context, _ string, ts time.Time, h string, _ int64) error {
	f.checkpoint = ts
	f.hash = h
	return nil
}

func (f *fakeStore) ListTenantIDs(context.Context) ([]string, error) { return []string{"tenant1"}, nil }

type fakeUploader struct {
	key  string
	body []byte
}

func (f *fakeUploader) Upload(_ context.Context, key string, body []byte) error {
	f.key = key
	f.body = body
	return nil
}

func TestArchiveTenantBuildsBundleAndAdvancesCheckpoint(t *testing.T) {
	ev1 := evidence.ChainEvent{
		EventSeq:     1,
		EventID:      "e1",
		PrevHash:     "",
		CanonPayload: []byte(`{"a":1}`),
		CanonResult:  []byte(`{"ok":true}`),
		ReceivedAt:   time.Now().UTC().Add(-2 * time.Minute),
	}
	ev1.Hash = evidence.ChainHash("", ev1.CanonPayload, ev1.CanonResult)
	ev2 := evidence.ChainEvent{
		EventSeq:     2,
		EventID:      "e2",
		PrevHash:     ev1.Hash,
		CanonPayload: []byte(`{"a":2}`),
		CanonResult:  []byte(`{"ok":true}`),
		ReceivedAt:   time.Now().UTC().Add(-1 * time.Minute),
	}
	ev2.Hash = evidence.ChainHash(ev1.Hash, ev2.CanonPayload, ev2.CanonResult)

	store := &fakeStore{events: []evidence.ChainEvent{ev1, ev2}}
	up := &fakeUploader{}
	s := New(store, up)

	key, err := s.ArchiveTenant(context.Background(), "tenant1")
	if err != nil {
		t.Fatalf("archive tenant: %v", err)
	}
	if key == "" || up.key == "" || len(up.body) == 0 {
		t.Fatalf("expected uploaded bundle")
	}
	if store.hash != ev2.Hash {
		t.Fatalf("expected checkpoint hash %s got %s", ev2.Hash, store.hash)
	}
}
