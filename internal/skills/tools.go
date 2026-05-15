package skills

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/llm"
)

var loadSkillDef = llm.ToolDef{
	Name:        "load_skill",
	Description: "Load a skill's full documentation into context. Use action 'list' to see available skills, 'load' to read one.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"list", "load"},
				"description": "list: show available skills. load: return the full skill document.",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Skill name (for load). Empty for list.",
			},
		},
		"required":             []string{"action", "name"},
		"additionalProperties": false,
	},
}

type SkillReflexDef struct {
	Name    string
	Command string   // empty for schedule-only reflexes
	Every   string   // natural language schedule (e.g. "every day at 11:30pm")
	Tools   []string // inherited from the parent skill's tools list
}

type SkillSummary struct {
	Name     string           `json:"name"`
	Summary  string           `json:"summary"`
	Tools    []string         `json:"tools"`
	Preload  bool             `json:"preload"`
	Reflexes []SkillReflexDef `json:"-"`
}

func BuildTools(cfg *config.Config) []llm.Tool {
	return []llm.Tool{
		{
			Def:     loadSkillDef,
			Handler: loadSkillHandler(cfg),
		},
	}
}

func loadSkillHandler(cfg *config.Config) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			Action string `json:"action"`
			Name   string `json:"name"`
		}

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		srcs := Sources(cfg.Home)

		switch args.Action {
		case "list":
			return handleList(srcs)
		case "load":
			return handleLoad(srcs, args.Name)
		default:
			return llm.ToolErrorf("unknown action %q", args.Action), nil
		}
	}
}

func handleList(srcs []fs.FS) (string, error) {
	summaries, err := ListSkills(srcs...)
	if err != nil {
		return llm.ToolError(err), nil
	}

	data, err := json.Marshal(map[string]any{"skills": summaries})
	if err != nil {
		return llm.ToolError(err), nil
	}

	return string(data), nil
}

func handleLoad(srcs []fs.FS, name string) (string, error) {
	if name == "" {
		return `{"error":"empty name"}`, nil
	}

	for i := len(srcs) - 1; i >= 0; i-- {
		data, err := fs.ReadFile(srcs[i], path.Join(name, "SKILL.md"))
		if err != nil {
			continue
		}
		return string(data), nil
	}

	return llm.ToolErrorf("skill %q not found", name), nil
}

// walkSkillSources iterates skill sources, parses frontmatter from each
// SKILL.md, and calls fn for each skill found. Later sources override
// earlier ones when skills share a name. A missing source (e.g. workspace
// dir not yet created) is silently skipped.
func walkSkillSources(srcs []fs.FS, fn func(s SkillSummary, data []byte)) error {
	for _, src := range srcs {
		entries, err := fs.ReadDir(src, ".")
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return fmt.Errorf("read skills root: %w", err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			data, err := fs.ReadFile(src, path.Join(entry.Name(), "SKILL.md"))
			if err != nil {
				continue
			}

			s, err := parseFrontmatter(data)
			if err != nil {
				continue
			}

			fn(s, data)
		}
	}

	return nil
}

// later sources override earlier ones when skills share a name.
func ListSkills(srcs ...fs.FS) ([]SkillSummary, error) {
	seen := map[string]int{}
	var summaries []SkillSummary

	err := walkSkillSources(srcs, func(s SkillSummary, _ []byte) {
		if idx, ok := seen[s.Name]; ok {
			summaries[idx] = s
		} else {
			seen[s.Name] = len(summaries)
			summaries = append(summaries, s)
		}
	})

	return summaries, err
}

// parseFrontmatter extracts name, summary, and tools from YAML frontmatter.
// Handles simple scalar and flow/block sequence formats without a YAML dependency.
func parseFrontmatter(data []byte) (SkillSummary, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))

	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return SkillSummary{}, fmt.Errorf("no frontmatter")
	}

	var s SkillSummary
	var descLines []string
	inDesc := false
	inTools := false
	inReflex := false
	inReflexItem := false
	var curReflexName, curReflexCmd, curReflexEvery string

	finishDesc := func() {
		if inDesc && len(descLines) > 0 {
			s.Summary = strings.Join(descLines, " ")
		}
	}

	flushReflexItem := func() {
		if curReflexName == "" || curReflexEvery == "" {
			curReflexName, curReflexCmd, curReflexEvery = "", "", ""
			return
		}
		s.Reflexes = append(s.Reflexes, SkillReflexDef{
			Name:    curReflexName,
			Command: curReflexCmd,
			Every:   curReflexEvery,
		})
		curReflexName, curReflexCmd, curReflexEvery = "", "", ""
	}

	resetBlock := func() {
		finishDesc()
		if inReflexItem {
			flushReflexItem()
		}
		inDesc = false
		inTools = false
		inReflex = false
		inReflexItem = false
	}

	for scanner.Scan() {
		line := scanner.Text()

		if strings.TrimSpace(line) == "---" {
			break
		}

		if strings.HasPrefix(line, "name:") {
			resetBlock()
			s.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
			continue
		}

		if strings.HasPrefix(line, "summary:") {
			resetBlock()
			rest := strings.TrimSpace(strings.TrimPrefix(line, "summary:"))
			if rest == ">" || rest == "|" {
				inDesc = true
				descLines = nil
			} else {
				s.Summary = rest
			}
			continue
		}

		if strings.HasPrefix(line, "preload:") {
			resetBlock()
			val := strings.TrimSpace(strings.TrimPrefix(line, "preload:"))
			s.Preload = val == "true"
			continue
		}

		if strings.HasPrefix(line, "tools:") {
			resetBlock()
			rest := strings.TrimSpace(strings.TrimPrefix(line, "tools:"))
			if rest != "" {
				s.Tools = parseFlowSequence(rest)
			} else {
				inTools = true
			}
			continue
		}

		if strings.HasPrefix(line, "reflex:") {
			resetBlock()
			inReflex = true
			continue
		}

		if inDesc {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				inDesc = false
				s.Summary = strings.Join(descLines, " ")
			} else {
				descLines = append(descLines, trimmed)
				continue
			}
		}

		if inTools {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "- ") {
				s.Tools = append(s.Tools, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
				continue
			}
			inTools = false
		}

		if inReflex {
			trimmed := strings.TrimSpace(line)

			if strings.HasPrefix(trimmed, "- name:") {
				flushReflexItem()
				inReflexItem = true
				curReflexName = strings.TrimSpace(strings.TrimPrefix(trimmed, "- name:"))
				continue
			}

			if inReflexItem {
				if strings.HasPrefix(trimmed, "command:") {
					curReflexCmd = strings.TrimSpace(strings.TrimPrefix(trimmed, "command:"))
					continue
				}
				if strings.HasPrefix(trimmed, "every:") {
					raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "every:"))
					curReflexEvery = strings.Trim(raw, "\"'")
					continue
				}
			}

			if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				flushReflexItem()
				inReflex = false
				inReflexItem = false
			}
		}
	}

	finishDesc()
	if inReflexItem {
		flushReflexItem()
	}

	return s, nil
}

func ListReflexes(srcs ...fs.FS) (map[string]SkillReflexDef, error) {
	result := map[string]SkillReflexDef{}

	err := walkSkillSources(srcs, func(s SkillSummary, _ []byte) {
		for _, r := range s.Reflexes {
			key := s.Name + "/" + r.Name
			result[key] = SkillReflexDef{
				Name:    r.Name,
				Command: r.Command,
				Every:   r.Every,
				Tools:   s.Tools,
			}
		}
	})

	return result, err
}

type PreloadedSkill struct {
	Name    string
	Tools   []string
	Content string
}

// later sources override earlier ones by name.
func PreloadedSkills(srcs ...fs.FS) ([]PreloadedSkill, error) {
	seen := map[string]int{}
	var result []PreloadedSkill

	err := walkSkillSources(srcs, func(s SkillSummary, data []byte) {
		if !s.Preload {
			return
		}

		body := stripFrontmatter(string(data))
		if body == "" {
			return
		}

		body = stripInstallSection(body)

		ps := PreloadedSkill{Name: s.Name, Tools: s.Tools, Content: body}

		if idx, ok := seen[s.Name]; ok {
			result[idx] = ps
		} else {
			seen[s.Name] = len(result)
			result = append(result, ps)
		}
	})

	return result, err
}

// stripFrontmatter removes the YAML frontmatter block (--- ... ---) and
// returns the remaining body, trimmed of leading whitespace.
func stripFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---") {
		return content
	}

	// find closing ---
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return content
	}

	body := rest[idx+4:] // skip past \n---
	return strings.TrimLeft(body, "\n")
}

func stripInstallSection(content string) string {
	idx := strings.Index(content, "\n## Install")
	if idx < 0 {
		return content
	}

	rest := content[idx+1:]
	end := strings.Index(rest[len("## Install"):], "\n## ")
	if end >= 0 {
		return content[:idx] + "\n" + rest[len("## Install")+end+1:]
	}

	return strings.TrimRight(content[:idx], "\n") + "\n"
}

// parseFlowSequence parses [a, b, c] YAML flow sequences.
func parseFlowSequence(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")

	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v != "" {
			result = append(result, v)
		}
	}

	return result
}
