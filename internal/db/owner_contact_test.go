package db

import (
	"context"
	"testing"
)

func TestOwnerContactEnsure(t *testing.T) {
	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()

	err = OwnerContactEnsure(ctx, conn)
	if err != nil {
		t.Fatalf("first ensure: %v", err)
	}

	contact, err := ContactGet(ctx, conn, OwnerContactID)
	if err != nil {
		t.Fatalf("get owner contact: %v", err)
	}

	if contact.Name != "owner" {
		t.Errorf("expected name owner, got %q", contact.Name)
	}

	err = OwnerContactEnsure(ctx, conn)
	if err != nil {
		t.Fatalf("second ensure (idempotent): %v", err)
	}
}
