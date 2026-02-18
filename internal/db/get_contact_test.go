package db

import (
	"context"
	"testing"
	"time"
)

func TestGetContactByIdentifier(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	seed, err := UpsertContact(ctx, conn, UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "alice@s.whatsapp.net",
		Name:          "Alice",
		Phone:         "11111",
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("upsert contact: %v", err)
	}

	gotByID, err := GetContact(ctx, conn, seed.ID)
	if err != nil {
		t.Fatalf("get contact by id: %v", err)
	}
	if gotByID.ID != seed.ID {
		t.Fatalf("expected id %q, got %q", seed.ID, gotByID.ID)
	}

	gotByJID, err := GetContact(ctx, conn, "alice@s.whatsapp.net")
	if err != nil {
		t.Fatalf("get contact by whatsapp id: %v", err)
	}
	if gotByJID.ID != seed.ID {
		t.Fatalf("expected same contact id %q, got %q", seed.ID, gotByJID.ID)
	}
	if gotByJID.Name != "Alice" {
		t.Fatalf("expected name Alice, got %q", gotByJID.Name)
	}
}
