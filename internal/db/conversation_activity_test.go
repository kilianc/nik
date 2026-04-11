package db

import (
	"context"
	"slices"
	"testing"
	"time"
)

func seedActivityConv(t *testing.T, conn interface {
	DBTX
	Close() error
}) string {
	t.Helper()
	ctx := context.Background()
	now := time.Now()
	err := ConversationUpsert(ctx, conn, ConversationUpsertParams{
		Platform:               "local",
		ExternalConversationID: "test-conv",
		Kind:                   "dm",
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("seed conversation: %v", err)
	}
	conv, err := ConversationGet(ctx, conn, ConversationGetParams{
		Platform:               "local",
		ExternalConversationID: "test-conv",
	})
	if err != nil {
		t.Fatalf("get seeded conversation: %v", err)
	}
	return conv.ID
}

func getActivity(t *testing.T, conn interface {
	DBTX
	Close() error
}, convID string) []string {
	t.Helper()
	conv, err := ConversationGet(context.Background(), conn, ConversationGetParams{ID: convID})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}
	return conv.Activity
}

func TestConversationActivityPush(t *testing.T) {
	ctx := context.Background()
	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	convID := seedActivityConv(t, conn)

	err = ConversationActivityPush(ctx, conn, convID, "thinking")
	if err != nil {
		t.Fatalf("push thinking: %v", err)
	}

	got := getActivity(t, conn, convID)
	if len(got) != 1 || got[0] != "thinking" {
		t.Fatalf("expected [thinking], got %v", got)
	}

	err = ConversationActivityPush(ctx, conn, convID, "typing")
	if err != nil {
		t.Fatalf("push typing: %v", err)
	}

	got = getActivity(t, conn, convID)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %v", got)
	}
	if !slices.Contains(got, "thinking") || !slices.Contains(got, "typing") {
		t.Fatalf("expected [thinking, typing], got %v", got)
	}
}

func TestConversationActivityPushIdempotent(t *testing.T) {
	ctx := context.Background()
	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	convID := seedActivityConv(t, conn)

	err = ConversationActivityPush(ctx, conn, convID, "thinking")
	if err != nil {
		t.Fatalf("push: %v", err)
	}

	err = ConversationActivityPush(ctx, conn, convID, "thinking")
	if err != nil {
		t.Fatalf("push duplicate: %v", err)
	}

	got := getActivity(t, conn, convID)
	if len(got) != 1 {
		t.Fatalf("expected 1 entry after duplicate push, got %v", got)
	}
}

func TestConversationActivityPop(t *testing.T) {
	ctx := context.Background()
	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	convID := seedActivityConv(t, conn)

	err = ConversationActivityPush(ctx, conn, convID, "thinking")
	if err != nil {
		t.Fatalf("push thinking: %v", err)
	}
	err = ConversationActivityPush(ctx, conn, convID, "typing")
	if err != nil {
		t.Fatalf("push typing: %v", err)
	}

	err = ConversationActivityPop(ctx, conn, convID, "typing")
	if err != nil {
		t.Fatalf("pop typing: %v", err)
	}

	got := getActivity(t, conn, convID)
	if len(got) != 1 || got[0] != "thinking" {
		t.Fatalf("expected [thinking], got %v", got)
	}
}

func TestConversationActivityPopLast(t *testing.T) {
	ctx := context.Background()
	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	convID := seedActivityConv(t, conn)

	err = ConversationActivityPush(ctx, conn, convID, "thinking")
	if err != nil {
		t.Fatalf("push: %v", err)
	}

	err = ConversationActivityPop(ctx, conn, convID, "thinking")
	if err != nil {
		t.Fatalf("pop: %v", err)
	}

	got := getActivity(t, conn, convID)
	if len(got) != 0 {
		t.Fatalf("expected empty activity after pop-last, got %v", got)
	}
}

func TestConversationActivityReset(t *testing.T) {
	ctx := context.Background()
	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	convID := seedActivityConv(t, conn)

	err = ConversationActivityPush(ctx, conn, convID, "thinking")
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	err = ConversationActivityPush(ctx, conn, convID, "typing")
	if err != nil {
		t.Fatalf("push: %v", err)
	}

	err = ConversationActivityReset(ctx, conn)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}

	got := getActivity(t, conn, convID)
	if len(got) != 0 {
		t.Fatalf("expected empty activity after reset, got %v", got)
	}
}
