package db

import (
	"context"
	"testing"
)

func TestNikContactEnsure(t *testing.T) {
	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	nikID := "00000000-0000-7000-8000-000000000001"

	err = NikContactEnsure(ctx, conn, nikID)
	if err != nil {
		t.Fatalf("first ensure: %v", err)
	}

	contact, err := ContactGet(ctx, conn, nikID)
	if err != nil {
		t.Fatalf("get nik contact: %v", err)
	}

	if contact.Name != "nik" {
		t.Errorf("expected name nik, got %q", contact.Name)
	}

	err = NikContactEnsure(ctx, conn, nikID)
	if err != nil {
		t.Fatalf("second ensure (idempotent): %v", err)
	}
}
