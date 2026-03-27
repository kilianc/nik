package workbench

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/kciuffolo/nik/internal/db"
)

type diffLine struct {
	Kind byte // ' ', '-', '+'
	Text string
}

type hunk struct {
	OldStart int
	Lines    []diffLine
}

type filePatch struct {
	Path  string
	Hunks []hunk
}

func ParseDiff(text string) ([]filePatch, error) {
	if text == "" {
		return nil, nil
	}

	var patches []filePatch
	lines := strings.Split(text, "\n")
	i := 0

	for i < len(lines) {
		if !strings.HasPrefix(lines[i], "--- a/") {
			i++
			continue
		}
		i++

		if i >= len(lines) || !strings.HasPrefix(lines[i], "+++ b/") {
			return nil, fmt.Errorf("expected +++ header at line %d", i+1)
		}
		path := strings.TrimPrefix(lines[i], "+++ b/")
		i++

		fp := filePatch{Path: path}

		for i < len(lines) && strings.HasPrefix(lines[i], "@@") {
			h, next, err := parseHunk(lines, i)
			if err != nil {
				return nil, err
			}
			fp.Hunks = append(fp.Hunks, h)
			i = next
		}

		if len(fp.Hunks) == 0 {
			return nil, fmt.Errorf("no hunks for %s", path)
		}
		patches = append(patches, fp)
	}

	return patches, nil
}

func parseHunk(lines []string, start int) (hunk, int, error) {
	header := lines[start]
	s := strings.TrimPrefix(header, "@@ -")
	idx := strings.Index(s, " ")
	if idx < 0 {
		return hunk{}, 0, fmt.Errorf("parse hunk header %q", header)
	}
	nums := strings.Split(s[:idx], ",")

	oldStart, err := strconv.Atoi(nums[0])
	if err != nil {
		return hunk{}, 0, fmt.Errorf("parse hunk start %q: %w", header, err)
	}

	h := hunk{OldStart: oldStart}
	i := start + 1

	for i < len(lines) {
		line := lines[i]

		if len(line) == 0 {
			j := i + 1
			for j < len(lines) && lines[j] == "" {
				j++
			}
			if j >= len(lines) || strings.HasPrefix(lines[j], "---") || strings.HasPrefix(lines[j], "@@") {
				break
			}
			h.Lines = append(h.Lines, diffLine{Kind: ' ', Text: ""})
			i++
			continue
		}

		if strings.HasPrefix(line, "--- a/") {
			break
		}

		kind := line[0]
		if kind != ' ' && kind != '-' && kind != '+' {
			break
		}
		h.Lines = append(h.Lines, diffLine{Kind: kind, Text: line[1:]})
		i++
	}

	return h, i, nil
}

func ApplyPatches(run *db.ExperimentVariantRun) error {
	if run.Patches == "" {
		return nil
	}

	patches, err := ParseDiff(run.Patches)
	if err != nil {
		return fmt.Errorf("parse diff: %w", err)
	}

	for _, fp := range patches {
		err := applyFilePatch(run, fp)
		if err != nil {
			return fmt.Errorf("patch %s: %w", fp.Path, err)
		}
	}

	return nil
}

func applyFilePatch(run *db.ExperimentVariantRun, fp filePatch) error {
	switch {
	case fp.Path == "instructions":
		patched, err := applyHunks(run.Instructions, fp.Hunks)
		if err != nil {
			return err
		}
		run.Instructions = patched
		return nil

	case strings.HasPrefix(fp.Path, "messages/"):
		return applyMessagesPatch(run, fp)

	case strings.HasPrefix(fp.Path, "tools/"):
		return applyToolsPatch(run, fp)

	default:
		return fmt.Errorf("unknown surface %q", fp.Path)
	}
}

func applyToolsPatch(run *db.ExperimentVariantRun, fp filePatch) error {
	parts := strings.SplitN(strings.TrimPrefix(fp.Path, "tools/"), "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("expected tools/<name>/<field>, got %q", fp.Path)
	}
	name, field := parts[0], parts[1]

	var tools []map[string]any
	err := json.Unmarshal([]byte(run.ToolSchemas), &tools)
	if err != nil {
		return fmt.Errorf("parse tool schemas: %w", err)
	}

	idx := -1
	for i, t := range tools {
		if n, _ := t["Name"].(string); n == name {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("tool %q not found", name)
	}

	raw, ok := tools[idx][field]
	if !ok {
		return fmt.Errorf("field %q not found in tool %q", field, name)
	}

	text := anyToText(raw)
	patched, err := applyHunks(text, fp.Hunks)
	if err != nil {
		return err
	}

	tools[idx][field] = textToAny(raw, patched)

	data, err := json.Marshal(tools)
	if err != nil {
		return fmt.Errorf("marshal tool schemas: %w", err)
	}
	run.ToolSchemas = string(data)
	return nil
}

func applyMessagesPatch(run *db.ExperimentVariantRun, fp filePatch) error {
	parts := strings.SplitN(strings.TrimPrefix(fp.Path, "messages/"), "/", 2)

	index, err := strconv.Atoi(parts[0])
	if err != nil {
		return fmt.Errorf("invalid message index %q: %w", parts[0], err)
	}

	field := "content"
	if len(parts) > 1 {
		field = parts[1]
	}

	var msgs []map[string]string
	err = json.Unmarshal([]byte(run.Messages), &msgs)
	if err != nil {
		return fmt.Errorf("parse messages: %w", err)
	}

	if index < 0 || index >= len(msgs) {
		return fmt.Errorf("message index %d out of range (have %d)", index, len(msgs))
	}

	text, ok := msgs[index][field]
	if !ok {
		return fmt.Errorf("field %q not found in message %d", field, index)
	}

	patched, err := applyHunks(text, fp.Hunks)
	if err != nil {
		return err
	}

	msgs[index][field] = patched

	data, err := json.Marshal(msgs)
	if err != nil {
		return fmt.Errorf("marshal messages: %w", err)
	}

	run.Messages = string(data)
	return nil
}

func applyHunks(text string, hunks []hunk) (string, error) {
	lines := strings.Split(text, "\n")
	var result []string
	pos := 0

	for _, h := range hunks {
		start := h.OldStart - 1
		if start < pos {
			return "", fmt.Errorf("overlapping hunks at line %d", h.OldStart)
		}

		result = append(result, lines[pos:start]...)
		pos = start

		for _, dl := range h.Lines {
			switch dl.Kind {
			case ' ':
				if pos >= len(lines) || lines[pos] != dl.Text {
					got := "<EOF>"
					if pos < len(lines) {
						got = lines[pos]
					}
					return "", fmt.Errorf("context mismatch at line %d: expected %q, got %q", pos+1, dl.Text, got)
				}
				result = append(result, dl.Text)
				pos++

			case '-':
				if pos >= len(lines) || lines[pos] != dl.Text {
					got := "<EOF>"
					if pos < len(lines) {
						got = lines[pos]
					}
					return "", fmt.Errorf("delete mismatch at line %d: expected %q, got %q", pos+1, dl.Text, got)
				}
				pos++

			case '+':
				result = append(result, dl.Text)
			}
		}
	}

	result = append(result, lines[pos:]...)
	return strings.Join(result, "\n"), nil
}

func anyToText(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	data, _ := json.MarshalIndent(v, "", "  ")
	return string(data)
}

func textToAny(original any, text string) any {
	if _, ok := original.(string); ok {
		return text
	}
	var v any
	if json.Unmarshal([]byte(text), &v) == nil {
		return v
	}
	return text
}
