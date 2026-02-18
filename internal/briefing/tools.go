package briefing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/kciuffolo/nik/internal/llm"
)

var briefingWriteDef = llm.ToolDef{
	Name:        "briefing_write",
	Description: "Write today's morning briefing summary. Call once after you've finished reading and processing the news.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{
				"type":        "string",
				"description": "Summary of what you read, what you stored, and any actions taken.",
			},
		},
		"required":             []string{"content"},
		"additionalProperties": false,
	},
}

var briefingTopicsDef = llm.ToolDef{
	Name:        "briefing_topics",
	Description: "Manage your morning news feed topics. Use 'list' to see current topics, 'add' to follow something new, 'remove' to stop following a topic.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"list", "add", "remove"},
				"description": "list: show current topics. add: follow a new topic. remove: stop following a topic.",
			},
			"query": map[string]any{
				"type":        "string",
				"description": "Search query for the topic (for add). E.g. 'F1 racing news', 'news in Rome Italy'.",
			},
			"reason": map[string]any{
				"type":        "string",
				"description": "Why this topic matters (for add). E.g. 'CT loves F1', 'Mamma lives near Rome'.",
			},
			"contact_id": map[string]any{
				"type":        "string",
				"description": "Canonical contact_id this topic is for (for add). Empty if it's your own interest.",
			},
			"id": map[string]any{
				"type":        "string",
				"description": "Topic ID to remove (for remove). Get IDs from list.",
			},
		},
		"required":             []string{"action", "query", "reason", "contact_id", "id"},
		"additionalProperties": false,
	},
}

func BuildTools(svc *Service) []llm.Tool {
	return []llm.Tool{
		{
			Def:     briefingWriteDef,
			Handler: briefingWriteHandler(svc),
		},
		{
			Def:     briefingTopicsDef,
			Handler: briefingTopicsHandler(svc),
		},
	}
}

func briefingWriteHandler(svc *Service) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			Content string `json:"content"`
		}

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
		}

		if args.Content == "" {
			return `{"error":"empty content"}`, nil
		}

		err = svc.WriteBriefing(ctx, args.Content)
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
		}

		return `{"written":true}`, nil
	}
}

func briefingTopicsHandler(svc *Service) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			Action    string `json:"action"`
			Query     string `json:"query"`
			Reason    string `json:"reason"`
			ContactID string `json:"contact_id"`
			ID        string `json:"id"`
		}

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
		}

		switch args.Action {
		case "list":
			return listTopics(ctx, svc)
		case "add":
			return addTopic(ctx, svc, args.Query, args.Reason, args.ContactID)
		case "remove":
			return removeTopic(ctx, svc, args.ID)
		default:
			return `{"error":"action must be list, add, or remove"}`, nil
		}
	}
}

func listTopics(ctx context.Context, svc *Service) (string, error) {
	topics, err := svc.ListTopics(ctx)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
	}

	type topicEntry struct {
		ID        string `json:"id"`
		Query     string `json:"query"`
		Reason    string `json:"reason"`
		ContactID string `json:"contact_id,omitempty"`
	}

	entries := make([]topicEntry, len(topics))
	for i, t := range topics {
		entries[i] = topicEntry{
			ID:     t.ID,
			Query:  t.Query,
			Reason: t.Reason,
		}
		if t.ContactID.Valid {
			entries[i].ContactID = t.ContactID.String
		}
	}

	out, err := json.Marshal(map[string]any{"topics": entries})
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
	}

	return string(out), nil
}

func addTopic(ctx context.Context, svc *Service, query, reason, contactID string) (string, error) {
	if query == "" {
		return `{"error":"query is required for add"}`, nil
	}

	cid := sql.NullString{}
	if contactID != "" {
		cid = sql.NullString{String: contactID, Valid: true}
	}

	id, err := svc.AddTopic(ctx, query, reason, cid)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
	}

	return fmt.Sprintf(`{"added":true,"id":%q}`, id), nil
}

func removeTopic(ctx context.Context, svc *Service, id string) (string, error) {
	if id == "" {
		return `{"error":"id is required for remove"}`, nil
	}

	err := svc.RemoveTopic(ctx, id)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
	}

	return `{"removed":true}`, nil
}
