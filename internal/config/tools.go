package config

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"slices"
	"strconv"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/llm"
)

var updateConfigDef = llm.ToolDef{
	Name:        "update_config",
	Description: "Read or update nik's runtime configuration. Use action 'get' to see current config, 'set' to change a field, or allow_add/allow_remove/allow_reload to manage the conversation allow list.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []any{"get", "set", "allow_add", "allow_remove", "allow_reload"},
				"description": "The action to perform.",
			},
			"field": map[string]any{
				"type":        "string",
				"description": "Config field name for 'set'. Writable fields: timezone, location, model, reasoning_effort, debug_dir, media_dir, max_history.",
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

func BuildTools(cfg *Config, conn *sql.DB) []llm.Tool {
	return []llm.Tool{
		{
			Def:        updateConfigDef,
			Handler:    updateConfigHandler(cfg, conn),
			Privileged: true,
		},
	}
}

func updateConfigHandler(cfg *Config, conn *sql.DB) llm.ToolExecutor {
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
		case "allow_reload":
			return allowlistReload(cfg)
		default:
			return llm.ToolErrorf("unknown action %q", args.Action), nil
		}
	}
}

func configGet(cfg *Config) (string, error) {
	out := map[string]any{
		"model":                       cfg.Model,
		"reasoning_effort":            cfg.ReasoningEffort,
		"debug_dir":                   cfg.DebugDirValue,
		"media_dir":                   cfg.MediaDirValue,
		"max_history":                 cfg.MaxHistory,
		"timezone":                    cfg.Timezone,
		"location":                    cfg.Location,
		"allow_conversation_ids":      cfg.AllowConversationIDs,
		"privileged_conversation_ids": cfg.PrivilegedConversationIDs,
	}

	data, err := json.Marshal(out)
	if err != nil {
		return llm.ToolError(err), nil
	}

	return string(data), nil
}

var readOnlyFields = map[string]bool{
	"privileged_conversation_ids": true,
	"openai_key":                  true,
}

func configSet(cfg *Config, field, value string) (string, error) {
	if field == "" {
		return `{"error":"empty field"}`, nil
	}

	if readOnlyFields[field] {
		return llm.ToolErrorf("field %q is read-only", field), nil
	}

	switch field {
	case "timezone":
		cfg.Timezone = value
	case "location":
		cfg.Location = value
	case "model":
		cfg.Model = value
	case "reasoning_effort":
		valid := map[string]bool{
			"": true, "none": true, "minimal": true,
			"low": true, "medium": true, "high": true, "xhigh": true,
		}
		if !valid[value] {
			return llm.ToolErrorf("invalid reasoning_effort %q (none, minimal, low, medium, high, xhigh, or empty)", value), nil
		}
		cfg.ReasoningEffort = value
	case "debug_dir":
		cfg.DebugDirValue = value
	case "media_dir":
		cfg.MediaDirValue = value
	case "max_history":
		n, err := strconv.Atoi(value)
		if err != nil {
			return llm.ToolErrorf("invalid max_history: %s", value), nil
		}
		cfg.MaxHistory = n
	default:
		return llm.ToolErrorf("unknown field %q", field), nil
	}

	err := cfg.Save(cfg.ConfigPath())
	if err != nil {
		return llm.ToolError(err), nil
	}

	slog.Info("config set", "pkg", "config", "field", field, "value", value)

	return `{"ok":true}`, nil
}

func allowlistAdd(ctx context.Context, cfg *Config, conn *sql.DB, conversationID string) (string, error) {
	if conversationID == "" {
		return `{"error":"empty conversation_id"}`, nil
	}

	_, err := db.GetConversation(ctx, conn, db.GetConversationParams{ID: conversationID})
	if err != nil {
		return llm.ToolErrorf("conversation not found: %s", conversationID), nil
	}

	if slices.Contains(cfg.AllowConversationIDs, conversationID) {
		return `{"error":"already in allow list"}`, nil
	}

	cfg.AllowConversationIDs = append(cfg.AllowConversationIDs, conversationID)

	err = cfg.Save(cfg.ConfigPath())
	if err != nil {
		cfg.AllowConversationIDs = cfg.AllowConversationIDs[:len(cfg.AllowConversationIDs)-1]
		return llm.ToolError(err), nil
	}

	slog.Info("allowlist add", "pkg", "config", "conversation_id", conversationID)

	return `{"ok":true}`, nil
}

func allowlistReload(cfg *Config) (string, error) {
	fresh, err := Load(cfg.Home)
	if err != nil {
		return llm.ToolError(err), nil
	}

	cfg.AllowConversationIDs = append([]string(nil), fresh.AllowConversationIDs...)

	slog.Info("allowlist reload", "pkg", "config", "count", len(cfg.AllowConversationIDs))

	return `{"ok":true}`, nil
}

func allowlistRemove(cfg *Config, conversationID string) (string, error) {
	if conversationID == "" {
		return `{"error":"empty conversation_id"}`, nil
	}

	if len(cfg.AllowConversationIDs) <= 1 {
		return `{"error":"cannot remove last allow list entry"}`, nil
	}

	if slices.Contains(cfg.PrivilegedConversationIDs, conversationID) {
		return `{"error":"cannot remove privileged channel from allow list"}`, nil
	}

	idx := slices.Index(cfg.AllowConversationIDs, conversationID)
	if idx == -1 {
		return `{"error":"conversation_id not in allow list"}`, nil
	}

	cfg.AllowConversationIDs = slices.Delete(cfg.AllowConversationIDs, idx, idx+1)

	err := cfg.Save(cfg.ConfigPath())
	if err != nil {
		return llm.ToolError(err), nil
	}

	slog.Info("allowlist remove", "pkg", "config", "conversation_id", conversationID)

	return `{"ok":true}`, nil
}
