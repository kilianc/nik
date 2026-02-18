package db

import (
	"context"
	"testing"
	"time"
)

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
