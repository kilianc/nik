package timeline

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
)

func TestMessageEntryReactionToText(t *testing.T) {
	target := db.Message{
		ID:                "019577a1-0000-7000-8000-a1b2c3d4e5f6",
		ExternalMessageID: "ext-target-1",
		Kind:              "text",
		Body:              "personal",
		SentAt:            time.Now(),
	}
	reaction := db.Message{
		ID:              "019577a1-0000-7000-8000-ffffffffffff",
		Kind:            "reaction",
		Body:            "📚",
		IsFromMe:        true,
		ContextStanzaID: sql.NullString{Valid: true, String: "ext-target-1"},
		SentAt:          time.Now(),
	}

	lookup := map[string]db.Message{target.ExternalMessageID: target}
	e := messageEntry(reaction, "", lookup)

	shortID := id.Shorten(target.ID)
	want := `reacted 📚 to ` + shortID + ` "personal"`
	if e.text != want {
		t.Fatalf("got %q, want %q", e.text, want)
	}
	if e.from != "YOU" {
		t.Fatalf("expected sender YOU, got %q", e.from)
	}
}

func TestMessageEntryReactionToMedia(t *testing.T) {
	target := db.Message{
		ID:                "019577a1-0000-7000-8000-a1b2c3d4e5f6",
		ExternalMessageID: "ext-target-2",
		Kind:              "image",
		Body:              "",
		SentAt:            time.Now(),
	}
	reaction := db.Message{
		ID:              "019577a1-0000-7000-8000-ffffffffffff",
		Kind:            "reaction",
		Body:            "❤️",
		IsFromMe:        true,
		ContextStanzaID: sql.NullString{Valid: true, String: "ext-target-2"},
		SentAt:          time.Now(),
	}

	lookup := map[string]db.Message{target.ExternalMessageID: target}
	e := messageEntry(reaction, "", lookup)

	shortID := id.Shorten(target.ID)
	want := "reacted ❤️ to " + shortID + " (image)"
	if e.text != want {
		t.Fatalf("got %q, want %q", e.text, want)
	}
}

func TestMessageEntryReactionTargetMissing(t *testing.T) {
	reaction := db.Message{
		ID:              "019577a1-0000-7000-8000-ffffffffffff",
		Kind:            "reaction",
		Body:            "👍",
		IsFromMe:        true,
		ContextStanzaID: sql.NullString{Valid: true, String: "ext-not-in-window"},
		SentAt:          time.Now(),
	}

	lookup := map[string]db.Message{}
	e := messageEntry(reaction, "", lookup)

	if e.text != "reacted 👍" {
		t.Fatalf("expected fallback without target, got %q", e.text)
	}
}

func TestMessageEntryRemovedReaction(t *testing.T) {
	target := db.Message{
		ID:                "019577a1-0000-7000-8000-a1b2c3d4e5f6",
		ExternalMessageID: "ext-target-3",
		Kind:              "text",
		Body:              "personal",
		SentAt:            time.Now(),
	}
	reaction := db.Message{
		ID:              "019577a1-0000-7000-8000-ffffffffffff",
		Kind:            "reaction",
		Body:            "",
		IsFromMe:        true,
		ContextStanzaID: sql.NullString{Valid: true, String: "ext-target-3"},
		SentAt:          time.Now(),
	}

	lookup := map[string]db.Message{target.ExternalMessageID: target}
	e := messageEntry(reaction, "", lookup)

	shortID := id.Shorten(target.ID)
	want := `removed reaction to ` + shortID + ` "personal"`
	if e.text != want {
		t.Fatalf("got %q, want %q", e.text, want)
	}
}

func TestMessageEntryReactionTruncatesLongBody(t *testing.T) {
	longBody := strings.Repeat("a", 80)
	target := db.Message{
		ID:                "019577a1-0000-7000-8000-a1b2c3d4e5f6",
		ExternalMessageID: "ext-target-4",
		Kind:              "text",
		Body:              longBody,
		SentAt:            time.Now(),
	}
	reaction := db.Message{
		ID:              "019577a1-0000-7000-8000-ffffffffffff",
		Kind:            "reaction",
		Body:            "🔥",
		IsFromMe:        true,
		ContextStanzaID: sql.NullString{Valid: true, String: "ext-target-4"},
		SentAt:          time.Now(),
	}

	lookup := map[string]db.Message{target.ExternalMessageID: target}
	e := messageEntry(reaction, "", lookup)

	shortID := id.Shorten(target.ID)
	want := `reacted 🔥 to ` + shortID + ` "` + longBody[:50] + `…"`
	if e.text != want {
		t.Fatalf("got %q, want %q", e.text, want)
	}
}
