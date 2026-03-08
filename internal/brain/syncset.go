package brain

import "sync"

type SyncSet struct {
	mu sync.Mutex
	m  map[string]struct{}
}

func NewSyncSet() *SyncSet {
	return &SyncSet{m: make(map[string]struct{})}
}

func (s *SyncSet) TrySet(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.m[key]; ok {
		return false
	}

	s.m[key] = struct{}{}
	return true
}

func (s *SyncSet) Delete(key string) {
	s.mu.Lock()
	delete(s.m, key)
	s.mu.Unlock()
}
