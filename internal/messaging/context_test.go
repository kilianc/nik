package messaging

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/db"
)

func TestBuildConversationInputIncludesSessionBlock(t *testing.T) {
	conv := db.Conversation{
		ID:       "conv-1",
		Platform: "whatsapp",
		Kind:     "dm",
	}
	msgs := []db.Message{
		{
			ID:               "msg-1",
			ContactID:        "contact-1",
			ExternalSenderID: "alice@s.whatsapp.net",
			Kind:             "text",
			Body:             "hello",
			SentAt:           time.Now(),
		},
	}

	lines := BuildConversationInput(conv, msgs, map[string]string{"msg-1": "Alice"}, SessionContext{
		Lines: []string{
			"Platform: whatsapp",
			"Type: dm",
			"Contact: Alice",
		},
	}, nil)

	out := strings.Join(lines, "\n")
	if !strings.Contains(out, "## Session") {
		t.Fatalf("expected session section in output, got %q", out)
	}
	if !strings.Contains(out, "Contact: Alice") {
		t.Fatalf("expected dm profile line in output, got %q", out)
	}
}

func TestBuildConversationInputNoCanonicalRefs(t *testing.T) {
	conv := db.Conversation{
		ID:       "conv-1",
		Platform: "whatsapp",
		Kind:     "dm",
	}
	msgs := []db.Message{
		{ID: "msg-1", Kind: "text", Body: "hello", SentAt: time.Now()},
		{ID: "msg-2", Kind: "text", Body: "world", SentAt: time.Now()},
	}
	labels := map[string]string{"msg-1": "Alice", "msg-2": "Bob"}
	lines := BuildConversationInput(conv, msgs, labels, SessionContext{}, nil)
	out := strings.Join(lines, "\n")

	if strings.Contains(out, "Canonical refs") {
		t.Fatalf("output should not contain Canonical refs, got %q", out)
	}
	if strings.Contains(out, "m1 ") || strings.Contains(out, "m2 ") {
		t.Fatalf("output should not contain inline message refs, got %q", out)
	}
}

func TestBuildConversationInputUsesSubHeadings(t *testing.T) {
	now := time.Now()
	conv := db.Conversation{
		ID:         "conv-1",
		Platform:   "whatsapp",
		Kind:       "dm",
		LastReadAt: sql.NullTime{Valid: true, Time: now.Add(-time.Second)},
	}
	msgs := []db.Message{
		{ID: "msg-1", Kind: "text", Body: "old", SentAt: now.Add(-2 * time.Second)},
		{ID: "msg-2", Kind: "text", Body: "new", SentAt: now},
	}
	labels := map[string]string{"msg-1": "Alice", "msg-2": "Alice"}
	lines := BuildConversationInput(conv, msgs, labels, SessionContext{}, nil)
	out := strings.Join(lines, "\n")

	if !strings.Contains(out, "### Context") {
		t.Fatalf("expected ### Context sub-heading, got %q", out)
	}
	if !strings.Contains(out, "### New messages") {
		t.Fatalf("expected ### New messages sub-heading, got %q", out)
	}
	if strings.Contains(out, "## Conversation") {
		t.Fatalf("should not have ## Conversation heading, got %q", out)
	}
}

func TestBuildConversationInputDateSeparators(t *testing.T) {
	day1 := time.Date(2026, time.February, 27, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, time.February, 28, 9, 0, 0, 0, time.UTC)

	conv := db.Conversation{ID: "conv-1", Platform: "whatsapp", Kind: "dm"}
	msgs := []db.Message{
		{ID: "msg-1", Kind: "text", Body: "day one", SentAt: day1},
		{ID: "msg-2", Kind: "text", Body: "day two", SentAt: day2},
	}
	labels := map[string]string{"msg-1": "Alice", "msg-2": "Alice"}
	lines := BuildConversationInput(conv, msgs, labels, SessionContext{}, nil)
	out := strings.Join(lines, "\n")

	if !strings.Contains(out, "--- Feb 27, 2026 ---") {
		t.Fatalf("expected first date separator, got %q", out)
	}
	if !strings.Contains(out, "--- Feb 28, 2026 ---") {
		t.Fatalf("expected second date separator, got %q", out)
	}
}

func TestFormatMessageLineIncludesMediaAndEditContext(t *testing.T) {
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

	line := FormatMessageLine(msg, "Alice")
	if !strings.Contains(line, "edited:") {
		t.Fatalf("expected edit metadata, got %q", line)
	}
	if !strings.Contains(line, "reaction") {
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

func TestFormatMessageLineMediaUnavailable(t *testing.T) {
	msg := db.Message{
		ID:     "msg-audio",
		Kind:   "audio",
		Body:   "",
		SentAt: time.Now(),
	}

	line := FormatMessageLine(msg, "Alice")
	if !strings.Contains(line, "media_unavailable") {
		t.Fatalf("expected media_unavailable for audio with no path, got %q", line)
	}
	if !strings.Contains(line, "(audio)") {
		t.Fatalf("expected (audio) kind marker, got %q", line)
	}
}

func TestFormatMessageLineMediaUnavailableNotShownWithPath(t *testing.T) {
	msg := db.Message{
		ID:             "msg-audio",
		Kind:           "audio",
		Body:           "",
		MediaLocalPath: sql.NullString{Valid: true, String: "media/abc.ogg"},
		SentAt:         time.Now(),
	}

	line := FormatMessageLine(msg, "Alice")
	if strings.Contains(line, "media_unavailable") {
		t.Fatalf("should not show media_unavailable when path exists, got %q", line)
	}
	if !strings.Contains(line, "media=media/abc.ogg") {
		t.Fatalf("expected media path, got %q", line)
	}
}

func TestFormatMessageLineMediaUnavailableNotShownWithDescription(t *testing.T) {
	msg := db.Message{
		ID:                "msg-audio",
		Kind:              "audio",
		Body:              "",
		MediaDescribeText: sql.NullString{Valid: true, String: "a voice note saying hello"},
		SentAt:            time.Now(),
	}

	line := FormatMessageLine(msg, "Alice")
	if strings.Contains(line, "media_unavailable") {
		t.Fatalf("should not show media_unavailable when description exists, got %q", line)
	}
}

func TestFormatMessageLineMediaUnavailableNotShownForText(t *testing.T) {
	msg := db.Message{
		ID:     "msg-text",
		Kind:   "text",
		Body:   "hello",
		SentAt: time.Now(),
	}

	line := FormatMessageLine(msg, "Alice")
	if strings.Contains(line, "media_unavailable") {
		t.Fatalf("text messages should not have media_unavailable, got %q", line)
	}
}

func TestFormatMessageLineSecondPrecision(t *testing.T) {
	msg := db.Message{
		ID:     "msg-1",
		Kind:   "text",
		Body:   "hello",
		SentAt: time.Date(2026, time.February, 28, 9, 32, 10, 0, time.UTC),
	}

	line := FormatMessageLine(msg, "Alice")
	if !strings.Contains(line, "[09:32:10]") {
		t.Fatalf("expected second-precision timestamp, got %q", line)
	}
}
