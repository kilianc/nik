package brain

import "sync"

type SyncSet struct {
	mu sync.Mutex
	m  map[string]struct{}
}

func NewSyncSet() *SyncSet {
	return &SyncSet{m: make(map[string]struct{})}
}

func (s *SyncSet) Has(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.m[key]
	return ok
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

func (s *SyncSet) Set(key string) {
	s.mu.Lock()
	s.m[key] = struct{}{}
	s.mu.Unlock()
}

func (s *SyncSet) Delete(key string) {
	s.mu.Lock()
	delete(s.m, key)
	s.mu.Unlock()
}
