package db

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestUpsertContactWhatsAppInsertThenUpdate(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	t0 := time.Now().Add(-time.Minute)
	inserted, err := UpsertContact(ctx, conn, UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "wa-user@s.whatsapp.net",
		Name:          "Alice",
		Phone:         "11111",
		LastMessageAt: t0,
	})
	if err != nil {
		t.Fatalf("insert contact: %v", err)
	}

	t1 := time.Now()
	updated, err := UpsertContact(ctx, conn, UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "wa-user@s.whatsapp.net",
		Name:          "Alice A",
		Phone:         "22222",
		LastMessageAt: t1,
	})
	if err != nil {
		t.Fatalf("update existing contact: %v", err)
	}

	if inserted.ID != updated.ID {
		t.Fatalf("expected same contact id, got %q and %q", inserted.ID, updated.ID)
	}
	if !containsStringUpsertContact(updated.PhoneNumbers, "11111") || !containsStringUpsertContact(updated.PhoneNumbers, "22222") {
		t.Fatalf("expected both phones in %+v", updated.PhoneNumbers)
	}
	if !updated.LastMessageAt.Valid {
		t.Fatalf("expected valid last_message_at")
	}
	if !inserted.LastMessageAt.Valid {
		t.Fatalf("expected valid inserted last_message_at")
	}
	if updated.LastMessageAt.Time.Before(inserted.LastMessageAt.Time) {
		t.Fatalf("expected updated last_message_at, got %+v", updated.LastMessageAt)
	}
}

func TestUpsertContactSelfWhatsApp(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	_, err = UpsertContact(ctx, conn, UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "self@s.whatsapp.net",
		IsSelf:        true,
		SelfID:        "",
		LastMessageAt: time.Now(),
	})
	if err == nil {
		t.Fatalf("expected error for empty self contact id")
	}
	if !strings.Contains(err.Error(), "empty self contact id") {
		t.Fatalf("unexpected error: %v", err)
	}

	id := NewID()
	first, err := UpsertContact(ctx, conn, UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "self@s.whatsapp.net",
		IsSelf:        true,
		SelfID:        id,
		LastMessageAt: time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("upsert self contact first: %v", err)
	}

	second, err := UpsertContact(ctx, conn, UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "self2@s.whatsapp.net",
		IsSelf:        true,
		SelfID:        id,
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("upsert self contact second: %v", err)
	}

	if first.ID != second.ID {
		t.Fatalf("expected same self contact id, got %q and %q", first.ID, second.ID)
	}
	if second.Name != "nik" {
		t.Fatalf("expected name nik, got %q", second.Name)
	}
	if !containsStringUpsertContact(second.Nicknames, "nik") {
		t.Fatalf("expected nik nickname in %+v", second.Nicknames)
	}
	if !containsStringUpsertContact(second.WhatsappIDs, "self@s.whatsapp.net") || !containsStringUpsertContact(second.WhatsappIDs, "self2@s.whatsapp.net") {
		t.Fatalf("expected both whatsapp ids in %+v", second.WhatsappIDs)
	}
}

func TestUpsertContactSelfWhatsAppPreservesRenamedName(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	id := NewID()

	_, err = UpsertContact(ctx, conn, UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "self@s.whatsapp.net",
		IsSelf:        true,
		SelfID:        id,
		LastMessageAt: time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("initial self upsert: %v", err)
	}

	err = UpdateContactField(ctx, conn, id, "name", "Nikolai", nil)
	if err != nil {
		t.Fatalf("rename self contact: %v", err)
	}

	after, err := UpsertContact(ctx, conn, UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "self@s.whatsapp.net",
		IsSelf:        true,
		SelfID:        id,
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("re-upsert self contact: %v", err)
	}

	if after.Name != "Nikolai" {
		t.Fatalf("expected name to be preserved as %q, got %q", "Nikolai", after.Name)
	}
}

func containsStringUpsertContact(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}

	return false
}
