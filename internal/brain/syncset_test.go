package brain

import (
	"sync"
	"testing"
)

func TestSyncSetTrySetFirstCallSucceeds(t *testing.T) {
	s := NewSyncSet()
	if !s.TrySet("a") {
		t.Fatal("first TrySet should return true")
	}
}

func TestSyncSetTrySetDuplicateFails(t *testing.T) {
	s := NewSyncSet()
	s.TrySet("a")
	if s.TrySet("a") {
		t.Fatal("duplicate TrySet should return false")
	}
}

func TestSyncSetDeleteAllowsReuse(t *testing.T) {
	s := NewSyncSet()
	s.TrySet("a")
	s.Delete("a")
	if !s.TrySet("a") {
		t.Fatal("TrySet after Delete should return true")
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
