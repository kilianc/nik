package recall

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/db"
)

func TestRecallDisabledWhenNoClient(t *testing.T) {
	svc := &Service{}

	result := svc.Recall(context.Background(), "hello world")
	if result != "" {
		t.Fatalf("expected empty string when client is nil, got %q", result)
	}
}

func TestFormatAllMessages(t *testing.T) {
	messages := []db.RecallMessage{
		{
			Body:              "hey how are you",
			SentAt:            time.Date(2026, 3, 1, 10, 30, 0, 0, time.UTC),
			IsFromMe:          false,
			SenderName:        "Kilian",
			ConversationTitle: "DM with Kilian",
			ConversationKind:  "dm",
		},
		{
			Body:              "doing great!",
			SentAt:            time.Date(2026, 3, 1, 10, 31, 0, 0, time.UTC),
			IsFromMe:          true,
			SenderName:        "Nik Ciuffolo",
			ConversationTitle: "DM with Kilian",
			ConversationKind:  "dm",
		},
	}

	got := formatAll(messages, "", nil, nil, nil, nil, nil)
	if got == "" {
		t.Fatal("expected non-empty output")
	}

	if !strings.Contains(got, "[2026-03-01 10:30] Kilian: hey how are you") {
		t.Fatalf("missing message line in:\n%s", got)
	}
}

func TestFormatAllMemories(t *testing.T) {
	raw := "| 2026-03-01 | preference | Kilian | likes coffee |\n"

	got := formatAll(nil, raw, nil, nil, nil, nil, nil)
	if !strings.Contains(got, "likes coffee") {
		t.Fatalf("missing memory content in:\n%s", got)
	}
	if !strings.Contains(got, "## Memories") {
		t.Fatalf("missing memories header in:\n%s", got)
	}
}

func TestFormatAllContacts(t *testing.T) {
	contacts := []db.RecallContact{
		{Name: "Kilian Ciuffolo", Nicknames: []string{"K"}},
	}

	got := formatAll(nil, "", contacts, nil, nil, nil, nil)
	if !strings.Contains(got, "Kilian Ciuffolo (aka K)") {
		t.Fatalf("missing contact line in:\n%s", got)
	}
}

func TestFormatAllEmpty(t *testing.T) {
	got := formatAll(nil, "", nil, nil, nil, nil, nil)
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestTokenEstimate(t *testing.T) {
	s := "hello world!" // 12 chars -> 3 tokens
	if got := tokenEstimate(s); got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}
}

func TestPartitionSingleChunk(t *testing.T) {
	block := "line one\nline two\nline three"
	chunks := partition(block, 1_000_000)

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}

	if chunks[0] != block {
		t.Fatalf("chunk content mismatch: %q", chunks[0])
	}
}

func TestPartitionMultipleChunks(t *testing.T) {
	var lines []string
	for i := 0; i < 10; i++ {
		lines = append(lines, "abcdefghij")
	}
	block := joinLines(lines)

	chunks := partition(block, 5)

	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}

	reconstructed := joinChunks(chunks)
	if reconstructed != block {
		t.Fatalf("content lost in partitioning:\ngot:  %q\nwant: %q", reconstructed, block)
	}
}

func joinLines(lines []string) string {
	var b []byte
	for i, l := range lines {
		if i > 0 {
			b = append(b, '\n')
		}
		b = append(b, l...)
	}
	return string(b)
}

func joinChunks(chunks []string) string {
	var b []byte
	for i, c := range chunks {
		if i > 0 {
			b = append(b, '\n')
		}
		b = append(b, c...)
	}
	return string(b)
}
