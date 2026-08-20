package apisvc

import (
	"context"
	"database/sql"
	"testing"

	"github.com/kciuffolo/nik/internal/api"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/contacts"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/messaging"
)

// These run against a real database rather than a fake, because what they are
// checking is the translation — a Message with a dozen sql.Null fields coming
// out the other side as something a browser can render, and "local" resolving
// to the UUID the rows are actually keyed by.
func newTestChat(t *testing.T) (*Chat, *sql.DB) {
	t.Helper()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	ctx := context.Background()
	for _, ensure := range []func(context.Context, *sql.DB) error{
		db.NikContactEnsure,
		db.OwnerContactEnsure,
		db.LocalConversationEnsure,
	} {
		err = ensure(ctx, conn)
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	contactsSvc := contacts.NewService(conn)
	msg := messaging.NewService(&config.Config{}, conn, contactsSvc)
	msg.RegisterPlatform(messaging.NewLocalAdapter(conn))

	return NewChat(conn, msg), conn
}

func TestSendThenReadBack(t *testing.T) {
	chat, _ := newTestChat(t)
	ctx := context.Background()

	err := chat.Send(ctx, api.LocalConversationID, "is the oven on")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	messages, err := chat.Messages(ctx, api.MessagesQuery{
		ConversationID: api.LocalConversationID,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(messages))
	}
	if messages[0].Body != "is the oven on" {
		t.Fatalf("body = %q", messages[0].Body)
	}
	if messages[0].ID == "" {
		t.Fatal("message has no id, so no client can page from it")
	}
}

// Oldest-first is the contract. The query returns newest-first because that is
// what a LIMIT wants, and every reader wants the other order — getting this
// wrong renders a conversation backwards.
func TestMessagesAreOldestFirst(t *testing.T) {
	chat, _ := newTestChat(t)
	ctx := context.Background()

	for _, body := range []string{"first", "second", "third"} {
		err := chat.Send(ctx, api.LocalConversationID, body)
		if err != nil {
			t.Fatalf("Send %q: %v", body, err)
		}
	}

	messages, err := chat.Messages(ctx, api.MessagesQuery{
		ConversationID: api.LocalConversationID,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}

	want := []string{"first", "second", "third"}
	if len(messages) != len(want) {
		t.Fatalf("got %d messages, want %d", len(messages), len(want))
	}
	for i, body := range want {
		if messages[i].Body != body {
			t.Fatalf("message %d = %q, want %q", i, messages[i].Body, body)
		}
	}
}

func TestAfterPagesForward(t *testing.T) {
	chat, _ := newTestChat(t)
	ctx := context.Background()

	err := chat.Send(ctx, api.LocalConversationID, "old news")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	first, err := chat.Messages(ctx, api.MessagesQuery{ConversationID: api.LocalConversationID, Limit: 10})
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}

	err = chat.Send(ctx, api.LocalConversationID, "fresh news")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	rest, err := chat.Messages(ctx, api.MessagesQuery{
		ConversationID: api.LocalConversationID,
		After:          first[len(first)-1].ID,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("Messages after: %v", err)
	}

	if len(rest) != 1 || rest[0].Body != "fresh news" {
		t.Fatalf("after returned %d messages (%v), want just the new one", len(rest), rest)
	}
}

// "local" is an API spelling; the rows are keyed by a UUID. Both have to work,
// or a client that read an id out of a message cannot use it in a URL.
func TestLocalAliasAndRealIDBothResolve(t *testing.T) {
	chat, _ := newTestChat(t)
	ctx := context.Background()

	byAlias, err := chat.Conversation(ctx, api.LocalConversationID)
	if err != nil {
		t.Fatalf("Conversation(local): %v", err)
	}

	byID, err := chat.Conversation(ctx, db.LocalConversationID)
	if err != nil {
		t.Fatalf("Conversation(uuid): %v", err)
	}

	if byAlias.ID != byID.ID {
		t.Fatalf("alias resolved to %q, uuid to %q", byAlias.ID, byID.ID)
	}
	if byAlias.ID != db.LocalConversationID {
		t.Fatalf("conversation id = %q, want the row's real id", byAlias.ID)
	}
}

func TestUnknownConversationIsNotFound(t *testing.T) {
	chat, _ := newTestChat(t)

	_, err := chat.Conversation(context.Background(), "00000000-0000-0000-0000-00000000dead")
	if err != api.ErrNotFound {
		t.Fatalf("err = %v, want api.ErrNotFound", err)
	}
}

// Sending into a WhatsApp thread would put words in nik's mouth: the brain
// decides to speak by calling message_send, and this door skips that.
func TestSendRefusesNonLocalConversations(t *testing.T) {
	chat, _ := newTestChat(t)

	err := chat.Send(context.Background(), "00000000-0000-0000-0000-00000000beef", "hello group")
	if err == nil {
		t.Fatal("sending into a non-local conversation was allowed")
	}
}

// Activity is how a console shows nik thinking. It rides on the conversation,
// and an empty one must be [] rather than null for the same reason messages are.
func TestConversationCarriesActivity(t *testing.T) {
	chat, conn := newTestChat(t)
	ctx := context.Background()

	conv, err := chat.Conversation(ctx, api.LocalConversationID)
	if err != nil {
		t.Fatalf("Conversation: %v", err)
	}
	if conv.Activity == nil {
		t.Fatal("activity is nil, want an empty slice")
	}

	err = db.ConversationActivityPush(ctx, conn, db.LocalConversationID, "typing")
	if err != nil {
		t.Fatalf("push activity: %v", err)
	}

	conv, err = chat.Conversation(ctx, api.LocalConversationID)
	if err != nil {
		t.Fatalf("Conversation: %v", err)
	}
	if len(conv.Activity) != 1 || conv.Activity[0] != "typing" {
		t.Fatalf("activity = %v, want [typing]", conv.Activity)
	}
}
