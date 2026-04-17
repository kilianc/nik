package db

import (
	"context"
	"testing"
)

func TestSettingSetAndGet(t *testing.T) {
	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()

	err = SettingSet(ctx, conn, "genesis_completed_at", "2026-04-01T12:00:00.000Z")
	if err != nil {
		t.Fatalf("set setting: %v", err)
	}

	s, err := SettingGet(ctx, conn, "genesis_completed_at")
	if err != nil {
		t.Fatalf("get setting: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil setting")
	}
	if s.Key != "genesis_completed_at" {
		t.Errorf("expected key genesis_completed_at, got %s", s.Key)
	}
	if s.Value != "2026-04-01T12:00:00.000Z" {
		t.Errorf("expected value 2026-04-01T12:00:00.000Z, got %s", s.Value)
	}
	if s.CreatedAt.IsZero() {
		t.Error("expected non-zero created_at")
	}

	err = SettingSet(ctx, conn, "genesis_completed_at", "2026-04-02T00:00:00.000Z")
	if err != nil {
		t.Fatalf("upsert setting: %v", err)
	}

	s, err = SettingGet(ctx, conn, "genesis_completed_at")
	if err != nil {
		t.Fatalf("get setting after upsert: %v", err)
	}
	if s.Value != "2026-04-02T00:00:00.000Z" {
		t.Errorf("expected value2 after upsert, got %s", s.Value)
	}
}

func TestSettingGetNotFound(t *testing.T) {
	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()

	s, err := SettingGet(ctx, conn, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s != nil {
		t.Fatal("expected nil for nonexistent setting")
	}
}
