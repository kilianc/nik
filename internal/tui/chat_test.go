package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kciuffolo/nik/internal/db"
)

type stubSender struct {
	lastBody string
}

func (s *stubSender) Send(_ context.Context, body string) error {
	s.lastBody = body
	return nil
}

func TestChatModelRendersEmptyState(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	c := newChatModel(conn, nil)

	view := c.View()
	if !strings.Contains(view, "❯") {
		t.Error("expected prompt in view")
	}
}

func TestChatNewMessagesMsg(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	c := newChatModel(conn, nil)

	msgs := []db.Message{
		{ID: "msg-1", Body: "hello", Kind: "text", ContactID: db.OwnerContactID, SentAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "msg-2", Body: "hi there", Kind: "text", ContactID: db.SystemContactID, IsFromMe: true, SentAt: time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC)},
	}

	c, _ = c.Update(newMessagesMsg{messages: msgs})

	if len(c.messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(c.messages))
	}
	if c.lastID != "msg-2" {
		t.Errorf("expected lastID msg-2, got %q", c.lastID)
	}
}

func TestChatActivityThinking(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	c := newChatModel(conn, nil)
	c, _ = c.Update(newMessagesMsg{activity: []string{"thinking"}})

	view := c.View()
	if !strings.Contains(view, "thinking") {
		t.Error("expected thinking indicator in view")
	}
}

func TestChatWindowResize(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	c := newChatModel(conn, nil)
	c, _ = c.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	if c.width != 50 {
		t.Errorf("expected width 50 (half of 100), got %d", c.width)
	}
}

func TestChatSendMessage(t *testing.T) {
	stub := &stubSender{}
	cmd := sendMessageCmd(stub, "test message")
	msg := cmd()

	sent, ok := msg.(messageSentMsg)
	if !ok {
		t.Fatalf("expected messageSentMsg, got %T", msg)
	}
	if sent.err != nil {
		t.Fatalf("send message: %v", sent.err)
	}
	if stub.lastBody != "test message" {
		t.Errorf("expected body 'test message', got %q", stub.lastBody)
	}
}

func TestChatEmptyPollSchedulesNext(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	c := newChatModel(conn, nil)
	c, cmd := c.Update(newMessagesMsg{})

	if cmd == nil {
		t.Error("expected next poll command even on empty result")
	}
	if len(c.messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(c.messages))
	}
}

func TestChatViewIncludesMessages(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	c := newChatModel(conn, nil)
	c.width = 80
	c.messages = []db.Message{
		{ID: "1", Body: "hello world", Kind: "text", ContactID: db.OwnerContactID, SentAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)},
	}

	view := c.View()
	if !strings.Contains(view, "hello world") {
		t.Error("expected messages in view")
	}
	if !strings.Contains(view, "❯") {
		t.Error("expected prompt after messages")
	}
}

func TestRenderMessages(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)

	msgs := []db.Message{
		{ID: "1", Body: "hello", Kind: "text", ContactID: db.OwnerContactID, SentAt: base},
		{ID: "2", Body: "hi there", Kind: "text", ContactID: db.SystemContactID, IsFromMe: true, SentAt: base.Add(time.Minute)},
	}

	out := renderMessages(msgs, 80)

	if !strings.Contains(out, "hello") {
		t.Error("expected hello in output")
	}
	if !strings.Contains(out, "hi there") {
		t.Error("expected hi there in output")
	}
	if !strings.Contains(out, "you") {
		t.Error("expected you label")
	}
	if !strings.Contains(out, "nik") {
		t.Error("expected nik label")
	}
}

func TestRenderMessagesGroupsSameSender(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)

	msgs := []db.Message{
		{ID: "1", Body: "first", Kind: "text", ContactID: db.OwnerContactID, SentAt: base},
		{ID: "2", Body: "second", Kind: "text", ContactID: db.OwnerContactID, SentAt: base.Add(30 * time.Second)},
		{ID: "3", Body: "third", Kind: "text", ContactID: db.OwnerContactID, SentAt: base.Add(time.Minute)},
	}

	out := renderMessages(msgs, 80)

	count := strings.Count(out, "you")
	if count != 1 {
		t.Errorf("expected 'you' once (grouped), got %d", count)
	}
}

func TestRenderMessagesSenderChange(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)

	msgs := []db.Message{
		{ID: "1", Body: "hey", Kind: "text", ContactID: db.OwnerContactID, SentAt: base},
		{ID: "2", Body: "sup", Kind: "text", ContactID: db.SystemContactID, IsFromMe: true, SentAt: base.Add(time.Minute)},
	}

	out := renderMessages(msgs, 80)
	lines := strings.Split(out, "\n")

	foundBlank := false
	for i, line := range lines {
		if strings.TrimSpace(line) == "" && i > 0 {
			rest := strings.Join(lines[i+1:], "\n")
			if strings.Contains(rest, "nik") {
				foundBlank = true
				break
			}
		}
	}

	if !foundBlank {
		t.Error("expected blank line between sender change")
	}
}

func TestRenderMessagesDateSeparator(t *testing.T) {
	yesterday := time.Now().Local().AddDate(0, 0, -1)
	yesterday = time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 23, 0, 0, 0, time.Local)
	today := time.Now().Local()
	today = time.Date(today.Year(), today.Month(), today.Day(), 1, 0, 0, 0, time.Local)

	msgs := []db.Message{
		{ID: "1", Body: "old", Kind: "text", ContactID: db.OwnerContactID, SentAt: yesterday},
		{ID: "2", Body: "new", Kind: "text", ContactID: db.OwnerContactID, SentAt: today},
	}

	out := renderMessages(msgs, 80)

	if !strings.Contains(out, "Yesterday") {
		t.Error("expected Yesterday separator")
	}
	if !strings.Contains(out, "Today") {
		t.Error("expected Today separator")
	}
}

func TestRenderMessagesBigGap(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)

	msgs := []db.Message{
		{ID: "1", Body: "before", Kind: "text", ContactID: db.OwnerContactID, SentAt: base},
		{ID: "2", Body: "after gap", Kind: "text", ContactID: db.OwnerContactID, SentAt: base.Add(10 * time.Minute)},
	}

	out := renderMessages(msgs, 80)

	count := strings.Count(out, "you")
	if count != 2 {
		t.Errorf("expected 'you' twice (gap breaks grouping), got %d", count)
	}
}
