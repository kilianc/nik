package task

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/llm"
	"github.com/kciuffolo/nik/internal/log"
)

const runnerTimeout = 20 * time.Minute

const (
	runnerMaxAttempts = 3
	runnerLoopThreshold = 4
)

type Runner struct {
	cfg       *config.Config
	llm       *llm.Client
	recorder  llm.ActivationRecorder
	criticLLM *llm.Client
	svc       *Service
	tools     []llm.Tool
	cancels   sync.Map
	wg        sync.WaitGroup
}

func NewRunner(cfg *config.Config, llmClient *llm.Client, svc *Service, tools []llm.Tool) *Runner {
	return &Runner{
		cfg:      cfg,
		llm:      llmClient,
		recorder: llm.NoopRecorder{},
		svc:      svc,
		tools:    tools,
	}
}

func (r *Runner) SetRecorder(rec llm.ActivationRecorder) {
	r.recorder = rec
}

func (r *Runner) SetCriticLLM(c *llm.Client) {
	r.criticLLM = c
}

func (r *Runner) Wait() { r.wg.Wait() }

func (r *Runner) Run(ctx context.Context, t db.Task) {
	defer r.wg.Done()
	ctx, cancel := context.WithTimeout(ctx, runnerTimeout)
	r.cancels.Store(t.ID, cancel)
	defer r.cancels.Delete(t.ID)
	defer cancel()

	actID := id.V7()

	ctx = context.WithValue(ctx, "meta", map[string]string{
		"conversation_id": t.ConversationID,
		"task_id":         t.ID,
		"activation_id":   actID,
		"sources":         `["task"]`,
	})

	tools := r.tools
	if !r.cfg.IsPrivileged(t.ConversationID) {
		tools = filterUnprivileged(tools)
	}

	reportTool := BuildReportTool(r.svc, t.ID)
	allTools := append(tools, reportTool)
	defs, exec := llm.SplitTools(allTools)

	instructions := r.renderPrompt(t, defs)

	err := r.svc.Start(ctx, t.ID, actID)
	if err != nil {
		slog.Error("start task", "pkg", "task", "task_id", t.ID, "error", err)
		return
	}

	slog.Info("task started", "pkg", "task", "task_id", t.ID, "goal", t.Goal, "thinking", t.Thinking)

	act := llm.NewActivation(r.llm, r.recorder, instructions, defs)
	act.Start(ctx)
	defer act.Close(ctx)
	act.SetInput("")

	runErr := r.runLoop(ctx, t, act, exec)

	if ctx.Err() != nil {
		r.svc.UpdateStatus(context.Background(), t.ID, "cancelled")
		slog.Info("task cancelled", "pkg", "task", "task_id", t.ID, "reason", ctx.Err())
		return
	}

	if runErr != nil {
		r.svc.UpdateStatus(ctx, t.ID, "failed")
		slog.Info("task failed", "pkg", "task", "task_id", t.ID, "error", runErr)
	} else {
		finalStatus := "failed"
		reportStatus, reportErr := r.svc.LastReportStatus(ctx, t.ID)
		if reportErr == nil && (reportStatus == "completed" || reportStatus == "failed") {
			finalStatus = reportStatus
		}
		r.svc.UpdateStatus(ctx, t.ID, finalStatus)
		slog.Info("task "+finalStatus, "pkg", "task", "task_id", t.ID, "goal", t.Goal)
	}

	t.ActivationID = actID
	current, getErr := r.svc.Get(ctx, t.ID)
	if getErr == nil {
		t = current
	} else {
		slog.Warn("get task for critic", "pkg", "task", "task_id", t.ID, "error", getErr)
	}

	r.RunCritic(context.Background(), t)
}

func (r *Runner) runLoop(ctx context.Context, t db.Task, act *llm.Activation, exec llm.ToolExecutor) error {
	var nudged bool

	for {
		result, err := act.Round(ctx)
		if err != nil && llm.IsTransient(err) && act.Attempt() <= runnerMaxAttempts {
			slog.Warn("transient API error, retrying", "pkg", "task", "task_id", t.ID, "attempt", act.Attempt(), "error", err)
			time.Sleep(llm.RetryDelay(act.Attempt()))
			continue
		}
		if err != nil {
			return err
		}

		if result.Incomplete {
			return fmt.Errorf("response incomplete in round %d", act.RoundNumber()-1)
		}

		if len(result.ToolCalls) == 0 {
			if !nudged {
				nudged = true
				nudgeText := r.loadNudge()
				if nudgeText != "" {
					reportStatus, reportErr := r.svc.LastReportStatus(ctx, t.ID)
					if reportErr == nil && (reportStatus == "completed" || reportStatus == "failed") {
						return nil
					}
					act.AppendAssistantText(result.Text)
					act.AppendUserMessage(nudgeText)
					continue
				}
			}
			return nil
		}

		if act.Repeats() >= runnerLoopThreshold {
			return fmt.Errorf("loop: %d identical rounds calling %s", act.Repeats(), result.ToolCalls[0].Name)
		}

		for _, call := range result.ToolCalls {
			slog.Info("tool call", log.ToolCallAttrs(ctx, "task", call.Name, act.RoundNumber()-1, call.Arguments)...)
		}

		act.ExecuteTools(ctx, result, exec)
		act.Prune()
	}
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
