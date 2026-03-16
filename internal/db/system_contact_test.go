package db

import (
	"context"
	"testing"
)

func TestEnsureSystemContactIsIdempotent(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	err = EnsureSystemContact(ctx, conn)
	if err != nil {
		t.Fatalf("first ensure: %v", err)
	}

	err = EnsureSystemContact(ctx, conn)
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}

	contact, err := GetContact(ctx, conn, SystemContactID)
	if err != nil {
		t.Fatalf("get system contact: %v", err)
	}

	if contact.ID != SystemContactID {
		t.Fatalf("expected system contact id %q, got %q", SystemContactID, contact.ID)
	}
	if contact.Name != "system" {
		t.Fatalf("expected system contact name %q, got %q", "system", contact.Name)
	}
}
