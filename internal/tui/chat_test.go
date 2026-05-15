package tui

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
)

func newTestChat(t *testing.T, conn *sql.DB, sender MessageSender, opts Options) chatModel {
	t.Helper()
	return newChatModel(&config.Config{Home: t.TempDir()}, conn, sender, opts)
}

type stubSender struct {
	lastBody string
}

func (s *stubSender) Send(_ context.Context, body string) error {
	s.lastBody = body
	return nil
}

func stripAnsi(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(s, "")
}

func TestChatModelRendersEmptyState(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	c := newTestChat(t, conn, nil, Options{})

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

	c := newTestChat(t, conn, nil, Options{})

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

	c := newTestChat(t, conn, nil, Options{})
	c, _ = c.Update(newMessagesMsg{activity: []string{"thinking"}})

	if len(c.activity) == 0 {
		t.Error("expected activity to be set")
	}
	if c.activity[0] != "thinking" {
		t.Errorf("expected thinking activity, got %q", c.activity[0])
	}
}

func TestGhostBubblePrecedence(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	c := newTestChat(t, conn, nil, Options{})
	c, _ = c.Update(tea.WindowSizeMsg{Width: 80, Height: 30})

	// thinking only -> ghost shows "thinking"
	c, _ = c.Update(newMessagesMsg{activity: []string{"thinking"}})
	out := stripAnsi(c.convCache)
	if !strings.Contains(out, "thinking") {
		t.Errorf("expected 'thinking' ghost bubble when thinking, got %q", out)
	}
	if strings.Contains(out, "typing") {
		t.Errorf("did not expect 'typing' label when only thinking, got %q", out)
	}

	// typing+thinking -> typing wins
	c, _ = c.Update(newMessagesMsg{activity: []string{"typing", "thinking"}})
	out = stripAnsi(c.convCache)
	if !strings.Contains(out, "typing") {
		t.Errorf("expected 'typing' ghost bubble when typing+thinking, got %q", out)
	}

	// no activity -> no ghost bubble
	c, _ = c.Update(newMessagesMsg{activity: nil})
	out = stripAnsi(c.convCache)
	if strings.Contains(out, "typing") || strings.Contains(out, "thinking") {
		t.Errorf("did not expect ghost bubble when idle, got %q", out)
	}
}

func TestChatWindowResize(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	c := newTestChat(t, conn, nil, Options{})
	c, _ = c.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	if c.width != 100 {
		t.Errorf("expected width 100, got %d", c.width)
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

	c := newTestChat(t, conn, nil, Options{})
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

	c := newTestChat(t, conn, nil, Options{})
	c, _ = c.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	c, _ = c.Update(newMessagesMsg{
		messages: []db.Message{
			{ID: "1", Body: "hello world", Kind: "text", ContactID: db.OwnerContactID, SentAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)},
		},
	})

	view := c.View()
	if !strings.Contains(view, "hello world") {
		t.Error("expected messages in view")
	}
	if !strings.Contains(view, "❯") {
		t.Error("expected prompt after messages")
	}
}

func TestRenderConversation_ContainsMessages(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)

	msgs := []db.Message{
		{ID: "1", Body: "hello", Kind: "text", ContactID: db.OwnerContactID, SentAt: base},
		{ID: "2", Body: "hi there", Kind: "text", ContactID: db.SystemContactID, IsFromMe: true, SentAt: base.Add(time.Minute)},
	}

	out := stripAnsi(renderConversation(msgs, 80, 0, -1, "", false))

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

func TestRenderConversation_GroupsSameSender(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)

	msgs := []db.Message{
		{ID: "1", Body: "first", Kind: "text", ContactID: db.OwnerContactID, SentAt: base},
		{ID: "2", Body: "second", Kind: "text", ContactID: db.OwnerContactID, SentAt: base.Add(30 * time.Second)},
		{ID: "3", Body: "third", Kind: "text", ContactID: db.OwnerContactID, SentAt: base.Add(time.Minute)},
	}

	out := stripAnsi(renderConversation(msgs, 80, 0, -1, "", false))

	count := strings.Count(out, "you")
	if count != 1 {
		t.Errorf("expected 'you' once (grouped), got %d", count)
	}
}

func TestRenderConversation_SenderChange(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)

	msgs := []db.Message{
		{ID: "1", Body: "hey", Kind: "text", ContactID: db.OwnerContactID, SentAt: base},
		{ID: "2", Body: "sup", Kind: "text", ContactID: db.SystemContactID, IsFromMe: true, SentAt: base.Add(time.Minute)},
	}

	out := renderConversation(msgs, 80, 0, -1, "", false)
	lines := strings.Split(out, "\n")

	foundBlank := false
	for i, line := range lines {
		if strings.TrimSpace(stripAnsi(line)) == "" && i > 0 {
			rest := stripAnsi(strings.Join(lines[i+1:], "\n"))
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

func TestRenderConversation_DateSeparator(t *testing.T) {
	yesterday := time.Now().Local().AddDate(0, 0, -1)
	yesterday = time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 23, 0, 0, 0, time.Local)
	today := time.Now().Local()
	today = time.Date(today.Year(), today.Month(), today.Day(), 1, 0, 0, 0, time.Local)

	msgs := []db.Message{
		{ID: "1", Body: "old", Kind: "text", ContactID: db.OwnerContactID, SentAt: yesterday},
		{ID: "2", Body: "new", Kind: "text", ContactID: db.OwnerContactID, SentAt: today},
	}

	out := stripAnsi(renderConversation(msgs, 80, 0, -1, "", false))

	if !strings.Contains(out, "Yesterday") {
		t.Error("expected Yesterday separator")
	}
	if !strings.Contains(out, "Today") {
		t.Error("expected Today separator")
	}
}

func TestRenderConversation_BigGap(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)

	msgs := []db.Message{
		{ID: "1", Body: "before", Kind: "text", ContactID: db.OwnerContactID, SentAt: base},
		{ID: "2", Body: "after gap", Kind: "text", ContactID: db.OwnerContactID, SentAt: base.Add(10 * time.Minute)},
	}

	out := stripAnsi(renderConversation(msgs, 80, 0, -1, "", false))

	count := strings.Count(out, "you")
	if count != 2 {
		t.Errorf("expected 'you' twice (gap breaks grouping), got %d", count)
	}
}

func TestBubble_ContainsBody(t *testing.T) {
	out := stripAnsi(bubble("hello world", true, false, 80, nil))
	if !strings.Contains(out, "hello world") {
		t.Error("expected body in bubble")
	}
}

func TestBubble_RightAlignedForYou(t *testing.T) {
	out := bubble("test", false, false, 80, nil)
	lines := strings.Split(out, "\n")
	for _, l := range lines {
		stripped := stripAnsi(l)
		if strings.TrimSpace(stripped) != "" && !strings.HasPrefix(stripped, " ") {
			t.Errorf("expected right-aligned you bubble to have leading spaces, got %q", stripped)
		}
	}
}

func TestBubble_LeftAlignedForNik(t *testing.T) {
	out := bubble("test", true, false, 80, nil)
	lines := strings.Split(out, "\n")
	first := stripAnsi(lines[0])
	if strings.HasPrefix(first, "  ") {
		t.Errorf("nik bubble should be left-aligned, got %q", first)
	}
}

func TestGhostBubble_Content(t *testing.T) {
	out := stripAnsi(ghostBubble(0, "typing"))
	if !strings.Contains(out, "typing") {
		t.Error("expected typing in ghost bubble")
	}
	out = stripAnsi(ghostBubble(0, "thinking"))
	if !strings.Contains(out, "thinking") {
		t.Error("expected thinking in ghost bubble")
	}
}

func TestRenderToolLine_Running(t *testing.T) {
	out := stripAnsi(renderToolLine(0, "search_contacts", toolRunning, "finding kilian", 80))
	if !strings.Contains(out, "search_contacts") {
		t.Errorf("should contain tool name, got %q", out)
	}
	if !strings.Contains(out, "finding kilian") {
		t.Errorf("should contain reason, got %q", out)
	}
	if strings.Contains(out, "✓") {
		t.Error("running tool should not have checkmark")
	}
}

func TestRenderToolLine_Done(t *testing.T) {
	out := stripAnsi(renderToolLine(0, "search_contacts", toolDone, "finding kilian", 80))
	if !strings.Contains(out, "✓") {
		t.Error("done tool should have checkmark")
	}
}

func TestRenderToolLine_Error(t *testing.T) {
	out := stripAnsi(renderToolLine(0, "db_query", toolError, "check data", 80))
	if !strings.Contains(out, "✗") {
		t.Error("error tool should have ✗ indicator")
	}
	if strings.Contains(out, "✓") {
		t.Error("error tool should not have checkmark")
	}
}

func TestRenderToolLine_NoReason(t *testing.T) {
	out := stripAnsi(renderToolLine(0, "db_query", toolDone, "", 80))
	if !strings.Contains(out, "db_query") {
		t.Errorf("should contain tool name, got %q", out)
	}
	if strings.Contains(out, "—") {
		t.Error("should not show separator when reason is empty")
	}
}

func TestParseToolCallStart(t *testing.T) {
	tests := []struct {
		body       string
		wantName   string
		wantReason string
	}{
		{
			`{"name":"message_send","input":"{\"reason\":\"reply to user\"}"}`,
			"message_send", "reply to user",
		},
		{
			`{"name":"db_query","input":"{\"reason\":\"check status\"}"}`,
			"db_query", "check status",
		},
		{`{"name":"done","input":"{}"}`, "done", ""},
		{`{}`, "", ""},
		{`invalid`, "", ""},
	}
	for _, tt := range tests {
		name, reason := parseToolCallStart(tt.body)
		if name != tt.wantName {
			t.Errorf("parseToolCallStart(%q) name = %q, want %q", tt.body, name, tt.wantName)
		}
		if reason != tt.wantReason {
			t.Errorf("parseToolCallStart(%q) reason = %q, want %q", tt.body, reason, tt.wantReason)
		}
	}
}

func TestResolveToolCallState(t *testing.T) {
	tests := []struct {
		name   string
		paired *db.Message
		want   toolState
	}{
		{"nil paired", nil, toolRunning},
		{"done", &db.Message{Body: `{"output":"{\"sent\":true}"}`}, toolDone},
		{"error", &db.Message{Body: `{"output":"{\"error\":\"table not found\"}"}`}, toolError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveToolCallState(tt.paired)
			if got != tt.want {
				t.Errorf("resolveToolCallState() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRenderConversation_ToolCalls(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)

	msgs := []db.Message{
		{ID: "1", Body: "do something", Kind: "text", ContactID: db.OwnerContactID, SentAt: base},
		{ID: "start-1", Body: `{"name":"db_query","input":"{\"reason\":\"check data\"}","round":1}`, Kind: "tool_call_start", ContactID: db.SystemContactID, IsFromMe: true, Platform: "system", SentAt: base.Add(time.Second)},
		{ID: "call-1", Body: `{"name":"db_query","input":"{\"reason\":\"check data\"}","output":"1","round":1}`, Kind: "tool_call", ContactID: db.SystemContactID, IsFromMe: true, Platform: "system", SentAt: base.Add(time.Second), ContextStanzaID: sql.NullString{Valid: true, String: "start-1"}},
		{ID: "start-2", Body: `{"name":"message_send","input":"{\"reason\":\"reply to user\"}","round":1}`, Kind: "tool_call_start", ContactID: db.SystemContactID, IsFromMe: true, Platform: "system", SentAt: base.Add(2 * time.Second)},
		{ID: "call-2", Body: `{"name":"message_send","input":"{\"reason\":\"reply to user\"}","output":"ok","round":1}`, Kind: "tool_call", ContactID: db.SystemContactID, IsFromMe: true, Platform: "system", SentAt: base.Add(2 * time.Second), ContextStanzaID: sql.NullString{Valid: true, String: "start-2"}},
		{ID: "4", Body: "here you go", Kind: "text", ContactID: db.SystemContactID, IsFromMe: true, SentAt: base.Add(3 * time.Second)},
	}

	out := stripAnsi(renderConversation(msgs, 80, 0, -1, "", false))

	if !strings.Contains(out, "db_query") {
		t.Error("expected db_query tool call in output")
	}
	if strings.Contains(out, "message_send") {
		t.Error("message_send tool call should be filtered from output")
	}
	if !strings.Contains(out, "check data") {
		t.Error("expected reason 'check data' in output")
	}
	if strings.Contains(out, "reply to user") {
		t.Error("message_send reason 'reply to user' should be filtered from output")
	}
	if !strings.Contains(out, "✓") {
		t.Error("expected checkmarks for completed tool calls")
	}
	if !strings.Contains(out, "╭") {
		t.Error("expected tool rail cap")
	}
	if !strings.Contains(out, "╰") {
		t.Error("expected tool rail closing cap")
	}
}

func TestRenderConversation_DoneToolCallFiltered(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)

	msgs := []db.Message{
		{ID: "start-done", Body: `{"name":"done","input":"{\"reason\":\"nothing to do\"}"}`, Kind: "tool_call_start", ContactID: db.SystemContactID, IsFromMe: true, Platform: "system", SentAt: base},
	}

	out := stripAnsi(renderConversation(msgs, 80, 0, -1, "", false))

	if strings.Contains(out, "nothing to do") {
		t.Error("done tool call starts should be filtered out")
	}
}

func TestRenderConversation_InProgressToolCall(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)

	msgs := []db.Message{
		{ID: "start-1", Body: `{"name":"db_query","input":"{\"reason\":\"fetching rows\"}"}`, Kind: "tool_call_start", ContactID: db.SystemContactID, IsFromMe: true, Platform: "system", SentAt: base},
	}

	out := stripAnsi(renderConversation(msgs, 80, 5, -1, "", false))

	if !strings.Contains(out, "db_query") {
		t.Error("expected in-progress tool call name")
	}
	if !strings.Contains(out, "fetching rows") {
		t.Error("expected reason for in-progress tool call")
	}
	if strings.Contains(out, "✓") {
		t.Error("in-progress tool call should not have checkmark")
	}
}

func TestSameGroup(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)

	tests := []struct {
		name string
		a, b entry
		want bool
	}{
		{
			"same sender close time",
			entry{lt: base, isNik: true},
			entry{lt: base.Add(time.Minute), isNik: true},
			true,
		},
		{
			"different sender",
			entry{lt: base, isNik: true},
			entry{lt: base.Add(time.Minute), isNik: false},
			false,
		},
		{
			"gap over 5min",
			entry{lt: base, isNik: true},
			entry{lt: base.Add(6 * time.Minute), isNik: true},
			false,
		},
		{
			"exactly 5min",
			entry{lt: base, isNik: true},
			entry{lt: base.Add(5 * time.Minute), isNik: true},
			true,
		},
		{
			"different date",
			entry{lt: time.Date(2026, 1, 1, 23, 59, 0, 0, time.Local), isNik: true},
			entry{lt: time.Date(2026, 1, 2, 0, 1, 0, 0, time.Local), isNik: true},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sameGroup(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("sameGroup() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRenderConversation_ToolCallsSplitByTimeGap(t *testing.T) {
	base := time.Date(2026, 1, 1, 11, 14, 0, 0, time.Local)

	msgs := []db.Message{
		{ID: "start-1", Body: `{"name":"task_cancel","input":"{\"reason\":\"stop current task\"}","round":1}`, Kind: "tool_call_start", ContactID: db.SystemContactID, IsFromMe: true, Platform: "system", SentAt: base},
		{ID: "call-1", Body: `{"name":"task_cancel","input":"{}","output":"{\"ok\":true}","round":1}`, Kind: "tool_call", ContactID: db.SystemContactID, IsFromMe: true, Platform: "system", SentAt: base, ContextStanzaID: sql.NullString{Valid: true, String: "start-1"}},
		{ID: "start-2", Body: `{"name":"load_skill","input":"{\"reason\":\"skill reflex fired\"}","round":1}`, Kind: "tool_call_start", ContactID: db.SystemContactID, IsFromMe: true, Platform: "system", SentAt: base.Add(46 * time.Minute)},
		{ID: "call-2", Body: `{"name":"load_skill","input":"{}","output":"{\"ok\":true}","round":1}`, Kind: "tool_call", ContactID: db.SystemContactID, IsFromMe: true, Platform: "system", SentAt: base.Add(46 * time.Minute), ContextStanzaID: sql.NullString{Valid: true, String: "start-2"}},
	}

	out := stripAnsi(renderConversation(msgs, 80, 0, -1, "", false))

	openCount := strings.Count(out, "╭")
	closeCount := strings.Count(out, "╰")
	if openCount != 2 {
		t.Errorf("expected 2 rail opens, got %d\n%s", openCount, out)
	}
	if closeCount != 2 {
		t.Errorf("expected 2 rail closes, got %d\n%s", closeCount, out)
	}

	if !strings.Contains(out, "task_cancel") {
		t.Error("expected task_cancel in output")
	}
	if !strings.Contains(out, "load_skill") {
		t.Error("expected load_skill in output")
	}
}

func TestChatPulseAdvancesOnActivity(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	c := newTestChat(t, conn, nil, Options{})
	c, _ = c.Update(newMessagesMsg{activity: []string{"thinking"}})
	c, _ = c.Update(pulseTickMsg{tag: 0})

	if c.pulse.Energy() <= 0 {
		t.Errorf("expected energy > 0 after pulse tick, got %f", c.pulse.Energy())
	}
	if c.pulse.Tick() != 1 {
		t.Errorf("expected pulse tick 1, got %d", c.pulse.Tick())
	}
}

func TestChatMessagesCapped(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	c := newTestChat(t, conn, nil, Options{})
	c, _ = c.Update(tea.WindowSizeMsg{Width: 80, Height: 30})

	var msgs []db.Message
	for i := 0; i < 600; i++ {
		msgs = append(msgs, db.Message{
			ID:        fmt.Sprintf("msg-%d", i),
			Body:      fmt.Sprintf("message %d", i),
			Kind:      "text",
			ContactID: db.OwnerContactID,
			SentAt:    time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC),
		})
	}

	c, _ = c.Update(newMessagesMsg{messages: msgs})

	if len(c.messages) != 500 {
		t.Errorf("expected 500 messages after cap, got %d", len(c.messages))
	}
	if c.messages[0].ID != "msg-100" {
		t.Errorf("expected oldest kept message msg-100, got %q", c.messages[0].ID)
	}
	if c.lastID != "msg-599" {
		t.Errorf("expected lastID msg-599, got %q", c.lastID)
	}
}

func TestChatConvCacheDirtyOnNewMessages(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	c := newTestChat(t, conn, nil, Options{})
	c, _ = c.Update(tea.WindowSizeMsg{Width: 80, Height: 30})

	initial := c.convCache

	c, _ = c.Update(newMessagesMsg{
		messages: []db.Message{
			{ID: "1", Body: "hello", Kind: "text", ContactID: db.OwnerContactID, SentAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)},
		},
	})

	if c.convCache == initial {
		t.Error("expected convCache to change after new messages")
	}
	if !strings.Contains(c.convCache, "hello") {
		t.Error("expected convCache to contain message text")
	}
}

func TestChatConvCacheStableOnPulseTick(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	c := newTestChat(t, conn, nil, Options{})
	c, _ = c.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	c, _ = c.Update(newMessagesMsg{
		messages: []db.Message{
			{ID: "1", Body: "hello", Kind: "text", ContactID: db.OwnerContactID, SentAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)},
		},
		activity: []string{"thinking"},
	})

	cached := c.convCache

	c, _ = c.Update(pulseTickMsg{tag: c.pulse.tag})
	c, _ = c.Update(pulseTickMsg{tag: c.pulse.tag})

	if c.convCache != cached {
		t.Error("expected convCache to remain stable across pulse ticks within the same frame window")
	}
}

func TestBubble_WithReactions(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		reactions []string
	}{
		{"single", "hello world", []string{"👍"}},
		{"multiple", "sounds good", []string{"👍", "❤️", "🔥"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := stripAnsi(bubble(tt.body, true, false, 80, tt.reactions))
			lines := strings.Split(out, "\n")
			lastLine := lines[len(lines)-1]

			if !strings.Contains(out, tt.body) {
				t.Errorf("expected body %q in bubble", tt.body)
			}
			for _, emoji := range tt.reactions {
				if !strings.Contains(out, emoji) {
					t.Errorf("expected %s in bubble output", emoji)
				}
				if !strings.Contains(lastLine, emoji) {
					t.Errorf("expected %s in bottom border, got %q", emoji, lastLine)
				}
			}
			if !strings.Contains(lastLine, "╰") || !strings.Contains(lastLine, "╯") {
				t.Errorf("expected bottom border characters, got %q", lastLine)
			}
		})
	}
}

func TestBubble_ShortTextWidensForReaction(t *testing.T) {
	withReaction := stripAnsi(bubble("hi", true, false, 80, []string{"👍"}))
	without := stripAnsi(bubble("hi", true, false, 80, nil))

	withLines := strings.Split(withReaction, "\n")
	withoutLines := strings.Split(without, "\n")

	if len(withLines[0]) <= len(withoutLines[0]) {
		t.Error("expected bubble with reaction to be at least as wide as without")
	}
}

func TestRenderConversation_Reactions(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)

	msgs := []db.Message{
		{ID: "1", ExternalMessageID: "ext-1", Body: "hello", Kind: "text", ContactID: db.OwnerContactID, SentAt: base},
		{ID: "2", ExternalMessageID: "ext-2", Body: "hi there", Kind: "text", ContactID: db.SystemContactID, IsFromMe: true, SentAt: base.Add(time.Minute)},
		{ID: "3", ExternalMessageID: "ext-3", Body: "👍", Kind: "reaction", ContactID: db.SystemContactID, IsFromMe: true, SentAt: base.Add(2 * time.Minute), ContextStanzaID: sql.NullString{Valid: true, String: "ext-1"}},
		{ID: "4", ExternalMessageID: "ext-4", Body: "❤️", Kind: "reaction", ContactID: db.OwnerContactID, SentAt: base.Add(3 * time.Minute), ContextStanzaID: sql.NullString{Valid: true, String: "ext-2"}},
	}

	out := stripAnsi(renderConversation(msgs, 80, 0, -1, "", false))

	if !strings.Contains(out, "hello") {
		t.Error("expected hello in output")
	}
	if !strings.Contains(out, "👍") {
		t.Error("expected 👍 reaction in output")
	}
	if !strings.Contains(out, "❤️") {
		t.Error("expected ❤️ reaction in output")
	}
}

func TestRenderConversation_ReactionRemoval(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)

	msgs := []db.Message{
		{ID: "1", ExternalMessageID: "ext-1", Body: "hello", Kind: "text", ContactID: db.OwnerContactID, SentAt: base},
		{ID: "2", ExternalMessageID: "ext-2", Body: "👍", Kind: "reaction", ContactID: db.SystemContactID, IsFromMe: true, SentAt: base.Add(time.Minute), ContextStanzaID: sql.NullString{Valid: true, String: "ext-1"}},
		{ID: "3", ExternalMessageID: "ext-3", Body: "", Kind: "reaction", ContactID: db.SystemContactID, IsFromMe: true, SentAt: base.Add(2 * time.Minute), ContextStanzaID: sql.NullString{Valid: true, String: "ext-1"}},
	}

	out := stripAnsi(renderConversation(msgs, 80, 0, -1, "", false))

	if !strings.Contains(out, "hello") {
		t.Error("expected hello in output")
	}
	if strings.Contains(out, "👍") {
		t.Error("expected 👍 to be removed after empty-body reaction")
	}
}

func TestCollectReactions(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		msgs := []db.Message{
			{ID: "1", ExternalMessageID: "ext-1", Kind: "text"},
			{ID: "2", ExternalMessageID: "ext-2", Kind: "reaction", Body: "👍", ContactID: "c1", ContextStanzaID: sql.NullString{Valid: true, String: "ext-1"}},
		}

		got := collectReactions(msgs)
		emojis := got["ext-1"]
		if len(emojis) != 1 || emojis[0] != "👍" {
			t.Errorf("expected [👍], got %v", emojis)
		}
	})

	t.Run("removal", func(t *testing.T) {
		msgs := []db.Message{
			{ID: "1", ExternalMessageID: "ext-1", Kind: "text"},
			{ID: "2", ExternalMessageID: "ext-2", Kind: "reaction", Body: "👍", ContactID: "c1", ContextStanzaID: sql.NullString{Valid: true, String: "ext-1"}},
			{ID: "3", ExternalMessageID: "ext-3", Kind: "reaction", Body: "", ContactID: "c1", ContextStanzaID: sql.NullString{Valid: true, String: "ext-1"}},
		}

		got := collectReactions(msgs)
		if len(got["ext-1"]) != 0 {
			t.Errorf("expected no reactions after removal, got %v", got["ext-1"])
		}
	})

	t.Run("multiple senders", func(t *testing.T) {
		msgs := []db.Message{
			{ID: "1", ExternalMessageID: "ext-1", Kind: "text"},
			{ID: "2", ExternalMessageID: "ext-2", Kind: "reaction", Body: "👍", ContactID: "c1", ContextStanzaID: sql.NullString{Valid: true, String: "ext-1"}},
			{ID: "3", ExternalMessageID: "ext-3", Kind: "reaction", Body: "❤️", ContactID: "c2", ContextStanzaID: sql.NullString{Valid: true, String: "ext-1"}},
		}

		got := collectReactions(msgs)
		emojis := got["ext-1"]
		if len(emojis) != 2 {
			t.Fatalf("expected 2 reactions, got %v", emojis)
		}
		if emojis[0] != "👍" || emojis[1] != "❤️" {
			t.Errorf("expected [👍 ❤️], got %v", emojis)
		}
	})

	t.Run("duplicate emoji gets count", func(t *testing.T) {
		msgs := []db.Message{
			{ID: "1", ExternalMessageID: "ext-1", Kind: "text"},
			{ID: "2", ExternalMessageID: "ext-2", Kind: "reaction", Body: "👍", ContactID: "c1", ContextStanzaID: sql.NullString{Valid: true, String: "ext-1"}},
			{ID: "3", ExternalMessageID: "ext-3", Kind: "reaction", Body: "👍", ContactID: "c2", ContextStanzaID: sql.NullString{Valid: true, String: "ext-1"}},
		}

		got := collectReactions(msgs)
		emojis := got["ext-1"]
		if len(emojis) != 1 || emojis[0] != "👍2" {
			t.Errorf("expected [👍2], got %v", emojis)
		}
	})

	t.Run("orphan reaction ignored", func(t *testing.T) {
		msgs := []db.Message{
			{ID: "1", ExternalMessageID: "ext-1", Kind: "text"},
			{ID: "2", ExternalMessageID: "ext-2", Kind: "reaction", Body: "👍", ContactID: "c1", ContextStanzaID: sql.NullString{Valid: true, String: "ext-unknown"}},
		}

		got := collectReactions(msgs)
		if len(got) != 0 {
			t.Errorf("expected no reactions for orphan, got %v", got)
		}
	})
}

func TestWorkloadCmdPullsRealCounts(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	if err := db.OwnerContactEnsure(ctx, conn); err != nil {
		t.Fatalf("ensure owner contact: %v", err)
	}
	if err := db.LocalConversationEnsure(ctx, conn); err != nil {
		t.Fatalf("ensure local conversation: %v", err)
	}

	if _, err := db.AlarmCreate(ctx, conn, db.AlarmCreateParams{
		OriginConversationID: db.LocalConversationID,
		Goal:                 "ring me",
		NextFireAt:           time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("seed alarm: %v", err)
	}

	if err := db.TaskInsert(ctx, conn, db.TaskInsertParams{
		ID:             "task-1",
		ConversationID: db.LocalConversationID,
		Goal:           "do thing",
		Plan:           "p",
		Thinking:       "low",
		Status:         "running",
		CreatedAt:      time.Now(),
	}); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	msg := workloadCmd(conn, 0)()
	wl, ok := msg.(workloadMsg)
	if !ok {
		t.Fatalf("expected workloadMsg, got %T", msg)
	}
	if wl.alarms != 1 {
		t.Errorf("expected 1 alarm, got %d", wl.alarms)
	}
	if wl.tasks != 1 {
		t.Errorf("expected 1 task, got %d", wl.tasks)
	}

	c := newChatModel(&config.Config{Home: t.TempDir()}, conn, nil, Options{})
	c, _ = c.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	c, _ = c.Update(wl)
	out := stripAnsi(c.renderHeader())
	if !strings.Contains(out, "1 alarm") {
		t.Errorf("expected '1 alarm' in header, got %q", out)
	}
	if !strings.Contains(out, "1 task") {
		t.Errorf("expected '1 task' in header, got %q", out)
	}
}

func TestViewportScrollKeys(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	c := newTestChat(t, conn, nil, Options{})
	c, _ = c.Update(tea.WindowSizeMsg{Width: 80, Height: 15})

	var msgs []db.Message
	for i := 0; i < 50; i++ {
		msgs = append(msgs, db.Message{
			ID:        fmt.Sprintf("m-%d", i),
			Body:      fmt.Sprintf("line %d", i),
			Kind:      "text",
			ContactID: db.OwnerContactID,
			SentAt:    time.Date(2026, 1, 1, 12, 0, i, 0, time.Local),
		})
	}
	c, _ = c.Update(newMessagesMsg{messages: msgs})

	c.viewport.GotoBottom()
	bottomY := c.viewport.YOffset()

	c, _ = c.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	if c.viewport.YOffset() >= bottomY {
		t.Errorf("expected pgup to scroll up, offset stayed at %d", c.viewport.YOffset())
	}
}

func TestViewportMouseWheel(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	c := newTestChat(t, conn, nil, Options{})
	c, _ = c.Update(tea.WindowSizeMsg{Width: 80, Height: 15})

	var msgs []db.Message
	for i := 0; i < 50; i++ {
		msgs = append(msgs, db.Message{
			ID:        fmt.Sprintf("m-%d", i),
			Body:      fmt.Sprintf("line %d", i),
			Kind:      "text",
			ContactID: db.OwnerContactID,
			SentAt:    time.Date(2026, 1, 1, 12, 0, i, 0, time.Local),
		})
	}
	c, _ = c.Update(newMessagesMsg{messages: msgs})

	c.viewport.GotoBottom()
	bottomY := c.viewport.YOffset()

	c, _ = c.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if c.viewport.YOffset() >= bottomY {
		t.Errorf("expected mouse wheel up to scroll viewport, offset stayed at %d", c.viewport.YOffset())
	}

	afterUp := c.viewport.YOffset()
	c, _ = c.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if c.viewport.YOffset() <= afterUp {
		t.Errorf("expected mouse wheel down to scroll viewport back, offset stayed at %d", c.viewport.YOffset())
	}
}

func TestViewportStickyBottom(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	c := newTestChat(t, conn, nil, Options{})
	c, _ = c.Update(tea.WindowSizeMsg{Width: 80, Height: 15})

	var msgs []db.Message
	for i := 0; i < 40; i++ {
		msgs = append(msgs, db.Message{
			ID:        fmt.Sprintf("m-%d", i),
			Body:      fmt.Sprintf("line %d", i),
			Kind:      "text",
			ContactID: db.OwnerContactID,
			SentAt:    time.Date(2026, 1, 1, 12, 0, i, 0, time.Local),
		})
	}
	c, _ = c.Update(newMessagesMsg{messages: msgs})
	c.viewport.GotoBottom()

	more := []db.Message{{
		ID: "new", Body: "fresh message", Kind: "text",
		ContactID: db.OwnerContactID,
		SentAt:    time.Date(2026, 1, 1, 13, 0, 0, 0, time.Local),
	}}
	c, _ = c.Update(newMessagesMsg{messages: more})

	if !c.viewport.AtBottom() {
		t.Error("expected viewport to stay at bottom after new messages when already at bottom")
	}
}

func TestLoadGenesisStartedAt(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	started := time.Date(2025, 1, 1, 8, 0, 0, 0, time.UTC)
	if err := db.SettingSet(ctx, conn, db.GenesisStartedAtKey, db.ISO8601MS(started)); err != nil {
		t.Fatalf("set setting: %v", err)
	}

	msg := loadGenesisCmd(conn)()
	loaded, ok := msg.(genesisLoadedMsg)
	if !ok {
		t.Fatalf("expected genesisLoadedMsg, got %T", msg)
	}
	if !loaded.at.Equal(started) {
		t.Errorf("expected loaded.at=%v, got %v", started, loaded.at)
	}
}

func TestGhostReservationKeepsHeightStable(t *testing.T) {
	now := time.Now()
	nikMsg := db.Message{
		ID: "1", ExternalMessageID: "e1", Platform: "local",
		Kind: "text", Body: "hi", IsFromMe: true, SentAt: now, ContactID: db.NikContactID,
	}
	userMsg := db.Message{
		ID: "2", ExternalMessageID: "e2", Platform: "local",
		Kind: "text", Body: "yo", IsFromMe: false, SentAt: now, ContactID: db.OwnerContactID,
	}

	countLines := func(s string) int { return strings.Count(s, "\n") }

	cases := []struct {
		name string
		msgs []db.Message
	}{
		{"empty", nil},
		{"nik-last", []db.Message{nikMsg}},
		{"user-last", []db.Message{userMsg}},
		{"user-then-nik", []db.Message{userMsg, nikMsg}},
		{"nik-then-user", []db.Message{nikMsg, userMsg}},
	}

	for _, tc := range cases {
		noGhost := renderConversation(tc.msgs, 80, 0, -1, "", false)
		withGhost := renderConversation(tc.msgs, 80, 0, 0, "thinking", false)
		if countLines(noGhost) != countLines(withGhost) {
			t.Errorf("%s: line count differs — no-ghost=%d with-ghost=%d\nno-ghost:\n%s\nwith-ghost:\n%s",
				tc.name, countLines(noGhost), countLines(withGhost), noGhost, withGhost)
		}
	}
}
