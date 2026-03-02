package journal

import (
	"context"
	"encoding/json"

	"github.com/kciuffolo/nik/internal/llm"
)

var journalWriteDef = llm.ToolDef{
	Name:        "journal_write",
	Description: "Write today's diary page. This is your private journal — nobody else sees it. Write honestly, in first person, as your own thoughts. Call this once at the end of your reflection.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{
				"type":        "string",
				"description": "The full journal entry for today.",
			},
		},
		"required":             []string{"content"},
		"additionalProperties": false,
	},
}

func BuildTools(svc *Service) []llm.Tool {
	return []llm.Tool{
		{
			Def:     journalWriteDef,
			Handler: journalWriteHandler(svc),
		},
	}
}

func journalWriteHandler(svc *Service) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			Content string `json:"content"`
		}

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		if args.Content == "" {
			return `{"error":"empty content"}`, nil
		}

		err = svc.WritePage(ctx, args.Content)
		if err != nil {
			return llm.ToolError(err), nil
		}

		return `{"written":true}`, nil
	}
}
