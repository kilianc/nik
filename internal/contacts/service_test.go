package contacts

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/db"
)

func TestPhoneFromWhatsAppIDExtractsNumber(t *testing.T) {
	got := phoneFromWhatsAppID("12345@s.whatsapp.net")
	if got != "12345" {
		t.Fatalf("expected phone number 12345, got %q", got)
	}
}

func TestPhoneFromWhatsAppIDReturnsLIDUser(t *testing.T) {
	got := phoneFromWhatsAppID("99999@lid")
	if got != "99999" {
		t.Fatalf("expected 99999, got %q", got)
	}
}

func TestEnsureContactForMessageValidatesInputs(t *testing.T) {
	svc := NewService(nil)

	_, err := svc.EnsureContactForMessage(context.Background(), "whatsapp", nil, false, time.Now())
	if err == nil {
		t.Fatalf("expected error for nil external ids")
	}

	_, err = svc.EnsureContactForMessage(context.Background(), "whatsapp", []string{""}, false, time.Now())
	if err == nil {
		t.Fatalf("expected error for empty external sender id")
	}

	_, err = svc.EnsureContactForMessage(context.Background(), "telegram", []string{"me-id"}, true, time.Now())
	if err == nil {
		t.Fatalf("expected unsupported self-contact error")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("expected self-contact not implemented error, got %v", err)
	}
}

func TestEnsureContactLinksBothJIDs(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	svc := NewService(conn)
	now := time.Now()

	id1, err := svc.EnsureContactForMessage(ctx, "whatsapp", []string{"12345@s.whatsapp.net", "99999@lid"}, false, now)
	if err != nil {
		t.Fatalf("first ensure: %v", err)
	}

	contact, err := db.GetContact(ctx, conn, id1)
	if err != nil {
		t.Fatalf("get contact: %v", err)
	}

	if len(contact.WhatsappIDs) != 2 {
		t.Fatalf("expected 2 whatsapp ids after first ensure, got %v", contact.WhatsappIDs)
	}
}

func TestEnsureContactResolvesViaSecondaryJID(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	svc := NewService(conn)
	now := time.Now()

	id1, err := svc.EnsureContactForMessage(ctx, "whatsapp", []string{"12345@s.whatsapp.net"}, false, now)
	if err != nil {
		t.Fatalf("first ensure: %v", err)
	}

	id2, err := svc.EnsureContactForMessage(ctx, "whatsapp", []string{"99999@lid", "12345@s.whatsapp.net"}, false, now)
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}

	if id1 != id2 {
		t.Fatalf("expected same contact id, got %s and %s", id1, id2)
	}

	contact, err := db.GetContact(ctx, conn, id1)
	if err != nil {
		t.Fatalf("get contact: %v", err)
	}

	if len(contact.WhatsappIDs) != 2 {
		t.Fatalf("expected 2 whatsapp ids after linking, got %v", contact.WhatsappIDs)
	}
}

func TestEnsureContactSelfLinksAllJIDs(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	svc := NewService(conn)
	now := time.Now()

	id, err := svc.EnsureContactForMessage(ctx, "whatsapp", []string{"nik@s.whatsapp.net", "nikLID@lid"}, true, now)
	if err != nil {
		t.Fatalf("ensure self: %v", err)
	}

	if id != NikContactID {
		t.Fatalf("expected nik contact id, got %s", id)
	}

	contact, err := db.GetContact(ctx, conn, NikContactID)
	if err != nil {
		t.Fatalf("get nik contact: %v", err)
	}

	if len(contact.WhatsappIDs) != 2 {
		t.Fatalf("expected 2 whatsapp ids for nik, got %v", contact.WhatsappIDs)
	}
}
