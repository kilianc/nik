package messaging

import (
	"context"
	"time"

	"github.com/kciuffolo/nik/internal/brain"
	"github.com/kciuffolo/nik/internal/config"
)

// TaskQuerier provides active task context for conversation inputs.
type TaskQuerier interface {
	ConversationTasks(ctx context.Context, conversationID string) ([]TaskInfo, error)
}

// TaskInfo is the subset of task data needed for conversation context.
type TaskInfo struct {
	ID        string
	Goal      string
	Status    string
	CreatedAt time.Time
}

type DataSource struct {
	cfg   *config.Config
	svc   *Service
	tasks TaskQuerier
}

func NewDataSource(cfg *config.Config, svc *Service, tasks TaskQuerier) *DataSource {
	return &DataSource{
		cfg:   cfg,
		svc:   svc,
		tasks: tasks,
	}
}

func (d *DataSource) Check(ctx context.Context) ([]brain.DataSourceOutput, error) {
	conversationIDs, err := d.svc.PollUnreadConversationIDs(ctx)
	if err != nil {
		return nil, err
	}

	maxHistory := d.cfg.MaxHistory
	if maxHistory == 0 {
		maxHistory = 20
	}

	var outputs []brain.DataSourceOutput
	for _, conversationID := range conversationIDs {
		conv, msgs, convErr := d.svc.ConversationWithMessages(ctx, conversationID, maxHistory)
		if convErr != nil {
			continue
		}

		lastMessage := msgs[len(msgs)-1]

		// mark read synchronously so subsequent polls don't re-trigger
		if err := d.svc.MarkRead(ctx, conv.ID, lastMessage.SentAt); err != nil {
			continue
		}

		senderLabels := d.svc.SenderLabels(ctx, msgs)
		session := d.svc.SessionContext(ctx, conv)

		var tasks []TaskInfo
		if d.tasks != nil {
			tasks, _ = d.tasks.ConversationTasks(ctx, conversationID)
		}

		lines := BuildConversationInput(conv, msgs, senderLabels, session, tasks)

		meta := map[string]string{
			"conversation_id": conversationID,
			"platform":        conv.Platform,
			"source":          "message",
			"source_id":       lastMessage.ID,
		}

		outputs = append(outputs, brain.DataSourceOutput{
			Lines: lines,
			Meta:  meta,
		})
	}

	return outputs, nil
}
