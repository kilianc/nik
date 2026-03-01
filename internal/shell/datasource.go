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
	msgsSvc      *messaging.Service
	isActivating func(string) bool
}

func NewDataSource(msgsSvc *messaging.Service, isActivating func(string) bool) *DataSource {
	return &DataSource{
		msgsSvc:      msgsSvc,
		isActivating: isActivating,
	}
}

func (d *DataSource) Check(ctx context.Context) ([]brain.DataSourceOutput, error) {
	sessions, err := listSessions()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	var outputs []brain.DataSourceOutput

	for _, s := range sessions {
		meta, err := loadMeta(s.ID)
		if err != nil {
			slog.Warn("shell check: load meta", "pkg", "shell", "id", s.ID, "error", err)
			continue
		}

		if d.isActivating(meta.ActivationID) {
			continue
		}

		// dead sessions trigger immediately, alive sessions wait for next_check_at
		if s.isAlive && now.Before(meta.NextCheckAt) {
			continue
		}

		status := "running"
		if !s.isAlive {
			status = "exited"
		}

		slog.Info("shell nudge", "pkg", "shell", "id", s.ID, "status", status)

		sessionID := s.ID
		conversationID, msgs := d.conversationContext(ctx, meta.ConversationID)
		senderLabels := d.msgsSvc.SenderLabels(ctx, msgs)

		lines := []string{
			fmt.Sprintf("[Shell session %s needs attention]", status),
			fmt.Sprintf("Session: %s", sessionID),
			fmt.Sprintf("Command: %s", meta.Command),
			fmt.Sprintf("Description: %s", meta.Description),
			fmt.Sprintf("Conversation ID: %s", conversationID),
			fmt.Sprintf("Duration: %s", time.Since(meta.StartedAt).Truncate(time.Second)),
			"",
			"Use shell read to check on this session.",
		}

		lines = append(lines, formatConversationContext(msgs, senderLabels)...)

		outputs = append(outputs, brain.DataSourceOutput{
			Lines: lines,
			Meta: map[string]string{
				"conversation_id": meta.ConversationID,
				"message_id":      meta.MessageID,
				"source":          "shell",
				"source_id":       sessionID,
			},
			Processing: func(ctx context.Context) error {
				ctxMeta, _ := ctx.Value("meta").(map[string]string)
				m, err := loadMeta(sessionID)
				if err != nil {
					return err
				}
				m.ActivationID = ctxMeta["activation_id"]
				m.NextCheckAt = time.Now().Add(defaultCheckIn).UTC()
				return saveMeta(sessionID, m)
			},
		})
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
