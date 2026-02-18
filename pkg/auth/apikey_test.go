package auth

import "testing"

func TestNewKeyStore(t *testing.T) {
	ks := NewKeyStore("tenant1:sk-abc,tenant2:sk-def")

	tests := []struct {
		key    string
		tenant string
		ok     bool
	}{
		{"sk-abc", "tenant1", true},
		{"sk-def", "tenant2", true},
		{"sk-unknown", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		tenant, ok := ks.Lookup(tt.key)
		if ok != tt.ok {
			t.Errorf("Lookup(%q) ok=%v, want %v", tt.key, ok, tt.ok)
		}
		if tenant != tt.tenant {
			t.Errorf("Lookup(%q) tenant=%q, want %q", tt.key, tenant, tt.tenant)
		}
	}
}

func TestNewKeyStore_Empty(t *testing.T) {
	ks := NewKeyStore("")
	if _, ok := ks.Lookup("anything"); ok {
		t.Error("empty store should not match")
	}
}

func TestNewKeyStore_Whitespace(t *testing.T) {
	ks := NewKeyStore(" tenant1 : sk-abc , tenant2 : sk-def ")
	if tenant, ok := ks.Lookup("sk-abc"); !ok || tenant != "tenant1" {
		t.Error("should handle whitespace in key pairs")
	}
}
