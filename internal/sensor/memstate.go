package sensor

// MemState stores last-seen hashes in memory only.
// It is used to avoid re-emitting unchanged resources within a single process lifetime.
type MemState struct {
	items map[string]string
}

func NewMemState() *MemState {
	return &MemState{items: map[string]string{}}
}

func (s *MemState) Get(key string) (string, bool) {
	if s == nil || s.items == nil {
		return "", false
	}
	v, ok := s.items[key]
	return v, ok
}

func (s *MemState) Set(key, hash string) {
	if s == nil {
		return
	}
	if s.items == nil {
		s.items = map[string]string{}
	}
	s.items[key] = hash
}

