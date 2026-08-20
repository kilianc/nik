package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/llm"
)

const (
	maxQueryRows         = 500
	maxQueryContextBytes = 32 * 1024
	maxQueryValueBytes   = 1024
)

var queryToolDef = llm.ToolDef{
	Name:        "db_query",
	Description: "Run a read-only SQL query against nik's SQLite database. Only SELECT, WITH, SHOW, DESCRIBE, and read-only PRAGMA (table_info, table_list, foreign_key_list, etc.) are allowed. Single statement only.",
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

var settingSetToolDef = llm.ToolDef{
	Name:        "setting_set",
	Description: "Write a key-value pair to nik's settings table. Use for persistent configuration flags.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"key": map[string]any{
				"type":        "string",
				"description": "Setting key.",
			},
			"value": map[string]any{
				"type":        "string",
				"description": "Setting value.",
			},
		},
		"required":             []string{"key", "value"},
		"additionalProperties": false,
	},
}

var pruneToolDef = llm.ToolDef{
	Name:        "db_prune",
	Description: "Delete activations, tasks, and all dependents older than the configured retention period. Returns count of deleted rows. Use to reclaim disk space.",
	Parameters: map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"required":             []string{},
		"additionalProperties": false,
	},
}

func BuildTools(roConn *sql.DB, rwConn *sql.DB, retention func() time.Duration) []llm.Tool {
	return []llm.Tool{
		{Def: queryToolDef, Handler: queryHandler(roConn)},
		{Def: pruneToolDef, Handler: pruneHandler(rwConn, retention)},
		{Def: settingSetToolDef, Handler: settingSetHandler(rwConn)},
	}
}

func settingSetHandler(conn *sql.DB) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}
		if args.Key == "" {
			return `{"error":"empty key"}`, nil
		}

		err = SettingSet(ctx, conn, args.Key, args.Value)
		if err != nil {
			return llm.ToolError(err), nil
		}

		return fmt.Sprintf(`{"ok":true,"key":%q}`, args.Key), nil
	}
}

func pruneHandler(conn *sql.DB, retention func() time.Duration) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		cutoff := time.Now().UTC().Add(-retention()).Format("2006-01-02T15:04:05.000Z")

		n, err := Prune(ctx, conn, cutoff)
		if err != nil {
			return llm.ToolError(err), nil
		}

		return fmt.Sprintf(`{"rows_deleted":%d,"cutoff":"%s"}`, n, cutoff), nil
	}
}

// ErrNotReadOnly is a query that would write. The rejection is the point of
// db_query: nik and a console can look at everything and change nothing.
var ErrNotReadOnly = errors.New("only single SELECT, WITH, SHOW, DESCRIBE, or read-only PRAGMA statements are allowed")

// QueryResult is what a read-only query produced, already bounded and with
// sensitive columns redacted.
type QueryResult struct {
	Rows  []map[string]any `json:"rows"`
	Count int              `json:"count"`
	// Truncated says the answer is partial and why. Never silently cut: a
	// result that lost rows without saying so reads as a complete answer.
	Truncated        bool   `json:"truncated,omitempty"`
	TruncationReason string `json:"truncation_reason,omitempty"`
	MaxBytes         int    `json:"max_bytes,omitempty"`
}

// Query runs one read-only statement and bounds the result.
//
// Shared by the db_query brain tool and the API, deliberately: the redaction
// and the read-only check are the safety properties of this operation, and a
// second implementation would eventually have only one of them.
func Query(ctx context.Context, conn *sql.DB, query string) (QueryResult, error) {
	if query == "" {
		return QueryResult{}, fmt.Errorf("%w: empty query", ErrNotReadOnly)
	}
	if !isReadOnly(query) {
		return QueryResult{}, ErrNotReadOnly
	}

	rows, err := conn.QueryContext(ctx, query)
	if err != nil {
		return QueryResult{}, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return QueryResult{}, err
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
			return QueryResult{}, err
		}

		row := make(map[string]any, len(cols))
		for i, col := range cols {
			row[col] = normalizeValue(vals[i])
		}
		applyRedaction(row)

		results = append(results, row)

		data, err := marshalQueryResult(results, "")
		if err != nil {
			return QueryResult{}, err
		}

		if len(data) > maxQueryContextBytes {
			results = results[:len(results)-1]
			truncationReason = "context_bytes"

			break
		}
	}

	err = rows.Err()
	if err != nil {
		return QueryResult{}, err
	}

	data, err := marshalQueryResult(results, truncationReason)
	if err != nil {
		return QueryResult{}, err
	}

	for len(data) > maxQueryContextBytes && len(results) > 0 {
		results = results[:len(results)-1]

		data, err = marshalQueryResult(results, truncationReason)
		if err != nil {
			return QueryResult{}, err
		}
	}

	out := QueryResult{Rows: results, Count: len(results)}
	if out.Rows == nil {
		out.Rows = []map[string]any{}
	}
	if truncationReason != "" {
		out.Truncated = true
		out.TruncationReason = truncationReason
		out.MaxBytes = maxQueryContextBytes
	}

	return out, nil
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

		result, err := Query(ctx, conn, args.Query)
		if err != nil {
			return llm.ToolError(err), nil
		}

		data, err := json.Marshal(result)
		if err != nil {
			return llm.ToolError(err), nil
		}

		return string(data), nil
	}
}

var readOnlyPrefixes = []string{"SELECT", "WITH", "SHOW", "DESCRIBE"}

// safePragmas lists PRAGMA commands that only read metadata.
// State-mutating pragmas (journal_mode, foreign_keys, etc.) are excluded
// because they can alter DB behaviour through the db_query tool.
var safePragmas = map[string]bool{
	"TABLE_INFO":        true,
	"TABLE_LIST":        true,
	"TABLE_XINFO":       true,
	"FOREIGN_KEY_LIST":  true,
	"FOREIGN_KEY_CHECK": true,
	"INDEX_LIST":        true,
	"INDEX_INFO":        true,
	"DATABASE_LIST":     true,
	"COMPILE_OPTIONS":   true,
	"INTEGRITY_CHECK":   true,
	"QUICK_CHECK":       true,
}

// isReadOnly rejects multi-statement queries to prevent piggy-backed writes
// (e.g. "SELECT 1; DROP TABLE x") — mattn/go-sqlite3 executes all statements
// passed to QueryContext, so a prefix check alone is not sufficient.
func isReadOnly(query string) bool {
	trimmed := strings.TrimRight(strings.TrimSpace(query), ";")
	if strings.Contains(trimmed, ";") {
		return false
	}

	upper := strings.ToUpper(strings.TrimSpace(trimmed))

	for _, prefix := range readOnlyPrefixes {
		if strings.HasPrefix(upper, prefix) {
			return true
		}
	}

	if strings.HasPrefix(upper, "PRAGMA") {
		return isSafePragma(upper)
	}

	return false
}

func isSafePragma(upper string) bool {
	rest := strings.TrimSpace(strings.TrimPrefix(upper, "PRAGMA"))
	name := strings.FieldsFunc(rest, func(r rune) bool {
		return r == '(' || r == ' ' || r == '.'
	})
	if len(name) == 0 {
		return false
	}

	pragmaName := name[0]
	if strings.Contains(rest, ".") {
		parts := strings.SplitN(rest, ".", 2)
		if len(parts) == 2 {
			pragmaName = strings.FieldsFunc(parts[1], func(r rune) bool {
				return r == '(' || r == ' '
			})[0]
		}
	}

	return safePragmas[pragmaName]
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

func applyRedaction(row map[string]any) {
	v, ok := row["is_redacted"]
	if !ok {
		return
	}

	switch r := v.(type) {
	case int64:
		if r == 0 {
			return
		}
	case float64:
		if r == 0 {
			return
		}
	default:
		return
	}

	if _, has := row["body"]; has {
		row["body"] = "[message redacted]"
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
