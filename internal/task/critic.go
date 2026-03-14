package task

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/llm"
)

type criticPromptData struct {
	Now        string
	Home       string
	Goal       string
	Plan       string
	Status     string
	ToolCalls  string
	Reports    string
	Skills     string
	ToolDocs   string
	SkillIndex string
}

const criticTimeout = 5 * time.Minute

func (r *Runner) RunCritic(ctx context.Context, t db.Task) {
	if !r.cfg.CriticEnabled || r.criticLLM == nil {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, criticTimeout)
	defer cancel()

	ctx = context.WithValue(ctx, "meta", map[string]string{
		"conversation_id": t.ConversationID,
		"task_id":         t.ID,
		"sources":         `["critic"]`,
	})

	var toolCalls []db.ToolCallInfo
	if t.ActivationID != "" {
		toolCalls, _ = r.svc.AllToolCalls(ctx, t.ActivationID)
	}

	toolCallsStr := formatToolCalls(toolCalls)
	reportsStr := r.formatReports(ctx, t.ID)
	skillsStr := extractSkillNames(toolCalls)

	assessTool := BuildAssessTool(r.svc, t.ID)

	filtered := make([]llm.Tool, 0, len(r.tools)+1)
	filtered = append(filtered, assessTool)
	for _, tool := range r.tools {
		if tool.Def.Name != "task_report" {
			filtered = append(filtered, tool)
		}
	}

	defs, exec := llm.SplitTools(filtered)
	instructions := r.renderCriticPrompt(t, toolCallsStr, reportsStr, skillsStr, defs)

	_, ch := r.criticLLM.Complete(ctx, instructions, llm.StaticInput(""), defs, exec)
	result := <-ch

	if result.Err != nil {
		slog.Warn("critic failed", "pkg", "task", "task_id", t.ID, "error", result.Err)
	}
}

func formatToolCalls(calls []db.ToolCallInfo) string {
	if len(calls) == 0 {
		return "(no tool calls recorded)"
	}

	var b strings.Builder
	for _, tc := range calls {
		status := "ok"
		if tc.Error {
			status = "ERROR"
		}
		fmt.Fprintf(&b, "- %s [%s] %dms\n", tc.Name, status, tc.DurationMS)

		if tc.Error && tc.Output != "" {
			output := tc.Output
			if len(output) > 200 {
				output = output[:200] + "..."
			}
			fmt.Fprintf(&b, "  error: %s\n", output)
		}
	}

	return b.String()
}

func (r *Runner) formatReports(ctx context.Context, taskID string) string {
	reports, err := r.svc.ReportsByTask(ctx, taskID)
	if err != nil || len(reports) == 0 {
		return "(no reports)"
	}

	var b strings.Builder
	for _, rpt := range reports {
		fmt.Fprintf(&b, "- [%s] %s: %s\n", rpt.CreatedAt.Format("15:04:05"), rpt.Status, rpt.Content)
	}

	return b.String()
}

func extractSkillNames(calls []db.ToolCallInfo) string {
	var loaded []string
	for _, tc := range calls {
		if tc.Name != "load_skill" {
			continue
		}

		var args struct {
			Action string `json:"action"`
			Name   string `json:"name"`
		}

		err := json.Unmarshal([]byte(tc.Input), &args)
		if err != nil || args.Action != "load" || args.Name == "" {
			continue
		}

		loaded = append(loaded, args.Name)
	}

	if len(loaded) == 0 {
		return "(none)"
	}

	return strings.Join(loaded, ", ")
}

func (r *Runner) renderCriticPrompt(t db.Task, toolCalls, reports, skills string, tools []llm.ToolDef) string {
	tmplPath := filepath.Join(r.cfg.PromptsPath(), "critic-00.md")

	raw, err := os.ReadFile(tmplPath)
	if err != nil {
		slog.Warn("load critic prompt template", "pkg", "task", "error", err)
		return fallbackCriticPrompt(t, toolCalls, reports)
	}

	tmpl, err := template.New("critic").Parse(string(raw))
	if err != nil {
		slog.Warn("parse critic prompt template", "pkg", "task", "error", err)
		return fallbackCriticPrompt(t, toolCalls, reports)
	}

	loc := r.cfg.TZ()
	now := time.Now().In(loc).Format("Monday, January 2, 2006 3:04 PM")

	data := criticPromptData{
		Now:        now,
		Home:       r.cfg.Home,
		Goal:       t.Goal,
		Plan:       t.Plan,
		Status:     t.Status,
		ToolCalls:  toolCalls,
		Reports:    reports,
		Skills:     skills,
		ToolDocs:   buildToolDocs(tools),
		SkillIndex: buildSkillDocs(r.cfg),
	}

	var buf strings.Builder
	err = tmpl.Execute(&buf, data)
	if err != nil {
		slog.Warn("execute critic prompt template", "pkg", "task", "error", err)
		return fallbackCriticPrompt(t, toolCalls, reports)
	}

	return buf.String()
}

func fallbackCriticPrompt(t db.Task, toolCalls, reports string) string {
	return fmt.Sprintf("Evaluate task %s.\nGoal: %s\nStatus: %s\n\nTool calls:\n%s\nReports:\n%s",
		t.ID, t.Goal, t.Status, toolCalls, reports)
}
