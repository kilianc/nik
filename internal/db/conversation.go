package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

type UpsertConversationParams struct {
	Platform               string
	ExternalConversationID string
	Kind                   string
	Title                  string
	Topic                  *string
	IsAnnounce             *bool
	IsLocked               *bool
	OwnerExternalID        *string
	ParticipantExternalIDs []string
	LastMessageAt          *time.Time
}

type GetConversationParams struct {
	ID                     string
	Platform               string
	ExternalConversationID string
}

type MarkConversationsReadParams struct {
	ConversationID string
	Platform       string
	ReadAt         time.Time
}

func scanConversation(s scanner) (Conversation, error) {
	var conv Conversation
	var participantExternalIDs any

	err := s.Scan(
		&conv.ID,
		&conv.Platform,
		&conv.ExternalConversationID,
		&conv.Kind,
		&conv.Title,
		&conv.Topic,
		&conv.IsAnnounce,
		&conv.IsLocked,
		&conv.OwnerExternalID,
		&participantExternalIDs,
		&conv.LastMessageAt,
		&conv.LastReadAt,
		&conv.CreatedAt,
		&conv.UpdatedAt,
	)
	if err != nil {
		return Conversation{}, err
	}

	conv.ParticipantExternalIDs, err = scanStringSlice(participantExternalIDs)
	if err != nil {
		return Conversation{}, fmt.Errorf("scan participant_external_ids: %w", err)
	}

	return conv, nil
}

func UpsertConversation(ctx context.Context, db DBTX, p UpsertConversationParams) error {
	if p.Platform == "" {
		return fmt.Errorf("empty platform")
	}
	if p.ExternalConversationID == "" {
		return fmt.Errorf("empty external_conversation_id")
	}
	if p.Kind == "" {
		p.Kind = "dm"
	}

	title := any(nil)
	if p.Title != "" {
		title = p.Title
	}

	lastMessageAt := any(nil)
	if p.LastMessageAt != nil {
		lastMessageAt = *p.LastMessageAt
	}

	topic := any(nil)
	if p.Topic != nil {
		topic = *p.Topic
	}

	isAnnounce := any(nil)
	if p.IsAnnounce != nil {
		isAnnounce = *p.IsAnnounce
	}

	isLocked := any(nil)
	if p.IsLocked != nil {
		isLocked = *p.IsLocked
	}

	ownerExternalID := any(nil)
	if p.OwnerExternalID != nil && *p.OwnerExternalID != "" {
		ownerExternalID = *p.OwnerExternalID
	}

	participantExternalIDs := any(nil)
	if p.ParticipantExternalIDs != nil {
		participantExternalIDs = MarshalStringSlice(p.ParticipantExternalIDs)
	}

	_, err := db.ExecContext(
		ctx,
		queries.ConversationUpsert,
		id.V7(),
		p.Platform,
		p.ExternalConversationID,
		p.Kind,
		title,
		lastMessageAt,
		topic,
		isAnnounce,
		isLocked,
		ownerExternalID,
		participantExternalIDs,
	)
	if err != nil {
		return fmt.Errorf("upsert conversation %s/%s: %w", p.Platform, p.ExternalConversationID, err)
	}

	return nil
}

func GetConversation(ctx context.Context, db DBTX, p GetConversationParams) (Conversation, error) {
	if p.ID == "" && (p.Platform == "" || p.ExternalConversationID == "") {
		return Conversation{}, fmt.Errorf("get conversation: no filter provided")
	}

	row := db.QueryRowContext(ctx, queries.ConversationGet, p.ID, p.Platform, p.ExternalConversationID)
	return scanConversation(row)
}

func MarkConversationsRead(ctx context.Context, db *sql.DB, p MarkConversationsReadParams) error {
	if p.ConversationID != "" {
		_, err := db.ExecContext(ctx, queries.ConversationMarkRead, p.ConversationID, p.ReadAt)
		if err != nil {
			return fmt.Errorf("mark conversation read %s: %w", p.ConversationID, err)
		}

		return nil
	}

	if p.Platform != "" {
		_, err := db.ExecContext(ctx, queries.ConversationMarkAllRead, p.Platform)
		if err != nil {
			return fmt.Errorf("mark all conversations read for %s: %w", p.Platform, err)
		}

		return nil
	}

	return fmt.Errorf("mark conversations read: no filter provided")
}

func UpsertConversationParticipant(ctx context.Context, db DBTX, conversationID, contactID string, displayName *string) error {
	if contactID == "" {
		return fmt.Errorf("empty contact_id")
	}

	_, err := db.ExecContext(
		ctx,
		queries.ConversationUpsertParticipant,
		conversationID,
		contactID,
		displayName,
	)
	if err != nil {
		return fmt.Errorf("upsert conversation participant %s/%s: %w", conversationID, contactID, err)
	}

	return nil
}

func GetConversationParticipants(ctx context.Context, db *sql.DB, conversationID string) ([]ConversationParticipant, error) {
	rows, err := db.QueryContext(ctx, queries.ConversationGetParticipants, conversationID)
	if err != nil {
		return nil, fmt.Errorf("get conversation participants %s: %w", conversationID, err)
	}
	defer rows.Close()

	var participants []ConversationParticipant
	for rows.Next() {
		var p ConversationParticipant
		err = rows.Scan(
			&p.ContactID,
			&p.DisplayName,
			&p.ContactName,
			&p.Timezone,
			&p.Location,
			&p.OneLiner,
		)
		if err != nil {
			return nil, fmt.Errorf("scan conversation participant: %w", err)
		}

		participants = append(participants, p)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate conversation participants: %w", err)
	}

	return participants, nil
}
