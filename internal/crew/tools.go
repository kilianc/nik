package crew

import (
	"context"
	"encoding/json"

	"github.com/kciuffolo/nik/internal/llm"
)

var hireToolDef = llm.ToolDef{
	Name:        "crew_hire",
	Description: "Hire a new crew member. Give them a name and a base prompt that defines who they are and how they work. This is a big moment -- you're growing your team.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "The crew member's name.",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "Base prompt defining personality, expertise, and working style.",
			},
		},
		"required":             []string{"name", "prompt"},
		"additionalProperties": false,
	},
}

type hireArgs struct {
	Name   string `json:"name"`
	Prompt string `json:"prompt"`
}

func BuildTools(svc *Service) []llm.Tool {
	return []llm.Tool{
		{Def: hireToolDef, Handler: hireHandler(svc)},
	}
}

func hireHandler(svc *Service) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args hireArgs

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		if args.Name == "" {
			return llm.ToolErrorf("name is required"), nil
		}
		if args.Prompt == "" {
			return llm.ToolErrorf("prompt is required"), nil
		}

		m, err := svc.Hire(ctx, args.Name, args.Prompt)
		if err != nil {
			return llm.ToolError(err), nil
		}

		return llm.ToolResult(map[string]any{
			"id":   m.ID,
			"name": m.Name,
		}), nil
	}
}
