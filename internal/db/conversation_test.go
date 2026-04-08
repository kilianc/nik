package db

import (
	"context"
	"testing"
	"time"
)

func TestConversationGroupMetadataRoundTrip(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	now := time.Now()
	err = ConversationUpsert(ctx, conn, ConversationUpsertParams{
		Platform:               "whatsapp",
		ExternalConversationID: "12345-67890@g.us",
		Kind:                   "group",
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("upsert conversation: %v", err)
	}

	title := "Family"
	topic := "dinner plans"
	isAnnounce := true
	isLocked := true
	owner := "11111@s.whatsapp.net"
	participants := []string{"11111@s.whatsapp.net", "22222@s.whatsapp.net"}

	err = ConversationUpsert(ctx, conn, ConversationUpsertParams{
		Platform:               "whatsapp",
		ExternalConversationID: "12345-67890@g.us",
		Kind:                   "group",
		Title:                  title,
		Topic:                  &topic,
		IsAnnounce:             &isAnnounce,
		IsLocked:               &isLocked,
		OwnerExternalID:        &owner,
		ParticipantExternalIDs: participants,
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("upsert conversation group metadata: %v", err)
	}

	conv, err := ConversationGet(ctx, conn, ConversationGetParams{
		Platform:               "whatsapp",
		ExternalConversationID: "12345-67890@g.us",
	})
	if err != nil {
		t.Fatalf("get conversation by external: %v", err)
	}

	if !conv.Topic.Valid || conv.Topic.String != topic {
		t.Fatalf("expected topic %q, got %+v", topic, conv.Topic)
	}
	if !conv.OwnerExternalID.Valid || conv.OwnerExternalID.String != owner {
		t.Fatalf("expected owner %q, got %+v", owner, conv.OwnerExternalID)
	}
	if !conv.IsAnnounce {
		t.Fatalf("expected is_announce=true")
	}
	if !conv.IsLocked {
		t.Fatalf("expected is_locked=true")
	}
	if len(conv.ParticipantExternalIDs) != 2 {
		t.Fatalf("expected 2 participant ids, got %d", len(conv.ParticipantExternalIDs))
	}
}

func TestConversationParticipantListIncludesContactProfile(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contact, err := ContactUpsert(ctx, conn, ContactUpsertParams{
		Platform:      "whatsapp",
		ExternalID:    "alice@s.whatsapp.net",
		Name:          "Alice",
		Phone:         "alice",
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("upsert contact: %v", err)
	}

	now := time.Now()
	err = ConversationUpsert(ctx, conn, ConversationUpsertParams{
		Platform:               "whatsapp",
		ExternalConversationID: "alice@s.whatsapp.net",
		Kind:                   "dm",
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("upsert conversation: %v", err)
	}

	conversation, err := ConversationGet(ctx, conn, ConversationGetParams{
		Platform:               "whatsapp",
		ExternalConversationID: "alice@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}
	conversationID := conversation.ID

	displayName := "Ali"
	err = ConversationParticipantUpsert(ctx, conn, ConversationParticipantUpsertParams{
		ConversationID: conversationID,
		ContactID:      contact.ID,
		DisplayName:    &displayName,
	})
	if err != nil {
		t.Fatalf("upsert conversation participant: %v", err)
	}

	participants, err := ConversationParticipantList(ctx, conn, conversationID)
	if err != nil {
		t.Fatalf("get conversation participants: %v", err)
	}

	if len(participants) != 1 {
		t.Fatalf("expected 1 participant, got %d", len(participants))
	}

	if !participants[0].ContactName.Valid || participants[0].ContactName.String != "Alice" {
		t.Fatalf("expected contact name Alice, got %+v", participants[0].ContactName)
	}
	if participants[0].ContactID != contact.ID {
		t.Fatalf("expected contact id %s, got %s", contact.ID, participants[0].ContactID)
	}
}

func TestConversationGetByContactID(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contact, err := ContactUpsert(ctx, conn, ContactUpsertParams{
		Platform:      "whatsapp",
		ExternalID:    "bob@lid",
		Name:          "Bob",
		Phone:         "bob",
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("upsert contact: %v", err)
	}

	convID := seedConversation(t, ctx, conn, "whatsapp", "bob@lid", "dm")

	err = ConversationParticipantUpsert(ctx, conn, ConversationParticipantUpsertParams{
		ConversationID: convID,
		ContactID:      contact.ID,
	})
	if err != nil {
		t.Fatalf("upsert participant: %v", err)
	}

	found, err := ConversationGet(ctx, conn, ConversationGetParams{
		Platform:  "whatsapp",
		ContactID: contact.ID,
	})
	if err != nil {
		t.Fatalf("get by contact id: %v", err)
	}
	if found.ID != convID {
		t.Fatalf("expected conversation %s, got %s", convID, found.ID)
	}

	_, err = ConversationGet(ctx, conn, ConversationGetParams{
		Platform:  "whatsapp",
		ContactID: "nonexistent-contact",
	})
	if err == nil {
		t.Fatalf("expected error for nonexistent contact")
	}
}

func TestConversationUpdate(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn, "whatsapp", "old-jid@lid", "dm")

	now := time.Now()
	err = ConversationUpdate(ctx, conn, ConversationUpdateParams{
		ID:                     convID,
		ExternalConversationID: "new-jid@s.whatsapp.net",
		Title:                  "Bob",
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("update conversation: %v", err)
	}

	conv, err := ConversationGet(ctx, conn, ConversationGetParams{ID: convID})
	if err != nil {
		t.Fatalf("get updated conversation: %v", err)
	}

	if conv.ExternalConversationID != "new-jid@s.whatsapp.net" {
		t.Fatalf("expected external_conversation_id new-jid@s.whatsapp.net, got %s", conv.ExternalConversationID)
	}
	if !conv.Title.Valid || conv.Title.String != "Bob" {
		t.Fatalf("expected title Bob, got %+v", conv.Title)
	}
}

func TestConversationUpsertParticipantDeduplicatesByContactID(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contact, err := ContactUpsert(ctx, conn, ContactUpsertParams{
		Platform:      "whatsapp",
		ExternalID:    "alice@s.whatsapp.net",
		Name:          "Alice",
		Phone:         "alice",
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("upsert contact: %v", err)
	}

	conversationID := seedConversation(t, ctx, conn, "whatsapp", "group@g.us", "group")

	err = ConversationParticipantUpsert(ctx, conn, ConversationParticipantUpsertParams{
		ConversationID: conversationID,
		ContactID:      contact.ID,
	})
	if err != nil {
		t.Fatalf("upsert participant first: %v", err)
	}

	err = ConversationParticipantUpsert(ctx, conn, ConversationParticipantUpsertParams{
		ConversationID: conversationID,
		ContactID:      contact.ID,
	})
	if err != nil {
		t.Fatalf("upsert participant second: %v", err)
	}

	var rowID string
	err = conn.QueryRowContext(ctx,
		"SELECT id FROM conversation_participant WHERE conversation_id = ?1 AND contact_id = ?2",
		conversationID,
		contact.ID,
	).Scan(&rowID)
	if err != nil {
		t.Fatalf("query participant id: %v", err)
	}
	if rowID == "" {
		t.Fatalf("expected non-empty participant id")
	}

	participants, err := ConversationParticipantList(ctx, conn, conversationID)
	if err != nil {
		t.Fatalf("get participants: %v", err)
	}

	if len(participants) != 1 {
		t.Fatalf("expected 1 participant (dedup by contact_id), got %d", len(participants))
	}

	if participants[0].ContactID != contact.ID {
		t.Fatalf("expected contact id %s, got %s", contact.ID, participants[0].ContactID)
	}
	if participants[0].ID != rowID {
		t.Fatalf("expected participant id %s, got %s", rowID, participants[0].ID)
	}
}
