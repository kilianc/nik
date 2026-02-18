package shell

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kciuffolo/nik/internal/brain"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/messaging"
)

type DataSource struct {
	msgsSvc *messaging.Service
}

func NewDataSource(msgsSvc *messaging.Service) *DataSource {
	return &DataSource{msgsSvc: msgsSvc}
}

func (d *DataSource) Check(ctx context.Context) ([]brain.DataSourceOutput, error) {
	sessions, err := listSessions()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	var outputs []brain.DataSourceOutput

	for _, s := range sessions {
		env, _ := getAllEnv(s.ID)

		if !s.Alive {
			output, _ := captureOutput(s.ID)
			code, _ := exitCode(s.ID)
			conversationID, msgs := d.conversationContext(ctx, env["NIK_CONVERSATION_ID"])
			senderLabels := d.msgsSvc.SenderLabels(ctx, msgs)

			slog.Info("shell completed", "pkg", "shell", "id", s.ID, "exit_code", code)

			killSession(s.ID)
			outputs = append(outputs, d.completionOutput(s, env, conversationID, output, code, msgs, senderLabels))
			continue
		}

		raw, ok := env["NIK_NEXT_CHECK_AT"]
		if !ok || raw == "" {
			continue
		}

		checkAt, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			continue
		}

		if now.Before(checkAt) {
			continue
		}

		// reset to default +5m; model overrides via read/send
		resetTime := now.Add(defaultCheckIn).UTC().Format(time.RFC3339)
		setEnv(s.ID, "NIK_NEXT_CHECK_AT", resetTime)

		output, _ := captureOutput(s.ID)
		conversationID, msgs := d.conversationContext(ctx, env["NIK_CONVERSATION_ID"])
		senderLabels := d.msgsSvc.SenderLabels(ctx, msgs)

		slog.Info("shell check-in", "pkg", "shell", "id", s.ID)

		outputs = append(outputs, d.checkInOutput(s, env, conversationID, output, msgs, senderLabels))
	}

	return outputs, nil
}

func (d *DataSource) conversationContext(ctx context.Context, conversationID string) (string, []db.Message) {
	if conversationID == "" {
		return "", nil
	}

	conv, msgs, err := d.msgsSvc.ConversationWithMessages(ctx, conversationID, 20)
	if err != nil {
		slog.Warn("shell context: resolve conversation", "pkg", "shell", "conversation_id", conversationID, "error", err)
		return "", nil
	}

	return conv.ID, msgs
}

func (d *DataSource) completionOutput(s SessionInfo, env map[string]string, conversationID string, output string, code int, msgs []db.Message, senderLabels map[string]string) brain.DataSourceOutput {
	duration := formatDuration(env["NIK_STARTED_AT"])

	lines := []string{
		"[Shell completed]",
		fmt.Sprintf("Session: %s", s.ID),
		fmt.Sprintf("Command: %s", env["NIK_COMMAND"]),
		fmt.Sprintf("Description: %s", env["NIK_DESCRIPTION"]),
		fmt.Sprintf("Conversation ID: %s", conversationID),
		fmt.Sprintf("Exit code: %d", code),
		fmt.Sprintf("Duration: %s", duration),
		"",
		"Output (last 16KB):",
		output,
	}

	lines = append(lines, formatConversationContext(msgs, senderLabels)...)

	meta := map[string]string{
		"conversation_id": env["NIK_CONVERSATION_ID"],
		"message_id":      env["NIK_MESSAGE_ID"],
		"source":          "shell",
		"source_id":       s.ID,
	}

	return brain.DataSourceOutput{
		Lines: lines,
		Meta:  meta,
	}
}

func (d *DataSource) checkInOutput(s SessionInfo, env map[string]string, conversationID string, output string, msgs []db.Message, senderLabels map[string]string) brain.DataSourceOutput {
	duration := formatDuration(env["NIK_STARTED_AT"])

	lines := []string{
		"[Shell check-in]",
		fmt.Sprintf("Session: %s", s.ID),
		fmt.Sprintf("Command: %s", env["NIK_COMMAND"]),
		fmt.Sprintf("Description: %s", env["NIK_DESCRIPTION"]),
		fmt.Sprintf("Conversation ID: %s", conversationID),
		"Status: running",
		fmt.Sprintf("Duration: %s", duration),
		"",
		"Current output (last 16KB):",
		output,
	}

	lines = append(lines, formatConversationContext(msgs, senderLabels)...)
	lines = append(lines, "", "Decide: send (type input), kill (stop), or reply to user and yield (next check-in in 5m). Read once to reschedule next_check_at.")

	meta := map[string]string{
		"conversation_id": env["NIK_CONVERSATION_ID"],
		"message_id":      env["NIK_MESSAGE_ID"],
		"source":          "shell",
		"source_id":       s.ID,
	}

	return brain.DataSourceOutput{
		Lines: lines,
		Meta:  meta,
	}
}

func formatConversationContext(msgs []db.Message, senderLabels map[string]string) []string {
	if len(msgs) == 0 {
		return nil
	}

	lines := []string{"", "## Recent conversation messages", ""}
	for _, m := range msgs {
		line := messaging.FormatMessageLine(m, senderLabels[m.ID])
		if line != "" {
			lines = append(lines, line)
		}
	}

	return lines
}

func formatDuration(startedAtStr string) string {
	if startedAtStr == "" {
		return "unknown"
	}

	started, err := time.Parse(time.RFC3339, startedAtStr)
	if err != nil {
		return "unknown"
	}

	return time.Since(started).Truncate(time.Second).String()
}
