package brain

import (
	"sync"
	"testing"
)

func TestSyncSetTrySet(t *testing.T) {
	tests := []struct {
		name  string
		setup func(s *SyncSet)
		want  bool
	}{
		{"first call succeeds", func(s *SyncSet) {}, true},
		{"duplicate fails", func(s *SyncSet) { s.TrySet("a") }, false},
		{"after delete succeeds", func(s *SyncSet) { s.TrySet("a"); s.Delete("a") }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSyncSet()
			tt.setup(s)
			got := s.TrySet("a")
			if got != tt.want {
				t.Fatalf("TrySet = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSyncSetConcurrentAccess(t *testing.T) {
	s := NewSyncSet()
	var wg sync.WaitGroup

	wins := make(chan bool, 100)
	for i := range 100 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			wins <- s.TrySet("key")
		}(i)
	}
	wg.Wait()
	close(wins)

	successCount := 0
	for w := range wins {
		if w {
			successCount++
		}
	}
	if successCount != 1 {
		t.Fatalf("expected exactly 1 winner, got %d", successCount)
	}
}
