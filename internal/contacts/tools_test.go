package contacts

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/llm"
)

func TestUpdateContactHandlerRejectsInvalidJSON(t *testing.T) {
	handler := updateContactHandler(nil)

	out, err := handler(context.Background(), llm.ToolCall{Arguments: "{"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"error"`) {
		t.Fatalf("expected error response, got %q", out)
	}
}

func TestUpdateContactHandlerUpdatesEachField(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	seed, err := db.ContactUpsert(ctx, conn, db.ContactUpsertParams{
		Platform:      "whatsapp",
		ExternalID:    "test@s.whatsapp.net",
		Name:          "Test",
		Phone:         "99999",
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("upsert contact: %v", err)
	}

	handler := updateContactHandler(conn)

	cases := []struct {
		field string
		value string
		check func(t *testing.T, c db.Contact)
	}{
		{
			field: "name",
			value: "Updated Name",
			check: func(t *testing.T, c db.Contact) {
				if c.Name != "Updated Name" {
					t.Fatalf("expected name %q, got %q", "Updated Name", c.Name)
				}
			},
		},
		{
			field: "notes",
			value: "likes espresso",
			check: func(t *testing.T, c db.Contact) {
				if !c.Notes.Valid || c.Notes.String != "likes espresso" {
					t.Fatalf("expected notes %q, got %v", "likes espresso", c.Notes)
				}
			},
		},
		{
			field: "one_liner",
			value: "coffee enthusiast",
			check: func(t *testing.T, c db.Contact) {
				if !c.OneLiner.Valid || c.OneLiner.String != "coffee enthusiast" {
					t.Fatalf("expected one_liner %q, got %v", "coffee enthusiast", c.OneLiner)
				}
			},
		},
		{
			field: "timezone",
			value: "America/New_York",
			check: func(t *testing.T, c db.Contact) {
				if !c.Timezone.Valid || c.Timezone.String != "America/New_York" {
					t.Fatalf("expected timezone %q, got %v", "America/New_York", c.Timezone)
				}
			},
		},
		{
			field: "location",
			value: "Brooklyn, NY",
			check: func(t *testing.T, c db.Contact) {
				if !c.Location.Valid || c.Location.String != "Brooklyn, NY" {
					t.Fatalf("expected location %q, got %v", "Brooklyn, NY", c.Location)
				}
			},
		},
		{
			field: "nicknames",
			value: `["T","Testy"]`,
			check: func(t *testing.T, c db.Contact) {
				if len(c.Nicknames) != 2 || c.Nicknames[0] != "T" || c.Nicknames[1] != "Testy" {
					t.Fatalf("expected nicknames [T Testy], got %v", c.Nicknames)
				}
			},
		},
		{
			field: "emails",
			value: `["test@example.com","other@example.com"]`,
			check: func(t *testing.T, c db.Contact) {
				if len(c.Emails) != 2 || c.Emails[0] != "test@example.com" || c.Emails[1] != "other@example.com" {
					t.Fatalf("expected emails [test@example.com other@example.com], got %v", c.Emails)
				}
			},
		},
		{
			field: "phone_numbers",
			value: `["+15551234","+15555678"]`,
			check: func(t *testing.T, c db.Contact) {
				if len(c.PhoneNumbers) != 2 || c.PhoneNumbers[0] != "+15551234" || c.PhoneNumbers[1] != "+15555678" {
					t.Fatalf("expected phone_numbers [+15551234 +15555678], got %v", c.PhoneNumbers)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			args := fmt.Sprintf(`{"contact_id":%q,"field":%q,"value":%q}`, seed.ID, tc.field, tc.value)

			out, err := handler(ctx, llm.ToolCall{Arguments: args})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out != `{"ok":true}` {
				t.Fatalf("expected ok, got %q", out)
			}

			got, err := db.ContactGet(ctx, conn, seed.ID)
			if err != nil {
				t.Fatalf("get contact after update: %v", err)
			}

			tc.check(t, got)
		})
	}
}
