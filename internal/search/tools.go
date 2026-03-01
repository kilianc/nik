package search

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kciuffolo/nik/internal/llm"
)

const maxQueryRows = 50

var queryToolDef = llm.ToolDef{
	Name:        "db_query",
	Description: "Run a read-only SQL query against nik's SQLite database. Only SELECT, WITH, SHOW, DESCRIBE, and PRAGMA are allowed.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Read-only SQL query.",
			},
		},
		"required":             []string{"query"},
		"additionalProperties": false,
	},
}

var contactSearchToolDef = llm.ToolDef{
	Name:        "search_contacts",
	Description: "Search contacts by id, external ids, email, phone, or fuzzy text. Accepts multiple queries at once to reduce round-trips.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"queries": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "One or more search queries.",
			},
			"threshold": map[string]any{
				"type":        "number",
				"description": "Fuzzy threshold between 0 and 1. Use 0.85 unless needed otherwise.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Max rows per query.",
			},
		},
		"required":             []string{"queries", "threshold", "limit"},
		"additionalProperties": false,
	},
}

func BuildTools(conn *sql.DB, svc *Service) []llm.Tool {
	return []llm.Tool{
		{
			Def:        queryToolDef,
			Handler:    queryHandler(conn),
			Privileged: true,
		},
		{
			Def:     contactSearchToolDef,
			Handler: contactSearchHandler(svc),
		},
	}
}

func contactSearchHandler(svc *Service) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			Queries   []string `json:"queries"`
			Threshold float64  `json:"threshold"`
			Limit     int      `json:"limit"`
		}

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
		}

		if len(args.Queries) == 0 {
			return `{"error":"empty queries"}`, nil
		}

		if args.Threshold == 0 {
			args.Threshold = 0.85
		}
		if args.Limit == 0 {
			args.Limit = 10
		}

		out := map[string]any{}

		for _, q := range args.Queries {
			results, err := svc.SearchContacts(ctx, q, args.Threshold, args.Limit)
			if err != nil {
				out[q] = map[string]any{"error": err.Error()}
				continue
			}

			out[q] = map[string]any{
				"rows":  results,
				"count": len(results),
			}
		}

		data, err := json.Marshal(map[string]any{
			"results": out,
			"limit":   args.Limit,
			"thresh":  args.Threshold,
		})
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
		}

		return string(data), nil
	}
}

func queryHandler(conn *sql.DB) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			Query string `json:"query"`
		}

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
		}
		if args.Query == "" {
			return `{"error":"empty query"}`, nil
		}

		if !isReadOnly(args.Query) {
			return `{"error":"only SELECT, WITH, SHOW, DESCRIBE, and PRAGMA statements are allowed"}`, nil
		}

		rows, err := conn.QueryContext(ctx, args.Query)
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
		}
		defer rows.Close()

		cols, err := rows.Columns()
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
		}

		var results []map[string]any
		truncated := false

		for rows.Next() {
			if len(results) >= maxQueryRows {
				truncated = true
				break
			}

			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}

			err = rows.Scan(ptrs...)
			if err != nil {
				return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
			}

			row := make(map[string]any, len(cols))
			for i, col := range cols {
				row[col] = normalizeValue(vals[i])
			}

			results = append(results, row)
		}

		err = rows.Err()
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
		}

		out := map[string]any{
			"rows":  results,
			"count": len(results),
		}
		if truncated {
			out["truncated"] = true
		}

		data, err := json.Marshal(out)
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
		}

		return string(data), nil
	}
}

var readOnlyPrefixes = []string{"SELECT", "WITH", "SHOW", "DESCRIBE", "PRAGMA"}

func isReadOnly(query string) bool {
	upper := strings.ToUpper(strings.TrimSpace(query))
	for _, prefix := range readOnlyPrefixes {
		if strings.HasPrefix(upper, prefix) {
			return true
		}
	}
	return false
}

func normalizeValue(v any) any {
	switch val := v.(type) {
	case []byte:
		return string(val)
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = normalizeValue(item)
		}
		return out
	default:
		return v
	}
}
