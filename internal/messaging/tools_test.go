package messaging

import (
	"context"
	"strings"
	"testing"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/llm"
)

func TestBuildToolsReturnsExpectedToolNames(t *testing.T) {
	tools := BuildTools(&Service{})
	if len(tools) != 5 {
		t.Fatalf("expected 5 tools, got %d", len(tools))
	}

	want := []string{
		"message_reply",
		"message_noop",
		"message_react",
		"message_set_presence",
		"message_update_media_description",
	}
	for i, name := range want {
		if tools[i].Def.Name != name {
			t.Fatalf("expected tool %d to be %q, got %q", i, name, tools[i].Def.Name)
		}
	}
}

func TestReplyToolDefHasMessagesArray(t *testing.T) {
	props, ok := replyToolDef.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties map")
	}

	msgsProp, ok := props["messages"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'messages' parameter in reply tool def")
	}

	if msgsProp["type"] != "array" {
		t.Fatalf("expected messages to be array type, got %v", msgsProp["type"])
	}

	required, ok := replyToolDef.Parameters["required"].([]string)
	if !ok {
		t.Fatalf("expected required slice")
	}

	found := false
	for _, r := range required {
		if r == "messages" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'messages' in required list")
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

func TestReplyHandlerRejectsEmptyMessages(t *testing.T) {
	handler := replyHandler(&Service{})

	ctx := context.WithValue(
		context.Background(),
		"meta",
		map[string]string{"conversation_id": "conv-123"},
	)
	out, err := handler(ctx, llm.ToolCall{
		Arguments: `{"conversation_id":"","contact_id":"","messages":[]}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "empty") {
		t.Fatalf("expected empty messages error, got %q", out)
	}
}

func TestReplyHandlerVoiceWithoutSpeechFnReturnsError(t *testing.T) {
	handler := replyHandler(&Service{})

	ctx := context.WithValue(
		context.Background(),
		"meta",
		map[string]string{"conversation_id": "conv-123"},
	)
	out, err := handler(ctx, llm.ToolCall{
		Arguments: `{"conversation_id":"conv-123","contact_id":"","messages":[{"text":"hello","image_path":"","voice":true}]}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "not configured") {
		t.Fatalf("expected 'not configured' error, got %q", out)
	}
}

func TestReplyToolDefHasVoiceField(t *testing.T) {
	props, ok := replyToolDef.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties map")
	}

	msgsProp, ok := props["messages"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'messages' parameter")
	}

	items, ok := msgsProp["items"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'items' in messages")
	}

	itemProps, ok := items["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'properties' in items")
	}

	voiceProp, ok := itemProps["voice"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'voice' in message properties")
	}

	if voiceProp["type"] != "boolean" {
		t.Fatalf("expected voice type boolean, got %v", voiceProp["type"])
	}
}

func TestReplyHandlerBannedWordPrevalidation(t *testing.T) {
	cfg := &config.Config{
		BannedWords: []string{"goblin"},
	}
	svc := &Service{cfg: cfg}
	handler := replyHandler(svc)

	ctx := context.WithValue(
		context.Background(),
		"meta",
		map[string]string{"conversation_id": "conv-123"},
	)

	out, err := handler(ctx, llm.ToolCall{
		Arguments: `{"conversation_id":"conv-123","contact_id":"","messages":[` +
			`{"text":"first message is fine","image_path":"","voice":false},` +
			`{"text":"second has goblin in it","image_path":"","voice":false}]}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "banned word") {
		t.Fatalf("expected banned word error, got %q", out)
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
