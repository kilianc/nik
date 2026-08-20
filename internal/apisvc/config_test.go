package apisvc

import (
	"context"
	"testing"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
)

// Timezone and location live on nik's and the owner's contact cards as well as
// in config.yaml, because that is where the brain reads them. The setup wizard
// used to write both halves; now anything that changes the config gets the
// second half for free.
func TestSetTimezonePropagatesToContacts(t *testing.T) {
	_, conn := newTestChat(t)
	ctx := context.Background()

	cfg := config.Default(t.TempDir())
	svc := NewConfig(cfg, conn)

	err := svc.Set(ctx, "timezone", "Europe/Rome")
	if err != nil {
		t.Fatalf("Set timezone: %v", err)
	}
	err = svc.Set(ctx, "location", "Rome, Italy")
	if err != nil {
		t.Fatalf("Set location: %v", err)
	}

	for _, contactID := range []string{db.OwnerContactID, db.NikContactID} {
		contact, err := db.ContactGet(ctx, conn, contactID)
		if err != nil {
			t.Fatalf("get contact %s: %v", contactID, err)
		}
		if !contact.Timezone.Valid || contact.Timezone.String != "Europe/Rome" {
			t.Errorf("%s timezone = %v, want Europe/Rome", contactID, contact.Timezone)
		}
		if !contact.Location.Valid || contact.Location.String != "Rome, Italy" {
			t.Errorf("%s location = %v, want Rome, Italy", contactID, contact.Location)
		}
	}
}

// A field that does not fan out must not try to.
func TestSetUnrelatedFieldTouchesNoContacts(t *testing.T) {
	_, conn := newTestChat(t)

	cfg := config.Default(t.TempDir())

	err := NewConfig(cfg, conn).Set(context.Background(), "max_history", "50")
	if err != nil {
		t.Fatalf("Set max_history: %v", err)
	}
	if cfg.MaxHistory != 50 {
		t.Fatalf("max_history = %d, want 50", cfg.MaxHistory)
	}
}
