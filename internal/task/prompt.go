package task

import (
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/llm"
	"github.com/kciuffolo/nik/internal/skills"
)

type taskPromptData struct {
	Now        string
	ShellEnv   string
	TableList  string
	TokenTraps string
	ToolDocs   string
	Skills     string
	Plan       string
	Timeout    string
	MaxRounds  int
}

func (r *Runner) renderPrompt(t db.Task, tools []llm.ToolDef) string {
	raw, err := readFromPromptsRoot(r.cfg.PromptsPath(), "task-00.md")
	if err != nil {
		slog.Warn("load task prompt template", "pkg", "task", "error", err)
		return fmt.Sprintf("Goal: %s\n\n%s", t.Goal, t.Plan)
	}

	tmpl, err := template.New("task").Parse(string(raw))
	if err != nil {
		slog.Warn("parse task prompt template", "pkg", "task", "error", err)
		return fmt.Sprintf("Goal: %s\n\n%s", t.Goal, t.Plan)
	}

	loc := r.cfg.TZ()
	now := time.Now().In(loc).Format("Monday, January 2, 2006 3:04 PM")

	var shellEnv string
	if r.cfg.Shell.DockerImage != "" {
		shellEnv = "Docker container (Debian bookworm)"
	}

	timeout := r.cfg.Task.TimeoutOrDefault()
	maxRounds := r.cfg.Task.MaxRoundsOrDefault()

	data := taskPromptData{
		Now:        now,
		ShellEnv:   shellEnv,
		TableList:  db.TableList(),
		TokenTraps: scanTokenTraps(r.cfg.Home),
		ToolDocs:   buildToolDocs(tools),
		Skills:     buildSkillDocs(r.cfg, tools),
		Plan:       t.Plan,
		Timeout:    timeout.String(),
		MaxRounds:  maxRounds,
	}

	var buf strings.Builder
	err = tmpl.Execute(&buf, data)
	if err != nil {
		slog.Warn("execute task prompt template", "pkg", "task", "error", err)
		return fmt.Sprintf("Goal: %s\n\n%s", t.Goal, t.Plan)
	}

	return buf.String()
}

func (r *Runner) loadNudge() string {
	data, err := readFromPromptsRoot(r.cfg.PromptsPath(), "task-01-nudge.md")
	if err != nil {
		return ""
	}
	return string(data)
}

func readFromPromptsRoot(promptsDir, name string) ([]byte, error) {
	root, err := os.OpenRoot(promptsDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return io.ReadAll(f)
}

func buildToolDocs(tools []llm.ToolDef) string {
	var b strings.Builder
	for _, t := range tools {
		fmt.Fprintf(&b, "- **%s**: %s\n", t.Name, t.Description)
	}
	return b.String()
}

func buildSkillDocs(cfg *config.Config, toolDefs []llm.ToolDef) string {
	dirs := []string{cfg.SkillsPath(), cfg.WorkspaceSkillsPath()}

	available := make(map[string]bool, len(toolDefs))
	for _, td := range toolDefs {
		available[td.Name] = true
	}

	preloaded, err := skills.PreloadedSkills(dirs...)
	if err != nil {
		slog.Warn("load task preloaded skills", "pkg", "task", "error", err)
	}

	var b strings.Builder
	for _, s := range preloaded {
		if len(s.Tools) > 0 && !anyAvailable(s.Tools, available) {
			continue
		}
		fmt.Fprintf(&b, "### %s\n\n%s\n\n", s.Name, s.Content)
	}

	summaries, err := skills.ListSkills(dirs...)
	if err != nil {
		slog.Warn("load task skill index", "pkg", "task", "error", err)
	}

	var extras []string
	for _, s := range summaries {
		if s.Preload {
			continue
		}
		extras = append(extras, fmt.Sprintf("- **%s**: %s (tools: %s)", s.Name, s.Summary, strings.Join(s.Tools, ", ")))
	}

	if len(extras) > 0 {
		b.WriteString("Use `load_skill` to read full instructions before using these:\n\n")
		for _, line := range extras {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}

	return b.String()
}

func anyAvailable(tools []string, available map[string]bool) bool {
	for _, t := range tools {
		if available[t] {
			return true
		}
	}
	return false
}

func scanTokenTraps(home string) string {
	const (
		sizeThreshold  = 50 * 1024
		countThreshold = 30
	)

	skipDirs := map[string]bool{
		".git":     true,
		".cursor":  true,
		".gocache": true,
		".tmp":     true,
		"vendor":   true,
		"media":    true,
		"backups":  true,
		"tmp":      true,
	}

	datePattern := regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)

	type datedDir struct {
		Path   string
		Count  int
		Latest string
	}

	type largeFile struct {
		Path string
		Size int64
	}

	type denseDir struct {
		Path  string
		Count int
	}

	var (
		dateds []datedDir
		large  []largeFile
		denses []denseDir
	)

	_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path != home && d.IsDir() {
			name := d.Name()
			if skipDirs[name] {
				return filepath.SkipDir
			}
		}

		if !d.IsDir() {
			return nil
		}

		entries, err := os.ReadDir(path)
		if err != nil {
			return nil
		}

		var (
			fileCount int
			latest    string
			isDated   bool
		)

		for _, entry := range entries {
			name := entry.Name()
			isDir := entry.IsDir()
			if !isDir {
				fileCount++
			}

			matchesDate := datePattern.MatchString(name) || strings.HasPrefix(name, "latest.")
			if matchesDate {
				isDated = true
				if name > latest {
					latest = name
				}
			}

			if isDir {
				continue
			}

			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.Size() > sizeThreshold {
				rel, err := filepath.Rel(home, filepath.Join(path, name))
				if err != nil {
					continue
				}
				large = append(large, largeFile{
					Path: rel,
					Size: info.Size(),
				})
			}
		}

		rel, err := filepath.Rel(home, path)
		if err != nil {
			return nil
		}
		if rel == "." {
			rel = ""
		}
		rel = rel + string(filepath.Separator)

		if isDated {
			dateds = append(dateds, datedDir{
				Path:   rel,
				Count:  fileCount,
				Latest: latest,
			})
			return nil
		}

		if fileCount > countThreshold {
			denses = append(denses, denseDir{
				Path:  rel,
				Count: fileCount,
			})
		}

		return nil
	})

	sort.Slice(dateds, func(i, j int) bool {
		return dateds[i].Count > dateds[j].Count
	})
	sort.Slice(large, func(i, j int) bool {
		return large[i].Size > large[j].Size
	})
	sort.Slice(denses, func(i, j int) bool {
		return denses[i].Count > denses[j].Count
	})

	if len(dateds) == 0 && len(large) == 0 && len(denses) == 0 {
		return ""
	}

	var b strings.Builder
	if len(dateds) > 0 {
		b.WriteString("Dated directories — read latest or most recent, never list:\n")
		for _, d := range dateds {
			fmt.Fprintf(&b, "  %-18s %d entries, latest: %s\n", d.Path, d.Count, d.Latest)
		}
	}

	if len(large) > 0 {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("Large files — do not read:\n")
		for _, f := range large {
			fmt.Fprintf(&b, "  %-50s %d KB\n", f.Path, (f.Size+1023)/1024)
		}
	}

	if len(denses) > 0 {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("Dense directories — avoid listing contents:\n")
		for _, d := range denses {
			fmt.Fprintf(&b, "  %-18s %d files\n", d.Path, d.Count)
		}
	}

	return strings.TrimSpace(b.String())
}
