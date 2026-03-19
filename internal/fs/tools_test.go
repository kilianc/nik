package fs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kciuffolo/nik/internal/llm"
)

func callTool(t *testing.T, home, name, args string) string {
	t.Helper()
	tools := BuildTools(home)
	for _, tool := range tools {
		if tool.Def.Name == name {
			tc := llm.ToolCall{CallID: "test", Name: name, Arguments: args}
			result, err := tool.Handler(context.Background(), tc)
			if err != nil {
				t.Fatalf("handler error: %v", err)
			}
			return result
		}
	}
	t.Fatalf("tool %q not found", name)
	return ""
}

func call(t *testing.T, home, args string) string {
	t.Helper()
	return callTool(t, home, "write_file", args)
}

func callRead(t *testing.T, home, args string) string {
	t.Helper()
	return callTool(t, home, "read_file", args)
}

func TestWriteCreatesFile(t *testing.T) {
	dir := t.TempDir()
	args := `{"action":"write","path":"out.txt","content":"hello world"}`

	result := call(t, dir, args)
	if !strings.Contains(result, `"ok":true`) {
		t.Fatalf("expected ok, got %s", result)
	}

	data, err := os.ReadFile(filepath.Join(dir, "out.txt"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("content = %q, want %q", string(data), "hello world")
	}
}

func TestWriteOverwrites(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "out.txt"), []byte("old"), 0o644)

	args := `{"action":"write","path":"out.txt","content":"new"}`
	call(t, dir, args)

	data, _ := os.ReadFile(filepath.Join(dir, "out.txt"))
	if string(data) != "new" {
		t.Errorf("content = %q, want %q", string(data), "new")
	}
}

func TestAppend(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "out.txt"), []byte("first"), 0o644)

	args := `{"action":"append","path":"out.txt","content":" second"}`
	call(t, dir, args)

	data, _ := os.ReadFile(filepath.Join(dir, "out.txt"))
	if string(data) != "first second" {
		t.Errorf("content = %q, want %q", string(data), "first second")
	}
}

func TestAppendCreatesIfMissing(t *testing.T) {
	dir := t.TempDir()
	args := `{"action":"append","path":"new.txt","content":"first"}`
	call(t, dir, args)

	data, err := os.ReadFile(filepath.Join(dir, "new.txt"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "first" {
		t.Errorf("content = %q, want %q", string(data), "first")
	}
}

func TestRelativePath(t *testing.T) {
	dir := t.TempDir()
	args := `{"action":"write","path":"sub/file.txt","content":"ok"}`
	call(t, dir, args)

	data, err := os.ReadFile(filepath.Join(dir, "sub", "file.txt"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "ok" {
		t.Errorf("content = %q, want %q", string(data), "ok")
	}
}

func TestPathTraversalBlocked(t *testing.T) {
	dir := t.TempDir()
	args := `{"action":"write","path":"../../etc/passwd","content":"bad"}`

	result := call(t, dir, args)
	if !strings.Contains(result, "error") {
		t.Fatalf("expected error for traversal, got %s", result)
	}
}

func TestAbsolutePathBlocked(t *testing.T) {
	dir := t.TempDir()
	args := `{"action":"write","path":"/tmp/escape.txt","content":"bad"}`

	result := call(t, dir, args)
	if !strings.Contains(result, "absolute paths not allowed") {
		t.Fatalf("expected absolute path error, got %s", result)
	}
}

func TestSymlinkEscape(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()

	os.Symlink(outside, filepath.Join(home, "escape"))

	args := `{"action":"write","path":"escape/pwned.txt","content":"bad"}`
	result := call(t, home, args)
	if !strings.Contains(result, "error") {
		t.Fatalf("expected error for symlink escape, got %s", result)
	}

	if _, err := os.Stat(filepath.Join(outside, "pwned.txt")); err == nil {
		t.Fatal("file was written outside home via symlink")
	}
}

func TestEmptyHome(t *testing.T) {
	args := `{"action":"write","path":"file.txt","content":"x"}`
	result := call(t, "", args)
	if !strings.Contains(result, "requires a home directory") {
		t.Fatalf("expected home directory error, got %s", result)
	}
}

func TestEmptyPath(t *testing.T) {
	dir := t.TempDir()
	args := `{"action":"write","path":"","content":"x"}`

	result := call(t, dir, args)
	if !strings.Contains(result, "empty path") {
		t.Fatalf("expected empty path error, got %s", result)
	}
}

func TestUnknownAction(t *testing.T) {
	dir := t.TempDir()
	args := `{"action":"delete","path":"file.txt","content":""}`

	result := call(t, dir, args)
	if !strings.Contains(result, "unknown action") {
		t.Fatalf("expected unknown action error, got %s", result)
	}
}

func TestResultIncludesBytes(t *testing.T) {
	dir := t.TempDir()
	args := `{"action":"write","path":"sized.txt","content":"12345"}`

	result := call(t, dir, args)

	var out map[string]any
	json.Unmarshal([]byte(result), &out)

	b, ok := out["bytes"].(float64)
	if !ok || b != 5 {
		t.Errorf("bytes = %v, want 5", out["bytes"])
	}
}

// read_file tests

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("line1\nline2\nline3\n"), 0o644)

	result := callRead(t, dir, `{"path":"hello.txt"}`)
	if !strings.Contains(result, "1: line1") || !strings.Contains(result, "3: line3") {
		t.Fatalf("expected numbered lines, got %s", result)
	}

	var out map[string]any
	json.Unmarshal([]byte(result), &out)
	if tl, _ := out["total_lines"].(float64); tl != 3 {
		t.Errorf("total_lines = %v, want 3", out["total_lines"])
	}
}

func TestReadFileOffset(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("a\nb\nc\nd\ne\n"), 0o644)

	result := callRead(t, dir, `{"path":"f.txt","offset":3}`)
	if strings.Contains(result, "1: a") || strings.Contains(result, "2: b") {
		t.Fatalf("should not contain lines before offset, got %s", result)
	}
	if !strings.Contains(result, "3: c") {
		t.Fatalf("should contain line at offset, got %s", result)
	}
}

func TestReadFileLimit(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("a\nb\nc\nd\ne\n"), 0o644)

	result := callRead(t, dir, `{"path":"f.txt","limit":2}`)
	if !strings.Contains(result, "1: a") || !strings.Contains(result, "2: b") {
		t.Fatalf("should contain first 2 lines, got %s", result)
	}
	if strings.Contains(result, "3: c") {
		t.Fatalf("should not contain line 3, got %s", result)
	}
}

func TestReadFileOffsetAndLimit(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("a\nb\nc\nd\ne\n"), 0o644)

	result := callRead(t, dir, `{"path":"f.txt","offset":2,"limit":2}`)
	if !strings.Contains(result, "2: b") || !strings.Contains(result, "3: c") {
		t.Fatalf("should contain lines 2-3, got %s", result)
	}
	if strings.Contains(result, "1: a") || strings.Contains(result, "4: d") {
		t.Fatalf("should not contain lines outside range, got %s", result)
	}
}

func TestReadFileTruncatesLongLines(t *testing.T) {
	dir := t.TempDir()
	long := strings.Repeat("x", 3000)
	os.WriteFile(filepath.Join(dir, "long.txt"), []byte(long+"\n"), 0o644)

	result := callRead(t, dir, `{"path":"long.txt"}`)

	var out map[string]any
	json.Unmarshal([]byte(result), &out)
	content, _ := out["content"].(string)

	lineContent := strings.TrimPrefix(strings.TrimSpace(content), "1: ")
	if len(lineContent) > maxLineLength+10 {
		t.Errorf("line not truncated: len=%d, want <=%d", len(lineContent), maxLineLength)
	}
}

func TestReadFileDefaultLimit(t *testing.T) {
	dir := t.TempDir()

	var sb strings.Builder
	for i := 1; i <= 2500; i++ {
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	os.WriteFile(filepath.Join(dir, "big.txt"), []byte(sb.String()), 0o644)

	result := callRead(t, dir, `{"path":"big.txt"}`)

	var out map[string]any
	json.Unmarshal([]byte(result), &out)

	if tl, _ := out["total_lines"].(float64); tl != 2500 {
		t.Errorf("total_lines = %v, want 2500", out["total_lines"])
	}
	if trunc, _ := out["truncated"].(bool); !trunc {
		t.Error("expected truncated=true for file exceeding default limit")
	}

	content, _ := out["content"].(string)
	if strings.Contains(content, "2001: ") {
		t.Error("should not contain lines beyond default limit")
	}
}

func TestReadFileNotFound(t *testing.T) {
	dir := t.TempDir()
	result := callRead(t, dir, `{"path":"nope.txt"}`)
	if !strings.Contains(result, "error") {
		t.Fatalf("expected error for missing file, got %s", result)
	}
}

func TestReadFileTraversalBlocked(t *testing.T) {
	dir := t.TempDir()
	result := callRead(t, dir, `{"path":"../../etc/passwd"}`)
	if !strings.Contains(result, "error") {
		t.Fatalf("expected error for traversal, got %s", result)
	}
}

func TestReadFileAbsolutePathBlocked(t *testing.T) {
	dir := t.TempDir()
	result := callRead(t, dir, `{"path":"/etc/passwd"}`)
	if !strings.Contains(result, "absolute paths not allowed") {
		t.Fatalf("expected absolute path error, got %s", result)
	}
}

func TestReadFileSymlinkBlocked(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("nope"), 0o644)
	os.Symlink(outside, filepath.Join(home, "escape"))

	result := callRead(t, home, `{"path":"escape/secret.txt"}`)
	if !strings.Contains(result, "error") {
		t.Fatalf("expected error for symlink escape, got %s", result)
	}
}

func TestReadFileEmpty(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "empty.txt"), []byte(""), 0o644)

	result := callRead(t, dir, `{"path":"empty.txt"}`)
	if !strings.Contains(result, "empty or no lines in range") {
		t.Fatalf("expected empty message, got %s", result)
	}
}
