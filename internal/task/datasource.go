package task

import (
	"context"
	"fmt"
	"log/slog"
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

	reports, err := d.svc.UnreadReports(ctx)
	if err != nil {
		return nil, fmt.Errorf("query unread task reports: %w", err)
	}

	for _, report := range reports {
		lines := d.formatReport(ctx, report)
		reportID := report.ID

		outputs = append(outputs, brain.DataSourceOutput{
			Lines: lines,
			Meta:  buildBrainMeta(report.TaskID, report.Meta),
			Processing: func(ctx context.Context) error {
				return d.svc.MarkRead(ctx, reportID)
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

	if convID := t.Meta["conversation_id"]; convID != "" {
		convLines := d.conversationContext(ctx, convID)
		if len(convLines) > 0 {
			lines = append(lines, "")
			lines = append(lines, convLines...)
		}
	}

	return lines
}

func (d *DataSource) formatReport(ctx context.Context, r db.TaskReport) []string {
	header := "[Task result]"
	switch r.Kind {
	case "error":
		header = "[Task error]"
	case "attention":
		header = "[Task needs attention]"
	}

	lines := []string{
		header,
		fmt.Sprintf("Task ID: %s", r.TaskID),
		fmt.Sprintf("Goal: %s", r.Goal),
		fmt.Sprintf("Status: %s", r.Status),
		"",
	}

	switch r.Kind {
	case "result":
		lines = append(lines, "## Result", "")
	case "error":
		lines = append(lines, "## Error", "")
	case "attention":
		lines = append(lines, "## Note", "")
	}

	lines = append(lines, r.Content)

	if convID := r.Meta["conversation_id"]; convID != "" {
		convLines := d.conversationContext(ctx, convID)
		if len(convLines) > 0 {
			lines = append(lines, "")
			lines = append(lines, convLines...)
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
