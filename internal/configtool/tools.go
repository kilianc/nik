// Package configtool exposes nik's configuration to the brain as a tool.
//
// It lives outside internal/config because it needs internal/llm and
// internal/db, and internal/config is linked by nikctl for its struct alone.
// A tool definition in there would put the whole daemon — SQLite included —
// inside the client binary.
package configtool

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"regexp"
	"strings"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/llm"
)

var configDef = llm.ToolDef{
	Name:        "config",
	Description: "Read or update nik's runtime configuration. Use action 'get' to see current config, 'set' to change a field, or allow_add/allow_remove to manage the conversation allow list. Config is live-reloaded from disk automatically.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []any{"get", "set", "allow_add", "allow_remove"},
				"description": "The action to perform.",
			},
			"field": map[string]any{
				"type":        "string",
				"description": "Config field name for 'set'. Writable fields: timezone, location, max_history, task.max_rounds, task.timeout, models.main.*, models.task.*, models.recall.*, shell.docker_image.",
			},
			"value": map[string]any{
				"type":        "string",
				"description": "New value for 'set', or conversation_id for allow_add/allow_remove.",
			},
		},
		"required":             []string{"action", "field", "value"},
		"additionalProperties": false,
	},
}

func BuildTools(cfg *config.Config, conn *sql.DB) []llm.Tool {
	return []llm.Tool{
		{Def: configDef, Handler: configHandler(cfg, conn)},
	}
}

func configHandler(cfg *config.Config, conn *sql.DB) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			Action string `json:"action"`
			Field  string `json:"field"`
			Value  string `json:"value"`
		}

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		switch args.Action {
		case "get":
			return configGet(cfg)
		case "set":
			return configSet(cfg, args.Field, args.Value)
		case "allow_add":
			return allowlistAdd(ctx, cfg, conn, args.Value)
		case "allow_remove":
			return allowlistRemove(cfg, args.Value)
		default:
			return llm.ToolErrorf("unknown action %q", args.Action), nil
		}
	}
}

func configGet(cfg *config.Config) (string, error) {
	data, err := json.Marshal(config.Snapshot(cfg))
	if err != nil {
		return llm.ToolError(err), nil
	}

	return string(data), nil
}

func configSet(cfg *config.Config, field, value string) (string, error) {
	err := config.SetField(cfg, field, value)
	if err != nil {
		return llm.ToolError(err), nil
	}

	return `{"ok":true}`, nil
}

func allowlistAdd(ctx context.Context, cfg *config.Config, conn *sql.DB, conversationID string) (string, error) {
	if conversationID == "" {
		return `{"error":"empty conversation_id"}`, nil
	}

	conv, err := db.ConversationGet(ctx, conn, db.ConversationGetParams{ID: conversationID})
	if err != nil {
		return llm.ToolErrorf("conversation not found: %s", conversationID), nil
	}

	if cfg.AllowConversationIDs.ContainsID(conversationID) {
		return `{"error":"already in allow list"}`, nil
	}

	label := deriveLabel(ctx, conn, conv)
	cfg.AllowConversationIDs.Append(label, conversationID)

	err = cfg.Save(cfg.ConfigPath())
	if err != nil {
		cfg.AllowConversationIDs.Remove(conversationID)
		return llm.ToolError(err), nil
	}

	slog.Info("allowlist add", "pkg", "config", "label", label, "conversation_id", conversationID)

	return `{"ok":true}`, nil
}

func allowlistRemove(cfg *config.Config, conversationID string) (string, error) {
	if conversationID == "" {
		return `{"error":"empty conversation_id"}`, nil
	}

	if len(cfg.AllowConversationIDs) <= 1 {
		return `{"error":"cannot remove last allow list entry"}`, nil
	}

	if cfg.IsPrivileged(conversationID) {
		return `{"error":"cannot remove privileged channel from allow list"}`, nil
	}

	label := cfg.AllowConversationIDs.LabelFor(conversationID)
	if label == "" {
		return `{"error":"conversation_id not in allow list"}`, nil
	}

	cfg.AllowConversationIDs.Remove(conversationID)

	err := cfg.Save(cfg.ConfigPath())
	if err != nil {
		return llm.ToolError(err), nil
	}

	slog.Info("allowlist remove", "pkg", "config", "label", label, "conversation_id", conversationID)

	return `{"ok":true}`, nil
}

var labelSanitizer = regexp.MustCompile(`[^a-z0-9-]`)

func deriveLabel(ctx context.Context, conn *sql.DB, conv db.Conversation) string {
	if conv.Title.Valid && strings.TrimSpace(conv.Title.String) != "" {
		raw := strings.ToLower(strings.TrimSpace(conv.Title.String))
		raw = strings.ReplaceAll(raw, " ", "-")
		return labelSanitizer.ReplaceAllString(raw, "")
	}

	if conv.Kind == "dm" {
		participants, err := db.ConversationParticipantList(ctx, conn, conv.ID)
		if err == nil {
			for _, p := range participants {
				name := p.DisplayName.String
				if name == "" {
					name = p.ContactName.String
				}
				if name != "" {
					raw := strings.ToLower(strings.TrimSpace(name))
					raw = strings.ReplaceAll(raw, " ", "-")
					return labelSanitizer.ReplaceAllString(raw, "")
				}
			}
		}
	}

	return conv.Kind + "-" + id.Shorten(conv.ID)[:6]
}
