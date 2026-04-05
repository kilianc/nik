package messaging

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/llm"
)

func TestBuildToolsReturnsExpectedToolNames(t *testing.T) {
	tools := BuildTools(&Service{})
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	want := []string{
		"message_send",
		"message_react",
	}
	for i, name := range want {
		if tools[i].Def.Name != name {
			t.Fatalf("expected tool %d to be %q, got %q", i, name, tools[i].Def.Name)
		}
	}
}

func TestSendToolDefSchema(t *testing.T) {
	props, ok := sendToolDef.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties map")
	}

	msgsProp, ok := props["messages"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'messages' parameter in send tool def")
	}
	if msgsProp["type"] != "array" {
		t.Fatalf("expected messages to be array type, got %v", msgsProp["type"])
	}

	required, ok := sendToolDef.Parameters["required"].([]string)
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

func TestSendToolDefHasQuoteFields(t *testing.T) {
	props, ok := sendToolDef.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties map")
	}

	msgsProp := props["messages"].(map[string]any)
	items := msgsProp["items"].(map[string]any)
	itemProps := items["properties"].(map[string]any)

	for _, field := range []string{"quote_text", "quote_time"} {
		prop, ok := itemProps[field].(map[string]any)
		if !ok {
			t.Fatalf("expected %q in message properties", field)
		}
		if prop["type"] != "string" {
			t.Fatalf("expected %q type string, got %v", field, prop["type"])
		}
	}

	required := items["required"].([]string)
	hasQuoteText := false
	hasQuoteTime := false
	for _, r := range required {
		if r == "quote_text" {
			hasQuoteText = true
		}
		if r == "quote_time" {
			hasQuoteTime = true
		}
	}
	if !hasQuoteText || !hasQuoteTime {
		t.Fatalf("expected quote_text and quote_time in required list")
	}
}

func TestSendHandlerInputValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		ctx     context.Context
		wantSub string
	}{
		{
			"invalid JSON",
			"{",
			context.Background(),
			`"error"`,
		},
		{
			"empty messages",
			`{"conversation_id":"","contact_id":"","messages":[]}`,
			context.WithValue(context.Background(), "meta", map[string]string{"conversation_id": "conv-123"}),
			"empty",
		},
	}

	handler := sendHandler(&Service{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := handler(tt.ctx, llm.ToolCall{Arguments: tt.args})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(out, tt.wantSub) {
				t.Fatalf("expected %q in output, got %q", tt.wantSub, out)
			}
		})
	}
}

func TestSendHandlerVoiceWithoutSpeechFnReturnsError(t *testing.T) {
	handler := sendHandler(&Service{})

	ctx := context.WithValue(
		context.Background(),
		"meta",
		map[string]string{"conversation_id": "conv-123"},
	)
	out, err := handler(ctx, llm.ToolCall{
		Arguments: `{"conversation_id":"conv-123","contact_id":"","messages":[{"text":"hello","file_path":"","voice":true}]}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "not configured") {
		t.Fatalf("expected 'not configured' error, got %q", out)
	}
}

func TestSendHandlerBannedWordPrevalidation(t *testing.T) {
	cfg := &config.Config{
		AllowConversationIDs: config.ConversationList{{Label: "test", ID: "conv-123"}},
		BannedWords:          []string{"goblin"},
	}
	svc := &Service{cfg: cfg}
	handler := sendHandler(svc)

	ctx := context.WithValue(
		context.Background(),
		"meta",
		map[string]string{"conversation_id": "conv-123"},
	)

	out, err := handler(ctx, llm.ToolCall{
		Arguments: `{"conversation_id":"conv-123","contact_id":"","messages":[` +
			`{"text":"first message is fine","file_path":"","voice":false},` +
			`{"text":"second has goblin in it","file_path":"","voice":false}]}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "banned word") {
		t.Fatalf("expected banned word error, got %q", out)
	}
}

func TestSendHandlerPathSecurity(t *testing.T) {
	makeCtx := func() context.Context {
		return context.WithValue(
			context.Background(),
			"meta",
			map[string]string{"conversation_id": "conv-123"},
		)
	}

	t.Run("absolute path blocked", func(t *testing.T) {
		home := t.TempDir()
		cfg := &config.Config{
			Home:                 home,
			AllowConversationIDs: config.ConversationList{{Label: "owner", ID: "conv-123"}},
		}
		handler := sendHandler(&Service{cfg: cfg})

		out, err := handler(makeCtx(), llm.ToolCall{
			Arguments: `{"conversation_id":"conv-123","contact_id":"","messages":[{"text":"look","file_path":"/etc/passwd","voice":false}]}`,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "must be relative") {
			t.Fatalf("expected relative path error, got %q", out)
		}
	})

	t.Run("traversal blocked", func(t *testing.T) {
		home := t.TempDir()
		cfg := &config.Config{
			Home:                 home,
			AllowConversationIDs: config.ConversationList{{Label: "owner", ID: "conv-123"}},
		}
		handler := sendHandler(&Service{cfg: cfg})

		out, err := handler(makeCtx(), llm.ToolCall{
			Arguments: `{"conversation_id":"conv-123","contact_id":"","messages":[{"text":"look","file_path":"../../../etc/passwd","voice":false}]}`,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "error") {
			t.Fatalf("expected error for traversal path, got %q", out)
		}
	})

	t.Run("symlink escape blocked", func(t *testing.T) {
		home := t.TempDir()
		outside := t.TempDir()
		os.WriteFile(filepath.Join(outside, "secret.png"), []byte("img"), 0o644)
		os.Symlink(outside, filepath.Join(home, "escape"))

		cfg := &config.Config{
			Home:                 home,
			AllowConversationIDs: config.ConversationList{{Label: "owner", ID: "conv-123"}},
		}
		handler := sendHandler(&Service{cfg: cfg})

		out, err := handler(makeCtx(), llm.ToolCall{
			Arguments: `{"conversation_id":"conv-123","contact_id":"","messages":[{"text":"look","file_path":"escape/secret.png","voice":false}]}`,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "error") {
			t.Fatalf("expected error for symlink escape, got %q", out)
		}
	})
}

func TestSendHandlerAllowList(t *testing.T) {
	cfg := &config.Config{
		AllowConversationIDs: config.ConversationList{{Label: "owner", ID: "allowed-conv"}},
	}
	svc := &Service{cfg: cfg}
	handler := sendHandler(svc)

	ctx := context.WithValue(
		context.Background(),
		"meta",
		map[string]string{"conversation_id": "allowed-conv"},
	)

	t.Run("blocks disallowed conversation", func(t *testing.T) {
		out, err := handler(ctx, llm.ToolCall{
			Arguments: `{"conversation_id":"not-allowed-conv","contact_id":"","messages":[{"text":"hi","file_path":"","voice":false}]}`,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "allow list") {
			t.Fatalf("expected allow list error, got %q", out)
		}
	})

	t.Run("allows context conversation", func(t *testing.T) {
		out, err := handler(ctx, llm.ToolCall{
			Arguments: `{"conversation_id":"","contact_id":"","messages":[{"text":"hi","file_path":"","voice":true}]}`,
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
	})
}

func TestReactHandlerValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		wantSub string
	}{
		{"empty text", `{"text":"","time":"09:00:00","emoji":"👍"}`, "missing text"},
		{"empty time", `{"text":"hello","time":"","emoji":"👍"}`, "missing time"},
		{"missing conversation_id", `{"text":"hello","time":"09:00:00","emoji":"👍"}`, "missing conversation_id"},
	}

	handler := reactHandler(&Service{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := handler(context.Background(), llm.ToolCall{Arguments: tt.args})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(out, tt.wantSub) {
				t.Fatalf("expected %q in output, got %q", tt.wantSub, out)
			}
		})
	}
}

func TestToolDefsHaveTextAndTimeParams(t *testing.T) {
	props, ok := reactToolDef.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties map")
	}

	for _, param := range []string{"text", "time"} {
		if _, ok := props[param]; !ok {
			t.Fatalf("expected %q parameter", param)
		}
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
