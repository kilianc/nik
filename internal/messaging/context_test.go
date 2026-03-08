package messaging

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/db"
)

func TestMessageLineIncludesMediaAndEditContext(t *testing.T) {
	msg := db.Message{
		ID:                  "msg-2",
		ExternalSenderID:    "alice@s.whatsapp.net",
		Kind:                "reaction",
		Body:                "👍",
		IsEdit:              true,
		EditTargetMessageID: sql.NullString{Valid: true, String: "target-1"},
		ContextStanzaID:     sql.NullString{Valid: true, String: "target-1"},
		MediaLocalPath:      sql.NullString{Valid: true, String: "media/x.jpg"},
		MediaDescribeText:   sql.NullString{Valid: true, String: "photo of a cat"},
		SentAt:              time.Now(),
	}

	line := formatMessageLine(msg, "Alice")
	if !strings.Contains(line, "edited:") {
		t.Fatalf("expected edit metadata, got %q", line)
	}
	if !strings.Contains(line, "reacted") {
		t.Fatalf("expected reaction formatting, got %q", line)
	}
	if !strings.Contains(line, "Alice:") {
		t.Fatalf("expected sender in output, got %q", line)
	}
	if !strings.Contains(line, "media=media/x.jpg") {
		t.Fatalf("expected relative media path in output, got %q", line)
	}
	if !strings.Contains(line, "media_description=photo of a cat") {
		t.Fatalf("expected media description in output, got %q", line)
	}
}

func TestMessageLineMediaUnavailable(t *testing.T) {
	msg := db.Message{
		ID:     "msg-audio",
		Kind:   "audio",
		Body:   "",
		SentAt: time.Now(),
	}

	line := formatMessageLine(msg, "Alice")
	if !strings.Contains(line, "media_unavailable") {
		t.Fatalf("expected media_unavailable for audio with no path, got %q", line)
	}
	if !strings.Contains(line, "(audio)") {
		t.Fatalf("expected (audio) kind marker, got %q", line)
	}
}

func TestMessageLineMediaUnavailableNotShownWithPath(t *testing.T) {
	msg := db.Message{
		ID:             "msg-audio",
		Kind:           "audio",
		Body:           "",
		MediaLocalPath: sql.NullString{Valid: true, String: "media/abc.ogg"},
		SentAt:         time.Now(),
	}

	line := formatMessageLine(msg, "Alice")
	if strings.Contains(line, "media_unavailable") {
		t.Fatalf("should not show media_unavailable when path exists, got %q", line)
	}
	if !strings.Contains(line, "media=media/abc.ogg") {
		t.Fatalf("expected media path, got %q", line)
	}
}

func TestMessageLineMediaUnavailableNotShownWithDescription(t *testing.T) {
	msg := db.Message{
		ID:                "msg-audio",
		Kind:              "audio",
		Body:              "",
		MediaDescribeText: sql.NullString{Valid: true, String: "a voice note saying hello"},
		SentAt:            time.Now(),
	}

	line := formatMessageLine(msg, "Alice")
	if strings.Contains(line, "media_unavailable") {
		t.Fatalf("should not show media_unavailable when description exists, got %q", line)
	}
}

func TestMessageLineMediaUnavailableNotShownForText(t *testing.T) {
	msg := db.Message{
		ID:     "msg-text",
		Kind:   "text",
		Body:   "hello",
		SentAt: time.Now(),
	}

	line := formatMessageLine(msg, "Alice")
	if strings.Contains(line, "media_unavailable") {
		t.Fatalf("text messages should not have media_unavailable, got %q", line)
	}
}

func TestMessageLineSecondPrecision(t *testing.T) {
	msg := db.Message{
		ID:     "msg-1",
		Kind:   "text",
		Body:   "hello",
		SentAt: time.Date(2026, time.February, 28, 9, 32, 10, 0, time.UTC),
	}

	line := formatMessageLine(msg, "Alice")
	if !strings.Contains(line, "[09:32:10]") {
		t.Fatalf("expected second-precision timestamp, got %q", line)
	}
}
