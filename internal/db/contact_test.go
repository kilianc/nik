package db

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/id"
)

func TestContactGetByIdentifier(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	seed, err := ContactUpsert(ctx, conn, ContactUpsertParams{
		Platform:      "whatsapp",
		ExternalID:    "alice@s.whatsapp.net",
		Name:          "Alice",
		Phone:         "11111",
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("upsert contact: %v", err)
	}

	gotByID, err := ContactGet(ctx, conn, seed.ID)
	if err != nil {
		t.Fatalf("get contact by id: %v", err)
	}
	if gotByID.ID != seed.ID {
		t.Fatalf("expected id %q, got %q", seed.ID, gotByID.ID)
	}

	gotByJID, err := ContactGet(ctx, conn, "alice@s.whatsapp.net")
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

func TestSystemContactEnsureIsIdempotent(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	err = SystemContactEnsure(ctx, conn)
	if err != nil {
		t.Fatalf("first ensure: %v", err)
	}

	err = SystemContactEnsure(ctx, conn)
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}

	contact, err := ContactGet(ctx, conn, SystemContactID)
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

func TestContactUpsertWhatsAppInsertThenUpdate(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	t0 := time.Now().Add(-time.Minute)
	inserted, err := ContactUpsert(ctx, conn, ContactUpsertParams{
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
	updated, err := ContactUpsert(ctx, conn, ContactUpsertParams{
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

func TestContactUpsertSelfWhatsApp(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	_, err = ContactUpsert(ctx, conn, ContactUpsertParams{
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
	first, err := ContactUpsert(ctx, conn, ContactUpsertParams{
		Platform:      "whatsapp",
		ExternalID:    "self@s.whatsapp.net",
		IsSelf:        true,
		SelfID:        selfID,
		LastMessageAt: time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("upsert self contact first: %v", err)
	}

	second, err := ContactUpsert(ctx, conn, ContactUpsertParams{
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

func TestContactUpsertSelfWhatsAppPreservesRenamedName(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	selfID := id.V7()

	_, err = ContactUpsert(ctx, conn, ContactUpsertParams{
		Platform:      "whatsapp",
		ExternalID:    "self@s.whatsapp.net",
		IsSelf:        true,
		SelfID:        selfID,
		LastMessageAt: time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("initial self upsert: %v", err)
	}

	err = ContactUpdate(ctx, conn, ContactUpdateParams{
		ID:    selfID,
		Field: "name",
		Value: "Nikolai",
	})
	if err != nil {
		t.Fatalf("rename self contact: %v", err)
	}

	after, err := ContactUpsert(ctx, conn, ContactUpsertParams{
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

func TestContactUpdateAllowedFields(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	seed, err := ContactUpsert(ctx, conn, ContactUpsertParams{
		Platform:      "whatsapp",
		ExternalID:    "update@s.whatsapp.net",
		Name:          "Updater",
		Phone:         "88888",
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	err = ContactUpdate(ctx, conn, ContactUpdateParams{
		ID:    seed.ID,
		Field: "notes",
		Value: "new notes",
	})
	if err != nil {
		t.Fatalf("update notes: %v", err)
	}

	err = ContactUpdate(ctx, conn, ContactUpdateParams{
		ID:         seed.ID,
		Field:      "nicknames",
		ArrayValue: []string{"u1", "u2"},
	})
	if err != nil {
		t.Fatalf("update nicknames: %v", err)
	}

	got, err := ContactGet(ctx, conn, seed.ID)
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

func TestContactUpdateRejectsUnknownField(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	err = ContactUpdate(ctx, conn, ContactUpdateParams{
		ID:    id.V7(),
		Field: "bad_field",
		Value: "value",
	})
	if err == nil {
		t.Fatalf("expected error for unknown field")
	}
	if !strings.Contains(err.Error(), "unknown contact field") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestContactAddWhatsAppID(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	t.Run("noop on empty args", func(t *testing.T) {
		err := ContactAddWhatsAppID(ctx, nil, ContactAddWhatsAppIDParams{})
		if err != nil {
			t.Fatalf("expected noop, got error: %v", err)
		}
		err = ContactAddWhatsAppID(ctx, nil, ContactAddWhatsAppIDParams{ContactID: "some-id"})
		if err != nil {
			t.Fatalf("expected noop, got error: %v", err)
		}
	})

	t.Run("appends new JID", func(t *testing.T) {
		contact, err := ContactUpsert(ctx, conn, ContactUpsertParams{
			Platform:      "whatsapp",
			ExternalID:    "12345@s.whatsapp.net",
			Name:          "Alice",
			Phone:         "12345",
			LastMessageAt: time.Now(),
		})
		if err != nil {
			t.Fatalf("upsert contact: %v", err)
		}

		err = ContactAddWhatsAppID(ctx, conn, ContactAddWhatsAppIDParams{
			ContactID: contact.ID,
			JID:       "99999@lid",
		})
		if err != nil {
			t.Fatalf("add whatsapp id: %v", err)
		}

		got, err := ContactGet(ctx, conn, contact.ID)
		if err != nil {
			t.Fatalf("get contact: %v", err)
		}
		if len(got.WhatsappIDs) != 2 {
			t.Fatalf("expected 2 whatsapp ids, got %v", got.WhatsappIDs)
		}
		if got.WhatsappIDs[0] != "12345@s.whatsapp.net" || got.WhatsappIDs[1] != "99999@lid" {
			t.Fatalf("unexpected whatsapp ids: %v", got.WhatsappIDs)
		}
	})

	t.Run("skips duplicate", func(t *testing.T) {
		contact, err := ContactUpsert(ctx, conn, ContactUpsertParams{
			Platform:      "whatsapp",
			ExternalID:    "dup@s.whatsapp.net",
			Name:          "Bob",
			Phone:         "22222",
			LastMessageAt: time.Now(),
		})
		if err != nil {
			t.Fatalf("upsert contact: %v", err)
		}

		err = ContactAddWhatsAppID(ctx, conn, ContactAddWhatsAppIDParams{
			ContactID: contact.ID,
			JID:       "dup@s.whatsapp.net",
			Phone:     "22222",
		})
		if err != nil {
			t.Fatalf("add whatsapp id: %v", err)
		}

		got, err := ContactGet(ctx, conn, contact.ID)
		if err != nil {
			t.Fatalf("get contact: %v", err)
		}
		if len(got.WhatsappIDs) != 1 {
			t.Fatalf("expected 1 whatsapp id (no duplicate), got %v", got.WhatsappIDs)
		}
	})

	t.Run("also adds phone", func(t *testing.T) {
		contact, err := ContactUpsert(ctx, conn, ContactUpsertParams{
			Platform:      "whatsapp",
			ExternalID:    "88888@lid",
			Name:          "",
			Phone:         "",
			LastMessageAt: time.Now(),
		})
		if err != nil {
			t.Fatalf("upsert contact: %v", err)
		}

		err = ContactAddWhatsAppID(ctx, conn, ContactAddWhatsAppIDParams{
			ContactID: contact.ID,
			JID:       "33333@s.whatsapp.net",
			Phone:     "33333",
		})
		if err != nil {
			t.Fatalf("add whatsapp id: %v", err)
		}

		got, err := ContactGet(ctx, conn, contact.ID)
		if err != nil {
			t.Fatalf("get contact: %v", err)
		}
		if len(got.WhatsappIDs) != 2 {
			t.Fatalf("expected 2 whatsapp ids, got %v", got.WhatsappIDs)
		}
		if len(got.PhoneNumbers) != 1 || got.PhoneNumbers[0] != "33333" {
			t.Fatalf("expected phone number 33333, got %v", got.PhoneNumbers)
		}
	})
}

func containsStringContact(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}

	return false
}
