// Package apisvc implements nikd's API against the things nikd owns.
//
// It exists so internal/api can stay a transport: the handlers there know
// about HTTP and JSON, these types know about SQLite and the messaging
// service, and the interface between them is what keeps internal/nikapi —
// which nikctl links — from dragging the database into the client binary.
package apisvc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/kciuffolo/nik/internal/api"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/messaging"
)

type Chat struct {
	conn      *sql.DB
	messaging *messaging.Service
}

func NewChat(conn *sql.DB, msg *messaging.Service) *Chat {
	return &Chat{conn: conn, messaging: msg}
}

// resolveID turns the API's spelling of a conversation into the database's.
// "local" is an alias for a fixed UUID; everything else is already an id.
func resolveID(convID string) string {
	if convID == api.LocalConversationID {
		return db.LocalConversationID
	}

	return convID
}

func (c *Chat) Conversation(ctx context.Context, convID string) (api.Conversation, error) {
	conv, err := db.ConversationGet(ctx, c.conn, db.ConversationGetParams{ID: resolveID(convID)})
	if errors.Is(err, sql.ErrNoRows) {
		return api.Conversation{}, api.ErrNotFound
	}
	if err != nil {
		return api.Conversation{}, fmt.Errorf("get conversation: %w", err)
	}

	out := api.Conversation{
		ID:       conv.ID,
		Kind:     conv.Kind,
		Title:    conv.Title.String,
		Activity: conv.Activity,
	}
	if conv.LastMessageAt.Valid {
		out.LastMessageAt = conv.LastMessageAt.Time
	}
	if out.Activity == nil {
		out.Activity = []string{}
	}

	return out, nil
}

func (c *Chat) Messages(ctx context.Context, p api.MessagesQuery) ([]api.Message, error) {
	rows, err := db.MessageList(ctx, c.conn, db.MessageListParams{
		ConversationID: resolveID(p.ConversationID),
		AfterID:        p.After,
		Limit:          p.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}

	// The query returns newest-first because that is what a LIMIT wants;
	// every reader wants oldest-first. Reversing here rather than in each
	// client is what stops the TUI and the console disagreeing about it.
	slices.Reverse(rows)

	out := make([]api.Message, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAPIMessage(row))
	}

	return out, nil
}

// Send records a message from the owner. It is the same call the TUI makes
// today, moved into the process that owns the brain — so a message and the
// activation that answers it are no longer two processes writing one file.
func (c *Chat) Send(ctx context.Context, convID, body string) error {
	if resolveID(convID) != db.LocalConversationID {
		// Sending into a WhatsApp thread is nik's decision, not a client's:
		// the brain calls message_send after deciding to speak, and a
		// side-door that skips that would put words in nik's mouth.
		return fmt.Errorf("can only send to the local conversation, not %q", convID)
	}

	return c.messaging.ReceiveMessage(ctx, messaging.InboundMessage{
		Platform:               "local",
		ExternalConversationID: db.LocalConversationID,
		ExternalMessageID:      id.V7(),
		ExternalSenderID:       db.OwnerContactID,
		ExternalSenderIDs:      []string{db.OwnerContactID},
		Kind:                   "text",
		Body:                   body,
		SentAt:                 time.Now(),
	})
}

func toAPIMessage(m db.Message) api.Message {
	return api.Message{
		ID:          m.ID,
		Kind:        m.Kind,
		Body:        m.Body,
		SentAt:      m.SentAt,
		IsFromMe:    m.IsFromMe,
		Platform:    m.Platform,
		ContactID:   m.ContactID,
		MediaID:     m.MediaID.String,
		Transcript:  m.MediaTranscriptText.String,
		Description: m.MediaDescribeText.String,
	}
}
