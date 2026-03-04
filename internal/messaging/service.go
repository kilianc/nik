package messaging

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/contacts"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

type ContactService interface {
	EnsureContactForMessage(ctx context.Context, platform string, externalIDs []string, isFromMe bool, at time.Time) (string, error)
}

type Service struct {
	cfg        *config.Config
	db         *sql.DB
	registry   *Registry
	contacts   ContactService
	replyDelay func(body string) time.Duration
}

func NewService(cfg *config.Config, conn *sql.DB, contacts ContactService) *Service {
	return &Service{
		cfg:        cfg,
		db:         conn,
		registry:   NewRegistry(),
		contacts:   contacts,
		replyDelay: humanizedReplyDelay,
	}
}

func (s *Service) RegisterPlatform(platform MessagingPlatform) {
	err := s.registry.Register(platform)
	if err != nil {
		panic(fmt.Sprintf("register platform: %v", err))
	}
}

func (s *Service) ReceiveConversation(ctx context.Context, conv Conversation) error {
	lastMessageAt := conv.LastMessageAt
	if lastMessageAt.IsZero() {
		lastMessageAt = time.Now()
	}

	err := db.UpsertConversation(ctx, s.db, db.UpsertConversationParams{
		Platform:               conv.Platform,
		ExternalConversationID: conv.ExternalConversationID,
		Kind:                   conv.Kind,
		Title:                  conv.Title,
		Topic:                  conv.Topic,
		IsAnnounce:             conv.IsAnnounce,
		IsLocked:               conv.IsLocked,
		OwnerExternalID:        conv.OwnerExternalID,
		ParticipantExternalIDs: conv.ParticipantExternalIDs,
		LastMessageAt:          &lastMessageAt,
	})
	if err != nil {
		return fmt.Errorf("receive conversation: %w", err)
	}

	return nil
}

func (s *Service) ReceiveMessage(ctx context.Context, msg InboundMessage) error {
	sentAt := msg.SentAt
	if sentAt.IsZero() {
		sentAt = time.Now()
	}

	if msg.ExternalSenderID == "" {
		return fmt.Errorf("empty external_sender_id")
	}

	if s.contacts == nil {
		return fmt.Errorf("contact service not configured")
	}

	externalIDs := msg.ExternalSenderIDs
	if len(externalIDs) == 0 {
		externalIDs = []string{msg.ExternalSenderID}
	}

	contactID, err := s.contacts.EnsureContactForMessage(ctx, msg.Platform, externalIDs, msg.IsFromMe, sentAt)
	if err != nil {
		return fmt.Errorf("resolve contact: %w", err)
	}
	if contactID == "" {
		return fmt.Errorf("resolve contact: empty contact id")
	}

	mimeType := nullable(msg.MimeType)
	editTarget := nullable(msg.EditTargetMessageID)
	contextStanza := nullable(msg.ContextStanzaID)
	contextParticipant := nullable(msg.ContextParticipant)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin receive message tx: %w", err)
	}
	defer tx.Rollback()

	err = db.UpsertConversation(ctx, tx, db.UpsertConversationParams{
		Platform:               msg.Platform,
		ExternalConversationID: msg.ExternalConversationID,
		Kind:                   inferConversationKind(msg),
		Title:                  "",
		LastMessageAt:          &sentAt,
	})
	if err != nil {
		return fmt.Errorf("upsert message conversation: %w", err)
	}

	conversation, err := db.GetConversation(ctx, tx, db.GetConversationParams{
		Platform:               msg.Platform,
		ExternalConversationID: msg.ExternalConversationID,
	})
	if err != nil {
		return err
	}
	conversationID := conversation.ID

	kind := msg.Kind
	if kind == "" {
		kind = "text"
	}

	msgID := id.V7()

	logAttrs := []any{
		"pkg", "messaging",
		"platform", msg.Platform,
		"conversation_id", conversationID,
		"message_id", msgID,
		"kind", kind,
		"is_from_me", msg.IsFromMe,
		"is_edit", msg.IsEdit,
	}
	if msg.MimeType != "" {
		logAttrs = append(logAttrs, "attachment", msg.MimeType)
	}
	slog.Info("receive inbound message", logAttrs...)

	_, err = tx.ExecContext(
		ctx,
		queries.MessageInsert,
		msgID,
		conversationID,
		contactID,
		msg.Platform,
		msg.ExternalConversationID,
		msg.ExternalMessageID,
		msg.ExternalSenderID,
		sentAt,
		msg.IsFromMe,
		msg.IsGroup,
		kind,
		msg.Body,
		mimeType,
		msg.IsEdit,
		editTarget,
		contextStanza,
		contextParticipant,
		msg.ContextIsForwarded,
		msg.ContextForwardingScore,
		db.MarshalStringSlice(msg.ContextMentionedIDs),
		msg.IsEphemeral,
		msg.IsViewOnce,
	)
	if err != nil {
		return fmt.Errorf("insert message %s/%s: %w", msg.Platform, msg.ExternalMessageID, err)
	}

	err = db.UpsertConversationParticipant(ctx, tx, conversationID, contactID, nil)
	if err != nil {
		return err
	}

	if msg.MediaHash != "" {
		localPath := nullable(msg.LocalPath)
		var sizeBytes *int64
		if msg.MediaSizeBytes > 0 {
			sizeBytes = &msg.MediaSizeBytes
		}

		err = db.UpsertMedia(ctx, tx, db.UpsertMediaParams{
			ID:        msg.MediaHash,
			MimeType:  mimeType,
			LocalPath: localPath,
			SizeBytes: sizeBytes,
		})
		if err != nil {
			return err
		}

		err = db.UpsertMessageMedia(ctx, tx, msgID, msg.MediaHash)
		if err != nil {
			return err
		}
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("commit receive message tx: %w", err)
	}

	return nil
}

func (s *Service) OnHistorySyncComplete(ctx context.Context, platform string) error {
	return db.MarkConversationsRead(ctx, s.db, db.MarkConversationsReadParams{
		Platform: platform,
	})
}

func (s *Service) Reply(ctx context.Context, conversationID string, body string) error {
	conv, err := db.GetConversation(ctx, s.db, db.GetConversationParams{ID: conversationID})
	if err != nil {
		return err
	}

	platform, err := s.registry.Get(conv.Platform)
	if err != nil {
		return err
	}

	_ = platform.StartTyping(ctx, conv.ExternalConversationID)

	delayFn := s.replyDelay
	if delayFn == nil {
		delayFn = humanizedReplyDelay
	}

	delay := delayFn(body)
	if delay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}

	outbound, err := platform.Reply(ctx, conv.ExternalConversationID, body)
	_ = platform.StopTyping(ctx, conv.ExternalConversationID)
	if err != nil {
		return err
	}

	if outbound.ExternalMessageID == "" {
		return fmt.Errorf("platform reply missing external_message_id")
	}

	if outbound.ExternalSenderID == "" {
		return fmt.Errorf("platform reply missing external_sender_id")
	}

	sentAt := outbound.SentAt
	if sentAt.IsZero() {
		sentAt = time.Now()
	}

	kind := outbound.Kind
	if kind == "" {
		kind = "text"
	}

	replyBody := outbound.Body
	if replyBody == "" {
		replyBody = body
	}

	mimeType := outbound.MimeType

	return s.ReceiveMessage(ctx, InboundMessage{
		Platform:               conv.Platform,
		ExternalConversationID: conv.ExternalConversationID,
		ExternalMessageID:      outbound.ExternalMessageID,
		ExternalSenderID:       outbound.ExternalSenderID,
		Kind:                   kind,
		Body:                   replyBody,
		MimeType:               mimeType,
		SentAt:                 sentAt,
		IsFromMe:               true,
		IsGroup:                conv.Kind == "group",
	})
}

func (s *Service) SendImage(ctx context.Context, conversationID string, imagePath string, caption string) error {
	conv, err := db.GetConversation(ctx, s.db, db.GetConversationParams{ID: conversationID})
	if err != nil {
		return err
	}

	platform, err := s.registry.Get(conv.Platform)
	if err != nil {
		return err
	}

	_ = platform.StartTyping(ctx, conv.ExternalConversationID)

	delayFn := s.replyDelay
	if delayFn == nil {
		delayFn = humanizedReplyDelay
	}

	delay := delayFn(caption)
	if delay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}

	outbound, err := platform.SendImage(ctx, conv.ExternalConversationID, imagePath, caption)
	_ = platform.StopTyping(ctx, conv.ExternalConversationID)
	if err != nil {
		return err
	}

	if outbound.ExternalMessageID == "" {
		return fmt.Errorf("platform send image missing external_message_id")
	}

	if outbound.ExternalSenderID == "" {
		return fmt.Errorf("platform send image missing external_sender_id")
	}

	sentAt := outbound.SentAt
	if sentAt.IsZero() {
		sentAt = time.Now()
	}

	kind := outbound.Kind
	if kind == "" {
		kind = "image"
	}

	return s.ReceiveMessage(ctx, InboundMessage{
		Platform:               conv.Platform,
		ExternalConversationID: conv.ExternalConversationID,
		ExternalMessageID:      outbound.ExternalMessageID,
		ExternalSenderID:       outbound.ExternalSenderID,
		Kind:                   kind,
		Body:                   caption,
		MimeType:               outbound.MimeType,
		SentAt:                 sentAt,
		IsFromMe:               true,
		IsGroup:                conv.Kind == "group",
		LocalPath:              outbound.LocalPath,
		MediaHash:              mediaHashFromPath(imagePath),
		MediaSizeBytes:         fileSize(imagePath),
	})
}

func (s *Service) React(ctx context.Context, messageID string, emoji string) error {
	msg, err := db.GetMessage(ctx, s.db, db.GetMessageParams{ID: messageID})
	if err != nil {
		return err
	}

	platform, err := s.registry.Get(msg.Platform)
	if err != nil {
		return err
	}

	return platform.React(ctx, msg.ExternalConversationID, msg.ExternalMessageID, msg.ExternalSenderID, emoji)
}

func (s *Service) SetPresence(ctx context.Context, platformName string, available bool) error {
	platform, err := s.registry.Get(platformName)
	if err != nil {
		return err
	}

	return platform.SetPresence(ctx, available)
}

func (s *Service) MarkRead(ctx context.Context, conversationID string, readAt time.Time) error {
	if readAt.IsZero() {
		return nil
	}

	conv, err := db.GetConversation(ctx, s.db, db.GetConversationParams{ID: conversationID})
	if err != nil {
		return err
	}

	err = db.MarkConversationsRead(ctx, s.db, db.MarkConversationsReadParams{
		ConversationID: conversationID,
		ReadAt:         readAt,
	})
	if err != nil {
		return err
	}

	msgs, err := db.GetMessagesByConversation(ctx, s.db, conversationID, "", 50)
	if err != nil {
		return err
	}

	var unread []InboundMessage
	for _, msg := range msgs {
		if msg.IsFromMe {
			continue
		}
		if conv.LastReadAt.Valid && !msg.SentAt.After(conv.LastReadAt.Time) {
			continue
		}
		if msg.SentAt.After(readAt) {
			continue
		}

		unread = append(unread, InboundMessage{
			Platform:               msg.Platform,
			ExternalConversationID: msg.ExternalConversationID,
			ExternalMessageID:      msg.ExternalMessageID,
			ExternalSenderID:       msg.ExternalSenderID,
			SentAt:                 msg.SentAt,
		})
	}

	if len(unread) == 0 {
		return nil
	}

	platform, err := s.registry.Get(conv.Platform)
	if err != nil {
		return err
	}

	err = platform.MarkRead(ctx, unread)
	if err != nil {
		return err
	}

	return nil
}

func (s *Service) UpdateMediaDescription(ctx context.Context, messageID, description, body string) error {
	if description == "" {
		return fmt.Errorf("empty description")
	}

	msg, err := db.GetMessage(ctx, s.db, db.GetMessageParams{ID: messageID})
	if err != nil {
		return err
	}

	if !msg.MediaID.Valid {
		return fmt.Errorf("message %s has no media", messageID)
	}

	now := time.Now()
	result, err := s.db.ExecContext(ctx, queries.MediaUpdateDescription, description, now, msg.MediaID.String)
	if err != nil {
		return fmt.Errorf("update media description %s: %w", msg.MediaID.String, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected for media %s: %w", msg.MediaID.String, err)
	}
	if rows == 0 {
		return fmt.Errorf("media %s not found", msg.MediaID.String)
	}

	if body != "" {
		err = db.UpdateMessageBody(ctx, s.db, messageID, body)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) PollUnreadConversationIDs(ctx context.Context) ([]string, error) {
	return db.PollUnreadConversations(ctx, s.db, s.cfg.AllowConversationIDs)
}

func (s *Service) ConversationWithMessages(ctx context.Context, conversationID string, maxHistory int) (db.Conversation, []db.Message, error) {
	conv, err := db.GetConversation(ctx, s.db, db.GetConversationParams{ID: conversationID})
	if err != nil {
		return db.Conversation{}, nil, err
	}

	msgs, err := db.GetMessagesByConversation(ctx, s.db, conversationID, "", maxHistory)
	if err != nil {
		return db.Conversation{}, nil, err
	}

	reverseMessages(msgs)
	return conv, msgs, nil
}

func (s *Service) MessagesAround(ctx context.Context, conversationID string, pivot time.Time, limit int) ([]db.Message, error) {
	return db.GetMessagesAround(ctx, s.db, conversationID, pivot, limit)
}

func (s *Service) ResolveConversation(ctx context.Context, contactID string) (string, error) {
	contact, err := db.GetContact(ctx, s.db, contactID)
	if err != nil {
		return "", fmt.Errorf("get contact %s: %w", contactID, err)
	}

	if len(contact.WhatsappIDs) == 0 {
		return "", fmt.Errorf("contact %s has no whatsapp id", contactID)
	}
	jid := contact.WhatsappIDs[0]

	now := time.Now()
	err = db.UpsertConversation(ctx, s.db, db.UpsertConversationParams{
		Platform:               "whatsapp",
		ExternalConversationID: jid,
		Kind:                   "dm",
		LastMessageAt:          &now,
	})
	if err != nil {
		return "", fmt.Errorf("upsert conversation for contact %s: %w", contactID, err)
	}

	conv, err := db.GetConversation(ctx, s.db, db.GetConversationParams{
		Platform:               "whatsapp",
		ExternalConversationID: jid,
	})
	if err != nil {
		return "", fmt.Errorf("get conversation for contact %s: %w", contactID, err)
	}

	err = db.UpsertConversationParticipant(ctx, s.db, conv.ID, contactID, nil)
	if err != nil {
		return "", fmt.Errorf("link participant for contact %s: %w", contactID, err)
	}

	return conv.ID, nil
}

func (s *Service) ConversationIDFromExternal(ctx context.Context, platform, externalConversationID string) (string, error) {
	conversation, err := db.GetConversation(ctx, s.db, db.GetConversationParams{
		Platform:               platform,
		ExternalConversationID: externalConversationID,
	})
	if err != nil {
		return "", err
	}

	return conversation.ID, nil
}

func (s *Service) SessionContext(ctx context.Context, conv db.Conversation) SessionContext {
	participants, err := db.GetConversationParticipants(ctx, s.db, conv.ID)
	if err != nil || len(participants) == 0 {
		participants = nil
	}

	title := sessionTitle(conv, participants)

	session := SessionContext{
		Lines: []string{
			fmt.Sprintf("Conversation: %s", conv.ID),
			fmt.Sprintf("Title: %s", title),
			fmt.Sprintf("Platform: %s", conv.Platform),
			fmt.Sprintf("Type: %s", conv.Kind),
		},
	}

	if conv.Topic.Valid && strings.TrimSpace(conv.Topic.String) != "" {
		session.Lines = append(session.Lines, fmt.Sprintf("Topic: %s", conv.Topic.String))
	}

	if len(participants) > 0 {
		session.Lines = append(session.Lines, "", "Participants:")
		for i, p := range participants {
			fallbackName := fmt.Sprintf("participant-%d", i+1)
			name := participantName(p, fallbackName)
			session.Lines = append(session.Lines, fmt.Sprintf("- %s (%s)", name, p.ContactID))

			detail := participantDetail(p)
			if detail != "" {
				session.Lines = append(session.Lines, fmt.Sprintf("  %s", detail))
			}
		}
	}

	return session
}

func sessionTitle(conv db.Conversation, participants []db.ConversationParticipant) string {
	if conv.Title.Valid && strings.TrimSpace(conv.Title.String) != "" {
		return conv.Title.String
	}

	if conv.Kind == "dm" {
		for _, p := range participants {
			if p.ContactID == contacts.NikContactID {
				continue
			}
			name := participantName(p, "someone")
			return fmt.Sprintf("You and %s private DMs", name)
		}
	}

	return conv.Kind
}

func participantDetail(p db.ConversationParticipant) string {
	var parts []string

	tz := ""
	if p.Timezone.Valid {
		tz = strings.TrimSpace(p.Timezone.String)
	}

	loc := ""
	if p.Location.Valid {
		loc = strings.TrimSpace(p.Location.String)
	}

	if tz != "" || loc != "" {
		locPart := strings.Join(nonEmpty(tz, loc), ", ")
		parts = append(parts, locPart)
	}

	if p.OneLiner.Valid && strings.TrimSpace(p.OneLiner.String) != "" {
		parts = append(parts, strings.TrimSpace(p.OneLiner.String))
	}

	return strings.Join(parts, " — ")
}

func nonEmpty(ss ...string) []string {
	var out []string
	for _, s := range ss {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func (s *Service) SenderLabels(ctx context.Context, msgs []db.Message) map[string]string {
	byContactID := make(map[string]string)
	byMessageID := make(map[string]string)

	for _, msg := range msgs {
		if msg.ContactID == "" {
			if msg.IsFromMe {
				byMessageID[msg.ID] = "nik"
				continue
			}

			byMessageID[msg.ID] = "[unknown]"
			continue
		}

		label, ok := byContactID[msg.ContactID]
		if !ok {
			label = s.ContactLabel(ctx, msg.ContactID)
			if label == "" {
				if msg.IsFromMe {
					label = "nik"
				} else {
					label = "[unknown]"
				}
			}

			byContactID[msg.ContactID] = label
		}

		byMessageID[msg.ID] = label
	}

	return byMessageID
}

func (s *Service) ContactLabel(ctx context.Context, contactID string) string {
	if strings.TrimSpace(contactID) == "" {
		return ""
	}

	contact, err := db.GetContact(ctx, s.db, contactID)
	if err != nil {
		return ""
	}

	label := strings.TrimSpace(contact.Name)
	if label != "" {
		return label
	}

	if contactID == contacts.NikContactID {
		return "nik"
	}

	return ""
}

func inferConversationKind(msg InboundMessage) string {
	if msg.IsGroup {
		return "group"
	}
	return "dm"
}

func humanizedReplyDelay(body string) time.Duration {
	runes := utf8.RuneCountInString(strings.TrimSpace(body))
	if runes <= 0 {
		return 500 * time.Millisecond
	}

	delay := time.Duration(runes*35) * time.Millisecond
	if delay < 700*time.Millisecond {
		return 700 * time.Millisecond
	}
	if delay > 4*time.Second {
		return 4 * time.Second
	}

	return delay
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func reverseMessages(messages []db.Message) {
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
}

func participantName(p db.ConversationParticipant, fallback string) string {
	name := strings.TrimSpace(p.ContactName.String)
	if name != "" {
		return name
	}

	name = strings.TrimSpace(p.DisplayName.String)
	if name != "" {
		return name
	}

	if p.ContactID == contacts.NikContactID {
		return "nik"
	}

	return fallback
}

func previewForLog(s string, maxRunes int) string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" || maxRunes <= 0 {
		return ""
	}

	if utf8.RuneCountInString(trimmed) <= maxRunes {
		return trimmed
	}

	runes := []rune(trimmed)
	return string(runes[:maxRunes])
}

// the snippet is matched against formatted message lines (the same lines
// the LLM sees in the prompt), so any substring the LLM copies will match.
func (s *Service) FindMessage(ctx context.Context, conversationID, text string) (db.Message, error) {
	msgs, err := db.GetMessagesByConversation(ctx, s.db, conversationID, "", 200)
	if err != nil {
		return db.Message{}, fmt.Errorf("get messages: %w", err)
	}

	labels := s.SenderLabels(ctx, msgs)

	var matches []db.Message
	for _, msg := range msgs {
		line := FormatMessageLine(msg, labels[msg.ID])
		if strings.Contains(line, text) {
			matches = append(matches, msg)
		}
	}

	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) == 0 {
		return db.Message{}, fmt.Errorf("no message matching %q", text)
	}

	// if all matches produce identical formatted lines (same sender, same
	// second, same text), pick the most recent -- they are indistinguishable
	// to the LLM and the target doesn't matter
	allSame := true
	firstLine := FormatMessageLine(matches[0], labels[matches[0].ID])
	for _, m := range matches[1:] {
		if FormatMessageLine(m, labels[m.ID]) != firstLine {
			allSame = false
			break
		}
	}
	if allSame {
		return matches[len(matches)-1], nil
	}

	return db.Message{}, fmt.Errorf("%d messages match %q, quote more text or include sender", len(matches), text)
}

func mediaHashFromPath(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}

	return info.Size()
}
