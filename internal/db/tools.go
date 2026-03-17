package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/kciuffolo/nik/internal/llm"
)

const (
	maxQueryRows         = 500
	maxQueryContextBytes = 32 * 1024
	maxQueryValueBytes   = 1024
)

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
		truncationReason := ""

		for rows.Next() {
			if len(results) >= maxQueryRows {
				truncationReason = "rows"
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

			data, err := marshalQueryResult(results, "")
			if err != nil {
				return llm.ToolError(err), nil
			}

			if len(data) > maxQueryContextBytes {
				results = results[:len(results)-1]
				truncationReason = "context_bytes"
				break
			}
		}

		err = rows.Err()
		if err != nil {
			return llm.ToolError(err), nil
		}

		data, err := marshalQueryResult(results, truncationReason)
		if err != nil {
			return llm.ToolError(err), nil
		}

		for len(data) > maxQueryContextBytes && len(results) > 0 {
			results = results[:len(results)-1]

			data, err = marshalQueryResult(results, truncationReason)
			if err != nil {
				return llm.ToolError(err), nil
			}
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
		return truncateString(string(val), maxQueryValueBytes)
	case string:
		return truncateString(val, maxQueryValueBytes)
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

func marshalQueryResult(results []map[string]any, truncationReason string) ([]byte, error) {
	out := map[string]any{
		"rows":  results,
		"count": len(results),
	}

	if truncationReason != "" {
		out["truncated"] = true
		out["truncation_reason"] = truncationReason
		out["max_bytes"] = maxQueryContextBytes
	}

	return json.Marshal(out)
}

func truncateString(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}

	const suffix = " [truncated]"

	limit := maxBytes - len(suffix)
	if limit <= 0 {
		return suffix
	}

	return s[:limit] + suffix
}
