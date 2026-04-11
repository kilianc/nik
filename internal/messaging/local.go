package messaging

import (
	"context"
	"database/sql"
	"time"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
)

type LocalAdapter struct {
	conn *sql.DB
}

func NewLocalAdapter(conn *sql.DB) *LocalAdapter {
	return &LocalAdapter{conn: conn}
}

func (a *LocalAdapter) Platform() string { return "local" }

func (a *LocalAdapter) Start(_ context.Context, _ MessageReceiver) error { return nil }
func (a *LocalAdapter) Stop(_ context.Context) error                     { return nil }

func (a *LocalAdapter) Reply(_ context.Context, _ string, body string, _ *QuoteTarget) (OutboundMessage, error) {
	return OutboundMessage{
		ExternalMessageID: id.V7(),
		ExternalSenderID:  db.NikContactID,
		SentAt:            time.Now(),
		Kind:              "text",
		Body:              body,
	}, nil
}

func (a *LocalAdapter) SendFile(_ context.Context, _ string, _ string, caption string) (OutboundMessage, error) {
	return OutboundMessage{
		ExternalMessageID: id.V7(),
		ExternalSenderID:  db.NikContactID,
		SentAt:            time.Now(),
		Kind:              "document",
		Body:              caption,
	}, nil
}

func (a *LocalAdapter) SendVoiceNote(_ context.Context, _ string, _ string) (OutboundMessage, error) {
	return OutboundMessage{
		ExternalMessageID: id.V7(),
		ExternalSenderID:  db.NikContactID,
		SentAt:            time.Now(),
		Kind:              "audio",
	}, nil
}

func (a *LocalAdapter) React(_ context.Context, _, _, _, _ string) (OutboundMessage, error) {
	return OutboundMessage{}, nil
}

func (a *LocalAdapter) SetPresence(_ context.Context, _ bool) error { return nil }

func (a *LocalAdapter) StartTyping(ctx context.Context, _ string) error {
	return db.ConversationActivityPush(ctx, a.conn, db.LocalConversationID, "typing")
}

func (a *LocalAdapter) StopTyping(ctx context.Context, _ string) error {
	return db.ConversationActivityPop(ctx, a.conn, db.LocalConversationID, "typing")
}

func (a *LocalAdapter) MarkRead(_ context.Context, _ []InboundMessage) error { return nil }
