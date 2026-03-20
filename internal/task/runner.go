package task

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/llm"
	"github.com/kciuffolo/nik/internal/skills"
)

const runnerTimeout = 20 * time.Minute

type taskPromptData struct {
	Now        string
	Home       string
	Tmp        string
	TokenTraps string
	ToolDocs   string
	Skills     string
	Plan       string
}

type Runner struct {
	cfg       *config.Config
	llm       llm.Completer
	criticLLM llm.Completer
	svc       *Service
	tools     []llm.Tool
	cancels   sync.Map
	wg        sync.WaitGroup
}

func NewRunner(cfg *config.Config, llmClient llm.Completer, svc *Service, tools []llm.Tool) *Runner {
	return &Runner{
		cfg:   cfg,
		llm:   llmClient,
		svc:   svc,
		tools: tools,
	}
}

func (r *Runner) SetCriticLLM(c llm.Completer) {
	r.criticLLM = c
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

	data := taskPromptData{
		Now:        now,
		Home:       r.cfg.Home,
		Tmp:        r.cfg.TmpPath(),
		TokenTraps: scanTokenTraps(r.cfg.Home),
		ToolDocs:   buildToolDocs(tools),
		Skills:     buildSkillDocs(r.cfg, tools),
		Plan:       t.Plan,
	}

	var buf strings.Builder
	err = tmpl.Execute(&buf, data)
	if err != nil {
		slog.Warn("execute task prompt template", "pkg", "task", "error", err)
		return fmt.Sprintf("Goal: %s\n\n%s", t.Goal, t.Plan)
	}

	return buf.String()
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

func (r *Runner) Wait() { r.wg.Wait() }

func (r *Runner) Run(ctx context.Context, t db.Task) {
	defer r.wg.Done()
	ctx, cancel := context.WithTimeout(ctx, runnerTimeout)
	r.cancels.Store(t.ID, cancel)
	defer r.cancels.Delete(t.ID)
	defer cancel()

	ctx = context.WithValue(ctx, "meta", map[string]string{
		"conversation_id": t.ConversationID,
		"task_id":         t.ID,
		"sources":         `["task"]`,
	})

	// Workers inherit the spawning conversation's privilege level. Without
	// this check, any allowed conversation could spawn a task with shell and
	// db_query access, bypassing the brain's privilege controls.
	tools := r.tools
	if !r.cfg.IsPrivileged(t.ConversationID) {
		tools = filterUnprivileged(tools)
	}

	reportTool := BuildReportTool(r.svc, t.ID)
	allTools := append(tools, reportTool)
	defs, exec := llm.SplitTools(allTools)

	instructions := r.renderPrompt(t, defs)
	nudge := r.buildNudge(t)
	actID, ch := r.llm.Complete(ctx, instructions, llm.StaticInput(""), defs, exec, llm.WithOnIdle(nudge))

	err := r.svc.Start(ctx, t.ID, actID)
	if err != nil {
		slog.Error("start task", "pkg", "task", "task_id", t.ID, "error", err)
		return
	}

	slog.Info("task started", "pkg", "task", "task_id", t.ID, "goal", t.Goal, "thinking", t.Thinking)

	result := <-ch

	if ctx.Err() != nil {
		r.svc.UpdateStatus(context.Background(), t.ID, "cancelled")
		slog.Info("task cancelled", "pkg", "task", "task_id", t.ID, "reason", ctx.Err())
		return
	}

	if result.Err != nil {
		r.svc.UpdateStatus(ctx, t.ID, "failed")
		slog.Info("task failed", "pkg", "task", "task_id", t.ID, "error", result.Err)
		return
	}

	finalStatus := "failed"
	reportStatus, err := r.svc.LastReportStatus(ctx, t.ID)
	if err == nil && (reportStatus == "completed" || reportStatus == "failed") {
		finalStatus = reportStatus
	}

	r.svc.UpdateStatus(ctx, t.ID, finalStatus)
	slog.Info("task "+finalStatus, "pkg", "task", "task_id", t.ID, "goal", t.Goal)

	t.Status = finalStatus
	t.ActivationID = actID

	current, err := r.svc.Get(ctx, t.ID)
	if err == nil {
		t = current
	} else {
		slog.Warn("get task for critic", "pkg", "task", "task_id", t.ID, "error", err)
	}

	r.RunCritic(context.Background(), t)
}

func (r *Runner) Cancel(taskID string) bool {
	v, ok := r.cancels.LoadAndDelete(taskID)
	if !ok {
		return false
	}

	v.(context.CancelFunc)()
	return true
}

func filterUnprivileged(tools []llm.Tool) []llm.Tool {
	var out []llm.Tool
	for _, t := range tools {
		if !t.Privileged {
			out = append(out, t)
		}
	}
	return out
}

func (r *Runner) buildNudge(t db.Task) func(string) string {
	return func(_ string) string {
		reportStatus, err := r.svc.LastReportStatus(context.Background(), t.ID)
		if err == nil && (reportStatus == "completed" || reportStatus == "failed") {
			return ""
		}

		nudge, err := readFromPromptsRoot(r.cfg.PromptsPath(), "task-01-nudge.md")
		if err != nil {
			return ""
		}

		return string(nudge)
	}
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
