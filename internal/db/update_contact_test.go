package db

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestUpdateContactFieldUpdatesAllowedFields(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	seed, err := UpsertContact(ctx, conn, UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "update@s.whatsapp.net",
		Name:          "Updater",
		Phone:         "88888",
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	err = UpdateContactField(ctx, conn, seed.ID, "notes", "new notes", nil)
	if err != nil {
		t.Fatalf("update notes: %v", err)
	}

	err = UpdateContactField(ctx, conn, seed.ID, "nicknames", "", []string{"u1", "u2"})
	if err != nil {
		t.Fatalf("update nicknames: %v", err)
	}

	got, err := GetContact(ctx, conn, seed.ID)
	if err != nil {
		t.Fatalf("get contact: %v", err)
	}

	if !got.Notes.Valid || got.Notes.String != "new notes" {
		t.Fatalf("expected notes new notes, got %+v", got.Notes)
	}
	if len(got.Nicknames) != 2 || got.Nicknames[0] != "u1" || got.Nicknames[1] != "u2" {
		t.Fatalf("unexpected nicknames: %+v", got.Nicknames)
	}
}

func TestUpdateContactFieldRejectsUnknownField(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	err = UpdateContactField(ctx, conn, NewID(), "bad_field", "value", nil)
	if err == nil {
		t.Fatalf("expected error for unknown field")
	}
	if !strings.Contains(err.Error(), "unknown contact field") {
		t.Fatalf("unexpected error: %v", err)
	}
}
