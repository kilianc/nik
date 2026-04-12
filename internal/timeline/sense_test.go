package timeline

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/contacts"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/messaging"
)

func setupTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	ctx := context.Background()

	_, err = db.ContactUpsert(ctx, conn, db.ContactUpsertParams{
		Platform:      "whatsapp",
		ExternalID:    "sender@s.whatsapp.net",
		Name:          "Sender",
		Phone:         "11111",
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	contact, err := db.ContactGet(ctx, conn, "sender@s.whatsapp.net")
	if err != nil {
		t.Fatalf("get contact: %v", err)
	}

	now := time.Now()
	err = db.ConversationUpsert(ctx, conn, db.ConversationUpsertParams{
		Platform:               "whatsapp",
		ExternalConversationID: "ext-conv@s.whatsapp.net",
		Kind:                   "dm",
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("seed conversation: %v", err)
	}

	conv, err := db.ConversationGet(ctx, conn, db.ConversationGetParams{
		Platform:               "whatsapp",
		ExternalConversationID: "ext-conv@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}

	_ = contact
	return conn, conv.ID
}

func insertMsg(t *testing.T, conn *sql.DB, convID string, id string, extMsgID string, kind string, body string, sentAt time.Time) {
	t.Helper()

	contact, err := db.ContactGet(context.Background(), conn, "sender@s.whatsapp.net")
	if err != nil {
		t.Fatalf("get contact: %v", err)
	}

	err = db.MessageInsert(context.Background(), conn, db.MessageInsertParams{
		ID: id, ConversationID: convID, ContactID: contact.ID,
		Platform: "whatsapp", ExternalConversationID: "ext-conv@s.whatsapp.net",
		ExternalMessageID: extMsgID, ExternalSenderID: "sender@s.whatsapp.net",
		SentAt: sentAt, Kind: kind, Body: body,
		ContextMentionedIDs: "[]",
	})
	if err != nil {
		t.Fatalf("insert message %s: %v", id, err)
	}
}

func TestMessageEntryReaction(t *testing.T) {
	longBody := strings.Repeat("a", 250)

	tests := []struct {
		name       string
		targetID   string
		targetKind string
		targetBody string
		emoji      string
		wantText   string
		wantFrom   string
	}{
		{
			"to text",
			"ext-target-1", "text", "personal", "📚",
			`(📚) (reacting to [09:12:30] Sender: personal)`,
			"YOU",
		},
		{
			"to media",
			"ext-target-2", "image", "", "❤️",
			"(❤️) (reacting to [09:12:30] Sender: (image))",
			"YOU",
		},
		{
			"removed reaction",
			"ext-target-3", "text", "personal", "",
			`(removed reaction) (reacting to [09:12:30] Sender: personal)`,
			"YOU",
		},
		{
			"truncates long body",
			"ext-target-4", "text", longBody, "🔥",
			`(🔥) (reacting to [09:12:30] Sender: ` + longBody[:200] + `…)`,
			"YOU",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, convID := setupTestDB(t)

			now := time.Date(2026, 3, 14, 9, 12, 30, 0, time.UTC)
			insertMsg(t, conn, convID, "target-"+tt.name, tt.targetID, tt.targetKind, tt.targetBody, now)

			reaction := db.Message{
				ID: "react-" + tt.name, Kind: "reaction", Body: tt.emoji,
				IsFromMe: true, Platform: "whatsapp",
				ContextStanzaID: sql.NullString{Valid: true, String: tt.targetID},
				SentAt:          now.Add(time.Second),
			}

			e := messageEntry(reaction, "", conn)

			if e.text != tt.wantText {
				t.Fatalf("got %q, want %q", e.text, tt.wantText)
			}
			if tt.wantFrom != "" && e.from != tt.wantFrom {
				t.Fatalf("expected from %q, got %q", tt.wantFrom, e.from)
			}
		})
	}
}

func TestMessageEntryTargetMissing(t *testing.T) {
	conn, _ := setupTestDB(t)

	t.Run("reaction", func(t *testing.T) {
		reaction := db.Message{
			ID: "react-3", Kind: "reaction", Body: "👍",
			IsFromMe: true, Platform: "whatsapp",
			ContextStanzaID: sql.NullString{Valid: true, String: "ext-not-in-db"},
			SentAt:          time.Now(),
		}
		e := messageEntry(reaction, "", conn)
		if e.text != "(👍)" {
			t.Fatalf("expected fallback without target, got %q", e.text)
		}
	})

	t.Run("reply", func(t *testing.T) {
		reply := db.Message{
			ID: "reply-2", Kind: "text", Body: "where?",
			IsFromMe: false, Platform: "whatsapp",
			ContextStanzaID: sql.NullString{Valid: true, String: "ext-missing"},
			SentAt:          time.Now(),
		}
		e := messageEntry(reply, "Alice", conn)
		if e.text != "where?" {
			t.Fatalf("expected fallback without target, got %q", e.text)
		}
	})
}

func TestMessageEntryReplyContext(t *testing.T) {
	conn, convID := setupTestDB(t)
	now := time.Date(2026, 3, 14, 9, 12, 30, 0, time.UTC)

	t.Run("in window", func(t *testing.T) {
		insertMsg(t, conn, convID, "target-reply", "ext-reply-target", "text", "ok", now)

		reply := db.Message{
			ID: "reply-1", Kind: "text", Body: "where?",
			IsFromMe: false, Platform: "whatsapp",
			ContextStanzaID: sql.NullString{Valid: true, String: "ext-reply-target"},
			SentAt:          now.Add(time.Second),
		}
		e := messageEntry(reply, "Alice", conn)

		want := `where? (quote replying to [09:12:30] Sender: ok)`
		if e.text != want {
			t.Fatalf("got %q, want %q", e.text, want)
		}
		if e.from != "Alice" {
			t.Fatalf("expected sender Alice, got %q", e.from)
		}
	})

	t.Run("out of window", func(t *testing.T) {
		oldTime := time.Date(2026, 3, 14, 8, 30, 15, 0, time.UTC)
		replyTime := time.Date(2026, 3, 14, 14, 0, 0, 0, time.UTC)

		insertMsg(t, conn, convID, "old-msg", "ext-old", "text", "how about saturday?", oldTime)

		reply := db.Message{
			ID: "reply-3", Kind: "text", Body: "yes!",
			IsFromMe: false, Platform: "whatsapp",
			ContextStanzaID: sql.NullString{Valid: true, String: "ext-old"},
			SentAt:          replyTime,
		}
		e := messageEntry(reply, "Alice", conn)

		want := `yes! (quote replying to [08:30:15] Sender: how about saturday?)`
		if e.text != want {
			t.Fatalf("got %q, want %q", e.text, want)
		}
	})
}

func TestMessageEntryPlainText(t *testing.T) {
	msg := db.Message{
		ID: "plain-1", Kind: "text", Body: "hello",
		IsFromMe: false, SentAt: time.Now(),
	}

	e := messageEntry(msg, "Bob", nil)

	if e.text != "hello" {
		t.Fatalf("expected 'hello', got %q", e.text)
	}
	if e.from != "Bob" {
		t.Fatalf("expected sender Bob, got %q", e.from)
	}
}

func TestCheckIgnoresDoneToolCall(t *testing.T) {
	conn, convID := setupTestDB(t)
	ctx := context.Background()

	err := db.SystemContactEnsure(ctx, conn)
	if err != nil {
		t.Fatalf("ensure system contact: %v", err)
	}

	now := time.Now().UTC()
	readAt := now.Add(-time.Second).Format("2006-01-02T15:04:05.000Z")
	_, err = conn.ExecContext(ctx, "UPDATE conversation SET last_read_at = ? WHERE id = ?", readAt, convID)
	if err != nil {
		t.Fatalf("set last_read_at: %v", err)
	}

	_, err = db.SystemMessageInsert(ctx, conn, db.SystemMessageParams{
		ConversationID: convID,
		Kind:           "tool_call",
		Body:           map[string]string{"name": "done", "input": `{"reason":"all done"}`, "output": `{"ok":true}`},
		SentAt:         now,
	})
	if err != nil {
		t.Fatalf("insert done tool call: %v", err)
	}

	cfg := &config.Config{
		MaxHistory:           10,
		AllowConversationIDs: config.ConversationList{{Label: "test", ID: convID}},
	}
	msgSvc := messaging.NewService(cfg, conn, contacts.NewService(conn))
	tl := New(cfg, msgSvc)

	stimuli, err := tl.Check(ctx)
	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if len(stimuli) != 0 {
		t.Fatalf("expected no stimuli (done should be filtered), got %d", len(stimuli))
	}
}

func TestToolCallAppearsInTimeline(t *testing.T) {
	conn, convID := setupTestDB(t)
	ctx := context.Background()

	err := db.SystemContactEnsure(ctx, conn)
	if err != nil {
		t.Fatalf("ensure system contact: %v", err)
	}

	_, err = db.SystemMessageInsert(ctx, conn, db.SystemMessageParams{
		ConversationID: convID,
		Kind:           "tool_call",
		Body:           map[string]string{"name": "message_send", "input": `{"body":"hi"}`, "output": `{"sent":1}`},
		SentAt:         time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("insert tool call: %v", err)
	}

	cfg := &config.Config{MaxHistory: 10}
	msgSvc := messaging.NewService(cfg, conn, contacts.NewService(conn))
	tl := New(cfg, msgSvc)

	_, rendered, err := tl.Render(ctx, convID)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	out := strings.Join(rendered, "\n")
	if !strings.Contains(out, "YOU: called message_send") {
		t.Fatalf("expected tool call in rendered timeline, got %q", out)
	}
}

func TestMarkReadUsesMaxSentAt(t *testing.T) {
	conn, convID := setupTestDB(t)
	ctx := context.Background()

	past := time.Date(2026, 3, 14, 10, 0, 0, 0, time.UTC)
	insertMsg(t, conn, convID, "msg-markread", "ext-markread", "text", "hello", past)

	cfg := &config.Config{
		MaxHistory: 10,
	}
	msgSvc := messaging.NewService(cfg, conn, contacts.NewService(conn))
	tl := New(cfg, msgSvc)

	tl.Read(ctx, convID)

	conv, err := db.ConversationGet(ctx, conn, db.ConversationGetParams{ID: convID})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}

	if !conv.LastReadAt.Valid {
		t.Fatal("expected last_read_at to be set")
	}
	if !conv.LastReadAt.Time.Equal(past) {
		t.Fatalf("expected last_read_at = %v (max sent_at), got %v", past, conv.LastReadAt.Time)
	}
}

func TestRenderUsesSystemMessagesOnly(t *testing.T) {
	conn, convID := setupTestDB(t)
	ctx := context.Background()

	err := db.SystemContactEnsure(ctx, conn)
	if err != nil {
		t.Fatalf("ensure system contact: %v", err)
	}

	_, err = db.SystemMessageInsert(ctx, conn, db.SystemMessageParams{
		ConversationID: convID,
		Kind:           "task_report",
		Body: db.TaskReport{
			TaskID:  "aaaa-bbbb-cccc-dddd",
			Goal:    "check build",
			Status:  "running",
			Content: "working",
		},
		SentAt: time.Date(2026, 3, 14, 15, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("insert system message: %v", err)
	}

	cfg := &config.Config{MaxHistory: 10}
	msgSvc := messaging.NewService(cfg, conn, contacts.NewService(conn))
	tl := New(cfg, msgSvc)

	header, rendered, err := tl.Render(ctx, convID)
	if err != nil {
		t.Fatalf("render timeline: %v", err)
	}

	headerOut := strings.Join(header, "\n")
	if !strings.Contains(headerOut, "id: "+convID) {
		t.Fatalf("expected header to include conversation id line, got %q", headerOut)
	}

	full := tl.Read(ctx, convID)
	if !strings.Contains(full, "## Conversation") {
		t.Fatalf("expected timeline output to use conversation header, got %q", full)
	}

	out := strings.Join(rendered, "\n")
	if !strings.Contains(out, "[task report]") {
		t.Fatalf("expected rendered timeline to include task report, got %q", out)
	}
	if !strings.Contains(out, "goal: check build") {
		t.Fatalf("expected rendered timeline to include task goal, got %q", out)
	}
}

func TestSystemMessagesTrimmedByAge(t *testing.T) {
	conn, convID := setupTestDB(t)
	ctx := context.Background()

	err := db.SystemContactEnsure(ctx, conn)
	if err != nil {
		t.Fatalf("ensure system contact: %v", err)
	}

	now := time.Now().UTC()

	insertMsg(t, conn, convID, "msg-old-human", "ext-old-human", "text", "old chat", now.Add(-2*time.Hour))

	_, err = db.SystemMessageInsert(ctx, conn, db.SystemMessageParams{
		ConversationID: convID,
		Kind:           "task_report",
		Body: db.TaskReport{
			TaskID:  "aaaa-bbbb-cccc-dddd",
			Goal:    "stale task",
			Status:  "done",
			Content: "finished long ago",
		},
		SentAt: now.Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("insert old system message: %v", err)
	}

	_, err = db.SystemMessageInsert(ctx, conn, db.SystemMessageParams{
		ConversationID: convID,
		Kind:           "alarm_fired",
		Body:           db.Alarm{ID: "eeee-ffff-0000-1111", Goal: "recent alarm"},
		SentAt:         now.Add(-5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("insert recent system message: %v", err)
	}

	cfg := &config.Config{
		MaxHistory:          10,
		SystemMessageMaxAge: 1 * time.Hour,
	}
	msgSvc := messaging.NewService(cfg, conn, contacts.NewService(conn))
	tl := New(cfg, msgSvc)

	out := tl.Read(ctx, convID)

	if !strings.Contains(out, "old chat") {
		t.Fatalf("expected old conversation message to be kept, got %q", out)
	}
	if strings.Contains(out, "stale task") {
		t.Fatalf("expected stale system message to be trimmed, got %q", out)
	}
	if !strings.Contains(out, "recent alarm") {
		t.Fatalf("expected recent system message to be kept, got %q", out)
	}
}

func TestMessageEntryMentions(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		mentionedIDs []string
		seedContacts []struct {
			externalID string
			name       string
		}
		wantContains    string
		wantNotContains string
	}{
		{
			name:         "resolved mention",
			body:         "@247506181066940 what you think?",
			mentionedIDs: []string{"247506181066940@lid"},
			seedContacts: []struct {
				externalID string
				name       string
			}{
				{"247506181066940@lid", "Penelope"},
			},
			wantContains: "(mentioning Penelope)",
		},
		{
			name:            "unresolved mention",
			body:            "@999999999999999 hey",
			mentionedIDs:    []string{"999999999999999@lid"},
			wantNotContains: "(mentioning",
		},
		{
			name:         "multiple mentions",
			body:         "@111111111111111 @222222222222222 let's go",
			mentionedIDs: []string{"111111111111111@lid", "222222222222222@lid"},
			seedContacts: []struct {
				externalID string
				name       string
			}{
				{"111111111111111@lid", "Alice"},
				{"222222222222222@lid", "Bob"},
			},
			wantContains: "(mentioning Alice, Bob)",
		},
		{
			name:            "empty mentions",
			body:            "no mentions here",
			mentionedIDs:    nil,
			wantNotContains: "(mentioning",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, err := db.OpenInMemory()
			if err != nil {
				t.Fatalf("open in-memory db: %v", err)
			}
			defer conn.Close()

			ctx := context.Background()
			for _, sc := range tt.seedContacts {
				_, err := db.ContactUpsert(ctx, conn, db.ContactUpsertParams{
					Platform:      "whatsapp",
					ExternalID:    sc.externalID,
					Name:          sc.name,
					Phone:         "0000",
					LastMessageAt: time.Now(),
				})
				if err != nil {
					t.Fatalf("seed contact %s: %v", sc.name, err)
				}
			}

			msg := db.Message{
				ID:                  "mention-" + tt.name,
				Kind:                "text",
				Body:                tt.body,
				IsFromMe:            false,
				SentAt:              time.Now(),
				ContextMentionedIDs: tt.mentionedIDs,
			}

			e := messageEntry(msg, "Sender", conn)

			if tt.wantContains != "" && !strings.Contains(e.text, tt.wantContains) {
				t.Fatalf("expected text to contain %q, got %q", tt.wantContains, e.text)
			}
			if tt.wantNotContains != "" && strings.Contains(e.text, tt.wantNotContains) {
				t.Fatalf("expected text to not contain %q, got %q", tt.wantNotContains, e.text)
			}
		})
	}
}

func TestPeekSkipsSystemMessages(t *testing.T) {
	conn, convID := setupTestDB(t)
	ctx := context.Background()

	err := db.SystemContactEnsure(ctx, conn)
	if err != nil {
		t.Fatalf("ensure system contact: %v", err)
	}

	now := time.Date(2026, 3, 14, 10, 0, 0, 0, time.UTC)
	insertMsg(t, conn, convID, "msg-human", "ext-human", "text", "hey nik", now)

	_, err = db.SystemMessageInsert(ctx, conn, db.SystemMessageParams{
		ConversationID: convID,
		Kind:           "task_report",
		Body: db.TaskReport{
			TaskID:  "aaaa-bbbb-cccc-dddd",
			Goal:    "run backup",
			Status:  "running",
			Content: "in progress",
		},
		SentAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("insert system message: %v", err)
	}

	cfg := &config.Config{MaxHistory: 10}
	msgSvc := messaging.NewService(cfg, conn, contacts.NewService(conn))
	tl := New(cfg, msgSvc)

	peek := tl.Peek(ctx, convID)

	if !strings.Contains(peek, "hey nik") {
		t.Fatalf("expected peek to contain human message, got %q", peek)
	}
	if strings.Contains(peek, "task report") || strings.Contains(peek, "run backup") {
		t.Fatalf("expected peek to exclude system messages, got %q", peek)
	}

	conv, err := db.ConversationGet(ctx, conn, db.ConversationGetParams{ID: convID})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}
	if conv.LastReadAt.Valid {
		t.Fatalf("expected peek to not advance last_read_at, got %v", conv.LastReadAt.Time)
	}
}
