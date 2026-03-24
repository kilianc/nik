package db

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestToolCallInsert(t *testing.T) {
	tests := []struct {
		name        string
		toolName    string
		input       string
		output      string
		duration    time.Duration
		isError     bool
		wantErrFlag int
	}{
		{
			name:        "persists row",
			toolName:    "shell",
			input:       `{"action":"run","command":"ls"}`,
			output:      "file1\nfile2",
			duration:    150 * time.Millisecond,
			isError:     false,
			wantErrFlag: 0,
		},
		{
			name:        "error flag",
			toolName:    "db_query",
			input:       `{"sql":"SELECT bad"}`,
			output:      "no such table",
			duration:    30 * time.Millisecond,
			isError:     true,
			wantErrFlag: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			conn, err := OpenInMemory()
			if err != nil {
				t.Fatalf("open in-memory db: %v", err)
			}
			defer conn.Close()

			convID := seedConversation(t, ctx, conn, "whatsapp", "ext-tc-"+tt.name, "")

			actID := "act-tc-" + tt.name
			_, err = conn.ExecContext(ctx,
				"INSERT INTO activation (id, conversation_id, sources, model, created_at) VALUES (?, ?, '[\"task\"]', 'gpt-4', NOW_ISO8601_MS())",
				actID, convID)
			if err != nil {
				t.Fatalf("insert activation: %v", err)
			}

			roundID, err := ActivationRoundInsert(ctx, conn, ActivationRoundInsertParams{
				ActivationID: actID,
				Round:        0,
				UserInput:    "test input",
			})
			if err != nil {
				t.Fatalf("insert activation round: %v", err)
			}

			err = ToolCallInsert(ctx, conn, ToolCallInsertParams{
				ActivationID:      actID,
				ActivationRoundID: roundID,
				Name:              tt.toolName,
				Input:             tt.input,
				Output:            tt.output,
				Duration:          tt.duration,
				IsError:           tt.isError,
			})
			if err != nil {
				t.Fatalf("insert tool call: %v", err)
			}

			var name string
			var gotRoundID sql.NullString
			var durationMS, errFlag int
			err = conn.QueryRowContext(ctx,
				"SELECT name, activation_round_id, duration_ms, error FROM tool_call WHERE activation_id = ?", actID,
			).Scan(&name, &gotRoundID, &durationMS, &errFlag)
			if err != nil {
				t.Fatalf("query tool call: %v", err)
			}

			if name != tt.toolName {
				t.Fatalf("expected name %q, got %q", tt.toolName, name)
			}
			if !gotRoundID.Valid || gotRoundID.String != roundID {
				t.Fatalf("expected activation_round_id %q, got %v", roundID, gotRoundID)
			}
			if errFlag != tt.wantErrFlag {
				t.Fatalf("expected error flag %d, got %d", tt.wantErrFlag, errFlag)
			}
		})
	}

	t.Run("list by activation", func(t *testing.T) {
		ctx := context.Background()

		conn, err := OpenInMemory()
		if err != nil {
			t.Fatalf("open in-memory db: %v", err)
		}
		defer conn.Close()

		convID := seedConversation(t, ctx, conn, "whatsapp", "ext-tc-list", "")

		actID := "act-tc-list"
		_, err = conn.ExecContext(ctx,
			"INSERT INTO activation (id, conversation_id, sources, model, created_at) VALUES (?, ?, '[\"task\"]', 'gpt-4', NOW_ISO8601_MS())",
			actID, convID)
		if err != nil {
			t.Fatalf("insert activation: %v", err)
		}

		r0ID, err := ActivationRoundInsert(ctx, conn, ActivationRoundInsertParams{
			ActivationID: actID,
			Round:        0,
			UserInput:    "round 0",
		})
		if err != nil {
			t.Fatalf("insert round 0: %v", err)
		}

		r1ID, err := ActivationRoundInsert(ctx, conn, ActivationRoundInsertParams{
			ActivationID: actID,
			Round:        1,
			UserInput:    "round 1",
		})
		if err != nil {
			t.Fatalf("insert round 1: %v", err)
		}
		_ = r1ID

		err = ToolCallInsert(ctx, conn, ToolCallInsertParams{
			ActivationID:      actID,
			ActivationRoundID: r0ID,
			Name:              "shell",
			Input:             "{}",
			Output:            "ok",
			Duration:          10 * time.Millisecond,
		})
		if err != nil {
			t.Fatalf("insert tc round 0: %v", err)
		}

		err = ToolCallInsert(ctx, conn, ToolCallInsertParams{
			ActivationID:      actID,
			ActivationRoundID: r1ID,
			Name:              "db_query",
			Input:             "{}",
			Output:            "rows",
			Duration:          5 * time.Millisecond,
		})
		if err != nil {
			t.Fatalf("insert tc round 1: %v", err)
		}

		all, err := ToolCallList(ctx, conn, actID, nil)
		if err != nil {
			t.Fatalf("list all: %v", err)
		}
		if len(all) != 2 {
			t.Fatalf("expected 2 tool calls, got %d", len(all))
		}

		round0 := 0
		r0Only, err := ToolCallList(ctx, conn, actID, &round0)
		if err != nil {
			t.Fatalf("list round 0: %v", err)
		}
		if len(r0Only) != 1 {
			t.Fatalf("expected 1 tool call for round 0, got %d", len(r0Only))
		}
		if r0Only[0].Name != "shell" {
			t.Fatalf("expected name %q, got %q", "shell", r0Only[0].Name)
		}
	})

	t.Run("nil round id", func(t *testing.T) {
		ctx := context.Background()

		conn, err := OpenInMemory()
		if err != nil {
			t.Fatalf("open in-memory db: %v", err)
		}
		defer conn.Close()

		convID := seedConversation(t, ctx, conn, "whatsapp", "ext-tc-nil-round", "")

		actID := "act-tc-nil-round"
		_, err = conn.ExecContext(ctx,
			"INSERT INTO activation (id, conversation_id, sources, model, created_at) VALUES (?, ?, '[\"task\"]', 'gpt-4', NOW_ISO8601_MS())",
			actID, convID)
		if err != nil {
			t.Fatalf("insert activation: %v", err)
		}

		err = ToolCallInsert(ctx, conn, ToolCallInsertParams{
			ActivationID: actID,
			Name:         "shell",
			Input:        "{}",
			Output:       "ok",
			Duration:     10 * time.Millisecond,
		})
		if err != nil {
			t.Fatalf("insert tool call: %v", err)
		}

		var gotRoundID sql.NullString
		err = conn.QueryRowContext(ctx,
			"SELECT activation_round_id FROM tool_call WHERE activation_id = ?", actID,
		).Scan(&gotRoundID)
		if err != nil {
			t.Fatalf("query tool call: %v", err)
		}

		if gotRoundID.Valid {
			t.Fatalf("expected NULL activation_round_id, got %q", gotRoundID.String)
		}
	})
}
