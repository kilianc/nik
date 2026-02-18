package db

import (
	"context"
	"testing"
	"time"
)

func TestListContactsSupportsLimitOffset(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	_, err = UpsertContact(ctx, conn, UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "one@s.whatsapp.net",
		Name:          "One",
		Phone:         "1",
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("seed contact one: %v", err)
	}

	_, err = UpsertContact(ctx, conn, UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "two@s.whatsapp.net",
		Name:          "Two",
		Phone:         "2",
		LastMessageAt: time.Now().Add(time.Second),
	})
	if err != nil {
		t.Fatalf("seed contact two: %v", err)
	}

	pageOne, err := ListContacts(ctx, conn, 1, 0)
	if err != nil {
		t.Fatalf("list contacts page one: %v", err)
	}
	if len(pageOne) != 1 {
		t.Fatalf("expected 1 contact in first page, got %d", len(pageOne))
	}

	pageTwo, err := ListContacts(ctx, conn, 1, 1)
	if err != nil {
		t.Fatalf("list contacts page two: %v", err)
	}
	if len(pageTwo) != 1 {
		t.Fatalf("expected 1 contact in second page, got %d", len(pageTwo))
	}

	if pageOne[0].ID == pageTwo[0].ID {
		t.Fatalf("expected different contacts across pages, got duplicate id %q", pageOne[0].ID)
	}
}
