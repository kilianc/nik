package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/kciuffolo/nik/internal/llm"
)

const dedupeThreshold = 0.85

const mergePrompt = `You are merging two related memories into one. Combine all unique information from both into a single concise memory. Preserve specific facts, dates, names, and preferences. Drop redundancy. Return a JSON object with a single key "merged" containing the merged memory text.`

var storeMemoryToolDef = llm.ToolDef{
	Name:        "store_memory",
	Description: "Persist a new memory for future retrieval. Store atomic facts — one idea per memory.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{
				"type":        "string",
				"description": "The memory content to store.",
			},
		},
		"required":             []string{"content"},
		"additionalProperties": false,
	},
}

var searchMemoryToolDef = llm.ToolDef{
	Name:        "search_memory",
	Description: "Search the memory store for relevant memories by semantic similarity. Accepts multiple queries at once to reduce round-trips.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"queries": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "One or more search queries. Results are deduplicated across queries.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Max results per query. Default 10.",
			},
		},
		"required":             []string{"queries", "limit"},
		"additionalProperties": false,
	},
}

var deleteMemoryToolDef = llm.ToolDef{
	Name:        "delete_memory",
	Description: "Delete one or more memories by ID. Use after search_memory to remove outdated or incorrect memories.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ids": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "IDs of memories to delete (from search_memory results).",
			},
		},
		"required":             []string{"ids"},
		"additionalProperties": false,
	},
}

func BuildTools(svc *Service) []llm.Tool {
	return []llm.Tool{
		{Def: storeMemoryToolDef, Handler: storeMemoryHandler(svc)},
		{Def: searchMemoryToolDef, Handler: searchMemoryHandler(svc)},
		{Def: deleteMemoryToolDef, Handler: deleteMemoryHandler(svc)},
	}
}

func storeMemoryHandler(svc *Service) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			Content string `json:"content"`
		}

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
		}

		metadata := map[string]any{"source": "brain"}

		var source, sourceID string
		if meta, ok := ctx.Value("meta").(map[string]string); ok {
			source = meta["source"]
			sourceID = meta["source_id"]
		}

		similar, searchErr := svc.Search(ctx, args.Content, 5)
		if searchErr != nil {
			slog.Warn("store_memory similarity search failed, inserting anyway", "pkg", "memory", "err", searchErr)

			m, err := svc.Add(ctx, args.Content, metadata, source, sourceID)
			if err != nil {
				return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
			}

			return fmt.Sprintf(`{"stored":true,"id":%q}`, m.ID), nil
		}

		if len(similar) > 0 && similar[0].Score >= dedupeThreshold {
			existing := similar[0]
			slog.Info("store_memory near-duplicate found", "pkg", "memory",
				"score", fmt.Sprintf("%.3f", existing.Score),
				"existing", truncate(existing.Content, 80),
				"incoming", truncate(args.Content, 80))

			merged, mergeErr := mergeMemories(ctx, svc.llm, existing.Content, args.Content)
			if mergeErr != nil {
				slog.Warn("store_memory merge failed, inserting as new", "pkg", "memory", "err", mergeErr)

				m, err := svc.Add(ctx, args.Content, metadata, source, sourceID)
				if err != nil {
					return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
				}

				return fmt.Sprintf(`{"stored":true,"id":%q}`, m.ID), nil
			}

			_ = svc.Delete(ctx, existing.ID)

			m, err := svc.Add(ctx, merged, metadata, source, sourceID)
			if err != nil {
				return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
			}

			slog.Info("store_memory merged", "pkg", "memory", "old_id", existing.ID, "new_id", m.ID)
			return fmt.Sprintf(`{"merged":true,"old_id":%q,"new_id":%q}`, existing.ID, m.ID), nil
		}

		m, err := svc.Add(ctx, args.Content, metadata, source, sourceID)
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
		}

		return fmt.Sprintf(`{"stored":true,"id":%q}`, m.ID), nil
	}
}

func searchMemoryHandler(svc *Service) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			Queries []string `json:"queries"`
			Limit   int      `json:"limit"`
		}

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
		}

		if len(args.Queries) == 0 {
			return `{"error":"empty queries"}`, nil
		}

		if args.Limit <= 0 {
			args.Limit = 10
		}

		results, err := svc.SearchMulti(ctx, args.Queries, args.Limit)
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
		}

		type memOut struct {
			ID      string  `json:"id"`
			Content string  `json:"content"`
			Score   float64 `json:"score"`
		}

		out := make([]memOut, len(results))
		for i, r := range results {
			out[i] = memOut{ID: r.ID, Content: r.Content, Score: r.Score}
		}

		b, _ := json.Marshal(map[string]any{"memories": out})
		return string(b), nil
	}
}

func deleteMemoryHandler(svc *Service) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			IDs []string `json:"ids"`
		}

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
		}

		if len(args.IDs) == 0 {
			return `{"error":"empty ids"}`, nil
		}

		var deleted []string
		errors := map[string]string{}

		for _, memID := range args.IDs {
			err := svc.Delete(ctx, memID)
			if err != nil {
				errors[memID] = err.Error()
				continue
			}

			deleted = append(deleted, memID)
		}

		slog.Info("delete_memory", "pkg", "memory", "deleted", len(deleted), "errors", len(errors))

		b, _ := json.Marshal(map[string]any{"deleted": deleted, "errors": errors})
		return string(b), nil
	}
}

func mergeMemories(ctx context.Context, llmClient *llm.Client, existing, incoming string) (string, error) {
	input := fmt.Sprintf("Merge the following two memories into one. Respond in json.\n\nExisting: %s\n\nNew: %s", existing, incoming)

	raw, _, _, err := llmClient.Complete(ctx, mergePrompt, input, nil, nil)
	if err != nil {
		return "", fmt.Errorf("merge completion: %w", err)
	}

	var result struct {
		Merged string `json:"merged"`
	}

	err = json.Unmarshal([]byte(raw), &result)
	if err != nil {
		return "", fmt.Errorf("parse merge response: %w", err)
	}

	if result.Merged == "" {
		return "", fmt.Errorf("merge returned empty content")
	}

	return result.Merged, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}

	return s[:n] + "..."
}
