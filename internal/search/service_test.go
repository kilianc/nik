package search

import "testing"

func TestNewServiceStoresDB(t *testing.T) {
	svc := NewService(nil)
	if svc == nil {
		t.Fatalf("expected non-nil service")
	}
	if svc.db != nil {
		t.Fatalf("expected nil db when initialized with nil")
	}
}
