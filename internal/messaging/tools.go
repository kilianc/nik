package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kciuffolo/nik/internal/llm"
)

var replyToolDef = llm.ToolDef{
	Name:        "message_reply",
	Description: "Send one or more messages to a conversation or contact. Each message is a separate text bubble, like texting. Use conversation_id for existing conversations, or contact_id to start a new DM.",
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
							"description": "Message text, or image caption when sending an image.",
						},
						"image_path": map[string]any{
							"type":        "string",
							"description": "Absolute path to an image file to send. Omit or pass empty string for text-only.",
						},
						"voice": map[string]any{
							"type":        "boolean",
							"description": "When true, the message text is converted to a voice note via TTS instead of sent as text.",
						},
					},
					"required":             []string{"text", "image_path", "voice"},
					"additionalProperties": false,
				},
				"description": "Array of messages to send, in order. Each becomes a separate bubble.",
			},
		},
		"required":             []string{"conversation_id", "contact_id", "messages"},
		"additionalProperties": false,
	},
}

var noopToolDef = llm.ToolDef{
	Name:        "message_noop",
	Description: "Acknowledge intentional silence for this turn without sending anything.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"conversation_id": map[string]any{
				"type":        "string",
				"description": "Nik conversation UUID. Pass empty string to use the current conversation from context.",
			},
			"reason": map[string]any{
				"type":        "string",
				"description": "Short reason for staying silent.",
			},
		},
		"required":             []string{"conversation_id", "reason"},
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
				"description": "Exact message content as shown after sender name in timeline, before any to [...] context.",
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

var setPresenceToolDef = llm.ToolDef{
	Name:        "message_set_presence",
	Description: "Set account-level presence for a messaging platform.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"platform": map[string]any{
				"type":        "string",
				"description": "Platform name (e.g. whatsapp).",
			},
			"available": map[string]any{
				"type":        "boolean",
				"description": "True for available, false for unavailable.",
			},
		},
		"required":             []string{"platform", "available"},
		"additionalProperties": false,
	},
}

var updateMediaDescriptionToolDef = llm.ToolDef{
	Name:        "message_update_media_description",
	Description: "Persist media description for a message and optionally replace body text.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "Exact message content as shown after sender name in timeline, before any to [...] context.",
			},
			"time": map[string]any{
				"type":        "string",
				"description": "Timestamp in HH:MM:SS from the timeline brackets.",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Description or transcript text.",
			},
			"body": map[string]any{
				"type":        "string",
				"description": "Optional replacement body text (pass empty string to skip).",
			},
		},
		"required":             []string{"text", "time", "description", "body"},
		"additionalProperties": false,
	},
}

func BuildTools(svc *Service) []llm.Tool {
	return []llm.Tool{
		{Def: replyToolDef, Handler: replyHandler(svc)},
		{Def: noopToolDef, Handler: noopHandler()},
		{Def: reactToolDef, Handler: reactHandler(svc)},
		{Def: setPresenceToolDef, Handler: setPresenceHandler(svc)},
		{Def: updateMediaDescriptionToolDef, Handler: updateMediaDescriptionHandler(svc)},
	}
}

type replyMessage struct {
	Text      string `json:"text"`
	ImagePath string `json:"image_path"`
	Voice     bool   `json:"voice"`
}

func replyHandler(svc *Service) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			ConversationID string         `json:"conversation_id"`
			ContactID      string         `json:"contact_id"`
			Messages       []replyMessage `json:"messages"`
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

		for _, msg := range args.Messages {
			err = svc.checkBannedWords(msg.Text)
			if err != nil {
				return llm.ToolError(err), nil
			}
		}

		for _, msg := range args.Messages {
			switch {
			case msg.Voice:
				if svc.speechFn == nil {
					return `{"error":"voice messages not configured"}`, nil
				}
				audioPath, speechErr := svc.speechFn(ctx, msg.Text)
				if speechErr != nil {
					return llm.ToolError(speechErr), nil
				}
				err = svc.SendAudio(ctx, args.ConversationID, audioPath, true, msg.Text)
			case strings.TrimSpace(msg.ImagePath) != "":
				err = svc.SendImage(ctx, args.ConversationID, msg.ImagePath, msg.Text)
			default:
				err = svc.Reply(ctx, args.ConversationID, msg.Text)
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

func noopHandler() llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args json.RawMessage

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		return `{"ok":true,"silent":true}`, nil
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

func setPresenceHandler(svc *Service) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			Platform  string `json:"platform"`
			Available bool   `json:"available"`
		}

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		err = svc.SetPresence(ctx, args.Platform, args.Available)
		if err != nil {
			return llm.ToolError(err), nil
		}

		return `{"ok":true}`, nil
	}
}

func updateMediaDescriptionHandler(svc *Service) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			Text        string `json:"text"`
			Time        string `json:"time"`
			Description string `json:"description"`
			Body        string `json:"body"`
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

		err = svc.UpdateMediaDescription(ctx, msg.ID, args.Description, args.Body)
		if err != nil {
			return llm.ToolError(err), nil
		}

		return `{"ok":true}`, nil
	}
}

func contextMetaValue(ctx context.Context, key string) string {
	meta, ok := ctx.Value("meta").(map[string]string)
	if !ok {
		return ""
	}

	return strings.TrimSpace(meta[key])
}
