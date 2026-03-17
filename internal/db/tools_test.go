package db

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/kciuffolo/nik/internal/llm"
)

func TestIsReadOnlyRecognizesAllowedPrefixes(t *testing.T) {
	if !isReadOnly("select 1") {
		t.Fatalf("expected SELECT query to be read-only")
	}
	if !isReadOnly("  with x as (select 1) select * from x") {
		t.Fatalf("expected WITH query to be read-only")
	}
	if isReadOnly("delete from message") {
		t.Fatalf("expected DELETE query to be rejected")
	}
}

func TestNormalizeValueConvertsBytesAndNestedSlices(t *testing.T) {
	got := normalizeValue([]any{[]byte("x"), []any{[]byte("y")}})
	want := []any{"x", []any{"y"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestNormalizeValueTruncatesLongStrings(t *testing.T) {
	got := normalizeValue(stringsOfLen(maxQueryValueBytes + 10))

	s, ok := got.(string)
	if !ok {
		t.Fatalf("expected string, got %T", got)
	}

	if len(s) > maxQueryValueBytes {
		t.Fatalf("expected truncated string length <= %d, got %d", maxQueryValueBytes, len(s))
	}
	if !strings.HasSuffix(s, " [truncated]") {
		t.Fatalf("expected truncated suffix, got %q", s)
	}
}

func TestQueryHandlerTruncatesContextBytes(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	call := llm.ToolCall{
		Name:      "db_query",
		Arguments: `{"query":"WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM seq WHERE n < 200) SELECT n, hex(zeroblob(2048)) AS payload FROM seq"}`,
	}

	out, err := queryHandler(conn)(ctx, call)
	if err != nil {
		t.Fatalf("run query handler: %v", err)
	}

	if len(out) > maxQueryContextBytes {
		t.Fatalf("expected output length <= %d, got %d", maxQueryContextBytes, len(out))
	}

	var result struct {
		Count            int              `json:"count"`
		Rows             []map[string]any `json:"rows"`
		Truncated        bool             `json:"truncated"`
		TruncationReason string           `json:"truncation_reason"`
		MaxBytes         int              `json:"max_bytes"`
	}

	err = json.Unmarshal([]byte(out), &result)
	if err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	if !result.Truncated {
		t.Fatalf("expected truncated result")
	}
	if result.TruncationReason != "context_bytes" {
		t.Fatalf("expected context_bytes truncation, got %q", result.TruncationReason)
	}
	if result.MaxBytes != maxQueryContextBytes {
		t.Fatalf("expected max_bytes %d, got %d", maxQueryContextBytes, result.MaxBytes)
	}
	if result.Count != len(result.Rows) {
		t.Fatalf("expected count %d to match rows %d", result.Count, len(result.Rows))
	}
	if len(result.Rows) == 0 {
		t.Fatalf("expected at least one row in truncated result")
	}
}

func stringsOfLen(n int) string {
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = 'x'
	}

	return string(buf)
}
