package skills

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
			"reason": map[string]any{
				"type":        "string",
				"description": "Why you are loading this skill -- what problem you expect it to solve. Empty for list.",
			},
		},
		"required":             []string{"action", "name", "reason"},
		"additionalProperties": false,
	},
}

type SkillSummary struct {
	Name    string   `json:"name"`
	Summary string   `json:"summary"`
	Tools   []string `json:"tools"`
	Preload bool     `json:"preload"`
}

func BuildTools(cfg *config.Config, availableTools func() []string) []llm.Tool {
	return []llm.Tool{
		{
			Def:     loadSkillDef,
			Handler: loadSkillHandler(cfg, availableTools),
		},
	}
}

func loadSkillHandler(cfg *config.Config, availableTools func() []string) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			Action string `json:"action"`
			Name   string `json:"name"`
		}

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		dirs := []string{cfg.SkillsPath(), cfg.WorkspaceSkillsPath()}

		switch args.Action {
		case "list":
			return handleList(dirs)
		case "load":
			return handleLoad(dirs, args.Name, availableTools)
		default:
			return llm.ToolErrorf("unknown action %q", args.Action), nil
		}
	}
}

func handleList(dirs []string) (string, error) {
	summaries, err := ListSkills(dirs...)
	if err != nil {
		return llm.ToolError(err), nil
	}

	data, err := json.Marshal(map[string]any{"skills": summaries})
	if err != nil {
		return llm.ToolError(err), nil
	}

	return string(data), nil
}

func handleLoad(dirs []string, name string, availableTools func() []string) (string, error) {
	if name == "" {
		return `{"error":"empty name"}`, nil
	}

	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return llm.ToolErrorf("invalid skill name %q", name), nil
	}

	for i := len(dirs) - 1; i >= 0; i-- {
		path := filepath.Join(dirs[i], name, "SKILL.md")

		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		content := string(data)
		warning := checkToolPrereqs(path, availableTools)
		if warning != "" {
			content = warning + "\n" + content
		}

		return content, nil
	}

	return llm.ToolErrorf("skill %q not found", name), nil
}

func checkToolPrereqs(path string, availableTools func() []string) string {
	if availableTools == nil {
		return ""
	}

	s, err := parseFrontmatter(path)
	if err != nil || len(s.Tools) == 0 {
		return ""
	}

	names := availableTools()
	have := make(map[string]bool, len(names))
	for _, n := range names {
		have[n] = true
	}

	var missing []string
	for _, t := range s.Tools {
		if !have[t] {
			missing = append(missing, t)
		}
	}

	if len(missing) == 0 {
		return ""
	}

	return fmt.Sprintf("warning: skill %s declares tools %v which are not available in this activation", s.Name, missing)
}

// walkSkillDirs iterates skill directories, parses frontmatter from each
// SKILL.md, and calls fn for each unique skill. Later directories override
// earlier ones when skills share a name.
func walkSkillDirs(dirs []string, fn func(path string, s SkillSummary)) error {
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("read skills dir %s: %w", dir, err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			path := filepath.Join(dir, entry.Name(), "SKILL.md")

			s, err := parseFrontmatter(path)
			if err != nil {
				continue
			}

			fn(path, s)
		}
	}

	return nil
}

// later directories override earlier ones when skills share a name.
func ListSkills(dirs ...string) ([]SkillSummary, error) {
	seen := map[string]int{}
	var summaries []SkillSummary

	err := walkSkillDirs(dirs, func(_ string, s SkillSummary) {
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
func parseFrontmatter(path string) (SkillSummary, error) {
	f, err := os.Open(path)
	if err != nil {
		return SkillSummary{}, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return SkillSummary{}, fmt.Errorf("no frontmatter in %s", path)
	}

	var s SkillSummary
	var descLines []string
	inDesc := false
	inTools := false

	for scanner.Scan() {
		line := scanner.Text()

		if strings.TrimSpace(line) == "---" {
			break
		}

		if strings.HasPrefix(line, "name:") {
			inDesc = false
			inTools = false
			s.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
			continue
		}

		if strings.HasPrefix(line, "summary:") {
			inTools = false
			rest := strings.TrimSpace(strings.TrimPrefix(line, "summary:"))
			if rest == ">" || rest == "|" {
				inDesc = true
				descLines = nil
			} else {
				inDesc = false
				s.Summary = rest
			}
			continue
		}

		if strings.HasPrefix(line, "preload:") {
			if inDesc && len(descLines) > 0 {
				s.Summary = strings.Join(descLines, " ")
			}
			inDesc = false
			inTools = false
			val := strings.TrimSpace(strings.TrimPrefix(line, "preload:"))
			s.Preload = val == "true"
			continue
		}

		if strings.HasPrefix(line, "install:") {
			if inDesc && len(descLines) > 0 {
				s.Summary = strings.Join(descLines, " ")
			}
			inDesc = false
			inTools = false
			continue
		}

		if strings.HasPrefix(line, "tools:") {
			if inDesc && len(descLines) > 0 {
				s.Summary = strings.Join(descLines, " ")
			}
			inDesc = false
			rest := strings.TrimSpace(strings.TrimPrefix(line, "tools:"))
			if rest != "" {
				s.Tools = parseFlowSequence(rest)
				inTools = false
			} else {
				inTools = true
			}
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
	}

	if inDesc && len(descLines) > 0 {
		s.Summary = strings.Join(descLines, " ")
	}

	return s, nil
}

type PreloadedSkill struct {
	Name    string
	Content string
}

// later directories override earlier ones by name.
func PreloadedSkills(dirs ...string) ([]PreloadedSkill, error) {
	seen := map[string]int{}
	var result []PreloadedSkill

	err := walkSkillDirs(dirs, func(path string, s SkillSummary) {
		if !s.Preload {
			return
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return
		}

		body := stripFrontmatter(string(data))
		if body == "" {
			return
		}

		ps := PreloadedSkill{Name: s.Name, Content: body}

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
