package messaging

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/contacts"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
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
	speechFn   func(ctx context.Context, text string) (string, error)
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

func (s *Service) SetSpeechFn(fn func(ctx context.Context, text string) (string, error)) {
	s.speechFn = fn
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

	err := db.ConversationUpsert(ctx, s.db, db.ConversationUpsertParams{
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

	var dmRecipientID string
	var dmTitle string
	if !msg.IsGroup && msg.ExternalConversationID != "" {
		recipient, err := db.ContactGet(ctx, s.db, msg.ExternalConversationID)
		if err == nil {
			dmTitle = recipient.Name
			if recipient.ID != contactID {
				dmRecipientID = recipient.ID
			}
		}
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

	err = db.ConversationUpsert(ctx, tx, db.ConversationUpsertParams{
		Platform:               msg.Platform,
		ExternalConversationID: msg.ExternalConversationID,
		Kind:                   inferConversationKind(msg),
		Title:                  dmTitle,
		LastMessageAt:          &sentAt,
	})
	if err != nil {
		return fmt.Errorf("upsert message conversation: %w", err)
	}

	conversation, err := db.ConversationGet(ctx, tx, db.ConversationGetParams{
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

	err = db.MessageInsert(ctx, tx, db.MessageInsertParams{
		ID:                     msgID,
		ConversationID:         conversationID,
		ContactID:              contactID,
		Platform:               msg.Platform,
		ExternalConversationID: msg.ExternalConversationID,
		ExternalMessageID:      msg.ExternalMessageID,
		ExternalSenderID:       msg.ExternalSenderID,
		SentAt:                 sentAt,
		IsFromMe:               msg.IsFromMe,
		IsGroup:                msg.IsGroup,
		Kind:                   kind,
		Body:                   msg.Body,
		MimeType:               mimeType,
		IsEdit:                 msg.IsEdit,
		EditTargetMessageID:    editTarget,
		ContextStanzaID:        contextStanza,
		ContextParticipant:     contextParticipant,
		ContextIsForwarded:     msg.ContextIsForwarded,
		ContextForwardingScore: msg.ContextForwardingScore,
		ContextMentionedIDs:    db.MarshalStringSlice(msg.ContextMentionedIDs),
		IsEphemeral:            msg.IsEphemeral,
		IsViewOnce:             msg.IsViewOnce,
	})
	if err != nil {
		return err
	}

	err = db.ConversationParticipantUpsert(ctx, tx, db.ConversationParticipantUpsertParams{
		ConversationID: conversationID,
		ContactID:      contactID,
	})
	if err != nil {
		return err
	}

	if dmRecipientID != "" {
		err = db.ConversationParticipantUpsert(ctx, tx, db.ConversationParticipantUpsertParams{
			ConversationID: conversationID,
			ContactID:      dmRecipientID,
		})
		if err != nil {
			return err
		}
	}

	if msg.MediaID != "" {
		localPath := nullable(msg.LocalPath)
		var sizeBytes *int64
		if msg.MediaSizeBytes > 0 {
			sizeBytes = &msg.MediaSizeBytes
		}

		transcriptText := nullable(msg.MediaTranscriptText)
		var transcribedAt *time.Time
		if msg.MediaTranscriptText != "" {
			now := time.Now()
			transcribedAt = &now
		}

		err = db.MediaInsert(ctx, tx, db.MediaInsertParams{
			ID:             msg.MediaID,
			MimeType:       mimeType,
			LocalPath:      localPath,
			SizeBytes:      sizeBytes,
			TranscriptText: transcriptText,
			TranscribedAt:  transcribedAt,
		})
		if err != nil {
			return err
		}

		err = db.MessageMediaUpsert(ctx, tx, msgID, msg.MediaID)
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
	return db.ConversationMarkRead(ctx, s.db, db.ConversationMarkReadParams{
		Platform: platform,
	})
}

func (s *Service) checkBannedWords(text string) error {
	if s.cfg == nil || len(s.cfg.BannedWords) == 0 {
		return nil
	}

	lower := strings.ToLower(text)
	for _, word := range s.cfg.BannedWords {
		if strings.Contains(lower, strings.ToLower(word)) {
			return fmt.Errorf("message contains banned word, rephrase without it")
		}
	}

	return nil
}

func (s *Service) Reply(ctx context.Context, conversationID string, body string, quote *QuoteTarget) error {
	err := s.checkBannedWords(body)
	if err != nil {
		return err
	}

	conv, err := db.ConversationGet(ctx, s.db, db.ConversationGetParams{ID: conversationID})
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
		timer := time.NewTimer(delay)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}

	outbound, err := platform.Reply(ctx, conv.ExternalConversationID, body, quote)
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

	echo := InboundMessage{
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
	}

	if quote != nil {
		echo.ContextStanzaID = quote.ExternalMessageID
		echo.ContextParticipant = quote.ExternalSenderID
	}

	return s.ReceiveMessage(ctx, echo)
}

func (s *Service) SendFile(ctx context.Context, conversationID string, filePath string, caption string) error {
	err := s.checkBannedWords(caption)
	if err != nil {
		return err
	}

	conv, err := db.ConversationGet(ctx, s.db, db.ConversationGetParams{ID: conversationID})
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
		timer := time.NewTimer(delay)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}

	outbound, err := platform.SendFile(ctx, conv.ExternalConversationID, filePath, caption)
	_ = platform.StopTyping(ctx, conv.ExternalConversationID)
	if err != nil {
		return err
	}

	if outbound.ExternalMessageID == "" {
		return fmt.Errorf("platform send file missing external_message_id")
	}

	if outbound.ExternalSenderID == "" {
		return fmt.Errorf("platform send file missing external_sender_id")
	}

	sentAt := outbound.SentAt
	if sentAt.IsZero() {
		sentAt = time.Now()
	}

	kind := outbound.Kind
	if kind == "" {
		kind = "document"
	}

	mediaID := id.V7()
	ext := filepath.Ext(filePath)
	datePrefix := time.Now().Format("2006/01")
	mediaDir := filepath.Join(s.cfg.MediaPath(), datePrefix)
	localPath := ""

	mkErr := os.MkdirAll(mediaDir, 0o755)
	if mkErr == nil {
		mediaFile := filepath.Join(mediaDir, mediaID+ext)
		cpErr := copyFile(filePath, mediaFile)
		if cpErr != nil {
			slog.Warn("copy outbound file to media dir", "pkg", "messaging", "error", cpErr)
		} else {
			localPath = filepath.Join("media", datePrefix, mediaID+ext)
		}
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
		LocalPath:              localPath,
		MediaID:                mediaID,
		MediaSizeBytes:         fileSize(filePath),
	})
}

func (s *Service) SendVoiceNote(ctx context.Context, conversationID string, audioPath string, body string) error {
	conv, err := db.ConversationGet(ctx, s.db, db.ConversationGetParams{ID: conversationID})
	if err != nil {
		return err
	}

	platform, err := s.registry.Get(conv.Platform)
	if err != nil {
		return err
	}

	mediaID := id.V7()
	ext := filepath.Ext(audioPath)
	datePrefix := time.Now().Format("2006/01")
	mediaDir := filepath.Join(s.cfg.MediaPath(), datePrefix)
	localPath := ""

	mkErr := os.MkdirAll(mediaDir, 0o755)
	if mkErr == nil {
		mediaFile := filepath.Join(mediaDir, mediaID+ext)
		cpErr := copyFile(audioPath, mediaFile)
		if cpErr != nil {
			slog.Warn("copy outbound voice note to media dir", "pkg", "messaging", "error", cpErr)
		} else {
			localPath = filepath.Join("media", datePrefix, mediaID+ext)
		}
	}

	outbound, err := platform.SendVoiceNote(ctx, conv.ExternalConversationID, audioPath)
	if err != nil {
		return err
	}

	if outbound.ExternalMessageID == "" {
		return fmt.Errorf("platform send voice note missing external_message_id")
	}

	if outbound.ExternalSenderID == "" {
		return fmt.Errorf("platform send voice note missing external_sender_id")
	}

	sentAt := outbound.SentAt
	if sentAt.IsZero() {
		sentAt = time.Now()
	}

	kind := outbound.Kind
	if kind == "" {
		kind = "audio"
	}

	return s.ReceiveMessage(ctx, InboundMessage{
		Platform:               conv.Platform,
		ExternalConversationID: conv.ExternalConversationID,
		ExternalMessageID:      outbound.ExternalMessageID,
		ExternalSenderID:       outbound.ExternalSenderID,
		Kind:                   kind,
		Body:                   body,
		MimeType:               outbound.MimeType,
		SentAt:                 sentAt,
		IsFromMe:               true,
		IsGroup:                conv.Kind == "group",
		LocalPath:              localPath,
		MediaID:                mediaID,
		MediaSizeBytes:         fileSize(audioPath),
		MediaTranscriptText:    body,
	})
}

func (s *Service) React(ctx context.Context, messageID string, emoji string) error {
	msg, err := db.MessageGet(ctx, s.db, db.MessageGetParams{ID: messageID})
	if err != nil {
		return err
	}

	conv, err := db.ConversationGet(ctx, s.db, db.ConversationGetParams{ID: msg.ConversationID})
	if err != nil {
		return err
	}

	platform, err := s.registry.Get(msg.Platform)
	if err != nil {
		return err
	}

	outbound, err := platform.React(ctx, msg.ExternalConversationID, msg.ExternalMessageID, msg.ExternalSenderID, emoji)
	if err != nil {
		return err
	}

	if outbound.ExternalMessageID == "" {
		return nil
	}

	sentAt := outbound.SentAt
	if sentAt.IsZero() {
		sentAt = time.Now()
	}

	return s.ReceiveMessage(ctx, InboundMessage{
		Platform:               conv.Platform,
		ExternalConversationID: conv.ExternalConversationID,
		ExternalMessageID:      outbound.ExternalMessageID,
		ExternalSenderID:       outbound.ExternalSenderID,
		Kind:                   "reaction",
		Body:                   emoji,
		SentAt:                 sentAt,
		IsFromMe:               true,
		IsGroup:                conv.Kind == "group",
	})
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

	conv, err := db.ConversationGet(ctx, s.db, db.ConversationGetParams{ID: conversationID})
	if err != nil {
		return err
	}

	err = db.ConversationMarkRead(ctx, s.db, db.ConversationMarkReadParams{
		ConversationID: conversationID,
		ReadAt:         readAt,
	})
	if err != nil {
		return err
	}

	msgs, err := db.MessageList(ctx, s.db, db.MessageListParams{
		ConversationID: conversationID,
		Limit:          50,
	})
	if err != nil {
		return err
	}

	var unread []InboundMessage
	for _, msg := range msgs {
		if msg.IsFromMe {
			continue
		}
		if msg.Platform != conv.Platform {
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

	return platform.MarkRead(ctx, unread)
}

func (s *Service) ConversationWithMessages(ctx context.Context, conversationID string, maxHistory int) (db.Conversation, []db.Message, error) {
	conv, err := db.ConversationGet(ctx, s.db, db.ConversationGetParams{ID: conversationID})
	if err != nil {
		return db.Conversation{}, nil, err
	}

	msgs, err := db.MessageList(ctx, s.db, db.MessageListParams{
		ConversationID: conversationID,
		Limit:          maxHistory,
	})
	if err != nil {
		return db.Conversation{}, nil, err
	}

	reverseMessages(msgs)
	return conv, msgs, nil
}

func (s *Service) ResolveConversation(ctx context.Context, contactID string) (string, error) {
	contact, err := db.ContactGet(ctx, s.db, contactID)
	if err != nil {
		return "", fmt.Errorf("get contact %s: %w", contactID, err)
	}

	if len(contact.WhatsappIDs) == 0 {
		return "", fmt.Errorf("contact %s has no whatsapp id", contactID)
	}
	jid := contact.WhatsappIDs[0]

	now := time.Now()
	err = db.ConversationUpsert(ctx, s.db, db.ConversationUpsertParams{
		Platform:               "whatsapp",
		ExternalConversationID: jid,
		Kind:                   "dm",
		Title:                  contact.Name,
		LastMessageAt:          &now,
	})
	if err != nil {
		return "", fmt.Errorf("upsert conversation for contact %s: %w", contactID, err)
	}

	conv, err := db.ConversationGet(ctx, s.db, db.ConversationGetParams{
		Platform:               "whatsapp",
		ExternalConversationID: jid,
	})
	if err != nil {
		return "", fmt.Errorf("get conversation for contact %s: %w", contactID, err)
	}

	err = db.ConversationParticipantUpsert(ctx, s.db, db.ConversationParticipantUpsertParams{
		ConversationID: conv.ID,
		ContactID:      contactID,
	})
	if err != nil {
		return "", fmt.Errorf("link participant for contact %s: %w", contactID, err)
	}

	return conv.ID, nil
}

func (s *Service) ConversationHeader(ctx context.Context, conv db.Conversation) ConversationHeader {
	participants, err := db.ConversationParticipantList(ctx, s.db, conv.ID)
	if err != nil || len(participants) == 0 {
		participants = nil
	}

	title := sessionTitle(conv, participants)

	session := ConversationHeader{
		Lines: []string{
			fmt.Sprintf("id: %s", conv.ID),
			fmt.Sprintf("title: %s", title),
			fmt.Sprintf("platform: %s", conv.Platform),
			fmt.Sprintf("type: %s", conv.Kind),
		},
	}

	if conv.Topic.Valid && strings.TrimSpace(conv.Topic.String) != "" {
		session.Lines = append(session.Lines, fmt.Sprintf("topic: %s", conv.Topic.String))
	}

	if len(participants) > 0 {
		session.Lines = append(session.Lines, "participants:")
		for i, p := range participants {
			fallbackName := fmt.Sprintf("participant-%d", i+1)
			name := participantName(p, fallbackName)
			session.Lines = append(session.Lines, fmt.Sprintf("- %s (%s)", name, p.ContactID))

			detail := participantDetail(p)
			if detail != "" {
				session.Lines = append(session.Lines, fmt.Sprintf("  %s", detail))
			}

			if p.ContactID != contacts.NikContactID {
				if gaps := participantGaps(p); gaps != "" {
					session.Lines = append(session.Lines, fmt.Sprintf("  %s", gaps))
				}
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

func participantGaps(p db.ConversationParticipant) string {
	var missing []string

	if !p.ContactName.Valid || strings.TrimSpace(p.ContactName.String) == "" {
		missing = append(missing, "name")
	}
	if !p.Timezone.Valid || strings.TrimSpace(p.Timezone.String) == "" {
		missing = append(missing, "timezone")
	}
	if !p.Location.Valid || strings.TrimSpace(p.Location.String) == "" {
		missing = append(missing, "location")
	}
	if !p.OneLiner.Valid || strings.TrimSpace(p.OneLiner.String) == "" {
		missing = append(missing, "one_liner")
	}

	if len(missing) == 0 {
		return ""
	}

	return "[needs: " + strings.Join(missing, ", ") + "]"
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
			label = s.contactLabel(ctx, msg.ContactID)
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

func (s *Service) contactLabel(ctx context.Context, contactID string) string {
	if strings.TrimSpace(contactID) == "" {
		return ""
	}

	contact, err := db.ContactGet(ctx, s.db, contactID)
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

func (s *Service) FindMessage(ctx context.Context, conversationID, text, at string) (db.Message, error) {
	msgs, err := db.MessageList(ctx, s.db, db.MessageListParams{
		ConversationID: conversationID,
		Limit:          200,
	})
	if err != nil {
		return db.Message{}, fmt.Errorf("get messages: %w", err)
	}

	var matches []db.Message
	for _, msg := range msgs {
		if FormatMessageText(msg) == text && msg.SentAt.Format("15:04:05") == at {
			matches = append(matches, msg)
		}
	}

	if len(matches) == 0 {
		return db.Message{}, fmt.Errorf("no message matching text=%q time=%s", text, at)
	}
	return matches[len(matches)-1], nil
}

func (s *Service) PersistMediaResult(ctx context.Context, localPath, text string, isTranscript bool) error {
	res, err := db.MediaResolveByPath(ctx, s.db, localPath)
	if err != nil {
		return fmt.Errorf("no media record for %s", localPath)
	}

	now := time.Now()

	if isTranscript {
		_, err = db.MediaUpdate(ctx, s.db, db.MediaUpdateParams{
			ID:             res.MediaID,
			TranscriptText: &text,
			TranscribedAt:  &now,
		})
	} else {
		_, err = db.MediaUpdate(ctx, s.db, db.MediaUpdateParams{
			ID:           res.MediaID,
			DescribeText: &text,
			DescribedAt:  &now,
		})
	}
	if err != nil {
		return err
	}

	return db.SystemMessageInsert(ctx, s.db, db.SystemMessageParams{
		ConversationID:  res.ConversationID,
		Kind:            "media_processed",
		Body:            struct{ FilePath string }{localPath},
		ContextStanzaID: res.MessageID,
		SentAt:          now,
	})
}

func (s *Service) DB() *sql.DB { return s.db }

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}

	return info.Size()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	err = os.MkdirAll(filepath.Dir(dst), 0o755)
	if err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
