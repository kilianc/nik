package task

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/brain"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/messaging"
)

const (
	staleThreshold = 2 * time.Minute
	maxRunningTime = 10 * time.Minute
)

type DataSource struct {
	svc    *Service
	msgSvc *messaging.Service
}

func NewDataSource(svc *Service, msgSvc *messaging.Service) *DataSource {
	return &DataSource{
		svc:    svc,
		msgSvc: msgSvc,
	}
}

func (d *DataSource) Check(ctx context.Context) ([]brain.DataSourceOutput, error) {
	var outputs []brain.DataSourceOutput

	staleTasks, err := d.svc.StaleTasks(ctx, staleThreshold, maxRunningTime)
	if err != nil {
		slog.Warn("check stale tasks", "pkg", "task", "error", err)
	}

	for _, t := range staleTasks {
		lines := d.formatStaleTask(ctx, t)
		taskID := t.ID

		outputs = append(outputs, brain.DataSourceOutput{
			Lines: lines,
			Meta:  buildBrainMeta(taskID, t.Meta),
			Processed: func(ctx context.Context) error {
				return d.svc.MarkSeen(ctx, taskID)
			},
		})
	}

	items, err := d.svc.TasksNeedingAttention(ctx)
	if err != nil {
		return nil, fmt.Errorf("query tasks needing attention: %w", err)
	}

	for _, item := range items {
		lines := d.formatTaskAttention(ctx, item)
		a := item

		outputs = append(outputs, brain.DataSourceOutput{
			Lines: lines,
			Meta:  buildBrainMeta(a.TaskID, a.Meta),
			Processing: func(ctx context.Context) error {
				if a.ReportIDs != "" {
					for _, rid := range strings.Split(a.ReportIDs, ",") {
						d.svc.MarkRead(ctx, rid)
					}
				}
				return d.svc.MarkSeen(ctx, a.TaskID)
			},
		})
	}

	return outputs, nil
}

func (d *DataSource) formatStaleTask(ctx context.Context, t db.Task) []string {
	reason := fmt.Sprintf("No tool activity for %s.", staleThreshold)
	if t.StartedAt.Valid && time.Since(t.StartedAt.Time) > maxRunningTime {
		reason = fmt.Sprintf("Running for %s (over %s limit).", time.Since(t.StartedAt.Time).Truncate(time.Second), maxRunningTime)
	}

	lines := []string{
		"[Long-running task]",
		fmt.Sprintf("Task ID: %s", t.ID),
		fmt.Sprintf("Goal: %s", t.Goal),
		fmt.Sprintf("Status: %s", t.Status),
		"",
		reason + " Check on it, cancel it, or leave it if you have a reason.",
	}

	if t.RetryNumber > 0 {
		lines = append(lines, fmt.Sprintf("Retry: #%d", t.RetryNumber))
	}

	lines = append(lines, d.retryChainContext(ctx, t.RetryForTaskID, t.RetryNumber)...)

	if convID := t.Meta["conversation_id"]; convID != "" {
		convLines := d.conversationContext(ctx, convID)
		if len(convLines) > 0 {
			lines = append(lines, "")
			lines = append(lines, convLines...)
		}
	}

	return lines
}

func (d *DataSource) formatTaskAttention(ctx context.Context, a db.TaskAttention) []string {
	var header string
	switch a.Status {
	case "failed":
		header = "[Task failed]"
	case "completed":
		header = "[Task completed]"
	default:
		header = "[Task needs attention]"
	}

	lines := []string{
		header,
		fmt.Sprintf("Task ID: %s", a.TaskID),
		fmt.Sprintf("Goal: %s", a.Goal),
		fmt.Sprintf("Status: %s", a.Status),
	}

	if a.RetryNumber > 0 {
		lines = append(lines, fmt.Sprintf("Retry: #%d", a.RetryNumber))
	}

	if a.Reports != "" {
		lines = append(lines, "", a.Reports)
	}

	lines = append(lines, d.retryChainContext(ctx, a.RetryForTaskID, a.RetryNumber)...)

	if convID := a.Meta["conversation_id"]; convID != "" {
		convLines := d.conversationContext(ctx, convID)
		if len(convLines) > 0 {
			lines = append(lines, "")
			lines = append(lines, convLines...)
		}
	}

	return lines
}

func (d *DataSource) retryChainContext(ctx context.Context, retryForTaskID string, retryNumber int) []string {
	if retryForTaskID == "" && retryNumber == 0 {
		return nil
	}

	rootID := retryForTaskID
	if rootID == "" {
		return nil
	}

	chain, err := d.svc.RetryChain(ctx, rootID)
	if err != nil {
		slog.Warn("fetch retry chain for datasource", "pkg", "task", "root_id", rootID, "error", err)
		return nil
	}

	if len(chain) <= 1 {
		return nil
	}

	var lines []string
	lines = append(lines, "", "## Previous attempts")
	for _, entry := range chain {
		lines = append(lines, fmt.Sprintf("- Attempt #%d (%s): %s", entry.RetryNumber, entry.Status, entry.Goal))
		for _, r := range entry.Reports {
			prefix := ""
			if r.ReportedAt.Valid {
				prefix = "[already read] "
			}
			lines = append(lines, fmt.Sprintf("  Report: %s%s", prefix, r.Content))
		}
	}

	return lines
}

func buildBrainMeta(taskID string, taskMeta map[string]string) map[string]string {
	meta := map[string]string{
		"source":    "task",
		"source_id": taskID,
	}
	for k, v := range taskMeta {
		if v != "" {
			meta[k] = v
		}
	}
	return meta
}

func (d *DataSource) conversationContext(ctx context.Context, conversationID string) []string {
	if d.msgSvc == nil {
		return nil
	}

	_, msgs, err := d.msgSvc.ConversationWithMessages(ctx, conversationID, 10)
	if err != nil {
		return nil
	}

	senderLabels := d.msgSvc.SenderLabels(ctx, msgs)

	lines := []string{"## Recent conversation", ""}
	for _, m := range msgs {
		line := messaging.FormatMessageLine(m, senderLabels[m.ID])
		if line != "" {
			lines = append(lines, line)
		}
	}

	return lines
}
