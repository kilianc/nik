package task

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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
	Now      string
	Home     string
	Tmp      string
	ToolDocs string
	Skills   string
	Plan     string
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
	tmplPath := filepath.Join(r.cfg.PromptsPath(), "task-00.md")

	raw, err := os.ReadFile(tmplPath)
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
		Now:      now,
		Home:     r.cfg.Home,
		Tmp:      r.cfg.TmpPath(),
		ToolDocs: buildToolDocs(tools),
		Skills:   buildSkillDocs(r.cfg),
		Plan:     t.Plan,
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

func buildSkillDocs(cfg *config.Config) string {
	dirs := []string{cfg.SkillsPath(), cfg.WorkspaceSkillsPath()}

	preloaded, err := skills.PreloadedSkills(dirs...)
	if err != nil {
		slog.Warn("load task preloaded skills", "pkg", "task", "error", err)
	}

	var b strings.Builder
	for _, s := range preloaded {
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

	reportTool := BuildReportTool(r.svc, t.ID)
	allTools := append(r.tools, reportTool)
	defs, exec := llm.SplitTools(allTools)

	instructions := r.renderPrompt(t, defs)
	actID, ch := r.llm.Complete(ctx, instructions, llm.StaticInput(""), defs, exec)

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

	finalStatus := "completed"
	reportStatus, err := r.svc.LastReportStatus(ctx, t.ID)
	if err == nil && (reportStatus == "completed" || reportStatus == "failed") {
		finalStatus = reportStatus
	}

	r.svc.UpdateStatus(ctx, t.ID, finalStatus)
	slog.Info("task "+finalStatus, "pkg", "task", "task_id", t.ID, "goal", t.Goal)

	t.Status = finalStatus
	t.ActivationID = actID
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
