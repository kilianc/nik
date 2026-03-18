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
		AllowConversationIDs: map[string]string{"test": "conv-123"},
		BannedWords:          []string{"goblin"},
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

func TestReplyHandlerBlocksImagePathTraversal(t *testing.T) {
	cfg := &config.Config{
		Home:                 "/home/nik",
		AllowConversationIDs: map[string]string{"owner": "conv-123"},
	}
	svc := &Service{cfg: cfg}
	handler := replyHandler(svc)

	ctx := context.WithValue(
		context.Background(),
		"meta",
		map[string]string{"conversation_id": "conv-123"},
	)

	out, err := handler(ctx, llm.ToolCall{
		Arguments: `{"conversation_id":"conv-123","contact_id":"","messages":[{"text":"look","image_path":"/etc/passwd","voice":false}]}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "must be within") {
		t.Fatalf("expected path containment error, got %q", out)
	}
}

func TestReplyHandlerBlocksDisallowedConversation(t *testing.T) {
	cfg := &config.Config{
		AllowConversationIDs: map[string]string{"owner": "allowed-conv"},
	}
	svc := &Service{cfg: cfg}
	handler := replyHandler(svc)

	ctx := context.WithValue(
		context.Background(),
		"meta",
		map[string]string{"conversation_id": "allowed-conv"},
	)

	out, err := handler(ctx, llm.ToolCall{
		Arguments: `{"conversation_id":"not-allowed-conv","contact_id":"","messages":[{"text":"hi","image_path":"","voice":false}]}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "allow list") {
		t.Fatalf("expected allow list error, got %q", out)
	}
}

func TestReplyHandlerAllowsContextConversation(t *testing.T) {
	cfg := &config.Config{
		AllowConversationIDs: map[string]string{"owner": "allowed-conv"},
	}
	svc := &Service{cfg: cfg}
	handler := replyHandler(svc)

	ctx := context.WithValue(
		context.Background(),
		"meta",
		map[string]string{"conversation_id": "allowed-conv"},
	)

	// use voice=true with no speechFn: this triggers "not configured" before
	// touching the DB, confirming the allow check passed
	out, err := handler(ctx, llm.ToolCall{
		Arguments: `{"conversation_id":"","contact_id":"","messages":[{"text":"hi","image_path":"","voice":true}]}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "allow list") {
		t.Fatalf("expected to pass allow check, got %q", out)
	}
	if !strings.Contains(out, "not configured") {
		t.Fatalf("expected 'not configured' (past allow check), got %q", out)
	}
}

func TestReactHandlerRejectsEmptyText(t *testing.T) {
	handler := reactHandler(&Service{})

	out, err := handler(context.Background(), llm.ToolCall{
		Arguments: `{"text":"","time":"09:00:00","emoji":"👍"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"error"`) || !strings.Contains(out, "missing text") {
		t.Fatalf("expected missing text error, got %q", out)
	}
}

func TestReactHandlerRejectsEmptyTime(t *testing.T) {
	handler := reactHandler(&Service{})

	out, err := handler(context.Background(), llm.ToolCall{
		Arguments: `{"text":"hello","time":"","emoji":"👍"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "missing time") {
		t.Fatalf("expected missing time error, got %q", out)
	}
}

func TestReactHandlerRejectsMissingConversationID(t *testing.T) {
	handler := reactHandler(&Service{})

	out, err := handler(context.Background(), llm.ToolCall{
		Arguments: `{"text":"hello","time":"09:00:00","emoji":"👍"}`,
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
		Arguments: `{"text":"","time":"09:00:00","description":"desc","body":""}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "missing text") {
		t.Fatalf("expected missing text error, got %q", out)
	}
}

func TestUpdateMediaDescriptionHandlerRejectsEmptyTime(t *testing.T) {
	handler := updateMediaDescriptionHandler(&Service{})

	out, err := handler(context.Background(), llm.ToolCall{
		Arguments: `{"text":"hello","time":"","description":"desc","body":""}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "missing time") {
		t.Fatalf("expected missing time error, got %q", out)
	}
}

func TestReactToolDefUsesTextAndTimeParams(t *testing.T) {
	props, ok := reactToolDef.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties map")
	}

	if _, ok := props["text"]; !ok {
		t.Fatalf("expected 'text' parameter in react tool def")
	}
	if _, ok := props["time"]; !ok {
		t.Fatalf("expected 'time' parameter in react tool def")
	}

	required := reactToolDef.Parameters["required"].([]string)
	hasTime := false
	for _, r := range required {
		if r == "time" {
			hasTime = true
		}
	}
	if !hasTime {
		t.Fatalf("expected 'time' in required list")
	}
}

func TestUpdateMediaToolDefUsesTextAndTimeParams(t *testing.T) {
	props, ok := updateMediaDescriptionToolDef.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties map")
	}

	if _, ok := props["text"]; !ok {
		t.Fatalf("expected 'text' parameter in update media tool def")
	}
	if _, ok := props["time"]; !ok {
		t.Fatalf("expected 'time' parameter in update media tool def")
	}

	required := updateMediaDescriptionToolDef.Parameters["required"].([]string)
	hasTime := false
	for _, r := range required {
		if r == "time" {
			hasTime = true
		}
	}
	if !hasTime {
		t.Fatalf("expected 'time' in required list")
	}
}
