package db

import (
	"context"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/id"
)

func TestActivationInsertPersistsRow(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedActivationConv(t, conn)

	now := time.Now().UTC().Truncate(time.Second)
	row := ActivationRow{
		ID:              id.V7(),
		ConversationID:  convID,
		Sources:         `["message"]`,
		Model:           "gpt-5",
		ReasoningEffort: "medium",
		InputTokens:     1000,
		OutputTokens:    500,
		TotalTokens:     1500,
		CachedTokens:    200,
		ReasoningTokens: 100,
		CostUSD:         0.0042,
		ToolCallCount:   3,
		DurationMS:      1234,
		CreatedAt:       now,
	}

	err = ActivationInsert(ctx, conn, row)
	if err != nil {
		t.Fatalf("insert activation: %v", err)
	}

	var got struct {
		id      string
		convID  string
		sources string
		model   string
		tokens  int64
		errStr  string
	}

	err = conn.QueryRowContext(ctx,
		"SELECT id, conversation_id, sources, model, total_tokens, error FROM activation WHERE id = ?1",
		row.ID,
	).Scan(&got.id, &got.convID, &got.sources, &got.model, &got.tokens, &got.errStr)
	if err != nil {
		t.Fatalf("query activation: %v", err)
	}

	if got.id != row.ID {
		t.Fatalf("expected id %q, got %q", row.ID, got.id)
	}
	if got.convID != convID {
		t.Fatalf("expected conversation_id %q, got %q", convID, got.convID)
	}
	if got.sources != `["message"]` {
		t.Fatalf("expected sources [\"message\"], got %q", got.sources)
	}
	if got.tokens != 1500 {
		t.Fatalf("expected 1500 total_tokens, got %d", got.tokens)
	}
	if got.errStr != row.Error {
		t.Fatalf("expected error %q, got %q", row.Error, got.errStr)
	}
}

func TestActivationUpdateStatsPersistsRow(t *testing.T) {
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
		Sources:        `["message"]`,
		Model:          "gpt-5",
		CreatedAt:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("insert activation: %v", err)
	}

	output := "replied to kevin about weather\n\n- **Perceive**: kevin asking about weather"
	errText := "complete round 3: context deadline exceeded"

	err = ActivationUpdateStats(ctx, conn, actID, ActivationStatsUpdate{
		InputTokens:    1000,
		OutputTokens:   200,
		TotalTokens:    1200,
		RoundCount:     4,
		MaxInputTokens: 300,
		MaxTotalTokens: 360,
		DurationMS:     500,
		Error:          errText,
		Output:         output,
	})
	if err != nil {
		t.Fatalf("update stats: %v", err)
	}

	var gotOutput, gotErr string
	var gotRoundCount int
	var gotMaxInput int64
	var gotMaxTotal int64
	err = conn.QueryRowContext(ctx,
		"SELECT output, error, round_count, max_input_tokens_per_round, max_total_tokens_per_round FROM activation WHERE id = ?1",
		actID,
	).Scan(&gotOutput, &gotErr, &gotRoundCount, &gotMaxInput, &gotMaxTotal)
	if err != nil {
		t.Fatalf("query output: %v", err)
	}

	if gotOutput != output {
		t.Fatalf("expected output %q, got %q", output, gotOutput)
	}
	if gotErr != errText {
		t.Fatalf("expected error %q, got %q", errText, gotErr)
	}

	if gotRoundCount != 4 {
		t.Fatalf("expected round_count 4, got %d", gotRoundCount)
	}
	if gotMaxInput != 300 {
		t.Fatalf("expected max_input_tokens_per_round 300, got %d", gotMaxInput)
	}
	if gotMaxTotal != 360 {
		t.Fatalf("expected max_total_tokens_per_round 360, got %d", gotMaxTotal)
	}
}

func seedActivationConv(t *testing.T, conn DBTX) string {
	t.Helper()
	convID := id.V7()
	_, err := conn.ExecContext(context.Background(),
		"INSERT INTO conversation (id, platform, external_conversation_id) VALUES (?, 'whatsapp', ?)",
		convID, "ext-"+convID)
	if err != nil {
		t.Fatalf("seed conversation: %v", err)
	}
	return convID
}
