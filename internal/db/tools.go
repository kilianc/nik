package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/kciuffolo/nik/internal/llm"
)

const maxQueryRows = 500

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

func BuildTools(conn *sql.DB) []llm.Tool {
	return []llm.Tool{
		{
			Def:        queryToolDef,
			Handler:    queryHandler(conn),
			Privileged: true,
		},
	}
}

func queryHandler(conn *sql.DB) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			Query string `json:"query"`
		}

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}
		if args.Query == "" {
			return `{"error":"empty query"}`, nil
		}

		if !isReadOnly(args.Query) {
			return `{"error":"only SELECT, WITH, SHOW, DESCRIBE, and PRAGMA statements are allowed"}`, nil
		}

		rows, err := conn.QueryContext(ctx, args.Query)
		if err != nil {
			return llm.ToolError(err), nil
		}
		defer rows.Close()

		cols, err := rows.Columns()
		if err != nil {
			return llm.ToolError(err), nil
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
				return llm.ToolError(err), nil
			}

			row := make(map[string]any, len(cols))
			for i, col := range cols {
				row[col] = normalizeValue(vals[i])
			}

			results = append(results, row)
		}

		err = rows.Err()
		if err != nil {
			return llm.ToolError(err), nil
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
			return llm.ToolError(err), nil
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
