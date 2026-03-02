package dream

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/kciuffolo/nik/internal/llm"
)

var dreamWriteDef = llm.ToolDef{
	Name:        "dream_write",
	Description: "Record this dream pass. Write your raw dream experience — what surfaced, what connected, what shifted. Called once per pass.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{
				"type":        "string",
				"description": "The dream content for this pass.",
			},
		},
		"required":             []string{"content"},
		"additionalProperties": false,
	},
}

var soulEvolveDef = llm.ToolDef{
	Name:        "soul_evolve",
	Description: "Write a new version of your soul. This is who you are right now — personality, values, taste, ideas, voice, relationships, interests, growth edges. Write it in your own voice. Only available during the wake pass.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{
				"type":        "string",
				"description": "The full soul document. Structured sections (## personality, ## core values, ## taste, ## ideas, ## voice, ## relationships, ## interests, ## growth edges) plus any new sections you've grown into.",
			},
		},
		"required":             []string{"content"},
		"additionalProperties": false,
	},
}

func BuildTools(svc *Service) []llm.Tool {
	return []llm.Tool{
		{Def: dreamWriteDef, Handler: dreamWriteHandler(svc)},
		{Def: soulEvolveDef, Handler: soulEvolveHandler(svc)},
	}
}

func dreamWriteHandler(svc *Service) llm.ToolExecutor {
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

		pass, err := passFromContext(ctx)
		if err != nil {
			return llm.ToolError(err), nil
		}

		err = svc.WriteDream(ctx, pass, args.Content)
		if err != nil {
			return llm.ToolError(err), nil
		}

		return llm.ToolResult(map[string]any{"written": true, "pass": pass}), nil
	}
}

func soulEvolveHandler(svc *Service) llm.ToolExecutor {
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

		pass, err := passFromContext(ctx)
		if err != nil {
			return llm.ToolError(err), nil
		}

		if pass != totalPasses {
			return `{"error":"soul_evolve is only available during the wake pass"}`, nil
		}

		version, err := svc.WriteSoul(ctx, args.Content)
		if err != nil {
			return llm.ToolError(err), nil
		}

		return llm.ToolResult(map[string]any{"evolved": true, "version": version}), nil
	}
}

func passFromContext(ctx context.Context) (int, error) {
	meta, ok := ctx.Value("meta").(map[string]string)
	if !ok {
		return 0, fmt.Errorf("missing meta context")
	}

	passStr, ok := meta["dream_pass"]
	if !ok {
		return 0, fmt.Errorf("missing dream_pass in meta")
	}

	pass, err := strconv.Atoi(passStr)
	if err != nil {
		return 0, fmt.Errorf("parse dream_pass %q: %w", passStr, err)
	}

	return pass, nil
}
