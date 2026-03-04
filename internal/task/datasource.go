package task

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kciuffolo/nik/internal/brain"
	"github.com/kciuffolo/nik/internal/messaging"
)

const staleThreshold = 2 * time.Minute

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

	staleTasks, err := d.svc.StaleTasks(ctx, staleThreshold)
	if err != nil {
		slog.Warn("check stale tasks", "pkg", "task", "error", err)
	}

	for _, t := range staleTasks {
		lines := d.formatStaleTask(ctx, t)
		taskID := t.ID

		meta := map[string]string{
			"source":    t.Source,
			"source_id": t.SourceID,
		}
		if t.Source == "message" && t.SourceID != "" {
			meta["conversation_id"] = t.SourceID
		}

		outputs = append(outputs, brain.DataSourceOutput{
			Lines: lines,
			Meta:  meta,
			Processed: func(ctx context.Context) error {
				return d.svc.MarkChecked(ctx, taskID)
			},
		})
	}

	reports, err := d.svc.UnreportedReports(ctx)
	if err != nil {
		return nil, fmt.Errorf("query unreported task reports: %w", err)
	}

	for _, report := range reports {
		lines := d.formatReport(ctx, report)
		reportID := report.ID

		meta := map[string]string{
			"source":    report.Source,
			"source_id": report.SourceID,
		}
		if report.Source == "message" && report.SourceID != "" {
			meta["conversation_id"] = report.SourceID
		}

		outputs = append(outputs, brain.DataSourceOutput{
			Lines: lines,
			Meta:  meta,
			Processing: func(ctx context.Context) error {
				return d.svc.MarkReported(ctx, reportID)
			},
		})
	}

	return outputs, nil
}

func (d *DataSource) formatStaleTask(ctx context.Context, t Task) []string {
	lines := []string{
		"[Stale task]",
		fmt.Sprintf("Task ID: %s", t.ID),
		fmt.Sprintf("Goal: %s", t.Goal),
		fmt.Sprintf("Status: %s", t.Status),
		"",
		fmt.Sprintf("No tool activity for %s. Cancel it or leave it if you have a reason.", staleThreshold),
	}

	if t.Source == "message" && t.SourceID != "" {
		convLines := d.conversationContext(ctx, t.SourceID)
		if len(convLines) > 0 {
			lines = append(lines, "")
			lines = append(lines, convLines...)
		}
	}

	return lines
}

func (d *DataSource) formatReport(ctx context.Context, r Report) []string {
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

	if r.Source == "message" && r.SourceID != "" {
		convLines := d.conversationContext(ctx, r.SourceID)
		if len(convLines) > 0 {
			lines = append(lines, "")
			lines = append(lines, convLines...)
		}
	}

	return lines
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

// implements brain.DebugTaskQuerier.
func (s *Service) ActiveTasksForConversation(ctx context.Context, conversationID string) ([]brain.DebugTaskInfo, error) {
	active, err := s.ActiveTasks(ctx, "message", conversationID)
	if err != nil {
		return nil, err
	}

	out := make([]brain.DebugTaskInfo, len(active))
	for i, t := range active {
		out[i] = brain.DebugTaskInfo{
			ID:        t.ID,
			Goal:      t.Goal,
			Status:    t.Status,
			CreatedAt: t.CreatedAt,
		}
	}

	return out, nil
}

// implements messaging.TaskQuerier.
func (s *Service) ConversationTasks(ctx context.Context, conversationID string) ([]messaging.TaskInfo, error) {
	active, err := s.ActiveTasks(ctx, "message", conversationID)
	if err != nil {
		return nil, err
	}

	out := make([]messaging.TaskInfo, len(active))
	for i, t := range active {
		out[i] = messaging.TaskInfo{
			ID:        t.ID,
			Goal:      t.Goal,
			Status:    t.Status,
			CreatedAt: t.CreatedAt,
		}
	}

	return out, nil
}
