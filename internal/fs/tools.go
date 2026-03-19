package fs

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kciuffolo/nik/internal/llm"
)

const (
	defaultReadLimit = 2000
	maxLineLength    = 2000
)

var readFileDef = llm.ToolDef{
	Name: "read_file",
	Description: "Read a file from the workspace. Returns numbered lines (1-indexed). " +
		"By default returns up to 2000 lines from the start. " +
		"Use offset to start from a later line, limit to cap the number of lines returned. " +
		"Lines longer than 2000 characters are truncated. " +
		"Call this tool in parallel when reading multiple files.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "File path relative to workspace.",
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "1-based line number to start reading from. Defaults to 1.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of lines to return. Defaults to 2000.",
			},
		},
		"required":             []string{"path"},
		"additionalProperties": false,
	},
}

type readFileArgs struct {
	Path   string `json:"path"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

var writeFileDef = llm.ToolDef{
	Name:        "write_file",
	Description: "Write or append content to a file. Use this instead of shell heredocs for any file writes -- it handles quoting and encoding automatically.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"write", "append"},
				"description": "write: overwrite file. append: add to end.",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "File path relative to workspace.",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Content to write.",
			},
		},
		"required":             []string{"action", "path", "content"},
		"additionalProperties": false,
	},
}

type writeFileArgs struct {
	Action  string `json:"action"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

func BuildTools(home string) []llm.Tool {
	return []llm.Tool{
		{
			Def:        readFileDef,
			Handler:    readFileHandler(home),
			Privileged: true,
		},
		{
			Def:        writeFileDef,
			Handler:    writeFileHandler(home),
			Privileged: true,
		},
	}
}

func readFileHandler(home string) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args readFileArgs

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		if args.Path == "" {
			return llm.ToolErrorf("empty path"), nil
		}

		if home == "" {
			return llm.ToolErrorf("read_file requires a home directory"), nil
		}

		if filepath.IsAbs(args.Path) {
			return llm.ToolErrorf("absolute paths not allowed, use a relative path"), nil
		}

		root, err := os.OpenRoot(home)
		if err != nil {
			return llm.ToolError(err), nil
		}
		defer root.Close()

		f, err := root.Open(args.Path)
		if err != nil {
			return llm.ToolError(err), nil
		}
		defer f.Close()

		offset := args.Offset
		if offset < 1 {
			offset = 1
		}

		limit := args.Limit
		if limit <= 0 {
			limit = defaultReadLimit
		}

		var b strings.Builder
		scanner := bufio.NewScanner(f)
		lineNum := 0
		emitted := 0
		truncated := false

		for scanner.Scan() {
			lineNum++

			if lineNum < offset {
				continue
			}

			if emitted >= limit {
				truncated = true
				break
			}

			line := scanner.Text()
			if len(line) > maxLineLength {
				line = line[:maxLineLength]
			}

			fmt.Fprintf(&b, "%d: %s\n", lineNum, line)
			emitted++
		}

		// count remaining lines for total
		if truncated {
			for scanner.Scan() {
				lineNum++
			}
		}

		if scanErr := scanner.Err(); scanErr != nil {
			return llm.ToolError(scanErr), nil
		}

		if emitted == 0 {
			return llm.ToolResult(map[string]any{
				"path":        args.Path,
				"total_lines": lineNum,
				"content":     "(empty or no lines in range)",
			}), nil
		}

		result := map[string]any{
			"path":        args.Path,
			"total_lines": lineNum,
			"content":     b.String(),
		}

		if truncated {
			result["truncated"] = true
		}

		return llm.ToolResult(result), nil
	}
}

func writeFileHandler(home string) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args writeFileArgs

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		if args.Path == "" {
			return llm.ToolErrorf("empty path"), nil
		}

		if home == "" {
			return llm.ToolErrorf("write_file requires a home directory"), nil
		}

		if filepath.IsAbs(args.Path) {
			return llm.ToolErrorf("absolute paths not allowed, use a relative path"), nil
		}

		root, err := os.OpenRoot(home)
		if err != nil {
			return llm.ToolError(err), nil
		}
		defer root.Close()

		dir := filepath.Dir(args.Path)
		if dir != "." {
			err = root.MkdirAll(dir, 0o755)
			if err != nil {
				return llm.ToolError(err), nil
			}
		}

		switch args.Action {
		case "write":
			f, openErr := root.OpenFile(args.Path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if openErr != nil {
				return llm.ToolError(openErr), nil
			}
			_, err = f.WriteString(args.Content)
			f.Close()

		case "append":
			f, openErr := root.OpenFile(args.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
			if openErr != nil {
				return llm.ToolError(openErr), nil
			}
			_, err = f.WriteString(args.Content)
			f.Close()

		default:
			return llm.ToolErrorf("unknown action %q", args.Action), nil
		}

		if err != nil {
			return llm.ToolError(err), nil
		}

		info, statErr := root.Stat(args.Path)
		size := int64(0)
		if statErr == nil {
			size = info.Size()
		}

		return llm.ToolResult(map[string]any{
			"ok":    true,
			"path":  filepath.Join(home, args.Path),
			"bytes": size,
		}), nil
	}
}
