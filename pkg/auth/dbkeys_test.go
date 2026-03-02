package auth

import "testing"

type mockKeyLookup struct {
	tenantID string
	ok       bool
	called   int
}

func (m *mockKeyLookup) Lookup(_ string) (string, bool) {
	m.called++
	return m.tenantID, m.ok
}

func TestCompositeKeyStoreLookup_FirstMatch(t *testing.T) {
	first := &mockKeyLookup{tenantID: "tenant-1", ok: true}
	second := &mockKeyLookup{tenantID: "tenant-2", ok: true}
	store := NewCompositeKeyStore(first, second)

	tid, ok := store.Lookup("any-key")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if tid != "tenant-1" {
		t.Errorf("tenantID = %q, want %q", tid, "tenant-1")
	}
	if first.called != 1 {
		t.Errorf("first.called = %d, want 1", first.called)
	}
	if second.called != 0 {
		t.Errorf("second.called = %d, want 0 (should short-circuit)", second.called)
	}
}

func TestCompositeKeyStoreLookup_Fallthrough(t *testing.T) {
	first := &mockKeyLookup{tenantID: "", ok: false}
	second := &mockKeyLookup{tenantID: "tenant-2", ok: true}
	store := NewCompositeKeyStore(first, second)

	tid, ok := store.Lookup("any-key")
	if !ok {
		t.Fatal("expected ok=true from second store")
	}
	if tid != "tenant-2" {
		t.Errorf("tenantID = %q, want %q", tid, "tenant-2")
	}
	if first.called != 1 {
		t.Errorf("first.called = %d, want 1", first.called)
	}
	if second.called != 1 {
		t.Errorf("second.called = %d, want 1", second.called)
	}
}

func TestCompositeKeyStoreLookup_NoMatch(t *testing.T) {
	first := &mockKeyLookup{tenantID: "", ok: false}
	second := &mockKeyLookup{tenantID: "", ok: false}
	store := NewCompositeKeyStore(first, second)

	tid, ok := store.Lookup("any-key")
	if ok {
		t.Fatal("expected ok=false when no store matches")
	}
	if tid != "" {
		t.Errorf("tenantID = %q, want empty", tid)
	}
	if first.called != 1 {
		t.Errorf("first.called = %d, want 1", first.called)
	}
	if second.called != 1 {
		t.Errorf("second.called = %d, want 1", second.called)
	}
}
