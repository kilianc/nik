package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kciuffolo/nik/internal/queries"
)

func scanMessage(s scanner) (Message, error) {
	var m Message
	var mentioned any

	err := s.Scan(
		&m.ID,
		&m.ConversationID,
		&m.ContactID,
		&m.Platform,
		&m.ExternalConversationID,
		&m.ExternalMessageID,
		&m.ExternalSenderID,
		&m.SentAt,
		&m.IsFromMe,
		&m.IsGroup,
		&m.Kind,
		&m.Body,
		&m.MimeType,
		&m.IsEdit,
		&m.EditTargetMessageID,
		&m.ContextStanzaID,
		&m.ContextParticipant,
		&m.ContextIsForwarded,
		&m.ContextForwardingScore,
		&mentioned,
		&m.IsEphemeral,
		&m.IsViewOnce,
		&m.MediaID,
		&m.MediaLocalPath,
		&m.MediaDescribeText,
		&m.MediaTranscriptText,
		&m.CreatedAt,
	)
	if err != nil {
		return Message{}, err
	}

	m.ContextMentionedIDs, err = scanStringSlice(mentioned)
	if err != nil {
		return Message{}, fmt.Errorf("scan context_mentioned_ids: %w", err)
	}

	return m, nil
}

type GetMessageParams struct {
	ID                string
	Platform          string
	ExternalMessageID string
}

func GetMessage(ctx context.Context, db *sql.DB, p GetMessageParams) (Message, error) {
	if p.ID == "" && (p.Platform == "" || p.ExternalMessageID == "") {
		return Message{}, fmt.Errorf("get message: no filter provided")
	}

	row := db.QueryRowContext(ctx, queries.MessageGet, p.ID, p.Platform, p.ExternalMessageID)
	return scanMessage(row)
}

func GetMessagesByConversation(ctx context.Context, db *sql.DB, conversationID, beforeID string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := db.QueryContext(ctx, queries.MessageList, conversationID, beforeID, limit)
	if err != nil {
		return nil, fmt.Errorf("get messages by conversation %s: %w", conversationID, err)
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		m, scanErr := scanMessage(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, m)
	}

	err = rows.Err()
	if err != nil {
		return nil, err
	}

	return out, nil
}

type InsertMessageParams struct {
	ID                     string
	ConversationID         string
	ContactID              string
	Platform               string
	ExternalConversationID string
	ExternalMessageID      string
	ExternalSenderID       string
	SentAt                 any
	IsFromMe               bool
	IsGroup                bool
	Kind                   string
	Body                   string
	MimeType               any
	IsEdit                 bool
	EditTargetMessageID    any
	ContextStanzaID        any
	ContextParticipant     any
	ContextIsForwarded     bool
	ContextForwardingScore any
	ContextMentionedIDs    any
	IsEphemeral            bool
	IsViewOnce             bool
}

func InsertMessage(ctx context.Context, db DBTX, p InsertMessageParams) error {
	_, err := db.ExecContext(ctx, queries.MessageInsert,
		p.ID,
		p.ConversationID,
		p.ContactID,
		p.Platform,
		p.ExternalConversationID,
		p.ExternalMessageID,
		p.ExternalSenderID,
		p.SentAt,
		p.IsFromMe,
		p.IsGroup,
		p.Kind,
		p.Body,
		p.MimeType,
		p.IsEdit,
		p.EditTargetMessageID,
		p.ContextStanzaID,
		p.ContextParticipant,
		p.ContextIsForwarded,
		p.ContextForwardingScore,
		p.ContextMentionedIDs,
		p.IsEphemeral,
		p.IsViewOnce,
	)
	if err != nil {
		return fmt.Errorf("insert message %s/%s: %w", p.Platform, p.ExternalMessageID, err)
	}

	return nil
}

func UpdateMessageBody(ctx context.Context, db *sql.DB, messageID, body string) error {
	_, err := db.ExecContext(ctx, queries.MessageUpdateBody, messageID, body)
	if err != nil {
		return fmt.Errorf("update message body %s: %w", messageID, err)
	}

	return nil
}
