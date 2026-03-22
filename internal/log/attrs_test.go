package log

import (
	"context"
	"testing"
)

func TestToolCallAttrs(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		ctx := context.Background()
		attrs := ToolCallAttrs(ctx, "brain", "db_query", 2, `{"query":"SELECT 1"}`)

		want := map[string]string{"pkg": "brain", "tool": "db_query", "query": "SELECT 1"}
		for k, v := range want {
			found := false
			for i := 0; i < len(attrs)-1; i += 2 {
				if attrs[i] == k && attrs[i+1] == v {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("missing %s=%s in %v", k, v, attrs)
			}
		}
	})

	t.Run("with meta", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), "meta", map[string]string{
			"activation_id": "act-123",
			"task_id":       "task-456",
		})
		attrs := ToolCallAttrs(ctx, "task", "shell", 0, `{}`)

		want := map[string]string{"activation_id": "act-123", "task_id": "task-456"}
		for k, v := range want {
			found := false
			for i := 0; i < len(attrs)-1; i += 2 {
				if attrs[i] == k && attrs[i+1] == v {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("missing %s=%s in %v", k, v, attrs)
			}
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		ctx := context.Background()
		attrs := ToolCallAttrs(ctx, "brain", "shell", 0, "not json")

		found := false
		for i := 0; i < len(attrs)-1; i += 2 {
			if attrs[i] == "args" && attrs[i+1] == "not json" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected raw args fallback in %v", attrs)
		}
	})
}
