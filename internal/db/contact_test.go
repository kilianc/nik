package db

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/id"
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
	if !containsStringContact(updated.PhoneNumbers, "11111") || !containsStringContact(updated.PhoneNumbers, "22222") {
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

	selfID := id.V7()
	first, err := UpsertContact(ctx, conn, UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "self@s.whatsapp.net",
		IsSelf:        true,
		SelfID:        selfID,
		LastMessageAt: time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("upsert self contact first: %v", err)
	}

	second, err := UpsertContact(ctx, conn, UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "self2@s.whatsapp.net",
		IsSelf:        true,
		SelfID:        selfID,
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
	if !containsStringContact(second.Nicknames, "nik") {
		t.Fatalf("expected nik nickname in %+v", second.Nicknames)
	}
	if !containsStringContact(second.WhatsappIDs, "self@s.whatsapp.net") || !containsStringContact(second.WhatsappIDs, "self2@s.whatsapp.net") {
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

	selfID := id.V7()

	_, err = UpsertContact(ctx, conn, UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "self@s.whatsapp.net",
		IsSelf:        true,
		SelfID:        selfID,
		LastMessageAt: time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("initial self upsert: %v", err)
	}

	err = UpdateContactField(ctx, conn, selfID, "name", "Nikolai", nil)
	if err != nil {
		t.Fatalf("rename self contact: %v", err)
	}

	after, err := UpsertContact(ctx, conn, UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "self@s.whatsapp.net",
		IsSelf:        true,
		SelfID:        selfID,
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("re-upsert self contact: %v", err)
	}

	if after.Name != "Nikolai" {
		t.Fatalf("expected name to be preserved as %q, got %q", "Nikolai", after.Name)
	}
}

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

	err = UpdateContactField(ctx, conn, id.V7(), "bad_field", "value", nil)
	if err == nil {
		t.Fatalf("expected error for unknown field")
	}
	if !strings.Contains(err.Error(), "unknown contact field") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddWhatsAppIDAppendsNewJID(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contact, err := UpsertContact(ctx, conn, UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "12345@s.whatsapp.net",
		Name:          "Alice",
		Phone:         "12345",
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("upsert contact: %v", err)
	}

	err = AddWhatsAppID(ctx, conn, contact.ID, "99999@lid", "")
	if err != nil {
		t.Fatalf("add whatsapp id: %v", err)
	}

	got, err := GetContact(ctx, conn, contact.ID)
	if err != nil {
		t.Fatalf("get contact: %v", err)
	}

	if len(got.WhatsappIDs) != 2 {
		t.Fatalf("expected 2 whatsapp ids, got %v", got.WhatsappIDs)
	}
	if got.WhatsappIDs[0] != "12345@s.whatsapp.net" || got.WhatsappIDs[1] != "99999@lid" {
		t.Fatalf("unexpected whatsapp ids: %v", got.WhatsappIDs)
	}
}

func TestAddWhatsAppIDSkipsDuplicate(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contact, err := UpsertContact(ctx, conn, UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "12345@s.whatsapp.net",
		Name:          "Alice",
		Phone:         "12345",
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("upsert contact: %v", err)
	}

	err = AddWhatsAppID(ctx, conn, contact.ID, "12345@s.whatsapp.net", "12345")
	if err != nil {
		t.Fatalf("add whatsapp id: %v", err)
	}

	got, err := GetContact(ctx, conn, contact.ID)
	if err != nil {
		t.Fatalf("get contact: %v", err)
	}

	if len(got.WhatsappIDs) != 1 {
		t.Fatalf("expected 1 whatsapp id (no duplicate), got %v", got.WhatsappIDs)
	}
}

func TestAddWhatsAppIDAlsoAddsPhone(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contact, err := UpsertContact(ctx, conn, UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "99999@lid",
		Name:          "",
		Phone:         "",
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("upsert contact: %v", err)
	}

	err = AddWhatsAppID(ctx, conn, contact.ID, "12345@s.whatsapp.net", "12345")
	if err != nil {
		t.Fatalf("add whatsapp id: %v", err)
	}

	got, err := GetContact(ctx, conn, contact.ID)
	if err != nil {
		t.Fatalf("get contact: %v", err)
	}

	if len(got.WhatsappIDs) != 2 {
		t.Fatalf("expected 2 whatsapp ids, got %v", got.WhatsappIDs)
	}
	if len(got.PhoneNumbers) != 1 || got.PhoneNumbers[0] != "12345" {
		t.Fatalf("expected phone number 12345, got %v", got.PhoneNumbers)
	}
}

func TestAddWhatsAppIDNoopOnEmptyArgs(t *testing.T) {
	err := AddWhatsAppID(context.Background(), nil, "", "", "")
	if err != nil {
		t.Fatalf("expected noop, got error: %v", err)
	}

	err = AddWhatsAppID(context.Background(), nil, "some-id", "", "")
	if err != nil {
		t.Fatalf("expected noop, got error: %v", err)
	}
}

func containsStringContact(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}

	return false
}
