package skills

import (
	"context"
	"fmt"
	"testing"

	"github.com/kciuffolo/nik/internal/db"
)

func TestResolveCron(t *testing.T) {
	tests := []struct {
		name      string
		every     string
		seed      string // pre-cached cron_expr for this every string
		llmReply  string
		llmErr    error
		wantCalls int
		wantErr   bool
	}{
		{
			name:      "cache hit",
			every:     "every day at 6am",
			seed:      "0 6 * * *",
			wantCalls: 0,
		},
		{
			name:      "cache miss calls llm",
			every:     "every day at 11:30pm",
			llmReply:  "30 23 * * *",
			wantCalls: 1,
		},
		{
			name:      "llm error",
			every:     "every day at noon",
			llmErr:    fmt.Errorf("api down"),
			wantCalls: 1,
			wantErr:   true,
		},
		{
			name:      "llm returns invalid cron",
			every:     "every full moon",
			llmReply:  "not a cron",
			wantCalls: 1,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, err := db.OpenInMemory()
			if err != nil {
				t.Fatalf("open db: %v", err)
			}
			t.Cleanup(func() { conn.Close() })

			ctx := context.Background()

			if tt.seed != "" {
				err = db.EveryToCronInsert(ctx, conn, tt.every, tt.seed)
				if err != nil {
					t.Fatalf("seed cache: %v", err)
				}
			}

			calls := 0
			complete := func(_ context.Context, _, _ string) (string, error) {
				calls++
				return tt.llmReply, tt.llmErr
			}

			sched, err := resolveCron(ctx, conn, tt.every, complete)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if sched == nil {
				t.Fatal("expected non-nil schedule")
			}

			if calls != tt.wantCalls {
				t.Errorf("llm calls = %d, want %d", calls, tt.wantCalls)
			}

			if tt.seed == "" {
				cached, err := db.EveryToCronGet(ctx, conn, tt.every)
				if err != nil {
					t.Fatalf("read cache: %v", err)
				}
				if cached != tt.llmReply {
					t.Errorf("cached = %q, want %q", cached, tt.llmReply)
				}
			}
		})
	}
}
