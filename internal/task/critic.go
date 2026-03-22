package task

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"text/template"
	"time"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/llm"
)

type criticPromptData struct {
	Now              string
	Goal             string
	Plan             string
	Status           string
	ObservedDuration string
	ToolCalls        string
	Reports          string
	Skills           string
}

type assessOutput struct {
	EffectivenessScore      int    `json:"effectiveness_score"`
	EffectivenessFeedback   string `json:"effectiveness_feedback"`
	ExpectedDurationSeconds int    `json:"expected_duration_seconds"`
	DurationFeedback        string `json:"duration_feedback"`
	ToolFeedback            string `json:"tool_feedback"`
	SkillFeedback           string `json:"skill_feedback"`
	Recommendations         string `json:"recommendations"`
}

const criticTimeout = 5 * time.Minute

const criticRetryInput = "Your previous response did not satisfy the required output contract. Return a single json object only."

func (r *Runner) RunCritic(ctx context.Context, t db.Task) {
	if !r.cfg.Models.Critic.Enabled || r.criticLLM == nil {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, criticTimeout)
	defer cancel()

	actID := id.V7()

	ctx = context.WithValue(ctx, "meta", map[string]string{
		"conversation_id": t.ConversationID,
		"task_id":         t.ID,
		"activation_id":   actID,
		"sources":         `["critic"]`,
	})

	var toolCalls []db.ToolCallInfo
	if t.ActivationID != "" {
		toolCalls, _ = r.svc.AllToolCalls(ctx, t.ActivationID)
	}

	toolCallsStr := formatToolCalls(toolCalls)
	reportsStr := r.formatReports(ctx, t.ID)
	skillsStr := extractSkillNames(toolCalls)

	instructions := r.renderCriticPrompt(t, toolCallsStr, reportsStr, skillsStr)

	act := llm.NewActivation(r.criticLLM, llm.NoopRecorder{}, instructions, nil)
	act.SetInput("")

	result, err := act.Round(ctx)
	if err != nil {
		slog.Warn("critic failed", "pkg", "task", "task_id", t.ID, "error", err)
		return
	}

	assessment, parseErr := parseCriticOutput(result.Text)
	if parseErr != nil {
		slog.Warn("critic parse failed, retrying", "pkg", "task", "task_id", t.ID, "error", parseErr)

		act.AppendAssistantText(result.Text)
		act.AppendUserMessage(criticRetryInput)

		result, err = act.Round(ctx)
		if err != nil {
			slog.Warn("critic retry failed", "pkg", "task", "task_id", t.ID, "error", err)
			return
		}

		assessment, parseErr = parseCriticOutput(result.Text)
		if parseErr != nil {
			slog.Warn("critic parse failed after retry", "pkg", "task", "task_id", t.ID, "error", parseErr)
			return
		}
	}

	err = r.svc.InsertAssessment(ctx, db.TaskAssessmentInsertParams{
		TaskID:                  t.ID,
		ActivationID:            actID,
		EffectivenessScore:      assessment.EffectivenessScore,
		EffectivenessFeedback:   assessment.EffectivenessFeedback,
		ExpectedDurationSeconds: assessment.ExpectedDurationSeconds,
		DurationFeedback:        assessment.DurationFeedback,
		ToolFeedback:            assessment.ToolFeedback,
		SkillFeedback:           assessment.SkillFeedback,
		Recommendations:         assessment.Recommendations,
	})
	if err != nil {
		slog.Warn("critic insert assessment", "pkg", "task", "task_id", t.ID, "error", err)
	}
}

func parseCriticOutput(raw string) (assessOutput, error) {
	raw = strings.TrimSpace(raw)

	if strings.HasPrefix(raw, "```") {
		lines := strings.SplitN(raw, "\n", 2)
		if len(lines) == 2 {
			raw = lines[1]
		}
		if idx := strings.LastIndex(raw, "```"); idx >= 0 {
			raw = raw[:idx]
		}
		raw = strings.TrimSpace(raw)
	}

	var intermediate struct {
		EffectivenessScore      int             `json:"effectiveness_score"`
		EffectivenessFeedback   json.RawMessage `json:"effectiveness_feedback"`
		ExpectedDurationSeconds *int            `json:"expected_duration_seconds"`
		DurationFeedback        json.RawMessage `json:"duration_feedback"`
		ToolFeedback            json.RawMessage `json:"tool_feedback"`
		SkillFeedback           json.RawMessage `json:"skill_feedback"`
		Recommendations         json.RawMessage `json:"recommendations"`
	}

	err := json.Unmarshal([]byte(raw), &intermediate)
	if err != nil {
		return assessOutput{}, fmt.Errorf("unmarshal critic output: %w", err)
	}

	if intermediate.EffectivenessScore < 1 || intermediate.EffectivenessScore > 5 {
		return assessOutput{}, fmt.Errorf("effectiveness_score must be 1-5, got %d", intermediate.EffectivenessScore)
	}

	if intermediate.ExpectedDurationSeconds == nil {
		return assessOutput{}, fmt.Errorf("expected_duration_seconds is required")
	}

	if *intermediate.ExpectedDurationSeconds < 0 {
		return assessOutput{}, fmt.Errorf("expected_duration_seconds must be >= 0, got %d", *intermediate.ExpectedDurationSeconds)
	}

	out := assessOutput{
		EffectivenessScore:      intermediate.EffectivenessScore,
		EffectivenessFeedback:   coerceString(intermediate.EffectivenessFeedback),
		ExpectedDurationSeconds: *intermediate.ExpectedDurationSeconds,
		DurationFeedback:        coerceString(intermediate.DurationFeedback),
		ToolFeedback:            coerceString(intermediate.ToolFeedback),
		SkillFeedback:           coerceString(intermediate.SkillFeedback),
		Recommendations:         coerceString(intermediate.Recommendations),
	}

	return out, nil
}

// coerceString extracts a string from raw JSON. If the value is a JSON
// string it is unquoted; objects and arrays are returned as compact JSON.
func coerceString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}

	return string(raw)
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
		fmt.Fprintf(&b, "- round %d: %s [%s] %dms\n", tc.Round, tc.Name, status, tc.DurationMS)

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

func (r *Runner) renderCriticPrompt(t db.Task, toolCalls, reports, skills string) string {
	raw, err := readFromPromptsRoot(r.cfg.PromptsPath(), "critic-00.md")
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
		Now:              now,
		Goal:             t.Goal,
		Plan:             t.Plan,
		Status:           t.Status,
		ObservedDuration: observedDuration(t),
		ToolCalls:        toolCalls,
		Reports:          reports,
		Skills:           skills,
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
	return fmt.Sprintf("Evaluate task %s.\nGoal: %s\nStatus: %s\nObserved duration: %s\n\nTool calls:\n%s\nReports:\n%s\n\nRespond with JSON: {\"effectiveness_score\": <1-5>, \"effectiveness_feedback\": \"...\", \"expected_duration_seconds\": <integer>, \"duration_feedback\": \"...\", \"tool_feedback\": \"...\", \"skill_feedback\": \"...\", \"recommendations\": \"...\"}\n\nAll feedback fields must be plain markdown prose -- no nested JSON.",
		t.ID, t.Goal, t.Status, observedDuration(t), toolCalls, reports)
}

func observedDuration(t db.Task) string {
	if !t.CompletedAt.Valid {
		return "unknown"
	}

	start := t.CreatedAt
	if t.StartedAt.Valid {
		start = t.StartedAt.Time
	}

	duration := t.CompletedAt.Time.Sub(start)
	if duration < 0 {
		duration = 0
	}

	seconds := int(duration / time.Second)
	return fmt.Sprintf("%d seconds", seconds)
}
