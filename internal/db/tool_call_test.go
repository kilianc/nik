package db

import (
	"context"
	"testing"
	"time"
)

func TestToolCallInsertOne(t *testing.T) {
	tests := []struct {
		name        string
		toolName    string
		round       int
		input       string
		output      string
		duration    time.Duration
		isError     bool
		wantErrFlag int
	}{
		{
			name:        "persists row",
			toolName:    "shell",
			round:       7,
			input:       `{"action":"run","command":"ls"}`,
			output:      "file1\nfile2",
			duration:    150 * time.Millisecond,
			isError:     false,
			wantErrFlag: 0,
		},
		{
			name:        "error flag",
			toolName:    "db_query",
			round:       2,
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

			err = ToolCallInsertOne(ctx, conn, ToolCallInsertParams{
				ActivationID: actID,
				Name:         tt.toolName,
				Round:        tt.round,
				Input:        tt.input,
				Output:       tt.output,
				Duration:     tt.duration,
				IsError:      tt.isError,
			})
			if err != nil {
				t.Fatalf("insert tool call: %v", err)
			}

			var name string
			var round, durationMS, errFlag int
			err = conn.QueryRowContext(ctx,
				"SELECT name, round, duration_ms, error FROM tool_call WHERE activation_id = ?", actID,
			).Scan(&name, &round, &durationMS, &errFlag)
			if err != nil {
				t.Fatalf("query tool call: %v", err)
			}

			if name != tt.toolName {
				t.Fatalf("expected name %q, got %q", tt.toolName, name)
			}
			if round != tt.round {
				t.Fatalf("expected round %d, got %d", tt.round, round)
			}
			if errFlag != tt.wantErrFlag {
				t.Fatalf("expected error flag %d, got %d", tt.wantErrFlag, errFlag)
			}
		})
	}
}
