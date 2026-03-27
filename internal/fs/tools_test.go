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

func TestWriteFile(t *testing.T) {
	tests := []struct {
		name       string
		action     string
		path       string
		content    string
		preContent string
		want       string
	}{
		{"creates file", "write", "out.txt", "hello world", "", "hello world"},
		{"overwrites existing", "write", "out.txt", "new", "old", "new"},
		{"appends to existing", "append", "out.txt", " second", "first", "first second"},
		{"append creates if missing", "append", "new.txt", "first", "", "first"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.preContent != "" {
				os.WriteFile(filepath.Join(dir, tt.path), []byte(tt.preContent), 0o644)
			}

			args := fmt.Sprintf(`{"action":%q,"path":%q,"content":%q}`, tt.action, tt.path, tt.content)
			result := call(t, dir, args)
			if !strings.Contains(result, `"ok":true`) {
				t.Fatalf("expected ok, got %s", result)
			}

			data, err := os.ReadFile(filepath.Join(dir, tt.path))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if string(data) != tt.want {
				t.Errorf("content = %q, want %q", string(data), tt.want)
			}
		})
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

func TestWriteFilePathSecurity(t *testing.T) {
	t.Run("traversal blocked", func(t *testing.T) {
		dir := t.TempDir()
		result := call(t, dir, `{"action":"write","path":"../../etc/passwd","content":"bad"}`)
		if !strings.Contains(result, "error") {
			t.Fatalf("expected error for traversal, got %s", result)
		}
	})

	t.Run("absolute path blocked", func(t *testing.T) {
		dir := t.TempDir()
		result := call(t, dir, `{"action":"write","path":"/tmp/escape.txt","content":"bad"}`)
		if !strings.Contains(result, "absolute paths not allowed") {
			t.Fatalf("expected absolute path error, got %s", result)
		}
	})

	t.Run("symlink escape blocked", func(t *testing.T) {
		home := t.TempDir()
		outside := t.TempDir()
		os.Symlink(outside, filepath.Join(home, "escape"))

		result := call(t, home, `{"action":"write","path":"escape/pwned.txt","content":"bad"}`)
		if !strings.Contains(result, "error") {
			t.Fatalf("expected error for symlink escape, got %s", result)
		}
		if _, err := os.Stat(filepath.Join(outside, "pwned.txt")); err == nil {
			t.Fatal("file was written outside home via symlink")
		}
	})
}

func TestWriteFileValidation(t *testing.T) {
	tests := []struct {
		name        string
		home        string
		args        string
		wantContain string
	}{
		{"empty home", "", `{"action":"write","path":"file.txt","content":"x"}`, "requires a home directory"},
		{"empty path", "TMPDIR", `{"action":"write","path":"","content":"x"}`, "empty path"},
		{"unknown action", "TMPDIR", `{"action":"delete","path":"file.txt","content":""}`, "unknown action"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := tt.home
			if home == "TMPDIR" {
				home = t.TempDir()
			}
			result := call(t, home, tt.args)
			if !strings.Contains(result, tt.wantContain) {
				t.Fatalf("expected %q in result, got %s", tt.wantContain, result)
			}
		})
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

func TestReadFileOffsetAndLimit(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("a\nb\nc\nd\ne\n"), 0o644)

	tests := []struct {
		name           string
		args           string
		wantContain    []string
		wantNotContain []string
	}{
		{
			name:           "offset only",
			args:           `{"path":"f.txt","offset":3}`,
			wantContain:    []string{"3: c"},
			wantNotContain: []string{"1: a", "2: b"},
		},
		{
			name:           "limit only",
			args:           `{"path":"f.txt","limit":2}`,
			wantContain:    []string{"1: a", "2: b"},
			wantNotContain: []string{"3: c"},
		},
		{
			name:           "offset and limit",
			args:           `{"path":"f.txt","offset":2,"limit":2}`,
			wantContain:    []string{"2: b", "3: c"},
			wantNotContain: []string{"1: a", "4: d"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callRead(t, dir, tt.args)
			for _, s := range tt.wantContain {
				if !strings.Contains(result, s) {
					t.Errorf("expected %q in result, got %s", s, result)
				}
			}
			for _, s := range tt.wantNotContain {
				if strings.Contains(result, s) {
					t.Errorf("expected %q not in result, got %s", s, result)
				}
			}
		})
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

func TestReadFilePathSecurity(t *testing.T) {
	t.Run("traversal blocked", func(t *testing.T) {
		dir := t.TempDir()
		result := callRead(t, dir, `{"path":"../../etc/passwd"}`)
		if !strings.Contains(result, "error") {
			t.Fatalf("expected error for traversal, got %s", result)
		}
	})

	t.Run("absolute path blocked", func(t *testing.T) {
		dir := t.TempDir()
		result := callRead(t, dir, `{"path":"/etc/passwd"}`)
		if !strings.Contains(result, "absolute paths not allowed") {
			t.Fatalf("expected absolute path error, got %s", result)
		}
	})

	t.Run("symlink blocked", func(t *testing.T) {
		home := t.TempDir()
		outside := t.TempDir()
		os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("nope"), 0o644)
		os.Symlink(outside, filepath.Join(home, "escape"))

		result := callRead(t, home, `{"path":"escape/secret.txt"}`)
		if !strings.Contains(result, "error") {
			t.Fatalf("expected error for symlink escape, got %s", result)
		}
	})
}

func TestReadFileEmpty(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "empty.txt"), []byte(""), 0o644)

	result := callRead(t, dir, `{"path":"empty.txt"}`)
	if !strings.Contains(result, "empty or no lines in range") {
		t.Fatalf("expected empty message, got %s", result)
	}
}
