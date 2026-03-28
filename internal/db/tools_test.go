package db

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/llm"
)

func TestIsReadOnlyRecognizesAllowedPrefixes(t *testing.T) {
	allowed := []string{
		"select 1",
		"SELECT * FROM message",
		"  with x as (select 1) select * from x",
		"DESCRIBE foo",
		"SELECT 1;",
		"SELECT 1 ;  ",
		"PRAGMA table_info(message)",
		"PRAGMA main.table_info(message)",
		"PRAGMA foreign_key_list(task)",
		"PRAGMA integrity_check",
	}
	for _, q := range allowed {
		if !isReadOnly(q) {
			t.Fatalf("expected %q to be read-only", q)
		}
	}

	rejected := []string{
		"delete from message",
		"DROP TABLE message",
		"UPDATE contact SET name = 'x'",
		"INSERT INTO message (id) VALUES ('x')",
		"SELECT 1; DROP TABLE message",
		"SELECT 1; DELETE FROM contact",
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=OFF",
		"PRAGMA journal_mode",
		"PRAGMA",
	}
	for _, q := range rejected {
		if isReadOnly(q) {
			t.Fatalf("expected %q to be rejected", q)
		}
	}
}

func TestNormalizeValue(t *testing.T) {
	t.Run("converts bytes and nested slices", func(t *testing.T) {
		got := normalizeValue([]any{[]byte("x"), []any{[]byte("y")}})
		want := []any{"x", []any{"y"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("expected %v, got %v", want, got)
		}
	})

	t.Run("truncates long strings", func(t *testing.T) {
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
	})
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

func TestReadOnlyConnectionRejectsWrites(t *testing.T) {
	dir := t.TempDir()
	dbPath := dir + "/test.db"

	rw, err := Open(dbPath, nil)
	if err != nil {
		t.Fatalf("open rw db: %v", err)
	}
	defer rw.Close()

	ro, err := OpenReadOnly(dbPath, nil)
	if err != nil {
		t.Fatalf("open read-only db: %v", err)
	}
	defer ro.Close()

	_, err = ro.Exec("INSERT INTO contact (id, name) VALUES ('test', 'test')")
	if err == nil {
		t.Fatal("expected read-only connection to reject INSERT")
	}
	if !strings.Contains(err.Error(), "readonly") {
		t.Fatalf("expected readonly error, got: %v", err)
	}

	var count int
	err = ro.QueryRow("SELECT count(*) FROM contact").Scan(&count)
	if err != nil {
		t.Fatalf("read-only SELECT failed: %v", err)
	}
}

func TestPruneHandler(t *testing.T) {
	ctx := context.Background()
	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	retention := func() time.Duration { return 24 * time.Hour }
	handler := pruneHandler(conn, retention)

	call := llm.ToolCall{Name: "db_prune", Arguments: "{}"}
	out, err := handler(ctx, call)
	if err != nil {
		t.Fatalf("prune handler: %v", err)
	}

	var result struct {
		RowsDeleted int    `json:"rows_deleted"`
		Cutoff      string `json:"cutoff"`
	}
	err = json.Unmarshal([]byte(out), &result)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if result.Cutoff == "" {
		t.Fatal("expected non-empty cutoff")
	}
}

func stringsOfLen(n int) string {
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = 'x'
	}

	return string(buf)
}
