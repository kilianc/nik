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
	if strings.Contains(line, "edited:") {
		t.Fatalf("edit prefix should not appear in FormatMessageText, got %q", line)
	}
	if !strings.Contains(line, "(👍)") {
		t.Fatalf("expected paren-wrapped reaction, got %q", line)
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
	tests := []struct {
		name           string
		msg            db.Message
		wantUnavail    bool
		wantSubstrings []string
	}{
		{
			name:           "audio with no path",
			msg:            db.Message{ID: "msg-1", Kind: "audio", SentAt: time.Now()},
			wantUnavail:    true,
			wantSubstrings: []string{"(audio)"},
		},
		{
			name:           "has path",
			msg:            db.Message{ID: "msg-2", Kind: "audio", MediaLocalPath: sql.NullString{Valid: true, String: "media/abc.ogg"}, SentAt: time.Now()},
			wantUnavail:    false,
			wantSubstrings: []string{"media=media/abc.ogg"},
		},
		{
			name:        "has description",
			msg:         db.Message{ID: "msg-3", Kind: "audio", MediaDescribeText: sql.NullString{Valid: true, String: "a voice note"}, SentAt: time.Now()},
			wantUnavail: false,
		},
		{
			name:        "text kind",
			msg:         db.Message{ID: "msg-4", Kind: "text", Body: "hello", SentAt: time.Now()},
			wantUnavail: false,
		},
		{
			name:           "second precision timestamp",
			msg:            db.Message{ID: "msg-5", Kind: "text", Body: "hello", SentAt: time.Date(2026, time.February, 28, 9, 32, 10, 0, time.UTC)},
			wantUnavail:    false,
			wantSubstrings: []string{"[09:32:10]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := formatMessageLine(tt.msg, "Alice")

			hasUnavail := strings.Contains(line, "media_unavailable")
			if hasUnavail != tt.wantUnavail {
				t.Fatalf("media_unavailable=%v, want %v; line=%q", hasUnavail, tt.wantUnavail, line)
			}

			for _, sub := range tt.wantSubstrings {
				if !strings.Contains(line, sub) {
					t.Fatalf("expected %q in line %q", sub, line)
				}
			}
		})
	}
}
