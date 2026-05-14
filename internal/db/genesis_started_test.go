package db

import (
	"context"
	"testing"
	"time"
)

func TestGenesisStartedAtEnsureIsIdempotent(t *testing.T) {
	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()

	first, err := GenesisStartedAtEnsure(ctx, conn)
	if err != nil {
		t.Fatalf("ensure 1: %v", err)
	}

	time.Sleep(5 * time.Millisecond)

	second, err := GenesisStartedAtEnsure(ctx, conn)
	if err != nil {
		t.Fatalf("ensure 2: %v", err)
	}

	if !second.Equal(first) {
		t.Errorf("expected idempotent stamp; first=%v second=%v", first, second)
	}
}

func TestGenesisStartedAtEnsureRespectsPreExistingValue(t *testing.T) {
	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()

	pinned := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	if err := SettingSet(ctx, conn, GenesisStartedAtKey, ISO8601MS(pinned)); err != nil {
		t.Fatalf("seed %s: %v", GenesisStartedAtKey, err)
	}

	got, err := GenesisStartedAtEnsure(ctx, conn)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if !got.Equal(pinned) {
		t.Errorf("expected pre-existing stamp %v, got %v", pinned, got)
	}
}
