package reconcile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// StableHash produces a deterministic hash for arbitrary decoded YAML/JSON-like data.
func StableHash(v any) (string, error) {
	n := normalize(v)
	b, err := json.Marshal(n)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func normalize(v any) any {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]any, 0, len(keys)*2)
		for _, k := range keys {
			out = append(out, k, normalize(t[k]))
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i := range t {
			out[i] = normalize(t[i])
		}
		return out
	default:
		return t
	}
}
