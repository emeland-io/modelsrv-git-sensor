package reconcile

import "strings"

// State stores last-seen hashes in memory only.
// It is used to avoid re-emitting unchanged resources within a single process lifetime.
// It also tracks which resource keys belong to each file path so that Delete events
// can be emitted when documents are removed from a file or a file disappears entirely.
type State struct {
	items    map[string]string              // "ResourceType/UUID" -> hash
	pathKeys map[string]map[string]struct{} // filePath -> set of "ResourceType/UUID" keys
}

func NewState() *State {
	return &State{
		items:    map[string]string{},
		pathKeys: map[string]map[string]struct{}{},
	}
}

func (s *State) Get(key string) (string, bool) {
	if s == nil || s.items == nil {
		return "", false
	}
	v, ok := s.items[key]
	return v, ok
}

func (s *State) Set(key, hash string) {
	if s == nil {
		return
	}
	if s.items == nil {
		s.items = map[string]string{}
	}
	s.items[key] = hash
}

// DeleteKey removes a single key from the hash store and from any path association.
func (s *State) DeleteKey(key string) {
	if s == nil {
		return
	}
	delete(s.items, key)
	for _, keys := range s.pathKeys {
		delete(keys, key)
	}
}

// KeysForPath returns the set of keys last known for the given file path.
func (s *State) KeysForPath(path string) []string {
	if s == nil || s.pathKeys == nil {
		return nil
	}
	m := s.pathKeys[path]
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// SetKeysForPath replaces the key set for a path with the provided seen map.
// Keys that are no longer in seenKeys are removed from the hash store.
func (s *State) SetKeysForPath(path string, seenKeys map[string]struct{}) {
	if s == nil {
		return
	}
	if s.pathKeys == nil {
		s.pathKeys = map[string]map[string]struct{}{}
	}
	old := s.pathKeys[path]
	for k := range old {
		if _, still := seenKeys[k]; !still {
			delete(s.items, k)
		}
	}
	if len(seenKeys) == 0 {
		delete(s.pathKeys, path)
		return
	}
	newSet := make(map[string]struct{}, len(seenKeys))
	for k := range seenKeys {
		newSet[k] = struct{}{}
	}
	s.pathKeys[path] = newSet
}

// PurgePath removes the path and all its keys from the state, returning the
// former key list so callers can emit Delete events.
func (s *State) PurgePath(path string) []string {
	if s == nil || s.pathKeys == nil {
		return nil
	}
	m := s.pathKeys[path]
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
		delete(s.items, k)
	}
	delete(s.pathKeys, path)
	return keys
}

// PathsUnderDir returns all tracked file paths that are directly or transitively
// under dir (i.e. have dir as a path prefix).
func (s *State) PathsUnderDir(dir string) []string {
	if s == nil || s.pathKeys == nil {
		return nil
	}
	prefix := dir
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	var out []string
	for p := range s.pathKeys {
		if strings.HasPrefix(p, prefix) {
			out = append(out, p)
		}
	}
	return out
}
