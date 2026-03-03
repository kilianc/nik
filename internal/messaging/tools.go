package messaging

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/kciuffolo/nik/internal/llm"
)

var replyToolDef = llm.ToolDef{
	Name:        "message_reply",
	Description: "Send a reply to a conversation. Supports text and image messages.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"conversation_id": map[string]any{
				"type":        "string",
				"description": "Nik conversation UUID. Pass empty string to use the current conversation from context.",
			},
			"message": map[string]any{
				"type":        "string",
				"description": "Reply text, or image caption when sending an image.",
			},
			"image_path": map[string]any{
				"type":        "string",
				"description": "Absolute path to an image file to send. Pass empty string for text-only replies.",
			},
		},
		"required":             []string{"conversation_id", "message", "image_path"},
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
	Description: "React to a specific message with one emoji. Identify the message by quoting its text (substring match on the formatted line). Include sender name if ambiguous.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "Quote from the message to target (substring match on the formatted message line).",
			},
			"emoji": map[string]any{
				"type":        "string",
				"description": "Reaction emoji.",
			},
		},
		"required":             []string{"text", "emoji"},
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
	Description: "Persist media description for a message and optionally replace body text. Identify the message by quoting its text (substring match on the formatted message line). Include sender name if ambiguous.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "Quote from the message to target (substring match on the formatted message line).",
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
		"required":             []string{"text", "description", "body"},
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

func replyHandler(svc *Service) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			ConversationID string `json:"conversation_id"`
			Message        string `json:"message"`
			ImagePath      string `json:"image_path"`
		}

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		if strings.TrimSpace(args.ConversationID) == "" {
			args.ConversationID = contextMetaValue(ctx, "conversation_id")
		}
		if strings.TrimSpace(args.ConversationID) == "" {
			return `{"error":"missing conversation_id"}`, nil
		}

		if strings.TrimSpace(args.ImagePath) != "" {
			err = svc.SendImage(ctx, args.ConversationID, args.ImagePath, args.Message)
		} else {
			err = svc.Reply(ctx, args.ConversationID, args.Message)
		}

		if err != nil {
			return llm.ToolError(err), nil
		}

		return `{"sent":true}`, nil
	}
}

func noopHandler() llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			ConversationID string `json:"conversation_id"`
			Reason         string `json:"reason"`
		}

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		if strings.TrimSpace(args.ConversationID) == "" {
			args.ConversationID = contextMetaValue(ctx, "conversation_id")
		}
		if strings.TrimSpace(args.ConversationID) == "" {
			return `{"error":"missing conversation_id"}`, nil
		}

		return `{"ok":true,"silent":true}`, nil
	}
}

func reactHandler(svc *Service) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			Text  string `json:"text"`
			Emoji string `json:"emoji"`
		}

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		if strings.TrimSpace(args.Text) == "" {
			return `{"error":"missing text"}`, nil
		}

		conversationID := contextMetaValue(ctx, "conversation_id")
		if conversationID == "" {
			return `{"error":"missing conversation_id in context"}`, nil
		}

		msg, err := svc.FindMessage(ctx, conversationID, args.Text)
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

		conversationID := contextMetaValue(ctx, "conversation_id")
		if conversationID == "" {
			return `{"error":"missing conversation_id in context"}`, nil
		}

		msg, err := svc.FindMessage(ctx, conversationID, args.Text)
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
