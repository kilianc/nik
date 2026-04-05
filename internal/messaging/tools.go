package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kciuffolo/nik/internal/llm"
)

var sendToolDef = llm.ToolDef{
	Name:        "message_send",
	Description: "Send one or more messages to a conversation. Each message is a separate text bubble. Use quote_text and quote_time on a message to send it as a quote reply anchored to a specific message in the conversation.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"conversation_id": map[string]any{
				"type":        "string",
				"description": "Nik conversation UUID. Pass empty string to use the current conversation from context.",
			},
			"contact_id": map[string]any{
				"type":        "string",
				"description": "Contact UUID. Use to start a new DM when no conversation exists yet. Pass empty string when using conversation_id.",
			},
			"messages": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"text": map[string]any{
							"type":        "string",
							"description": "Message text, or caption when sending a file.",
						},
						"file_path": map[string]any{
							"type":        "string",
							"description": "Path to a file relative to workspace (image, video, audio, document, etc.). Omit or pass empty string for text-only.",
						},
						"voice": map[string]any{
							"type":        "boolean",
							"description": "When true, the message text is converted to a voice note via TTS instead of sent as text.",
						},
						"quote_text": map[string]any{
							"type":        "string",
							"description": "Exact message content to quote, as shown after sender name in timeline (before any (quote replying to ...) context). Pass empty string for no quote.",
						},
						"quote_time": map[string]any{
							"type":        "string",
							"description": "HH:MM:SS timestamp of the message to quote, from the timeline brackets. Pass empty string for no quote.",
						},
					},
					"required":             []string{"text", "file_path", "voice", "quote_text", "quote_time"},
					"additionalProperties": false,
				},
				"description": "Array of messages to send, in order. Each becomes a separate bubble.",
			},
		},
		"required":             []string{"conversation_id", "contact_id", "messages"},
		"additionalProperties": false,
	},
}

var reactToolDef = llm.ToolDef{
	Name:        "message_react",
	Description: "React to a specific message with one emoji.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "Exact message content as shown after sender name in timeline, before any (quote replying to ...) or (reacting to ...) context.",
			},
			"time": map[string]any{
				"type":        "string",
				"description": "Timestamp in HH:MM:SS from the timeline brackets.",
			},
			"emoji": map[string]any{
				"type":        "string",
				"description": "Reaction emoji.",
			},
		},
		"required":             []string{"text", "time", "emoji"},
		"additionalProperties": false,
	},
}

func BuildTools(svc *Service) []llm.Tool {
	return []llm.Tool{
		{Def: sendToolDef, Handler: sendHandler(svc)},
		{Def: reactToolDef, Handler: reactHandler(svc)},
	}
}

type sendMessage struct {
	Text      string `json:"text"`
	FilePath  string `json:"file_path"`
	Voice     bool   `json:"voice"`
	QuoteText string `json:"quote_text"`
	QuoteTime string `json:"quote_time"`
}

func sendHandler(svc *Service) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			ConversationID string        `json:"conversation_id"`
			ContactID      string        `json:"contact_id"`
			Messages       []sendMessage `json:"messages"`
		}

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		if len(args.Messages) == 0 {
			return `{"error":"messages array is empty"}`, nil
		}

		resolvedFromContact := false

		if strings.TrimSpace(args.ConversationID) == "" {
			args.ConversationID = contextMetaValue(ctx, "conversation_id")
		}

		if strings.TrimSpace(args.ConversationID) == "" && strings.TrimSpace(args.ContactID) != "" {
			convID, resolveErr := svc.ResolveConversation(ctx, args.ContactID)
			if resolveErr != nil {
				return llm.ToolError(resolveErr), nil
			}
			args.ConversationID = convID
			resolvedFromContact = true
		}

		if strings.TrimSpace(args.ConversationID) == "" {
			return `{"error":"missing conversation_id and contact_id"}`, nil
		}

		// Prevent the LLM from messaging conversations outside the allow list.
		// The context conversation is guaranteed to be allowed (timeline only
		// produces stimuli for allowed conversations), but explicit or
		// contact-resolved conversation IDs must be verified.
		if svc.cfg != nil && !svc.cfg.IsAllowed(args.ConversationID) {
			return llm.ToolErrorf("conversation %s is not in the allow list", args.ConversationID), nil
		}

		for _, msg := range args.Messages {
			err = svc.checkBannedWords(msg.Text)
			if err != nil {
				return llm.ToolError(err), nil
			}

			if p := strings.TrimSpace(msg.FilePath); p != "" && svc.cfg != nil {
				if filepath.IsAbs(p) {
					return llm.ToolErrorf("file_path must be relative to workspace"), nil
				}

				root, rootErr := os.OpenRoot(svc.cfg.Home)
				if rootErr != nil {
					return llm.ToolError(rootErr), nil
				}

				_, statErr := root.Stat(p)
				root.Close()
				if statErr != nil {
					return llm.ToolErrorf("file_path: %v", statErr), nil
				}
			}
		}

		for _, msg := range args.Messages {
			var quote *QuoteTarget
			qt := strings.TrimSpace(msg.QuoteText)
			qts := strings.TrimSpace(msg.QuoteTime)
			if qt != "" && qts != "" {
				target, findErr := svc.FindMessage(ctx, args.ConversationID, qt, qts)
				if findErr != nil {
					return llm.ToolError(findErr), nil
				}
				quote = &QuoteTarget{
					ExternalMessageID: target.ExternalMessageID,
					ExternalSenderID:  target.ExternalSenderID,
					Body:              target.Body,
					Kind:              target.Kind,
				}
			}

			switch {
			case msg.Voice:
				if svc.speechFn == nil {
					return `{"error":"voice messages not configured"}`, nil
				}
				audioPath, speechErr := svc.speechFn(ctx, msg.Text)
				if speechErr != nil {
					return llm.ToolError(speechErr), nil
				}
				err = svc.SendVoiceNote(ctx, args.ConversationID, audioPath, msg.Text)
			case strings.TrimSpace(msg.FilePath) != "":
				absPath := filepath.Join(svc.cfg.Home, strings.TrimSpace(msg.FilePath))
				err = svc.SendFile(ctx, args.ConversationID, absPath, msg.Text)
			default:
				err = svc.Reply(ctx, args.ConversationID, msg.Text, quote)
			}

			if err != nil {
				return llm.ToolError(err), nil
			}
		}

		result := fmt.Sprintf(`{"sent":%d}`, len(args.Messages))
		if resolvedFromContact {
			result = fmt.Sprintf(`{"sent":%d,"conversation_id":%q}`, len(args.Messages), args.ConversationID)
		}

		return result, nil
	}
}

func reactHandler(svc *Service) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			Text  string `json:"text"`
			Time  string `json:"time"`
			Emoji string `json:"emoji"`
		}

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		if strings.TrimSpace(args.Text) == "" {
			return `{"error":"missing text"}`, nil
		}

		if strings.TrimSpace(args.Time) == "" {
			return `{"error":"missing time"}`, nil
		}

		conversationID := contextMetaValue(ctx, "conversation_id")
		if conversationID == "" {
			return `{"error":"missing conversation_id in context"}`, nil
		}

		msg, err := svc.FindMessage(ctx, conversationID, args.Text, args.Time)
		if err != nil {
			return llm.ToolError(err), nil
		}

		err = svc.React(ctx, msg.ID, args.Emoji)
		if err != nil {
			return llm.ToolError(err), nil
		}

		return `{"sent":true}`, nil
	}
}

func contextMetaValue(ctx context.Context, key string) string {
	meta, ok := ctx.Value("meta").(map[string]string)
	if !ok {
		return ""
	}

	return strings.TrimSpace(meta[key])
}
