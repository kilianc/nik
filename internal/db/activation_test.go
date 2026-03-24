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
	})
	if err != nil {
		t.Fatalf("update stats: %v", err)
	}

	var gotErr string
	var gotRoundCount int
	var gotMaxInput int64
	var gotMaxTotal int64
	err = conn.QueryRowContext(ctx,
		"SELECT error, round_count, max_input_tokens_per_round, max_total_tokens_per_round FROM activation WHERE id = ?1",
		actID,
	).Scan(&gotErr, &gotRoundCount, &gotMaxInput, &gotMaxTotal)
	if err != nil {
		t.Fatalf("query stats: %v", err)
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

func TestActivationUpdateDetailPersistsToolSchemas(t *testing.T) {
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

	schemas := `[{"Name":"db_query","Description":"run a query","Parameters":{"type":"object"}}]`
	err = ActivationUpdateDetail(ctx, conn, actID, "test instructions", []string{"db_query"}, schemas)
	if err != nil {
		t.Fatalf("update detail: %v", err)
	}

	var gotInstructions, gotTools, gotSchemas string
	err = conn.QueryRowContext(ctx,
		"SELECT instructions, tools, tool_schemas FROM activation WHERE id = ?1",
		actID,
	).Scan(&gotInstructions, &gotTools, &gotSchemas)
	if err != nil {
		t.Fatalf("query detail: %v", err)
	}

	if gotInstructions != "test instructions" {
		t.Fatalf("expected instructions %q, got %q", "test instructions", gotInstructions)
	}
	if gotTools != `["db_query"]` {
		t.Fatalf("expected tools %q, got %q", `["db_query"]`, gotTools)
	}
	if gotSchemas != schemas {
		t.Fatalf("expected tool_schemas %q, got %q", schemas, gotSchemas)
	}
}

func TestActivationGet(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedActivationConv(t, conn)
	actID := id.V7()

	err = ActivationInsert(ctx, conn, ActivationRow{
		ID:              actID,
		ConversationID:  convID,
		Model:           "gpt-5",
		ReasoningEffort: "medium",
		CreatedAt:       time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("insert activation: %v", err)
	}

	schemas := `[{"Name":"db_query"}]`
	err = ActivationUpdateDetail(ctx, conn, actID, "test instructions", []string{"db_query"}, schemas)
	if err != nil {
		t.Fatalf("update detail: %v", err)
	}

	got, err := ActivationGet(ctx, conn, actID)
	if err != nil {
		t.Fatalf("get activation: %v", err)
	}

	if got.ID != actID {
		t.Fatalf("expected id %q, got %q", actID, got.ID)
	}
	if got.Model != "gpt-5" {
		t.Fatalf("expected model %q, got %q", "gpt-5", got.Model)
	}
	if got.ReasoningEffort != "medium" {
		t.Fatalf("expected reasoning_effort %q, got %q", "medium", got.ReasoningEffort)
	}
	if got.Instructions != "test instructions" {
		t.Fatalf("expected instructions %q, got %q", "test instructions", got.Instructions)
	}
	if got.ToolSchemas != schemas {
		t.Fatalf("expected tool_schemas %q, got %q", schemas, got.ToolSchemas)
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
