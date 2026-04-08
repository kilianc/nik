package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

type ConversationUpsertParams struct {
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

type ConversationGetParams struct {
	ID                     string
	Platform               string
	ExternalConversationID string
	ContactID              string
}

type ConversationMarkReadParams struct {
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

func ConversationUpsert(ctx context.Context, db DBTX, p ConversationUpsertParams) error {
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

func ConversationGet(ctx context.Context, db DBTX, p ConversationGetParams) (Conversation, error) {
	if p.ID == "" && (p.Platform == "" || (p.ExternalConversationID == "" && p.ContactID == "")) {
		return Conversation{}, fmt.Errorf("get conversation: no filter provided")
	}

	row := db.QueryRowContext(ctx, queries.ConversationGet, p.ID, p.Platform, p.ExternalConversationID, p.ContactID)
	return scanConversation(row)
}

type ConversationUpdateParams struct {
	ID                     string
	ExternalConversationID string
	Title                  string
	LastMessageAt          *time.Time
}

func ConversationUpdate(ctx context.Context, db DBTX, p ConversationUpdateParams) error {
	title := any(nil)
	if p.Title != "" {
		title = p.Title
	}

	lastMessageAt := any(nil)
	if p.LastMessageAt != nil {
		lastMessageAt = *p.LastMessageAt
	}

	_, err := db.ExecContext(ctx, queries.ConversationUpdate,
		p.ID,
		p.ExternalConversationID,
		title,
		lastMessageAt,
	)
	if err != nil {
		return fmt.Errorf("update conversation %s: %w", p.ID, err)
	}

	return nil
}

func ConversationMarkRead(ctx context.Context, db *sql.DB, p ConversationMarkReadParams) error {
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

type ConversationParticipantUpsertParams struct {
	ConversationID string
	ContactID      string
	DisplayName    *string
}

func ConversationParticipantUpsert(ctx context.Context, db DBTX, p ConversationParticipantUpsertParams) error {
	if p.ContactID == "" {
		return fmt.Errorf("empty contact_id")
	}

	_, err := db.ExecContext(
		ctx,
		queries.ConversationParticipantUpsert,
		id.V7(),
		p.ConversationID,
		p.ContactID,
		p.DisplayName,
	)
	if err != nil {
		return fmt.Errorf("upsert conversation participant %s/%s: %w", p.ConversationID, p.ContactID, err)
	}

	return nil
}

func ConversationParticipantList(ctx context.Context, db *sql.DB, conversationID string) ([]ConversationParticipant, error) {
	rows, err := db.QueryContext(ctx, queries.ConversationParticipantList, conversationID)
	if err != nil {
		return nil, fmt.Errorf("get conversation participants %s: %w", conversationID, err)
	}
	defer rows.Close()

	var participants []ConversationParticipant
	for rows.Next() {
		var p ConversationParticipant
		err = rows.Scan(
			&p.ID,
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
