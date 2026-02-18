package messaging

import (
	"context"

	"github.com/kciuffolo/nik/internal/brain"
	"github.com/kciuffolo/nik/internal/config"
)

type DataSource struct {
	cfg *config.Config
	svc *Service
}

func NewDataSource(cfg *config.Config, svc *Service) *DataSource {
	return &DataSource{
		cfg: cfg,
		svc: svc,
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

		senderLabels := d.svc.SenderLabels(ctx, msgs)
		session := d.svc.SessionContext(ctx, conv)
		lines := BuildConversationInput(conv, msgs, senderLabels, session)
		lastMessage := msgs[len(msgs)-1]

		meta := map[string]string{
			"conversation_id": conversationID,
			"platform":        conv.Platform,
			"source":          "message",
			"source_id":       msgs[len(msgs)-1].ID,
		}

		outputs = append(outputs, brain.DataSourceOutput{
			Lines: lines,
			Meta:  meta,
			Processing: func(ctx context.Context) error {
				return d.svc.MarkRead(ctx, conv.ID, lastMessage.SentAt)
			},
		})
	}

	return outputs, nil
}
