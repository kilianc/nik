package shell

import (
	"context"
	"testing"
)

func TestNewServiceStoresFields(t *testing.T) {
	svc := NewService(nil, "/tmp/test")
	if svc.conn != nil {
		t.Fatalf("expected nil conn")
	}
	if svc.home != "/tmp/test" {
		t.Fatalf("expected home '/tmp/test', got %q", svc.home)
	}
}

func TestCheckSessionsNilConnNoops(t *testing.T) {
	svc := NewService(nil, "/tmp/test")
	svc.CheckSessions(context.Background())
}
