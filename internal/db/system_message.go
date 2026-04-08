package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/id"
)

type SystemMessageParams struct {
	ConversationID  string
	Kind            string
	Body            any
	SentAt          time.Time
	ContextStanzaID string
}

func SystemMessageInsert(ctx context.Context, db DBTX, p SystemMessageParams) error {
	msgID := id.V7()

	bodyJSON, err := json.Marshal(p.Body)
	if err != nil {
		return fmt.Errorf("marshal system message body: %w", err)
	}

	params := MessageInsertParams{
		ID:                     msgID,
		ConversationID:         p.ConversationID,
		ContactID:              SystemContactID,
		Platform:               "system",
		ExternalConversationID: p.ConversationID,
		ExternalMessageID:      msgID,
		ExternalSenderID:       SystemContactID,
		Kind:                   p.Kind,
		Body:                   string(bodyJSON),
		ContextMentionedIDs:    "[]",
		SentAt:                 p.SentAt,
	}

	if p.ContextStanzaID != "" {
		params.ContextStanzaID = p.ContextStanzaID
	}

	_, err = MessageInsert(ctx, db, params)
	return err
}
