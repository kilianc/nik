package task

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/crew"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/llm"
	"github.com/kciuffolo/nik/internal/skills"
)

const runnerTimeout = 20 * time.Minute

type taskPromptData struct {
	Now      string
	Member   string
	ToolDocs string
	Skills   string
	Plan     string
}

type Runner struct {
	cfg     *config.Config
	llm     *llm.Client
	svc     *Service
	conn    *sql.DB
	tools   []llm.ToolDef
	exec    llm.ToolExecutor
	cancels sync.Map
}

func NewRunner(cfg *config.Config, llmClient *llm.Client, svc *Service, conn *sql.DB, tools []llm.ToolDef, exec llm.ToolExecutor) *Runner {
	return &Runner{
		cfg:   cfg,
		llm:   llmClient,
		svc:   svc,
		conn:  conn,
		tools: tools,
		exec:  exec,
	}
}

func (r *Runner) renderPrompt(t Task, tools []llm.ToolDef, member *crew.Member) string {
	tmplPath := filepath.Join(r.cfg.PromptsPath(), "task.md")

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

	var memberPrompt string
	if member != nil {
		memberPrompt = member.Prompt
	}

	data := taskPromptData{
		Now:      now,
		Member:   memberPrompt,
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

	for _, s := range summaries {
		if s.Preload {
			continue
		}
		fmt.Fprintf(&b, "- **%s**: %s (tools: %s)\n", s.Name, s.Summary, strings.Join(s.Tools, ", "))
	}

	return b.String()
}

func (r *Runner) Run(ctx context.Context, t Task, member *crew.Member) {
	ctx, cancel := context.WithTimeout(ctx, runnerTimeout)
	r.cancels.Store(t.ID, cancel)
	defer r.cancels.Delete(t.ID)
	defer cancel()

	actID := id.V7()
	now := time.Now().UTC()

	err := db.ActivationInsert(ctx, r.conn, db.ActivationRow{
		ID:        actID,
		Source:    "task",
		SourceID:  t.ID,
		Model:     r.llm.Model(),
		CreatedAt: now,
	})
	if err != nil {
		slog.Error("create task activation", "pkg", "task", "task_id", t.ID, "error", err)
		return
	}

	err = r.svc.SetActivationID(ctx, t.ID, actID)
	if err != nil {
		slog.Error("set task activation id", "pkg", "task", "task_id", t.ID, "error", err)
		return
	}

	err = r.svc.UpdateStatus(ctx, t.ID, "running")
	if err != nil {
		slog.Error("update task status to running", "pkg", "task", "task_id", t.ID, "error", err)
		return
	}

	ctx = context.WithValue(ctx, "meta", map[string]string{
		"activation_id": actID,
		"source":        "task",
		"source_id":     t.ID,
	})

	slog.Info("task started", "pkg", "task", "task_id", t.ID, "goal", t.Goal, "thinking", t.Thinking)

	// build per-task tool set: runner tools + task_report for this task
	reportTool := BuildReportTool(r.svc, t.ID)
	tools := append(r.tools, reportTool.Def)

	reportExec := reportTool.Handler
	loggingExec := func(ctx context.Context, call llm.ToolCall) (string, error) {
		start := time.Now()

		var result string
		var execErr error
		if call.Name == "task_report" {
			result, execErr = reportExec(ctx, call)
		} else {
			result, execErr = r.exec(ctx, call)
		}

		logErr := db.ToolCallInsertOne(ctx, r.conn, actID, call.Name, time.Since(start), execErr != nil)
		if logErr != nil {
			slog.Warn("log tool call", "pkg", "task", "task_id", t.ID, "error", logErr)
		}

		return result, execErr
	}

	instructions := r.renderPrompt(t, tools, member)

	output, usage, toolCalls, _, completeErr := r.llm.Complete(ctx, instructions, "", tools, loggingExec)

	statsErr := db.ActivationUpdateStats(ctx, r.conn, actID, db.ActivationStatsUpdate{
		InputTokens:     usage.InputTokens,
		OutputTokens:    usage.OutputTokens,
		TotalTokens:     usage.TotalTokens,
		CachedTokens:    usage.CachedTokens,
		ReasoningTokens: usage.ReasoningTokens,
		CostUSD:         llm.ComputeCost(r.llm.Model(), usage.InputTokens, usage.OutputTokens, usage.CachedTokens),
		ToolCallCount:   len(toolCalls),
		DurationMS:      time.Since(now).Milliseconds(),
	})
	if statsErr != nil {
		slog.Warn("update task activation stats", "pkg", "task", "task_id", t.ID, "error", statsErr)
	}

	if ctx.Err() != nil {
		bg := context.Background()
		r.svc.InsertReport(bg, t.ID, "error", "task timed out")
		r.svc.UpdateStatus(bg, t.ID, "cancelled")
		slog.Info("task timed out", "pkg", "task", "task_id", t.ID)
		return
	}

	if completeErr != nil {
		r.svc.InsertReport(ctx, t.ID, "error", completeErr.Error())
		r.svc.UpdateStatus(ctx, t.ID, "failed")
		slog.Info("task failed", "pkg", "task", "task_id", t.ID, "error", completeErr)
		return
	}

	r.svc.InsertReport(ctx, t.ID, "result", output)
	r.svc.UpdateStatus(ctx, t.ID, "completed")
	slog.Info("task completed", "pkg", "task", "task_id", t.ID, "goal", t.Goal)
}

func (r *Runner) Cancel(taskID string) bool {
	v, ok := r.cancels.LoadAndDelete(taskID)
	if !ok {
		return false
	}

	v.(context.CancelFunc)()
	return true
}
