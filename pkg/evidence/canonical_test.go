package evidence

import (
	"testing"
)

func TestCanonicalJSON_StableKeyOrder(t *testing.T) {
	// Two maps with same keys in different insertion order
	a := map[string]any{"z": 1, "a": 2, "m": 3}
	b := map[string]any{"a": 2, "m": 3, "z": 1}

	ca, err := CanonicalJSON(a)
	if err != nil {
		t.Fatalf("canonical a: %v", err)
	}
	cb, err := CanonicalJSON(b)
	if err != nil {
		t.Fatalf("canonical b: %v", err)
	}

	if string(ca) != string(cb) {
		t.Errorf("canonical mismatch:\n  a=%s\n  b=%s", ca, cb)
	}

	expected := `{"a":2,"m":3,"z":1}`
	if string(ca) != expected {
		t.Errorf("expected %s, got %s", expected, ca)
	}
}

func TestCanonicalJSON_NestedObjects(t *testing.T) {
	obj := map[string]any{
		"b": map[string]any{"y": 2, "x": 1},
		"a": "hello",
	}

	canon, err := CanonicalJSON(obj)
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}

	expected := `{"a":"hello","b":{"x":1,"y":2}}`
	if string(canon) != expected {
		t.Errorf("expected %s, got %s", expected, canon)
	}
}

func TestCanonicalJSON_Array(t *testing.T) {
	obj := map[string]any{
		"items": []any{
			map[string]any{"b": 2, "a": 1},
			map[string]any{"d": 4, "c": 3},
		},
	}

	canon, err := CanonicalJSON(obj)
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}

	expected := `{"items":[{"a":1,"b":2},{"c":3,"d":4}]}`
	if string(canon) != expected {
		t.Errorf("expected %s, got %s", expected, canon)
	}
}

func TestHashPayload_Deterministic(t *testing.T) {
	obj := map[string]any{"foo": "bar", "num": 42}

	_, h1, err := HashPayload(obj)
	if err != nil {
		t.Fatalf("hash 1: %v", err)
	}
	_, h2, err := HashPayload(obj)
	if err != nil {
		t.Fatalf("hash 2: %v", err)
	}

	if h1 != h2 {
		t.Errorf("non-deterministic hash: %s != %s", h1, h2)
	}
	if len(h1) != 64 {
		t.Errorf("expected SHA-256 hex length 64, got %d", len(h1))
	}
}
