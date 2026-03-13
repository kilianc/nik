package db

import (
	"context"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/id"
)

func TestActivationDetailInsertPersistsRow(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedActivationConv(t, conn)
	actID := id.V7()

	err = ActivationInsert(ctx, conn, ActivationRow{
		ID:             actID,
		ConversationID: convID,
		Model:          "test-model",
		CreatedAt:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("insert activation: %v", err)
	}

	p := ActivationDetailParams{
		ActivationID:       actID,
		Instructions:       "you are nik",
		UserInput:          "hello",
		Tools:              []string{"message_reply", "shell"},
		ReasoningSummaries: []string{"thinking about greeting"},
	}

	err = ActivationDetailInsert(ctx, conn, p)
	if err != nil {
		t.Fatalf("insert activation detail: %v", err)
	}

	var got struct {
		instructions       string
		userInput          string
		tools              string
		reasoningSummaries string
	}

	err = conn.QueryRowContext(ctx,
		"SELECT instructions, user_input, tools, reasoning_summaries FROM activation_detail WHERE activation_id = ?1",
		actID,
	).Scan(&got.instructions, &got.userInput, &got.tools, &got.reasoningSummaries)
	if err != nil {
		t.Fatalf("query activation detail: %v", err)
	}

	if got.instructions != "you are nik" {
		t.Fatalf("expected instructions %q, got %q", "you are nik", got.instructions)
	}
	if got.userInput != "hello" {
		t.Fatalf("expected user_input %q, got %q", "hello", got.userInput)
	}
	if got.tools != `["message_reply","shell"]` {
		t.Fatalf("expected tools %q, got %q", `["message_reply","shell"]`, got.tools)
	}
	if got.reasoningSummaries != `["thinking about greeting"]` {
		t.Fatalf("expected reasoning_summaries %q, got %q", `["thinking about greeting"]`, got.reasoningSummaries)
	}
}

func TestActivationDetailInsertReplacesOnConflict(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedActivationConv(t, conn)
	actID := id.V7()

	err = ActivationInsert(ctx, conn, ActivationRow{
		ID:             actID,
		ConversationID: convID,
		Model:          "test-model",
		CreatedAt:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("insert activation: %v", err)
	}

	err = ActivationDetailInsert(ctx, conn, ActivationDetailParams{
		ActivationID: actID,
		Instructions: "first",
		UserInput:    "first input",
	})
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}

	err = ActivationDetailInsert(ctx, conn, ActivationDetailParams{
		ActivationID: actID,
		Instructions: "second",
		UserInput:    "second input",
	})
	if err != nil {
		t.Fatalf("second insert: %v", err)
	}

	var got string
	err = conn.QueryRowContext(ctx,
		"SELECT instructions FROM activation_detail WHERE activation_id = ?1",
		actID,
	).Scan(&got)
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	if got != "second" {
		t.Fatalf("expected replaced instructions %q, got %q", "second", got)
	}
}
