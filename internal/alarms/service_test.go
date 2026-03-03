package alarms

import (
	"context"
	"strings"
	"testing"
)

func TestNewServiceStoresDB(t *testing.T) {
	svc := New(nil)
	if svc == nil {
		t.Fatalf("expected non-nil service")
	}
	if svc.db != nil {
		t.Fatalf("expected nil db when initialized with nil")
	}
}

func TestCreateAlarmRejectsInvalidTimestamp(t *testing.T) {
	svc := New(nil)

	_, err := svc.CreateAlarm(context.Background(), "kevin", "", "wake up", "", "", "", "not-a-time")
	if err == nil {
		t.Fatalf("expected parse error")
	}

	if !strings.Contains(err.Error(), "parse next_fire_at") {
		t.Fatalf("expected parse next_fire_at error, got %v", err)
	}
}
