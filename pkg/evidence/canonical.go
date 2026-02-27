// Package evidence provides canonicalization, hashing, and audit-evidence storage.
package evidence

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// CanonicalJSON produces a stable byte representation of v.
// Object keys are sorted lexicographically; no extraneous whitespace.
func CanonicalJSON(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("canonical json marshal: %w", err)
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var generic any
	if err := dec.Decode(&generic); err != nil {
		return nil, fmt.Errorf("canonical json unmarshal: %w", err)
	}

	sorted := sortKeys(generic)
	out, err := json.Marshal(sorted)
	if err != nil {
		return nil, fmt.Errorf("canonical json re-marshal: %w", err)
	}
	return out, nil
}

// HashBytes returns the hex-encoded SHA-256 of data.
func HashBytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// HashPayload creates a canonical JSON representation and returns its SHA-256.
func HashPayload(v any) (canon []byte, hash string, err error) {
	canon, err = CanonicalJSON(v)
	if err != nil {
		return nil, "", err
	}
	return canon, HashBytes(canon), nil
}

// sortKeys recursively sorts map keys for deterministic serialization.
func sortKeys(v any) any {
	switch val := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		sorted := make(orderedMap, 0, len(val))
		for _, k := range keys {
			sorted = append(sorted, kv{Key: k, Value: sortKeys(val[k])})
		}
		return sorted

	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = sortKeys(item)
		}
		return out

	default:
		return val
	}
}

// orderedMap preserves insertion order during JSON marshalling.
type orderedMap []kv

type kv struct {
	Key   string
	Value any
}

func (om orderedMap) MarshalJSON() ([]byte, error) {
	buf := []byte{'{'}
	for i, item := range om {
		if i > 0 {
			buf = append(buf, ',')
		}
		key, err := json.Marshal(item.Key)
		if err != nil {
			return nil, err
		}
		val, err := json.Marshal(item.Value)
		if err != nil {
			return nil, err
		}
		buf = append(buf, key...)
		buf = append(buf, ':')
		buf = append(buf, val...)
	}
	buf = append(buf, '}')
	return buf, nil
}
