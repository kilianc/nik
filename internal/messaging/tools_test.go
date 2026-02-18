package messaging

import (
	"context"
	"strings"
	"testing"

	"github.com/kciuffolo/nik/internal/llm"
)

func TestBuildToolsReturnsExpectedToolNames(t *testing.T) {
	tools := BuildTools(&Service{})
	if len(tools) != 7 {
		t.Fatalf("expected 7 tools, got %d", len(tools))
	}

	want := []string{
		"message_reply",
		"message_noop",
		"message_react",
		"message_start_typing",
		"message_stop_typing",
		"message_set_presence",
		"message_update_media_description",
	}
	for i, name := range want {
		if tools[i].Def.Name != name {
			t.Fatalf("expected tool %d to be %q, got %q", i, name, tools[i].Def.Name)
		}
	}
}

func TestReplyHandlerRejectsInvalidJSON(t *testing.T) {
	handler := replyHandler(&Service{})

	out, err := handler(context.Background(), llm.ToolCall{Arguments: "{"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"error"`) {
		t.Fatalf("expected JSON error response, got %q", out)
	}
}

func TestNoopHandlerRejectsInvalidJSON(t *testing.T) {
	handler := noopHandler()

	out, err := handler(context.Background(), llm.ToolCall{Arguments: "{"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"error"`) {
		t.Fatalf("expected JSON error response, got %q", out)
	}
}

func TestNoopHandlerUsesContextConversationID(t *testing.T) {
	handler := noopHandler()

	ctx := context.WithValue(
		context.Background(),
		"meta",
		map[string]string{"conversation_id": "conv-123"},
	)
	out, err := handler(ctx, llm.ToolCall{
		Arguments: `{"conversation_id":"","reason":"group thread, staying silent"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"silent":true`) {
		t.Fatalf("expected silent true response, got %q", out)
	}
}

func TestReactHandlerRejectsEmptyText(t *testing.T) {
	handler := reactHandler(&Service{})

	out, err := handler(context.Background(), llm.ToolCall{
		Arguments: `{"text":"","emoji":"👍"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"error"`) || !strings.Contains(out, "missing text") {
		t.Fatalf("expected missing text error, got %q", out)
	}
}

func TestReactHandlerRejectsMissingConversationID(t *testing.T) {
	handler := reactHandler(&Service{})

	out, err := handler(context.Background(), llm.ToolCall{
		Arguments: `{"text":"hello","emoji":"👍"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "missing conversation_id") {
		t.Fatalf("expected missing conversation_id error, got %q", out)
	}
}

func TestUpdateMediaDescriptionHandlerRejectsEmptyText(t *testing.T) {
	handler := updateMediaDescriptionHandler(&Service{})

	out, err := handler(context.Background(), llm.ToolCall{
		Arguments: `{"text":"","description":"desc","body":""}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "missing text") {
		t.Fatalf("expected missing text error, got %q", out)
	}
}

func TestReactToolDefUsesTextParam(t *testing.T) {
	props, ok := reactToolDef.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties map")
	}

	if _, ok := props["text"]; !ok {
		t.Fatalf("expected 'text' parameter in react tool def")
	}
	if _, ok := props["message_id"]; ok {
		t.Fatalf("message_id should not be in react tool def")
	}
}

func TestUpdateMediaToolDefUsesTextParam(t *testing.T) {
	props, ok := updateMediaDescriptionToolDef.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties map")
	}

	if _, ok := props["text"]; !ok {
		t.Fatalf("expected 'text' parameter in update media tool def")
	}
	if _, ok := props["message_id"]; ok {
		t.Fatalf("message_id should not be in update media tool def")
	}
}
