package whatsapp

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kciuffolo/nik/internal/messaging"
	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type Adapter struct {
	client   *Client
	receiver messaging.MessageReceiver
}

func NewAdapter(client *Client) *Adapter {
	return &Adapter{client: client}
}

func (a *Adapter) Platform() string {
	return "whatsapp"
}

func (a *Adapter) Start(_ context.Context, receiver messaging.MessageReceiver) error {
	a.receiver = receiver
	a.client.AddEventHandler(a.handleEvent)
	return nil
}

func (a *Adapter) Stop(_ context.Context) error {
	return nil
}

func (a *Adapter) Reply(ctx context.Context, externalConversationID string, body string) (messaging.OutboundMessage, error) {
	return a.client.Reply(ctx, externalConversationID, body)
}

func (a *Adapter) SendImage(ctx context.Context, externalConversationID string, imagePath string, caption string) (messaging.OutboundMessage, error) {
	return a.client.SendImage(ctx, externalConversationID, imagePath, caption)
}

func (a *Adapter) React(ctx context.Context, externalConversationID string, externalMessageID string, externalSenderID string, emoji string) error {
	return a.client.React(ctx, externalConversationID, externalMessageID, externalSenderID, emoji)
}

func (a *Adapter) SetPresence(ctx context.Context, available bool) error {
	return a.client.SetPresence(ctx, available)
}

func (a *Adapter) StartTyping(ctx context.Context, externalConversationID string) error {
	return a.client.StartTyping(ctx, externalConversationID)
}

func (a *Adapter) StopTyping(ctx context.Context, externalConversationID string) error {
	return a.client.StopTyping(ctx, externalConversationID)
}

func (a *Adapter) MarkRead(ctx context.Context, refs []messaging.InboundMessage) error {
	for _, ref := range refs {
		err := a.client.MarkRead(
			ctx,
			ref.ExternalConversationID,
			[]string{ref.ExternalMessageID},
			ref.ExternalSenderID,
			ref.SentAt,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func (a *Adapter) handleEvent(evt any) {
	switch v := evt.(type) {
	case *events.Message:
		err := a.handleMessage(v)
		if err != nil {
			slog.Error("handle whatsapp message", "pkg", "whatsapp", "error", err)
		}
	case *events.HistorySync:
		err := a.handleHistorySync(v)
		if err != nil {
			slog.Error("handle whatsapp history sync", "pkg", "whatsapp", "error", err)
		}
	case *events.GroupInfo:
		err := a.handleGroupInfo(v)
		if err != nil {
			slog.Error("handle whatsapp group info", "pkg", "whatsapp", "error", err)
		}
	}
}

func (a *Adapter) handleMessage(evt *events.Message) error {
	if a.receiver == nil {
		return nil
	}

	var editTargetMessageID string
	var kind, body string

	editType := string(evt.Info.Edit)
	if isUnknownEditType(editType) {
		panic(fmt.Sprintf("unknown whatsapp edit type %q for message %s", editType, evt.Info.ID))
	}

	if editType != "" {
		pm := evt.Message.GetProtocolMessage()
		if pm == nil {
			return nil
		}
		if k := pm.GetKey(); k != nil {
			editTargetMessageID = k.GetID()
		}
		em := pm.GetEditedMessage()
		if em != nil {
			kind, body = extractMessageContent(em)
		}
	} else {
		kind, body = extractMessageContent(evt.Message)
	}

	if kind == "text" && body == "" {
		return nil
	}

	slog.Info(
		"whatsapp inbound message",
		"pkg",
		"whatsapp",
		"chat_jid",
		evt.Info.Chat.String(),
		"sender_jid",
		evt.Info.Sender.String(),
		"message_id",
		string(evt.Info.ID),
		"kind",
		kind,
		"body_preview_20",
		previewForLog(body, 20),
		"is_group",
		evt.Info.IsGroup,
		"is_from_me",
		evt.Info.IsFromMe,
		"edit_type",
		editType,
		"is_ephemeral",
		evt.IsEphemeral,
		"is_view_once",
		evt.IsViewOnce,
	)

	conversation := messaging.Conversation{
		Platform:               "whatsapp",
		ExternalConversationID: evt.Info.Chat.String(),
		Kind:                   inferKind(evt.Info.IsGroup),
		LastMessageAt:          evt.Info.Timestamp,
	}
	err := a.receiver.ReceiveConversation(context.Background(), conversation)
	if err != nil {
		return err
	}

	if evt.Info.IsGroup {
		err = a.syncGroupMetadata(context.Background(), evt.Info.Chat.String(), evt.Info.Timestamp)
		if err != nil {
			slog.Warn("sync group metadata", "pkg", "whatsapp", "conversation", evt.Info.Chat.String(), "error", err)
		}
	}

	ci := extractMessageContextInfo(evt.Message)

	var media *mediaResult
	switch kind {
	case "image", "audio", "video", "ptv", "document", "sticker":
		media = a.client.downloadMedia(context.Background(), evt.Message, string(evt.Info.ID), kind)
	}

	senderJID := normalizeJID(evt.Info.Sender.String())
	senderIDs := []string{senderJID}
	if alt := jidString(evt.Info.SenderAlt); alt != "" {
		senderIDs = append(senderIDs, normalizeJID(alt))
	}

	msg := messaging.InboundMessage{
		Platform:               "whatsapp",
		ExternalConversationID: evt.Info.Chat.String(),
		ExternalMessageID:      string(evt.Info.ID),
		ExternalSenderID:       senderJID,
		ExternalSenderIDs:      senderIDs,
		Kind:                   kind,
		Body:                   body,
		SentAt:                 evt.Info.Timestamp,
		IsFromMe:               evt.Info.IsFromMe,
		IsGroup:                evt.Info.IsGroup,
		IsEdit:                 evt.IsEdit || editType != "",
		EditTargetMessageID:    editTargetMessageID,
		ContextStanzaID:        value(ci.stanzaID),
		ContextParticipant:     value(ci.participant),
		ContextIsForwarded:     ci.isForwarded,
		ContextForwardingScore: ci.forwardingScore,
		ContextMentionedIDs:    ci.mentionedJID,
		IsEphemeral:            evt.IsEphemeral,
		IsViewOnce:             evt.IsViewOnce,
	}

	if media != nil {
		msg.MimeType = media.mimeType
		msg.LocalPath = filepath.Join(a.client.mediaDir, media.filename)
		msg.MediaHash = media.hash
		msg.MediaSizeBytes = media.sizeBytes
	}

	if msg.IsFromMe {
		if jid := a.client.SelfJID(); jid != "" {
			msg.ExternalSenderID = jid
		}
	}
	if msg.ExternalSenderID == "" {
		return fmt.Errorf("empty external sender id for message %s", msg.ExternalMessageID)
	}

	return a.receiver.ReceiveMessage(context.Background(), msg)
}

func (a *Adapter) handleHistorySync(evt *events.HistorySync) error {
	if a.receiver == nil || evt == nil || evt.Data == nil {
		return nil
	}

	conversations := evt.Data.GetConversations()
	for _, conv := range conversations {
		conversationJID := conv.GetID()
		if conversationJID == "" {
			continue
		}

		parsedJID, err := types.ParseJID(conversationJID)
		if err != nil {
			continue
		}

		if parsedJID.Server == types.GroupServer {
			metaErr := a.syncGroupMetadata(context.Background(), conversationJID, time.Now())
			if metaErr != nil {
				slog.Warn("history sync group metadata", "pkg", "whatsapp", "conversation", conversationJID, "error", metaErr)
			}
		}

		for _, hsMsg := range conv.GetMessages() {
			webMsg := hsMsg.GetMessage()
			if webMsg == nil {
				continue
			}

			msgEvt, err := a.client.wm.ParseWebMessage(parsedJID, webMsg)
			if err != nil {
				continue
			}

			err = a.handleMessage(msgEvt)
			if err != nil {
				return err
			}
		}
	}

	return a.receiver.OnHistorySyncComplete(context.Background(), a.Platform())
}

func (a *Adapter) handleGroupInfo(evt *events.GroupInfo) error {
	if evt == nil {
		return nil
	}

	return a.syncGroupMetadata(context.Background(), evt.JID.String(), evt.Timestamp)
}

func (a *Adapter) syncGroupMetadata(ctx context.Context, conversationJID string, at time.Time) error {
	if a.receiver == nil {
		return nil
	}

	parsed, err := types.ParseJID(conversationJID)
	if err != nil {
		return fmt.Errorf("parse group jid: %w", err)
	}

	info, err := a.client.wm.GetGroupInfo(ctx, parsed)
	if err != nil {
		return fmt.Errorf("get group info: %w", err)
	}

	if info == nil {
		return nil
	}

	title := strings.TrimSpace(info.GroupName.Name)
	topic := strings.TrimSpace(info.GroupTopic.Topic)
	owner := strings.TrimSpace(info.OwnerJID.String())

	isAnnounce := info.IsAnnounce
	isLocked := info.IsLocked

	var participants []string
	seen := make(map[string]struct{})
	for _, p := range info.Participants {
		for _, j := range []types.JID{p.JID, p.PhoneNumber, p.LID} {
			s := jidString(j)
			if s == "" {
				continue
			}
			if _, ok := seen[s]; ok {
				continue
			}
			seen[s] = struct{}{}
			participants = append(participants, s)
		}
	}

	lastMessageAt := at
	if lastMessageAt.IsZero() {
		lastMessageAt = time.Now()
	}

	return a.receiver.ReceiveConversation(ctx, messaging.Conversation{
		Platform:               a.Platform(),
		ExternalConversationID: conversationJID,
		Kind:                   "group",
		Title:                  title,
		Topic:                  strPtrIfNotEmpty(topic),
		IsAnnounce:             &isAnnounce,
		IsLocked:               &isLocked,
		OwnerExternalID:        strPtrIfNotEmpty(owner),
		ParticipantExternalIDs: participants,
		LastMessageAt:          lastMessageAt,
	})
}

func isUnknownEditType(editType string) bool {
	if editType == "" {
		return false
	}

	switch editType {
	case "1", "2", "3", "7", "8":
		return false
	default:
		return true
	}
}

func inferKind(isGroup bool) string {
	if isGroup {
		return "group"
	}
	return "dm"
}

type contextInfoResult struct {
	stanzaID        *string
	participant     *string
	isForwarded     bool
	forwardingScore *int32
	mentionedJID    []string
}

func extractMessageContextInfo(msg *waProto.Message) contextInfoResult {
	if msg == nil {
		return contextInfoResult{}
	}

	if msg.GetReactionMessage() != nil {
		messageKey := msg.GetReactionMessage().GetKey()
		if messageKey != nil {
			return contextInfoResult{
				stanzaID: strPtrIfNotEmpty(messageKey.GetID()),
			}
		}
	}

	var ci *waProto.ContextInfo
	switch {
	case msg.GetExtendedTextMessage() != nil:
		ci = msg.GetExtendedTextMessage().GetContextInfo()
	case msg.GetImageMessage() != nil:
		ci = msg.GetImageMessage().GetContextInfo()
	case msg.GetAudioMessage() != nil:
		ci = msg.GetAudioMessage().GetContextInfo()
	case msg.GetVideoMessage() != nil:
		ci = msg.GetVideoMessage().GetContextInfo()
	case msg.GetPtvMessage() != nil:
		ci = msg.GetPtvMessage().GetContextInfo()
	case msg.GetDocumentMessage() != nil:
		ci = msg.GetDocumentMessage().GetContextInfo()
	case msg.GetStickerMessage() != nil:
		ci = msg.GetStickerMessage().GetContextInfo()
	case msg.GetLocationMessage() != nil:
		ci = msg.GetLocationMessage().GetContextInfo()
	case msg.GetContactMessage() != nil:
		ci = msg.GetContactMessage().GetContextInfo()
	}

	if ci == nil {
		return contextInfoResult{}
	}

	var fwdScore *int32
	if fs := ci.GetForwardingScore(); fs > 0 {
		v := int32(fs)
		fwdScore = &v
	}

	return contextInfoResult{
		stanzaID:        strPtrIfNotEmpty(ci.GetStanzaID()),
		participant:     strPtrIfNotEmpty(ci.GetParticipant()),
		isForwarded:     ci.GetIsForwarded(),
		forwardingScore: fwdScore,
		mentionedJID:    ci.GetMentionedJID(),
	}
}

func extractMessageContent(msg *waProto.Message) (kind, body string) {
	if msg == nil {
		return "text", ""
	}
	if c := msg.GetConversation(); c != "" {
		return "text", c
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil {
		return "text", ext.GetText()
	}
	if img := msg.GetImageMessage(); img != nil {
		return "image", img.GetCaption()
	}
	if audio := msg.GetAudioMessage(); audio != nil {
		return "audio", ""
	}
	if video := msg.GetVideoMessage(); video != nil {
		return "video", video.GetCaption()
	}
	if doc := msg.GetDocumentMessage(); doc != nil {
		return "document", doc.GetFileName()
	}
	if ptv := msg.GetPtvMessage(); ptv != nil {
		return "ptv", ptv.GetCaption()
	}
	if msg.GetStickerMessage() != nil {
		return "sticker", ""
	}
	if loc := msg.GetLocationMessage(); loc != nil {
		return "location", formatLocationBody(loc)
	}
	if react := msg.GetReactionMessage(); react != nil {
		return "reaction", react.GetText()
	}
	if msg.GetContactMessage() != nil {
		return "contact", msg.GetContactMessage().GetDisplayName()
	}
	if msg.GetPollCreationMessage() != nil {
		return "poll", msg.GetPollCreationMessage().GetName()
	}

	return "text", ""
}

func formatLocationBody(loc *waProto.LocationMessage) string {
	var parts []string

	if c := loc.GetComment(); c != "" {
		parts = append(parts, c)
	}

	lat := loc.GetDegreesLatitude()
	lon := loc.GetDegreesLongitude()
	if lat != 0 || lon != 0 {
		parts = append(parts, fmt.Sprintf("%.6f,%.6f", lat, lon))
	}

	if n := loc.GetName(); n != "" {
		parts = append(parts, n)
	}

	if a := loc.GetAddress(); a != "" {
		parts = append(parts, a)
	}

	return strings.Join(parts, " | ")
}

func normalizeJID(raw string) string {
	parsed, err := types.ParseJID(raw)
	if err != nil {
		return raw
	}
	return parsed.ToNonAD().String()
}

func jidString(jid types.JID) string {
	if jid.IsEmpty() {
		return ""
	}
	return jid.String()
}

func strPtrIfNotEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func value(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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
